package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/longbridgeapp/opencc"
)

const (
	KeywordFilterMatchTypeKeyword = "keyword"
	KeywordFilterMatchTypeRegex   = "regex"

	KeywordFilterActionBlock = "block"

	KeywordFilterMatchModeAuto        = "auto"
	KeywordFilterMatchModeContains    = "contains"
	KeywordFilterMatchModeFuzzy       = "fuzzy"
	KeywordFilterMatchModeToken       = "token"
	KeywordFilterMatchModeExactPhrase = "exact_phrase"
	KeywordFilterMatchModeCJKToken    = "cjk_token"

	defaultKeywordFilterBlockStatus      = http.StatusForbidden
	defaultKeywordFilterBlockMessage     = "输入内容命中关键词过滤规则，请调整后重试"
	defaultKeywordFilterHitRetentionDays = 180
	maxKeywordFilterRetentionDays        = 3650
	maxKeywordFilterExcerptRunes         = 240
	maxKeywordFilterPatternRunes         = 256
	maxKeywordFilterRules                = 1000
	keywordFilterCleanupInterval         = 24 * time.Hour
	keywordFilterCleanupDelay            = 7 * time.Minute
	keywordFilterCleanupTimeout          = 30 * time.Minute
)

type KeywordFilterConfig struct {
	Enabled          bool                         `json:"enabled"`
	AllGroups        bool                         `json:"all_groups"`
	GroupIDs         []int64                      `json:"group_ids"`
	Keywords         []string                     `json:"keywords"`
	Whitelist        []string                     `json:"whitelist"`
	KeywordRules     []KeywordFilterRule          `json:"keyword_rules"`
	WhitelistRules   []KeywordFilterWhitelistRule `json:"whitelist_rules"`
	RegexRules       []KeywordFilterRegexRule     `json:"regex_rules"`
	BlockStatus      int                          `json:"block_status"`
	BlockMessage     string                       `json:"block_message"`
	HitRetentionDays int                          `json:"hit_retention_days"`
}

type KeywordFilterConfigView = KeywordFilterConfig

type KeywordFilterRegexRule struct {
	Name    string `json:"name"`
	Pattern string `json:"pattern"`
	Enabled bool   `json:"enabled"`
	Builtin bool   `json:"builtin,omitempty"`
}

type KeywordFilterRule struct {
	ID        string `json:"id"`
	Pattern   string `json:"pattern"`
	MatchMode string `json:"match_mode"`
	Enabled   bool   `json:"enabled"`
	Action    string `json:"action"`
}

type KeywordFilterWhitelistRule struct {
	ID            string   `json:"id"`
	Pattern       string   `json:"pattern"`
	MatchMode     string   `json:"match_mode"`
	TargetRuleIDs []string `json:"target_rule_ids"`
	Enabled       bool     `json:"enabled"`
}

type UpdateKeywordFilterConfigInput struct {
	Enabled          *bool                         `json:"enabled"`
	AllGroups        *bool                         `json:"all_groups"`
	GroupIDs         *[]int64                      `json:"group_ids"`
	Keywords         *[]string                     `json:"keywords"`
	Whitelist        *[]string                     `json:"whitelist"`
	KeywordRules     *[]KeywordFilterRule          `json:"keyword_rules"`
	WhitelistRules   *[]KeywordFilterWhitelistRule `json:"whitelist_rules"`
	RegexRules       *[]KeywordFilterRegexRule     `json:"regex_rules"`
	BlockStatus      *int                          `json:"block_status"`
	BlockMessage     *string                       `json:"block_message"`
	HitRetentionDays *int                          `json:"hit_retention_days"`
}

type KeywordFilterCheckInput struct {
	RequestID  string
	UserID     int64
	UserEmail  string
	APIKeyID   int64
	APIKeyName string
	GroupID    *int64
	GroupName  string
	Endpoint   string
	Provider   string
	Model      string
	Protocol   string
	Body       []byte
}

type KeywordFilterDecision struct {
	Allowed     bool   `json:"allowed"`
	Blocked     bool   `json:"blocked"`
	Message     string `json:"message"`
	StatusCode  int    `json:"status_code"`
	MatchType   string `json:"match_type"`
	RuleName    string `json:"rule_name"`
	MatchedText string `json:"matched_text"`
	InputHash   string `json:"input_hash,omitempty"`
	Action      string `json:"action"`
}

type KeywordFilterLog struct {
	ID           int64     `json:"id"`
	RequestID    string    `json:"request_id"`
	UserID       *int64    `json:"user_id,omitempty"`
	UserEmail    string    `json:"user_email"`
	APIKeyID     *int64    `json:"api_key_id,omitempty"`
	APIKeyName   string    `json:"api_key_name"`
	GroupID      *int64    `json:"group_id,omitempty"`
	GroupName    string    `json:"group_name"`
	Endpoint     string    `json:"endpoint"`
	Provider     string    `json:"provider"`
	Model        string    `json:"model"`
	Protocol     string    `json:"protocol"`
	MatchType    string    `json:"match_type"`
	RuleName     string    `json:"rule_name"`
	MatchedText  string    `json:"matched_text"`
	InputExcerpt string    `json:"input_excerpt"`
	InputHash    string    `json:"input_hash"`
	Action       string    `json:"action"`
	BlockStatus  int       `json:"block_status"`
	UserStatus   string    `json:"user_status"`
	CreatedAt    time.Time `json:"created_at"`
}

type KeywordFilterLogFilter struct {
	Pagination pagination.PaginationParams
	MatchType  string
	GroupID    *int64
	Endpoint   string
	Search     string
	From       *time.Time
	To         *time.Time
}

type KeywordFilterLogsResponse struct {
	Items []KeywordFilterLog `json:"items"`
}

type KeywordFilterTestInput struct {
	Text   string
	Config *UpdateKeywordFilterConfigInput
}

type KeywordFilterTestResult struct {
	Blocked           bool     `json:"blocked"`
	Whitelisted       bool     `json:"whitelisted"`
	MatchType         string   `json:"match_type"`
	RuleID            string   `json:"rule_id"`
	RuleName          string   `json:"rule_name"`
	MatchedText       string   `json:"matched_text"`
	MatchMode         string   `json:"match_mode"`
	ResolvedMatchMode string   `json:"resolved_match_mode"`
	SegmentIndex      int      `json:"segment_index"`
	MessageIndex      int      `json:"message_index"`
	PartIndex         int      `json:"part_index"`
	SegmentText       string   `json:"segment_text"`
	NormalizedText    string   `json:"normalized_text"`
	RegexText         string   `json:"regex_text"`
	Segments          []string `json:"segments"`
}

type KeywordFilterCleanupResult struct {
	Deleted    int64     `json:"deleted"`
	FinishedAt time.Time `json:"finished_at"`
}

type KeywordFilterRepository interface {
	CreateLog(ctx context.Context, log *KeywordFilterLog) error
	ListLogs(ctx context.Context, filter KeywordFilterLogFilter) ([]KeywordFilterLog, *pagination.PaginationResult, error)
	CleanupExpiredLogs(ctx context.Context, before time.Time) (*KeywordFilterCleanupResult, error)
}

type KeywordFilterService struct {
	settingRepo       SettingRepository
	repo              KeywordFilterRepository
	groupRepo         GroupRepository
	converter         *opencc.OpenCC
	lastCleanupUnix   atomic.Int64
	lastCleanupDelete atomic.Int64
}

