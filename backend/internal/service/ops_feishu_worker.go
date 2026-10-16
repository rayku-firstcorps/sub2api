package service

import (
	"context"
	"errors"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

func (s *OpsService) TestFeishuNotification(ctx context.Context) error {
	if s.feishu == nil || s.feishu.repo == nil {
		return errors.New("Feishu service unavailable")
	}
	cfg, err := s.feishu.loadConfig(ctx)
	if err != nil {
		return err
	}
	if _, _, err := s.feishu.credentials(cfg); err != nil {
		return feishuConfigError(err.Error())
	}
	allowed, err := s.feishu.repo.ReserveFeishuSend(ctx, cfg.RateLimitPerHour, true)
	if err != nil {
		return errors.New("Feishu rate limiter unavailable")
	}
	if !allowed {
		return infraerrors.New(429, "FEISHU_RATE_LIMITED", "Feishu send limit reached; try again later")
	}
	if err := s.feishu.send(ctx, cfg, "[Sub2API] 飞书告警测试\n强制签名已启用。\n"+time.Now().UTC().Format(time.RFC3339)); err != nil {
		return infraerrors.New(502, "FEISHU_SEND_FAILED", err.Error())
	}
	return nil
}

func feishuEligible(cfg *opsFeishuStoredConfig, runtime *OpsAlertRuntimeSettings, rule *OpsAlertRule, event *OpsAlertEvent) bool {
	return cfg != nil && cfg.Enabled && cfg.Version != "" && cfg.WebhookConfigured && cfg.SecretConfigured &&
		rule != nil && rule.Enabled && rule.NotifyFeishu && event != nil &&
		shouldSendOpsAlertEmailByMinSeverity(cfg.MinSeverity, event.Severity) &&
		(runtime == nil || !isOpsAlertSilenced(time.Now().UTC(), rule, event, runtime.Silencing))
}

func (s *OpsFeishuService) run(ctx context.Context) {
	if s.repo == nil {
		return
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			jobCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			err := s.processOne(jobCtx)
			cancel()
			if err != nil && ctx.Err() == nil {
				logger.LegacyPrintf("service.ops_feishu", "[OpsFeishu] delivery processing failed: %v", err)
			}
		}
	}
}

func (s *OpsFeishuService) processOne(ctx context.Context) error {
	d, err := s.repo.ClaimFeishuDelivery(ctx)
	if err != nil || d == nil {
		return err
	}
	finish := func(status, message string, delay time.Duration, attempted bool) error {
		return s.repo.FinishFeishuDelivery(ctx, d, status, message, time.Now().Add(delay), attempted)
	}
	deferJob := func() error {
		return finish("pending", "Notification settings temporarily unavailable", time.Minute, false)
	}
	if d.Attempts > 3 {
		return finish("failed", "Delivery attempts exhausted", 0, false)
	}
	if !s.ops.IsMonitoringEnabled(ctx) {
		return finish("cancelled", "Monitoring disabled", 0, false)
	}
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return deferJob()
	}
	if !cfg.Enabled || cfg.Version != d.ConfigVersion {
		return finish("cancelled", "Channel disabled or webhook replaced", 0, false)
	}
	event, err := s.ops.opsRepo.GetAlertEventByID(ctx, d.EventID)
	if err != nil {
		return deferJob()
	}
	if event == nil {
		return finish("cancelled", "Alert no longer exists", 0, false)
	}
	rules, err := s.ops.opsRepo.ListAlertRules(ctx)
	if err != nil {
		return deferJob()
	}
	var rule *OpsAlertRule
	for _, candidate := range rules {
		if candidate != nil && candidate.ID == event.RuleID {
			rule = candidate
			break
		}
	}
	runtime, err := s.ops.GetOpsAlertRuntimeSettings(ctx)
	if err != nil {
		return deferJob()
	}
	if !feishuEligible(cfg, runtime, rule, event) {
		return finish("cancelled", "Rule disabled, filtered or silenced", 0, false)
	}
	platform, groupID, region := parseOpsAlertRuleScope(rule.Filters)
	if platform != "" {
		silenced, err := s.ops.IsAlertSilenced(ctx, rule.ID, platform, groupID, region, time.Now().UTC())
		if err != nil {
			return deferJob()
		}
		if silenced {
			return finish("cancelled", "Alert silenced", 0, false)
		}
	}
	if d.Kind == OpsAlertStatusFiring && event.Status != OpsAlertStatusFiring {
		return finish("cancelled", "Alert resolved before delivery", 0, false)
	}
	if d.Kind == OpsAlertStatusResolved {
		if !cfg.IncludeResolvedAlerts || event.Status != OpsAlertStatusResolved {
			return finish("cancelled", "Recovery notification disabled or manually resolved", 0, false)
		}
		deliveries, err := s.repo.ListFeishuDeliveries(ctx, []int64{event.ID})
		if err != nil {
			return deferJob()
		}
		triggerSent := false
		for _, trigger := range deliveries {
			if trigger.Kind != OpsAlertStatusFiring || trigger.ConfigVersion != d.ConfigVersion {
				continue
			}
			if trigger.Status == "sending" || trigger.Status == "pending" {
				return finish("pending", "Waiting for trigger delivery", 10*time.Second, false)
			}
			triggerSent = trigger.Status == "sent"
		}
		if !triggerSent {
			return finish("cancelled", "Trigger notification was not delivered", 0, false)
		}
	}
	allowed, err := s.repo.ReserveFeishuSend(ctx, cfg.RateLimitPerHour, false)
	if err != nil {
		return deferJob()
	}
	if !allowed {
		return finish("pending", "Deferred by send limit", 30*time.Second, false)
	}
	err = s.send(ctx, cfg, buildFeishuAlertMessage(event, d.Kind))
	if err == nil {
		return finish("sent", "", 0, true)
	}
	var sendErr *feishuSendError
	if errors.As(err, &sendErr) && sendErr.retryable && d.Attempts < 3 {
		delay := 10 * time.Second
		if d.Attempts > 1 {
			delay = time.Minute
		}
		return finish("pending", sendErr.Error(), delay, true)
	}
	return finish("failed", err.Error(), 0, true)
}

func (s *OpsService) attachFeishuDeliveries(ctx context.Context, events []*OpsAlertEvent) error {
	repo, ok := s.opsRepo.(OpsFeishuRepository)
	if !ok || len(events) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(events))
	byID := make(map[int64]*OpsAlertEvent, len(events))
	for _, event := range events {
		if event != nil {
			ids = append(ids, event.ID)
			byID[event.ID] = event
		}
	}
	deliveries, err := repo.ListFeishuDeliveries(ctx, ids)
	if err != nil {
		return err
	}
	for _, d := range deliveries {
		if event := byID[d.EventID]; event != nil {
			event.FeishuDeliveries = append(event.FeishuDeliveries, d)
		}
	}
	return nil
}
