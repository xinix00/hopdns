package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"easydns/internal/dns"
)

func main() {
	listenAddr := flag.String("listen", ":5353", "DNS address to listen on (use :53 for standard DNS)")
	agentAddr := flag.String("agent", "http://127.0.0.1:8080", "Local easyrun agent address")
	domain := flag.String("domain", "easyrun.local", "DNS domain suffix")
	apiKey := flag.String("api-key", "", "API key for easyrun agent authentication")
	flag.Parse()

	log.Printf("Starting easydns on %s, agent=%s, domain=%s", *listenAddr, *agentAddr, *domain)

	cache := dns.NewCache()
	watcher := dns.NewWatcher(*agentAddr, cache, *apiKey)
	server := dns.NewServer(cache, *listenAddr, *domain)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go watcher.Run(ctx)

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
