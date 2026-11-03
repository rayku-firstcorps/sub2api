package reqcapture

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// Windows does not honour Unix permission bits, so only assert 0600 elsewhere.
func runtimeIsUnix() bool { return runtime.GOOS != "windows" }

// reset re-reads the environment and drops the singleton sink.
func reset(t *testing.T, env map[string]string) string {
	t.Helper()
	closeSink()
	once = sync.Once{}
	cfg, enabled, out = settings{}, false, nil
	dir := t.TempDir()
	for _, k := range []string{EnvDir, envPaths, envKeyIDs, envClients, envMaxBody, envMaxFileMB, envMaxRecords} {
		t.Setenv(k, "")
	}
	if _, ok := env[EnvDir]; !ok {
		env[EnvDir] = dir
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
	t.Cleanup(func() {
		closeSink()
		once = sync.Once{}
		cfg, enabled, out = settings{}, false, nil
	})
	return env[EnvDir]
}

func readEvents(t *testing.T, dir string) []map[string]any {
	t.Helper()
	f, err := os.Open(filepath.Join(dir, captureFileName))
	if err != nil {
		t.Fatalf("open capture file: %v", err)
	}
	defer f.Close()
	var evs []map[string]any
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
	for sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatalf("invalid jsonl line: %v\n%s", err, sc.Text())
		}
		evs = append(evs, m)
	}
	return evs
}

func TestDisabledByDefault(t *testing.T) {
	reset(t, map[string]string{EnvDir: ""})
	if Enabled() {
		t.Fatal("capture must be disabled when SUB2API_CAPTURE_DIR is unset")
	}
	Record(map[string]any{"event": "x"}) // must not panic or write anywhere
}

func TestFiltersAndDefaults(t *testing.T) {
	reset(t, map[string]string{envKeyIDs: "10, 12", envClients: "Hermes", envPaths: "/v1/messages,/v1/chat/completions"})
	if !Enabled() {
		t.Fatal("expected enabled")
	}
	if !PathMatches("/v1/messages") || !PathMatches("/v1/chat/completions") || PathMatches("/v1/models") {
		t.Fatal("path filter wrong")
	}
	if !KeyAllowed(10) || !KeyAllowed(12) || KeyAllowed(11) {
		t.Fatal("api key filter wrong")
	}
	if !ClientAllowed("hermes") || ClientAllowed("claude-code") {
		t.Fatal("client filter must be case-insensitive and exclusive")
	}

	reset(t, map[string]string{})
	if !PathMatches("/v1/messages") || PathMatches("/v1/responses") {
		t.Fatal("default path must be /v1/messages only")
	}
	if !KeyAllowed(999) || !ClientAllowed("anything") {
		t.Fatal("no filter configured must allow all")
	}
}

func TestRedactHeaders(t *testing.T) {
	h := http.Header{}
	h.Set("Authorization", "Bearer sk-secret-value")
	h.Set("X-Api-Key", "sk-another")
	h.Set("Cookie", "a=b")
	h.Set("X-Goog-Api-Key", "g")
	h.Set("Anthropic-Beta", "interleaved-thinking-2025-05-14")
	h.Set("User-Agent", "claude-cli/2.0")
	got := RedactHeaders(h)
	for _, k := range []string{"Authorization", "X-Api-Key", "Cookie", "X-Goog-Api-Key"} {
		if !strings.HasPrefix(got[k], "<redacted") {
			t.Errorf("%s not redacted: %q", k, got[k])
		}
	}
	for k, v := range got {
		if strings.Contains(v, "sk-secret-value") || strings.Contains(v, "sk-another") {
			t.Errorf("secret leaked via %s", k)
		}
	}
	if got["Anthropic-Beta"] == "" || got["User-Agent"] == "" {
		t.Error("non-sensitive headers must be preserved")
	}
}

func TestAttachBody(t *testing.T) {
	reset(t, map[string]string{envMaxBody: "64"})

	ev := map[string]any{}
	AttachBody(ev, "body", []byte(`{"a":1}`))
	if _, ok := ev["body"].(json.RawMessage); !ok {
		t.Fatalf("valid small JSON must be embedded as JSON, got %T", ev["body"])
	}
	if ev["body_bytes"] != 7 || ev["body_sha256"] == "" {
		t.Fatalf("size/hash missing: %+v", ev)
	}
	if _, has := ev["body_truncated"]; has {
		t.Fatal("untruncated body must not carry the truncated flag")
	}

	big := []byte(`{"k":"` + strings.Repeat("x", 200) + `"}`)
	ev = map[string]any{}
	AttachBody(ev, "body", big)
	s, ok := ev["body"].(string)
	if !ok || len(s) != 64 || ev["body_truncated"] != true || ev["body_bytes"] != len(big) {
		t.Fatalf("oversized body must be truncated to the cap and flagged: %+v", ev)
	}

	ev = map[string]any{}
	AttachBody(ev, "body", []byte("not json"))
	if s, ok := ev["body"].(string); !ok || s != "not json" {
		t.Fatalf("non-JSON body must be kept as a string: %+v", ev["body"])
	}
}

func TestRecordWritesJSONLines(t *testing.T) {
	dir := reset(t, map[string]string{})
	Record(map[string]any{"event": "ingress", "request_id": "r1"})
	Record(map[string]any{"event": "egress", "request_id": "r1", "status": 429})
	evs := readEvents(t, dir)
	if len(evs) != 2 || evs[0]["event"] != "ingress" || evs[1]["status"] != float64(429) || evs[0]["ts"] == nil {
		t.Fatalf("unexpected events: %+v", evs)
	}
	st, err := os.Stat(filepath.Join(dir, captureFileName))
	if err != nil {
		t.Fatal(err)
	}
	if runtimeIsUnix() && st.Mode().Perm() != 0o600 {
		t.Fatalf("capture file must be private, got %v", st.Mode().Perm())
	}
}

