-- Additional query_logs columns for observability and evaluation
ALTER TABLE query_logs ADD COLUMN IF NOT EXISTS faithfulness       FLOAT;
ALTER TABLE query_logs ADD COLUMN IF NOT EXISTS failure_mode       VARCHAR(50);
ALTER TABLE query_logs ADD COLUMN IF NOT EXISTS retrieval_strategy VARCHAR(50);
ALTER TABLE query_logs ADD COLUMN IF NOT EXISTS chunk_size_used    INT;
ALTER TABLE query_logs ADD COLUMN IF NOT EXISTS session_id         VARCHAR(100);

CREATE INDEX IF NOT EXISTS idx_query_logs_failure_mode ON query_logs(failure_mode);
