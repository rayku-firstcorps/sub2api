package admin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type KiroImportRequest struct {
	Content              string         `json:"content"`
	Contents             []string       `json:"contents"`
	Name                 string         `json:"name"`
	UpdateExisting       *bool          `json:"update_existing"`
	Notes                *string        `json:"notes"`
	GroupIDs             []int64        `json:"group_ids"`
	ProxyID              *int64         `json:"proxy_id"`
	Concurrency          *int           `json:"concurrency"`
	Priority             *int           `json:"priority"`
	RateMultiplier       *float64       `json:"rate_multiplier"`
	LoadFactor           *int           `json:"load_factor"`
	AuthMethod           string         `json:"auth_method"`
	ClientID             string         `json:"client_id"`
	ClientSecret         string         `json:"client_secret"`
	Region               string         `json:"region"`
	ProfileArn           string         `json:"profile_arn"`
	SkipValidation       *bool          `json:"skip_validation"`
	SkipDefaultGroupBind *bool          `json:"skip_default_group_bind"`
	Extra                map[string]any `json:"extra"`
}

type KiroImportResult struct {
	Total   int                `json:"total"`
	Created int                `json:"created"`
	Updated int                `json:"updated"`
	Skipped int                `json:"skipped"`
	Failed  int                `json:"failed"`
	Items   []KiroImportItem   `json:"items,omitempty"`
	Errors  []KiroImportError  `json:"errors,omitempty"`
}

type KiroImportItem struct {
	Index     int    `json:"index"`
	Action    string `json:"action"`
	AccountID int64  `json:"account_id,omitempty"`
	Message   string `json:"message,omitempty"`
}

type KiroImportError struct {
	Index   int    `json:"index"`
	Message string `json:"message"`
}

func (h *AccountHandler) ImportKiroAccount(c *gin.Context) {
	var req KiroImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if req.Concurrency != nil && *req.Concurrency < 0 {
		response.BadRequest(c, "concurrency must be >= 0")
		return
	}
	if req.Priority != nil && *req.Priority < 0 {
		response.BadRequest(c, "priority must be >= 0")
		return
	}
	if req.RateMultiplier != nil && *req.RateMultiplier < 0 {
		response.BadRequest(c, "rate_multiplier must be >= 0")
		return
	}
	if req.LoadFactor != nil && *req.LoadFactor > 10000 {
		response.BadRequest(c, "load_factor must be <= 10000")
		return
	}

	authMethod := normalizeKiroAuthMethod(req.AuthMethod)
	if authMethod == "" {
		authMethod = kiro.AuthMethodSocial
	}
	if authMethod != kiro.AuthMethodSocial && authMethod != kiro.AuthMethodBuilderID {
		response.BadRequest(c, fmt.Sprintf("unsupported auth_method: %s (use 'social' or 'builder_id')", authMethod))
		return
	}
	if authMethod == kiro.AuthMethodBuilderID {
		if strings.TrimSpace(req.ClientID) == "" || strings.TrimSpace(req.ClientSecret) == "" {
			response.BadRequest(c, "client_id and client_secret are required for builder_id auth_method")
			return
		}
	}

	tokens := parseKiroImportTokens(req)
	if len(tokens) == 0 {
		response.BadRequest(c, "请输入 refresh_token")
		return
	}

	executeAdminIdempotentJSON(c, "admin.accounts.import_kiro", req, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		return h.importKiroAccounts(ctx, req, tokens, authMethod)
	})
}

func normalizeKiroAuthMethod(authMethod string) string {
	switch strings.TrimSpace(authMethod) {
	case "social_auth":
		return kiro.AuthMethodSocial
	default:
		return strings.TrimSpace(authMethod)
	}
}

