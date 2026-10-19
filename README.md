# ArXiv Research Assistant

> Project boundaries and goals: [`PROJECT_CONTEXT.md`](PROJECT_CONTEXT.md) and [`PROJECT_GOALS.md`](PROJECT_GOALS.md).
> RAG system for querying academic papers — Go backend, Python ML service, and a Prometheus/Grafana observability stack.

![CI](https://github.com/malikimayzar/arxiv-research-assistant/actions/workflows/test.yml/badge.svg)
![Lint](https://github.com/malikimayzar/arxiv-research-assistant/actions/workflows/lint.yml/badge.svg)

---

## What This Is

A system that lets you query academic papers in natural language. You ingest papers from ArXiv, they get chunked, embedded, and stored in a vector database. When you ask a question, the system retrieves relevant chunks and generates an answer using a local LLM.

Built to be **auditable** (queries logged best-effort), **observable** (Prometheus + Grafana), and **resilient** (circuit breaker, documented failure scenarios).

---

## Architecture
```
Browser / CLI
     │
     ▼
Go Fiber API :8080
     │
     ├── PostgreSQL :5432   (paper metadata, chunks, query logs)
     │
     └── Python FastAPI :8001
               │
               ├── Qdrant :6333      (vector search)
               ├── Ollama :11434     (local LLM)
               └── Redis :6379       (cache — planned, not wired yet)

Prometheus :9090  ←  scrapes Go + Python metrics
Grafana :3000     ←  UI (no dashboards provisioned)
```

---

## Stack

| Layer | Tool | Why |
|-------|------|-----|
| Backend | Go + Fiber | Concurrency, explicit error handling, performance |
| ML Service | Python + FastAPI | Best ML ecosystem, clean service boundary |
| Vector DB | Qdrant | Production-grade, filterable, REST API |
| Relational DB | PostgreSQL | Metadata, chunks, query logs |
| Cache | Redis | Planned embedding/session cache — container runs, no code uses it yet |
| LLM | Ollama (phi3:mini); Groq optional | Local by default, no API cost; LLM_PROVIDER=groq switches to cloud |
| Monitoring | Prometheus + Grafana | Prometheus scrapes both services; Grafana runs without provisioned dashboards |
| CI/CD | GitHub Actions | Lint + Go tests on every push |

---

## Quick Start

**Prerequisites:** Go 1.23+, Python 3.12+, Docker, Ollama
```bash
# 1. Clone
git clone https://github.com/malikimayzar/arxiv-research-assistant.git
cd arxiv-research-assistant

# 2. Pull Ollama model
ollama pull phi3:mini

# 3. Setup everything
make setup

# 4. Terminal 1 — ML service
make run-ml

# 5. Terminal 2 — Go backend
make run

# 6. Ingest a paper
make ingest

# 7. Query
curl -X POST http://localhost:8080/query \
  -H "Content-Type: application/json" \
  -d '{"query": "What is RAG?", "top_k": 5}'
```

---

## API Reference

### Go Backend `:8080`

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Health check (pings Postgres + ML; Qdrant status is a placeholder) |
| GET | `/papers` | List ingested papers |
| GET | `/papers/:id` | Get paper by ID |
| DELETE | `/papers/:id` | Delete paper |
| POST | `/query` | Natural language query |
| GET | `/query/history` | Query logs |
| GET | `/metrics` | Prometheus metrics |

> Query contract: an empty retrieval is a valid result, not an error. When the ML
> service finds no relevant chunks, `/query` returns `200` with
> `insufficient_context: true`, `answer: "No relevant chunks found."` and an empty
> `sources` array. ML-service failures (5xx, transport errors, open circuit
> breaker) return `503` with a generic message.

### Python ML Service `:8001`

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/ingest` | Ingest ArXiv papers |
| POST | `/embed` | Generate embeddings |
| POST | `/chunk` | Chunk text |
| POST | `/query` | RAG pipeline |
| POST | `/evaluate` | Faithfulness scoring (experimental — see note below) |
| GET | `/health` | Embedding model load status |
| GET | `/metrics` | Prometheus metrics |

> Known limitation: `/evaluate` runs only against the local Ollama model; when the
> model output does not match the expected format, the score falls back to a neutral
> 0.5. Treat these scores as experimental until the evaluator is hardened.

---

## Observability

Prometheus scrapes both services. Grafana runs at `http://localhost:3000` (no dashboards provisioned yet).

| Metric | Type | What it detects |
|--------|------|-----------------|
| `http_request_duration_seconds` | Histogram | Slow endpoints |
| `http_requests_total` | Counter | Traffic + error rate |
| `qdrant_search_duration_seconds` | Histogram | Vector search performance |
| `ollama_generation_duration_seconds` | Histogram | LLM latency |
| `failure_mode_total` | Counter | correct / insufficient_ctx / error |
| `active_goroutines` | Gauge | Goroutine leak detection |
| `postgres_connections_active` | Gauge | Connection pool exhaustion |

> `query_faithfulness_score` is registered but not populated yet — evaluation is not
> wired into the query path, so the gauge would read 0.

---

## Chaos Testing

5 failure scenarios tested and documented in `docs/postmortem/`:
```bash
bash scripts/failure_inject.sh kill-qdrant       # Scenario 1: Qdrant down
bash scripts/failure_inject.sh kill-postgres     # Scenario 2: PostgreSQL down
bash scripts/failure_inject.sh kill-ml           # Scenario 3: ML service down
bash scripts/failure_inject.sh inject-garbage    # Scenario 4: Noisy data
bash scripts/failure_inject.sh fill-disk         # Scenario 5: Disk pressure

bash scripts/failure_inject.sh status            # Check system
```

The ML client's circuit breaker opens when a 60 s window has at least 5 requests with a ≥60% failure ratio; an open breaker returns an error immediately instead of waiting on a down ML service. Only transport errors and 5xx count as failures — 4xx answers (e.g. an empty retrieval) pass through without tripping the breaker.

---

## Ablation Study

Compare retrieval configurations by latency and sample answers (no quality metric yet):
```bash
make ablation
```

---

## Development
```bash
make setup      # Full setup from scratch
make run        # Start Go backend
make run-ml     # Start Python ML service
make test       # Go tests
make lint       # Go vet
make build      # Go build
make health     # Check system health
make ingest     # Ingest sample paper (2312.10997)
make ablation   # Run ablation study
make up         # Docker services up
make down       # Docker services down
make ps         # Docker services status
```

---

## Project Structure
```
arxiv-research-assistant/
├── .github/workflows/        # CI/CD — lint.yml + test.yml
├── go-backend/
│   ├── cmd/server/main.go    # Entry point
│   ├── internal/
│   │   ├── api/              # HTTP handlers (health, papers, query)
│   │   ├── client/           # ML service client + circuit breaker
│   │   ├── middleware/       # Prometheus metrics, request ID
│   │   └── repository/       # PostgreSQL (papers, query_logs, db)
│   └── migrations/           # SQL migrations
├── python-ml/
│   ├── engine/
│   │   ├── evaluation/       # Faithfulness scoring
│   │   ├── generation/       # LLM clients (Ollama, Groq)
│   │   ├── ingestion/        # ArXiv fetcher, PDF parser, chunker
│   │   └── retrieval/        # Qdrant client
│   ├── routers/              # FastAPI endpoints
│   ├── main.py               # FastAPI app + Prometheus
│   └── state.py              # Shared model state
├── infra/
│   ├── docker-compose.yml    # All services
│   ├── postgres/init.sql     # DB schema
│   └── prometheus/           # Scrape config
├── scripts/
│   ├── failure_inject.sh     # Chaos testing
│   └── ablation_study.sh     # Retrieval benchmarks
├── docs/
│   ├── architecture.md
│   ├── threat_model.md
│   └── postmortem/           # 5 failure scenario analyses
├── ui/index.html             # Web UI
└── Makefile
```

---

**Live Demo:** temporarily offline.

*Built by [Maliki Mayzar](https://github.com/malikimayzar) — 2026*
