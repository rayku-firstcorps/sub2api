package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
	"github.com/tidwall/gjson"
)

const (
	kiroPromptCacheDefaultTTL  = 5 * time.Minute
	kiroPromptCacheExtendedTTL = time.Hour
	kiroPromptCacheToolDescMax = 9216
)

// KiroCacheBreakpoint is a cumulative prompt-cache boundary for pseudo billing.
type KiroCacheBreakpoint struct {
	Hash   string
	Tokens int
	TTL    time.Duration
}

// KiroCacheResult is the pseudo cache billing result for one request.
type KiroCacheResult struct {
	CacheReadInputTokens     int
	CacheCreationInputTokens int
	CacheCreation5mTokens    int
	CacheCreation1hTokens    int
	UncachedInputTokens      int
}

// KiroPromptCache stores Kiro pseudo prompt-cache breakpoints.
type KiroPromptCache interface {
	LookupOrCreate(ctx context.Context, namespace string, breakpoints []KiroCacheBreakpoint, totalInputTokens int) (KiroCacheResult, error)
}

func computeKiroCacheBreakpoints(body []byte) []KiroCacheBreakpoint {
	parsed := gjson.ParseBytes(body)
	hasher := sha256.New()
	breakpoints := make([]KiroCacheBreakpoint, 0, 4)
	cumulativeTokens := 0

	tools := parsed.Get("tools")
	if tools.IsArray() {
		toolsArr := tools.Array()
		sort.SliceStable(toolsArr, func(i, j int) bool {
			return toolsArr[i].Get("name").String() < toolsArr[j].Get("name").String()
		})
		for _, tool := range toolsArr {
			normalized := normalizeKiroCacheTool(tool)
			if normalized == "" {
				continue
			}
			_, _ = hasher.Write([]byte(normalized))
			cumulativeTokens += estimateTokensFromRuneCount(utf8.RuneCountInString(normalized))
			if cc := tool.Get("cache_control"); cc.Exists() {
				breakpoints = append(breakpoints, newKiroCacheBreakpoint(hasher, cumulativeTokens, cc))
			}
		}
	}

	system := parsed.Get("system")
	switch {
	case system.IsArray():
		for _, item := range system.Array() {
			text := normalizeKiroCacheSystemText(systemBlockText(item))
			_, _ = hasher.Write([]byte(text))
			cumulativeTokens += estimateTokensFromRuneCount(utf8.RuneCountInString(text))
			if cc := item.Get("cache_control"); cc.Exists() {
				breakpoints = append(breakpoints, newKiroCacheBreakpoint(hasher, cumulativeTokens, cc))
			}
		}
	case system.IsObject():
		text := normalizeKiroCacheSystemText(system.Get("text").String())
		_, _ = hasher.Write([]byte(text))
		cumulativeTokens += estimateTokensFromRuneCount(utf8.RuneCountInString(text))
		if cc := system.Get("cache_control"); cc.Exists() {
			breakpoints = append(breakpoints, newKiroCacheBreakpoint(hasher, cumulativeTokens, cc))
		}
	case system.Type == gjson.String:
		text := normalizeKiroCacheSystemText(system.String())
		_, _ = hasher.Write([]byte(text))
		cumulativeTokens += estimateTokensFromRuneCount(utf8.RuneCountInString(text))
	}

	messages := parsed.Get("messages")
	if messages.IsArray() {
		for _, msg := range messages.Array() {
			content := msg.Get("content")
			if content.IsArray() {
				for _, block := range content.Array() {
					blockJSON := normalizeKiroCacheBlock(block.Raw)
					_, _ = hasher.Write([]byte(blockJSON))
					if block.Get("type").String() == "text" {
						text := block.Get("text").String()
						cumulativeTokens += estimateTokensFromRuneCount(utf8.RuneCountInString(text))
					}
					if cc := block.Get("cache_control"); cc.Exists() {
						breakpoints = append(breakpoints, newKiroCacheBreakpoint(hasher, cumulativeTokens, cc))
					}
				}
				continue
			}
			if content.Type == gjson.String {
				text := content.String()
				_, _ = hasher.Write([]byte(text))
				cumulativeTokens += estimateTokensFromRuneCount(utf8.RuneCountInString(text))
			}
		}
	}

	return breakpoints
}

