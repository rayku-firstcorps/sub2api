// Package reqcapture records real gateway traffic (client ingress, the request
// actually sent upstream, and the upstream answer) into rotating JSONL files so
// that failures can be diagnosed offline and different clients can be compared.
//
// It is strictly an observer:
//   - disabled unless SUB2API_CAPTURE_DIR is set (zero overhead otherwise);
//   - never changes request/response semantics;
//   - credentials in headers are always redacted;
//   - bounded: per-body size cap, per-file size cap with rotation, and a
//     process-lifetime record cap.
//
// Captured files contain end-user prompts. Keep the directory private and turn
// the feature off (unset the variable and restart) once the investigation is done.
//
// Environment variables (read once at first use):
//
//	SUB2API_CAPTURE_DIR             enable + target directory (e.g. /app/data/capture)
//	SUB2API_CAPTURE_PATHS           comma list of exact request paths, default "/v1/messages"
//	SUB2API_CAPTURE_API_KEY_IDS     comma list of sub2api api_key ids to capture (default: all)
//	SUB2API_CAPTURE_CLIENTS         comma list of client kinds to capture, e.g. "claude-code,hermes" (default: all)
//	SUB2API_CAPTURE_MAX_BODY_BYTES  per-body cap, default 4194304 (4 MiB)
//	SUB2API_CAPTURE_MAX_FILE_MB     rotate when the file exceeds this size, default 200
//	SUB2API_CAPTURE_MAX_RECORDS     stop after this many events per process, default 5000
package reqcapture

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// EnvDir enables the feature and selects the output directory.
	EnvDir          = "SUB2API_CAPTURE_DIR"
	envPaths        = "SUB2API_CAPTURE_PATHS"
	envKeyIDs       = "SUB2API_CAPTURE_API_KEY_IDS"
	envClients      = "SUB2API_CAPTURE_CLIENTS"
	envMaxBody      = "SUB2API_CAPTURE_MAX_BODY_BYTES"
	envMaxFileMB    = "SUB2API_CAPTURE_MAX_FILE_MB"
	envMaxRecords   = "SUB2API_CAPTURE_MAX_RECORDS"
	captureFileName = "capture.jsonl"
	keepRotated     = 4

	defaultMaxBody    = 4 << 20
	defaultMaxFileMB  = 200
	defaultMaxRecords = 5000
	defaultPath       = "/v1/messages"
)

type settings struct {
	dir          string
	paths        map[string]struct{}
	keyIDs       map[int64]struct{}  // empty = all
	clients      map[string]struct{} // empty = all
	maxBody      int
	maxFileBytes int64
	maxRecords   int64
}

var (
	once    sync.Once
	cfg     settings
	enabled bool
	out     *sink
)

func settingsFromEnv(getenv func(string) string) settings {
	s := settings{
		dir:          strings.TrimSpace(getenv(EnvDir)),
		paths:        map[string]struct{}{},
		keyIDs:       map[int64]struct{}{},
		clients:      map[string]struct{}{},
		maxBody:      defaultMaxBody,
		maxFileBytes: int64(defaultMaxFileMB) << 20,
		maxRecords:   defaultMaxRecords,
	}
	for _, p := range splitList(getenv(envPaths)) {
		s.paths[p] = struct{}{}
	}
	if len(s.paths) == 0 {
		s.paths[defaultPath] = struct{}{}
	}
	for _, v := range splitList(getenv(envKeyIDs)) {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			s.keyIDs[id] = struct{}{}
		}
	}
	for _, v := range splitList(getenv(envClients)) {
		s.clients[strings.ToLower(v)] = struct{}{}
	}
	if n, err := strconv.Atoi(strings.TrimSpace(getenv(envMaxBody))); err == nil && n > 0 {
		s.maxBody = n
	}
	if n, err := strconv.Atoi(strings.TrimSpace(getenv(envMaxFileMB))); err == nil && n > 0 {
		s.maxFileBytes = int64(n) << 20
	}
	if n, err := strconv.ParseInt(strings.TrimSpace(getenv(envMaxRecords)), 10, 64); err == nil && n > 0 {
		s.maxRecords = n
	}
	return s
}

func splitList(v string) []string {
	var res []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			res = append(res, p)
		}
	}
	return res
}

func load() {
	once.Do(func() {
		cfg = settingsFromEnv(os.Getenv)
		if cfg.dir == "" {
			return
		}
		if err := os.MkdirAll(cfg.dir, 0o700); err != nil {
			log.Printf("[reqcapture] disabled: cannot create %q: %v", cfg.dir, err)
			return
		}
		out = &sink{path: filepath.Join(cfg.dir, captureFileName), maxFile: cfg.maxFileBytes, max: cfg.maxRecords}
		enabled = true
		log.Printf("[reqcapture] ENABLED dir=%s paths=%v api_key_ids=%v clients=%v max_body=%d max_file=%dMB max_records=%d "+
			"(captured files contain user prompts; disable when done)",
			cfg.dir, keysOfStr(cfg.paths), keysOfInt(cfg.keyIDs), keysOfStr(cfg.clients), cfg.maxBody, cfg.maxFileBytes>>20, cfg.maxRecords)
	})
}

// Reload drops the cached configuration and file handle so that the environment
// is read again on next use. It is meant for tests and controlled reconfiguration;
// it must not be called while requests are being captured.
func Reload() {
	closeSink()
	once = sync.Once{}
	cfg, enabled, out = settings{}, false, nil
}

// Enabled reports whether capture is turned on for this process.
func Enabled() bool {
	load()
	return enabled
}

// PathMatches reports whether the request path should be captured.
func PathMatches(path string) bool {
	load()
	_, ok := cfg.paths[path]
	return ok
}