func NewKeywordFilterService(settingRepo SettingRepository, repo KeywordFilterRepository, groupRepo GroupRepository) *KeywordFilterService {
	var converter *opencc.OpenCC
	if c, err := opencc.New("s2t"); err == nil {
		converter = c
	} else {
		slog.Warn("keyword_filter.opencc_init_failed", "error", err)
	}
	svc := &KeywordFilterService{
		settingRepo: settingRepo,
		repo:        repo,
		groupRepo:   groupRepo,
		converter:   converter,
	}
	if settingRepo != nil && repo != nil {
		go svc.cleanupWorker()
	}
	return svc
}

func (s *KeywordFilterService) GetConfig(ctx context.Context) (*KeywordFilterConfigView, error) {
	if s == nil || s.settingRepo == nil {
		cfg := defaultKeywordFilterConfig()
		cfg.normalize()
		view := *cfg
		return &view, nil
	}
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return nil, err
	}
	view := *cfg
	return &view, nil
}

func (s *KeywordFilterService) UpdateConfig(ctx context.Context, input UpdateKeywordFilterConfigInput) (*KeywordFilterConfigView, error) {
	if s == nil || s.settingRepo == nil {
		return nil, infraerrors.InternalServer("KEYWORD_FILTER_UNAVAILABLE", "keyword filter service unavailable")
	}
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return nil, err
	}
	applyKeywordFilterConfigPatch(cfg, input)
	if err := s.validateConfig(ctx, cfg); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal keyword filter config: %w", err)
	}
	if err := s.settingRepo.Set(ctx, SettingKeyKeywordFilterConfig, string(raw)); err != nil {
		return nil, fmt.Errorf("save keyword filter config: %w", err)
	}
	view := *cfg
	return &view, nil
}

func (s *KeywordFilterService) Check(ctx context.Context, input KeywordFilterCheckInput) (*KeywordFilterDecision, error) {
	allow := &KeywordFilterDecision{Allowed: true}
	if s == nil || s.settingRepo == nil || s.repo == nil {
		return allow, nil
	}
	if !s.isKeywordFilterEnabled(ctx) {
		return allow, nil
	}
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		slog.Warn("keyword_filter.config_load_failed", "error", err)
		return allow, nil
	}
	if !cfg.Enabled || !cfg.includesGroup(input.GroupID) {
		return allow, nil
	}
	segments := ExtractKeywordFilterSegments(input.Protocol, input.Body)
	if len(segments) == 0 {
		return allow, nil
	}
	normalizedSegments := s.normalizeSegments(segments)
	match := s.matchSegments(cfg, normalizedSegments)
	if match == nil {
		return allow, nil
	}
	joinedRaw := joinKeywordFilterSegmentTexts(segments)
	joinedNormalized := joinKeywordFilterNormalizedTexts(normalizedSegments)
	inputHash := keywordFilterInputHash(joinedNormalized)
	if inputHash == "" {
		inputHash = keywordFilterInputHash(joinedRaw)
	}
	log := s.buildLog(input, cfg, match, joinedRaw, inputHash)
	if err := s.repo.CreateLog(ctx, log); err != nil {
		slog.Warn("keyword_filter.create_log_failed", "error", err)
	}
	return &KeywordFilterDecision{
		Allowed:     false,
		Blocked:     true,
		Message:     cfg.BlockMessage,
		StatusCode:  cfg.BlockStatus,
		MatchType:   match.MatchType,
		RuleName:    match.RuleName,
		MatchedText: match.DisplayText,
		InputHash:   inputHash,
		Action:      KeywordFilterActionBlock,
	}, nil
}

func (s *KeywordFilterService) Test(ctx context.Context, input KeywordFilterTestInput) (*KeywordFilterTestResult, error) {
	cfg := defaultKeywordFilterConfig()
	if s != nil && s.settingRepo != nil {
		loaded, err := s.loadConfig(ctx)
		if err != nil {
			return nil, err
		}
		cfg = loaded
	}
	if input.Config != nil && keywordFilterConfigPatchProvided(*input.Config) {
		applyKeywordFilterConfigPatch(cfg, *input.Config)
		cfg.normalize()
	}
	segments := []KeywordFilterTextSegment{{Text: input.Text, SegmentIndex: 0, MessageIndex: -1, PartIndex: -1}}
	normalizedSegments := s.normalizeSegments(segments)
	match := s.matchSegments(cfg, normalizedSegments)
	normalized := normalizedKeywordText{}
	if len(normalizedSegments) > 0 {
		normalized = normalizedSegments[0].Normalized
	}
	result := &KeywordFilterTestResult{
		NormalizedText: normalized.Text,
		RegexText:      normalized.RegexText,
		Segments:       keywordFilterTestSegmentTexts(normalizedSegments),
	}
	if match != nil {
		result.Blocked = true
		result.Whitelisted = match.Whitelisted
		result.MatchType = match.MatchType
		result.RuleID = match.RuleID
		result.RuleName = match.RuleName
		result.MatchedText = match.DisplayText
		result.MatchMode = match.MatchMode
		result.ResolvedMatchMode = match.ResolvedMatchMode
		result.SegmentIndex = match.SegmentIndex
		result.MessageIndex = match.MessageIndex
		result.PartIndex = match.PartIndex
		result.SegmentText = match.SegmentText
	}
	return result, nil
}

func (s *KeywordFilterService) ListLogs(ctx context.Context, filter KeywordFilterLogFilter) ([]KeywordFilterLog, *pagination.PaginationResult, error) {
	if s == nil || s.repo == nil {
		return nil, nil, infraerrors.InternalServer("KEYWORD_FILTER_UNAVAILABLE", "keyword filter service unavailable")
	}
	if filter.Pagination.Page <= 0 {
		filter.Pagination.Page = 1
	}
	if filter.Pagination.PageSize <= 0 {
		filter.Pagination.PageSize = 20
	}
	if filter.Pagination.PageSize > 100 {
		filter.Pagination.PageSize = 100
	}
	if filter.Pagination.SortOrder == "" {
		filter.Pagination.SortOrder = pagination.SortOrderDesc
	}
	return s.repo.ListLogs(ctx, filter)
}

func (s *KeywordFilterService) loadConfig(ctx context.Context) (*KeywordFilterConfig, error) {
	cfg := defaultKeywordFilterConfig()
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyKeywordFilterConfig)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			cfg.normalize()
			return cfg, nil
		}
		return nil, fmt.Errorf("get keyword filter config: %w", err)
	}
	if strings.TrimSpace(raw) == "" {
		cfg.normalize()
		return cfg, nil
	}
	if err := json.Unmarshal([]byte(raw), cfg); err != nil {
		return nil, infraerrors.BadRequest("INVALID_KEYWORD_FILTER_CONFIG", "关键词过滤配置不是有效 JSON")
	}
	cfg.normalize()
	return cfg, nil
}

func (s *KeywordFilterService) isKeywordFilterEnabled(ctx context.Context) bool {
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyKeywordFilterEnabled)
	if err != nil {
		return false
	}
	return raw == "true"
}

