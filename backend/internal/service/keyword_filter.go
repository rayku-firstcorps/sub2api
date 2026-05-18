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
	Enabled          bool                     `json:"enabled"`
	AllGroups        bool                     `json:"all_groups"`
	GroupIDs         []int64                  `json:"group_ids"`
	Keywords         []string                 `json:"keywords"`
	Whitelist        []string                 `json:"whitelist"`
	RegexRules       []KeywordFilterRegexRule `json:"regex_rules"`
	BlockStatus      int                      `json:"block_status"`
	BlockMessage     string                   `json:"block_message"`
	HitRetentionDays int                      `json:"hit_retention_days"`
}

type KeywordFilterConfigView = KeywordFilterConfig

type KeywordFilterRegexRule struct {
	Name    string `json:"name"`
	Pattern string `json:"pattern"`
	Enabled bool   `json:"enabled"`
	Builtin bool   `json:"builtin,omitempty"`
}

type UpdateKeywordFilterConfigInput struct {
	Enabled          *bool                     `json:"enabled"`
	AllGroups        *bool                     `json:"all_groups"`
	GroupIDs         *[]int64                  `json:"group_ids"`
	Keywords         *[]string                 `json:"keywords"`
	Whitelist        *[]string                 `json:"whitelist"`
	RegexRules       *[]KeywordFilterRegexRule `json:"regex_rules"`
	BlockStatus      *int                      `json:"block_status"`
	BlockMessage     *string                   `json:"block_message"`
	HitRetentionDays *int                      `json:"hit_retention_days"`
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
	Blocked        bool     `json:"blocked"`
	MatchType      string   `json:"match_type"`
	RuleName       string   `json:"rule_name"`
	MatchedText    string   `json:"matched_text"`
	NormalizedText string   `json:"normalized_text"`
	RegexText      string   `json:"regex_text"`
	Segments       []string `json:"segments"`
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
	if !s.isRiskControlEnabled(ctx) {
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
	texts := ExtractKeywordFilterTexts(input.Protocol, input.Body)
	if len(texts) == 0 {
		return allow, nil
	}
	joined := strings.Join(texts, "\n")
	normalizedInput := s.normalizeText(joined)
	if strings.TrimSpace(normalizedInput.Text) == "" && strings.TrimSpace(normalizedInput.RegexText) == "" {
		return allow, nil
	}
	match := s.match(cfg, normalizedInput)
	if match == nil {
		return allow, nil
	}
	inputHash := keywordFilterInputHash(normalizedInput.Text)
	if inputHash == "" {
		inputHash = keywordFilterInputHash(normalizedInput.RegexText)
	}
	log := s.buildLog(input, cfg, match, joined, inputHash)
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
	normalized := s.normalizeText(input.Text)
	match := s.match(cfg, normalized)
	result := &KeywordFilterTestResult{
		NormalizedText: normalized.Text,
		RegexText:      normalized.RegexText,
		Segments:       splitKeywordFilterSegments(normalized.Text),
	}
	if match != nil {
		result.Blocked = true
		result.MatchType = match.MatchType
		result.RuleName = match.RuleName
		result.MatchedText = match.DisplayText
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

func (s *KeywordFilterService) isRiskControlEnabled(ctx context.Context) bool {
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyRiskControlEnabled)
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
	if cfg.BlockStatus < 400 || cfg.BlockStatus > 599 {
		return infraerrors.BadRequest("INVALID_KEYWORD_FILTER_BLOCK_STATUS", "拦截 HTTP 状态码必须在 400-599 之间")
	}
	if len(cfg.Keywords) > maxKeywordFilterRules || len(cfg.Whitelist) > maxKeywordFilterRules || len(cfg.RegexRules) > maxKeywordFilterRules {
		return infraerrors.BadRequest("KEYWORD_FILTER_RULE_LIMIT_EXCEEDED", "关键词过滤规则数量超过限制")
	}
	for _, keyword := range cfg.Keywords {
		if utf8.RuneCountInString(keyword) > maxKeywordFilterPatternRunes {
			return infraerrors.BadRequest("KEYWORD_FILTER_KEYWORD_TOO_LONG", "关键词过长")
		}
	}
	for _, keyword := range cfg.Whitelist {
		if utf8.RuneCountInString(keyword) > maxKeywordFilterPatternRunes {
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

func (s *KeywordFilterService) match(cfg *KeywordFilterConfig, input normalizedKeywordText) *keywordFilterMatch {
	if cfg == nil || (strings.TrimSpace(input.Text) == "" && strings.TrimSpace(input.RegexText) == "") {
		return nil
	}
	whitelist := s.normalizedWhitelistRanges(cfg.Whitelist, input.Text)
	if strings.TrimSpace(input.Text) != "" {
		matcher := newKeywordFilterMatcher(s.normalizedPatterns(cfg.Keywords))
		for _, found := range matcher.FindAll(input.Text) {
			if !rangeCoveredByAny(found.Start, found.End, whitelist) {
				display := input.originalForNormalizedRange(found.Start, found.End)
				return &keywordFilterMatch{
					MatchType:   KeywordFilterMatchTypeKeyword,
					RuleName:    found.Pattern,
					MatchedText: found.Pattern,
					DisplayText: sanitizeKeywordFilterMatchedText(display),
				}
			}
		}
	}
	for _, rule := range cfg.RegexRules {
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
				MatchType:   KeywordFilterMatchTypeRegex,
				RuleName:    rule.Name,
				MatchedText: rule.Name,
				DisplayText: sanitizeKeywordFilterMatchedText(display),
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
		}
	}
	return normalizedKeywordText{Text: builder.String(), RegexText: regexBuilder.String(), Original: text, Spans: spans, RegexSpans: regexSpans}
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
	}
	if input.Whitelist != nil {
		cfg.Whitelist = normalizeKeywordFilterList(*input.Whitelist)
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
	MatchType   string
	RuleName    string
	MatchedText string
	DisplayText string
}

type normalizedKeywordText struct {
	Text       string
	RegexText  string
	Original   string
	Spans      []keywordFilterSpan
	RegexSpans []keywordFilterSpan
}

func (n normalizedKeywordText) originalForNormalizedRange(start, end int) string {
	return n.originalForRange(start, end, n.Spans)
}

func (n normalizedKeywordText) originalForRegexRange(start, end int) string {
	return n.originalForRange(start, end, n.RegexSpans)
}

func (n normalizedKeywordText) originalForRange(start, end int, spans []keywordFilterSpan) string {
	if start < 0 || end <= start || len(spans) == 0 {
		return ""
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
		return ""
	}
	return n.Original[origStart:origEnd]
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
