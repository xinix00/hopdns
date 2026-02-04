package dns

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/url"
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

// Watcher polls the local easyrun agent and updates cache
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

// Run starts the watcher loop
func (w *Watcher) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	// Initial fetch
	w.refresh()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.refresh()
		}
	}
}

// refresh fetches data from easyrun and updates cache
func (w *Watcher) refresh() {
	// Get agents for IP mapping
	agents, err := w.fetchAgents()
	if err != nil {
		log.Printf("Failed to fetch agents: %v", err)
		return
	}

	// Build agent ID -> IP map
	agentIPs := make(map[string]net.IP)
	for _, agent := range agents {
		ip := extractIP(agent.Endpoint)
		if ip != nil {
			agentIPs[agent.ID] = ip
		}
	}

	// Get cluster status (all tasks from all agents)
	status, err := w.fetchClusterStatus()
	if err != nil {
		log.Printf("Failed to fetch cluster status: %v", err)
		return
	}

	// Build job -> IPs map
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

			// Add IP to job's list (avoid duplicates)
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