func (s *KeywordFilterService) validateConfig(ctx context.Context, cfg *KeywordFilterConfig) error {
	if cfg == nil {
		return infraerrors.BadRequest("INVALID_KEYWORD_FILTER_CONFIG", "关键词过滤配置不能为空")
	}
	cfg.normalize()
	if err := validateKeywordFilterRuleModes(cfg); err != nil {
		return err
	}
	if cfg.BlockStatus < 400 || cfg.BlockStatus > 599 {
		return infraerrors.BadRequest("INVALID_KEYWORD_FILTER_BLOCK_STATUS", "拦截 HTTP 状态码必须在 400-599 之间")
	}
	if len(cfg.KeywordRules) > maxKeywordFilterRules || len(cfg.WhitelistRules) > maxKeywordFilterRules || len(cfg.RegexRules) > maxKeywordFilterRules {
		return infraerrors.BadRequest("KEYWORD_FILTER_RULE_LIMIT_EXCEEDED", "关键词过滤规则数量超过限制")
	}
	for _, rule := range cfg.KeywordRules {
		if utf8.RuneCountInString(rule.Pattern) > maxKeywordFilterPatternRunes {
			return infraerrors.BadRequest("KEYWORD_FILTER_KEYWORD_TOO_LONG", "关键词过长")
		}
	}
	for _, rule := range cfg.WhitelistRules {
		if utf8.RuneCountInString(rule.Pattern) > maxKeywordFilterPatternRunes {
			return infraerrors.BadRequest("KEYWORD_FILTER_WHITELIST_TOO_LONG", "白名单词过长")
		}
	}
	for _, rule := range cfg.RegexRules {
		if strings.TrimSpace(rule.Name) == "" {
			return infraerrors.BadRequest("KEYWORD_FILTER_REGEX_NAME_REQUIRED", "正则规则名称不能为空")
		}
		if strings.TrimSpace(rule.Pattern) == "" {
			return infraerrors.BadRequest("KEYWORD_FILTER_REGEX_PATTERN_REQUIRED", "正则表达式不能为空")
		}
		if _, err := regexp.Compile(rule.Pattern); err != nil {
			return infraerrors.BadRequest("INVALID_KEYWORD_FILTER_REGEX", fmt.Sprintf("Invalid regex rule: %s", rule.Name))
		}
	}
	if !cfg.AllGroups && len(cfg.GroupIDs) > 0 && s.groupRepo != nil {
		for _, groupID := range cfg.GroupIDs {
			if _, err := s.groupRepo.GetByIDLite(ctx, groupID); err != nil {
				return infraerrors.BadRequest("INVALID_KEYWORD_FILTER_GROUP", fmt.Sprintf("Keyword filter group does not exist: %d", groupID))
			}
		}
	}
	return nil
}

func validateKeywordFilterRuleModes(cfg *KeywordFilterConfig) error {
	if cfg == nil {
		return nil
	}
	for _, rule := range cfg.KeywordRules {
		if !isValidKeywordFilterMatchMode(rule.MatchMode) {
			return infraerrors.BadRequest("INVALID_KEYWORD_FILTER_MATCH_MODE", fmt.Sprintf("Invalid keyword match mode: %s", rule.MatchMode))
		}
	}
	for _, rule := range cfg.WhitelistRules {
		if !isValidKeywordFilterMatchMode(rule.MatchMode) {
			return infraerrors.BadRequest("INVALID_KEYWORD_FILTER_MATCH_MODE", fmt.Sprintf("Invalid whitelist match mode: %s", rule.MatchMode))
		}
	}
	return nil
}

type keywordFilterNormalizedSegment struct {
	Segment    KeywordFilterTextSegment
	Normalized normalizedKeywordText
}

func (s *KeywordFilterService) normalizeSegments(segments []KeywordFilterTextSegment) []keywordFilterNormalizedSegment {
	out := make([]keywordFilterNormalizedSegment, 0, len(segments))
	for _, segment := range segments {
		normalized := s.normalizeText(segment.Text)
		if strings.TrimSpace(normalized.Text) == "" && strings.TrimSpace(normalized.RegexText) == "" {
			continue
		}
		out = append(out, keywordFilterNormalizedSegment{Segment: segment, Normalized: normalized})
	}
	return out
}

func (s *KeywordFilterService) match(cfg *KeywordFilterConfig, input normalizedKeywordText) *keywordFilterMatch {
	if cfg == nil {
		return nil
	}
	segments := []keywordFilterNormalizedSegment{{
		Segment:    KeywordFilterTextSegment{Text: input.Original, SegmentIndex: 0, MessageIndex: -1, PartIndex: -1},
		Normalized: input,
	}}
	return s.matchSegments(cfg, segments)
}

func (s *KeywordFilterService) matchSegments(cfg *KeywordFilterConfig, segments []keywordFilterNormalizedSegment) *keywordFilterMatch {
	if cfg == nil || len(segments) == 0 {
		return nil
	}
	cfg.normalize()
	keywordRules := cfg.effectiveKeywordRules()
	whitelistRules := cfg.effectiveWhitelistRules()
	matcher, rulePatterns := s.buildKeywordRuleMatcher(keywordRules)
	for _, segment := range segments {
		input := segment.Normalized
		if strings.TrimSpace(input.Text) != "" && matcher != nil {
			for _, found := range matcher.FindAll(input.Text) {
				for _, rule := range rulePatterns[found.Pattern] {
					resolvedMode := resolveKeywordFilterMatchMode(rule.Pattern, rule.MatchMode)
					matchRange, ok := s.validateKeywordRuleMatch(rule, resolvedMode, input, found.Start, found.End)
					if !ok {
						continue
					}
					display := input.originalForNormalizedRange(matchRange.Start, matchRange.End)
					match := &keywordFilterMatch{
						MatchType:         KeywordFilterMatchTypeKeyword,
						RuleID:            rule.ID,
						RuleName:          keywordFilterRuleDisplayName(rule, resolvedMode),
						MatchedText:       rule.Pattern,
						DisplayText:       sanitizeKeywordFilterMatchedText(display),
						MatchMode:         normalizeKeywordFilterMatchMode(rule.MatchMode),
						ResolvedMatchMode: resolvedMode,
						SegmentIndex:      segment.Segment.SegmentIndex,
						MessageIndex:      segment.Segment.MessageIndex,
						PartIndex:         segment.Segment.PartIndex,
						SegmentText:       trimRunes(segment.Segment.Text, maxKeywordFilterExcerptRunes),
					}
					if s.keywordMatchCoveredByWhitelist(match, matchRange, input, whitelistRules) {
						match.Whitelisted = true
						continue
					}
					return match
				}
			}
		}
		if match := s.matchRegexRules(cfg.RegexRules, input, segment.Segment); match != nil {
			return match
		}
	}
	return nil
}

func (s *KeywordFilterService) matchRegexRules(rules []KeywordFilterRegexRule, input normalizedKeywordText, segment KeywordFilterTextSegment) *keywordFilterMatch {
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		re, err := regexp.Compile(rule.Pattern)
		if err != nil {
			continue
		}
		loc := re.FindStringIndex(input.RegexText)
		if len(loc) == 2 {
			display := input.originalForRegexRange(loc[0], loc[1])
			return &keywordFilterMatch{
				MatchType:         KeywordFilterMatchTypeRegex,
				RuleName:          rule.Name,
				MatchedText:       rule.Name,
				DisplayText:       sanitizeKeywordFilterMatchedText(display),
				SegmentIndex:      segment.SegmentIndex,
				MessageIndex:      segment.MessageIndex,
				PartIndex:         segment.PartIndex,
				SegmentText:       trimRunes(segment.Text, maxKeywordFilterExcerptRunes),
				MatchMode:         KeywordFilterMatchTypeRegex,
				ResolvedMatchMode: KeywordFilterMatchTypeRegex,
			}
		}
	}
	return nil
}

