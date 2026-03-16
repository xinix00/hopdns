package dns

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Task represents an easyrun task
type Task struct {
	ID      string `json:"id"`
	JobName string `json:"job_name"`
	State   string `json:"state"`
}

// Agent represents an easyrun agent
type Agent struct {
	ID       string `json:"id"`
	Endpoint string `json:"endpoint"`
}

// Watcher watches an easyrun cluster via SSE and keeps the cache updated.
// Every cluster (including your own) is a peer — there is no special "local" watcher.
type Watcher struct {
	agentAddr string
	cache     *Cache
	client    *http.Client
	interval  time.Duration
	apiKey    string
	cluster   string // discovered from /v1/status → cluster_name
}

// NewWatcher creates a watcher for a cluster endpoint
func NewWatcher(agentAddr string, cache *Cache, apiKey string) *Watcher {
	return &Watcher{
		agentAddr: agentAddr,
		cache:     cache,
		client:    &http.Client{Timeout: 10 * time.Second},
		interval:  5 * time.Second,
		apiKey:    apiKey,
	}
}

// get performs a GET request with API key authentication
func (w *Watcher) get(url string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	if w.apiKey != "" {
		req.Header.Set("X-API-Key", w.apiKey)
	}
	return w.client.Do(req)
}

// Run discovers the cluster name and then watches SSE for state changes.
// On disconnect, clears the cache for this cluster and retries.
func (w *Watcher) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}

		// Discover cluster name if not known yet
		if w.cluster == "" {
			name, err := w.discoverCluster()
			if err != nil {
				log.Printf("[%s] failed to discover cluster name: %v", w.agentAddr, err)
				select {
				case <-time.After(w.interval):
					continue
				case <-ctx.Done():
					return
				}
			}
			w.cluster = name
			log.Printf("[%s] cluster: %s", w.agentAddr, w.cluster)
		}

		err := w.watchSSE(ctx)
		if ctx.Err() != nil {
			return
		}

		// On disconnect, clear cache (stale data)
		w.cache.Clear(w.cluster)
		log.Printf("[%s] (%s) SSE disconnected: %v, reconnecting in %v", w.agentAddr, w.cluster, err, w.interval)

		// Re-discover cluster name on reconnect
		w.cluster = ""

		select {
		case <-time.After(w.interval):
		case <-ctx.Done():
			return
		}
	}
}

