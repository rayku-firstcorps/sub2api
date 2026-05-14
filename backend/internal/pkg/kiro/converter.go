package kiro

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

// ConvertRequest converts an Anthropic Messages API request body to a Kiro generateAssistantResponse request.
func ConvertRequest(body []byte, kiroModel string, profileArn string) (*GenerateRequest, error) {
	parsed := gjson.ParseBytes(body)

	system := extractSystem(parsed)
	messages := parsed.Get("messages").Array()
	tools := parsed.Get("tools").Array()
	thinkingType := parsed.Get("thinking.type").String()
	thinkingBudget := int(parsed.Get("thinking.budget_tokens").Int())
	thinkingEffort := parsed.Get("thinking.effort").String()

	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages in request")
	}

	// Build tool name maps for shortening
	toolNameMaps := buildToolNameMaps(tools)

	// Convert tools to Kiro format
	kiroTools := convertTools(tools)

	// Convert messages to Kiro history + current message
	history, currentMsg, err := convertMessages(messages, system, kiroModel, kiroTools, toolNameMaps, thinkingType, thinkingBudget, thinkingEffort)
	if err != nil {
		return nil, err
	}

	req := &GenerateRequest{
		ConversationState: ConversationState{
			AgentTaskType:   AgentTaskTypeVibe,
			ChatTriggerType: ChatTriggerTypeManual,
			ConversationID:  uuid.New().String(),
			History:         history,
			CurrentMessage:  currentMsg,
		},
		ProfileArn: profileArn,
	}

	return req, nil
}

// ShortenToolName shortens a tool name to fit within MaxToolNameLen using a hash suffix.
func ShortenToolName(name string) string {
	if len(name) <= MaxToolNameLen {
		return name
	}
	h := sha256.Sum256([]byte(name))
	hash := fmt.Sprintf("%x", h[:6])
	prefix := name[:MaxToolNameLen-len(hash)-1]
	return prefix + "_" + hash
}

func buildToolNameMaps(tools []gjson.Result) map[string]string {
	m := make(map[string]string)
	for _, tool := range tools {
		name := tool.Get("name").String()
		if name == "" {
			continue
		}
		shortened := ShortenToolName(name)
		if shortened != name {
			m[shortened] = name
		}
	}
	return m
}

func convertTools(tools []gjson.Result) []ToolSpecWrapper {
	if len(tools) == 0 {
		return nil
	}
	result := make([]ToolSpecWrapper, 0, len(tools))
	for _, tool := range tools {
		if isIgnoredKiroTool(tool) {
			continue
		}
		spec, ok := convertTool(tool)
		if !ok {
			continue
		}
		result = append(result, spec)
	}
	return result
}

func convertTool(tool gjson.Result) (ToolSpecWrapper, bool) {
	if !isKiroCompatibleTool(tool) {
		return ToolSpecWrapper{}, false
	}

	name := tool.Get("name").String()
	desc := tool.Get("description").String()
	if len(desc) > MaxToolDescLen {
		desc = desc[:MaxToolDescLen] + "..."
	}

	var schema any
	schemaRaw := tool.Get("input_schema").Raw
	if schemaRaw != "" {
		_ = json.Unmarshal([]byte(schemaRaw), &schema)
	} else {
		schema = map[string]any{}
	}

	return ToolSpecWrapper{
		ToolSpecification: ToolSpecification{
			Name:        ShortenToolName(name),
			Description: desc,
			InputSchema: InputSchema{JSON: schema},
		},
	}, true
}

func isIgnoredKiroTool(tool gjson.Result) bool {
	name := strings.ToLower(tool.Get("name").String())
	return name == "web_search" || name == "websearch"
}

func isKiroCompatibleTool(tool gjson.Result) bool {
	if tool.Get("name").String() == "" {
		return false
	}
	if strings.TrimSpace(tool.Get("description").String()) == "" {
		return false
	}
	schema := tool.Get("input_schema")
	return !schema.Exists() || schema.IsObject()
}

func extractSystem(parsed gjson.Result) string {
	sys := parsed.Get("system")
	if !sys.Exists() {
		return ""
	}
	if sys.IsArray() {
		var parts []string
		for _, item := range sys.Array() {
			if item.Get("type").String() == "text" {
				parts = append(parts, item.Get("text").String())
			}
		}
		return strings.Join(parts, "\n")
	}
	return sys.String()
}

