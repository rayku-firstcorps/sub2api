package service

import (
	"context"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/reqcapture"
)

// Upstream capture hooks for the Antigravity forwarding path.
//
// They only observe: every function is a no-op unless the originating request was
// marked by the RequestCapture middleware (SUB2API_CAPTURE_DIR set and the
// request passed the path/key/client filters).

// captureContext returns the context carrying the capture marker, or nil.
func (p antigravityRetryLoopParams) captureContext() context.Context {
	if reqcapture.Active(p.ctx) {
		return p.ctx
	}
	if p.c != nil && p.c.Request != nil && reqcapture.Active(p.c.Request.Context()) {
		return p.c.Request.Context()
	}
	return nil
}

// captureUpstreamRequest records the request exactly as it is sent to Google
// (the converted Gemini v1internal payload), tagged with the account used.
func captureUpstreamRequest(p antigravityRetryLoopParams, kind string, attempt int, req *http.Request) {
	ctx := p.captureContext()
	if ctx == nil || req == nil {
		return
	}
	m, _ := reqcapture.MetaFrom(ctx)
	ev := map[string]any{
		"event":             "upstream_request",
		"request_id":        m.RequestID,
		"client_request_id": m.ClientRequestID,
		"client_kind":       m.ClientKind,
		"kind":              kind, // primary | smart_retry | single_account_retry
		"attempt":           attempt,
		"account_id":        p.account.ID,
		"account_name":      p.account.Name,
		"requested_model":   p.requestedModel,
		"action":            p.action,
		"url":               safeUpstreamURL(req.URL.String()),
		"headers":           reqcapture.RedactHeaders(req.Header),
	}
	reqcapture.AttachBody(ev, "body", p.body)
	reqcapture.Record(ev)
}

// captureUpstreamResult records what Google answered. body is only provided for
// error responses (streamed 2xx bodies are deliberately not buffered).
func captureUpstreamResult(p antigravityRetryLoopParams, kind string, attempt int, resp *http.Response, err error, body []byte, start time.Time) {
	ctx := p.captureContext()
	if ctx == nil {
		return
	}
	m, _ := reqcapture.MetaFrom(ctx)
	ev := map[string]any{
		"event":             "upstream_response",
		"request_id":        m.RequestID,
		"client_request_id": m.ClientRequestID,
		"client_kind":       m.ClientKind,
		"kind":              kind,
		"attempt":           attempt,
		"account_id":        p.account.ID,
		"account_name":      p.account.Name,
		"requested_model":   p.requestedModel,
		"latency_ms":        time.Since(start).Milliseconds(),
	}
	if err != nil {
		ev["error"] = sanitizeUpstreamErrorMessage(err.Error())
	}
	if resp != nil {
		ev["status"] = resp.StatusCode
		ev["headers"] = reqcapture.RedactHeaders(resp.Header)
	}
	if len(body) > 0 {
		reqcapture.AttachBody(ev, "body", body)
	}
	reqcapture.Record(ev)
}
