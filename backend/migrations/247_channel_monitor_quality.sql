-- Optional answer-based quality probes. Existing monitors retain arithmetic checks.
ALTER TABLE channel_monitors
    ADD COLUMN IF NOT EXISTS reasoning_effort VARCHAR(32) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS prompt TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS expected_answer TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS notify_feishu BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE channel_monitor_histories
    DROP CONSTRAINT IF EXISTS channel_monitor_histories_status_check;
ALTER TABLE channel_monitor_histories
    ADD CONSTRAINT channel_monitor_histories_status_check
    CHECK (status IN ('operational', 'degraded', 'failed', 'error', 'quality_failed'));