func convertMessages(messages []gjson.Result, system, kiroModel string, kiroTools []ToolSpecWrapper, toolNameMaps map[string]string, thinkingType string, thinkingBudget int, thinkingEffort string) ([]Turn, *Turn, error) {
	if len(messages) == 0 {
		return nil, nil, fmt.Errorf("empty messages")
	}

	// Build thinking prefix for system prompt
	thinkingPrefix := buildThinkingPrefix(thinkingType, thinkingBudget, thinkingEffort)
	if thinkingPrefix != "" && hasThinkingPrefix(system) {
		thinkingPrefix = ""
	}

	// Merge adjacent same-role messages
	merged := mergeAdjacentMessages(removeTrailingPartialAssistant(messages))
	if len(merged) == 0 {
		return nil, nil, fmt.Errorf("empty messages")
	}

	var history []Turn
	fullSystem := thinkingPrefix + system
	prependSystemToCurrent := fullSystem != "" && len(merged) == 1 && merged[0].Get("role").String() == "user"
	startIndex := 0

	if fullSystem != "" && !prependSystemToCurrent {
		if merged[0].Get("role").String() == "user" {
			content := joinNonEmpty(fullSystem, contentText(merged[0]))
			history = append(history, buildUserTurn(content, nil, nil, nil, kiroModel))
			startIndex = 1
		} else {
			history = append(history, buildUserTurn(fullSystem, nil, nil, nil, kiroModel))
		}
	}

	for i := startIndex; i < len(merged)-1; i++ {
		msg := merged[i]
		switch msg.Get("role").String() {
		case "assistant":
			history = append(history, buildAssistantTurn(msg, toolNameMaps))
		default:
			content, images, toolResults := extractUserContentForHistory(msg, (len(merged)-1)-i <= MaxRecentImagesMessages)
			history = append(history, buildUserTurn(content, images, toolResults, nil, kiroModel))
		}
	}

	lastMsg := merged[len(merged)-1]
	var currentTurn *Turn
	if lastMsg.Get("role").String() == "assistant" {
		history = append(history, buildAssistantTurn(lastMsg, toolNameMaps))
		turn := buildUserTurn("Continue", nil, nil, kiroTools, kiroModel)
		currentTurn = &turn
	} else {
		if len(history) > 0 && !historyEndsWithAssistant(history) {
			history = append(history, Turn{
				AssistantResponseMessage: &AssistantResponseMessage{Content: "Continue"},
			})
		}
		content, images, toolResults := extractUserContent(lastMsg)
		if prependSystemToCurrent {
			content = joinNonEmpty(fullSystem, content)
		}
		if strings.TrimSpace(content) == "" {
			if len(toolResults) > 0 {
				content = "Tool results provided."
			} else {
				content = "Continue"
			}
		}
		turn := buildUserTurn(content, images, toolResults, kiroTools, kiroModel)
		currentTurn = &turn
	}

	return history, currentTurn, nil
}

func buildThinkingPrefix(thinkingType string, thinkingBudget int, thinkingEffort string) string {
	switch strings.ToLower(strings.TrimSpace(thinkingType)) {
	case "enabled":
		budget := thinkingBudget
		if budget < ThinkingMinBudget {
			budget = ThinkingDefaultBudget
		}
		if budget > ThinkingMaxBudget {
			budget = ThinkingMaxBudget
		}
		return fmt.Sprintf("<thinking_mode>enabled</thinking_mode><max_thinking_length>%d</max_thinking_length>\n\n", budget)
	case "adaptive":
		effort := strings.ToLower(strings.TrimSpace(thinkingEffort))
		if effort != "low" && effort != "medium" && effort != "high" {
			effort = "high"
		}
		return fmt.Sprintf("<thinking_mode>adaptive</thinking_mode><thinking_effort>%s</thinking_effort>\n\n", effort)
	default:
		return ""
	}
}

func hasThinkingPrefix(text string) bool {
	return strings.Contains(text, "<thinking_mode>") ||
		strings.Contains(text, "<max_thinking_length>") ||
		strings.Contains(text, "<thinking_effort>")
}

