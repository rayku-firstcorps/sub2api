package reqcapture

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/tidwall/gjson"
)

// Client kinds returned by ClassifyClient.
const (
	ClientClaudeCode = "claude-code"
	ClientHermes     = "hermes"
	ClientUnknown    = "unknown"
)

// Summary is a compact, comparison-friendly description of an Anthropic
// /v1/messages request. Fields is embedded in the ingress event; ToolNames feeds
// client classification.
type Summary struct {
	Fields    map[string]any
	ToolNames []string
}

// SummarizeAnthropic extracts the request shape (not the content) so different
// clients can be compared at a glance without opening the full body.
func SummarizeAnthropic(body []byte) Summary {
	f := map[string]any{}
	if !gjson.ValidBytes(body) {
		f["valid_json"] = false
		return Summary{Fields: f}
	}
	r := gjson.ParseBytes(body)
	f["valid_json"] = true
	f["model"] = r.Get("model").String()
	f["stream"] = r.Get("stream").Bool()
	f["max_tokens"] = r.Get("max_tokens").Int()
	for _, k := range []string{"temperature", "top_p", "top_k"} {
		if v := r.Get(k); v.Exists() {
			f[k] = v.Value()
		}
	}
	if v := r.Get("thinking"); v.Exists() {
		f["thinking_type"] = v.Get("type").String()
		if b := v.Get("budget_tokens"); b.Exists() {
			f["thinking_budget_tokens"] = b.Int()
		}
	}
	if v := r.Get("output_config.effort"); v.Exists() {
		f["output_effort"] = v.String()
	}
	if v := r.Get("tool_choice"); v.Exists() {
		f["tool_choice"] = v.Get("type").String()
	}
	f["has_metadata_user_id"] = r.Get("metadata.user_id").Exists()

	sys := r.Get("system")
	switch {
	case sys.IsArray():
		chars := 0
		sys.ForEach(func(_, b gjson.Result) bool {
			chars += len(b.Get("text").String())
			return true
		})
		f["system_blocks"] = len(sys.Array())
		f["system_chars"] = chars
	case sys.Exists():
		f["system_blocks"] = 1
		f["system_chars"] = len(sys.String())
	default:
		f["system_blocks"] = 0
		f["system_chars"] = 0
	}

	msgs := r.Get("messages")
	f["messages"] = len(msgs.Array())
	if arr := msgs.Array(); len(arr) > 0 {
		f["last_message_role"] = arr[len(arr)-1].Get("role").String()
	}

	var names []string
	builtinTypes := map[string]int{}
	var schemas []any
	r.Get("tools").ForEach(func(_, t gjson.Result) bool {
		names = append(names, t.Get("name").String())
		if ty := t.Get("type").String(); ty != "" && ty != "custom" {
			builtinTypes[ty]++
		}
		var schema any
		if err := json.Unmarshal([]byte(t.Get("input_schema").Raw), &schema); err == nil && schema != nil {
			schemas = append(schemas, schema)
		}
		return true
	})
	f["tool_count"] = len(names)
	f["tool_names"] = names
	if len(builtinTypes) > 0 {
		f["tool_builtin_types"] = builtinTypes
	}
	if kw := schemaKeywordCounts(schemas); len(kw) > 0 {
		f["tool_schema_keywords"] = kw
	}
	return Summary{Fields: f, ToolNames: names}
}

// interestingSchemaKeys are JSON-Schema constructs that commonly trip format
// converters (Anthropic -> Gemini function declarations).
var interestingSchemaKeys = map[string]struct{}{
	"anyOf": {}, "oneOf": {}, "allOf": {}, "not": {}, "$ref": {}, "$defs": {}, "definitions": {},
	"pattern": {}, "patternProperties": {}, "additionalProperties": {}, "const": {}, "format": {},
	"minItems": {}, "maxItems": {}, "uniqueItems": {}, "minimum": {}, "maximum": {},
	"minLength": {}, "maxLength": {}, "default": {}, "examples": {}, "nullable": {},
	"if": {}, "then": {}, "else": {}, "propertyNames": {}, "dependencies": {},
}

func schemaKeywordCounts(schemas []any) map[string]int {
	counts := map[string]int{}
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, child := range x {
				if _, ok := interestingSchemaKeys[k]; ok {
					counts[k]++
				}
				if k == "type" {
					if _, isList := child.([]any); isList {
						counts["type_as_list"]++
					}
				}
				walk(child)
			}
		case []any:
			for _, child := range x {
				walk(child)
			}
		}
	}
	for _, s := range schemas {
		walk(s)
	}
	return counts
}

// ClassifyClient guesses the calling client. Tool names are the primary signal
// because every client reaches sub2api through new-api, which may replace the
// User-Agent.
func ClassifyClient(toolNames []string, userAgent string) string {
	set := make(map[string]struct{}, len(toolNames))
	for _, n := range toolNames {
		set[n] = struct{}{}
	}
	has := func(names ...string) bool {
		for _, n := range names {
			if _, ok := set[n]; ok {
				return true
			}
		}
		return false
	}
	ua := strings.ToLower(userAgent)
	switch {
	case has("delegate_task", "browser_exec", "tool_search", "skill_view", "execute_code", "text_to_speech"):
		return ClientHermes
	case strings.Contains(ua, "claude-cli"), strings.Contains(ua, "claude-code"),
		has("Bash") && has("Read") && has("Edit", "Write"):
		return ClientClaudeCode
	default:
		return ClientUnknown
	}
}

// SortedToolNames returns a stable copy, handy for diffing two requests.
func SortedToolNames(names []string) []string {
	c := append([]string(nil), names...)
	sort.Strings(c)
	return c
}
