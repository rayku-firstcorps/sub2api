package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

type KiroGatewayService struct {
	accountRepo   AccountRepository
	tokenProvider *KiroTokenProvider
	httpUpstream  HTTPUpstream
	promptCache   KiroPromptCache
	rateLimit     *RateLimitService
}

func NewKiroGatewayService(
	accountRepo AccountRepository,
	tokenProvider *KiroTokenProvider,
	httpUpstream HTTPUpstream,
	promptCache KiroPromptCache,
	rateLimit *RateLimitService,
) *KiroGatewayService {
	return &KiroGatewayService{
		accountRepo:   accountRepo,
		tokenProvider: tokenProvider,
		httpUpstream:  httpUpstream,
		promptCache:   promptCache,
		rateLimit:     rateLimit,
	}
}

func (s *KiroGatewayService) Forward(ctx context.Context, c *gin.Context, account *Account, body []byte, _ bool, apiKey *APIKey) (*ForwardResult, error) {
	startTime := time.Now()

	parsed := gjson.ParseBytes(body)
	requestModel := parsed.Get("model").String()
	if requestModel == "" {
		return nil, s.writeError(c, http.StatusBadRequest, "invalid_request_error", "Missing model")
	}

	kiroModel := s.mapModel(account, requestModel)
	if kiroModel == "" {
		return nil, s.writeError(c, http.StatusBadRequest, "invalid_request_error",
			fmt.Sprintf("Model %s not supported on kiro platform", requestModel))
	}

	accessToken, err := s.tokenProvider.GetAccessToken(ctx, account)
	if err != nil {
		slog.Warn("kiro_gateway.get_access_token_failed", "account_id", account.ID, "error", err)
		return nil, &UpstreamFailoverError{
			StatusCode: http.StatusUnauthorized,
		}
	}

	profileArn := account.GetCredential("profile_arn")
	region := account.GetCredential("region")
	if region == "" {
		region = kiro.DefaultRegion
	}

	kiroReq, err := kiro.ConvertRequest(body, kiroModel, profileArn)
	if err != nil {
		return nil, s.writeError(c, http.StatusBadRequest, "invalid_request_error", "Failed to convert request: "+err.Error())
	}

	reqBody, err := json.Marshal(kiroReq)
	if err != nil {
		return nil, s.writeError(c, http.StatusInternalServerError, "api_error", "Failed to marshal kiro request")
	}
	logKiroRequestDiagnostics(c.Request.Context(), account.ID, requestModel, kiroModel, kiroReq, reqBody)

	endpoint := kiro.GetGenerateURL(region)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return nil, s.writeError(c, http.StatusInternalServerError, "api_error", "Failed to create request")
	}

	s.setRequestHeaders(httpReq, accessToken)

	proxyURL := account.GetCredential("proxy_url")
	resp, err := s.httpUpstream.Do(httpReq, proxyURL, account.ID, 1)
	if err != nil {
		slog.Warn("kiro_gateway.upstream_request_failed", "account_id", account.ID, "error", err)
		return nil, &UpstreamFailoverError{
			StatusCode: http.StatusBadGateway,
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		slog.Warn("kiro_gateway.upstream_error",
			"account_id", account.ID,
			"status", resp.StatusCode,
			"body", string(respBody),
		)
		if s.rateLimit != nil {
			s.rateLimit.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
		}
		if resp.StatusCode == http.StatusUnauthorized ||
			resp.StatusCode == http.StatusForbidden ||
			resp.StatusCode == http.StatusTooManyRequests {
			return nil, &UpstreamFailoverError{
				StatusCode:      resp.StatusCode,
				ResponseBody:    respBody,
				ResponseHeaders: resp.Header.Clone(),
			}
		}
		return nil, s.writeError(c, resp.StatusCode, "api_error",
			fmt.Sprintf("Kiro upstream error (HTTP %d)", resp.StatusCode))
	}

	toolNameMap := buildKiroToolNameMap(parsed)
	usage := ClaudeUsage{InputTokens: estimateKiroInputTokens(body)}
	s.applyPseudoCacheBilling(ctx, account, apiKey, body, &usage)
	isStream := parsed.Get("stream").Bool()

	var result *ForwardResult
	if isStream {
		result, err = s.streamResponse(c, resp.Body, toolNameMap, requestModel, usage, startTime, account.ID)
	} else {
		result, err = s.nonStreamResponse(c, resp.Body, toolNameMap, requestModel, usage, account.ID)
	}
	if err != nil {
		return nil, err
	}

	result.Model = requestModel
	result.UpstreamModel = kiroModel
	result.Stream = isStream
	result.Duration = time.Since(startTime)

	return result, nil
}

