package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const feishuTestURL = "https://open.feishu.cn/open-apis/bot/v2/hook/11111111-2222-3333-4444-555555555555"

type feishuTestEncryptor struct{}

func (feishuTestEncryptor) Encrypt(s string) (string, error) {
	return base64.StdEncoding.EncodeToString([]byte(s)), nil
}
func (feishuTestEncryptor) Decrypt(s string) (string, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	return string(b), err
}

type feishuRoundTripper func(*http.Request) (*http.Response, error)

func (f feishuRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func newFeishuTestService(t *testing.T) (*OpsFeishuService, *runtimeSettingRepoStub) {
	t.Helper()
	repo := newRuntimeSettingRepoStub()
	ops := &OpsService{settingRepo: repo}
	s := NewOpsFeishuService(ops, feishuTestEncryptor{})
	ops.feishu = s
	return s, repo
}

func configuredFeishu(t *testing.T, s *OpsFeishuService) *opsFeishuStoredConfig {
	t.Helper()
	url, secret := feishuTestURL, "test-signing-secret"
	_, err := s.updateConfig(context.Background(), &OpsFeishuConfigUpdate{Enabled: true, RateLimitPerHour: 60, WebhookURL: &url, SigningSecret: &secret})
	require.NoError(t, err)
	cfg, err := s.loadConfig(context.Background())
	require.NoError(t, err)
	return cfg
}

func TestOpsFeishuMandatorySigningAndCredentialLifecycle(t *testing.T) {
	s, settings := newFeishuTestService(t)
	ctx := context.Background()
	url, secret, empty := feishuTestURL, "test-signing-secret", " "
	for _, req := range []*OpsFeishuConfigUpdate{
		{Enabled: true, RateLimitPerHour: 60},
		{WebhookURL: &url, RateLimitPerHour: 60},
		{WebhookURL: &url, SigningSecret: &empty, RateLimitPerHour: 60},
		{SigningSecret: &secret, RateLimitPerHour: 60},
		{WebhookURL: &url, SigningSecret: &secret, RateLimitPerHour: 0},
	} {
		_, err := s.updateConfig(ctx, req)
		require.Error(t, err)
		require.Empty(t, settings.values)
	}
	cfg := configuredFeishu(t, s)
	require.NotContains(t, settings.values[SettingKeyOpsFeishuNotificationConfig], url)
	require.NotContains(t, settings.values[SettingKeyOpsFeishuNotificationConfig], secret)
	public, err := s.ops.GetFeishuNotificationConfig(ctx)
	require.NoError(t, err)
	raw, err := json.Marshal(public)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "ciphertext")
	require.NotContains(t, string(raw), secret)
	require.True(t, public.SecretConfigured)
	_, err = s.updateConfig(ctx, &OpsFeishuConfigUpdate{Enabled: true, RateLimitPerHour: 10, IncludeResolvedAlerts: true})
	require.NoError(t, err)
	preserved, err := s.loadConfig(ctx)
	require.NoError(t, err)
	require.Equal(t, cfg.WebhookCiphertext, preserved.WebhookCiphertext)
	require.Equal(t, cfg.SecretCiphertext, preserved.SecretCiphertext)
	require.Equal(t, cfg.Version, preserved.Version)
	_, err = s.updateConfig(ctx, &OpsFeishuConfigUpdate{Enabled: true, RateLimitPerHour: 60, ClearCredentials: true})
	require.Error(t, err)
	public, err = s.updateConfig(ctx, &OpsFeishuConfigUpdate{RateLimitPerHour: 60, ClearCredentials: true})
	require.NoError(t, err)
	require.False(t, public.Enabled)
	require.False(t, public.SecretConfigured)
	require.False(t, public.WebhookConfigured)
}

