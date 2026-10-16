ALTER TABLE ops_alert_rules ADD COLUMN IF NOT EXISTS notify_feishu BOOLEAN NOT NULL DEFAULT true;

CREATE TABLE IF NOT EXISTS ops_feishu_deliveries (
    id BIGSERIAL PRIMARY KEY,
    event_id BIGINT NOT NULL REFERENCES ops_alert_events(id) ON DELETE CASCADE,
    kind VARCHAR(16) NOT NULL CHECK (kind IN ('firing', 'resolved')),
    config_version TEXT NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'sending', 'sent', 'failed', 'cancelled')),
    attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    lease_until TIMESTAMPTZ,
    lease_token TEXT NOT NULL DEFAULT '',
    last_error TEXT NOT NULL DEFAULT '',
    sent_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (event_id, kind)
);
CREATE INDEX IF NOT EXISTS idx_ops_feishu_deliveries_pending
    ON ops_feishu_deliveries(next_attempt_at, id) WHERE status IN ('pending', 'sending');

-- Shared by all application replicas, including the test-message endpoint.
CREATE TABLE IF NOT EXISTS ops_feishu_rate_limit (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    next_send_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    next_test_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    window_start TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sends INTEGER NOT NULL DEFAULT 0
);
INSERT INTO ops_feishu_rate_limit (id) VALUES (1) ON CONFLICT DO NOTHING;
