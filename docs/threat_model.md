# Threat Model

Di mana sistem bisa salah dan dampaknya.

## Failure Modes

| Component       | Failure                      | Impact                       | Mitigation                    |
| --------------- | ---------------------------- | ---------------------------- | ----------------------------- |
| Qdrant down     | Vector search gagal          | Query 500                    | Circuit breaker, retry        |
| PostgreSQL down | Metadata tidak tersimpan     | Degraded (query masih jalan) | Graceful degrade              |
| ML Service down | Semua AI features down       | Query 500                    | Health check, restart policy  |
| Ollama timeout  | Generation lambat            | Query timeout                | Reduce context, smaller model |
| Disk penuh      | Qdrant/PostgreSQL write fail | Ingest gagal                 | Disk monitoring alert         |

## Known Weaknesses

**1. No authentication**
Semua endpoint publik. Siapapun bisa ingest paper atau query sistem.
Fix: Tambah API key middleware.

**2. Qdrant tidak di-health-check secara aktif**
Health endpoint menampilkan qdrant: "pending" bukan status real.
Fix: Tambah Qdrant ping di health handler.

**3. Query logging belum diimplementasi**
query_logs table ada di schema tapi belum diisi.
Fix: Tambah logging di query handler.

**4. No rate limiting**
Bisa di-abuse dengan banyak query sekaligus — Ollama akan overload.
Fix: Tambah rate limiter middleware di Go.

**5. Evaluation model bias**
Judge dan generator pakai model yang sama (phi3:mini).
Fix: Gunakan model berbeda untuk evaluasi.

## Chaos Test Results

| Scenario        | Behavior                           | Status                                |
| --------------- | ---------------------------------- | ------------------------------------- |
| Qdrant down     | 500 error dengan request_id        | [ERROR] Perlu 503 + pesan lebih jelas |
| PostgreSQL down | Health degraded, query masih jalan | [OK] Graceful                         |
| ML service down | Health degraded, query 500         | [OK] Detected                         |
