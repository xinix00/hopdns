# easydns

DNS service discovery for easyrun clusters.

## Build

```bash
go build -o bin/easydns ./cmd/easydns
```

## Run

```bash
./bin/easydns -listen :5353 -agent http://127.0.0.1:8080
```

## Architecture

- `internal/dns/cache.go` - Thread-safe cache for job->IPs mapping
- `internal/dns/watcher.go` - Polls easyrun agent, updates cache
- `internal/dns/server.go` - DNS server using miekg/dns
- `cmd/easydns/main.go` - CLI entry point

## Design

KISS approach:
- Each node runs easydns
- Queries local easyrun agent (no direct leader dependency)
- Caches results for availability
- If agent down, serves stale cache
