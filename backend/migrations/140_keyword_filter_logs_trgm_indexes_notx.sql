CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_keyword_filter_logs_user_email_trgm
    ON keyword_filter_logs USING gin (user_email gin_trgm_ops);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_keyword_filter_logs_api_key_name_trgm
    ON keyword_filter_logs USING gin (api_key_name gin_trgm_ops);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_keyword_filter_logs_model_trgm
    ON keyword_filter_logs USING gin (model gin_trgm_ops);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_keyword_filter_logs_rule_name_trgm
    ON keyword_filter_logs USING gin (rule_name gin_trgm_ops);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_keyword_filter_logs_input_excerpt_trgm
    ON keyword_filter_logs USING gin (input_excerpt gin_trgm_ops);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_keyword_filter_logs_request_id_trgm
    ON keyword_filter_logs USING gin (request_id gin_trgm_ops);