func TestRecordCap(t *testing.T) {
	dir := reset(t, map[string]string{envMaxRecords: "3"})
	for i := 0; i < 10; i++ {
		Record(map[string]any{"event": "e", "i": i})
	}
	if n := len(readEvents(t, dir)); n != 3 {
		t.Fatalf("record cap not enforced: got %d lines, want 3", n)
	}
}

func TestRotation(t *testing.T) {
	// 1 MB cap is the smallest the env allows; write ~2.5 MB so at least one rotation happens.
	dir := reset(t, map[string]string{envMaxFileMB: "1", envMaxRecords: "100000"})
	pad := strings.Repeat("y", 100_000)
	for i := 0; i < 25; i++ {
		Record(map[string]any{"event": "pad", "i": i, "pad": pad})
	}
	if _, err := os.Stat(filepath.Join(dir, captureFileName+".1")); err != nil {
		t.Fatalf("expected a rotated file: %v", err)
	}
	st, _ := os.Stat(filepath.Join(dir, captureFileName))
	if st.Size() > 1<<20+200_000 {
		t.Fatalf("active file did not rotate near the cap: %d bytes", st.Size())
	}
	// every line in every file must still be valid JSON (no torn writes across rotation)
	total := 0
	for _, name := range []string{captureFileName, captureFileName + ".1", captureFileName + ".2"} {
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 1<<20), 8<<20)
		for sc.Scan() {
			var m map[string]any
			if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
				t.Fatalf("torn line in %s: %v", name, err)
			}
			total++
		}
		f.Close()
	}
	if total != 25 {
		t.Fatalf("lost events across rotation: got %d, want 25", total)
	}
}

func TestSummarizeAnthropic(t *testing.T) {
	body := `{
	  "model":"claude-opus-5-5","max_tokens":128000,"stream":true,
	  "thinking":{"type":"adaptive"},"tool_choice":{"type":"auto"},
	  "metadata":{"user_id":"u"},
	  "system":[{"type":"text","text":"abcd"},{"type":"text","text":"ef"}],
	  "messages":[{"role":"user","content":"hi"}],
	  "tools":[
	    {"name":"web_search","description":"d","input_schema":{"type":"object","properties":{"q":{"anyOf":[{"type":"string"},{"type":"null"}],"default":"x"}}}},
	    {"name":"terminal","input_schema":{"type":"object","properties":{"n":{"type":"integer","minimum":1,"maximum":9}}}},
	    {"type":"web_search_20250305","name":"web_search"}
	  ]}`
	s := SummarizeAnthropic([]byte(body))
	f := s.Fields
	if f["model"] != "claude-opus-5-5" || f["max_tokens"] != int64(128000) || f["stream"] != true {
		t.Fatalf("basic fields wrong: %+v", f)
	}
	if f["thinking_type"] != "adaptive" || f["tool_choice"] != "auto" || f["has_metadata_user_id"] != true {
		t.Fatalf("thinking/tool_choice/metadata wrong: %+v", f)
	}
	if f["system_blocks"] != 2 || f["system_chars"] != 6 || f["messages"] != 1 || f["last_message_role"] != "user" {
		t.Fatalf("system/messages wrong: %+v", f)
	}
	if f["tool_count"] != 3 || len(s.ToolNames) != 3 {
		t.Fatalf("tool count wrong: %+v", f)
	}
	bt, _ := f["tool_builtin_types"].(map[string]int)
	if bt["web_search_20250305"] != 1 {
		t.Fatalf("built-in tool type must be reported separately: %+v", f["tool_builtin_types"])
	}
	kw, _ := f["tool_schema_keywords"].(map[string]int)
	if kw["anyOf"] != 1 || kw["default"] != 1 || kw["minimum"] != 1 || kw["maximum"] != 1 {
		t.Fatalf("schema keyword counts wrong: %+v", kw)
	}

	if bad := SummarizeAnthropic([]byte("{oops")); bad.Fields["valid_json"] != false {
		t.Fatal("invalid JSON must be flagged, not panic")
	}
	if empty := SummarizeAnthropic([]byte(`{"model":"m"}`)); empty.Fields["tool_count"] != 0 || empty.Fields["system_blocks"] != 0 {
		t.Fatalf("missing optional fields must default to zero: %+v", empty.Fields)
	}
}

func TestClassifyClient(t *testing.T) {
	cases := []struct {
		name  string
		tools []string
		ua    string
		want  string
	}{
		{"hermes by tools", []string{"terminal", "delegate_task", "web_search"}, "", ClientHermes},
		{"hermes beats claude-cli UA when its tools are present", []string{"browser_exec"}, "claude-cli/1.0", ClientHermes},
		{"claude code by tools", []string{"Bash", "Read", "Edit", "Glob"}, "", ClientClaudeCode},
		{"claude code by UA", nil, "claude-cli/2.1.0 (external, cli)", ClientClaudeCode},
		{"unknown", []string{"foo"}, "curl/8", ClientUnknown},
		{"bash+read only is not enough", []string{"Bash", "Read"}, "", ClientUnknown},
	}
	for _, c := range cases {
		if got := ClassifyClient(c.tools, c.ua); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}
