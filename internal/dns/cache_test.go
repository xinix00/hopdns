package dns

import (
	"net"
	"testing"
)

func TestCacheSetGet(t *testing.T) {
	c := NewCache()

	ips := []net.IP{net.ParseIP("192.168.1.10"), net.ParseIP("192.168.1.20")}
	c.Set("prod", "myapp", ips)

	got := c.Get("myapp")
	if len(got) != 2 {
		t.Errorf("Get() returned %d IPs, want 2", len(got))
	}
}

func TestCacheGetEmpty(t *testing.T) {
	c := NewCache()

	got := c.Get("nonexistent")
	if got != nil {
		t.Errorf("Get() returned %v, want nil", got)
	}
}

func TestCacheGetMergesAcrossClusters(t *testing.T) {
	c := NewCache()

	c.Set("prod-eu", "myapp", []net.IP{net.ParseIP("10.0.0.1")})
	c.Set("prod-us", "myapp", []net.IP{net.ParseIP("10.0.1.1")})

	got := c.Get("myapp")
	if len(got) != 2 {
		t.Errorf("Get() returned %d IPs, want 2 (merged across clusters)", len(got))
	}
}

func TestCacheGetDeduplicatesAcrossClusters(t *testing.T) {
	c := NewCache()

	// Same IP in two clusters (edge case)
	c.Set("prod-eu", "myapp", []net.IP{net.ParseIP("10.0.0.1")})
	c.Set("prod-us", "myapp", []net.IP{net.ParseIP("10.0.0.1")})

	got := c.Get("myapp")
	if len(got) != 1 {
		t.Errorf("Get() returned %d IPs, want 1 (deduplicated)", len(got))
	}
}

func TestCacheGetCluster(t *testing.T) {
	c := NewCache()

	c.Set("prod-eu", "myapp", []net.IP{net.ParseIP("10.0.0.1")})
	c.Set("prod-us", "myapp", []net.IP{net.ParseIP("10.0.1.1")})

	got := c.GetCluster("prod-eu", "myapp")
	if len(got) != 1 {
		t.Errorf("GetCluster() returned %d IPs, want 1", len(got))
	}
	if !got[0].Equal(net.ParseIP("10.0.0.1")) {
		t.Errorf("GetCluster() returned %s, want 10.0.0.1", got[0])
	}

	got = c.GetCluster("nonexistent", "myapp")
	if got != nil {
		t.Errorf("GetCluster(nonexistent) returned %v, want nil", got)
	}
}

func TestCacheUpdate(t *testing.T) {
	c := NewCache()

	// Set initial data
	c.Set("prod", "app1", []net.IP{net.ParseIP("10.0.0.1")})

	// Update with new data (replaces entire cluster)
	newData := map[string][]net.IP{
		"app2": {net.ParseIP("10.0.0.2")},
		"app3": {net.ParseIP("10.0.0.3")},
	}
	c.Update("prod", newData)

	// Old data should be gone
	if got := c.GetCluster("prod", "app1"); got != nil {
		t.Error("app1 should be gone after Update")
	}

	// New data should be present
	if got := c.GetCluster("prod", "app2"); len(got) != 1 {
		t.Error("app2 should have 1 IP")
	}
	if got := c.GetCluster("prod", "app3"); len(got) != 1 {
		t.Error("app3 should have 1 IP")
	}
}

func TestCacheClear(t *testing.T) {
	c := NewCache()

	c.Set("prod", "myapp", []net.IP{net.ParseIP("10.0.0.1")})
	c.Set("staging", "myapp", []net.IP{net.ParseIP("10.0.1.1")})

	c.Clear("prod")

	// prod should be gone
	if got := c.GetCluster("prod", "myapp"); got != nil {
		t.Error("prod cluster should be cleared")
	}

	// staging should still be there
	if got := c.GetCluster("staging", "myapp"); len(got) != 1 {
		t.Error("staging cluster should still have data")
	}

	// Get (merged) should only return staging
	if got := c.Get("myapp"); len(got) != 1 {
		t.Errorf("Get() returned %d IPs, want 1 (only staging)", len(got))
	}
}
