package dns

import (
	"net"
	"testing"

	"github.com/miekg/dns"
)

func TestServerHandleQuery(t *testing.T) {
	cache := NewCache()
	cache.Set("prod", "myapp", []net.IP{net.ParseIP("192.168.1.10"), net.ParseIP("192.168.1.20")})

	server := NewServer(cache, ":0", "easyrun.local")

	// Query: service.cluster.domain
	req := new(dns.Msg)
	req.SetQuestion("myapp.prod.easyrun.local.", dns.TypeA)

	rw := &mockResponseWriter{}
	server.handleQuery(rw, req)

	if rw.msg == nil {
		t.Fatal("No response written")
	}

	if len(rw.msg.Answer) != 2 {
		t.Errorf("Expected 2 answers, got %d", len(rw.msg.Answer))
	}

	for _, ans := range rw.msg.Answer {
		a, ok := ans.(*dns.A)
		if !ok {
			t.Error("Answer is not an A record")
			continue
		}
		if a.A.String() != "192.168.1.10" && a.A.String() != "192.168.1.20" {
			t.Errorf("Unexpected IP: %s", a.A.String())
		}
	}
}

func TestServerHandleQueryNoCluster(t *testing.T) {
	cache := NewCache()
	cache.Set("prod", "myapp", []net.IP{net.ParseIP("192.168.1.10")})

	server := NewServer(cache, ":0", "easyrun.local")

	// Query without cluster qualifier → should return 0 (not supported)
	req := new(dns.Msg)
	req.SetQuestion("myapp.easyrun.local.", dns.TypeA)

	rw := &mockResponseWriter{}
	server.handleQuery(rw, req)

	if len(rw.msg.Answer) != 0 {
		t.Errorf("Expected 0 answers for query without cluster, got %d", len(rw.msg.Answer))
	}
}

func TestServerHandleQueryWrongDomain(t *testing.T) {
	cache := NewCache()
	cache.Set("prod", "myapp", []net.IP{net.ParseIP("192.168.1.10")})

	server := NewServer(cache, ":0", "easyrun.local")

	req := new(dns.Msg)
	req.SetQuestion("myapp.other.local.", dns.TypeA)

	rw := &mockResponseWriter{}
	server.handleQuery(rw, req)

	if len(rw.msg.Answer) != 0 {
		t.Errorf("Expected 0 answers for wrong domain, got %d", len(rw.msg.Answer))
	}
}

func TestServerHandleQueryClusterSpecific(t *testing.T) {
	cache := NewCache()
	cache.Set("prod-eu", "myapp", []net.IP{net.ParseIP("10.0.0.1")})
	cache.Set("prod-us", "myapp", []net.IP{net.ParseIP("10.0.1.1")})

	server := NewServer(cache, ":0", "easyrun.local")

	// Query specific cluster
	req := new(dns.Msg)
	req.SetQuestion("myapp.prod-eu.easyrun.local.", dns.TypeA)

	rw := &mockResponseWriter{}
	server.handleQuery(rw, req)

	if len(rw.msg.Answer) != 1 {
		t.Fatalf("Expected 1 answer for cluster-specific query, got %d", len(rw.msg.Answer))
	}

	a := rw.msg.Answer[0].(*dns.A)
	if a.A.String() != "10.0.0.1" {
		t.Errorf("Expected 10.0.0.1, got %s", a.A.String())
	}
}

func TestServerHandleQueryDifferentClusters(t *testing.T) {
	cache := NewCache()
	cache.Set("prod-eu", "myapp", []net.IP{net.ParseIP("10.0.0.1")})
	cache.Set("prod-us", "myapp", []net.IP{net.ParseIP("10.0.1.1")})

	server := NewServer(cache, ":0", "easyrun.local")

	// prod-eu → only eu IP
	req := new(dns.Msg)
	req.SetQuestion("myapp.prod-eu.easyrun.local.", dns.TypeA)
	rw := &mockResponseWriter{}
	server.handleQuery(rw, req)
	if len(rw.msg.Answer) != 1 {
		t.Fatalf("prod-eu: expected 1, got %d", len(rw.msg.Answer))
	}
	if rw.msg.Answer[0].(*dns.A).A.String() != "10.0.0.1" {
		t.Errorf("prod-eu: expected 10.0.0.1, got %s", rw.msg.Answer[0].(*dns.A).A.String())
	}

	// prod-us → only us IP
	req2 := new(dns.Msg)
	req2.SetQuestion("myapp.prod-us.easyrun.local.", dns.TypeA)
	rw2 := &mockResponseWriter{}
	server.handleQuery(rw2, req2)
	if len(rw2.msg.Answer) != 1 {
		t.Fatalf("prod-us: expected 1, got %d", len(rw2.msg.Answer))
	}
	if rw2.msg.Answer[0].(*dns.A).A.String() != "10.0.1.1" {
		t.Errorf("prod-us: expected 10.0.1.1, got %s", rw2.msg.Answer[0].(*dns.A).A.String())
	}
}

// mockResponseWriter implements dns.ResponseWriter for testing
type mockResponseWriter struct {
	msg *dns.Msg
}

func (m *mockResponseWriter) LocalAddr() net.Addr       { return nil }
func (m *mockResponseWriter) RemoteAddr() net.Addr      { return nil }
func (m *mockResponseWriter) WriteMsg(msg *dns.Msg) error {
	m.msg = msg
	return nil
}
func (m *mockResponseWriter) Write([]byte) (int, error) { return 0, nil }
func (m *mockResponseWriter) Close() error              { return nil }
func (m *mockResponseWriter) TsigStatus() error         { return nil }
func (m *mockResponseWriter) TsigTimersOnly(bool)       {}
func (m *mockResponseWriter) Hijack()                   {}
