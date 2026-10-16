//go:build integration || feishu_integration

package repository

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpsFeishuPostgresLifecycle(t *testing.T) {
	ctx := context.Background()
	r := &opsRepository{db: integrationDB}
	_, err := integrationDB.ExecContext(ctx, "UPDATE ops_feishu_rate_limit SET next_send_at = NOW(), next_test_at = NOW(), window_start = NOW(), sends = 0 WHERE id = 1")
	require.NoError(t, err)
	rule, err := r.CreateAlertRule(ctx, &service.OpsAlertRule{
		Name: "Feishu integration", Enabled: true, Severity: "P1", MetricType: "error_rate",
		Operator: ">", Threshold: 5, WindowMinutes: 1, SustainedMinutes: 1, NotifyEmail: false, NotifyFeishu: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM ops_alert_events WHERE rule_id = $1", rule.ID)
		_ = r.DeleteAlertRule(ctx, rule.ID)
	})
	require.True(t, rule.NotifyFeishu)
	rule.NotifyFeishu = false
	rule.PreserveNotifyFeishu = true
	rule, err = r.UpdateAlertRule(ctx, rule)
	require.NoError(t, err)
	require.True(t, rule.NotifyFeishu, "old clients preserve the Feishu switch")
	rule.NotifyFeishu = false
	rule.PreserveNotifyFeishu = false
	rule, err = r.UpdateAlertRule(ctx, rule)
	require.NoError(t, err)
	require.False(t, rule.NotifyFeishu)
	rules, err := r.ListAlertRules(ctx)
	require.NoError(t, err)
	for _, item := range rules {
		if item.ID == rule.ID {
			require.False(t, item.NotifyFeishu)
		}
	}

	event, err := r.CreateAlertEventWithFeishu(ctx, &service.OpsAlertEvent{
		RuleID: rule.ID, Severity: "P1", Status: "firing", Title: "test", FiredAt: time.Now().UTC(),
	}, "v1")
	require.NoError(t, err)
	rows, err := r.ListFeishuDeliveries(ctx, []int64{event.ID})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "pending", rows[0].Status)

	// Two replicas compete for the same durable job: only one can claim it.
	var wg sync.WaitGroup
	jobs := make(chan *service.OpsFeishuDelivery, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); d, err := r.ClaimFeishuDelivery(ctx); jobs <- d; errs <- err }()
	}
	wg.Wait()
	close(jobs)
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	var claimed *service.OpsFeishuDelivery
	count := 0
	for d := range jobs {
		if d != nil {
			claimed = d
			count++
		}
	}
	require.Equal(t, 1, count)
	require.Equal(t, event.ID, claimed.EventID)
	stale := *claimed
	_, err = integrationDB.ExecContext(ctx, "UPDATE ops_feishu_deliveries SET lease_until = NOW() - INTERVAL '1 second' WHERE id = $1", claimed.ID)
	require.NoError(t, err)
	reclaimed, err := r.ClaimFeishuDelivery(ctx)
	require.NoError(t, err)
	require.NotNil(t, reclaimed)
	require.NotEqual(t, stale.LeaseToken, reclaimed.LeaseToken)
	require.NoError(t, r.FinishFeishuDelivery(ctx, &stale, "sent", "", time.Now(), true))
	rows, err = r.ListFeishuDeliveries(ctx, []int64{event.ID})
	require.NoError(t, err)
	require.Equal(t, "sending", rows[0].Status, "stale workers cannot complete a new lease")
	require.NoError(t, r.FinishFeishuDelivery(ctx, reclaimed, "sent", "", time.Now(), true))

	now := time.Now().UTC()
	require.NoError(t, r.ResolveAlertEventWithFeishu(ctx, event.ID, now, "v1"))
	require.NoError(t, r.ResolveAlertEventWithFeishu(ctx, event.ID, now, "v1"))
	rows, err = r.ListFeishuDeliveries(ctx, []int64{event.ID})
	require.NoError(t, err)
	require.Len(t, rows, 2, "recovery enqueue is idempotent")
	require.NotNil(t, rows[0].SentAt)
	require.Equal(t, "resolved", rows[1].Kind)
	restored, err := r.GetAlertEventByID(ctx, event.ID)
	require.NoError(t, err)
	require.Equal(t, "resolved", restored.Status)

	allowed, err := r.ReserveFeishuSend(ctx, 1, true)
	require.NoError(t, err)
	require.True(t, allowed)
	allowed, err = r.ReserveFeishuSend(ctx, 1, true)
	require.NoError(t, err)
	require.False(t, allowed)
}
