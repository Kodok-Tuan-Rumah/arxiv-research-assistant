-- Correlate backend query logs with the gateway audit row.
-- The gateway sends X-Request-ID downstream; storing it here makes
-- gateway_query_logs and query_logs joinable for one user request.
ALTER TABLE query_logs ADD COLUMN IF NOT EXISTS request_id TEXT;

CREATE INDEX IF NOT EXISTS idx_query_logs_request_id ON query_logs(request_id);