func (s *KiroGatewayService) applyPseudoCacheBilling(ctx context.Context, account *Account, apiKey *APIKey, body []byte, usage *ClaudeUsage) {
	if s == nil || s.promptCache == nil || usage == nil || usage.InputTokens <= 0 {
		return
	}
	breakpoints := computeKiroCacheBreakpoints(body)
	if len(breakpoints) == 0 {
		return
	}
	namespace := kiroPromptCacheNamespace(account, apiKey)
	if namespace == "" {
		return
	}
	cacheResult, err := s.promptCache.LookupOrCreate(ctx, namespace, breakpoints, usage.InputTokens)
	if err != nil {
		slog.Warn("kiro_gateway.pseudo_cache_lookup_failed", "namespace", namespace, "error", err)
		return
	}
	slog.Info("kiro_gateway.pseudo_cache_usage",
		"component", "service.kiro_gateway",
		"namespace", namespace,
		"breakpoints", len(breakpoints),
		"read", cacheResult.CacheReadInputTokens,
		"creation", cacheResult.CacheCreationInputTokens,
		"uncached", cacheResult.UncachedInputTokens,
	)
	applyKiroCacheResultToUsage(usage, cacheResult)
}

func (s *KiroGatewayService) streamResponse(c *gin.Context, body io.Reader, toolNameMap map[string]string, model string, usage ClaudeUsage, startTime time.Time, accountIDOpt ...int64) (*ForwardResult, error) {
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
	}
	accountID := optionalAccountID(accountIDOpt)

	requestID := uuid.New().String()
	parser := kiro.NewStreamParser()
	converter := kiro.NewStreamConverter(toolNameMap)

	var firstTokenMs *int
	clientDisconnect := false
	started := false
	sawPayload := false
	sawStop := false
	stopReason := "end_turn"
	var outputText strings.Builder
	var toolInputText strings.Builder
	toolDiag := newKiroToolDiagnostics(ginRequestContext(c), accountID, model, true)

	startStream := func() error {
		if started {
			return nil
		}
		c.Header("Content-Type", "text/event-stream; charset=utf-8")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")

		startUsage := usage
		startUsage.OutputTokens = 0
		msgStartEvent := gin.H{
			"type": "message_start",
			"message": gin.H{
				"id":            "msg_" + requestID,
				"type":          "message",
				"role":          "assistant",
				"content":       []any{},
				"model":         model,
				"stop_reason":   nil,
				"stop_sequence": nil,
				"usage":         kiroUsagePayload(startUsage),
			},
		}
		msgStartData, _ := json.Marshal(msgStartEvent)
		if _, err := fmt.Fprintf(c.Writer, "event: message_start\ndata: %s\n\n", msgStartData); err != nil {
			return err
		}
		flusher.Flush()
		started = true
		return nil
	}

	writeEvents := func(events []kiro.AnthropicSSEEvent) error {
		if len(events) == 0 {
			return nil
		}
		if err := startStream(); err != nil {
			return err
		}
		for _, sse := range events {
			if firstTokenMs == nil && sse.Event == "content_block_delta" {
				ms := int(time.Since(startTime).Milliseconds())
				firstTokenMs = &ms
			}
			data := kiro.FormatSSE(sse)
			if _, err := c.Writer.Write(data); err != nil {
				clientDisconnect = true
				return err
			}
			flusher.Flush()
		}
		return nil
	}

	readErr := readKiroEvents(body, parser, func(evt kiro.StreamEvent) error {
		if evt.Error != "" || evt.ErrorCode != "" {
			msg := evt.Error
			if msg == "" {
				msg = evt.ErrorMessage
			}
			if !started {
				return s.writeError(c, http.StatusBadGateway, "api_error", "Kiro stream error: "+msg)
			}
		}

		if evt.Content != "" {
			sawPayload = true
			outputText.WriteString(evt.Content)
		}
		if evt.Name != "" && evt.ToolUseID != "" {
			sawPayload = true
			stopReason = "tool_use"
			toolDiag.toolStart(evt, toolNameMap)
		}
		if evt.Input != "" {
			sawPayload = true
			toolInputText.WriteString(string(evt.Input))
			toolDiag.toolInput(evt)
		}
		if evt.Stop != nil && *evt.Stop {
			sawStop = true
			toolDiag.toolStop()
			converter.SetOutputTokens(estimateTextTokens(outputText.String() + toolInputText.String()))
			if !sawPayload && !started {
				return nil
			}
		}

		return writeEvents(converter.Convert(evt))
	})
	if readErr != nil && !clientDisconnect {
		return nil, readErr
	}

	if !clientDisconnect && !sawPayload {
		if !started {
			return nil, s.writeError(c, http.StatusBadGateway, "api_error", "Kiro upstream returned empty response")
		}
	}

	outputTokens := estimateTextTokens(outputText.String() + toolInputText.String())
	if !clientDisconnect && sawPayload && !sawStop {
		converter.SetOutputTokens(outputTokens)
		if err := writeEvents(converter.Finish(stopReason)); err != nil && !clientDisconnect {
			return nil, err
		}
	}

	return &ForwardResult{
		RequestID:        requestID,
		Usage:            kiroUsageWithOutputTokens(usage, outputTokens),
		Stream:           true,
		FirstTokenMs:     firstTokenMs,
		ClientDisconnect: clientDisconnect,
	}, nil
}

