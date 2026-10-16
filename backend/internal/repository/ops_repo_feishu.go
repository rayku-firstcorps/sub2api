package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

var _ service.OpsFeishuRepository = (*opsRepository)(nil)

func (r *opsRepository) CreateAlertEventWithFeishu(ctx context.Context, event *service.OpsAlertEvent, version string) (*service.OpsAlertEvent, error) {
	if version == "" {
		return r.CreateAlertEvent(ctx, event)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	created, err := r.createAlertEvent(ctx, tx, event)
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO ops_feishu_deliveries (event_id, kind, config_version) VALUES ($1, 'firing', $2) ON CONFLICT DO NOTHING`, created.ID, version)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return created, nil
}

func (r *opsRepository) ResolveAlertEventWithFeishu(ctx context.Context, eventID int64, resolvedAt time.Time, version string) error {
	// Enqueue recovery while updating the event atomically. The worker waits for
	// any in-flight trigger delivery before deciding whether recovery is useful.
	_, err := r.db.ExecContext(ctx, `WITH resolved AS (
		UPDATE ops_alert_events SET status = 'resolved', resolved_at = $2
		WHERE id = $1 AND status = 'firing' RETURNING id
	) INSERT INTO ops_feishu_deliveries (event_id, kind, config_version)
	SELECT id, 'resolved', $3 FROM resolved WHERE $3 <> ''
	AND EXISTS (SELECT 1 FROM ops_feishu_deliveries d WHERE d.event_id = resolved.id AND d.kind = 'firing' AND d.config_version = $3)
	ON CONFLICT DO NOTHING`, eventID, resolvedAt, version)
	return err
}

func (r *opsRepository) ClaimFeishuDelivery(ctx context.Context) (*service.OpsFeishuDelivery, error) {
	d := &service.OpsFeishuDelivery{}
	err := r.db.QueryRowContext(ctx, `WITH candidate AS (
		SELECT id FROM ops_feishu_deliveries
		WHERE (status = 'pending' AND next_attempt_at <= NOW()) OR (status = 'sending' AND lease_until < NOW())
		ORDER BY next_attempt_at, id FOR UPDATE SKIP LOCKED LIMIT 1
	) UPDATE ops_feishu_deliveries d SET status = 'sending', attempts = attempts + 1,
	lease_until = NOW() + INTERVAL '60 seconds', lease_token = $1
	FROM candidate WHERE d.id = candidate.id
	RETURNING d.id, event_id, kind, status, attempts, config_version, lease_token`, uuid.NewString()).Scan(
		&d.ID, &d.EventID, &d.Kind, &d.Status, &d.Attempts, &d.ConfigVersion, &d.LeaseToken)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return d, err
}

func (r *opsRepository) FinishFeishuDelivery(ctx context.Context, d *service.OpsFeishuDelivery, status, message string, next time.Time, attempted bool) error {
	_, err := r.db.ExecContext(ctx, `UPDATE ops_feishu_deliveries SET status = $3::varchar, last_error = $4,
		next_attempt_at = $5, lease_until = NULL, lease_token = '',
		attempts = GREATEST(0, attempts - CASE WHEN $6 THEN 0 ELSE 1 END),
		sent_at = CASE WHEN $3::varchar = 'sent' THEN NOW() ELSE sent_at END
		WHERE id = $1 AND lease_token = $2 AND status = 'sending'`, d.ID, d.LeaseToken, status, message, next, attempted)
	return err
}

func (r *opsRepository) ListFeishuDeliveries(ctx context.Context, eventIDs []int64) ([]*service.OpsFeishuDelivery, error) {
	if len(eventIDs) == 0 {
		return nil, nil
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id, event_id, kind, status, attempts, last_error, sent_at, config_version
		FROM ops_feishu_deliveries WHERE event_id = ANY($1) ORDER BY id`, pq.Array(eventIDs))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var deliveries []*service.OpsFeishuDelivery
	for rows.Next() {
		d := &service.OpsFeishuDelivery{}
		if err := rows.Scan(&d.ID, &d.EventID, &d.Kind, &d.Status, &d.Attempts, &d.LastError, &d.SentAt, &d.ConfigVersion); err != nil {
			return nil, err
		}
		deliveries = append(deliveries, d)
	}
	return deliveries, rows.Err()
}

func (r *opsRepository) ReserveFeishuSend(ctx context.Context, limit int, test bool) (bool, error) {
	// The single conditional UPDATE serializes replicas without holding a lock
	// during HTTP. At most one message/second, with an additional test cooldown.
	res, err := r.db.ExecContext(ctx, `UPDATE ops_feishu_rate_limit SET
		next_send_at = NOW() + INTERVAL '1 second',
		next_test_at = CASE WHEN $2 THEN NOW() + INTERVAL '10 seconds' ELSE next_test_at END,
		sends = CASE WHEN window_start <= NOW() - INTERVAL '1 hour' THEN 1 ELSE sends + 1 END,
		window_start = CASE WHEN window_start <= NOW() - INTERVAL '1 hour' THEN NOW() ELSE window_start END
		WHERE id = 1 AND next_send_at <= NOW() AND (NOT $2 OR next_test_at <= NOW())
		AND (window_start <= NOW() - INTERVAL '1 hour' OR sends < $1)`, limit, test)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}
