# go-elasticsearch-poc

A learning proof of concept demonstrating how **Elasticsearch can be used as the primary datastore behind a Go backend application**.
The application stores and queries domain data (Honda motorcycle catalog) as JSON documents directly in Elasticsearch — without MySQL or PostgreSQL.

> This project is **not** about logging, Elastic APM, OpenTelemetry, distributed tracing, or Kibana observability.

> **Current status: Phase 3 (Index & Document) — DONE.**
> - **Phase 1 (Infra):** Elasticsearch 9.5.5 single-node cluster via Docker Compose with persistent named volume and healthcheck.
> - **Phase 2 (Connection):** Official Elasticsearch Go Typed Client v9 (`github.com/elastic/go-elasticsearch/v9`).
> - **Phase 3 (Index & Document):** Index `motorcycles` with explicit strict mapping, `Motorcycle` domain model, deterministic Honda BeAT document insertion, real-time GET by ID, and verified idempotency.
> - HTTP API and search queries arrive in subsequent phases.
>
> Complete progress, architecture details, and verification evidence: [docs/ROADMAP.md](docs/ROADMAP.md).

---

## Documentation

| Document | Purpose |
|---|---|
| [docs/LEARNING_GUIDE.md](docs/LEARNING_GUIDE.md) | Comprehensive engineering guide (Bahasa Indonesia): Elasticsearch core concepts, explicit mapping, typed client, real-time GET vs search, and code architecture walkthrough |
| [docs/ROADMAP.md](docs/ROADMAP.md) | Phases 1–6: objective, scope, deliverable, definition of done, and runtime validation logs |
| [docs/TROUBLESHOOTING.md](docs/TROUBLESHOOTING.md) | Real-world troubleshooting guide for Docker Desktop, WSL2 memory pressure, swap thrashing, connection refused, and timeout handling |

---

## Architecture

**Target End-to-End Architecture (Phase 4+):**

```text
HTTP Client / Postman
        |
        v
     Go API (cmd/api, internal/handler)
        |
        v
     Service (internal/service)
        |
        v
   Repository (internal/repository/elastic)
        |
        v
  Elasticsearch (motorcycles index)
        |
        v
   JSON Document (Honda BeAT)
```

**Implemented in Phase 3:**

```text
Demo Runner (cmd/demo/main.go)
        |
        v
  Config (internal/config)
        |
        v
  Official Typed Client (internal/repository/elastic)
        |
        +---> EnsureIndex()  --> PUT /motorcycles (explicit strict mapping, 1 shard, 0 replica)
        +---> Create()       --> PUT /motorcycles/_create/honda-beat-001 (conflict protected)
        +---> Get()          --> GET /motorcycles/_doc/honda-beat-001 (real-time read)
```

### Repository Layout

```text
.
├── cmd/
│   ├── api/main.go                    # Entrypoint: Phase 2 connection check
│   └── demo/main.go                   # Entrypoint: Phase 3 index & document demo
├── internal/
│   ├── config/
│   │   ├── config.go                  # Env-based configuration & index validation
│   │   └── config_test.go             # Unit tests for config loading & validation
│   ├── model/
│   │   └── motorcycle.go              # Domain model & JSON tags
│   ├── repository/elastic/
│   │   ├── client.go                  # Elasticsearch Typed Client constructor
│   │   ├── motorcycle.go              # EnsureIndex, Create, and Get implementations
│   │   └── motorcycle_test.go         # Unit tests for mapping & error handling
│   ├── handler/                       # HTTP handlers (reserved for Phase 4)
│   └── service/                       # Business logic (reserved for Phase 4)
├── docs/
│   ├── LEARNING_GUIDE.md              # Concepts & technical explanations
│   ├── ROADMAP.md                     # Phase tracker & runtime evidence
│   └── TROUBLESHOOTING.md             # Common issues & step-by-step solutions
├── compose.yml                        # Elasticsearch 9.5.5 single-node setup
├── Makefile                           # Development task shortcuts
├── .env.example                       # Example environment variables
└── go.mod / go.sum
```

---

## Prerequisites

- Docker Desktop or Docker Engine with Docker Compose V2 (`docker compose`)
- Go (version `go 1.27.0` per `go.mod`)
- `curl`
- `make` (optional; commands have direct Go / Docker CLI equivalents)

Elasticsearch JVM heap is set to 1 GB (`-Xms1g -Xmx1g`). Allocate at least 2 GB (4 GB recommended) to Docker / WSL2.

---

## Quick Start

### 1. Start Elasticsearch

```bash
make up
# or: docker compose up -d
```

Wait until container status is `healthy`:

```bash
make ps
# or: docker compose ps
```

### 2. Verify Connection (Phase 2)

```bash
make run
# or: go run ./cmd/api
```

Expected output:
```text
Elasticsearch connected
Cluster : docker-cluster
Version : 9.5.5
```

### 3. Run Phase 3 Demo (Index & Document)

```bash
make demo
# or: go run ./cmd/demo
```

Expected output:
```text
Elasticsearch connected
Cluster : docker-cluster
Version : 9.5.5

Index  : motorcycles
Status : created (1 shard, 0 replicas, explicit mapping)

Motorcycle saved
ID     : honda-beat-001
Model  : Honda BeAT

Motorcycle fetched successfully (GET by ID)
Brand        : Honda
Model        : BeAT
Category     : matic
Engine (cc)  : 110
Transmission : automatic
Price (IDR)  : 19000000
Available    : true
Check        : stored document matches the sample
```

Run `make demo` a second time to verify **idempotency**:
```text
Index  : motorcycles
Status : already exists, mapping verified

Motorcycle already exists, left unchanged
ID     : honda-beat-001
Model  : Honda BeAT
...
```

### 4. Manual Verification via curl

```bash
# Check index status and doc count
curl "http://localhost:9200/_cat/indices?v"

# Inspect explicit mapping
curl "http://localhost:9200/motorcycles/_mapping?pretty"

# Inspect stored document by ID
curl "http://localhost:9200/motorcycles/_doc/honda-beat-001?pretty"
```

---

## Configuration

Configuration is loaded from environment variables only; `.env` files are not auto-loaded.

| Variable | Default | Purpose |
|---|---|---|
| `ELASTICSEARCH_URL` | `http://localhost:9200` | Elasticsearch HTTP endpoint (must be a valid `http`/`https` URL) |
| `ELASTICSEARCH_INDEX` | `motorcycles` | Target index name (validated against Elasticsearch naming rules) |

Override example:

```bash
ELASTICSEARCH_INDEX=motorcycles go run ./cmd/demo
```

---

## Automated Tests

Run unit tests:

```bash
go test -v ./...
go vet ./...
gofmt -l .
```

Unit tests cover:
- Environment variable configuration parsing, trimming, defaults, and index naming rule validation.
- Elasticsearch explicit mapping structure, property types, and sub-field mappings.
- Mapping compatibility comparator (detects drift or missing fields).
- Elasticsearch error type unmarshaling and unwrapping helpers.

---

## Stop and Cleanup

```bash
make down         # docker compose down     - stops container, PRESERVES volume data
make reset        # docker compose down -v  - stops container and DELETES volume data
```

> **Warning:** `make reset` permanently removes the `elasticsearch_data` volume and all indexed documents.
