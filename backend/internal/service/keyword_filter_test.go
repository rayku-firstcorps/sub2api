package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

func TestKeywordFilterService_MatchesNormalizedKeyword(t *testing.T) {
	svc := NewKeywordFilterService(nil, nil, nil)
	cfg := defaultKeywordFilterConfig()
	cfg.Keywords = []string{"testABCword"}

	input := svc.normalizeText("\uff34\uff45\uff53\uff54-\uff41\uff42\uff43 word")
	match := svc.match(cfg, input)
	if match == nil {
		t.Fatalf("expected normalized keyword match")
	}
	if match.MatchType != KeywordFilterMatchTypeKeyword {
		t.Fatalf("match type = %s, want keyword", match.MatchType)
	}
}

func TestKeywordFilterService_MatchesChineseAndASCIIKeywords(t *testing.T) {
	svc := NewKeywordFilterService(nil, nil, nil)
	cfg := defaultKeywordFilterConfig()
	cfg.Keywords = []string{"\u8fdd\u89c4\u8bcd", "badword"}

	if match := svc.match(cfg, svc.normalizeText("\u8fd9\u91cc\u6709\u8fdd-\u89c4 \u8bcd")); match == nil {
		t.Fatalf("expected noisy Chinese keyword to match")
	}
	if match := svc.match(cfg, svc.normalizeText("This is BAD-WORD")); match == nil {
		t.Fatalf("expected noisy ASCII keyword to match")
	}
}

func TestKeywordFilterService_WhitelistCoversKeyword(t *testing.T) {
	svc := NewKeywordFilterService(nil, nil, nil)
	cfg := defaultKeywordFilterConfig()
	cfg.Keywords = []string{"bad"}
	cfg.Whitelist = []string{"notbad"}

	if match := svc.match(cfg, svc.normalizeText("not bad")); match != nil {
		t.Fatalf("expected whitelist-covered keyword to pass, got %#v", match)
	}
	if match := svc.match(cfg, svc.normalizeText("not-bad")); match != nil {
		t.Fatalf("expected whitelist-covered noisy keyword to pass, got %#v", match)
	}
	if match := svc.match(cfg, svc.normalizeText("very bad")); match == nil {
		t.Fatalf("expected uncovered keyword to match")
	}
	if match := svc.match(cfg, svc.normalizeText("not bad and bad")); match == nil {
		t.Fatalf("expected later uncovered keyword to match")
	}
}

func TestKeywordFilterService_RegexDefaultsDisabled(t *testing.T) {
	svc := NewKeywordFilterService(nil, nil, nil)
	cfg := defaultKeywordFilterConfig()

	if match := svc.match(cfg, svc.normalizeText("call 13800138000")); match != nil {
		t.Fatalf("expected disabled default regex to pass, got %#v", match)
	}
	cfg.RegexRules[0].Enabled = true
	if match := svc.match(cfg, svc.normalizeText("call 13800138000")); match == nil || match.MatchType != KeywordFilterMatchTypeRegex {
		t.Fatalf("expected enabled regex to match, got %#v", match)
	}
	cfg.RegexRules[0].Enabled = false
	cfg.RegexRules[1].Enabled = true
	if match := svc.match(cfg, svc.normalizeText("open https://example.com/a?token=secret")); match == nil || match.MatchType != KeywordFilterMatchTypeRegex {
		t.Fatalf("expected enabled URL regex to match, got %#v", match)
	}
}

func TestKeywordFilterService_TokenModeAvoidsEnglishFalsePositives(t *testing.T) {
	svc := NewKeywordFilterService(nil, nil, nil)
	cfg := defaultKeywordFilterConfig()
	cfg.KeywordRules = []KeywordFilterRule{
		{ID: "ass", Pattern: "ass", MatchMode: KeywordFilterMatchModeAuto, Enabled: true, Action: KeywordFilterActionBlock},
		{ID: "drug", Pattern: "drug", MatchMode: KeywordFilterMatchModeAuto, Enabled: true, Action: KeywordFilterActionBlock},
		{ID: "sex", Pattern: "sex", MatchMode: KeywordFilterMatchModeAuto, Enabled: true, Action: KeywordFilterActionBlock},
	}

	if match := svc.match(cfg, svc.normalizeText("classic class name")); match != nil {
		t.Fatalf("expected ass not to match class, got %#v", match)
	}
	if match := svc.match(cfg, svc.normalizeText("red rug")); match != nil {
		t.Fatalf("expected drug not to match red rug, got %#v", match)
	}
	if match := svc.match(cfg, svc.normalizeText("Sussex")); match != nil {
		t.Fatalf("expected sex not to match Sussex, got %#v", match)
	}
	if match := svc.match(cfg, svc.normalizeText("bad ass")); match == nil || match.ResolvedMatchMode != KeywordFilterMatchModeToken {
		t.Fatalf("expected token match, got %#v", match)
	}
}

