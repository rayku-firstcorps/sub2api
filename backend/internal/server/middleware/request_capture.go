package middleware

import (
	"bytes"
	"io"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/reqcapture"
	"github.com/gin-gonic/gin"
)

const captureResponseHeadBytes = 2048

// captureWriter keeps the first bytes of the response so that error payloads
// (e.g. a 429 body) are visible in the capture file. It never alters what is sent.
type captureWriter struct {
	gin.ResponseWriter
	head bytes.Buffer
}

func (w *captureWriter) Write(b []byte) (int, error) {
	w.keep(b)
	return w.ResponseWriter.Write(b)
}

func (w *captureWriter) WriteString(s string) (int, error) {
	w.keep([]byte(s))
	return w.ResponseWriter.WriteString(s)
}

func (w *captureWriter) keep(b []byte) {
	if rem := captureResponseHeadBytes - w.head.Len(); rem > 0 {
		if len(b) > rem {
			b = b[:rem]
		}
		w.head.Write(b)
	}
}

type captureErrReader struct{ err error }

func (r captureErrReader) Read([]byte) (int, error) { return 0, r.err }

// RequestCapture records the ingress request and the final response for the
// paths/keys/clients selected through the SUB2API_CAPTURE_* variables, and marks
// the request context so upstream hooks can attach the converted upstream traffic.
//
// It must be registered after the API-key auth middleware (it reads the api key
// and group from the gin context). When capture is disabled it is a pass-through.
func RequestCapture() gin.HandlerFunc {
	if !reqcapture.Enabled() {
		return func(c *gin.Context) { c.Next() }
	}
	return func(c *gin.Context) {
		req := c.Request
		if req == nil || req.Method != http.MethodPost || !reqcapture.PathMatches(req.URL.Path) {
			c.Next()
			return
		}

		var keyID, groupID int64
		if k, ok := GetAPIKeyFromContext(c); ok && k != nil {
			keyID = k.ID
			if k.GroupID != nil {
				groupID = *k.GroupID
			}
		}
		if !reqcapture.KeyAllowed(keyID) {
			c.Next()
			return
		}

		body, readErr := io.ReadAll(req.Body)
		_ = req.Body.Close()
		if readErr != nil {
			// Preserve the original failure for downstream handlers; nothing to capture.
			req.Body = io.NopCloser(io.MultiReader(bytes.NewReader(body), captureErrReader{err: readErr}))
			c.Next()
			return
		}
		req.Body = io.NopCloser(bytes.NewReader(body))

		ctx := req.Context()
		requestID, _ := ctx.Value(ctxkey.RequestID).(string)
		clientRequestID, _ := ctx.Value(ctxkey.ClientRequestID).(string)
		summary := reqcapture.SummarizeAnthropic(body)
		kind := reqcapture.ClassifyClient(summary.ToolNames, req.UserAgent())
		if !reqcapture.ClientAllowed(kind) {
			c.Next()
			return
		}

		meta := reqcapture.Meta{
			RequestID:       requestID,
			ClientRequestID: clientRequestID,
			APIKeyID:        keyID,
			GroupID:         groupID,
			ClientKind:      kind,
		}
		c.Request = req.WithContext(reqcapture.WithMeta(ctx, meta))

		ingress := map[string]any{
			"event":             "ingress",
			"request_id":        requestID,
			"client_request_id": clientRequestID,
			"client_kind":       kind,
			"api_key_id":        keyID,
			"group_id":          groupID,
			"method":            req.Method,
			"path":              req.URL.Path,
			"remote_ip":         c.ClientIP(),
			"user_agent":        req.UserAgent(),
			"headers":           reqcapture.RedactHeaders(req.Header),
			"summary":           summary.Fields,
		}
		reqcapture.AttachBody(ingress, "body", body)
		reqcapture.Record(ingress)

		cw := &captureWriter{ResponseWriter: c.Writer}
		c.Writer = cw
		start := time.Now()
		c.Next()

		egress := map[string]any{
			"event":             "egress",
			"request_id":        requestID,
			"client_request_id": clientRequestID,
			"client_kind":       kind,
			"api_key_id":        keyID,
			"status":            c.Writer.Status(),
			"latency_ms":        time.Since(start).Milliseconds(),
			"resp_bytes":        c.Writer.Size(),
			"resp_content_type": c.Writer.Header().Get("Content-Type"),
			"resp_head":         cw.head.String(),
		}
		reqcapture.Record(egress)
	}
}
