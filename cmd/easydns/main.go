package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"easydns/internal/dns"
)

// stringSlice implements flag.Value for repeated -peer flags
type stringSlice []string

func (s *stringSlice) String() string { return strings.Join(*s, ",") }
func (s *stringSlice) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func main() {
	listenAddr := flag.String("listen", ":5353", "DNS address to listen on (use :53 for standard DNS)")
	domain := flag.String("domain", "easyrun.local", "DNS domain suffix")
	apiKey := flag.String("api-key", "", "API key for easyrun agent authentication")
	var peers stringSlice
	flag.Var(&peers, "peer", "Cluster agent endpoint (repeatable, e.g., -peer http://127.0.0.1:8080 -peer http://10.0.1.100:8080)")
	flag.Parse()

	if len(peers) == 0 {
		log.Fatal("at least one -peer required (e.g., -peer http://127.0.0.1:8080)")
	}

	log.Printf("Starting easydns on %s, domain=%s, peers=%v", *listenAddr, *domain, peers)

	cache := dns.NewCache()
	server := dns.NewServer(cache, *listenAddr, *domain)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start a watcher for each peer (including local cluster)
	for _, endpoint := range peers {
		endpoint = strings.TrimSpace(endpoint)
		if endpoint == "" {
			continue
		}
		w := dns.NewWatcher(endpoint, cache, *apiKey)
		go w.Run(ctx)
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("Shutting down...")
		cancel()
		_ = server.Shutdown()
	}()

	if err := server.Run(); err != nil {
		log.Fatalf("DNS server error: %v", err)
	}
}