func (s *KeywordFilterService) normalizedPatterns(patterns []string) []string {
	out := make([]string, 0, len(patterns))
	seen := make(map[string]struct{}, len(patterns))
	for _, pattern := range patterns {
		normalized := s.normalizeText(pattern).Text
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func (s *KeywordFilterService) normalizedWhitelistRanges(patterns []string, text string) []keywordFilterRange {
	var ranges []keywordFilterRange
	for _, pattern := range s.normalizedPatterns(patterns) {
		start := 0
		for {
			idx := strings.Index(text[start:], pattern)
			if idx < 0 {
				break
			}
			begin := start + idx
			end := begin + len(pattern)
			ranges = append(ranges, keywordFilterRange{Start: begin, End: end})
			start = end
			if start >= len(text) {
				break
			}
		}
	}
	sort.Slice(ranges, func(i, j int) bool {
		if ranges[i].Start == ranges[j].Start {
			return ranges[i].End < ranges[j].End
		}
		return ranges[i].Start < ranges[j].Start
	})
	return ranges
}

func (s *KeywordFilterService) buildKeywordRuleMatcher(rules []KeywordFilterRule) (*keywordFilterMatcher, map[string][]KeywordFilterRule) {
	patternMap := make(map[string][]KeywordFilterRule)
	patterns := make([]string, 0, len(rules))
	for _, rule := range rules {
		if !rule.Enabled || strings.TrimSpace(rule.Pattern) == "" {
			continue
		}
		normalized := s.normalizeText(rule.Pattern).Text
		if normalized == "" {
			continue
		}
		if _, ok := patternMap[normalized]; !ok {
			patterns = append(patterns, normalized)
		}
		patternMap[normalized] = append(patternMap[normalized], rule)
	}
	if len(patterns) == 0 {
		return nil, patternMap
	}
	return newKeywordFilterMatcher(patterns), patternMap
}

func (s *KeywordFilterService) validateKeywordRuleMatch(rule KeywordFilterRule, resolvedMode string, input normalizedKeywordText, start int, end int) (keywordFilterRange, bool) {
	pattern := s.normalizeText(rule.Pattern).Text
	if pattern == "" || start < 0 || end <= start {
		return keywordFilterRange{}, false
	}
	matchRange := keywordFilterRange{Start: start, End: end}
	if resolvedMode != KeywordFilterMatchModeContains && keywordFilterMatchCrossesHardPunctuation(input, matchRange) {
		return keywordFilterRange{}, false
	}
	switch resolvedMode {
	case KeywordFilterMatchModeContains, KeywordFilterMatchModeFuzzy:
		return matchRange, true
	case KeywordFilterMatchModeToken:
		if keywordFilterHasLatinDigitBoundary(input, start, end) {
			return matchRange, true
		}
	case KeywordFilterMatchModeExactPhrase:
		if keywordFilterPhraseAllowed(rule.Pattern, input, matchRange) {
			return matchRange, true
		}
	case KeywordFilterMatchModeCJKToken:
		if keywordFilterCJKTokenAllowed(rule.Pattern, input, matchRange) {
			return matchRange, true
		}
	default:
		return matchRange, true
	}
	return keywordFilterRange{}, false
}

func keywordFilterMatchCrossesHardPunctuation(input normalizedKeywordText, matchRange keywordFilterRange) bool {
	return keywordFilterContainsHardPunctuation(input.originalForNormalizedRange(matchRange.Start, matchRange.End))
}

func (s *KeywordFilterService) keywordMatchCoveredByWhitelist(match *keywordFilterMatch, matchRange keywordFilterRange, input normalizedKeywordText, whitelistRules []KeywordFilterWhitelistRule) bool {
	for _, rule := range whitelistRules {
		if !rule.Enabled || strings.TrimSpace(rule.Pattern) == "" {
			continue
		}
		if len(rule.TargetRuleIDs) > 0 && !keywordFilterStringInSlice(match.RuleID, rule.TargetRuleIDs) {
			continue
		}
		resolvedMode := resolveKeywordFilterMatchMode(rule.Pattern, rule.MatchMode)
		pattern := s.normalizeText(rule.Pattern).Text
		if pattern == "" {
			continue
		}
		start := 0
		for {
			idx := strings.Index(input.Text[start:], pattern)
			if idx < 0 {
				break
			}
			begin := start + idx
			end := begin + len(pattern)
			whitelistRange, ok := s.validateWhitelistRuleMatch(rule, resolvedMode, input, begin, end)
			if ok && whitelistRange.Start <= matchRange.Start && whitelistRange.End >= matchRange.End {
				return true
			}
			start = end
			if start >= len(input.Text) {
				break
			}
		}
	}
	return false
}

func (s *KeywordFilterService) validateWhitelistRuleMatch(rule KeywordFilterWhitelistRule, resolvedMode string, input normalizedKeywordText, start int, end int) (keywordFilterRange, bool) {
	filterRule := KeywordFilterRule{
		ID:        rule.ID,
		Pattern:   rule.Pattern,
		MatchMode: rule.MatchMode,
		Enabled:   rule.Enabled,
		Action:    KeywordFilterActionBlock,
	}
	return s.validateKeywordRuleMatch(filterRule, resolvedMode, input, start, end)
}

func InferKeywordFilterMatchMode(pattern string) string {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return KeywordFilterMatchModeContains
	}
	if keywordFilterHasObviousSeparator(pattern) {
		return KeywordFilterMatchModeExactPhrase
	}
	var hanCount int
	var latinDigitCount int
	var otherCount int
	for _, r := range pattern {
		folded := foldKeywordFilterWidthRune(r)
		if folded == 0 {
			folded = r
		}
		switch {
		case keywordFilterIsHan(folded):
			hanCount++
		case keywordFilterIsLatinOrDigit(folded):
			latinDigitCount++
		case unicode.IsSpace(folded) || keywordFilterIsWeakPunctuation(folded):
			otherCount++
		default:
			otherCount++
		}
	}
	switch {
	case hanCount > 0 && latinDigitCount > 0:
		return KeywordFilterMatchModeExactPhrase
	case hanCount > 0 && latinDigitCount == 0 && otherCount == 0:
		switch {
		case hanCount <= 2:
			return KeywordFilterMatchModeCJKToken
		case hanCount <= 4:
			return KeywordFilterMatchModeExactPhrase
		default:
			return KeywordFilterMatchModeContains
		}
	case latinDigitCount > 0 && hanCount == 0:
		return KeywordFilterMatchModeToken
	default:
		return KeywordFilterMatchModeContains
	}
}

func resolveKeywordFilterMatchMode(pattern string, mode string) string {
	mode = normalizeKeywordFilterMatchMode(mode)
	if mode == KeywordFilterMatchModeAuto {
		return InferKeywordFilterMatchMode(pattern)
	}
	return mode
}

func keywordFilterHasObviousSeparator(pattern string) bool {
	for _, r := range pattern {
		folded := foldKeywordFilterWidthRune(r)
		if unicode.IsSpace(folded) || keywordFilterIsHardPunctuation(folded) || keywordFilterIsWeakPunctuation(folded) {
			return true
		}
	}
	return false
}

func keywordFilterHasLatinDigitBoundary(input normalizedKeywordText, start int, end int) bool {
	origStart, origEnd := input.originalBoundsForNormalizedRange(start, end)
	if origStart < 0 || origEnd <= origStart {
		return false
	}
	leftOK := true
	rightOK := true
	if r, ok := keywordFilterPreviousRune(input.Original, origStart); ok {
		leftOK = !keywordFilterIsLatinOrDigit(foldKeywordFilterWidthRune(r))
	}
	if r, ok := keywordFilterNextRune(input.Original, origEnd); ok {
		rightOK = !keywordFilterIsLatinOrDigit(foldKeywordFilterWidthRune(r))
	}
	return leftOK && rightOK
}

func keywordFilterPhraseAllowed(pattern string, input normalizedKeywordText, matchRange keywordFilterRange) bool {
	if keywordFilterContainsHardPunctuation(input.originalForNormalizedRange(matchRange.Start, matchRange.End)) {
		return false
	}
	if keywordFilterPatternHasHan(pattern) && keywordFilterPatternHasLatinOrDigit(pattern) {
		if !keywordFilterMixedPhraseBoundaryAllowed(pattern, input, matchRange) {
			return false
		}
	} else if keywordFilterPatternHasLatinOrDigit(pattern) && !keywordFilterHasLatinDigitBoundary(input, matchRange.Start, matchRange.End) {
		return false
	}
	if keywordFilterPatternHasHan(pattern) {
		return keywordFilterCJKPhraseBoundaryAllowed(pattern, input, matchRange)
	}
	return true
}

func keywordFilterMixedPhraseBoundaryAllowed(pattern string, input normalizedKeywordText, matchRange keywordFilterRange) bool {
	origStart, origEnd := input.originalBoundsForNormalizedRange(matchRange.Start, matchRange.End)
	if origStart < 0 || origEnd <= origStart {
		return false
	}
	if r, ok := keywordFilterPreviousRune(input.Original, origStart); ok && keywordFilterIsLatinOrDigit(foldKeywordFilterWidthRune(r)) {
		return false
	}
	if r, ok := keywordFilterNextRune(input.Original, origEnd); ok && keywordFilterIsLatinOrDigit(foldKeywordFilterWidthRune(r)) {
		return false
	}
	return true
}

func keywordFilterCJKTokenAllowed(pattern string, input normalizedKeywordText, matchRange keywordFilterRange) bool {
	if !keywordFilterPatternHasHan(pattern) {
		return keywordFilterPhraseAllowed(pattern, input, matchRange)
	}
	if keywordFilterPatternHasLatinOrDigit(pattern) {
		return keywordFilterMixedPhraseBoundaryAllowed(pattern, input, matchRange)
	}
	if keywordFilterCJKForbiddenContext(input, matchRange) {
		return false
	}
	return true
}

func keywordFilterCJKPhraseBoundaryAllowed(pattern string, input normalizedKeywordText, matchRange keywordFilterRange) bool {
	runeCount := keywordFilterHanRuneCount(pattern)
	if runeCount <= 2 {
		return !keywordFilterCJKForbiddenContext(input, matchRange)
	}
	if keywordFilterPatternHasLatinOrDigit(pattern) {
		return true
	}
	return true
}

func keywordFilterCJKForbiddenContext(input normalizedKeywordText, matchRange keywordFilterRange) bool {
	leftHan := false
	rightHan := false
	origStart, origEnd := input.originalBoundsForNormalizedRange(matchRange.Start, matchRange.End)
	if origStart < 0 || origEnd <= origStart {
		return true
	}
	if r, ok := keywordFilterPreviousRune(input.Original, origStart); ok {
		leftHan = keywordFilterIsHan(foldKeywordFilterWidthRune(r))
	}
	if r, ok := keywordFilterNextRune(input.Original, origEnd); ok {
		rightHan = keywordFilterIsHan(foldKeywordFilterWidthRune(r))
	}
	return leftHan && rightHan
}

func normalizedKeywordPhrase(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var builder strings.Builder
	previousWasSpace := false
	for _, r := range value {
		folded := foldKeywordFilterWidthRune(r)
		if folded == 0 {
			folded = r
		}
		if unicode.IsSpace(folded) || keywordFilterIsWeakPunctuation(folded) {
			if builder.Len() > 0 && !previousWasSpace {
				builder.WriteByte(' ')
				previousWasSpace = true
			}
			continue
		}
		if keywordFilterIsHardPunctuation(folded) {
			if builder.Len() > 0 && !previousWasSpace {
				builder.WriteByte(' ')
				previousWasSpace = true
			}
			continue
		}
		for _, out := range strings.ToLower(string(folded)) {
			builder.WriteRune(out)
		}
		previousWasSpace = false
	}
	return strings.TrimSpace(builder.String())
}

func keywordFilterContainsHardPunctuation(value string) bool {
	for _, r := range value {
		if keywordFilterIsHardPunctuation(foldKeywordFilterWidthRune(r)) {
			return true
		}
	}
	return false
}

func keywordFilterSpanIndexBefore(spans []keywordFilterSpan, offset int) int {
	for i := len(spans) - 1; i >= 0; i-- {
		if spans[i].End <= offset {
			return i
		}
	}
	return -1
}

func keywordFilterSpanIndexAtOrAfter(spans []keywordFilterSpan, offset int) int {
	for i, span := range spans {
		if span.Start >= offset {
			return i
		}
	}
	return -1
}

func keywordFilterPreviousRune(text string, offset int) (rune, bool) {
	if offset <= 0 || offset > len(text) {
		return 0, false
	}
	r, _ := utf8.DecodeLastRuneInString(text[:offset])
	return r, r != utf8.RuneError
}

func keywordFilterNextRune(text string, offset int) (rune, bool) {
	if offset < 0 || offset >= len(text) {
		return 0, false
	}
	r, _ := utf8.DecodeRuneInString(text[offset:])
	return r, r != utf8.RuneError
}

func keywordFilterNormalizedRuneClass(r rune) keywordFilterRuneClass {
	switch {
	case keywordFilterIsHan(r):
		return keywordFilterRuneClassHan
	case keywordFilterIsLatinOrDigit(r):
		return keywordFilterRuneClassLatinDigit
	default:
		return keywordFilterRuneClassOther
	}
}

func keywordFilterIsHan(r rune) bool {
	return unicode.Is(unicode.Han, r)
}

func keywordFilterIsLatinOrDigit(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || unicode.IsDigit(r)
}

func keywordFilterIsWeakPunctuation(r rune) bool {
	switch r {
	case '_', '-', '.', ',', '\'', '"', '`', '~', '*', '+', '=', '|', '/', '\\':
		return true
	case '，', '。', '、', '·', '＇', '＂', '“', '”', '‘', '’':
		return true
	default:
		return false
	}
}

func keywordFilterIsHardPunctuation(r rune) bool {
	switch r {
	case '!', '?', ';', ':', '\n', '\r', '\t':
		return true
	case '！', '？', '；', '：':
		return true
	default:
		return false
	}
}

func keywordFilterPatternHasHan(pattern string) bool {
	for _, r := range pattern {
		if keywordFilterIsHan(foldKeywordFilterWidthRune(r)) {
			return true
		}
	}
	return false
}

func keywordFilterPatternHasLatinOrDigit(pattern string) bool {
	for _, r := range pattern {
		if keywordFilterIsLatinOrDigit(foldKeywordFilterWidthRune(r)) {
			return true
		}
	}
	return false
}

func keywordFilterHanRuneCount(pattern string) int {
	count := 0
	for _, r := range pattern {
		if keywordFilterIsHan(foldKeywordFilterWidthRune(r)) {
			count++
		}
	}
	return count
}

func keywordFilterStringInSlice(value string, values []string) bool {
	for _, item := range values {
		if strings.EqualFold(value, item) {
			return true
		}
	}
	return false
}

func keywordFilterRuleDisplayName(rule KeywordFilterRule, resolvedMode string) string {
	if rule.ID != "" {
		return fmt.Sprintf("%s/%s", rule.ID, resolvedMode)
	}
	return fmt.Sprintf("%s/%s", rule.Pattern, resolvedMode)
}

func (s *KeywordFilterService) normalizeText(text string) normalizedKeywordText {
	text = strings.TrimSpace(text)
	if text == "" {
		return normalizedKeywordText{}
	}
	if s != nil && s.converter != nil {
		if converted, err := s.converter.Convert(text); err == nil {
			text = converted
		}
	}
	var builder strings.Builder
	var regexBuilder strings.Builder
	var spans []keywordFilterSpan
	var regexSpans []keywordFilterSpan
	var classes []keywordFilterRuneClass
	for idx, r := range text {
		foldedForRegex := foldKeywordFilterWidthRune(r)
		if foldedForRegex == 0 {
			foldedForRegex = r
		}
		for _, outRune := range strings.ToLower(string(foldedForRegex)) {
			start := regexBuilder.Len()
			regexBuilder.WriteRune(outRune)
			regexSpans = append(regexSpans, keywordFilterSpan{
				Start:     start,
				End:       regexBuilder.Len(),
				OrigStart: idx,
				OrigEnd:   idx + utf8.RuneLen(r),
			})
		}
		folded := foldKeywordFilterRune(r)
		if folded == 0 {
			continue
		}
		lowered := strings.ToLower(string(folded))
		for _, outRune := range lowered {
			start := builder.Len()
			builder.WriteRune(outRune)
			spans = append(spans, keywordFilterSpan{
				Start:     start,
				End:       builder.Len(),
				OrigStart: idx,
				OrigEnd:   idx + utf8.RuneLen(r),
			})
			classes = append(classes, keywordFilterNormalizedRuneClass(outRune))
		}
	}
	return normalizedKeywordText{Text: builder.String(), RegexText: regexBuilder.String(), Original: text, Spans: spans, RegexSpans: regexSpans, Classes: classes}
}

func (s *KeywordFilterService) buildLog(input KeywordFilterCheckInput, cfg *KeywordFilterConfig, match *keywordFilterMatch, rawText string, inputHash string) *KeywordFilterLog {
	var userID *int64
	if input.UserID > 0 {
		userID = &input.UserID
	}
	var apiKeyID *int64
	if input.APIKeyID > 0 {
		apiKeyID = &input.APIKeyID
	}
	return &KeywordFilterLog{
		RequestID:    input.RequestID,
		UserID:       userID,
		UserEmail:    input.UserEmail,
		APIKeyID:     apiKeyID,
		APIKeyName:   input.APIKeyName,
		GroupID:      cloneInt64Ptr(input.GroupID),
		GroupName:    input.GroupName,
		Endpoint:     input.Endpoint,
		Provider:     input.Provider,
		Model:        input.Model,
		Protocol:     input.Protocol,
		MatchType:    match.MatchType,
		RuleName:     match.RuleName,
		MatchedText:  match.DisplayText,
		InputExcerpt: trimRunes(redactContentModerationSecrets(rawText), maxKeywordFilterExcerptRunes),
		InputHash:    inputHash,
		Action:       KeywordFilterActionBlock,
		BlockStatus:  cfg.BlockStatus,
	}
}

func (s *KeywordFilterService) cleanupWorker() {
	timer := time.NewTimer(keywordFilterCleanupDelay)
	defer timer.Stop()
	for {
		<-timer.C
		s.runCleanupOnce()
		timer.Reset(keywordFilterCleanupInterval)
	}
}

func (s *KeywordFilterService) runCleanupOnce() {
	if s == nil || s.repo == nil || s.settingRepo == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), keywordFilterCleanupTimeout)
	defer cancel()
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		slog.Warn("keyword_filter.cleanup_load_config_failed", "error", err)
		return
	}
	result, err := s.repo.CleanupExpiredLogs(ctx, time.Now().AddDate(0, 0, -cfg.HitRetentionDays))
	if err != nil {
		slog.Warn("keyword_filter.cleanup_failed", "error", err)
		return
	}
	if result != nil {
		s.lastCleanupUnix.Store(result.FinishedAt.Unix())
		s.lastCleanupDelete.Store(result.Deleted)
	}
}

