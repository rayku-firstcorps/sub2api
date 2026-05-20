package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type fakeKiroPromptCache struct {
	namespace string
	total     int
	count     int
	result    KiroCacheResult
}

func (f *fakeKiroPromptCache) LookupOrCreate(ctx context.Context, namespace string, breakpoints []KiroCacheBreakpoint, totalInputTokens int) (KiroCacheResult, error) {
	f.namespace = namespace
	f.total = totalInputTokens
	f.count = len(breakpoints)
	return f.result, nil
}

type statefulKiroPromptCache struct {
	values map[string]int
}

func (f *statefulKiroPromptCache) LookupOrCreate(ctx context.Context, namespace string, breakpoints []KiroCacheBreakpoint, totalInputTokens int) (KiroCacheResult, error) {
	if f.values == nil {
		f.values = make(map[string]int)
	}

	result := KiroCacheResult{UncachedInputTokens: totalInputTokens}
	hitIndex := -1
	for i := len(breakpoints) - 1; i >= 0; i-- {
		key := namespace + ":" + breakpoints[i].Hash
		if tokens, ok := f.values[key]; ok {
			result.CacheReadInputTokens = tokens
			hitIndex = i
			break
		}
	}

	prevTokens := 0
	start := 0
	if hitIndex >= 0 {
		prevTokens = result.CacheReadInputTokens
		start = hitIndex + 1
	}
	for _, bp := range breakpoints[start:] {
		f.values[namespace+":"+bp.Hash] = bp.Tokens
		delta := bp.Tokens - prevTokens
		if delta > 0 {
			result.CacheCreationInputTokens += delta
			if bp.TTL >= kiroPromptCacheExtendedTTL {
				result.CacheCreation1hTokens += delta
			} else {
				result.CacheCreation5mTokens += delta
			}
		}
		prevTokens = bp.Tokens
	}

	cachedTokens := result.CacheReadInputTokens + result.CacheCreationInputTokens
	result.UncachedInputTokens = totalInputTokens - cachedTokens
	if result.UncachedInputTokens < 0 {
		result.UncachedInputTokens = 0
	}
	return result, nil
}

func TestKiroGatewayStreamResponseWritesClaudeSSEAndUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	svc := &KiroGatewayService{}

	upstream := strings.NewReader("\x00\x01{\"content\":\"Hello world\"}{\"stop\":true}")
	result, err := svc.streamResponse(c, upstream, nil, "claude-opus-4-6", ClaudeUsage{InputTokens: 12}, time.Now())

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, result.Stream)
	require.Equal(t, 12, result.Usage.InputTokens)
	require.Greater(t, result.Usage.OutputTokens, 0)

	body := w.Body.String()
	require.Contains(t, body, "event: message_start")
	require.Contains(t, body, `"input_tokens":12`)
	require.Contains(t, body, "event: content_block_delta")
	require.Contains(t, body, `"text":"Hello world"`)
	require.Contains(t, body, "event: message_stop")
	require.Contains(t, body, `"output_tokens":3`)
}

func TestKiroGatewayStreamResponseIncludesPseudoCacheReadUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	svc := &KiroGatewayService{}
	usage := ClaudeUsage{
		InputTokens:          3,
		CacheReadInputTokens: 11,
	}

	upstream := strings.NewReader("\x00\x01{\"content\":\"Hello world\"}{\"stop\":true}")
	result, err := svc.streamResponse(c, upstream, nil, "claude-opus-4-6", usage, time.Now())

	require.NoError(t, err)
	require.Equal(t, 11, result.Usage.CacheReadInputTokens)
	body := w.Body.String()
	require.Contains(t, body, "event: message_start")
	require.Contains(t, body, `"cache_read_input_tokens":11`)
}

func TestKiroGatewayStreamResponseEmptyUpstreamReturns502BeforeSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	svc := &KiroGatewayService{}

	result, err := svc.streamResponse(c, strings.NewReader("\x00\x01no-json"), nil, "claude-opus-4-6", ClaudeUsage{InputTokens: 8}, time.Now())

	require.Error(t, err)
	require.Nil(t, result)
	require.Equal(t, http.StatusBadGateway, w.Code)
	require.NotContains(t, w.Body.String(), "event: message_start")
	require.Contains(t, w.Body.String(), "empty response")
}