func TestKeywordFilterService_CJKTokenAvoidsContinuousSubstringFalsePositive(t *testing.T) {
	svc := NewKeywordFilterService(nil, nil, nil)
	cfg := defaultKeywordFilterConfig()
	cfg.KeywordRules = []KeywordFilterRule{
		{ID: "cjk", Pattern: "\u6027\u611f", MatchMode: KeywordFilterMatchModeAuto, Enabled: true, Action: KeywordFilterActionBlock},
	}

	if match := svc.match(cfg, svc.normalizeText("\u6709\u4e3b\u89c2\u6027\u611f\u609f")); match != nil {
		t.Fatalf("expected cjk token not to match inside continuous phrase, got %#v", match)
	}
	if match := svc.match(cfg, svc.normalizeText("\u8fd9\u662f \u6027\u611f \u5185\u5bb9")); match == nil || match.ResolvedMatchMode != KeywordFilterMatchModeCJKToken {
		t.Fatalf("expected cjk token match, got %#v", match)
	}
}

func TestKeywordFilterService_MixedExactPhrase(t *testing.T) {
	svc := NewKeywordFilterService(nil, nil, nil)
	cfg := defaultKeywordFilterConfig()
	cfg.KeywordRules = []KeywordFilterRule{
		{ID: "mixed", Pattern: "AI\u8bc8\u9a97", MatchMode: KeywordFilterMatchModeAuto, Enabled: true, Action: KeywordFilterActionBlock},
	}

	if match := svc.match(cfg, svc.normalizeText("AI\u8bc8\u9a97")); match == nil || match.ResolvedMatchMode != KeywordFilterMatchModeExactPhrase {
		t.Fatalf("expected mixed exact phrase match, got %#v", match)
	}
	if match := svc.match(cfg, svc.normalizeText("AI \u8bc8\u9a97")); match == nil {
		t.Fatalf("expected mixed exact phrase with space to match")
	}
	if match := svc.match(cfg, svc.normalizeText("paid\u8bc8\u9a97")); match != nil {
		t.Fatalf("expected mixed phrase not to match inside latin token, got %#v", match)
	}
}

func TestKeywordFilterService_FuzzyModeAllowsWeakPunctuation(t *testing.T) {
	svc := NewKeywordFilterService(nil, nil, nil)
	cfg := defaultKeywordFilterConfig()
	cfg.KeywordRules = []KeywordFilterRule{
		{ID: "fuzzy", Pattern: "badword", MatchMode: KeywordFilterMatchModeFuzzy, Enabled: true, Action: KeywordFilterActionBlock},
	}

	if match := svc.match(cfg, svc.normalizeText("b-a_d w.o+r=d")); match == nil || match.ResolvedMatchMode != KeywordFilterMatchModeFuzzy {
		t.Fatalf("expected fuzzy noisy match, got %#v", match)
	}
}

func TestKeywordFilterService_WhitelistTargetRuleIDs(t *testing.T) {
	svc := NewKeywordFilterService(nil, nil, nil)
	cfg := defaultKeywordFilterConfig()
	cfg.KeywordRules = []KeywordFilterRule{
		{ID: "bad", Pattern: "bad", MatchMode: KeywordFilterMatchModeContains, Enabled: true, Action: KeywordFilterActionBlock},
		{ID: "evil", Pattern: "evil", MatchMode: KeywordFilterMatchModeContains, Enabled: true, Action: KeywordFilterActionBlock},
	}
	cfg.WhitelistRules = []KeywordFilterWhitelistRule{
		{ID: "safe_bad", Pattern: "safe bad", MatchMode: KeywordFilterMatchModeContains, TargetRuleIDs: []string{"bad"}, Enabled: true},
	}

	if match := svc.match(cfg, svc.normalizeText("safe bad")); match != nil {
		t.Fatalf("expected targeted whitelist to cover bad, got %#v", match)
	}
	if match := svc.match(cfg, svc.normalizeText("safe evil")); match == nil {
		t.Fatalf("expected whitelist not to cover non-target rule")
	}
}

func TestExtractKeywordFilterSegments_OpenAIChatParts(t *testing.T) {
	body := []byte(`{
		"messages": [
			{"role":"user","content":[{"type":"text","text":"first part"},{"type":"text","text":"second part"}]},
			{"role":"assistant","content":"assistant text"}
		]
	}`)
	got := ExtractKeywordFilterSegments(ContentModerationProtocolOpenAIChat, body)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2: %#v", len(got), got)
	}
	if got[0].MessageIndex != 0 || got[0].PartIndex != 0 || got[1].PartIndex != 1 {
		t.Fatalf("unexpected segment indexes: %#v", got)
	}
}

func TestExtractKeywordFilterTexts_AllUserTextOnly(t *testing.T) {
	body := []byte(`{
		"messages": [
			{"role":"system","content":"system secret"},
			{"role":"user","content":"first user"},
			{"role":"assistant","content":"assistant text"},
			{"role":"user","content":[{"type":"text","text":"second user"},{"type":"image_url","image_url":{"url":"https://example.com/a.png"}}]}
		]
	}`)
	got := ExtractKeywordFilterTexts(ContentModerationProtocolOpenAIChat, body)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2: %#v", len(got), got)
	}
	if got[0] != "first user" || got[1] != "second user" {
		t.Fatalf("unexpected texts: %#v", got)
	}
}