// discoverCluster fetches /v1/status and extracts cluster_name
func (w *Watcher) discoverCluster() (string, error) {
	resp, err := w.get(w.agentAddr + "/v1/status")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var status struct {
		ClusterName string `json:"cluster_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return "", err
	}
	if status.ClusterName == "" {
		return "", fmt.Errorf("remote cluster did not report cluster_name (upgrade easyrun?)")
	}
	return status.ClusterName, nil
}

// watchSSE connects to the SSE stream and triggers targeted refreshes per job.
// Does a full refresh on connect to seed the cache.
func (w *Watcher) watchSSE(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", w.agentAddr+"/v1/events", nil)
	if err != nil {
		return err
	}
	if w.apiKey != "" {
		req.Header.Set("X-API-Key", w.apiKey)
	}

	resp, err := (&http.Client{}).Do(req) // no timeout — SSE is long-lived
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &url.Error{Op: "GET", URL: w.agentAddr + "/v1/events", Err: http.ErrNotSupported}
	}

	w.refresh() // seed cache on (re)connect
	log.Printf("[%s] (%s) SSE connected", w.agentAddr, w.cluster)

	lineCh := make(chan string)
	go func() {
		defer close(lineCh)
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			lineCh <- scanner.Text()
		}
	}()

	debounce := time.NewTimer(0)
	if !debounce.Stop() {
		<-debounce.C
	}
	pending := make(map[string]struct{})

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case line, ok := <-lineCh:
			if !ok {
				return nil // stream closed
			}
			if strings.HasPrefix(line, "data:") {
				job := parseJobFromData(line)
				if job != "" {
					if len(pending) == 0 {
						debounce.Reset(500 * time.Millisecond)
					}
					pending[job] = struct{}{}
				}
			}
		case <-debounce.C:
			for job := range pending {
				w.refreshJob(job)
			}
			pending = make(map[string]struct{})
		}
	}
}

// parseJobFromData extracts the job name from an SSE data line.
// Handles both job events ({"name":"..."}) and task events ({"job":"...","event":"..."}).
func parseJobFromData(line string) string {
	data := strings.TrimPrefix(line, "data:")
	data = strings.TrimSpace(data)
	var ev struct {
		Name string `json:"name"`
		Job  string `json:"job"`
	}
	_ = json.Unmarshal([]byte(data), &ev)
	if ev.Name != "" {
		return ev.Name
	}
	return ev.Job
}

// refreshJob fetches status for a single job and updates the cache.
func (w *Watcher) refreshJob(jobName string) {
	resp, err := w.get(fmt.Sprintf("%s/v1/jobs/%s/status", w.agentAddr, jobName))
	if err != nil {
		log.Printf("[%s] (%s) failed to fetch job %s: %v", w.agentAddr, w.cluster, jobName, err)
		w.refresh()
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		w.refresh()
		return
	}

	var status struct {
		Agents       []Agent           `json:"agents"`
		TasksByAgent map[string][]Task `json:"tasks_by_agent"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		w.refresh()
		return
	}

	agentIPs := make(map[string]net.IP)
	for _, agent := range status.Agents {
		if ip := extractIP(agent.Endpoint); ip != nil {
			agentIPs[agent.ID] = ip
		}
	}

	var ips []net.IP
	for agentID, tasks := range status.TasksByAgent {
		ip := agentIPs[agentID]
		if ip == nil {
			continue
		}
		for _, task := range tasks {
			if task.State != "running" {
				continue
			}
			found := false
			for _, existing := range ips {
				if existing.Equal(ip) {
					found = true
					break
				}
			}
			if !found {
				ips = append(ips, ip)
			}
		}
	}

	w.cache.Set(w.cluster, jobName, ips)
	log.Printf("[%s] (%s) cache updated job %s: %d IPs", w.agentAddr, w.cluster, jobName, len(ips))
}

// refresh fetches all data and updates the entire cache for this cluster.
func (w *Watcher) refresh() {
	agents, err := w.fetchAgents()
	if err != nil {
		log.Printf("[%s] (%s) failed to fetch agents: %v", w.agentAddr, w.cluster, err)
		return
	}

	agentIPs := make(map[string]net.IP)
	for _, agent := range agents {
		if ip := extractIP(agent.Endpoint); ip != nil {
			agentIPs[agent.ID] = ip
		}
	}

	status, err := w.fetchClusterStatus()
	if err != nil {
		log.Printf("[%s] (%s) failed to fetch status: %v", w.agentAddr, w.cluster, err)
		return
	}

	data := make(map[string][]net.IP)
	for agentID, tasks := range status {
		ip := agentIPs[agentID]
		if ip == nil {
			continue
		}
		for _, task := range tasks {
			if task.State != "running" {
				continue
			}
			found := false
			for _, existingIP := range data[task.JobName] {
				if existingIP.Equal(ip) {
					found = true
					break
				}
			}
			if !found {
				data[task.JobName] = append(data[task.JobName], ip)
			}
		}
	}

	w.cache.Update(w.cluster, data)
	log.Printf("[%s] (%s) cache updated: %d jobs", w.agentAddr, w.cluster, len(data))
}

func (w *Watcher) fetchAgents() ([]Agent, error) {
	resp, err := w.get(w.agentAddr + "/v1/agents")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var agents []Agent
	if err := json.NewDecoder(resp.Body).Decode(&agents); err != nil {
		return nil, err
	}
	return agents, nil
}

func (w *Watcher) fetchClusterStatus() (map[string][]Task, error) {
	resp, err := w.get(w.agentAddr + "/v1/status")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var status struct {
		TasksByAgent map[string][]Task `json:"tasks_by_agent"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, err
	}
	return status.TasksByAgent, nil
}

// extractIP extracts IP from endpoint (http://ip:port -> ip)
func extractIP(endpoint string) net.IP {
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil
	}
	host := u.Hostname()
	return net.ParseIP(host)
}
