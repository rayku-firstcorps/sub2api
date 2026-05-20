INSERT INTO settings (key, value, updated_at)
VALUES ('keyword_filter_enabled', 'false', NOW())
ON CONFLICT (key) DO NOTHING;
