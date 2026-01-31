package dns

import (
	"net"
	"testing"

	"github.com/miekg/dns"
)

func TestServerHandleQuery(t *testing.T) {
	cache := NewCache()
	cache.Set("myapp", []net.IP{net.ParseIP("192.168.1.10"), net.ParseIP("192.168.1.20")})

	server := NewServer(cache, ":0", "easyrun.local")

	// Create a mock DNS request
	req := new(dns.Msg)
	req.SetQuestion("myapp.easyrun.local.", dns.TypeA)

	// Create a mock response writer
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

func TestServerHandleQueryNoMatch(t *testing.T) {
	cache := NewCache()
	server := NewServer(cache, ":0", "easyrun.local")

	req := new(dns.Msg)
	req.SetQuestion("unknown.easyrun.local.", dns.TypeA)

	rw := &mockResponseWriter{}
	server.handleQuery(rw, req)

	if len(rw.msg.Answer) != 0 {
		t.Errorf("Expected 0 answers for unknown job, got %d", len(rw.msg.Answer))
	}
}

func TestServerHandleQueryWrongDomain(t *testing.T) {
	cache := NewCache()
	cache.Set("myapp", []net.IP{net.ParseIP("192.168.1.10")})

	server := NewServer(cache, ":0", "easyrun.local")

	req := new(dns.Msg)
	req.SetQuestion("myapp.other.local.", dns.TypeA)

	rw := &mockResponseWriter{}
	server.handleQuery(rw, req)

	if len(rw.msg.Answer) != 0 {
		t.Errorf("Expected 0 answers for wrong domain, got %d", len(rw.msg.Answer))
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
