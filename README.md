# easydns

DNS service discovery for easyrun.

## Features

- Resolves job names to task IPs via DNS
- Caches results from local easyrun agent
- Returns multiple A records for jobs with multiple tasks
- Short TTL (5s) for dynamic services

## Usage

```bash
./easydns -listen :5353 -agent http://127.0.0.1:8080 -domain easyrun.local
```

Runs on each node, queries the local easyrun agent.

## Flags

- `-listen` - DNS address to listen on (default `:5353`, use `:53` for standard DNS)
- `-agent` - Local easyrun agent address (default `http://127.0.0.1:8080`)
- `-domain` - DNS domain suffix (default `easyrun.local`)

## How it works

1. Polls local easyrun agent every 5 seconds
2. Fetches all agents and their tasks from cluster
3. Builds cache: job name -> list of IPs
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

- Cache is updated every 5 seconds
- If agent is unreachable, serves stale cache
- This ensures DNS availability even during brief outages