// KeyAllowed reports whether the api key id passes the optional filter.
func KeyAllowed(apiKeyID int64) bool {
	load()
	if len(cfg.keyIDs) == 0 {
		return true
	}
	_, ok := cfg.keyIDs[apiKeyID]
	return ok
}

// ClientAllowed reports whether the detected client kind passes the optional filter.
func ClientAllowed(kind string) bool {
	load()
	if len(cfg.clients) == 0 {
		return true
	}
	_, ok := cfg.clients[strings.ToLower(kind)]
	return ok
}

// Meta ties upstream events back to the ingress request.
type Meta struct {
	RequestID       string
	ClientRequestID string
	APIKeyID        int64
	GroupID         int64
	ClientKind      string
}

type metaKey struct{}

// WithMeta marks ctx as "being captured" and carries the correlation data.
func WithMeta(ctx context.Context, m Meta) context.Context {
	return context.WithValue(ctx, metaKey{}, m)
}

// MetaFrom returns the capture metadata stored by WithMeta.
func MetaFrom(ctx context.Context) (Meta, bool) {
	if ctx == nil {
		return Meta{}, false
	}
	m, ok := ctx.Value(metaKey{}).(Meta)
	return m, ok
}

// Active reports whether ctx belongs to a captured request.
func Active(ctx context.Context) bool {
	_, ok := MetaFrom(ctx)
	return ok
}

// Record appends one event as a JSON line. Safe for concurrent use; no-op when disabled.
func Record(ev map[string]any) {
	if !Enabled() {
		return
	}
	ev["ts"] = time.Now().UTC().Format(time.RFC3339Nano)
	b, err := json.Marshal(ev)
	if err != nil {
		log.Printf("[reqcapture] marshal failed: %v", err)
		return
	}
	out.write(append(b, '\n'))
}

// AttachBody stores body under key together with its size and sha256. Valid JSON
// within the size cap is embedded as JSON (so it can be queried with jq); anything
// else is stored as a (possibly truncated) string.
func AttachBody(ev map[string]any, key string, body []byte) {
	load()
	sum := sha256.Sum256(body)
	ev[key+"_bytes"] = len(body)
	ev[key+"_sha256"] = hex.EncodeToString(sum[:])
	max := cfg.maxBody
	if max <= 0 {
		max = defaultMaxBody
	}
	if len(body) <= max && json.Valid(body) {
		ev[key] = json.RawMessage(append([]byte(nil), body...))
		return
	}
	trunc := len(body) > max
	if trunc {
		body = body[:max]
	}
	ev[key] = string(body)
	ev[key+"_truncated"] = trunc
}

// RedactHeaders flattens headers and masks anything credential-like.
func RedactHeaders(h http.Header) map[string]string {
	res := make(map[string]string, len(h))
	for k, vs := range h {
		v := strings.Join(vs, ", ")
		if isSensitiveHeader(k) {
			res[k] = fmt.Sprintf("<redacted len=%d>", len(v))
			continue
		}
		res[k] = v
	}
	return res
}

func isSensitiveHeader(name string) bool {
	n := strings.ToLower(name)
	for _, frag := range []string{"authorization", "api-key", "apikey", "token", "secret", "cookie", "password", "x-goog-api"} {
		if strings.Contains(n, frag) {
			return true
		}
	}
	return false
}

func keysOfStr(m map[string]struct{}) []string {
	res := make([]string, 0, len(m))
	for k := range m {
		res = append(res, k)
	}
	return res
}

func keysOfInt(m map[int64]struct{}) []int64 {
	res := make([]int64, 0, len(m))
	for k := range m {
		res = append(res, k)
	}
	return res
}

// ---------------------------------------------------------------------------
// sink: size-rotated, record-capped JSONL writer
// ---------------------------------------------------------------------------

type sink struct {
	mu      sync.Mutex
	path    string
	f       *os.File
	size    int64
	maxFile int64
	max     int64
	records atomic.Int64
	warned  atomic.Bool
}

func (s *sink) write(line []byte) {
	if s.records.Add(1) > s.max {
		if s.warned.CompareAndSwap(false, true) {
			log.Printf("[reqcapture] record limit (%d) reached; further events are dropped until restart", s.max)
		}
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.f == nil {
		if err := s.open(); err != nil {
			log.Printf("[reqcapture] open %s: %v", s.path, err)
			return
		}
	}
	if s.size > 0 && s.size+int64(len(line)) > s.maxFile {
		s.rotate()
		if s.f == nil {
			return
		}
	}
	n, err := s.f.Write(line)
	s.size += int64(n)
	if err != nil {
		log.Printf("[reqcapture] write: %v", err)
	}
}

func (s *sink) open() error {
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	s.f, s.size = f, st.Size()
	return nil
}

func (s *sink) rotate() {
	_ = s.f.Close()
	s.f = nil
	_ = os.Remove(fmt.Sprintf("%s.%d", s.path, keepRotated))
	for i := keepRotated - 1; i >= 1; i-- {
		_ = os.Rename(fmt.Sprintf("%s.%d", s.path, i), fmt.Sprintf("%s.%d", s.path, i+1))
	}
	_ = os.Rename(s.path, s.path+".1")
	if err := s.open(); err != nil {
		log.Printf("[reqcapture] reopen after rotate: %v", err)
	}
}

// closeSink releases the file handle (writes are unbuffered); used by tests.
func closeSink() {
	if out == nil {
		return
	}
	out.mu.Lock()
	defer out.mu.Unlock()
	if out.f != nil {
		_ = out.f.Close()
		out.f = nil
	}
}
