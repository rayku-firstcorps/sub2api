//go:build unit

package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/reqcapture"
	"github.com/stretchr/testify/require"
)

func readCaptureFile(t *testing.T, dir string) []map[string]any {
	t.Helper()
	f, err := os.Open(filepath.Join(dir, "capture.jsonl"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	defer f.Close()
	var evs []map[string]any
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 32<<20)
	for sc.Scan() {
		var m map[string]any
		require.NoError(t, json.Unmarshal(sc.Bytes(), &m))
		evs = append(evs, m)
	}
	return evs
}

// runRateLimitedLoop drives the real retry loop against a fake upstream that answers
// 429 with a long retry delay (-> account switch), exactly like the existing
// smart-retry tests, and returns the error the loop produced.
func runRateLimitedLoop(ctx context.Context, upstreamBody []byte) (error, *mockSmartRetryUpstream) {
	respBody := []byte(`{"error":{"status":"RESOURCE_EXHAUSTED","details":[` +
		`{"@type":"type.googleapis.com/google.rpc.ErrorInfo","metadata":{"model":"claude-opus-5-5-high"},"reason":"RATE_LIMIT_EXCEEDED"},` +
		`{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"30s"}]}}`)
	upstream := &mockSmartRetryUpstream{
		responses: []*http.Response{{
			StatusCode: http.StatusTooManyRequests,
			Header:     http.Header{"X-Goog-Request-Id": []string{"g-123"}},
			Body:       io.NopCloser(bytes.NewReader(respBody)),
		}},
		errors: []error{nil},
	}
	account := &Account{ID: 7, Name: "acc-7", Type: AccountTypeOAuth, Platform: PlatformAntigravity,
		Schedulable: true, Status: StatusActive, Concurrency: 1}
	svc := &AntigravityGatewayService{}
	_, err := svc.antigravityRetryLoop(antigravityRetryLoopParams{
		ctx:          ctx,
		prefix:       "[test]",
		account:      account,
		accessToken:  "ya29.super-secret-token",
		action:       "streamGenerateContent",
		body:         upstreamBody,
		httpUpstream: upstream,
		accountRepo:  &stubAntigravityAccountRepo{},
		handleError: func(context.Context, string, *Account, int, http.Header, []byte, string, int64, string, bool) *handleModelRateLimitResult {
			return nil
		},
		requestedModel: "claude-opus-5-5",
	})
	return err, upstream
}

func TestAntigravityCapture_ObservesWithoutChangingBehavior(t *testing.T) {
	converted := []byte(`{"request":{"contents":[{"role":"user","parts":[{"text":"hi"}]}],"generationConfig":{"maxOutputTokens":64000}}}`)

	// Baseline: capture disabled.
	t.Setenv(reqcapture.EnvDir, "")
	reqcapture.Reload()
	baseErr, baseUp := runRateLimitedLoop(context.Background(), converted)
	require.Error(t, baseErr)

	// Same scenario with capture enabled and the context marked as captured.
	dir := t.TempDir()
	t.Setenv(reqcapture.EnvDir, dir)
	reqcapture.Reload()
	t.Cleanup(func() {
		_ = os.Unsetenv(reqcapture.EnvDir)
		reqcapture.Reload()
	})
	ctx := reqcapture.WithMeta(context.Background(), reqcapture.Meta{
		RequestID: "req-42", ClientRequestID: "client-42", ClientKind: reqcapture.ClientHermes, APIKeyID: 10, GroupID: 10,
	})
	capErr, capUp := runRateLimitedLoop(ctx, converted)

	// 1) behaviour is identical
	require.Equal(t, baseErr.Error(), capErr.Error(), "capture must not change the loop's outcome")
	require.Equal(t, len(baseUp.calls), len(capUp.calls), "capture must not change the number of upstream calls")

	// 2) what was recorded matches what was actually sent/received
	evs := readCaptureFile(t, dir)
	require.GreaterOrEqual(t, len(evs), 2, "expected upstream_request + upstream_response, got %+v", evs)
	var reqEv, respEv map[string]any
	for _, e := range evs {
		switch e["event"] {
		case "upstream_request":
			reqEv = e
		case "upstream_response":
			respEv = e
		}
	}
	require.NotNil(t, reqEv, "upstream_request missing")
	require.NotNil(t, respEv, "upstream_response missing")

	require.Equal(t, "req-42", reqEv["request_id"])
	require.Equal(t, "hermes", reqEv["client_kind"])
	require.Equal(t, "primary", reqEv["kind"])
	require.Equal(t, float64(7), reqEv["account_id"])
	require.Equal(t, "acc-7", reqEv["account_name"])
	require.Equal(t, "claude-opus-5-5", reqEv["requested_model"])

	sent, err := json.Marshal(reqEv["body"])
	require.NoError(t, err)
	require.JSONEq(t, string(converted), string(sent), "captured upstream body must equal what was sent")
	require.Equal(t, float64(len(converted)), reqEv["body_bytes"])
	require.JSONEq(t, string(converted), string(capUp.requestBodies[0]))

	require.Equal(t, float64(http.StatusTooManyRequests), respEv["status"])
	require.Equal(t, "req-42", respEv["request_id"])
	body, err := json.Marshal(respEv["body"])
	require.NoError(t, err)
	require.Contains(t, string(body), "RESOURCE_EXHAUSTED", "429 error body must be captured")
	hdrs, _ := respEv["headers"].(map[string]any)
	require.Equal(t, "g-123", hdrs["X-Goog-Request-Id"], "upstream response headers (request ids) must be captured")

	// 3) secrets never reach the file
	raw, _ := json.Marshal(evs)
	require.False(t, strings.Contains(string(raw), "ya29.super-secret-token"), "access token leaked into capture file")
}

func TestAntigravityCapture_NoMarkerMeansNoRecords(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(reqcapture.EnvDir, dir)
	reqcapture.Reload()
	t.Cleanup(func() {
		_ = os.Unsetenv(reqcapture.EnvDir)
		reqcapture.Reload()
	})
	// Capture is enabled process-wide, but this request was filtered out at ingress
	// (no marker in its context): the upstream hooks must stay silent.
	_, _ = runRateLimitedLoop(context.Background(), []byte(`{"request":{}}`))
	require.Empty(t, readCaptureFile(t, dir))
}