func defaultKeywordFilterConfig() *KeywordFilterConfig {
	return &KeywordFilterConfig{
		Enabled:          false,
		AllGroups:        true,
		GroupIDs:         []int64{},
		Keywords:         []string{},
		Whitelist:        []string{},
		KeywordRules:     []KeywordFilterRule{},
		WhitelistRules:   []KeywordFilterWhitelistRule{},
		RegexRules:       defaultKeywordFilterRegexRules(),
		BlockStatus:      defaultKeywordFilterBlockStatus,
		BlockMessage:     defaultKeywordFilterBlockMessage,
		HitRetentionDays: defaultKeywordFilterHitRetentionDays,
	}
}

func defaultKeywordFilterRegexRules() []KeywordFilterRegexRule {
	return []KeywordFilterRegexRule{
		{Name: "phone_cn", Pattern: `(?:(?:\+?86[-\s]?)?1[3-9]\d{9})`, Enabled: false, Builtin: true},
		{Name: "url", Pattern: `(?i)\bhttps?://[^\s<>"']+`, Enabled: false, Builtin: true},
	}
}

func (cfg *KeywordFilterConfig) normalize() {
	if cfg == nil {
		return
	}
	cfg.GroupIDs = normalizeInt64IDs(cfg.GroupIDs)
	cfg.Keywords = normalizeKeywordFilterList(cfg.Keywords)
	cfg.Whitelist = normalizeKeywordFilterList(cfg.Whitelist)
	cfg.KeywordRules = normalizeKeywordFilterRules(cfg.KeywordRules)
	cfg.WhitelistRules = normalizeKeywordFilterWhitelistRules(cfg.WhitelistRules)
	if len(cfg.KeywordRules) == 0 && len(cfg.Keywords) > 0 {
		cfg.KeywordRules = keywordFilterRulesFromLegacyKeywords(cfg.Keywords)
	}
	if len(cfg.WhitelistRules) == 0 && len(cfg.Whitelist) > 0 {
		cfg.WhitelistRules = keywordFilterWhitelistRulesFromLegacy(cfg.Whitelist)
	}
	if len(cfg.Keywords) == 0 && len(cfg.KeywordRules) > 0 {
		cfg.Keywords = keywordFilterLegacyKeywordsFromRules(cfg.KeywordRules)
	}
	if len(cfg.Whitelist) == 0 && len(cfg.WhitelistRules) > 0 {
		cfg.Whitelist = keywordFilterLegacyWhitelistFromRules(cfg.WhitelistRules)
	}
	cfg.RegexRules = mergeKeywordFilterRegexRules(cfg.RegexRules)
	if cfg.BlockStatus < 400 || cfg.BlockStatus > 599 {
		cfg.BlockStatus = defaultKeywordFilterBlockStatus
	}
	if strings.TrimSpace(cfg.BlockMessage) == "" {
		cfg.BlockMessage = defaultKeywordFilterBlockMessage
	} else {
		cfg.BlockMessage = strings.TrimSpace(cfg.BlockMessage)
	}
	if cfg.HitRetentionDays <= 0 {
		cfg.HitRetentionDays = defaultKeywordFilterHitRetentionDays
	}
	if cfg.HitRetentionDays > maxKeywordFilterRetentionDays {
		cfg.HitRetentionDays = maxKeywordFilterRetentionDays
	}
}

