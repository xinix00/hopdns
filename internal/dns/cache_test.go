package dns

import (
	"net"
	"testing"
)

func TestCacheSetGet(t *testing.T) {
	c := NewCache()

	ips := []net.IP{net.ParseIP("192.168.1.10"), net.ParseIP("192.168.1.20")}
	c.Set("myapp", ips)

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

func TestCacheUpdate(t *testing.T) {
	c := NewCache()

	// Set initial data
	c.Set("app1", []net.IP{net.ParseIP("10.0.0.1")})

	// Update with new data
	newData := map[string][]net.IP{
		"app2": {net.ParseIP("10.0.0.2")},
		"app3": {net.ParseIP("10.0.0.3")},
	}
	c.Update(newData)

	// Old data should be gone
	if got := c.Get("app1"); got != nil {
		t.Error("app1 should be gone after Update")
	}

	// New data should be present
	if got := c.Get("app2"); len(got) != 1 {
		t.Error("app2 should have 1 IP")
	}
	if got := c.Get("app3"); len(got) != 1 {
		t.Error("app3 should have 1 IP")
	}
}

func TestCacheGetAll(t *testing.T) {
	c := NewCache()

	c.Set("app1", []net.IP{net.ParseIP("10.0.0.1")})
	c.Set("app2", []net.IP{net.ParseIP("10.0.0.2")})

	all := c.GetAll()
	if len(all) != 2 {
		t.Errorf("GetAll() returned %d entries, want 2", len(all))
	}
}