func (h *AccountHandler) importKiroAccounts(ctx context.Context, req KiroImportRequest, tokens []string, authMethod string) (KiroImportResult, error) {
	result := KiroImportResult{
		Total: len(tokens),
		Items: make([]KiroImportItem, 0, len(tokens)),
	}

	existingAccounts, err := h.listAccountsFiltered(ctx, service.PlatformKiro, service.AccountTypeOAuth, "", "", 0, "", "created_at", "desc")
	if err != nil {
		return result, err
	}
	existingByHash := buildKiroAccountIndex(existingAccounts)

	updateExisting := true
	if req.UpdateExisting != nil {
		updateExisting = *req.UpdateExisting
	}
	concurrency := 3
	if req.Concurrency != nil {
		concurrency = *req.Concurrency
	}
	priority := 50
	if req.Priority != nil {
		priority = *req.Priority
	}
	skipValidation := false
	if req.SkipValidation != nil {
		skipValidation = *req.SkipValidation
	}
	skipDefaultGroupBind := false
	if req.SkipDefaultGroupBind != nil {
		skipDefaultGroupBind = *req.SkipDefaultGroupBind
	}
	region := strings.TrimSpace(req.Region)
	if region == "" {
		region = kiro.DefaultRegion
	}

	seenHashes := map[string]int{}

	for i, token := range tokens {
		index := i + 1
		token = strings.TrimSpace(token)
		if token == "" {
			result.Failed++
			result.Items = append(result.Items, KiroImportItem{Index: index, Action: "failed", Message: "empty token"})
			result.Errors = append(result.Errors, KiroImportError{Index: index, Message: "empty token"})
			continue
		}

		tokenHash := kiroTokenHash(token)
		if prevIdx, ok := seenHashes[tokenHash]; ok {
			msg := fmt.Sprintf("与第 %d 条导入项重复，已跳过", prevIdx)
			result.Skipped++
			result.Items = append(result.Items, KiroImportItem{Index: index, Action: "skipped", Message: msg})
			continue
		}
		seenHashes[tokenHash] = index

		credentials := map[string]any{
			"refresh_token": token,
			"auth_method":   authMethod,
			"region":        region,
		}
		if req.ProfileArn != "" {
			credentials["profile_arn"] = strings.TrimSpace(req.ProfileArn)
		}
		if authMethod == kiro.AuthMethodBuilderID {
			credentials["client_id"] = strings.TrimSpace(req.ClientID)
			credentials["client_secret"] = strings.TrimSpace(req.ClientSecret)
		}
		if proxyID := req.ProxyID; proxyID != nil {
			credentials["proxy_url"] = "" // will be resolved by proxy system
		}

		if !skipValidation {
			refreshedCreds, refreshErr := validateKiroToken(ctx, token, authMethod, region, req.ClientID, req.ClientSecret)
			if refreshErr != nil {
				result.Failed++
				msg := fmt.Sprintf("token 验证失败: %s", refreshErr.Error())
				result.Items = append(result.Items, KiroImportItem{Index: index, Action: "failed", Message: msg})
				result.Errors = append(result.Errors, KiroImportError{Index: index, Message: msg})
				continue
			}
			for k, v := range refreshedCreds {
				credentials[k] = v
			}
		}

		if existing, ok := existingByHash[tokenHash]; ok {
			if !updateExisting {
				result.Skipped++
				result.Items = append(result.Items, KiroImportItem{Index: index, Action: "skipped", Message: "已存在相同 token 的账号", AccountID: existing.ID})
				continue
			}
			updateInput := &service.UpdateAccountInput{
				Credentials:    credentials,
				Concurrency:    req.Concurrency,
				Priority:       req.Priority,
				RateMultiplier: req.RateMultiplier,
				LoadFactor:     req.LoadFactor,
			}
			if req.ProxyID != nil {
				updateInput.ProxyID = req.ProxyID
			}
			if len(req.GroupIDs) > 0 {
				groupIDs := append([]int64(nil), req.GroupIDs...)
				updateInput.GroupIDs = &groupIDs
			}
			updated, updateErr := h.adminService.UpdateAccount(ctx, existing.ID, updateInput)
			if updateErr != nil {
				result.Failed++
				result.Items = append(result.Items, KiroImportItem{Index: index, Action: "failed", Message: updateErr.Error()})
				result.Errors = append(result.Errors, KiroImportError{Index: index, Message: updateErr.Error()})
				continue
			}
			if h.tokenCacheInvalidator != nil && updated != nil {
				_ = h.tokenCacheInvalidator.InvalidateToken(ctx, updated)
			}
			result.Updated++
			accountID := existing.ID
			if updated != nil {
				accountID = updated.ID
			}
			result.Items = append(result.Items, KiroImportItem{Index: index, Action: "updated", AccountID: accountID})
			continue
		}

		accountName := buildKiroAccountName(req.Name, index, len(tokens))
		extra := map[string]any{
			"import_source":      "kiro_refresh_token",
			"imported_at":        time.Now().UTC().Format(time.RFC3339),
			"refresh_token_hash": tokenHash,
		}
		if req.Extra != nil {
			for k, v := range req.Extra {
				extra[k] = v
			}
		}

		account, createErr := h.adminService.CreateAccount(ctx, &service.CreateAccountInput{
			Name:                 accountName,
			Notes:                req.Notes,
			Platform:             service.PlatformKiro,
			Type:                 service.AccountTypeOAuth,
			Credentials:          credentials,
			Extra:                extra,
			ProxyID:              req.ProxyID,
			Concurrency:          concurrency,
			Priority:             priority,
			RateMultiplier:       req.RateMultiplier,
			LoadFactor:           req.LoadFactor,
			GroupIDs:             req.GroupIDs,
			SkipDefaultGroupBind: skipDefaultGroupBind,
		})
		if createErr != nil {
			result.Failed++
			result.Items = append(result.Items, KiroImportItem{Index: index, Action: "failed", Message: createErr.Error()})
			result.Errors = append(result.Errors, KiroImportError{Index: index, Message: createErr.Error()})
			continue
		}
		result.Created++
		accountID := int64(0)
		if account != nil {
			accountID = account.ID
			existingByHash[tokenHash] = *account
		}
		result.Items = append(result.Items, KiroImportItem{Index: index, Action: "created", AccountID: accountID})
	}

	return result, nil
}

