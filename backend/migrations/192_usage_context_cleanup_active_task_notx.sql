-- Keep request-context cleanup bounded to one active task across all API instances.
-- This migration runs outside a transaction because the index is built concurrently.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS usage_cleanup_tasks_active_context_mode_uq
    ON usage_cleanup_tasks ((filters->>'mode'))
    WHERE status IN ('pending', 'running')
      AND filters->>'mode' = 'clear_request_context';