func mergeAdjacentMessages(messages []gjson.Result) []gjson.Result {
	if len(messages) <= 1 {
		return messages
	}
	var merged []gjson.Result
	for _, msg := range messages {
		if len(merged) > 0 && merged[len(merged)-1].Get("role").String() == msg.Get("role").String() {
			merged[len(merged)-1] = mergeMessageContent(merged[len(merged)-1], msg)
			continue
		}
		merged = append(merged, msg)
	}
	return merged
}

func removeTrailingPartialAssistant(messages []gjson.Result) []gjson.Result {
	if len(messages) == 0 {
		return messages
	}
	last := messages[len(messages)-1]
	if last.Get("role").String() != "assistant" {
		return messages
	}
	content := last.Get("content")
	if !content.IsArray() {
		return messages
	}
	parts := content.Array()
	if len(parts) == 0 || parts[0].Get("type").String() != "text" || parts[0].Get("text").String() != "{" {
		return messages
	}
	return messages[:len(messages)-1]
}

func mergeMessageContent(a, b gjson.Result) gjson.Result {
	var msg map[string]any
	if err := json.Unmarshal([]byte(a.Raw), &msg); err != nil {
		return a
	}
	parts := append(contentAsBlocks(msg["content"]), contentAsBlocks(gjsonValue(b.Get("content")))...)
	msg["content"] = parts
	raw, err := json.Marshal(msg)
	if err != nil {
		return a
	}
	return gjson.ParseBytes(raw)
}

func gjsonValue(result gjson.Result) any {
	var v any
	if result.Raw != "" && json.Unmarshal([]byte(result.Raw), &v) == nil {
		return v
	}
	return result.String()
}

func contentAsBlocks(content any) []any {
	switch v := content.(type) {
	case []any:
		return v
	case string:
		if v == "" {
			return nil
		}
		return []any{map[string]any{"type": "text", "text": v}}
	case nil:
		return nil
	default:
		return []any{v}
	}
}

func buildUserTurn(content string, images []Image, toolResults []ToolResult, tools []ToolSpecWrapper, kiroModel string) Turn {
	if strings.TrimSpace(content) == "" && len(toolResults) > 0 {
		content = "Tool results provided."
	}
	msg := &UserInputMessage{
		Content: content,
		ModelID: kiroModel,
		Origin:  OriginAIEditor,
		Images:  images,
	}
	ctx := &UserInputMessageContext{}
	if len(toolResults) > 0 {
		ctx.ToolResults = dedupeToolResults(toolResults)
	}
	if len(tools) > 0 {
		ctx.Tools = tools
	}
	if len(ctx.ToolResults) > 0 || len(ctx.Tools) > 0 {
		msg.UserInputMessageContext = ctx
	}
	return Turn{UserInputMessage: msg}
}

func buildAssistantTurn(msg gjson.Result, toolNameMaps map[string]string) Turn {
	content, toolUses := extractAssistantContent(msg, toolNameMaps)
	assistant := &AssistantResponseMessage{Content: content}
	if len(toolUses) > 0 {
		assistant.ToolUses = toolUses
	}
	return Turn{AssistantResponseMessage: assistant}
}

func historyEndsWithAssistant(history []Turn) bool {
	return len(history) > 0 && history[len(history)-1].AssistantResponseMessage != nil
}

func contentText(msg gjson.Result) string {
	content := msg.Get("content")
	if content.IsArray() {
		var parts []string
		for _, block := range content.Array() {
			switch block.Get("type").String() {
			case "text":
				parts = append(parts, block.Get("text").String())
			case "tool_result":
				parts = append(parts, extractToolResultText(block.Get("content")))
			}
		}
		return strings.Join(parts, "")
	}
	return content.String()
}

func joinNonEmpty(parts ...string) string {
	var filtered []string
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			filtered = append(filtered, part)
		}
	}
	return strings.Join(filtered, "\n\n")
}