func parseKiroImportTokens(req KiroImportRequest) []string {
	var tokens []string
	if s := strings.TrimSpace(req.Content); s != "" {
		for _, line := range strings.Split(s, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				tokens = append(tokens, line)
			}
		}
	}
	for _, content := range req.Contents {
		if s := strings.TrimSpace(content); s != "" {
			for _, line := range strings.Split(s, "\n") {
				line = strings.TrimSpace(line)
				if line != "" {
					tokens = append(tokens, line)
				}
			}
		}
	}
	return tokens
}

func validateKiroToken(ctx context.Context, refreshToken, authMethod, region, clientID, clientSecret string) (map[string]any, error) {
	account := &service.Account{
		Platform: service.PlatformKiro,
		Type:     service.AccountTypeOAuth,
		Credentials: map[string]any{
			"refresh_token": refreshToken,
			"auth_method":   authMethod,
			"region":        region,
			"client_id":     clientID,
			"client_secret": clientSecret,
		},
	}
	refresher := service.NewKiroTokenRefresher()
	return refresher.Refresh(ctx, account)
}

func kiroTokenHash(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}

func buildKiroAccountIndex(accounts []service.Account) map[string]service.Account {
	index := make(map[string]service.Account, len(accounts))
	for _, account := range accounts {
		rt, _ := account.Credentials["refresh_token"].(string)
		if rt = strings.TrimSpace(rt); rt != "" {
			index[kiroTokenHash(rt)] = account
		}
	}
	return index
}

func buildKiroAccountName(base string, index, total int) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "Kiro"
	}
	if total > 1 {
		return fmt.Sprintf("%s #%d", base, index)
	}
	return base
}
