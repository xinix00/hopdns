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
		domain = "internal."
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
// Format: <service>.<cluster>.<domain> → IPs from that cluster
// Example: myapp.prod-eu.hop.local → IPs for "myapp" in cluster "prod-eu"
func (s *Server) handleQuery(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true

	for _, q := range r.Question {
		if q.Qtype != dns.TypeA {
			continue
		}

		// Strip domain suffix: "myapp.prod-eu.hop.local." → "myapp.prod-eu"
		prefix := strings.TrimSuffix(q.Name, "."+s.domain)
		if prefix == q.Name {
			continue // Query doesn't match our domain
		}

		// Parse: "myapp.prod-eu" → service="myapp", cluster="prod-eu"
		service, cluster, ok := strings.Cut(prefix, ".")
		if !ok {
			continue // No cluster qualifier — ignore
		}
		ips := s.cache.GetCluster(cluster, service)

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

	// RFC 2308: add SOA to Authority section for empty responses so resolvers
	// use our MinTtl (5s) as negative cache TTL instead of their own default.
	if len(m.Answer) == 0 {
		m.Ns = append(m.Ns, &dns.SOA{
			Hdr: dns.RR_Header{
				Name:   s.domain,
				Rrtype: dns.TypeSOA,
				Class:  dns.ClassINET,
				Ttl:    5,
			},
			Ns:      "ns." + s.domain,
			Mbox:    "hostmaster." + s.domain,
			Serial:  1,
			Refresh: 5,
			Retry:   5,
			Expire:  5,
			Minttl:  5, // Negative cache TTL — match our 5s polling interval
		})
	}

	_ = w.WriteMsg(m)
}
