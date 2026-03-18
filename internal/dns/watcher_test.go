package dns

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"easylib"
)

func TestWatcherRefresh(t *testing.T) {
	// Mock easyrun agent
	mux := http.NewServeMux()

	var serverURL string

	// Mock /v1/agents endpoint
	mux.HandleFunc("/v1/agents", func(w http.ResponseWriter, r *http.Request) {
		agents := []easylib.Agent{
			{ID: "agent1", Endpoint: serverURL},
		}
		_ = json.NewEncoder(w).Encode(agents)
	})

	// Mock /v1/status endpoint (includes cluster_name for discovery)
	mux.HandleFunc("/v1/status", func(w http.ResponseWriter, r *http.Request) {
		status := map[string]any{
			"cluster_name": "test-cluster",
			"agents":       1,
			"total_tasks":  3,
			"tasks_by_agent": map[string][]easylib.Task{
				"agent1": {
					{ID: "task1", JobName: "myapp", State: "running"},
					{ID: "task2", JobName: "myapp", State: "running"},
					{ID: "task3", JobName: "other", State: "stopped"},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(status)
	})

	server := httptest.NewServer(mux)
	defer server.Close()
	serverURL = server.URL

	cache := NewCache()
	watcher := NewWatcher(server.URL, cache, "")
	watcher.cluster = "test-cluster" // pre-set for refresh test

	// Manually trigger refresh
	watcher.refresh()

	// Check cache - should have 127.0.0.1 for myapp
	ips := cache.GetCluster("test-cluster", "myapp")
	if len(ips) != 1 {
		t.Errorf("Expected 1 IP for myapp, got %d", len(ips))
	}

	// "other" is stopped, should not be in cache
	otherIPs := cache.GetCluster("test-cluster", "other")
	if len(otherIPs) != 0 {
		t.Errorf("Expected no IPs for stopped job, got %d", len(otherIPs))
	}
}

func TestExtractIP(t *testing.T) {
	tests := []struct {
		endpoint string
		want     string
	}{
		{"http://192.168.1.10:8080", "192.168.1.10"},
		{"https://10.0.0.1:443", "10.0.0.1"},
		{"http://127.0.0.1:8080", "127.0.0.1"},
	}

	for _, tt := range tests {
		got := extractIP(tt.endpoint)
		if got == nil {
			t.Errorf("extractIP(%q) = nil, want %s", tt.endpoint, tt.want)
			continue
		}
		if got.String() != tt.want {
			t.Errorf("extractIP(%q) = %s, want %s", tt.endpoint, got.String(), tt.want)
		}
	}
}

func TestExtractIPInvalid(t *testing.T) {
	got := extractIP("not-a-url")
	if got != nil {
		t.Errorf("extractIP(invalid) = %v, want nil", got)
	}
}

func TestWatcherNoAgents(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/agents", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]easylib.Agent{})
	})
	mux.HandleFunc("/v1/status", func(w http.ResponseWriter, r *http.Request) {
		status := map[string]any{
			"cluster_name":   "test-cluster",
			"agents":         0,
			"total_tasks":    0,
			"tasks_by_agent": map[string][]easylib.Task{},
		}
		_ = json.NewEncoder(w).Encode(status)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	cache := NewCache()
	// Pre-populate cache
	cache.Set("test-cluster", "oldapp", []net.IP{net.ParseIP("10.0.0.1")})

	watcher := NewWatcher(server.URL, cache, "")
	watcher.cluster = "test-cluster"
	watcher.refresh()

	// Cache should be cleared since no agents
	if ips := cache.GetCluster("test-cluster", "oldapp"); len(ips) != 0 {
		t.Error("Cache should be empty after refresh with no agents")
	}
}

func TestWatcherDiscoverCluster(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/status", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"cluster_name": "prod-eu",
			"agents":       1,
		})
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	cache := NewCache()
	watcher := NewWatcher(server.URL, cache, "")

	name, err := watcher.discoverCluster()
	if err != nil {
		t.Fatalf("discoverCluster() error: %v", err)
	}
	if name != "prod-eu" {
		t.Errorf("discoverCluster() = %q, want %q", name, "prod-eu")
	}
}

func TestWatcherDiscoverClusterMissing(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/status", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"agents": 1,
		})
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	cache := NewCache()
	watcher := NewWatcher(server.URL, cache, "")

	_, err := watcher.discoverCluster()
	if err == nil {
		t.Fatal("discoverCluster() should fail when cluster_name is missing")
	}
}
