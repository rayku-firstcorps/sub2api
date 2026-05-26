package service

import (
	"context"
	"encoding/json"
	"strings"
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

func TestKeywordFilterService_NormalizesChineseToSimplified(t *testing.T) {
	svc := NewKeywordFilterService(nil, nil, nil)

	simplifiedCfg := defaultKeywordFilterConfig()
	simplifiedCfg.Keywords = []string{"\u8fdd\u89c4\u8bcd"}
	if match := svc.match(simplifiedCfg, svc.normalizeText("\u9019\u88e1\u6709\u9055\u898f\u8a5e")); match == nil {
		t.Fatalf("expected simplified keyword to match traditional input")
	}

	traditionalCfg := defaultKeywordFilterConfig()
	traditionalCfg.Keywords = []string{"\u9055\u898f\u8a5e"}
	if match := svc.match(traditionalCfg, svc.normalizeText("\u8fd9\u91cc\u6709\u8fdd\u89c4\u8bcd")); match == nil {
		t.Fatalf("expected traditional keyword to match simplified input")
	}

	whitelistCfg := defaultKeywordFilterConfig()
	whitelistCfg.Keywords = []string{"\u8fdd\u89c4\u8bcd"}
	whitelistCfg.Whitelist = []string{"\u514d\u6b7b\u9055\u898f\u8a5e"}
	if match := svc.match(whitelistCfg, svc.normalizeText("\u514d\u6b7b\u8fdd\u89c4\u8bcd")); match != nil {
		t.Fatalf("expected mixed simplified/traditional whitelist to cover keyword, got %#v", match)
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

func TestKeywordFilterService_StreamScanContinuesAfterWhitelist(t *testing.T) {
	svc := NewKeywordFilterService(nil, nil, nil)
	cfg := defaultKeywordFilterConfig()
	cfg.KeywordRules = []KeywordFilterRule{
		{ID: "drug", Pattern: "drug", MatchMode: KeywordFilterMatchModeToken, Enabled: true, Action: KeywordFilterActionBlock},
		{ID: "sex", Pattern: "sex", MatchMode: KeywordFilterMatchModeToken, Enabled: true, Action: KeywordFilterActionBlock},
		{ID: "kill", Pattern: "kill", MatchMode: KeywordFilterMatchModeToken, Enabled: true, Action: KeywordFilterActionBlock},
	}
	cfg.WhitelistRules = []KeywordFilterWhitelistRule{
		{ID: "ok_drug", Pattern: "ok drug", MatchMode: KeywordFilterMatchModeExactPhrase, TargetRuleIDs: []string{"drug"}, Enabled: true},
	}

	match := svc.match(cfg, svc.normalizeText("ok drug sex kill"))
	if match == nil {
		t.Fatalf("expected later uncovered keyword to match")
	}
	if match.RuleID != "sex" {
		t.Fatalf("rule id = %s, want sex: %#v", match.RuleID, match)
	}
}

func TestKeywordFilterService_ValidateRegexRulesRejectsLiteralPatterns(t *testing.T) {
	svc := NewKeywordFilterService(nil, nil, nil)
	cfg := defaultKeywordFilterConfig()
	cfg.RegexRules = []KeywordFilterRegexRule{{Name: "literal", Pattern: "\u6d4b\u8bd5\u4e2d\u6587", Enabled: true}}

	if err := svc.validateConfig(context.Background(), cfg); err == nil {
		t.Fatalf("expected literal regex pattern to be rejected")
	}

	cfg.RegexRules = []KeywordFilterRegexRule{{Name: "anchored_literal", Pattern: `^drug$`, Enabled: true}}
	if err := svc.validateConfig(context.Background(), cfg); err == nil {
		t.Fatalf("expected anchored literal regex pattern to be rejected")
	}

	cfg.RegexRules = []KeywordFilterRegexRule{{Name: "digits", Pattern: `\d{11}`, Enabled: true}}
	if err := svc.validateConfig(context.Background(), cfg); err != nil {
		t.Fatalf("expected structured regex pattern to pass: %v", err)
	}

	cfg.RegexRules = []KeywordFilterRegexRule{{Name: "anchored_key", Pattern: `^sk-[A-Za-z0-9]+$`, Enabled: true}}
	if err := svc.validateConfig(context.Background(), cfg); err != nil {
		t.Fatalf("expected anchored structured regex pattern to pass: %v", err)
	}
}

func TestKeywordFilterService_ValidateRegexRulesRejectsLongPatterns(t *testing.T) {
	svc := NewKeywordFilterService(nil, nil, nil)
	cfg := defaultKeywordFilterConfig()
	cfg.RegexRules = []KeywordFilterRegexRule{{Name: "long", Pattern: `\d` + strings.Repeat("a", maxKeywordFilterRegexPatternRunes), Enabled: true}}

	if err := svc.validateConfig(context.Background(), cfg); err == nil {
		t.Fatalf("expected long regex pattern to be rejected")
	}
}

func TestKeywordFilterService_RegexScansLongInputWithoutChunkAnchorFalsePositive(t *testing.T) {
	svc := NewKeywordFilterService(nil, nil, nil)
	cfg := defaultKeywordFilterConfig()
	cfg.RegexRules = []KeywordFilterRegexRule{{Name: "tail_phone", Pattern: `1[3-9]\d{9}`, Enabled: true}}

	match := svc.match(cfg, svc.normalizeText(strings.Repeat("a", 70000)+" 13800138000"))
	if match == nil || match.MatchType != KeywordFilterMatchTypeRegex {
		t.Fatalf("expected regex match in long input tail, got %#v", match)
	}

	cfg.RegexRules = []KeywordFilterRegexRule{{Name: "anchored", Pattern: `^badword\d+`, Enabled: true}}
	match = svc.match(cfg, svc.normalizeText(strings.Repeat("a", 70000)+"badword123"))
	if match != nil {
		t.Fatalf("expected anchored regex not to match in long input tail, got %#v", match)
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

func TestKeywordFilterService_TestRejectsInvalidBlockStatusLikeUpdate(t *testing.T) {
	svc := NewKeywordFilterService(nil, nil, nil)
	invalidStatus := 200
	_, err := svc.Test(context.Background(), KeywordFilterTestInput{
		Text: "anything",
		Config: &UpdateKeywordFilterConfigInput{
			BlockStatus: &invalidStatus,
		},
	})
	if err == nil {
		t.Fatalf("expected invalid block status to be rejected")
	}
}

func TestKeywordFilterConfig_IncludesGroupEmptyScopedConfigMatchesNoGroups(t *testing.T) {
	cfg := defaultKeywordFilterConfig()
	cfg.AllGroups = false
	cfg.GroupIDs = nil
	groupID := int64(1)

	if cfg.includesGroup(nil) {
		t.Fatalf("expected nil group to be out of scope when group_ids is empty")
	}
	if cfg.includesGroup(&groupID) {
		t.Fatalf("expected concrete group to be out of scope when group_ids is empty")
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

func TestKeywordFilterService_CheckUsesRuntimeCacheUntilConfigRefresh(t *testing.T) {
	cfg := defaultKeywordFilterConfig()
	cfg.Enabled = true
	cfg.Keywords = []string{"bad"}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	repo := &keywordFilterSettingRepoStub{values: map[string]string{
		SettingKeyKeywordFilterEnabled: "true",
		SettingKeyKeywordFilterConfig:  string(raw),
	}}
	logRepo := &keywordFilterLogRepoStub{}
	svc := NewKeywordFilterService(repo, logRepo, nil)
	defer svc.Stop()

	decision, err := svc.Check(context.Background(), KeywordFilterCheckInput{
		Protocol: ContentModerationProtocolOpenAIChat,
		Body:     []byte(`{"messages":[{"role":"user","content":"bad"}]}`),
	})
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if decision == nil || !decision.Blocked {
		t.Fatalf("expected initial keyword block, got %#v", decision)
	}

	cfg.Keywords = []string{"evil"}
	raw, err = json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal updated config: %v", err)
	}
	repo.values[SettingKeyKeywordFilterConfig] = string(raw)
	decision, err = svc.Check(context.Background(), KeywordFilterCheckInput{
		Protocol: ContentModerationProtocolOpenAIChat,
		Body:     []byte(`{"messages":[{"role":"user","content":"evil"}]}`),
	})
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if decision == nil || !decision.Allowed || decision.Blocked {
		t.Fatalf("expected cached rules to remain active before refresh, got %#v", decision)
	}

	keywords := []string{"evil"}
	updated, err := svc.UpdateConfig(context.Background(), UpdateKeywordFilterConfigInput{Keywords: &keywords})
	if err != nil {
		t.Fatalf("UpdateConfig error: %v", err)
	}
	if updated == nil || len(updated.Keywords) != 1 || updated.Keywords[0] != "evil" {
		t.Fatalf("unexpected updated config: %#v", updated)
	}
	decision, err = svc.Check(context.Background(), KeywordFilterCheckInput{
		Protocol: ContentModerationProtocolOpenAIChat,
		Body:     []byte(`{"messages":[{"role":"user","content":"evil"}]}`),
	})
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if decision == nil || !decision.Blocked {
		t.Fatalf("expected refreshed keyword block, got %#v", decision)
	}
}

func TestKeywordFilterService_StopIsIdempotent(t *testing.T) {
	repo := &keywordFilterSettingRepoStub{values: map[string]string{}}
	svc := NewKeywordFilterService(repo, &keywordFilterLogRepoStub{}, nil)
	svc.Stop()
	svc.Stop()
}

func TestKeywordFilterService_LenientModeOnlyScansLastUserMessage(t *testing.T) {
	cfg := defaultKeywordFilterConfig()
	cfg.Enabled = true
	cfg.FilterMode = KeywordFilterModeLenient
	cfg.Keywords = []string{"bad"}
	raw, _ := json.Marshal(cfg)

	repo := &keywordFilterSettingRepoStub{values: map[string]string{
		SettingKeyKeywordFilterEnabled: "true",
		SettingKeyKeywordFilterConfig:  string(raw),
	}}
	logRepo := &keywordFilterLogRepoStub{}
	svc := NewKeywordFilterService(repo, logRepo, nil)

	// History message contains "bad" but last message is clean → should be allowed
	decision, err := svc.Check(context.Background(), KeywordFilterCheckInput{
		Protocol: ContentModerationProtocolOpenAIChat,
		Body:     []byte(`{"messages":[{"role":"user","content":"bad"},{"role":"assistant","content":"ok"},{"role":"user","content":"hello"}]}`),
	})
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if decision == nil || !decision.Allowed || decision.Blocked {
		t.Fatalf("expected allowed in lenient mode when only history matches: %#v", decision)
	}

	// Last user message contains "bad" → should be blocked
	decision, err = svc.Check(context.Background(), KeywordFilterCheckInput{
		Protocol: ContentModerationProtocolOpenAIChat,
		Body:     []byte(`{"messages":[{"role":"user","content":"hello"},{"role":"assistant","content":"ok"},{"role":"user","content":"bad"}]}`),
	})
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if decision == nil || decision.Allowed || !decision.Blocked {
		t.Fatalf("expected blocked in lenient mode when last message matches: %#v", decision)
	}
}

func TestKeywordFilterService_StrictModeScansAllMessages(t *testing.T) {
	cfg := defaultKeywordFilterConfig()
	cfg.Enabled = true
	cfg.FilterMode = KeywordFilterModeStrict
	cfg.Keywords = []string{"bad"}
	raw, _ := json.Marshal(cfg)

	repo := &keywordFilterSettingRepoStub{values: map[string]string{
		SettingKeyKeywordFilterEnabled: "true",
		SettingKeyKeywordFilterConfig:  string(raw),
	}}
	logRepo := &keywordFilterLogRepoStub{}
	svc := NewKeywordFilterService(repo, logRepo, nil)

	// History message contains "bad" → should be blocked in strict mode
	decision, err := svc.Check(context.Background(), KeywordFilterCheckInput{
		Protocol: ContentModerationProtocolOpenAIChat,
		Body:     []byte(`{"messages":[{"role":"user","content":"bad"},{"role":"assistant","content":"ok"},{"role":"user","content":"hello"}]}`),
	})
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if decision == nil || decision.Allowed || !decision.Blocked {
		t.Fatalf("expected blocked in strict mode when history matches: %#v", decision)
	}
}

func TestExtractKeywordFilterLastUserSegments_OpenAIChat(t *testing.T) {
	body := []byte(`{
		"messages": [
			{"role":"user","content":"first user"},
			{"role":"assistant","content":"assistant text"},
			{"role":"user","content":[{"type":"text","text":"second user"},{"type":"text","text":"third part"}]}
		]
	}`)
	got := ExtractKeywordFilterLastUserSegments(ContentModerationProtocolOpenAIChat, body)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2: %#v", len(got), got)
	}
	if got[0].Text != "second user" || got[1].Text != "third part" {
		t.Fatalf("unexpected texts: %#v", got)
	}
	if got[0].MessageIndex != 2 {
		t.Fatalf("expected MessageIndex=2 for last user msg, got %d", got[0].MessageIndex)
	}
}

func TestExtractKeywordFilterLastUserSegments_Anthropic(t *testing.T) {
	body := []byte(`{
		"messages": [
			{"role":"user","content":"first user"},
			{"role":"assistant","content":"assistant text"},
			{"role":"user","content":"last user"}
		]
	}`)
	got := ExtractKeywordFilterLastUserSegments(ContentModerationProtocolAnthropicMessages, body)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1: %#v", len(got), got)
	}
	if got[0].Text != "last user" {
		t.Fatalf("unexpected text: %s", got[0].Text)
	}
}

func TestKeywordFilterConfig_InvalidFilterModeRejected(t *testing.T) {
	svc := NewKeywordFilterService(nil, nil, nil)
	cfg := defaultKeywordFilterConfig()
	cfg.FilterMode = "invalid"
	err := svc.validateConfig(context.Background(), cfg)
	if err == nil {
		t.Fatalf("expected error for invalid filter_mode")
	}
	if !strings.Contains(err.Error(), "过滤模式") {
		t.Fatalf("unexpected error message: %v", err)
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