func (cfg *KeywordFilterConfig) effectiveKeywordRules() []KeywordFilterRule {
	if cfg == nil {
		return nil
	}
	if len(cfg.KeywordRules) > 0 {
		return cfg.KeywordRules
	}
	return keywordFilterRulesFromLegacyKeywords(cfg.Keywords)
}

func (cfg *KeywordFilterConfig) effectiveWhitelistRules() []KeywordFilterWhitelistRule {
	if cfg == nil {
		return nil
	}
	if len(cfg.WhitelistRules) > 0 {
		return cfg.WhitelistRules
	}
	return keywordFilterWhitelistRulesFromLegacy(cfg.Whitelist)
}

func (cfg *KeywordFilterConfig) includesGroup(groupID *int64) bool {
	if cfg == nil {
		return false
	}
	if cfg.AllGroups {
		return true
	}
	if groupID == nil {
		return false
	}
	for _, id := range cfg.GroupIDs {
		if id == *groupID {
			return true
		}
	}
	return false
}

func normalizeKeywordFilterList(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func normalizeKeywordFilterRegexRules(rules []KeywordFilterRegexRule) []KeywordFilterRegexRule {
	out := make([]KeywordFilterRegexRule, 0, len(rules))
	seen := map[string]struct{}{}
	for _, rule := range rules {
		rule.Name = strings.TrimSpace(rule.Name)
		rule.Pattern = strings.TrimSpace(rule.Pattern)
		if rule.Name == "" && rule.Pattern == "" {
			continue
		}
		key := strings.ToLower(rule.Name)
		if key == "" {
			key = rule.Pattern
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, rule)
	}
	return out
}

func normalizeKeywordFilterRules(rules []KeywordFilterRule) []KeywordFilterRule {
	out := make([]KeywordFilterRule, 0, len(rules))
	seen := map[string]struct{}{}
	for index, rule := range rules {
		rule.Pattern = strings.TrimSpace(rule.Pattern)
		if rule.Pattern == "" {
			continue
		}
		rule.ID = sanitizeKeywordFilterRuleID(rule.ID)
		if rule.ID == "" {
			rule.ID = keywordFilterRuleID("keyword", rule.Pattern, index)
		}
		rule.MatchMode = normalizeKeywordFilterMatchMode(rule.MatchMode)
		if rule.Action == "" {
			rule.Action = KeywordFilterActionBlock
		}
		key := strings.ToLower(rule.ID)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, rule)
	}
	return out
}

