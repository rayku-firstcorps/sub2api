package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// A quality failure is an answer mismatch, not proof of a different model.
func validateMonitorQuality(provider, checkMode, bodyMode, effort, prompt, expected string, notify bool) error {
	invalid := func(message string) error {
		return infraerrors.BadRequest("INVALID_MONITOR_QUALITY_CONFIG", message)
	}
	hasPrompt, hasExpected := strings.TrimSpace(prompt) != "", strings.TrimSpace(expected) != ""
	if hasPrompt != hasExpected {
		return invalid("prompt and expected_answer must be provided together")
	}
	if utf8.RuneCountInString(prompt) > 8000 || utf8.RuneCountInString(expected) > 2000 {
		return invalid("prompt must be at most 8000 characters and expected_answer at most 2000 characters")
	}
	if notify && !hasPrompt {
		return invalid("Feishu quality notifications require a prompt and expected_answer")
	}
	effort = strings.TrimSpace(effort)
	if !hasPrompt && effort == "" && !notify {
		return nil
	}
	if defaultCheckMode(checkMode) == MonitorCheckModeQuota {
		return invalid("quality checks and reasoning_effort require a probe mode")
	}
	if bodyMode == MonitorBodyOverrideModeReplace {
		return invalid("custom prompt and reasoning_effort cannot be combined with replace body mode")
	}
	if effort == "" {
		return nil
	}
	allowed := []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"}
	switch provider {
	case MonitorProviderAnthropic:
		allowed = []string{"low", "medium", "high", "max"}
	case MonitorProviderGemini:
		allowed = []string{"minimal", "low", "medium", "high"}
	}
	for _, value := range allowed {
		if effort == value {
			return nil
		}
	}
	return invalid("unsupported reasoning_effort for provider")
}

// Explicit probe settings take priority over advanced body merge fields.
func applyMonitorQualityBody(body map[string]any, provider, apiMode string, opts *CheckOptions) {
	if opts == nil {
		return
	}
	if strings.TrimSpace(opts.Prompt) != "" || opts.ReasoningEffort != "" {
		// The arithmetic default (50 tokens) is too small for reasoning tests.
		key := "max_tokens"
		if provider == MonitorProviderGemini {
			config := monitorBodyObject(body, "generationConfig")
			config["maxOutputTokens"] = 4096
		} else {
			if provider == MonitorProviderOpenAI && apiMode == MonitorAPIModeResponses {
				key = "max_output_tokens"
			}
			body[key] = 4096
		}
		if provider == MonitorProviderOpenAI && apiMode == MonitorAPIModeResponses && strings.TrimSpace(opts.Prompt) != "" {
			body["instructions"] = "Answer the user's test prompt. Follow its output format exactly."
		}
	}
}

func applyMonitorEffort(body map[string]any, provider, apiMode, effort string) {
	if effort == "" {
		return
	}
	switch provider {
	case MonitorProviderAnthropic:
		monitorBodyObject(body, "output_config")["effort"] = effort
		body["thinking"] = map[string]any{"type": "adaptive"}
	case MonitorProviderGemini:
		monitorBodyObject(monitorBodyObject(body, "generationConfig"), "thinkingConfig")["thinkingLevel"] = effort
	default:
		if provider == MonitorProviderOpenAI && apiMode == MonitorAPIModeResponses {
			monitorBodyObject(body, "reasoning")["effort"] = effort
		} else {
			body["reasoning_effort"] = effort
			if provider == MonitorProviderOpenAI {
				if _, exists := body["max_completion_tokens"]; !exists {
					body["max_completion_tokens"] = body["max_tokens"]
				}
				delete(body, "max_tokens")
			}
		}
	}
}

func monitorBodyObject(body map[string]any, key string) map[string]any {
	if value, ok := body[key].(map[string]any); ok {
		return value
	}
	value := map[string]any{}
	body[key] = value
	return value
}

// Bound text by runes so UTF-8 answers remain valid in PostgreSQL and Feishu.
func monitorQualityText(text string, limit int) string {
	text = sanitizeErrorMessage(strings.ToValidUTF8(text, "�"))
	runes := []rune(text)
	if len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return text
}

type ChannelMonitorQualityNotifier interface {
	NotifyChannelMonitorQuality(context.Context, *ChannelMonitor, []*CheckResult) error
}

func (s *ChannelMonitorService) SetQualityNotifier(notifier ChannelMonitorQualityNotifier) {
	s.qualityNotifier = notifier
}

func (s *ChannelMonitorService) notifyQualityFailure(ctx context.Context, m *ChannelMonitor, results []*CheckResult) {
	if !m.NotifyFeishu || s.qualityNotifier == nil {
		return
	}
	var failures []*CheckResult
	for _, result := range results {
		if result.Status == MonitorStatusQualityFailed {
			failures = append(failures, result)
		}
	}
	if len(failures) == 0 {
		return
	}
	// Probe timeout must not cancel the separately bounded notification attempt.
	notifyCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := s.qualityNotifier.NotifyChannelMonitorQuality(notifyCtx, m, failures); err != nil {
		slog.Warn("channel_monitor: quality notification failed", "monitor_id", m.ID, "error", err)
		for _, result := range failures {
			result.NotificationError = "Feishu quality notification was not delivered"
		}
	}
}

// Reuses operations credentials, mandatory signing, SSRF-safe transport and the
// shared rate budget. Quality probes are opt-in and do not create alert rules.
func (s *OpsService) NotifyChannelMonitorQuality(ctx context.Context, m *ChannelMonitor, failures []*CheckResult) error {
	if s == nil || s.feishu == nil || s.feishu.repo == nil {
		return fmt.Errorf("Feishu service unavailable")
	}
	if !s.IsMonitoringEnabled(ctx) {
		return fmt.Errorf("operations monitoring disabled")
	}
	cfg, err := s.feishu.loadConfig(ctx)
	if err != nil {
		return err
	}
	if !cfg.Enabled {
		return fmt.Errorf("Feishu notifications disabled")
	}
	if _, _, err := s.feishu.credentials(cfg); err != nil {
		return err
	}
	allowed, err := s.feishu.repo.ReserveFeishuSend(ctx, cfg.RateLimitPerHour, false)
	if err != nil || !allowed {
		return fmt.Errorf("Feishu send limit unavailable or reached")
	}
	return s.feishu.send(ctx, cfg, buildMonitorQualityMessage(m, failures))
}

func buildMonitorQualityMessage(m *ChannelMonitor, failures []*CheckResult) string {
	clean := func(value string, limit int) string {
		if m.APIKey != "" {
			value = strings.ReplaceAll(value, m.APIKey, "[REDACTED]")
		}
		for _, secret := range m.ExtraHeaders {
			if secret != "" {
				value = strings.ReplaceAll(value, secret, "[REDACTED]")
			}
		}
		return monitorQualityText(value, limit)
	}
	message := fmt.Sprintf("[Sub2API] 渠道疑似降智（答案不符）\n渠道: %s (#%d)\nProvider: %s\nEffort: %s\nPrompt: %s\n预期答案: %s",
		clean(m.Name, 100), m.ID, m.Provider, m.ReasoningEffort, clean(m.Prompt, 400), clean(m.ExpectedAnswer, 200))
	for i, result := range failures {
		if i >= 5 {
			message += fmt.Sprintf("\n另有 %d 个模型答案不符，请查看监控历史。", len(failures)-i)
			break
		}
		message += fmt.Sprintf("\n模型: %s\n实际回答: %s\n检测时间: %s", clean(result.Model, 100), clean(result.ResponseText, 300), result.CheckedAt.UTC().Format(time.RFC3339))
	}
	return message
}