func TestKiroStreamDiagnosticsCapturesUnusableResponse(t *testing.T) {
	parser := kiro.NewStreamParser()
	diag := newKiroStreamDiagnostics(context.Background(), 42, "claude-opus-4-6", true)

	err := readKiroEvents(strings.NewReader("prefix {not-json} suffix"), parser, diag, func(evt kiro.StreamEvent) error {
		t.Fatalf("unexpected parsed event: %#v", evt)
		return nil
	})

	require.NoError(t, err)
	require.Equal(t, len("prefix {not-json} suffix"), diag.bytesRead)
	require.Equal(t, 1, diag.chunksRead)
	require.Equal(t, 0, diag.events)
	require.Equal(t, 1, diag.invalid)
	require.Equal(t, 0, diag.buffered)
	require.Contains(t, string(diag.preview), "{not-json}")
}

func TestKiroStreamDiagnosticsUsesBinarySafePreview(t *testing.T) {
	diag := newKiroStreamDiagnostics(context.Background(), 42, "claude-opus-4-6", true)
	diag.recordRead([]byte{0, 0, 0, 16, 0xff, '{', 'x', '}'})

	attrs := diag.attrs("test")
	values := map[string]any{}
	for i := 0; i+1 < len(attrs); i += 2 {
		key, ok := attrs[i].(string)
		if !ok {
			continue
		}
		values[key] = attrs[i+1]
	}

	preview, ok := values["raw_preview"].(string)
	require.True(t, ok)
	require.Equal(t, "00000010ff7b787d", preview)
	require.NotContains(t, preview, "\x00")
}

