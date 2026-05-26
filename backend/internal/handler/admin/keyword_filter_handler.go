package admin

import (
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type KeywordFilterHandler struct {
	service *service.KeywordFilterService
}

func NewKeywordFilterHandler(svc *service.KeywordFilterService) *KeywordFilterHandler {
	return &KeywordFilterHandler{service: svc}
}

func (h *KeywordFilterHandler) available(c *gin.Context) bool {
	if h == nil || h.service == nil {
		response.Error(c, 500, "keyword filter service unavailable")
		return false
	}
	return true
}

type keywordFilterConfigRequest struct {
	Enabled          *bool                                 `json:"enabled"`
	FilterMode       *string                               `json:"filter_mode"`
	AllGroups        *bool                                 `json:"all_groups"`
	GroupIDs         *[]int64                              `json:"group_ids"`
	Keywords         *[]string                             `json:"keywords"`
	Whitelist        *[]string                             `json:"whitelist"`
	KeywordRules     *[]service.KeywordFilterRule          `json:"keyword_rules"`
	WhitelistRules   *[]service.KeywordFilterWhitelistRule `json:"whitelist_rules"`
	RegexRules       *[]service.KeywordFilterRegexRule     `json:"regex_rules"`
	BlockStatus      *int                                  `json:"block_status"`
	BlockMessage     *string                               `json:"block_message"`
	HitRetentionDays *int                                  `json:"hit_retention_days"`
}

type keywordFilterTestRequest struct {
	Text             string                                `json:"text"`
	Enabled          *bool                                 `json:"enabled"`
	AllGroups        *bool                                 `json:"all_groups"`
	GroupIDs         *[]int64                              `json:"group_ids"`
	Keywords         *[]string                             `json:"keywords"`
	Whitelist        *[]string                             `json:"whitelist"`
	KeywordRules     *[]service.KeywordFilterRule          `json:"keyword_rules"`
	WhitelistRules   *[]service.KeywordFilterWhitelistRule `json:"whitelist_rules"`
	RegexRules       *[]service.KeywordFilterRegexRule     `json:"regex_rules"`
	BlockStatus      *int                                  `json:"block_status"`
	BlockMessage     *string                               `json:"block_message"`
	HitRetentionDays *int                                  `json:"hit_retention_days"`
	Config           *keywordFilterTestConfigPatch         `json:"config"`
}

type keywordFilterTestConfigPatch struct {
	Enabled          *bool                                 `json:"enabled"`
	AllGroups        *bool                                 `json:"all_groups"`
	GroupIDs         *[]int64                              `json:"group_ids"`
	Keywords         *[]string                             `json:"keywords"`
	Whitelist        *[]string                             `json:"whitelist"`
	KeywordRules     *[]service.KeywordFilterRule          `json:"keyword_rules"`
	WhitelistRules   *[]service.KeywordFilterWhitelistRule `json:"whitelist_rules"`
	RegexRules       *[]service.KeywordFilterRegexRule     `json:"regex_rules"`
	BlockStatus      *int                                  `json:"block_status"`
	BlockMessage     *string                               `json:"block_message"`
	HitRetentionDays *int                                  `json:"hit_retention_days"`
}

func (h *KeywordFilterHandler) GetConfig(c *gin.Context) {
	if !h.available(c) {
		return
	}
	cfg, err := h.service.GetConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

func (h *KeywordFilterHandler) UpdateConfig(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var req keywordFilterConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	cfg, err := h.service.UpdateConfig(c.Request.Context(), service.UpdateKeywordFilterConfigInput{
		Enabled:          req.Enabled,
		FilterMode:       req.FilterMode,
		AllGroups:        req.AllGroups,
		GroupIDs:         req.GroupIDs,
		Keywords:         req.Keywords,
		Whitelist:        req.Whitelist,
		KeywordRules:     req.KeywordRules,
		WhitelistRules:   req.WhitelistRules,
		RegexRules:       req.RegexRules,
		BlockStatus:      req.BlockStatus,
		BlockMessage:     req.BlockMessage,
		HitRetentionDays: req.HitRetentionDays,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

func (h *KeywordFilterHandler) Test(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var req keywordFilterTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	patch := service.UpdateKeywordFilterConfigInput{
		Enabled:          req.Enabled,
		AllGroups:        req.AllGroups,
		GroupIDs:         req.GroupIDs,
		Keywords:         req.Keywords,
		Whitelist:        req.Whitelist,
		KeywordRules:     req.KeywordRules,
		WhitelistRules:   req.WhitelistRules,
		RegexRules:       req.RegexRules,
		BlockStatus:      req.BlockStatus,
		BlockMessage:     req.BlockMessage,
		HitRetentionDays: req.HitRetentionDays,
	}
	if req.Config != nil {
		patch = service.UpdateKeywordFilterConfigInput{
			Enabled:          req.Config.Enabled,
			AllGroups:        req.Config.AllGroups,
			GroupIDs:         req.Config.GroupIDs,
			Keywords:         req.Config.Keywords,
			Whitelist:        req.Config.Whitelist,
			KeywordRules:     req.Config.KeywordRules,
			WhitelistRules:   req.Config.WhitelistRules,
			RegexRules:       req.Config.RegexRules,
			BlockStatus:      req.Config.BlockStatus,
			BlockMessage:     req.Config.BlockMessage,
			HitRetentionDays: req.Config.HitRetentionDays,
		}
	}
	result, err := h.service.Test(c.Request.Context(), service.KeywordFilterTestInput{Text: req.Text, Config: &patch})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *KeywordFilterHandler) ListLogs(c *gin.Context) {
	if !h.available(c) {
		return
	}
	page, pageSize := response.ParsePagination(c)
	filter := service.KeywordFilterLogFilter{
		Pagination: pagination.PaginationParams{
			Page:      page,
			PageSize:  pageSize,
			SortOrder: pagination.SortOrderDesc,
		},
		MatchType: c.Query("match_type"),
		Endpoint:  c.Query("endpoint"),
		Search:    c.Query("search"),
	}
	if raw := strings.TrimSpace(c.Query("group_id")); raw != "" {
		groupID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || groupID <= 0 {
			response.BadRequest(c, "Invalid group_id")
			return
		}
		filter.GroupID = &groupID
	}
	if raw := strings.TrimSpace(c.Query("from")); raw != "" {
		t, _, err := parseContentModerationDate(raw)
		if err != nil {
			response.BadRequest(c, "Invalid from")
			return
		}
		filter.From = &t
	}
	if raw := strings.TrimSpace(c.Query("to")); raw != "" {
		t, dateOnly, err := parseContentModerationDate(raw)
		if err != nil {
			response.BadRequest(c, "Invalid to")
			return
		}
		if dateOnly {
			t = t.Add(24*time.Hour - time.Nanosecond)
		}
		filter.To = &t
	}
	items, pageResult, err := h.service.ListLogs(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, pageResult.Total, pageResult.Page, pageResult.PageSize)
}
