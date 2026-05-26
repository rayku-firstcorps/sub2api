package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func (h *GatewayHandler) checkKeywordFilter(c *gin.Context, reqLog *zap.Logger, apiKey *service.APIKey, subject middleware2.AuthSubject, protocol string, model string, body []byte) *service.KeywordFilterDecision {
	if h == nil || h.keywordFilterService == nil {
		return nil
	}
	return runKeywordFilter(c, reqLog, h.keywordFilterService, apiKey, subject, protocol, model, body)
}

func (h *OpenAIGatewayHandler) checkKeywordFilter(c *gin.Context, reqLog *zap.Logger, apiKey *service.APIKey, subject middleware2.AuthSubject, protocol string, model string, body []byte) *service.KeywordFilterDecision {
	if h == nil || h.keywordFilterService == nil {
		return nil
	}
	return runKeywordFilter(c, reqLog, h.keywordFilterService, apiKey, subject, protocol, model, body)
}

func runKeywordFilter(c *gin.Context, reqLog *zap.Logger, svc *service.KeywordFilterService, apiKey *service.APIKey, subject middleware2.AuthSubject, protocol string, model string, body []byte) *service.KeywordFilterDecision {
	if svc == nil || c == nil || c.Request == nil || protocol == service.ContentModerationProtocolOpenAIImages {
		return nil
	}
	input := buildKeywordFilterInput(c, apiKey, subject, protocol, model, body)
	decision, err := svc.Check(c.Request.Context(), input)
	if err != nil {
		if reqLog != nil {
			reqLog.Warn("keyword_filter.check_failed", zap.Error(err))
		}
		return nil
	}
	if reqLog != nil && decision != nil {
		reqLog.Info("keyword_filter.gateway_check_done",
			zap.String("request_id", input.RequestID),
			zap.Bool("allowed", decision.Allowed),
			zap.Bool("blocked", decision.Blocked),
			zap.String("match_type", decision.MatchType),
			zap.String("rule_name", decision.RuleName),
			zap.Int("status_code", decision.StatusCode),
		)
	}
	return decision
}

func buildKeywordFilterInput(c *gin.Context, apiKey *service.APIKey, subject middleware2.AuthSubject, protocol string, model string, body []byte) service.KeywordFilterCheckInput {
	input := service.KeywordFilterCheckInput{
		RequestID: keywordFilterRequestID(c.Request.Context()),
		UserID:    subject.UserID,
		Endpoint:  GetInboundEndpoint(c),
		Provider:  contentModerationProvider(apiKey),
		Model:     strings.TrimSpace(model),
		Protocol:  protocol,
		Body:      body,
	}
	if forcedPlatform, ok := middleware2.GetForcePlatformFromContext(c); ok {
		input.Provider = strings.TrimSpace(forcedPlatform)
	}
	if apiKey != nil {
		input.APIKeyID = apiKey.ID
		input.APIKeyName = apiKey.Name
		if apiKey.User != nil {
			input.UserEmail = apiKey.User.Email
		}
		if apiKey.GroupID != nil {
			groupID := *apiKey.GroupID
			input.GroupID = &groupID
		}
		if apiKey.Group != nil {
			input.GroupName = apiKey.Group.Name
		}
	}
	if input.Endpoint == "" && c.Request != nil && c.Request.URL != nil {
		input.Endpoint = c.Request.URL.Path
	}
	return input
}

func keywordFilterStatus(decision *service.KeywordFilterDecision) int {
	if decision == nil || decision.StatusCode < 400 || decision.StatusCode > 599 {
		return http.StatusForbidden
	}
	return decision.StatusCode
}

func keywordFilterErrorCode(decision *service.KeywordFilterDecision) string {
	return "keyword_filter_violation"
}

func keywordFilterRequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if requestID, ok := ctx.Value(ctxkey.RequestID).(string); ok {
		return strings.TrimSpace(requestID)
	}
	return ""
}

const wsKeywordFilterWindowMaxRunes = 512

type wsKeywordFilterSession struct {
	mu     sync.Mutex
	window []rune
}

func newWSKeywordFilterSession() *wsKeywordFilterSession {
	return &wsKeywordFilterSession{}
}

func (s *wsKeywordFilterSession) checkWithHistory(c *gin.Context, reqLog *zap.Logger, svc *service.KeywordFilterService, apiKey *service.APIKey, subject middleware2.AuthSubject, protocol string, model string, body []byte) *service.KeywordFilterDecision {
	if svc == nil {
		return nil
	}
	decision := runKeywordFilter(c, reqLog, svc, apiKey, subject, protocol, model, body)
	if decision != nil && decision.Blocked {
		return decision
	}
	if svc.IsLenientMode(c.Request.Context()) {
		return nil
	}
	texts := service.ExtractKeywordFilterTexts(protocol, body)
	if len(texts) == 0 {
		return nil
	}
	currentText := strings.Join(texts, " ")
	s.mu.Lock()
	s.window = append(s.window, []rune(currentText)...)
	if len(s.window) > wsKeywordFilterWindowMaxRunes {
		s.window = s.window[len(s.window)-wsKeywordFilterWindowMaxRunes:]
	}
	combined := string(s.window)
	s.mu.Unlock()
	if combined == currentText {
		return nil
	}
	syntheticBody, err := json.Marshal(struct {
		Input string `json:"input"`
	}{Input: combined})
	if err != nil {
		slog.Warn("keyword_filter.ws_synthetic_body_marshal_failed", "error", err)
		return nil
	}
	return runKeywordFilter(c, reqLog, svc, apiKey, subject, protocol, model, syntheticBody)
}
