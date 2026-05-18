INSERT INTO settings (key, value, updated_at)
VALUES ('keyword_filter_config', '', NOW())
ON CONFLICT (key) DO NOTHING;

CREATE TABLE IF NOT EXISTS keyword_filter_logs (
    id             BIGSERIAL PRIMARY KEY,
    request_id     VARCHAR(128) NOT NULL DEFAULT '',
    user_id        BIGINT REFERENCES users(id) ON DELETE SET NULL,
    user_email     VARCHAR(255) NOT NULL DEFAULT '',
    api_key_id     BIGINT REFERENCES api_keys(id) ON DELETE SET NULL,
    api_key_name   VARCHAR(100) NOT NULL DEFAULT '',
    group_id       BIGINT REFERENCES groups(id) ON DELETE SET NULL,
    group_name     VARCHAR(255) NOT NULL DEFAULT '',
    endpoint       VARCHAR(128) NOT NULL DEFAULT '',
    provider       VARCHAR(64) NOT NULL DEFAULT '',
    model          VARCHAR(255) NOT NULL DEFAULT '',
    protocol       VARCHAR(64) NOT NULL DEFAULT '',
    match_type     VARCHAR(32) NOT NULL DEFAULT '',
    rule_name      VARCHAR(128) NOT NULL DEFAULT '',
    matched_text   TEXT NOT NULL DEFAULT '',
    input_excerpt  TEXT NOT NULL DEFAULT '',
    input_hash     VARCHAR(64) NOT NULL DEFAULT '',
    action         VARCHAR(32) NOT NULL DEFAULT '',
    block_status   INT NOT NULL DEFAULT 403,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_keyword_filter_logs_created_at ON keyword_filter_logs(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_keyword_filter_logs_group_created_at ON keyword_filter_logs(group_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_keyword_filter_logs_user_created_at ON keyword_filter_logs(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_keyword_filter_logs_api_key_created_at ON keyword_filter_logs(api_key_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_keyword_filter_logs_endpoint_created_at ON keyword_filter_logs(endpoint, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_keyword_filter_logs_match_type_created_at ON keyword_filter_logs(match_type, created_at DESC);