func TestExtractKeywordFilterTexts_AnthropicUserText(t *testing.T) {
	body := []byte(`{
		"messages": [
			{"role":"assistant","content":"assistant text"},
			{"role":"user","content":"first user"},
			{"role":"user","content":[{"type":"text","text":"second user"},{"type":"image","source":{"type":"base64","data":"abcd"}}]}
		]
	}`)
	got := ExtractKeywordFilterTexts(ContentModerationProtocolAnthropicMessages, body)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2: %#v", len(got), got)
	}
	if got[0] != "first user" || got[1] != "second user" {
		t.Fatalf("unexpected texts: %#v", got)
	}
}

func TestKeywordFilterService_TestUsesInlineConfig(t *testing.T) {
	svc := NewKeywordFilterService(nil, nil, nil)
	keywords := []string{"\u4e34\u65f6\u9ed1\u8bcd"}
	whitelist := []string{}
	result, err := svc.Test(context.Background(), KeywordFilterTestInput{
		Text: "\u547d\u4e2d\u4e34\u65f6\u9ed1\u8bcd",
		Config: &UpdateKeywordFilterConfigInput{
			Keywords:  &keywords,
			Whitelist: &whitelist,
		},
	})
	if err != nil {
		t.Fatalf("Test error: %v", err)
	}
	if result == nil || !result.Blocked || result.MatchType != KeywordFilterMatchTypeKeyword {
		t.Fatalf("expected inline config keyword block, got %#v", result)
	}
}

func TestKeywordFilterService_CheckRespectsSwitches(t *testing.T) {
	repo := &keywordFilterSettingRepoStub{values: map[string]string{
		SettingKeyKeywordFilterEnabled: "false",
	}}
	logRepo := &keywordFilterLogRepoStub{}
	svc := NewKeywordFilterService(repo, logRepo, nil)
	decision, err := svc.Check(context.Background(), KeywordFilterCheckInput{
		Protocol: ContentModerationProtocolOpenAIChat,
		Body:     []byte(`{"messages":[{"role":"user","content":"bad"}]}`),
	})
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if decision == nil || !decision.Allowed || decision.Blocked {
		t.Fatalf("expected allowed when keyword filter system switch disabled: %#v", decision)
	}

	cfg := defaultKeywordFilterConfig()
	cfg.Enabled = true
	cfg.Keywords = []string{"bad"}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	repo.values[SettingKeyKeywordFilterEnabled] = "true"
	repo.values[SettingKeyKeywordFilterConfig] = string(raw)
	decision, err = svc.Check(context.Background(), KeywordFilterCheckInput{
		Protocol: ContentModerationProtocolOpenAIChat,
		Body:     []byte(`{"messages":[{"role":"user","content":"bad"}]}`),
	})
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if decision == nil || decision.Allowed || !decision.Blocked {
		t.Fatalf("expected blocked when system and page switches are enabled: %#v", decision)
	}
	if len(logRepo.logs) != 1 {
		t.Fatalf("expected one keyword filter log, got %d", len(logRepo.logs))
	}
}

type keywordFilterSettingRepoStub struct {
	values map[string]string
}

func (r *keywordFilterSettingRepoStub) Get(context.Context, string) (*Setting, error) {
	return nil, ErrSettingNotFound
}

func (r *keywordFilterSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	if r != nil && r.values != nil {
		if v, ok := r.values[key]; ok {
			return v, nil
		}
	}
	return "", ErrSettingNotFound
}

func (r *keywordFilterSettingRepoStub) Set(_ context.Context, key, value string) error {
	if r.values == nil {
		r.values = map[string]string{}
	}
	r.values[key] = value
	return nil
}

func (r *keywordFilterSettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := map[string]string{}
	for _, key := range keys {
		if v, ok := r.values[key]; ok {
			out[key] = v
		}
	}
	return out, nil
}

func (r *keywordFilterSettingRepoStub) SetMultiple(_ context.Context, values map[string]string) error {
	if r.values == nil {
		r.values = map[string]string{}
	}
	for key, value := range values {
		r.values[key] = value
	}
	return nil
}

func (r *keywordFilterSettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	return r.values, nil
}

func (r *keywordFilterSettingRepoStub) Delete(_ context.Context, key string) error {
	delete(r.values, key)
	return nil
}

type keywordFilterLogRepoStub struct {
	logs []KeywordFilterLog
}

func (r *keywordFilterLogRepoStub) CreateLog(_ context.Context, log *KeywordFilterLog) error {
	if log != nil {
		r.logs = append(r.logs, *log)
	}
	return nil
}

func (r *keywordFilterLogRepoStub) ListLogs(context.Context, KeywordFilterLogFilter) ([]KeywordFilterLog, *pagination.PaginationResult, error) {
	return r.logs, &pagination.PaginationResult{}, nil
}

func (r *keywordFilterLogRepoStub) CleanupExpiredLogs(context.Context, time.Time) (*KeywordFilterCleanupResult, error) {
	return &KeywordFilterCleanupResult{}, nil
}
