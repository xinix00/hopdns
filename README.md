# easydns

DNS service discovery for easyrun.

## Features

- Resolves job names to task IPs via DNS
- **Real-time updates** via SSE (Server-Sent Events) from easyrun agent
- Returns multiple A records for jobs with multiple tasks
- Short TTL (5s) for dynamic services
- Graceful fallback: reconnects automatically on SSE disconnect

## Usage

```bash
./easydns -listen :5353 -agent http://127.0.0.1:8080 -domain easyrun.local
```

Runs on each node, connects to the local easyrun agent.

## Flags

- `-listen` - DNS address to listen on (default `:5353`, use `:53` for standard DNS)
- `-agent` - Local easyrun agent address (default `http://127.0.0.1:8080`)
- `-domain` - DNS domain suffix (default `easyrun.local`)

## How it works

1. Connects to local easyrun agent via SSE (`/v1/events`)
2. Receives real-time notifications when jobs or tasks change
3. Fetches updated state and rebuilds cache: job name -> list of IPs
4. Only includes tasks in `running` state
5. Returns all IPs as A records (client can choose)

## Example

```bash
# Start easydns
./easydns -listen :5353

# Query a job
dig @localhost -p 5353 myapp.easyrun.local

# Returns:
# myapp.easyrun.local.  5  IN  A  192.168.1.10
# myapp.easyrun.local.  5  IN  A  192.168.1.20
```

## Cache behavior

- Cache is updated in real-time via SSE events
- If agent is unreachable, serves stale cache
- Automatically reconnects on SSE disconnect
- This ensures DNS availability even during brief outages