func TestKiroGatewayNonStreamResponseWritesJSONMessageAndUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	svc := &KiroGatewayService{}

	upstream := strings.NewReader("{\"content\":\"Hello\"}{\"content\":\" world\"}{\"stop\":true}")
	result, err := svc.nonStreamResponse(c, upstream, nil, "claude-opus-4-6", ClaudeUsage{InputTokens: 9})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.Stream)
	require.Equal(t, 9, result.Usage.InputTokens)
	require.Greater(t, result.Usage.OutputTokens, 0)
	require.Equal(t, http.StatusOK, w.Code)
	require.NotContains(t, w.Header().Get("Content-Type"), "text/event-stream")

	var payload struct {
		Type    string `json:"type"`
		Role    string `json:"role"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	require.Equal(t, "message", payload.Type)
	require.Equal(t, "assistant", payload.Role)
	require.Len(t, payload.Content, 1)
	require.Equal(t, "text", payload.Content[0].Type)
	require.Equal(t, "Hello world", payload.Content[0].Text)
	require.Equal(t, 9, payload.Usage.InputTokens)
	require.Equal(t, 3, payload.Usage.OutputTokens)
}

func TestKiroGatewayNonStreamResponseMissingStopCompletesByEOF(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	svc := &KiroGatewayService{}

	result, err := svc.nonStreamResponse(c, strings.NewReader(`{"content":"Hello"}`), nil, "claude-opus-4-6", ClaudeUsage{InputTokens: 9})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "end_turn", gjson.GetBytes(w.Body.Bytes(), "stop_reason").String())
	require.Equal(t, "Hello", gjson.GetBytes(w.Body.Bytes(), "content.0.text").String())
}

func TestKiroMessageCollectorMergesRepeatedToolUseEventsWithSameID(t *testing.T) {
	collector := newKiroMessageCollector(nil)

	collector.add(kiro.StreamEvent{Name: "WebFetch", ToolUseID: "toolu_1"})
	collector.add(kiro.StreamEvent{Name: "WebFetch", ToolUseID: "toolu_1", Input: kiro.JSONText(`{"url": "`)})
	collector.add(kiro.StreamEvent{Name: "WebFetch", ToolUseID: "toolu_1", Input: kiro.JSONText(`https://example.com", "prompt": "read"}`)})

	blocks := collector.contentBlocks()
	require.Len(t, blocks, 1)
	require.Equal(t, "tool_use", blocks[0]["type"])
	require.Equal(t, "toolu_1", blocks[0]["id"])
	require.Equal(t, map[string]any{"url": "https://example.com", "prompt": "read"}, blocks[0]["input"])
}

func TestKiroMessageCollectorParsesThinkingAndBracketToolCalls(t *testing.T) {
	collector := newKiroMessageCollector(nil)

	collector.add(kiro.StreamEvent{Content: "<thinking>\nplan\n</thinking>\n\nUse tool [Called Read with args: {path:\"README.md\",}]"})

	blocks := collector.contentBlocks()
	require.Len(t, blocks, 3)
	require.Equal(t, "thinking", blocks[0]["type"])
	require.Contains(t, blocks[0]["thinking"], "plan")
	require.Equal(t, "text", blocks[1]["type"])
	require.Contains(t, blocks[1]["text"], "Use tool")
	require.Equal(t, "tool_use", blocks[2]["type"])
	require.Equal(t, "Read", blocks[2]["name"])
	require.Equal(t, map[string]any{"path": "README.md"}, blocks[2]["input"])
	require.Equal(t, "tool_use", collector.stopReason())
}

func TestKiroMessageCollectorThinkingOnlyAddsTextBlockAndMaxTokens(t *testing.T) {
	collector := newKiroMessageCollector(nil)

	collector.add(kiro.StreamEvent{Content: "<thinking>\nplan\n</thinking>"})

	blocks := collector.contentBlocks()
	require.Len(t, blocks, 2)
	require.Equal(t, "thinking", blocks[0]["type"])
	require.Equal(t, "text", blocks[1]["type"])
	require.Equal(t, " ", blocks[1]["text"])
	require.Equal(t, "max_tokens", collector.stopReason())
}

func TestKiroGatewayStreamResponseThinkingOnlyAddsTextAndMaxTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	svc := &KiroGatewayService{}

	upstream := strings.NewReader("{\"content\":\"<thinking>plan</thinking>\"}{\"stop\":true}")
	result, err := svc.streamResponse(c, upstream, nil, "claude-opus-4-6", ClaudeUsage{InputTokens: 9}, time.Now())

	require.NoError(t, err)
	require.NotNil(t, result)
	body := w.Body.String()
	require.Contains(t, body, `"type":"thinking_delta"`)
	require.Contains(t, body, `"text":" "`)
	require.Contains(t, body, `"stop_reason":"max_tokens"`)
}

func TestKiroPseudoCacheBreakpointsIncludeToolsSystemAndMessages(t *testing.T) {
	body := []byte(`{
		"tools": [{
			"name": "demo_tool",
			"description": "demo",
			"input_schema": {"type":"object"},
			"cache_control": {"type":"ephemeral"}
		}],
		"system": [{"type":"text","text":"system prompt","cache_control":{"type":"ephemeral","ttl":"1h"}}],
		"messages": [{
			"role": "user",
			"content": [{"type":"text","text":"hello world","cache_control":{"type":"ephemeral"}}]
		}]
	}`)

	breakpoints := computeKiroCacheBreakpoints(body)

	require.Len(t, breakpoints, 3)
	require.Equal(t, kiroPromptCacheDefaultTTL, breakpoints[0].TTL)
	require.Equal(t, kiroPromptCacheExtendedTTL, breakpoints[1].TTL)
	require.Equal(t, kiroPromptCacheDefaultTTL, breakpoints[2].TTL)
	require.Greater(t, breakpoints[2].Tokens, breakpoints[1].Tokens)
}

