//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestMonitorQualityValidation(t *testing.T) {
	for _, tc := range []struct {
		name, provider, mode, body, effort, prompt, expected string
		notify, valid                                        bool
	}{
		{name: "legacy", valid: true},
		{name: "pair", prompt: "question", expected: "answer", notify: true, valid: true},
		{name: "prompt only", prompt: "question"},
		{name: "answer only", expected: "answer"},
		{name: "blank answer", prompt: "question", expected: " \n"},
		{name: "notify without pair", notify: true},
		{name: "long prompt", prompt: strings.Repeat("问", 8001), expected: "answer"},
		{name: "long answer", prompt: "question", expected: strings.Repeat("答", 2001)},
		{name: "replace conflict", prompt: "question", expected: "answer", body: "replace"},
		{name: "quota conflict", prompt: "question", expected: "answer", mode: "quota"},
		{name: "effort only", effort: "high", valid: true},
		{name: "unknown effort", effort: "ultra"},
		{name: "anthropic max", provider: "anthropic", effort: "max", valid: true},
		{name: "anthropic none", provider: "anthropic", effort: "none"},
		{name: "gemini high", provider: "gemini", effort: "high", valid: true},
		{name: "gemini max", provider: "gemini", effort: "max"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateMonitorQuality(tc.provider, tc.mode, tc.body, tc.effort, tc.prompt, tc.expected, tc.notify)
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestMonitorQualityRequestProtocols(t *testing.T) {
	for _, tc := range []struct{ provider, mode, effortPath, promptPath, tokenPath string }{
		{"openai", "chat_completions", "reasoning_effort", "messages.0.content", "max_completion_tokens"},
		{"openai", "responses", "reasoning.effort", "input", "max_output_tokens"},
		{"grok", "chat_completions", "reasoning_effort", "messages.0.content", "max_tokens"},
		{"anthropic", "chat_completions", "output_config.effort", "messages.0.content", "max_tokens"},
		{"gemini", "chat_completions", "generationConfig.thinkingConfig.thinkingLevel", "contents.0.parts.0.text", "generationConfig.maxOutputTokens"},
	} {
		t.Run(tc.provider+tc.mode, func(t *testing.T) {
			adapter, _, ok := providerAdapterFor(tc.provider, tc.mode)
			require.True(t, ok)
			opts := &CheckOptions{Prompt: "只回答测试答案", ExpectedAnswer: "答案", ReasoningEffort: "high"}
			body, err := buildRequestBody(adapter, tc.provider, tc.mode, "model", opts.Prompt, opts)
			require.NoError(t, err)
			require.Equal(t, "high", gjson.GetBytes(body, tc.effortPath).String())
			require.Equal(t, opts.Prompt, gjson.GetBytes(body, tc.promptPath).String())
			require.EqualValues(t, 4096, gjson.GetBytes(body, tc.tokenPath).Int())
			if tc.mode == "responses" {
				require.NotContains(t, gjson.GetBytes(body, "instructions").String(), "arithmetic")
			}
		})
	}
}

func TestMonitorQualityMergeDoesNotMutateSharedOptions(t *testing.T) {
	opts := &CheckOptions{Prompt: "question", ReasoningEffort: "high", BodyOverrideMode: "merge", BodyOverride: map[string]any{
		"reasoning": map[string]any{"effort": "low", "summary": "auto"}, "max_output_tokens": 8000,
	}}
	body, err := buildRequestBody(providerOpenAIResponsesAdapter, "openai", "responses", "model", opts.Prompt, opts)
	require.NoError(t, err)
	require.Equal(t, "high", gjson.GetBytes(body, "reasoning.effort").String())
	require.Equal(t, "auto", gjson.GetBytes(body, "reasoning.summary").String())
	require.EqualValues(t, 8000, gjson.GetBytes(body, "max_output_tokens").Int())
	require.Equal(t, "low", opts.BodyOverride["reasoning"].(map[string]any)["effort"])
}

func TestMonitorQualityAnswerClassification(t *testing.T) {
	for _, tc := range []struct {
		name, body, status string
		httpStatus         int
	}{
		{"match", `{"choices":[{"message":{"content":"  答案\n"},"finish_reason":"stop"}]}`, MonitorStatusOperational, 200},
		{"wrong", `{"choices":[{"message":{"content":"错误答案"}}]}`, MonitorStatusQualityFailed, 200},
		{"substring is not exact", `{"choices":[{"message":{"content":"这是答案"}}]}`, MonitorStatusQualityFailed, 200},
		{"empty", `{"choices":[{"message":{"content":""}}]}`, MonitorStatusQualityFailed, 200},
		{"truncated", `{"choices":[{"message":{"content":"答案"},"finish_reason":"length"}]}`, MonitorStatusFailed, 200},
		{"upstream error", `{"error":{"message":"unavailable"}}`, MonitorStatusError, 503},
		{"error envelope", `{"error":{"message":"unavailable"}}`, MonitorStatusError, 200},
		{"invalid JSON", `upstream error`, MonitorStatusError, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := monitorHTTPClient
			t.Cleanup(func() { monitorHTTPClient = original })
			monitorHTTPClient = &http.Client{Transport: feishuRoundTripper(func(req *http.Request) (*http.Response, error) {
				body, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				require.Equal(t, "只回答答案", gjson.GetBytes(body, "messages.0.content").String())
				return &http.Response{StatusCode: tc.httpStatus, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})}
			result := runCheckForModel(context.Background(), "openai", "https://example.com", "secret", "model", &CheckOptions{Prompt: "只回答答案", ExpectedAnswer: "答案"})
			require.Equal(t, tc.status, result.Status)
			if tc.status == MonitorStatusQualityFailed {
				require.Contains(t, result.Message, "expected")
				require.True(t, utf8.ValidString(result.Message))
			}
		})
	}
	for _, raw := range []string{`{"status":"incomplete"}`, `{"stop_reason":"max_tokens"}`, `{"candidates":[{"finishReason":"MAX_TOKENS"}]}`} {
		require.True(t, monitorResponseIncomplete(raw))
	}
	require.Equal(t, "答案", extractGeminiMonitorText([]byte(`{"candidates":[{"content":{"parts":[{"thought":true,"text":"thinking"},{"text":"答"},{"text":"案"}]}}]}`)))
}

type qualityRunRepo struct {
	ChannelMonitorRepository
	monitor *ChannelMonitor
	rows    []*ChannelMonitorHistoryRow
	checked bool
}

func (r *qualityRunRepo) GetByID(context.Context, int64) (*ChannelMonitor, error) {
	copy := *r.monitor
	return &copy, nil
}
func (r *qualityRunRepo) InsertHistoryBatch(_ context.Context, rows []*ChannelMonitorHistoryRow) error {
	r.rows = rows
	return nil
}
func (r *qualityRunRepo) MarkChecked(context.Context, int64, time.Time) error {
	r.checked = true
	return nil
}

func TestMonitorQualityRunCheckPersistsAndNotifies(t *testing.T) {
	original, originalPing := monitorHTTPClient, monitorPingHTTPClient
	t.Cleanup(func() { monitorHTTPClient, monitorPingHTTPClient = original, originalPing })
	monitorHTTPClient = &http.Client{Transport: feishuRoundTripper(func(req *http.Request) (*http.Response, error) {
		if req.Method == http.MethodPost {
			body, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			require.Equal(t, "question", gjson.GetBytes(body, "messages.0.content").String())
			require.Equal(t, "high", gjson.GetBytes(body, "reasoning_effort").String())
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"wrong answer"}}]}`))}, nil
	})}
	monitorPingHTTPClient = monitorHTTPClient
	repo := &qualityRunRepo{monitor: &ChannelMonitor{ID: 7, Provider: "openai", Endpoint: "https://example.com", APIKey: "OLD:secret",
		PrimaryModel: "primary", ExtraModels: []string{"extra"}, Prompt: "question", ExpectedAnswer: "answer", ReasoningEffort: "high", NotifyFeishu: true}}
	s := NewChannelMonitorService(repo, &duplicateChannelMonitorEncryptor{})
	s.SetRuntimeReader(channelMonitorRuntimeStub{rt: ChannelMonitorRuntime{Enabled: true, Mode: ChannelMonitorModeV1}})
	notifier := &qualityNotifierStub{}
	s.SetQualityNotifier(notifier)
	results, err := s.RunCheck(context.Background(), 7)
	require.NoError(t, err)
	require.Len(t, results, 2)
	require.True(t, repo.checked)
	require.Len(t, repo.rows, 2)
	for _, row := range repo.rows {
		require.Equal(t, MonitorStatusQualityFailed, row.Status)
	}
	require.Equal(t, 1, notifier.calls)
	require.Len(t, notifier.results, 2)
}

func TestMonitorQualityUpdateAndDuplicate(t *testing.T) {
	m := &ChannelMonitor{ID: 1, Provider: "openai", Endpoint: "https://example.com", APIKey: "OLD:secret", PrimaryModel: "model"}
	prompt, answer, effort, notify := "问题", "答案", "high", true
	require.NoError(t, applyMonitorUpdate(m, ChannelMonitorUpdateParams{Prompt: &prompt, ExpectedAnswer: &answer, ReasoningEffort: &effort, NotifyFeishu: &notify}))
	require.NoError(t, applyMonitorUpdate(m, ChannelMonitorUpdateParams{}))
	require.Equal(t, prompt, m.Prompt)
	repo := &duplicateChannelMonitorRepoStub{source: m}
	svc := NewChannelMonitorService(repo, &duplicateChannelMonitorEncryptor{})
	copy, err := svc.Duplicate(context.Background(), 1, 1, "admin:1", "")
	require.NoError(t, err)
	require.Equal(t, m.Prompt, copy.Prompt)
	require.Equal(t, m.ExpectedAnswer, copy.ExpectedAnswer)
	require.Equal(t, m.ReasoningEffort, copy.ReasoningEffort)
	require.True(t, copy.NotifyFeishu)
	empty, off := "", false
	require.NoError(t, applyMonitorUpdate(m, ChannelMonitorUpdateParams{Prompt: &empty, ExpectedAnswer: &empty, ReasoningEffort: &empty, NotifyFeishu: &off}))
	require.Empty(t, m.Prompt)
	require.Empty(t, m.ExpectedAnswer)
	require.False(t, m.NotifyFeishu)
}

type qualityNotifierStub struct {
	calls   int
	results []*CheckResult
	err     error
}

func (s *qualityNotifierStub) NotifyChannelMonitorQuality(_ context.Context, _ *ChannelMonitor, results []*CheckResult) error {
	s.calls++
	s.results = results
	return s.err
}

func TestMonitorQualityNotificationSelectionAndFailureIsolation(t *testing.T) {
	notifier := &qualityNotifierStub{err: errors.New("send failed")}
	s := NewChannelMonitorService(nil, nil)
	s.SetQualityNotifier(notifier)
	m := &ChannelMonitor{NotifyFeishu: true}
	results := []*CheckResult{{Status: MonitorStatusOperational}, {Status: MonitorStatusError}, {Status: MonitorStatusQualityFailed}}
	s.notifyQualityFailure(context.Background(), m, results)
	require.Equal(t, 1, notifier.calls)
	require.Len(t, notifier.results, 1)
	require.Equal(t, MonitorStatusQualityFailed, results[2].Status)
	require.NotEmpty(t, results[2].NotificationError)
	m.NotifyFeishu = false
	s.notifyQualityFailure(context.Background(), m, results)
	m.NotifyFeishu = true
	s.notifyQualityFailure(context.Background(), m, results[:2])
	require.Equal(t, 1, notifier.calls)
}

func TestMonitorQualityFeishuUsesSignedOpsChannelAndRateLimit(t *testing.T) {
	s, _ := newFeishuTestService(t)
	configuredFeishu(t, s)
	repo := &feishuWorkerRepo{allowed: true}
	s.repo = repo
	m := &ChannelMonitor{ID: 7, Name: "测试渠道", APIKey: "custom-secret", ReasoningEffort: "high", Prompt: "问题", ExpectedAnswer: "答案"}
	failures := []*CheckResult{{Model: "model", ResponseText: "wrong custom-secret", CheckedAt: time.Now()}}
	calls := 0
	s.client = &http.Client{Transport: feishuRoundTripper(func(req *http.Request) (*http.Response, error) {
		calls++
		var payload map[string]any
		require.NoError(t, json.NewDecoder(req.Body).Decode(&payload))
		require.NotEmpty(t, payload["sign"])
		message := payload["content"].(map[string]any)["text"].(string)
		require.Contains(t, message, "疑似降智")
		require.Contains(t, message, "预期答案: 答案")
		require.Contains(t, message, "Effort: high")
		require.NotContains(t, message, "custom-secret")
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"code":0}`))}, nil
	})}
	require.NoError(t, s.ops.NotifyChannelMonitorQuality(context.Background(), m, failures))
	repo.allowed = false
	require.Error(t, s.ops.NotifyChannelMonitorQuality(context.Background(), m, failures))
	require.Equal(t, 1, calls)
	_, err := s.updateConfig(context.Background(), &OpsFeishuConfigUpdate{Enabled: false, RateLimitPerHour: 60})
	require.NoError(t, err)
	repo.allowed = true
	require.Error(t, s.ops.NotifyChannelMonitorQuality(context.Background(), m, failures))
	require.Equal(t, 1, calls)
}
