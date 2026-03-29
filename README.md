# hopdns

DNS service discovery for hop with KISS federation.

## Features

- Resolves job names to task IPs via DNS
- **Real-time updates** via SSE (Server-Sent Events) from hop agents
- **Federation** — multi-cluster DNS, every cluster is a `-peer`
- Returns multiple A records for jobs with multiple tasks
- Short TTL (5s) for dynamic services
- Graceful fallback: reconnects automatically on SSE disconnect

## Usage

```bash
# Single cluster
./hopdns -listen :5353 -peer http://127.0.0.1:8080

# Federation: local + remote clusters (via VPN)
./hopdns -listen :5353 \
  -peer http://127.0.0.1:8080 \
  -peer http://10.0.1.100:8080 \
  -peer http://10.0.2.100:8080
```

## Flags

- `-listen` - DNS address to listen on (default `:5353`, use `:53` for standard DNS)
- `-peer` - Cluster agent endpoint (repeatable, at least one required)
- `-domain` - DNS domain suffix (default `hop.local`)
- `-api-key` - API key for hop agent authentication

## How it works

1. Connects to each peer via SSE (`/v1/events`)
2. Discovers cluster name from peer's `GET /v1/status` → `cluster_name`
3. Receives real-time notifications when jobs or tasks change
4. Fetches updated state and rebuilds cache per cluster: job name → list of IPs
5. Only includes tasks in `running` state
6. Returns all IPs as A records

## DNS Resolution

```bash
# All clusters merged
dig @localhost -p 5353 myapp.hop.local

# Specific cluster only
dig @localhost -p 5353 myapp.prod-eu.hop.local
```

| Query | Resolves to |
|-------|-------------|
| `myapp.hop.local` | IPs from **all** peers merged |
| `myapp.prod-eu.hop.local` | IPs from cluster "prod-eu" only |

## Example

```bash
# Start hopdns with 2 clusters
./hopdns -listen :5353 \
  -peer http://127.0.0.1:8080 \
  -peer http://10.0.1.100:8080

# Query a job (merged across all clusters)
dig @localhost -p 5353 myapp.hop.local
# myapp.hop.local.  5  IN  A  192.168.1.10
# myapp.hop.local.  5  IN  A  10.0.1.20

# Query a job in specific cluster
dig @localhost -p 5353 myapp.prod-eu.hop.local
# myapp.prod-eu.hop.local.  5  IN  A  192.168.1.10
```

## Cache behavior

- Cache is updated in real-time via SSE events per peer
- If a peer is unreachable, its cache is cleared (other peers unaffected)
- Automatically reconnects and re-discovers cluster name on SSE disconnect
- Duplicate IPs across clusters are deduplicated in merged queries
