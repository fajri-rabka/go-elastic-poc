# go-elasticsearch-poc

A learning proof of concept showing how **Elasticsearch can be used as the primary datastore behind a Go API**.

> **Current status: Phase 1.**
> This phase only provides a local, single-node Elasticsearch environment via Docker Compose.
> There is no Go application code yet. The Go API, service, and repository layers arrive in Phase 2.

---

## 1. Project objective

- Run Elasticsearch locally with persistent storage and a reliable health check.
- Provide a clean repository layout ready for a Go API (Phase 2) that uses Elasticsearch as its datastore.

## 2. Architecture

**Phase 1 (current):**

```
Developer
  ↓
Docker Compose
  ↓
Elasticsearch 9.5.5 (single node)
  ↓
Named Docker volume (elasticsearch_data)
```

**Future phases:**

```
Client
  ↓
Go API            (cmd/api, internal/handler)
  ↓
Service           (internal/service)
  ↓
Elasticsearch Repository (internal/repository/elastic)
  ↓
Elasticsearch
```

### Repository layout

```
.
├── cmd/api/                     # Go API entrypoint (Phase 2)
├── internal/
│   ├── config/                  # Configuration loading (Phase 2)
│   ├── handler/                 # HTTP handlers (Phase 2)
│   ├── model/                   # Domain models (Phase 2)
│   ├── repository/elastic/      # Elasticsearch repository (Phase 2)
│   └── service/                 # Business logic (Phase 2)
├── .env.example                 # Config the Go app will read
├── compose.yml                  # Local Elasticsearch
├── Makefile                     # Shortcuts for common commands
└── README.md
```

## 3. Prerequisites

- Docker Desktop or Docker Engine
- Docker Compose V2 (`docker compose`, not the legacy `docker-compose`)
- `curl`
- `make` (optional; every target has a plain `docker compose` / `curl` equivalent)
- Go is **not** required for Phase 1. It will be required from Phase 2.

Elasticsearch is configured with a 1 GB JVM heap. Give Docker at least **2 GB** of memory (more is better).

Check your tools:

```bash
docker --version
docker compose version
curl --version
```

## 4. Start Elasticsearch

```bash
make up
```

or:

```bash
docker compose up -d
```

The first start pulls the image and can take a minute or two. Elasticsearch is ready once the container reports `healthy`.

## 5. Verify container status

```bash
make ps
```

or:

```bash
docker compose ps
```