func dedupeToolResults(results []ToolResult) []ToolResult {
	if len(results) <= 1 {
		return results
	}
	seen := make(map[string]struct{}, len(results))
	out := make([]ToolResult, 0, len(results))
	for _, result := range results {
		if result.ToolUseID == "" {
			out = append(out, result)
			continue
		}
		if _, ok := seen[result.ToolUseID]; ok {
			continue
		}
		seen[result.ToolUseID] = struct{}{}
		out = append(out, result)
	}
	return out
}

func extractUserContent(msg gjson.Result) (string, []Image, []ToolResult) {
	return extractUserContentWithImagePolicy(msg, true)
}

func extractUserContentForHistory(msg gjson.Result, keepImages bool) (string, []Image, []ToolResult) {
	return extractUserContentWithImagePolicy(msg, keepImages)
}

func extractUserContentWithImagePolicy(msg gjson.Result, keepImages bool) (string, []Image, []ToolResult) {
	content := msg.Get("content")
	var text string
	var images []Image
	var toolResults []ToolResult
	imageCount := 0

	if content.IsArray() {
		for _, block := range content.Array() {
			switch block.Get("type").String() {
			case "text":
				text += block.Get("text").String()
			case "image":
				mediaType := block.Get("source.media_type").String()
				data := block.Get("source.data").String()
				format := "png"
				if strings.Contains(mediaType, "jpeg") || strings.Contains(mediaType, "jpg") {
					format = "jpeg"
				} else if strings.Contains(mediaType, "gif") {
					format = "gif"
				} else if strings.Contains(mediaType, "webp") {
					format = "webp"
				}
				if keepImages {
					images = append(images, Image{
						Format: format,
						Source: ImageSource{Bytes: data},
					})
				} else {
					imageCount++
				}
			case "tool_result":
				toolUseID := block.Get("tool_use_id").String()
				resultContent := extractToolResultText(block.Get("content"))
				toolResults = append(toolResults, ToolResult{
					ToolUseID: toolUseID,
					Content:   []ToolResultContent{{Text: resultContent}},
					Status:    "success",
				})
			}
		}
	} else {
		text = content.String()
	}

	if imageCount > 0 {
		placeholder := fmt.Sprintf("[This message contained %d image(s), omitted from older history]", imageCount)
		text = joinNonEmpty(text, placeholder)
	}

	return text, images, toolResults
}

func extractToolResultText(content gjson.Result) string {
	if content.IsArray() {
		var parts []string
		for _, part := range content.Array() {
			if part.Get("type").String() == "text" {
				parts = append(parts, part.Get("text").String())
			}
		}
		return strings.Join(parts, "\n")
	}
	if content.IsObject() {
		return content.Raw
	}
	return content.String()
}

func extractAssistantContent(msg gjson.Result, toolNameMaps map[string]string) (string, []ToolUse) {
	content := msg.Get("content")
	var text string
	var toolUses []ToolUse

	if content.IsArray() {
		for _, block := range content.Array() {
			switch block.Get("type").String() {
			case "text":
				if text != "" {
					text += "\n"
				}
				text += block.Get("text").String()
			case "thinking":
				thinkingText := block.Get("thinking").String()
				if thinkingText != "" {
					text += "<thinking>" + thinkingText + "</thinking>\n\n"
				}
			case "tool_use":
				name := block.Get("name").String()
				id := block.Get("id").String()
				toolUses = append(toolUses, ToolUse{
					ToolUseID: id,
					Name:      ShortenToolName(name),
					Input:     sanitizeToolInput(block.Get("input")),
				})
			}
		}
	} else {
		text = content.String()
	}

	return text, toolUses
}

func sanitizeToolInput(input gjson.Result) any {
	if !input.Exists() || input.Raw == "" || input.Raw == "null" {
		return nil
	}
	if !input.IsObject() {
		return gjsonValue(input)
	}

	var raw map[string]any
	if err := json.Unmarshal([]byte(input.Raw), &raw); err != nil {
		return map[string]any{}
	}
	sanitized := make(map[string]any, len(raw))
	for key, value := range raw {
		if key == "" {
			continue
		}
		sanitized[key] = value
	}
	return sanitized
}

// RestoreToolName restores a shortened tool name to its original.
func RestoreToolName(shortened string, nameMap map[string]string) string {
	if original, ok := nameMap[shortened]; ok {
		return original
	}
	return shortened
}
