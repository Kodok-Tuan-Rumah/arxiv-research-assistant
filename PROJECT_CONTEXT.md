# PROJECT CONTEXT — ArXiv Research Assistant
> **Repo:** `arxiv-research-assistant`  
> **Runtime Snapshot:** 2026-10-19  
> **Integration Status:** Integrated with `ws-net` under root compose (Mode A)  

---

## 1. RUNTIME SNAPSHOT & BOUNDARIES

Modul ini berjalan sebagai layanan dua lapis (*two-tier service*):

* **Go Backend (`:8080`):** Menggunakan framework Go Fiber v2. Berperan sebagai pintu masuk HTTP, memegang koneksi PostgreSQL via `sqlx`, mengekspos endpoint Prometheus `/metrics`, dan mengontrol circuit breaker ke Python ML.
* **Python ML Service (`:8001`):** Menggunakan FastAPI + Uvicorn. Memuat embedding model `sentence-transformers/all-MiniLM-L6-v2` saat startup, mengelola koleksi Qdrant `arxiv_chunks`, serta mengarahkan generasi jawaban ke Groq API (default: `openai/gpt-oss-20b`) atau daemon Ollama lokal (`phi3:mini`).

---

## 2. STATUS INTEGRASI WORKSPACE

* **Mode A (Root Compose - KANONIK):**
  - Container Go backend: `ws-arxiv-go-backend` (port host: `8080`).
  - Container Python ML: `ws-arxiv-python-ml` (port host: `8001`).
  - Terhubung ke `ws-net` bersama `ws-postgres`, `ws-qdrant`, `ws-ollama`, dan `ws-mcp-gateway`.
* **Mode B (Lokal Compose):**
  - Menggunakan [`infra/docker-compose.yml`](infra/docker-compose.yml).
  - Khusus untuk pengujian terisolasi fitur arXiv tanpa menyalakan modul workspace lain.

---

## 3. STATUS PENGUJIAN UNIT (TEST STATUS)

1. **Go Backend Tests:** 100% PASS.
   - `health_test.go`: 3 skenario status kesehatan (no DB, Qdrant sehat, Qdrant sakit).
   - `papers_test.go`: Pengujian penghapusan vektor sebelum baris database dihapus.
   - `query_test.go`: Validasi propagasi `X-Request-ID` dan pemetaan 404 menjadi status 200 `insufficient_context`.
   - `ml_client_test.go`: Pengujian aktivasi circuit breaker pada respons 5xx dan kegagalan transportasi.
   - `request_id_test.go`: Validasi middleware UUID injection.
2. **Python ML Tests:** 100% PASS (19 tests).
   - `test_chunker.py`: Batasan ukuran chunk dan validasi overlap.
   - `test_ingest_compensation.py`: Verifikasi transaksi kompensasi rollback PostgreSQL saat Qdrant gagal.
   - `test_qdrant_ids.py`: Validasi ID deterministik UUIDv5 dan penghapusan vektor usang.
   - `test_query_embed_guard.py`: Pengujian guard ketersediaan model embedding.
   - `test_validators.py`: Validasi format regex ArXiv ID dan pembatasan muatan request.

---

## 4. RUJUKAN DOKUMENTASI TERKAIT

* Spesifikasi Rekayasa Lengkap: [`PROJECT_GOALS.md`](PROJECT_GOALS.md)
* Desain Arsitektur Sistem: [`docs/architecture.md`](docs/architecture.md)
* Model Ancaman & Skenario Kegagalan: [`docs/threat_model.md`](docs/threat_model.md)
* Laporan Pasca-Insiden: [`docs/postmortem/`](docs/postmortem/)