func TestOpsFeishuWebhookValidation(t *testing.T) {
	require.NoError(t, validateFeishuWebhook(feishuTestURL))
	for _, value := range []string{
		"http://open.feishu.cn/open-apis/bot/v2/hook/1111111111111111",
		"https://open.feishu.cn.evil.example/open-apis/bot/v2/hook/1111111111111111",
		"https://127.0.0.1/open-apis/bot/v2/hook/1111111111111111",
		strings.Replace(feishuTestURL, "open.feishu.cn", "open.feishu.cn:443", 1),
		strings.Replace(feishuTestURL, "open.feishu.cn", "user@open.feishu.cn", 1),
		feishuTestURL + "?x=1", feishuTestURL + "?", feishuTestURL + "#fragment",
		strings.Replace(feishuTestURL, "hook/", "hook/%2f", 1),
	} {
		require.Error(t, validateFeishuWebhook(value), value)
	}
	client := newFeishuHTTPClient()
	require.ErrorIs(t, client.CheckRedirect(nil, nil), http.ErrUseLastResponse)
	_, err := client.Transport.(*http.Transport).DialContext(context.Background(), "tcp", "127.0.0.1:443")
	require.Error(t, err)
}

func TestOpsFeishuSignedRequestAndBusinessErrors(t *testing.T) {
	require.Equal(t, "q4jswNiMy51J5JuQV566yJat0/lQ/c+22kINzUgKsGU=", signFeishu(1599360473, "secret"))
	s, _ := newFeishuTestService(t)
	cfg := configuredFeishu(t, s)
	for _, tc := range []struct {
		name           string
		status         int
		body           string
		retry, success bool
	}{
		{"success", 200, `{"code":0}`, false, true},
		{"bad signature", 200, `{"code":19021,"msg":"test-signing-secret"}`, false, false},
		{"bad request", 200, `{"code":9499}`, false, false},
		{"throttled", 200, `{"code":11232}`, true, false},
		{"http limit", 429, ``, true, false},
		{"server failure", 503, ``, true, false},
		{"redirect", 302, ``, false, false},
		{"missing code", 200, `{}`, true, false},
		{"invalid JSON", 200, `garbage`, true, false},
		{"oversized response", 200, strings.Repeat("a", 65537), true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s.client = &http.Client{Transport: feishuRoundTripper(func(r *http.Request) (*http.Response, error) {
				require.Equal(t, http.MethodPost, r.Method)
				require.Equal(t, feishuTestURL, r.URL.String())
				var payload struct {
					Timestamp, Sign string
					MsgType         string `json:"msg_type"`
				}
				require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
				ts, err := strconv.ParseInt(payload.Timestamp, 10, 64)
				require.NoError(t, err)
				require.WithinDuration(t, time.Now(), time.Unix(ts, 0), 2*time.Second)
				require.Equal(t, signFeishu(ts, "test-signing-secret"), payload.Sign)
				require.Equal(t, "text", payload.MsgType)
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: http.Header{}}, nil
			})}
			err := s.send(context.Background(), cfg, "test")
			if tc.success {
				require.NoError(t, err)
				return
			}
			var sendErr *feishuSendError
			require.ErrorAs(t, err, &sendErr)
			require.Equal(t, tc.retry, sendErr.retryable)
			require.NotContains(t, err.Error(), "test-signing-secret")
		})
	}
}

func TestOpsFeishuSendFailsClosedAndRedactsNetworkErrors(t *testing.T) {
	s, _ := newFeishuTestService(t)
	cfg := configuredFeishu(t, s)
	calls := 0
	s.client = &http.Client{Transport: feishuRoundTripper(func(r *http.Request) (*http.Response, error) {
		calls++
		return nil, fmt.Errorf("network failure at %s", r.URL)
	})}
	err := s.send(context.Background(), cfg, "test")
	require.Error(t, err)
	require.NotContains(t, err.Error(), feishuTestURL)
	cfg.SecretCiphertext = ""
	require.Error(t, s.send(context.Background(), cfg, "test"))
	require.Equal(t, 1, calls)
}

