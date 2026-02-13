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

// Watcher watches the local easyrun agent for state changes via SSE
type Watcher struct {
	agentAddr string
	cache     *Cache
	client    *http.Client
	interval  time.Duration
}

// NewWatcher creates a new watcher
func NewWatcher(agentAddr string, cache *Cache) *Watcher {
	return &Watcher{
		agentAddr: agentAddr,
		cache:     cache,
		client:    &http.Client{Timeout: 10 * time.Second},
		interval:  5 * time.Second,
	}
}

// Run connects to the SSE event stream and refreshes on state changes.
// On disconnect, retries after a short delay (easydns runs with retry -1).
func (w *Watcher) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		err := w.watchSSE(ctx)
		if ctx.Err() != nil {
			return
		}
		log.Printf("SSE disconnected: %v, reconnecting in %v", err, w.interval)
		select {
		case <-time.After(w.interval):
		case <-ctx.Done():
			return
		}
	}
}

// watchSSE connects to the agent's SSE stream and triggers targeted
// refreshes per job. Does a full refresh on connect to seed the cache.
// Returns on disconnect.
func (w *Watcher) watchSSE(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", w.agentAddr+"/v1/events", nil)
	if err != nil {
		return err
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
	log.Printf("SSE connected to %s/v1/events", w.agentAddr)

	// Read lines in a goroutine so we can select on timers
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
// e.g. `data: {"job":"my-api"}` → "my-api", `data: {}` → ""
func parseJobFromData(line string) string {
	data := strings.TrimPrefix(line, "data:")
	data = strings.TrimSpace(data)
	var ev struct {
		Job string `json:"job"`
	}
	_ = json.Unmarshal([]byte(data), &ev)
	return ev.Job
}


// refreshJob fetches status for a single job and updates just that entry in the cache.
func (w *Watcher) refreshJob(jobName string) {
	resp, err := w.client.Get(fmt.Sprintf("%s/v1/jobs/%s/status", w.agentAddr, jobName))
	if err != nil {
		log.Printf("Failed to fetch job status for %s: %v", jobName, err)
		w.refresh() // fallback to full
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		w.refresh() // fallback to full
		return
	}

	var status struct {
		Agents       []Agent            `json:"agents"`
		TasksByAgent map[string][]Task  `json:"tasks_by_agent"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		w.refresh()
		return
	}

	// Build agent ID -> IP map from response
	agentIPs := make(map[string]net.IP)
	for _, agent := range status.Agents {
		if ip := extractIP(agent.Endpoint); ip != nil {
			agentIPs[agent.ID] = ip
		}
	}

	// Build IPs for this job
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
			// Avoid duplicates
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

	w.cache.Set(jobName, ips)
	log.Printf("Cache updated job %s: %d IPs", jobName, len(ips))
}

// refresh fetches data from easyrun and updates the entire cache
func (w *Watcher) refresh() {
	agents, err := w.fetchAgents()
	if err != nil {
		log.Printf("Failed to fetch agents: %v", err)
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
		log.Printf("Failed to fetch cluster status: %v", err)
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

	w.cache.Update(data)
	log.Printf("Cache updated: %d jobs", len(data))
}

// fetchAgents gets agents from local easyrun
func (w *Watcher) fetchAgents() ([]Agent, error) {
	resp, err := w.client.Get(w.agentAddr + "/v1/agents")
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

// fetchClusterStatus gets all tasks from all agents via leader
func (w *Watcher) fetchClusterStatus() (map[string][]Task, error) {
	resp, err := w.client.Get(w.agentAddr + "/v1/status")
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
