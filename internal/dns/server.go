package dns

import (
	"log"
	"strings"

	"github.com/miekg/dns"
)

// Server handles DNS queries for service discovery
type Server struct {
	cache  *Cache
	domain string
	addr   string
	server *dns.Server
}

// NewServer creates a new DNS server
func NewServer(cache *Cache, addr, domain string) *Server {
	if domain == "" {
		domain = "easyrun.local."
	}
	if !strings.HasSuffix(domain, ".") {
		domain += "."
	}

	return &Server{
		cache:  cache,
		domain: domain,
		addr:   addr,
	}
}

// Run starts the DNS server
func (s *Server) Run() error {
	dns.HandleFunc(s.domain, s.handleQuery)

	s.server = &dns.Server{
		Addr: s.addr,
		Net:  "udp",
	}

	log.Printf("DNS server listening on %s (domain: %s)", s.addr, s.domain)
	return s.server.ListenAndServe()
}

// Shutdown stops the DNS server
func (s *Server) Shutdown() error {
	if s.server != nil {
		return s.server.Shutdown()
	}
	return nil
}

// handleQuery handles DNS queries
func (s *Server) handleQuery(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true

	for _, q := range r.Question {
		if q.Qtype != dns.TypeA {
			continue
		}

		// Extract job name from query (e.g., "myapp.easyrun.local." -> "myapp")
		jobName := strings.TrimSuffix(q.Name, "."+s.domain)
		if jobName == q.Name {
			continue // Query doesn't match our domain
		}

		ips := s.cache.Get(jobName)
		for _, ip := range ips {
			rr := &dns.A{
				Hdr: dns.RR_Header{
					Name:   q.Name,
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    5, // Short TTL for dynamic services
				},
				A: ip,
			}
			m.Answer = append(m.Answer, rr)
		}
	}

	w.WriteMsg(m)
}
