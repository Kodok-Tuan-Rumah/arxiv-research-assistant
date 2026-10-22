# ARXIV RESEARCH ASSISTANT — ENGINEERING GOALS & PRODUCTION SPEC
> **Repo:** `arxiv-research-assistant`  
> **Role:** ArXiv Academic Ingestion & Paper-Scoped Retrieval Service  
> **Architecture:** Two-Tier Service (Go Fiber `:8080` + Python FastAPI `:8001`)  
> **Status:** Core ingestion/query paths have hardening and unit-test coverage; production readiness has not been established.
> **Last Updated:** 2026-10-22

---

## CORE PRINCIPLE

> **"Data integrity supersedes model speed. A paper must be stored idempotently, retrieved faithfully, and purged completely without leaving orphaned vectors."**  
> Repositori ini adalah pilar ingestion literatur dan paper-scoped RAG pada platform *Scientific Claim & Prior-Art Auditor*. Fokus utamanya adalah keandalan pipeline penarikan dokumen, sinkronisasi dua arah PostgreSQL vs Qdrant, ketahanan terhadap kegagalan jaringan (circuit breaking), dan pencatatan riwayat audit (*audit logging*).

---

## DAFTAR ISI

1. [Tujuan Sistem & Batasan Kepemilikan (System Objective & Ownership)](#1-tujuan-sistem--batasan-kepemilikan-system-objective--ownership)
2. [Arsitektur Dua Lapisan (Two-Tier Architecture)](#2-arsitektur-dua-lapisan-two-tier-architecture)
3. [Kriteria Kinerja Terukur (Measurable Success Criteria)](#3-kriteria-kinerja-terukur-measurable-success-criteria)
4. [Spesifikasi Kontrak & Siklus Hidup Dokumen (Document Lifecycle Contracts)](#4-spesifikasi-kontrak--siklus-hidup-dokumen-document-lifecycle-contracts)
5. [Registri Kegagalan & Ketahanan Sistem (Failure Mode & Resilience Registry)](#5-registri-kegagalan--ketahanan-sistem-failure-mode--resilience-registry)
6. [Aturan Operasional: Mode A vs Mode B](#6-aturan-operasional-mode-a-vs-mode-b)
7. [Roadmap Peningkatan Layanan (Phase-Gated Roadmap)](#7-roadmap-peningkatan-layanan-phase-gated-roadmap)
8. [Batas Non-Tujuan (Non-Goals)](#8-batas-non-tujuan-non-goals)

---

## 1. Tujuan Sistem & Batasan Kepemilikan (System Objective & Ownership)

### 1.1 Tanggung Jawab Repositori
1. **Penarikan & Pemrosesan Dokumen Ilmiah ArXiv:** Mengunduh berkas PDF resmi dari export.arxiv.org, mengekstrak teks via PyMuPDF (`fitz`), memvalidasi batasan ukuran ($\le 50$ MB), dan memotong teks secara deterministik.
2. **Sinkronisasi Transaksional Dua Arah:** Menjamin integritas data 1:1 antara baris metadata di PostgreSQL dan vektor titik di Qdrant (`arxiv_chunks`) dengan ID deterministik berbasis UUIDv5.
3. **Penyedia Layanan Query Tertarget Paper:** Menyediakan endpoint pencarian berbasis kemiripan kosinus (`all-MiniLM-L6-v2`) yang mendukung filter per-dokumen (`arxiv_id`).
4. **Pencatatan Riwayat & Jejak Audit:** Mencatat setiap query, jawaban, model yang digunakan, dan latensi ke dalam tabel `query_logs` secara asinkron.
5. **Antarmuka Pengguna Mandiri:** Menyediakan konsol web interaktif ([`ui/index.html`](ui/index.html)) untuk demonstrasi, pengujian langsung, dan pemantauan kesehatan servis.

### 1.2 Batasan Arsitektural (Boundary Rules)
* **Milik Repositori Ini:** Pipeline download ArXiv, chunking 512-karakter, skema database `papers`, `chunks`, dan `query_logs`, circuit breaker Go-ke-Python, metrik Prometheus spesifik service ArXiv.
* **BUKAN Milik Repositori Ini:** Orkestrasi lintas repositori (milik `mcp-gateway`), *hybrid sparse-dense search* multi-tahap atau reranking berat (milik `rag-research`), atau kerangka evaluasi konsistensi/sensitivitas tanpa LLM-as-judge (milik `llm-eval-framework`).

---

## 2. Arsitektur Dua Lapisan (Two-Tier Architecture)

```text
Klien Eksternal / mcp-gateway
              │
              ▼ (HTTP :8080)
┌─────────────────────────────────────────────────────────────────┐
│                    GO FIBER BACKEND (:8080)                     │
│  - Middleware: Request ID UUIDv4 (`X-Request-ID`), CORS, Logger │
│  - Middleware: Prometheus Metrics Adapter (`/metrics`)          │
│  - Database Repo: `papers` (CRUD) & `query_logs` (Async Writer) │
│  - Sony Circuit Breakers: `ml-query`, `ml-embed`, `ml-ingest`   │
└────────────────────────────────┬────────────────────────────────┘
                                 │
                 (HTTP Internal JSON / Unix TCP :8001)
                                 │
                                 ▼
┌─────────────────────────────────────────────────────────────────┐
│                   PYTHON ML SERVICE (:8001)                     │
│  - Lifecycle: Pre-warm `SentenceTransformer(all-MiniLM-L6-v2)`  │
│  - Router `/ingest`: Download PDF -> PyMuPDF -> Chunk -> Embed  │
│  - Router `/query`: Embed Query -> Qdrant Search -> LLM Gen     │
│  - Router `/papers/{arxiv_id}`: Idempotent Vector Purge         │
│  - LLM Switcher: Groq API (Default/Fast) vs Ollama (Local/Demo) │
└────────────────────────┬────────────────┬───────────────────────┘
                         │                │
                         ▼                ▼
            PostgreSQL (:5432)       Qdrant (:6333)
```

---

## 3. Kriteria Kinerja Terukur (Measurable Success Criteria)

| Parameter | Target Spesifikasi | Protokol Pengukuran | Status |
|---|---|---|---|
| **Deterministic Point IDs** | 100% UUIDv5 | `uuid5(NAMESPACE_URL, f"{arxiv_id}:{index}")` identik di PostgreSQL dan Qdrant. | `ENFORCED` ([`test_qdrant_ids.py`](python-ml/tests/test_qdrant_ids.py)) |
| **Saga Compensation** | 100% Rollback | Jika upsert Qdrant gagal pasca-commit Postgres, baris Postgres dihapus otomatis. | `ENFORCED` ([`test_ingest_compensation.py`](python-ml/tests/test_ingest_compensation.py)) |
| **Clean Vector Deletion** | 0 Orphaned Vectors | Hapus vektor di Qdrant terlebih dahulu; batalkan penghapusan Postgres jika purge gagal. | `ENFORCED` ([`papers_test.go`](go-backend/internal/api/papers_test.go)) |
| **Empty Retrieval Handling** | HTTP 200 OK | Saat tidak ada chunk cocok, kembalikan `insufficient_context: true` (bukan 500/404). | `ENFORCED` ([`query_test.go`](go-backend/internal/api/query_test.go)) |
| **Retrieval P95 Latency** | $\le 100$ ms | Waktu pencarian vektor Qdrant HNSW untuk top-5 chunk. | `MEASURED` di `middleware.ObserveQdrantSearch` |
| **Generation Latency (Groq)** | $\le 2.5$ s | Inferensi via `openai/gpt-oss-20b` atau `llama-3.3-70b-versatile`. | `VERIFIED` (3.1s vs Ollama 118s di CPU) |
| **Circuit Breaker Trip** | $\ge 5$ error berturut-turut | Sirkuit membuka status *Open*, mengembalikan error 503 cepat tanpa hang. | `ENFORCED` via `gobreaker` |

---

## 4. Spesifikasi Kontrak & Siklus Hidup Dokumen (Document Lifecycle Contracts)

### 4.1 Ingestion & Kompensasi Transaksi
* **Endpoint:** `POST http://localhost:8001/ingest`
* **Input Payload:**
  ```json
  {
    "arxiv_ids": ["2312.10997"],
    "chunk_size": 512,
    "overlap": 50
  }
  ```
* **Aturan Urutan Penulisan:**
  1. `pdf_parser.download_and_parse_pdf()` $\rightarrow$ batasan ukuran 50 MB, timeout 60 detik.
  2. `chunker.chunk_text()` $\rightarrow$ teks divalidasi $< 1.000.000$ karakter.
  3. `model.encode(chunks)` $\rightarrow$ vektor 384 dimensi.
  4. `compute_point_ids(arxiv_id, count)` $\rightarrow$ buat UUIDv5 deterministik.
  5. `upsert_paper(conn, paper)` + `insert_chunks(conn, paper_id, chunks, qdrant_ids)` $\rightarrow$ **Commit PostgreSQL**.
  6. `delete_by_arxiv_id(qdrant, arxiv_id)` (bersihkan vektor lama) $\rightarrow$ `upsert_chunks(qdrant, chunk_dicts)`.
  7. **Kegagalan Tahap 6:** Memicu `delete_paper(conn, paper_id)` untuk membatalkan metadata PostgreSQL.

### 4.2 Query & Pemetaan Respons
* **Endpoint:** `POST http://localhost:8080/query`
* **Input Payload:**
  ```json
  {
    "query": "What is Retrieval-Augmented Generation?",
    "top_k": 5,
    "model": "phi3:mini",
    "arxiv_id": "2312.10997"
  }
  ```
* **Respons Sukses (Ada Bukti):**
  ```json
  {
    "answer": "Retrieval-Augmented Generation (RAG) combines pre-trained language models with external retrieval mechanisms...",
    "sources": [
      {
        "chunk_id": "",
        "arxiv_id": "2312.10997",
        "text": "Retrieval-Augmented Generation (RAG) has emerged as a promising solution...",
        "score": 0.8621
      }
    ],
    "retrieval_ms": 48,
    "generation_ms": 1820,
    "model": "openai/gpt-oss-20b"
  }
  ```
* **Respons Abstensi (Konteks Kosong):**
  ```json
  {
    "answer": "No relevant chunks found.",
    "sources": [],
    "model": "phi3:mini",
    "insufficient_context": true
  }
  ```

---

## 5. Registri Kegagalan & Ketahanan Sistem (Failure Mode & Resilience Registry)

Modul ini telah diuji secara empiris menggunakan injeksi kekacauan (*chaos testing* via [`scripts/failure_inject.sh`](scripts/failure_inject.sh)). Berikut matriks perilaku sistem:

| Skenario Insiden | Gejala Teramati | Perilaku Sistem | Mitigasi yang Diimplementasikan |
|---|---|---|---|
| **S-1: Qdrant Down** | Qdrant crash atau port 6333 tidak merespons. | `/health` berstatus `degraded`; endpoint query melempar 503 setelah breaker terbuka. | Circuit breaker `ml-query` membuka sirkuit; health check aktif memantau `/healthz`. |
| **S-2: PostgreSQL Down** | Kontainer database berhenti tiba-tiba. | `/health` berstatus `degraded`; query RAG tetap melayani pembacaan; logging query gagal secara aman (*graceful degradation*). | Query logging dijalankan di goroutine terpisah dengan timeout 5 detik; kegagalan log tidak mematikan serving. |
| **S-3: ML Service Down** | Proses Python FastAPI exit (OOM atau mati). | Go backend mendeteksi kegagalan transportasi; mengembalikan error terkontrol. | Circuit breaker `gobreaker` melindungi Go backend dari penumpukan goroutine yang menggantung. |
| **S-4: Injeksi Chunk Sampah** | Vektor acak disisipkan ke koleksi Qdrant. | Similarity score terdistorsi; model menghasilkan jawaban tidak relevan. | Validator regex ketat pada `arxiv_id`; audit periodik koleksi Qdrant via `delete_by_arxiv_id`. |
| **S-5: Disk PostgreSQL Penuh** | Disk penyimpanan host mencapai kapasitas 100%. | Operasi penulisan metadata gagal. | Penolakan dini ukuran PDF di `pdf_parser` ($\le 50$ MB) dan batas teks chunker ($\le 1$ juta karakter). |

---

## 6. Aturan Operasional: Mode A vs Mode B

Untuk mencegah bentrok port dan inkonsistensi kredensial, aturan operasional berikut **wajib** dipatuhi:

| Atribut | **Mode A (Root Compose — KANONIK)** | **Mode B (Compose Lokal ArXiv)** |
|---|---|---|
| Direktori Eksekusi | Direktori root workspace (`/ai-infra-workspace`) | Direktori modul (`/arxiv-research-assistant`) |
| Perintah Mulai | `make up` | `make up` (menjalankan `infra/docker-compose.yml`) |
| Penamaan Kontainer | `ws-arxiv-go-backend`, `ws-arxiv-python-ml` | `arxiv-go-backend`, `arxiv-python-ml` |
| Sumber Kredensial | `.env` root otomatis diinjeksi | **WAJIB** `--env-file ../.env` |
| **Aturan Kunci** | **TIDAK BOLEH BERJALAN BERSAMAAN DENGAN MODE B** (Bentrok port host 8080/8001/5432/6333). |

---

## 7. Roadmap Peningkatan Layanan (Phase-Gated Roadmap)

### 🟢 Fase 1 — Standardisasi Pengujian & Lingkungan
* [x] Verifikasi 100% kelulusan Go test di `go-backend` (`health_test`, `papers_test`, `query_test`, `ml_client_test`, `request_id_test`).
* [x] Verifikasi 100% kelulusan 19 unit test di `python-ml/tests` (`test_chunker`, `test_ingest_compensation`, `test_qdrant_ids`, `test_query_embed_guard`, `test_validators`).
* [x] Menambahkan target `test-arxiv-ml` ke agregat root `make test` dengan `PYTHONPATH=.` dari direktori service.

### 🟡 Fase 2 — Kejelasan dan kepemilikan evaluasi
* [ ] Putuskan apakah endpoint `/evaluate` internal yang memakai `phi3:mini` tetap menjadi fitur eksperimen atau dipensiunkan. Jangan menyebut skornya sebagai evaluasi independen atau deterministik.
* [ ] Bila evaluasi paper melalui `llm-eval-framework` masih dibutuhkan, tetapkan satu pemilik alur dan kontrak. Hindari dua evaluator online yang hasilnya tampak sebanding padahal metodenya berbeda.
* [ ] Catat hasil uji integrasi evaluator dan statusnya; target ini belum dianggap selesai hanya karena gateway memiliki evaluator.

### 🔵 Fase 3 — Peningkatan yang perlu keputusan produk
* [ ] Evaluasi manfaat cache ArXiv. Redis workspace saat ini opsional dan cache yang terdokumentasi dimiliki RAG; jangan aktifkan Redis di ArXiv tanpa pemilik, invalidasi, dan metrik hit/miss yang jelas.
* [ ] Tinjau kebutuhan cross-paper synthesis sebagai fitur terpisah. Tetapkan provenance multi-paper dan evaluasi sebelum implementasi.

> Checklist ini adalah roadmap pengembangan, bukan deklarasi kesiapan produksi. Runtime readiness, keamanan deployment, dan acceptance kualitas harus dinilai lewat gate workspace.

---

## 8. Batas Non-Tujuan (Non-Goals)

1. **Bukan Pengganti Mesin RAG Multi-Hop Umum:** Layanan ini dioptimalkan khusus untuk literatur ilmiah terstruktur berbasis ArXiv ID, bukan mesin dokumen umum korporat (peran tersebut dipegang oleh `rag-research`).
2. **Tidak Menghasilkan Ringkasan Spekulatif:** Jika konteks paper tidak memuat jawaban, sistem menolak membuat sintesis di luar bukti teks (*strictly ungrounded output is prohibited*).
3. **Bukan Alat Scraping Masal Tak Terbatas:** Ingestion dibatasi maksimal 50 paper per panggilan API untuk melindungi kuota bandwidth ArXiv dan stabilitas memori embedding.
