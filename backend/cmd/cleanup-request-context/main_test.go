package main

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestCleanupDryRun(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	before := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`SELECT COUNT\(\*\), COALESCE\(SUM\(pg_column_size\(request_context_json\)\), 0\)`).
		WithArgs(before).
		WillReturnRows(sqlmock.NewRows([]string{"count", "bytes"}).AddRow(12, 4096))

	stats, err := cleanup(context.Background(), db, before, 1000, false)
	require.NoError(t, err)
	require.Equal(t, cleanupStats{matched: 12, storedBytes: 4096}, stats)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCleanupExecuteClearsContextInBatches(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	before := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`SELECT id, pg_column_size\(request_context_json\)`).
		WithArgs(int64(0), before, 2).
		WillReturnRows(sqlmock.NewRows([]string{"id", "bytes"}).AddRow(10, 100).AddRow(11, 200))
	mock.ExpectExec(`UPDATE usage_logs`).
		WithArgs(sqlmock.AnyArg(), before).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectQuery(`SELECT id, pg_column_size\(request_context_json\)`).
		WithArgs(int64(11), before, 2).
		WillReturnRows(sqlmock.NewRows([]string{"id", "bytes"}))

	stats, err := cleanup(context.Background(), db, before, 2, true)
	require.NoError(t, err)
	require.Equal(t, cleanupStats{matched: 2, cleared: 2, storedBytes: 300}, stats)
	require.NoError(t, mock.ExpectationsWereMet())
}
