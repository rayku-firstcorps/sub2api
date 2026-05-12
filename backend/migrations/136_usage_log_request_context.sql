-- Store sanitized request context snapshots for successful usage logs.
ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS request_context_json JSONB,
    ADD COLUMN IF NOT EXISTS request_context_truncated BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS request_context_bytes INTEGER;

COMMENT ON COLUMN usage_logs.request_context_json IS 'Sanitized, size-bounded client request body snapshot for context inspection';
COMMENT ON COLUMN usage_logs.request_context_truncated IS 'Whether request_context_json was truncated before storage';
COMMENT ON COLUMN usage_logs.request_context_bytes IS 'Original client request body size in bytes';