func (s *KiroGatewayService) nonStreamResponse(c *gin.Context, body io.Reader, toolNameMap map[string]string, model string, usage ClaudeUsage, accountIDOpt ...int64) (*ForwardResult, error) {
	parser := kiro.NewStreamParser()
	collector := newKiroMessageCollector(toolNameMap)
	accountID := optionalAccountID(accountIDOpt)
	toolDiag := newKiroToolDiagnostics(ginRequestContext(c), accountID, model, false)

	err := readKiroEvents(body, parser, func(evt kiro.StreamEvent) error {
		if evt.Error != "" || evt.ErrorCode != "" {
			msg := evt.Error
			if msg == "" {
				msg = evt.ErrorMessage
			}
			return s.writeError(c, http.StatusBadGateway, "api_error", "Kiro stream error: "+msg)
		}
		if evt.Name != "" && evt.ToolUseID != "" {
			toolDiag.toolStart(evt, toolNameMap)
		}
		if evt.Input != "" {
			toolDiag.toolInput(evt)
		}
		if evt.Stop != nil && *evt.Stop {
			toolDiag.toolStop()
		}
		collector.add(evt)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !collector.hasPayload {
		return nil, s.writeError(c, http.StatusBadGateway, "api_error", "Kiro upstream returned empty response")
	}

	outputTokens := estimateTextTokens(collector.outputText.String() + collector.toolInputText.String())
	usage = kiroUsageWithOutputTokens(usage, outputTokens)
	requestID := uuid.New().String()
	c.JSON(http.StatusOK, gin.H{
		"id":            "msg_" + requestID,
		"type":          "message",
		"role":          "assistant",
		"model":         model,
		"content":       collector.contentBlocks(),
		"stop_reason":   collector.stopReason(),
		"stop_sequence": nil,
		"usage":         kiroUsagePayload(usage),
	})

	return &ForwardResult{
		RequestID: requestID,
		Usage:     usage,
		Stream:    false,
	}, nil
}

func kiroUsageWithOutputTokens(usage ClaudeUsage, outputTokens int) ClaudeUsage {
	usage.OutputTokens = outputTokens
	return usage
}

func kiroUsagePayload(usage ClaudeUsage) gin.H {
	payload := gin.H{
		"input_tokens":  usage.InputTokens,
		"output_tokens": usage.OutputTokens,
	}
	if usage.CacheCreationInputTokens > 0 {
		payload["cache_creation_input_tokens"] = usage.CacheCreationInputTokens
	}
	if usage.CacheReadInputTokens > 0 {
		payload["cache_read_input_tokens"] = usage.CacheReadInputTokens
	}
	if usage.CacheCreation5mTokens > 0 || usage.CacheCreation1hTokens > 0 {
		payload["cache_creation"] = gin.H{
			"ephemeral_5m_input_tokens": usage.CacheCreation5mTokens,
			"ephemeral_1h_input_tokens": usage.CacheCreation1hTokens,
		}
	}
	return payload
}

func readKiroEvents(r io.Reader, parser *kiro.StreamParser, handle func(kiro.StreamEvent) error) error {
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			for _, evt := range parser.Feed(buf[:n]) {
				if handleErr := handle(evt); handleErr != nil {
					return handleErr
				}
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

type kiroToolDiagnostics struct {
	ctx       context.Context
	accountID int64
	model     string
	stream    bool

	toolUseID string
	toolName  string
	input     strings.Builder
}

func newKiroToolDiagnostics(ctx context.Context, accountID int64, model string, stream bool) *kiroToolDiagnostics {
	return &kiroToolDiagnostics{
		ctx:       ctx,
		accountID: accountID,
		model:     model,
		stream:    stream,
	}
}

func (d *kiroToolDiagnostics) toolStart(evt kiro.StreamEvent, toolNameMap map[string]string) {
	if d == nil {
		return
	}
	if d.toolUseID == evt.ToolUseID {
		input := string(evt.Input)
		slog.Info("kiro_gateway.tool_continue",
			d.baseAttrs(
				"tool_use_id", d.toolUseID,
				"tool_name", d.toolName,
				"kiro_tool_name", evt.Name,
				"input_len", len(input),
				"input_preview", previewForLog(input, 1200),
			)...,
		)
		return
	}
	d.flushOpenTool("implicit_new_tool")
	d.toolUseID = evt.ToolUseID
	d.toolName = kiro.RestoreToolName(evt.Name, toolNameMap)
	d.input.Reset()
	input := string(evt.Input)
	slog.Info("kiro_gateway.tool_start",
		d.baseAttrs(
			"tool_use_id", d.toolUseID,
			"tool_name", d.toolName,
			"kiro_tool_name", evt.Name,
			"initial_input_len", len(input),
			"initial_input_preview", previewForLog(input, 1200),
			"initial_input_json_valid", json.Valid([]byte(input)),
		)...,
	)
}

func (d *kiroToolDiagnostics) toolInput(evt kiro.StreamEvent) {
	if d == nil {
		return
	}
	input := string(evt.Input)
	d.input.WriteString(input)
	accum := d.input.String()
	slog.Info("kiro_gateway.tool_input_delta",
		d.baseAttrs(
			"tool_use_id", nonEmpty(evt.ToolUseID, d.toolUseID),
			"tool_name", d.toolName,
			"delta_len", len(input),
			"delta_preview", previewForLog(input, 1200),
			"delta_json_valid", json.Valid([]byte(input)),
			"accum_len", len(accum),
			"accum_preview", previewForLog(accum, 2000),
			"accum_json_valid", json.Valid([]byte(accum)),
		)...,
	)
}

func (d *kiroToolDiagnostics) toolStop() {
	if d == nil {
		return
	}
	d.flushOpenTool("stop")
}

func (d *kiroToolDiagnostics) flushOpenTool(reason string) {
	if d.toolUseID == "" && d.toolName == "" && d.input.Len() == 0 {
		return
	}
	input := d.input.String()
	slog.Info("kiro_gateway.tool_final",
		d.baseAttrs(
			"tool_use_id", d.toolUseID,
			"tool_name", d.toolName,
			"reason", reason,
			"input_len", len(input),
			"input_preview", previewForLog(input, 3000),
			"input_json_valid", json.Valid([]byte(input)),
			"input_json_type", jsonTypeForLog(input),
		)...,
	)
	d.toolUseID = ""
	d.toolName = ""
	d.input.Reset()
}

func (d *kiroToolDiagnostics) baseAttrs(attrs ...any) []any {
	base := []any{
		"component", "service.kiro_gateway",
		"request_id", requestIDFromContext(d.ctx),
		"client_request_id", clientRequestIDFromContext(d.ctx),
		"account_id", d.accountID,
		"model", d.model,
		"stream", d.stream,
	}
	return append(base, attrs...)
}

func logKiroRequestDiagnostics(ctx context.Context, accountID int64, requestModel, kiroModel string, req *kiro.GenerateRequest, reqBody []byte) {
	if req == nil || req.ConversationState.CurrentMessage == nil || req.ConversationState.CurrentMessage.UserInputMessage == nil {
		return
	}
	current := req.ConversationState.CurrentMessage.UserInputMessage
	var toolSummaries []map[string]any
	var toolResultIDs []string
	if current.UserInputMessageContext != nil {
		for _, tool := range current.UserInputMessageContext.Tools {
			summary := map[string]any{
				"name": tool.ToolSpecification.Name,
			}
			if schema, ok := tool.ToolSpecification.InputSchema.JSON.(map[string]any); ok {
				summary["schema_type"] = schema["type"]
				summary["required"] = schema["required"]
				if props, ok := schema["properties"].(map[string]any); ok {
					names := make([]string, 0, len(props))
					for name := range props {
						names = append(names, name)
					}
					sort.Strings(names)
					summary["properties"] = names
				}
			}
			toolSummaries = append(toolSummaries, summary)
		}
		for _, tr := range current.UserInputMessageContext.ToolResults {
			toolResultIDs = append(toolResultIDs, tr.ToolUseID)
		}
	}
	slog.Info("kiro_gateway.request_converted",
		"component", "service.kiro_gateway",
		"request_id", requestIDFromContext(ctx),
		"client_request_id", clientRequestIDFromContext(ctx),
		"account_id", accountID,
		"model", requestModel,
		"kiro_model", kiroModel,
		"body_bytes", len(reqBody),
		"history_len", len(req.ConversationState.History),
		"current_content_len", len(current.Content),
		"current_has_context", current.UserInputMessageContext != nil,
		"current_tool_count", len(toolSummaries),
		"current_tools", toolSummaries,
		"current_tool_result_ids", toolResultIDs,
	)
}

func requestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(ctxkey.RequestID).(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func clientRequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(ctxkey.ClientRequestID).(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func previewForLog(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max] + "...<truncated>"
}

func jsonTypeForLog(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "empty"
	}
	var parsed any
	if err := json.Unmarshal([]byte(value), &parsed); err != nil {
		return "invalid"
	}
	switch parsed.(type) {
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "bool"
	case nil:
		return "null"
	default:
		return "unknown"
	}
}

func nonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func optionalAccountID(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	return values[0]
}

func ginRequestContext(c *gin.Context) context.Context {
	if c != nil && c.Request != nil {
		return c.Request.Context()
	}
	return context.Background()
}

func (s *KiroGatewayService) setRequestHeaders(req *http.Request, accessToken string) {
	setKiroRequestHeaders(req, accessToken)
}

func setKiroRequestHeaders(req *http.Request, accessToken string) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("x-amzn-codewhisperer-optout", "true")
	req.Header.Set("x-amzn-kiro-agent-mode", "vibe")
	req.Header.Set("x-amz-user-agent", fmt.Sprintf("aws-sdk-js/%s KiroIDE-%s", kiro.APIVersion, kiro.KiroVersion))
	req.Header.Set("User-Agent", fmt.Sprintf("aws-sdk-js/%s ua/2.1 os/linux lang/js api/%s#%s m/E KiroIDE-%s", kiro.APIVersion, kiro.APIName, kiro.APIVersion, kiro.KiroVersion))
	req.Header.Set("amz-sdk-invocation-id", uuid.New().String())
	req.Header.Set("amz-sdk-request", "attempt=1; max=1")
	req.Header.Set("Connection", "close")
}

func (s *KiroGatewayService) mapModel(account *Account, model string) string {
	if account.Extra != nil {
		if mm, ok := account.Extra["model_mapping"]; ok {
			if mapping, ok := mm.(map[string]any); ok {
				if mapped, ok := mapping[model].(string); ok && mapped != "" {
					return mapped
				}
			}
		}
	}
	if mapped, ok := domain.DefaultKiroModelMapping[model]; ok {
		return mapped
	}
	return ""
}

func (s *KiroGatewayService) writeError(c *gin.Context, status int, errType, message string) error {
	c.JSON(status, gin.H{
		"type":  "error",
		"error": gin.H{"type": errType, "message": message},
	})
	return fmt.Errorf("%s", message)
}

func buildKiroToolNameMap(parsed gjson.Result) map[string]string {
	tools := parsed.Get("tools").Array()
	m := make(map[string]string)
	for _, tool := range tools {
		name := tool.Get("name").String()
		shortened := kiro.ShortenToolName(name)
		if shortened != name {
			m[shortened] = name
		}
	}
	return m
}

type kiroMessageCollector struct {
	toolNameMap   map[string]string
	blocks        []gin.H
	currentTool   *gin.H
	currentToolID string
	hasPayload    bool
	hasTool       bool
	hasText       bool
	hasThinking   bool
	sawStop       bool
	outputText    strings.Builder
	toolInputText strings.Builder
}

func newKiroMessageCollector(toolNameMap map[string]string) *kiroMessageCollector {
	return &kiroMessageCollector{toolNameMap: toolNameMap}
}

func (c *kiroMessageCollector) add(evt kiro.StreamEvent) {
	if evt.Content != "" {
		c.addParsedContentBlocks(kiro.ParseKiroContentBlocks(evt.Content, c.toolNameMap))
		return
	}

	if evt.Name != "" && evt.ToolUseID != "" {
		c.hasPayload = true
		c.hasTool = true
		if c.currentToolID == evt.ToolUseID && c.currentTool != nil {
			if evt.Input != "" {
				c.appendToolInput(string(evt.Input))
			}
			return
		}
		toolName := kiro.RestoreToolName(evt.Name, c.toolNameMap)
		block := gin.H{
			"type":  "tool_use",
			"id":    evt.ToolUseID,
			"name":  toolName,
			"input": gin.H{},
		}
		c.blocks = append(c.blocks, block)
		c.currentTool = &c.blocks[len(c.blocks)-1]
		c.currentToolID = evt.ToolUseID
		c.toolInputText.Reset()
		if evt.Input != "" {
			c.appendToolInput(string(evt.Input))
		}
		return
	}

	if evt.Input != "" {
		c.hasPayload = true
		c.appendToolInput(string(evt.Input))
		return
	}

	if evt.Stop != nil && *evt.Stop {
		c.sawStop = true
	}
}

func (c *kiroMessageCollector) appendToolInput(partial string) {
	c.toolInputText.WriteString(partial)
	if c.currentTool == nil {
		return
	}
	var parsed any
	if err := json.Unmarshal([]byte(c.toolInputText.String()), &parsed); err == nil && parsed != nil {
		(*c.currentTool)["input"] = parsed
	}
}

func (c *kiroMessageCollector) addParsedContentBlocks(blocks []kiro.ParsedContentBlock) {
	for _, block := range blocks {
		switch block.Type {
		case "thinking":
			if block.Thinking == "" {
				continue
			}
			c.hasPayload = true
			c.hasThinking = true
			c.outputText.WriteString(block.Thinking)
			c.blocks = append(c.blocks, gin.H{"type": "thinking", "thinking": block.Thinking})
			c.currentTool = nil
			c.currentToolID = ""
		case "tool_use":
			c.hasPayload = true
			c.hasTool = true
			c.toolInputText.Reset()
			if raw, err := json.Marshal(block.ToolInput); err == nil {
				c.toolInputText.WriteString(string(raw))
			}
			c.blocks = append(c.blocks, gin.H{
				"type":  "tool_use",
				"id":    block.ToolUseID,
				"name":  block.ToolName,
				"input": block.ToolInput,
			})
			c.currentTool = nil
			c.currentToolID = ""
		default:
			if block.Text == "" {
				continue
			}
			c.hasPayload = true
			if strings.TrimSpace(block.Text) != "" {
				c.hasText = true
			}
			c.outputText.WriteString(block.Text)
			if len(c.blocks) > 0 {
				if lastText, ok := c.blocks[len(c.blocks)-1]["text"].(string); ok && c.blocks[len(c.blocks)-1]["type"] == "text" {
					c.blocks[len(c.blocks)-1]["text"] = lastText + block.Text
					continue
				}
			}
			c.blocks = append(c.blocks, gin.H{"type": "text", "text": block.Text})
			c.currentTool = nil
			c.currentToolID = ""
		}
	}
}

func (c *kiroMessageCollector) contentBlocks() []gin.H {
	if c.hasThinking && !c.hasText && !c.hasTool {
		return append(c.blocks, gin.H{"type": "text", "text": " "})
	}
	return c.blocks
}

func (c *kiroMessageCollector) stopReason() string {
	if c.hasTool {
		return "tool_use"
	}
	if c.hasThinking && !c.hasText {
		return "max_tokens"
	}
	return "end_turn"
}

func estimateKiroInputTokens(body []byte) int {
	parsed := gjson.ParseBytes(body)
	var chars int

	system := parsed.Get("system")
	if system.Exists() {
		if system.IsArray() {
			for _, part := range system.Array() {
				chars += utf8.RuneCountInString(part.Get("text").String())
			}
		} else {
			chars += utf8.RuneCountInString(system.String())
		}
	}

	for _, msg := range parsed.Get("messages").Array() {
		chars += utf8.RuneCountInString(msg.Get("role").String()) + 4
		content := msg.Get("content")
		if content.IsArray() {
			for _, block := range content.Array() {
				switch block.Get("type").String() {
				case "text":
					chars += utf8.RuneCountInString(block.Get("text").String())
				case "tool_result":
					chars += utf8.RuneCountInString(block.Get("content").Raw)
				case "tool_use":
					chars += utf8.RuneCountInString(block.Get("name").String())
					chars += utf8.RuneCountInString(block.Get("input").Raw)
				case "image":
					chars += 1024
				}
			}
		} else {
			chars += utf8.RuneCountInString(content.String())
		}
	}

	toolsRaw := parsed.Get("tools").Raw
	if toolsRaw != "" {
		chars += utf8.RuneCountInString(toolsRaw)
	}

	return estimateTokensFromRuneCount(chars)
}

func estimateTextTokens(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	return estimateTokensFromRuneCount(utf8.RuneCountInString(text))
}

func estimateTokensFromRuneCount(chars int) int {
	if chars <= 0 {
		return 1
	}
	tokens := (chars + 3) / 4
	if tokens < 1 {
		return 1
	}
	return tokens
}

func mapKiroModel(account *Account, model string) string {
	if account == nil {
		return ""
	}
	if account.Extra != nil {
		if mm, ok := account.Extra["model_mapping"]; ok {
			if mapping, ok := mm.(map[string]any); ok {
				if mapped, ok := mapping[model].(string); ok && mapped != "" {
					return mapped
				}
			}
		}
	}
	if mapped, ok := domain.DefaultKiroModelMapping[model]; ok {
		return mapped
	}
	return ""
}