The `STATUS` column for `go-elasticsearch-poc-es` should move from `health: starting` to `healthy`.
If it stays at `starting` or becomes `unhealthy`, see [Troubleshooting](#12-troubleshooting).

## 6. Verify Elasticsearch directly

```bash
curl http://localhost:9200
```

You should get a JSON document containing:

- `name` — the node name (the container hostname)
- `cluster_name` — `docker-cluster` by default
- `cluster_uuid` — a generated identifier, different on every fresh volume
- `version.number` — `9.5.5`
- `tagline` — `"You Know, for Search"`

No username or password is needed because security is disabled for this local POC.

## 7. Verify cluster health

```bash
make health
```

or:

```bash
curl "http://localhost:9200/_cluster/health?pretty"
```

Key fields:

| Field | Meaning |
|---|---|
| `status` | `green`, `yellow`, or `red` |
| `number_of_nodes` | Should be `1` |
| `unassigned_shards` | Shards that could not be placed on a node |

**About `yellow` on a single node:**
Elasticsearch never places a replica shard on the same node as its primary.
On a single-node cluster, any index with `number_of_replicas >= 1` will have unassigned replicas, and the cluster reports `yellow`.
All primary shards are still available, so reads and writes work normally. **`yellow` is expected and acceptable for this local setup.**

A fresh cluster with no user indices typically reports `green`, but do not rely on that once indices exist.
`red` means at least one primary shard is unavailable and does need investigation.

The Docker healthcheck treats `yellow` or `green` as healthy and fails when Elasticsearch is unreachable or `red`.

## 8. View logs

```bash
make logs
```

or:

```bash
docker compose logs -f elasticsearch
```

Press `Ctrl+C` to stop following. For a fixed number of lines:

```bash
docker compose logs --tail=100 elasticsearch
```

## 9. Stop

```bash
make down
```

or:

```bash
docker compose down
```

This removes the container but **keeps** the `elasticsearch_data` volume. Indexed data is still there the next time you run `make up`.

## 10. Full reset

```bash
make reset
```

or:

```bash
docker compose down -v
```

> **Warning:** `reset` deletes the `elasticsearch_data` volume. **All indices and documents are permanently lost.**
> The next `make up` starts a brand-new, empty cluster with a new `cluster_uuid`.

Check which volumes exist:

```bash
docker volume ls | grep go-elasticsearch-poc
```

## 11. Local development security warning

`compose.yml` sets:

```yaml
- xpack.security.enabled=false
- xpack.security.autoconfiguration.enabled=false
```

This is **intentional and only for this local learning environment**. It removes TLS, authentication, and authorization so the setup stays simple.

With these settings, anyone who can reach port 9200 can read, modify, or delete all data.

- **Do not copy these settings to staging, production, or any shared environment.**
- Do not run this setup on a machine reachable from untrusted networks.
- Real deployments must enable security, use TLS, and authenticate with credentials or API keys.

The port mapping `9200:9200` binds to all host interfaces. To restrict access to your own machine only, change it to `127.0.0.1:9200:9200` in `compose.yml`.

## 12. Troubleshooting

### Inspect compose state

```bash
docker compose ps -a
docker inspect --format '{{json .State.Health}}' go-elasticsearch-poc-es
```

The second command shows recent healthcheck results and their output.

### Inspect logs

```bash
docker compose logs --tail=200 elasticsearch
```

Look for lines containing `ERROR`, `bootstrap check failure`, or `OutOfMemoryError`.

### Port 9200 already in use

Symptom: `docker compose up` fails with `bind: address already in use`.

Find what is using the port:

```bash
# Linux / WSL
ss -ltnp | grep 9200
# macOS
lsof -i :9200
```

Stop the conflicting process or container (often another local Elasticsearch), then run `make up` again.

### Container exits or keeps restarting

```bash
docker compose ps -a
docker compose logs --tail=200 elasticsearch
```

Common causes are insufficient memory and `vm.max_map_count` (both below). The restart policy is `unless-stopped`, so a failing container may restart in a loop; the logs show the actual cause.

### Insufficient Docker memory

Symptoms: container exits with code `137`, `OutOfMemoryError` in logs, or the host becomes very slow.

- Docker Desktop: *Settings → Resources → Memory*, allocate at least 2 GB (4 GB recommended).
- WSL2: memory is controlled by `%UserProfile%\.wslconfig` (`[wsl2]` section, `memory=4GB`), followed by `wsl --shutdown`.
- As a last resort for very constrained machines, lower the heap in `compose.yml`, e.g. `ES_JAVA_OPTS=-Xms512m -Xmx512m`.

### `vm.max_map_count` too low (Linux / WSL)

Elasticsearch requires a high memory-map limit. Check the current value first:

```bash
sysctl vm.max_map_count
```

If the logs contain a bootstrap error like:

```
max virtual memory areas vm.max_map_count [65530] is too low, increase to at least [1048576]
```

the required value is **1048576**.

Temporary (until reboot), on the Docker host:

```bash
sudo sysctl -w vm.max_map_count=1048576
```

Persistent on Linux:

```bash
echo 'vm.max_map_count=1048576' | sudo tee /etc/sysctl.d/99-elasticsearch.conf
sudo sysctl --system
```

**Windows + Docker Desktop + WSL2:** containers run inside the `docker-desktop` WSL VM, not in your Ubuntu (or other) distro. Depending on your setup, you may need to set the value there:

```powershell
wsl -d docker-desktop -u root sysctl -w vm.max_map_count=1048576
```

This does not survive a restart of Docker Desktop / WSL. Consult the Docker Desktop documentation for a persistent option suitable for your version.

This project never changes host kernel settings automatically; apply these changes only if Elasticsearch reports the error.

### Docker command not found inside WSL

If `docker` is missing in your WSL distro while Docker Desktop is installed on Windows, enable integration:
*Docker Desktop → Settings → Resources → WSL Integration →* enable your distro, then *Apply & restart*.
