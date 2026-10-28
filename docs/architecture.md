# System Architecture

## Overview

ArXiv Research Assistant adalah sistem RAG (Retrieval Augmented Generation) yang dibangun dengan prinsip:

- Setiap service punya tanggung jawab yang jelas
- Semua failure modes terdokumentasi
- Sistem bisa dimonitor dan di-debug secara independen

## Layer Architecture

```
CLIENT (curl / browser)
    │
    ▼
GO BACKEND :8080
    ├── Auth Middleware (API Key)
    ├── Request ID Middleware
    ├── Metrics Middleware (Prometheus)
    ├── Rate Limiter
    │
    ├── GET  /health     → cek semua service
    ├── POST /query      → RAG query pipeline
    └── GET  /metrics    → Prometheus scrape
    │
    ▼
PYTHON ML SERVICE :8001
    ├── POST /embed      → sentence-transformers
    ├── POST /chunk      → text chunker
    ├── POST /ingest     → ArXiv fetch + store
    ├── POST /query      → retrieve + generate
    └── POST /evaluate   → faithfulness scoring
    │
    ├── Qdrant :6333     → vector search
    └── Ollama :11434    → LLM generation

STORAGE
    ├── PostgreSQL :5432 → papers, chunks, query_logs
    ├── Qdrant :6333     → 384-dim vectors (all-MiniLM-L6-v2)
    └── Redis :6379      → cache (future)

MONITORING
    ├── Prometheus :9090 → metrics collection
    └── Grafana :3000    → dashboards + alerting
```

## Design Decisions

**Go untuk API layer** — concurrency native, statically typed, performa tinggi. Setiap request di-handle dalam goroutine terpisah.

**Python untuk ML layer** — ekosistem terbaik untuk ML. Service boundary yang jelas memungkinkan swap model tanpa ubah Go code.

**Qdrant bukan FAISS** — production-grade vector DB dengan REST API, filtering, dan persistence. FAISS adalah file lokal yang tidak bisa di-scale.

**Request ID di setiap request** — setiap request punya UUID yang di-propagate ke semua service. Memungkinkan distributed tracing.

## Data Flow: Query Pipeline

1. User kirim `POST /query` ke Go backend
2. Go forward ke Python ML `/query`
3. Python embed query dengan all-MiniLM-L6-v2 (384 dim)
4. Python search Qdrant untuk top-k chunks
5. Python kirim query + chunks ke Ollama
6. Ollama generate jawaban
7. Response dikembalikan ke user dengan sources + latency

## Embedding Model

Model: `all-MiniLM-L6-v2`

- Dimensi: 384
- Distance metric: Cosine similarity
- Alasan: Ringan, cepat, dan performa baik untuk semantic search
