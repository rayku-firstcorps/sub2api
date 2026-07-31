# Usage request context cleanup

This maintenance command clears stored request context snapshots from historical
`usage_logs` rows while retaining the usage, billing, and audit records. It is a
dry run unless `--execute` is supplied, and always requires an explicit RFC3339
cutoff.

Run from the `backend` directory with the same configuration environment used by
the server:

```sh
# Preview rows and JSON storage older than the cutoff.
go run ./cmd/cleanup-request-context --before 2026-07-30T00:00:00+08:00

# Clear the context fields in batches while preserving the usage log rows.
go run ./cmd/cleanup-request-context --before 2026-07-30T00:00:00+08:00 --execute
```

Use `--batch-size` to reduce each update transaction when the database is busy;
the allowed range is 1-500 and the default is 500. Each batch has a 15-second
timeout and a short pause before the next batch. Newer rows and rows without
stored request context are never changed.

After execution, run `VACUUM (ANALYZE) usage_logs;` during normal maintenance so
PostgreSQL can reuse the freed space. `VACUUM FULL usage_logs;` returns space to
the operating system but takes an exclusive table lock, so schedule it separately
in a maintenance window only when disk space must be reclaimed immediately.
