# go-elasticsearch-poc

A learning proof of concept demonstrating how **Elasticsearch can be used as the primary datastore behind a Go backend application**.
The application stores and queries domain data (Honda motorcycle catalog) as JSON documents directly in Elasticsearch — without MySQL or PostgreSQL.

> This project is **not** about logging, Elastic APM, OpenTelemetry, distributed tracing, or Kibana observability.

> **Current status: Phase 4 (Motorcycle REST API CRUD) — DONE.**
> - **Phase 1 (Infra):** Elasticsearch 9.5.5 single-node cluster via Docker Compose with persistent named volume and healthcheck.
> - **Phase 2 (Connection):** Official Elasticsearch Go Typed Client v9 (`github.com/elastic/go-elasticsearch/v9`).
> - **Phase 3 (Index & Document):** Index `motorcycles` with explicit strict mapping, `Motorcycle` domain model, sample Honda BeAT document, real-time GET by ID, and verified idempotency.
> - **Phase 4 (REST API CRUD):** Full HTTP REST API (`net/http`, Go 1.22+ routing) covering Create (POST), Read (GET), Replace (PUT), Delete (DELETE), and Health check, backed by optimistic concurrency control and strict business validation.
> - Search and filtering arrive in Phase 5.
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
     Go API (cmd/api)
        |
        v
     Handler (internal/handler)
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
   JSON Document (Honda catalog)
```

**Implemented in Phase 4:**

```text
HTTP Client / curl / Postman
  │
  ├── GET    /health           --> 200 {"status":"ok"}
  ├── POST   /motorcycles      --> 201 Created (validates brand=Honda, unique ID, stores doc)
  ├── GET    /motorcycles/{id} --> 200 OK (real-time read by _id)
  ├── PUT    /motorcycles/{id} --> 200 OK (full replacement with optimistic concurrency lock)
  └── DELETE /motorcycles/{id} --> 204 No Content (removes doc by _id)
```

### Repository Layout

```text
.
├── cmd/
│   ├── api/main.go                    # Entrypoint: Phase 4 REST API server (make run)
│   └── demo/main.go                   # Entrypoint: Phase 3 index & document demo (make demo)
├── internal/
│   ├── config/
│   │   ├── config.go                  # Configuration (ES URL, index, HTTP addr) & index validator
│   │   └── config_test.go             # Unit tests for config loading & validation
│   ├── handler/
│   │   ├── motorcycle.go              # HTTP handlers, strict JSON decoder, status mapper
│   │   └── motorcycle_test.go         # HTTP endpoint tests with net/http/httptest
│   ├── model/
│   │   └── motorcycle.go              # Domain model & JSON tags
│   ├── repository/elastic/
│   │   ├── client.go                  # Elasticsearch Typed Client constructor
│   │   ├── motorcycle.go              # EnsureIndex, Create, Get, Replace, Delete operations
│   │   └── motorcycle_test.go         # Mapping & error type unit tests
│   └── service/
│       ├── motorcycle.go              # Business validation rules & repository coordinator
│       └── motorcycle_test.go         # Unit tests for domain business logic
├── docs/
│   ├── LEARNING_GUIDE.md              # Technical explanations & 20 core concepts
│   ├── ROADMAP.md                     # Phase tracker & runtime evidence logs
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

### 2. Start REST API Server (Phase 4)

```bash
make run
# or: go run ./cmd/api
```

The server starts and listens on `http://127.0.0.1:8080` (configured via `HTTP_ADDR`).
On startup, it verifies connectivity to Elasticsearch and ensures the `motorcycles` index is ready.
Press `Ctrl+C` for graceful shutdown.

### 3. Test REST API Endpoints

```bash
# Health check
curl -i http://127.0.0.1:8080/health

# Create a new Honda motorcycle
curl -i -X POST http://127.0.0.1:8080/motorcycles \
  -H "Content-Type: application/json" \
  -d '{
    "id": "honda-scoopy-api-001",
    "brand": "Honda",
    "model_name": "Scoopy",
    "category": "matic",
    "engine_cc": 110,
    "transmission": "automatic",
    "price_idr": 22000000,
    "available": true,
    "description": "Motor matic Honda untuk kebutuhan harian"
  }'

# Read motorcycle by ID
curl -i http://127.0.0.1:8080/motorcycles/honda-scoopy-api-001

# Full replacement (PUT)
curl -i -X PUT http://127.0.0.1:8080/motorcycles/honda-scoopy-api-001 \
  -H "Content-Type: application/json" \
  -d '{
    "brand": "Honda",
    "model_name": "Scoopy Prestige",
    "category": "matic",
    "engine_cc": 110,
    "transmission": "automatic",
    "price_idr": 23500000,
    "available": true,
    "description": "Motor matic Honda edisi Prestige"
  }'

# Delete motorcycle
curl -i -X DELETE http://127.0.0.1:8080/motorcycles/honda-scoopy-api-001
```

### 4. Run Phase 3 Demo (Index & Document Verification)

```bash
make demo
# or: go run ./cmd/demo
```

Verifies the deterministic sample document `honda-beat-001` without affecting API runtime.

---

## Configuration

Configuration is loaded from environment variables only; `.env` files are not auto-loaded.

| Variable | Default | Purpose |
|---|---|---|
| `ELASTICSEARCH_URL` | `http://localhost:9200` | Elasticsearch HTTP endpoint (must be a valid `http`/`https` URL) |
| `ELASTICSEARCH_INDEX` | `motorcycles` | Target index name (validated against Elasticsearch naming rules) |
| `HTTP_ADDR` | `127.0.0.1:8080` | Local HTTP server bind address and port |

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