type feishuWorkerRepo struct {
	OpsRepository
	OpsFeishuRepository
	event           *OpsAlertEvent
	rule            *OpsAlertRule
	delivery        *OpsFeishuDelivery
	trigger         *OpsFeishuDelivery
	allowed         bool
	status, message string
	attempted       bool
	enqueued        bool
	resolved        bool
}

func (r *feishuWorkerRepo) GetLatestSystemMetrics(context.Context, int) (*OpsSystemMetricsSnapshot, error) {
	value := 99.0
	return &OpsSystemMetricsSnapshot{CPUUsagePercent: &value}, nil
}
func (r *feishuWorkerRepo) GetActiveAlertEvent(context.Context, int64) (*OpsAlertEvent, error) {
	return r.event, nil
}
func (r *feishuWorkerRepo) GetLatestAlertEvent(context.Context, int64) (*OpsAlertEvent, error) {
	return nil, nil
}
func (r *feishuWorkerRepo) UpsertJobHeartbeat(context.Context, *OpsUpsertJobHeartbeatInput) error {
	return nil
}
func (r *feishuWorkerRepo) CreateAlertEventWithFeishu(_ context.Context, event *OpsAlertEvent, _ string) (*OpsAlertEvent, error) {
	r.enqueued = true
	event.ID = 1
	r.event = event
	return event, nil
}
func (r *feishuWorkerRepo) ResolveAlertEventWithFeishu(context.Context, int64, time.Time, string) error {
	r.resolved = true
	return nil
}

func TestOpsFeishuEvaluatorDoesNotRequireEmailAndKeepsBreachedEventAfterRestart(t *testing.T) {
	s, _ := newFeishuTestService(t)
	configuredFeishu(t, s)
	r := &feishuWorkerRepo{
		rule: &OpsAlertRule{ID: 2, Enabled: true, NotifyFeishu: true, NotifyEmail: false,
			Severity: "P1", MetricType: "cpu_usage_percent", Operator: ">", Threshold: 80, SustainedMinutes: 1},
	}
	s.repo, s.ops.opsRepo = r, r
	evaluator := NewOpsAlertEvaluatorService(s.ops, r, nil, nil, nil, nil)
	evaluator.evaluateOnce(time.Minute)
	require.True(t, r.enqueued, "nil email service must not prevent Feishu enqueue")
	require.False(t, r.resolved)
	// A restart resets sustained-breach counters, but must not resolve an active
	// event whose current metric remains above the threshold.
	r.rule.SustainedMinutes = 5
	restarted := NewOpsAlertEvaluatorService(s.ops, r, nil, nil, nil, nil)
	restarted.evaluateOnce(time.Minute)
	require.False(t, r.resolved)
}

func (r *feishuWorkerRepo) ClaimFeishuDelivery(context.Context) (*OpsFeishuDelivery, error) {
	return r.delivery, nil
}
func (r *feishuWorkerRepo) FinishFeishuDelivery(_ context.Context, _ *OpsFeishuDelivery, status, message string, _ time.Time, attempted bool) error {
	r.status, r.message, r.attempted = status, message, attempted
	return nil
}
func (r *feishuWorkerRepo) ReserveFeishuSend(context.Context, int, bool) (bool, error) {
	return r.allowed, nil
}
func (r *feishuWorkerRepo) GetAlertEventByID(context.Context, int64) (*OpsAlertEvent, error) {
	return r.event, nil
}
func (r *feishuWorkerRepo) ListAlertRules(context.Context) ([]*OpsAlertRule, error) {
	return []*OpsAlertRule{r.rule}, nil
}
func (r *feishuWorkerRepo) ListFeishuDeliveries(context.Context, []int64) ([]*OpsFeishuDelivery, error) {
	if r.trigger == nil {
		return nil, nil
	}
	return []*OpsFeishuDelivery{r.trigger}, nil
}
func (r *feishuWorkerRepo) IsAlertSilenced(context.Context, int64, string, *int64, *string, time.Time) (bool, error) {
	return false, nil
}

