package middleware

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/reqcapture"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const hermesLikeBody = `{"model":"claude-opus-5-5","max_tokens":128000,"stream":true,
"messages":[{"role":"user","content":"hi"}],
"tools":[{"name":"terminal","input_schema":{"type":"object"}},{"name":"delegate_task","input_schema":{"type":"object"}}]}`

func captureEngine(t *testing.T, keyID int64, handler gin.HandlerFunc) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// stand-ins for RequestLogger/ClientRequestID and apiKeyAuth
	r.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = contextWithIDs(ctx, "req-1", "client-1")
		c.Request = c.Request.WithContext(ctx)
		c.Set(string(ContextKeyAPIKey), &service.APIKey{ID: keyID, GroupID: ptr(int64(10))})
		c.Next()
	})
	r.Use(RequestCapture())
	r.POST("/v1/messages", handler)
	r.POST("/v1/other", handler)
	return r
}

func ptr[T any](v T) *T { return &v }

func readCaptured(t *testing.T, dir string) []map[string]any {
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
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatalf("bad jsonl: %v", err)
		}
		evs = append(evs, m)
	}
	return evs
}

// The handler must still receive the exact request body, and the client must
// still receive the exact (streamed) response, while both ends are recorded.
func TestRequestCaptureIsTransparentAndRecords(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SUB2API_CAPTURE_DIR", dir)
	resetCaptureForTest(t)

	var seenBody string
	var seenCtxActive bool
	r := captureEngine(t, 7, func(c *gin.Context) {
		b, _ := io.ReadAll(c.Request.Body)
		seenBody = string(b)
		seenCtxActive = ctxIsCaptured(c)
		c.Header("Content-Type", "text/event-stream")
		c.Status(http.StatusTooManyRequests)
		_, _ = c.Writer.WriteString(`{"error":{"message":"Upstream rate limit exceeded"}}`)
		c.Writer.Flush()
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(hermesLikeBody))
	req.Header.Set("Authorization", "Bearer sk-must-not-leak")
	req.Header.Set("User-Agent", "Anthropic/Go 1.0")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if seenBody != hermesLikeBody {
		t.Fatalf("handler saw a modified body:\n got: %q\nwant: %q", seenBody, hermesLikeBody)
	}
	if !seenCtxActive {
		t.Fatal("request context must be marked as captured so upstream hooks can attach")
	}
	if w.Code != http.StatusTooManyRequests || !strings.Contains(w.Body.String(), "Upstream rate limit exceeded") {
		t.Fatalf("client response altered: %d %q", w.Code, w.Body.String())
	}

	evs := readCaptured(t, dir)
	if len(evs) != 2 {
		t.Fatalf("want ingress+egress, got %d events: %+v", len(evs), evs)
	}
	in, eg := evs[0], evs[1]
	if in["event"] != "ingress" || in["client_kind"] != "hermes" || in["api_key_id"] != float64(7) || in["group_id"] != float64(10) {
		t.Fatalf("ingress wrong: %+v", in)
	}
	if in["request_id"] != "req-1" || in["client_request_id"] != "client-1" {
		t.Fatalf("correlation ids missing: %+v", in)
	}
	if body, _ := in["body"].(map[string]any); body == nil || body["model"] != "claude-opus-5-5" {
		t.Fatalf("ingress body must be embedded JSON: %T", in["body"])
	}
	if sum, _ := in["summary"].(map[string]any); sum == nil || sum["tool_count"] != float64(2) {
		t.Fatalf("summary wrong: %+v", in["summary"])
	}
	if h, _ := in["headers"].(map[string]any); h == nil || !strings.HasPrefix(h["Authorization"].(string), "<redacted") {
		t.Fatalf("authorization header must be redacted: %+v", in["headers"])
	}
	raw, _ := json.Marshal(evs)
	if strings.Contains(string(raw), "sk-must-not-leak") {
		t.Fatal("credential leaked into capture file")
	}
	if eg["event"] != "egress" || eg["status"] != float64(429) || !strings.Contains(eg["resp_head"].(string), "rate limit") {
		t.Fatalf("egress wrong: %+v", eg)
	}
}

func TestRequestCaptureSkipsFilteredRequests(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SUB2API_CAPTURE_DIR", dir)
	t.Setenv("SUB2API_CAPTURE_API_KEY_IDS", "7")
	resetCaptureForTest(t)

	ok := func(c *gin.Context) {
		b, _ := io.ReadAll(c.Request.Body)
		c.String(http.StatusOK, "%d", len(b))
	}
	// wrong api key id
	w := httptest.NewRecorder()
	captureEngine(t, 8, ok).ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(hermesLikeBody)))
	// right key, wrong path
	w2 := httptest.NewRecorder()
	captureEngine(t, 7, ok).ServeHTTP(w2, httptest.NewRequest(http.MethodPost, "/v1/other", strings.NewReader(hermesLikeBody)))

	if w.Body.String() != strconv.Itoa(len(hermesLikeBody)) {
		t.Fatalf("filtered request body must pass through untouched, handler read %q bytes", w.Body.String())
	}
	if w2.Body.String() != strconv.Itoa(len(hermesLikeBody)) {
		t.Fatalf("filtered path body must pass through untouched, handler read %q bytes", w2.Body.String())
	}
	if evs := readCaptured(t, dir); len(evs) != 0 {
		t.Fatalf("filtered requests must not be captured: %+v", evs)
	}
}

func TestRequestCaptureDisabledIsPassThrough(t *testing.T) {
	t.Setenv("SUB2API_CAPTURE_DIR", "")
	resetCaptureForTest(t)
	w := httptest.NewRecorder()
	captureEngine(t, 7, func(c *gin.Context) {
		b, _ := io.ReadAll(c.Request.Body)
		c.String(http.StatusOK, "%s", string(b))
	}).ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(hermesLikeBody)))
	if w.Body.String() != hermesLikeBody {
		t.Fatalf("disabled capture must not touch traffic: %q", w.Body.String())
	}
}

func contextWithIDs(ctx context.Context, requestID, clientRequestID string) context.Context {
	ctx = context.WithValue(ctx, ctxkey.RequestID, requestID)
	return context.WithValue(ctx, ctxkey.ClientRequestID, clientRequestID)
}

func ctxIsCaptured(c *gin.Context) bool { return reqcapture.Active(c.Request.Context()) }

// resetCaptureForTest re-reads the SUB2API_CAPTURE_* variables set by the test and
// guarantees the singleton is switched off again afterwards.
func resetCaptureForTest(t *testing.T) {
	t.Helper()
	reqcapture.Reload()
	t.Cleanup(func() {
		_ = os.Unsetenv("SUB2API_CAPTURE_DIR") // runs before t.Setenv restores the old value
		reqcapture.Reload()
	})
}