func normalizeKiroCacheTool(tool gjson.Result) string {
	name := tool.Get("name").String()
	lowerName := strings.ToLower(name)
	if lowerName == "web_search" || lowerName == "websearch" {
		return ""
	}

	desc := tool.Get("description").String()
	if strings.TrimSpace(desc) == "" {
		return ""
	}
	if len(desc) > kiroPromptCacheToolDescMax {
		desc = desc[:kiroPromptCacheToolDescMax] + "..."
	}

	parts := []string{"name:" + kiro.ShortenToolName(name), "desc:" + desc}
	if schemaRaw := tool.Get("input_schema").Raw; strings.TrimSpace(schemaRaw) != "" {
		if schema := normalizeKiroCacheSchema(schemaRaw); schema != "" {
			parts = append(parts, "schema:"+schema)
		}
	}
	return strings.Join(parts, "|")
}

func normalizeKiroCacheSchema(raw string) string {
	normalized := normalizeKiroCacheJSONWithoutMetadata(raw)
	if normalized == "{}" || normalized == "null" {
		return ""
	}
	return normalized
}

func normalizeKiroCacheBlock(raw string) string {
	return normalizeKiroCacheJSONWithoutMetadata(raw)
}

func normalizeKiroCacheJSONWithoutMetadata(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return raw
	}
	data, err := json.Marshal(omitKiroCacheMetadata(v))
	if err != nil {
		return raw
	}
	return string(data)
}

func omitKiroCacheMetadata(value any) any {
	switch v := value.(type) {
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = omitKiroCacheMetadata(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			if key == "cache_control" {
				continue
			}
			out[key] = omitKiroCacheMetadata(item)
		}
		return out
	default:
		return value
	}
}

func systemBlockText(item gjson.Result) string {
	if item.Type == gjson.String {
		return item.String()
	}
	return item.Get("text").String()
}

func normalizeKiroCacheSystemText(text string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(text)), "x-anthropic-billing-header:") {
		return ""
	}
	return text
}

func newKiroCacheBreakpoint(hasher interface {
	Sum([]byte) []byte
}, tokens int, cc gjson.Result) KiroCacheBreakpoint {
	return KiroCacheBreakpoint{
		Hash:   fmt.Sprintf("%x", hasher.Sum(nil)),
		Tokens: tokens,
		TTL:    kiroCacheControlTTL(cc),
	}
}

func kiroCacheControlTTL(cc gjson.Result) time.Duration {
	if cc.Get("ttl").String() == "1h" {
		return kiroPromptCacheExtendedTTL
	}
	return kiroPromptCacheDefaultTTL
}

func applyKiroCacheResultToUsage(usage *ClaudeUsage, result KiroCacheResult) {
	if usage == nil {
		return
	}
	usage.InputTokens = result.UncachedInputTokens
	usage.CacheReadInputTokens = result.CacheReadInputTokens
	usage.CacheCreationInputTokens = result.CacheCreationInputTokens
	usage.CacheCreation5mTokens = result.CacheCreation5mTokens
	usage.CacheCreation1hTokens = result.CacheCreation1hTokens
}

func kiroPromptCacheNamespace(account *Account, apiKey *APIKey) string {
	credentialID := ""
	if account != nil {
		credentialID = firstNonEmptyKiroCacheValue(
			account.GetCredential("uuid"),
			account.GetCredential("profile_arn"),
			account.GetCredential("client_id"),
			account.GetCredential("clientId"),
			account.GetCredential("sso_client_id"),
		)
		if credentialID == "" && account.ID > 0 {
			credentialID = fmt.Sprintf("account:%d", account.ID)
		}
	}
	if credentialID != "" {
		return "kiro:" + credentialID
	}
	if apiKey != nil && apiKey.ID > 0 {
		return fmt.Sprintf("apikey:%d", apiKey.ID)
	}
	return ""
}

func firstNonEmptyKiroCacheValue(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