func normalizeKeywordFilterWhitelistRules(rules []KeywordFilterWhitelistRule) []KeywordFilterWhitelistRule {
	out := make([]KeywordFilterWhitelistRule, 0, len(rules))
	seen := map[string]struct{}{}
	for index, rule := range rules {
		rule.Pattern = strings.TrimSpace(rule.Pattern)
		if rule.Pattern == "" {
			continue
		}
		rule.ID = sanitizeKeywordFilterRuleID(rule.ID)
		if rule.ID == "" {
			rule.ID = keywordFilterRuleID("whitelist", rule.Pattern, index)
		}
		rule.MatchMode = normalizeKeywordFilterMatchMode(rule.MatchMode)
		rule.TargetRuleIDs = normalizeKeywordFilterStringIDs(rule.TargetRuleIDs)
		rule.Enabled = rule.Enabled
		key := strings.ToLower(rule.ID)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, rule)
	}
	return out
}

func keywordFilterRulesFromLegacyKeywords(keywords []string) []KeywordFilterRule {
	keywords = normalizeKeywordFilterList(keywords)
	out := make([]KeywordFilterRule, 0, len(keywords))
	for index, keyword := range keywords {
		out = append(out, KeywordFilterRule{
			ID:        keywordFilterRuleID("legacy_keyword", keyword, index),
			Pattern:   keyword,
			MatchMode: KeywordFilterMatchModeAuto,
			Enabled:   true,
			Action:    KeywordFilterActionBlock,
		})
	}
	return out
}

func keywordFilterWhitelistRulesFromLegacy(whitelist []string) []KeywordFilterWhitelistRule {
	whitelist = normalizeKeywordFilterList(whitelist)
	out := make([]KeywordFilterWhitelistRule, 0, len(whitelist))
	for index, pattern := range whitelist {
		out = append(out, KeywordFilterWhitelistRule{
			ID:        keywordFilterRuleID("legacy_whitelist", pattern, index),
			Pattern:   pattern,
			MatchMode: KeywordFilterMatchModeAuto,
			Enabled:   true,
		})
	}
	return out
}

func keywordFilterLegacyKeywordsFromRules(rules []KeywordFilterRule) []string {
	values := make([]string, 0, len(rules))
	for _, rule := range rules {
		if strings.TrimSpace(rule.Pattern) != "" {
			values = append(values, rule.Pattern)
		}
	}
	return normalizeKeywordFilterList(values)
}

func keywordFilterLegacyWhitelistFromRules(rules []KeywordFilterWhitelistRule) []string {
	values := make([]string, 0, len(rules))
	for _, rule := range rules {
		if strings.TrimSpace(rule.Pattern) != "" {
			values = append(values, rule.Pattern)
		}
	}
	return normalizeKeywordFilterList(values)
}

func normalizeKeywordFilterMatchMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case KeywordFilterMatchModeContains,
		KeywordFilterMatchModeFuzzy,
		KeywordFilterMatchModeToken,
		KeywordFilterMatchModeExactPhrase,
		KeywordFilterMatchModeCJKToken:
		return mode
	default:
		return KeywordFilterMatchModeAuto
	}
}

func isValidKeywordFilterMatchMode(mode string) bool {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "",
		KeywordFilterMatchModeAuto,
		KeywordFilterMatchModeContains,
		KeywordFilterMatchModeFuzzy,
		KeywordFilterMatchModeToken,
		KeywordFilterMatchModeExactPhrase,
		KeywordFilterMatchModeCJKToken:
		return true
	default:
		return false
	}
}

func sanitizeKeywordFilterRuleID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var builder strings.Builder
	for _, r := range value {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			builder.WriteRune(r)
		case r == '_' || r == '-':
			builder.WriteRune(r)
		}
	}
	return trimRunes(builder.String(), 80)
}

func keywordFilterRuleID(prefix string, pattern string, index int) string {
	normalizedPrefix := sanitizeKeywordFilterRuleID(prefix)
	if normalizedPrefix == "" {
		normalizedPrefix = "rule"
	}
	sum := sha256.Sum256([]byte(pattern))
	return fmt.Sprintf("%s_%d_%s", normalizedPrefix, index, hex.EncodeToString(sum[:])[:10])
}

