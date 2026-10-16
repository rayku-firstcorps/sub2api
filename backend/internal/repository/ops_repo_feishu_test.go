package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func feishuEventRows(now time.Time) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "rule_id", "severity", "status", "title", "description", "metric_value", "threshold_value", "dimensions", "fired_at", "resolved_at", "email_sent", "created_at"}).
		AddRow(1, 2, "P1", "firing", "Test", "", 90.0, 80.0, nil, now, nil, false, now)
}

func TestOpsFeishuEventAndOutboxAtomicity(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "commit", true: "rollback"}[fail], func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			r := &opsRepository{db: db}
			now := time.Now().UTC()
			mock.ExpectBegin()
			mock.ExpectQuery("INSERT INTO ops_alert_events").WillReturnRows(feishuEventRows(now))
			enqueue := mock.ExpectExec("INSERT INTO ops_feishu_deliveries").WithArgs(int64(1), "version")
			if fail {
				enqueue.WillReturnError(errors.New("outbox unavailable"))
				mock.ExpectRollback()
			} else {
				enqueue.WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectCommit()
			}
			_, err = r.CreateAlertEventWithFeishu(context.Background(), &service.OpsAlertEvent{RuleID: 2, FiredAt: now}, "version")
			if fail {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestOpsFeishuClaimLeaseAndFencedCompletion(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	r := &opsRepository{db: db}
	mock.ExpectQuery("(?s)WITH candidate AS .*FOR UPDATE SKIP LOCKED.*lease_until.*RETURNING").WithArgs(sqlmock.AnyArg()).WillReturnRows(
		sqlmock.NewRows([]string{"id", "event_id", "kind", "status", "attempts", "config_version", "lease_token"}).AddRow(3, 1, "firing", "sending", 1, "v", "lease"))
	d, err := r.ClaimFeishuDelivery(context.Background())
	require.NoError(t, err)
	require.Equal(t, "lease", d.LeaseToken)
	next := time.Now()
	mock.ExpectExec("(?s)UPDATE ops_feishu_deliveries.*WHERE id = \\$1 AND lease_token = \\$2 AND status = 'sending'").
		WithArgs(int64(3), "lease", "sent", "", next, true).WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, r.FinishFeishuDelivery(context.Background(), d, "sent", "", next, true))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOpsFeishuRateLimitAndRecoverySQL(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	r := &opsRepository{db: db}
	mock.ExpectExec("(?s)UPDATE ops_feishu_rate_limit.*next_test_at.*sends < \\$1").WithArgs(60, true).WillReturnResult(sqlmock.NewResult(0, 0))
	allowed, err := r.ReserveFeishuSend(context.Background(), 60, true)
	require.NoError(t, err)
	require.False(t, allowed)
	now := time.Now()
	mock.ExpectExec("(?s)WITH resolved AS.*status = 'firing'.*INSERT INTO ops_feishu_deliveries.*EXISTS.*ON CONFLICT DO NOTHING").WithArgs(int64(1), now, "v").WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, r.ResolveAlertEventWithFeishu(context.Background(), 1, now, "v"))
	require.NoError(t, mock.ExpectationsWereMet())
}