func TestKiroPseudoCacheBreakpointHashIgnoresCacheControlMetadata(t *testing.T) {
	bodyA := []byte(`{
		"messages": [{
			"role": "user",
			"content": [{"type":"text","text":"stable","cache_control":{"type":"ephemeral"}}]
		}]
	}`)
	bodyB := []byte(`{
		"messages": [{
			"role": "user",
			"content": [{"cache_control":{"type":"ephemeral","ttl":"1h"},"text":"stable","type":"text"}]
		}]
	}`)

	breakpointsA := computeKiroCacheBreakpoints(bodyA)
	breakpointsB := computeKiroCacheBreakpoints(bodyB)

	require.Len(t, breakpointsA, 1)
	require.Len(t, breakpointsB, 1)
	require.Equal(t, breakpointsA[0].Hash, breakpointsB[0].Hash)
	require.Equal(t, kiroPromptCacheDefaultTTL, breakpointsA[0].TTL)
	require.Equal(t, kiroPromptCacheExtendedTTL, breakpointsB[0].TTL)
}

func TestKiroPseudoCacheToolHashUsesKiroCompatibleNormalization(t *testing.T) {
	longName := strings.Repeat("tool_name_", 10)
	body := []byte(`{
		"tools": [
			{"name":"web_search","type":"web_search_20250305","cache_control":{"type":"ephemeral"}},
			{"name":"` + longName + `","description":"demo","input_schema":{"properties":{"q":{"type":"string"}},"type":"object","cache_control":{"type":"ephemeral"}},"cache_control":{"type":"ephemeral"}}
		],
		"messages": [{"role":"user","content":"hello"}]
	}`)

	normalized := normalizeKiroCacheTool(gjson.GetBytes(body, "tools.1"))
	breakpoints := computeKiroCacheBreakpoints(body)

	require.Len(t, breakpoints, 1)
	require.Contains(t, normalized, "name:"+kiro.ShortenToolName(longName))
	require.NotContains(t, normalized, "cache_control")
	require.NotContains(t, normalized, longName+"|")
}

func TestKiroPromptCacheNamespacePrefersKiroCredential(t *testing.T) {
	account := &Account{
		ID: 99,
		Credentials: map[string]any{
			"profile_arn": "profile-arn-1",
			"client_id":   "client-1",
		},
	}

	require.Equal(t, "kiro:profile-arn-1", kiroPromptCacheNamespace(account, &APIKey{ID: 42}))
	require.Equal(t, "kiro:account:99", kiroPromptCacheNamespace(&Account{ID: 99}, &APIKey{ID: 42}))
	require.Equal(t, "apikey:42", kiroPromptCacheNamespace(nil, &APIKey{ID: 42}))
}