func normalizeKeywordFilterStringIDs(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = sanitizeKeywordFilterRuleID(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func applyKeywordFilterConfigPatch(cfg *KeywordFilterConfig, input UpdateKeywordFilterConfigInput) {
	if cfg == nil {
		return
	}
	if input.Enabled != nil {
		cfg.Enabled = *input.Enabled
	}
	if input.AllGroups != nil {
		cfg.AllGroups = *input.AllGroups
	}
	if input.GroupIDs != nil {
		cfg.GroupIDs = normalizeInt64IDs(*input.GroupIDs)
	}
	if input.Keywords != nil {
		cfg.Keywords = normalizeKeywordFilterList(*input.Keywords)
		cfg.KeywordRules = keywordFilterRulesFromLegacyKeywords(cfg.Keywords)
	}
	if input.Whitelist != nil {
		cfg.Whitelist = normalizeKeywordFilterList(*input.Whitelist)
		cfg.WhitelistRules = keywordFilterWhitelistRulesFromLegacy(cfg.Whitelist)
	}
	if input.KeywordRules != nil {
		cfg.KeywordRules = normalizeKeywordFilterRules(*input.KeywordRules)
		cfg.Keywords = keywordFilterLegacyKeywordsFromRules(cfg.KeywordRules)
	}
	if input.WhitelistRules != nil {
		cfg.WhitelistRules = normalizeKeywordFilterWhitelistRules(*input.WhitelistRules)
		cfg.Whitelist = keywordFilterLegacyWhitelistFromRules(cfg.WhitelistRules)
	}
	if input.RegexRules != nil {
		cfg.RegexRules = normalizeKeywordFilterRegexRules(*input.RegexRules)
	}
	if input.BlockStatus != nil {
		cfg.BlockStatus = *input.BlockStatus
	}
	if input.BlockMessage != nil {
		cfg.BlockMessage = strings.TrimSpace(*input.BlockMessage)
	}
	if input.HitRetentionDays != nil {
		cfg.HitRetentionDays = *input.HitRetentionDays
	}
}

func keywordFilterConfigPatchProvided(input UpdateKeywordFilterConfigInput) bool {
	return input.Enabled != nil ||
		input.AllGroups != nil ||
		input.GroupIDs != nil ||
		input.Keywords != nil ||
		input.Whitelist != nil ||
		input.KeywordRules != nil ||
		input.WhitelistRules != nil ||
		input.RegexRules != nil ||
		input.BlockStatus != nil ||
		input.BlockMessage != nil ||
		input.HitRetentionDays != nil
}

func mergeKeywordFilterRegexRules(rules []KeywordFilterRegexRule) []KeywordFilterRegexRule {
	normalized := normalizeKeywordFilterRegexRules(rules)
	byName := make(map[string]int, len(normalized))
	for i, rule := range normalized {
		byName[strings.ToLower(rule.Name)] = i
	}
	for _, builtin := range defaultKeywordFilterRegexRules() {
		key := strings.ToLower(builtin.Name)
		if idx, ok := byName[key]; ok {
			normalized[idx].Builtin = true
			if strings.TrimSpace(normalized[idx].Pattern) == "" {
				normalized[idx].Pattern = builtin.Pattern
			}
			continue
		}
		normalized = append(normalized, builtin)
	}
	return normalized
}

func foldKeywordFilterRune(r rune) rune {
	r = foldKeywordFilterWidthRune(r)
	switch {
	case r == 0:
		return 0
	case unicode.IsLetter(r) || unicode.IsDigit(r):
		return r
	default:
		return 0
	}
}

func foldKeywordFilterWidthRune(r rune) rune {
	switch {
	case r >= 0xFF21 && r <= 0xFF3A:
		return r - 0xFF21 + 'A'
	case r >= 0xFF41 && r <= 0xFF5A:
		return r - 0xFF41 + 'a'
	case r >= 0xFF10 && r <= 0xFF19:
		return r - 0xFF10 + '0'
	case r == 0x3000:
		return ' '
	default:
		return r
	}
}

func keywordFilterInputHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func sanitizeKeywordFilterMatchedText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	text = redactContentModerationSecrets(text)
	text = keywordFilterPhonePattern.ReplaceAllString(text, "[PHONE]")
	text = keywordFilterURLPattern.ReplaceAllString(text, "[URL]")
	return trimRunes(text, 80)
}

var (
	keywordFilterPhonePattern = regexp.MustCompile(`(?:(?:\+?86[-\s]?)?1[3-9]\d{9})`)
	keywordFilterURLPattern   = regexp.MustCompile(`(?i)\bhttps?://[^\s<>"']+`)
)

type keywordFilterMatch struct {
	MatchType         string
	RuleID            string
	RuleName          string
	MatchedText       string
	DisplayText       string
	MatchMode         string
	ResolvedMatchMode string
	SegmentIndex      int
	MessageIndex      int
	PartIndex         int
	SegmentText       string
	Whitelisted       bool
}

type normalizedKeywordText struct {
	Text       string
	RegexText  string
	Original   string
	Spans      []keywordFilterSpan
	RegexSpans []keywordFilterSpan
	Classes    []keywordFilterRuneClass
}

func (n normalizedKeywordText) originalForNormalizedRange(start, end int) string {
	return n.originalForRange(start, end, n.Spans)
}

func (n normalizedKeywordText) originalForRegexRange(start, end int) string {
	return n.originalForRange(start, end, n.RegexSpans)
}

func (n normalizedKeywordText) originalForRange(start, end int, spans []keywordFilterSpan) string {
	origStart, origEnd := n.originalBoundsForRange(start, end, spans)
	if origStart < 0 || origEnd <= origStart || origEnd > len(n.Original) {
		return ""
	}
	return n.Original[origStart:origEnd]
}

func (n normalizedKeywordText) originalBoundsForNormalizedRange(start, end int) (int, int) {
	return n.originalBoundsForRange(start, end, n.Spans)
}

func (n normalizedKeywordText) originalBoundsForRange(start, end int, spans []keywordFilterSpan) (int, int) {
	if start < 0 || end <= start || len(spans) == 0 {
		return -1, -1
	}
	origStart := -1
	origEnd := -1
	for _, span := range spans {
		if span.End <= start {
			continue
		}
		if span.Start >= end {
			break
		}
		if origStart < 0 || span.OrigStart < origStart {
			origStart = span.OrigStart
		}
		if span.OrigEnd > origEnd {
			origEnd = span.OrigEnd
		}
	}
	if origStart < 0 || origEnd <= origStart || origEnd > len(n.Original) {
		return -1, -1
	}
	return origStart, origEnd
}

type keywordFilterSpan struct {
	Start     int
	End       int
	OrigStart int
	OrigEnd   int
}

type keywordFilterRange struct {
	Start int
	End   int
}

type keywordFilterRuneClass int

const (
	keywordFilterRuneClassOther keywordFilterRuneClass = iota
	keywordFilterRuneClassLatinDigit
	keywordFilterRuneClassHan
)

func rangeCoveredByAny(start, end int, ranges []keywordFilterRange) bool {
	for _, item := range ranges {
		if item.Start <= start && item.End >= end {
			return true
		}
	}
	return false
}

func splitKeywordFilterSegments(text string) []string {
	if text == "" {
		return []string{}
	}
	return []string{text}
}

func joinKeywordFilterSegmentTexts(segments []KeywordFilterTextSegment) string {
	parts := make([]string, 0, len(segments))
	for _, segment := range segments {
		if strings.TrimSpace(segment.Text) != "" {
			parts = append(parts, segment.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func joinKeywordFilterNormalizedTexts(segments []keywordFilterNormalizedSegment) string {
	parts := make([]string, 0, len(segments))
	for _, segment := range segments {
		if strings.TrimSpace(segment.Normalized.Text) != "" {
			parts = append(parts, segment.Normalized.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func keywordFilterTestSegmentTexts(segments []keywordFilterNormalizedSegment) []string {
	out := make([]string, 0, len(segments))
	for _, segment := range segments {
		out = append(out, segment.Normalized.Text)
	}
	return out
}