func TestOpsFeishuWorkerPolicy(t *testing.T) {
	for _, name := range []string{"sent", "disabled rule", "email disabled", "resolved before send", "rate limit", "transient", "exhausted", "permanent", "replacement", "recovery sent", "recovery waiting", "recovery missing trigger", "recovery disabled", "silenced", "severity filtered", "disabled channel"} {
		t.Run(name, func(t *testing.T) {
			s, settings := newFeishuTestService(t)
			cfg := configuredFeishu(t, s)
			r := &feishuWorkerRepo{allowed: true,
				event:    &OpsAlertEvent{ID: 1, RuleID: 2, Severity: "P1", Status: OpsAlertStatusFiring},
				rule:     &OpsAlertRule{ID: 2, Enabled: true, NotifyFeishu: true, NotifyEmail: false},
				delivery: &OpsFeishuDelivery{ID: 3, EventID: 1, Kind: OpsAlertStatusFiring, Attempts: 1, ConfigVersion: cfg.Version},
			}
			s.repo, s.ops.opsRepo = r, r
			response := `{"code":0}`
			want, attempted, calls := "sent", true, 0
			switch name {
			case "disabled rule":
				r.rule.NotifyFeishu = false
				want, attempted = "cancelled", false
			case "resolved before send":
				r.event.Status = OpsAlertStatusResolved
				want, attempted = "cancelled", false
			case "rate limit":
				r.allowed = false
				want, attempted = "pending", false
			case "transient":
				response = `{"code":11232}`
				want = "pending"
			case "exhausted":
				response = `{"code":11232}`
				r.delivery.Attempts = 3
				want = "failed"
			case "permanent":
				response = `{"code":19021}`
				want = "failed"
			case "replacement":
				r.delivery.ConfigVersion = "old"
				want, attempted = "cancelled", false
			case "recovery sent", "recovery waiting", "recovery missing trigger", "recovery disabled":
				r.delivery.Kind = OpsAlertStatusResolved
				r.event.Status = OpsAlertStatusResolved
				cfg.IncludeResolvedAlerts = name != "recovery disabled"
				r.trigger = &OpsFeishuDelivery{Kind: OpsAlertStatusFiring, Status: "sent", ConfigVersion: cfg.Version}
				if name == "recovery waiting" {
					r.trigger.Status = "sending"
					want, attempted = "pending", false
				}
				if name == "recovery missing trigger" {
					r.trigger = nil
					want, attempted = "cancelled", false
				}
				if name == "recovery disabled" {
					want, attempted = "cancelled", false
				}
			case "silenced":
				settings.values[SettingKeyOpsAlertRuntimeSettings] = `{"silencing":{"enabled":true,"global_until_rfc3339":"2099-01-01T00:00:00Z"}}`
				want, attempted = "cancelled", false
			case "severity filtered":
				cfg.MinSeverity = "critical"
				r.event.Severity = "P3"
				want, attempted = "cancelled", false
			case "disabled channel":
				cfg.Enabled = false
				want, attempted = "cancelled", false
			}
			raw, err := json.Marshal(cfg)
			require.NoError(t, err)
			settings.values[SettingKeyOpsFeishuNotificationConfig] = string(raw)
			s.client = &http.Client{Transport: feishuRoundTripper(func(*http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(response))}, nil
			})}
			require.NoError(t, s.processOne(context.Background()))
			require.Equal(t, want, r.status)
			require.Equal(t, attempted, r.attempted)
			if !attempted {
				require.Zero(t, calls)
			} else {
				require.Equal(t, 1, calls)
			}
		})
	}
}

func TestOpsFeishuConfigCorruptionFailsClosed(t *testing.T) {
	s, settings := newFeishuTestService(t)
	settings.values[SettingKeyOpsFeishuNotificationConfig] = `invalid`
	_, err := s.loadConfig(context.Background())
	require.Error(t, err)
	settings.getValueFn = func(string) (string, error) { return "", errors.New("db unavailable") }
	_, err = s.loadConfig(context.Background())
	require.Error(t, err)
}