func TestKiroGatewayAppliesPseudoCacheUsage(t *testing.T) {
	cache := &fakeKiroPromptCache{
		result: KiroCacheResult{
			CacheReadInputTokens:     70,
			CacheCreationInputTokens: 10,
			CacheCreation5mTokens:    4,
			CacheCreation1hTokens:    6,
			UncachedInputTokens:      20,
		},
	}
	svc := &KiroGatewayService{promptCache: cache}
	usage := ClaudeUsage{InputTokens: 100}
	body := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"hello","cache_control":{"type":"ephemeral"}}]}]}`)

	svc.applyPseudoCacheBilling(context.Background(), nil, &APIKey{ID: 42}, body, &usage)

	require.Equal(t, "apikey:42", cache.namespace)
	require.Equal(t, 100, cache.total)
	require.Equal(t, 1, cache.count)
	require.Equal(t, 20, usage.InputTokens)
	require.Equal(t, 70, usage.CacheReadInputTokens)
	require.Equal(t, 10, usage.CacheCreationInputTokens)
	require.Equal(t, 4, usage.CacheCreation5mTokens)
	require.Equal(t, 6, usage.CacheCreation1hTokens)
}

func TestKiroGatewayPseudoCacheReadsSameBreakpointOnSecondRequest(t *testing.T) {
	cache := &statefulKiroPromptCache{}
	svc := &KiroGatewayService{promptCache: cache}
	apiKey := &APIKey{ID: 42}
	body := []byte(`{
		"system": [{"type":"text","text":"stable system","cache_control":{"type":"ephemeral"}}],
		"messages": [{"role":"user","content":[{"type":"text","text":"hello"}]}]
	}`)

	firstUsage := ClaudeUsage{InputTokens: 20}
	svc.applyPseudoCacheBilling(context.Background(), nil, apiKey, body, &firstUsage)
	require.Greater(t, firstUsage.CacheCreationInputTokens, 0)
	require.Zero(t, firstUsage.CacheReadInputTokens)

	secondUsage := ClaudeUsage{InputTokens: 20}
	svc.applyPseudoCacheBilling(context.Background(), nil, apiKey, body, &secondUsage)
	require.Zero(t, secondUsage.CacheCreationInputTokens)
	require.Equal(t, firstUsage.CacheCreationInputTokens, secondUsage.CacheReadInputTokens)
	require.Equal(t, firstUsage.InputTokens, secondUsage.InputTokens)
}

func TestKiroNonStreamResponseIncludesPseudoCacheUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	svc := &KiroGatewayService{}
	usage := ClaudeUsage{
		InputTokens:              2,
		CacheReadInputTokens:     7,
		CacheCreationInputTokens: 5,
		CacheCreation5mTokens:    5,
	}

	result, err := svc.nonStreamResponse(c, strings.NewReader("{\"content\":\"Hello\"}{\"stop\":true}"), nil, "claude-opus-4-6", usage)

	require.NoError(t, err)
	require.Equal(t, 2, result.Usage.InputTokens)
	require.Equal(t, 7, result.Usage.CacheReadInputTokens)
	require.Equal(t, 5, result.Usage.CacheCreationInputTokens)
	require.Equal(t, int64(2), gjson.GetBytes(w.Body.Bytes(), "usage.input_tokens").Int())
	require.Equal(t, int64(7), gjson.GetBytes(w.Body.Bytes(), "usage.cache_read_input_tokens").Int())
	require.Equal(t, int64(5), gjson.GetBytes(w.Body.Bytes(), "usage.cache_creation_input_tokens").Int())
	require.Equal(t, int64(5), gjson.GetBytes(w.Body.Bytes(), "usage.cache_creation.ephemeral_5m_input_tokens").Int())
}

func TestKiroGatewayNonStreamResponseThinkingOnlyUsesMaxTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	svc := &KiroGatewayService{}

	result, err := svc.nonStreamResponse(c, strings.NewReader("{\"content\":\"<thinking>plan</thinking>\"}{\"stop\":true}"), nil, "claude-opus-4-6", ClaudeUsage{InputTokens: 9})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "max_tokens", gjson.GetBytes(w.Body.Bytes(), "stop_reason").String())
	require.Equal(t, "thinking", gjson.GetBytes(w.Body.Bytes(), "content.0.type").String())
	require.Equal(t, "text", gjson.GetBytes(w.Body.Bytes(), "content.1.type").String())
	require.Equal(t, " ", gjson.GetBytes(w.Body.Bytes(), "content.1.text").String())
}

func TestKiroBadRequestDiagnosticsPreviewRedactsSensitiveFields(t *testing.T) {
	body := []byte(`{
		"profileArn":"arn:aws:codewhisperer:us-east-1:123456789012:profile/demo",
		"access_token":"secret-token",
		"conversationState":{"currentMessage":{"userInputMessage":{"content":"hello"}}}
	}`)

	preview := redactedJSONPreviewForLog(body, 2000)

	require.Contains(t, preview, "hello")
	require.Contains(t, preview, "<redacted>")
	require.NotContains(t, preview, "arn:aws")
	require.NotContains(t, preview, "secret-token")
}

func TestGatewayServiceForwardCountTokensKiroUsesLocalEstimate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	svc := &GatewayService{}
	body := []byte(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hello"}]}`)
	parsed := &ParsedRequest{Model: "claude-sonnet-4-5", Body: body}
	account := &Account{ID: 1, Platform: PlatformKiro, Type: AccountTypeOAuth}

	err := svc.ForwardCountTokens(context.Background(), c, account, parsed)

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, w.Code)
	require.Greater(t, gjson.GetBytes(w.Body.Bytes(), "input_tokens").Int(), int64(0))
}
