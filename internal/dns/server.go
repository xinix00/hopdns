package dns

import (
	"log"
	"net"
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

// handleQuery handles DNS queries.
// Supports two formats:
//   - <service>.<domain>           → merged IPs from all clusters
//   - <service>.<cluster>.<domain> → IPs from specific cluster only
func (s *Server) handleQuery(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true

	for _, q := range r.Question {
		if q.Qtype != dns.TypeA {
			continue
		}

		// Strip domain suffix: "myapp.prod-eu.easyrun.local." → "myapp.prod-eu"
		prefix := strings.TrimSuffix(q.Name, "."+s.domain)
		if prefix == q.Name {
			continue // Query doesn't match our domain
		}

		var ips []net.IP

		// Check if there's a cluster qualifier: "myapp.prod-eu" → service="myapp", cluster="prod-eu"
		if service, cluster, ok := strings.Cut(prefix, "."); ok {
			// Specific cluster: <service>.<cluster>.<domain>
			ips = s.cache.GetCluster(cluster, service)
		} else {
			// All clusters merged: <service>.<domain>
			ips = s.cache.Get(prefix)
		}

		for _, ip := range ips {
			m.Answer = append(m.Answer, &dns.A{
				Hdr: dns.RR_Header{
					Name:   q.Name,
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    5,
				},
				A: ip,
			})
		}
	}

	_ = w.WriteMsg(m)
}
