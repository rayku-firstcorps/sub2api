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
	if len(result) == 0 {
		return []ToolSpecWrapper{placeholderToolSpec()}
	}
	return result
}

func convertTool(tool gjson.Result) (ToolSpecWrapper, bool) {
	if !isKiroCompatibleTool(tool) {
		return ToolSpecWrapper{}, false
	}

	name := tool.Get("name").String()
	desc := tool.Get("description").String()
	schemaResult := tool.Get("input_schema")
	if tool.Get("type").String() == "custom" {
		desc = tool.Get("custom.description").String()
		schemaResult = tool.Get("custom.input_schema")
	}
	if len(desc) > MaxToolDescLen {
		desc = desc[:MaxToolDescLen] + "..."
	}

	schema := normalizeKiroInputSchema(schemaResult)

	return ToolSpecWrapper{
		ToolSpecification: ToolSpecification{
			Name:        ShortenToolName(name),
			Description: desc,
			InputSchema: InputSchema{JSON: schema},
		},
	}, true
}

func placeholderToolSpec() ToolSpecWrapper {
	return ToolSpecWrapper{
		ToolSpecification: ToolSpecification{
			Name:        "no_tool_available",
			Description: "This is a placeholder tool when no other tools are available. It does nothing.",
			InputSchema: InputSchema{JSON: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			}},
		},
	}
}

func isIgnoredKiroTool(tool gjson.Result) bool {
	name := strings.ToLower(tool.Get("name").String())
	return name == "web_search" || name == "websearch"
}

func isKiroCompatibleTool(tool gjson.Result) bool {
	if tool.Get("name").String() == "" {
		return false
	}
	desc := tool.Get("description").String()
	schema := tool.Get("input_schema")
	if tool.Get("type").String() == "custom" {
		desc = tool.Get("custom.description").String()
		schema = tool.Get("custom.input_schema")
	}
	if strings.TrimSpace(desc) == "" {
		return false
	}
	return !schema.Exists() || schema.IsObject()
}

func normalizeKiroInputSchema(schemaResult gjson.Result) map[string]any {
	var schema map[string]any
	if schemaResult.Exists() && schemaResult.IsObject() {
		_ = json.Unmarshal([]byte(schemaResult.Raw), &schema)
	}
	if schema == nil {
		schema = map[string]any{}
	}
	defs := extractKiroSchemaDefs(schema)
	normalized, ok := normalizeKiroSchemaValue(schema, defs).(map[string]any)
	if !ok || normalized == nil {
		normalized = map[string]any{}
	}
	ensureKiroObjectSchema(normalized)
	return normalized
}

func normalizeKiroSchemaValue(value any, defs map[string]any) any {
	switch v := value.(type) {
	case map[string]any:
		return normalizeKiroSchemaMap(v, defs)
	case []any:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, normalizeKiroSchemaValue(item, defs))
		}
		return out
	default:
		return value
	}
}

func normalizeKiroSchemaMap(schema map[string]any, defs map[string]any) map[string]any {
	if ref, ok := schema["$ref"].(string); ok {
		if resolved, found := resolveLocalKiroRef(ref, schema, defs); found {
			for k, v := range resolved {
				if _, exists := schema[k]; !exists {
					schema[k] = deepCopyJSON(v)
				}
			}
		}
	}

	if merged := bestKiroUnionBranch(schema); merged != nil {
		for k, v := range merged {
			if k == "properties" {
				dst, _ := schema["properties"].(map[string]any)
				if dst == nil {
					dst = map[string]any{}
					schema["properties"] = dst
				}
				if src, ok := v.(map[string]any); ok {
					for pk, pv := range src {
						if _, exists := dst[pk]; !exists {
							dst[pk] = deepCopyJSON(pv)
						}
					}
				}
				continue
			}
			if _, exists := schema[k]; !exists {
				schema[k] = deepCopyJSON(v)
			}
		}
	}

	if allOf, ok := schema["allOf"].([]any); ok {
		for _, branch := range allOf {
			branchMap, ok := branch.(map[string]any)
			if !ok {
				continue
			}
			branchMap = normalizeKiroSchemaMap(branchMap, defs)
			for k, v := range branchMap {
				if k == "properties" {
					dst, _ := schema["properties"].(map[string]any)
					if dst == nil {
						dst = map[string]any{}
						schema["properties"] = dst
					}
					if src, ok := v.(map[string]any); ok {
						for pk, pv := range src {
							if _, exists := dst[pk]; !exists {
								dst[pk] = pv
							}
						}
					}
					continue
				}
				if _, exists := schema[k]; !exists {
					schema[k] = v
				}
			}
		}
	}

	if props, ok := schema["properties"].(map[string]any); ok {
		for key, prop := range props {
			props[key] = normalizeKiroSchemaValue(prop, defs)
		}
	}
	if items, ok := schema["items"]; ok {
		if itemArray, ok := items.([]any); ok {
			schema["items"] = normalizeKiroSchemaValue(selectBestKiroSchema(itemArray), defs)
		} else {
			schema["items"] = normalizeKiroSchemaValue(items, defs)
		}
	}

	migrateKiroSchemaConstraints(schema)
	for key := range schema {
		if !isAllowedKiroSchemaField(key) {
			delete(schema, key)
		}
	}
	normalizeKiroSchemaType(schema)
	normalizeKiroEnum(schema)
	filterKiroRequired(schema)
	if schema["type"] == "object" {
		if _, ok := schema["properties"].(map[string]any); !ok {
			schema["properties"] = map[string]any{}
		}
	}
	return schema
}

func ensureKiroObjectSchema(schema map[string]any) {
	if _, ok := schema["type"]; !ok {
		schema["type"] = "object"
	}
	if schema["type"] == "object" {
		if _, ok := schema["properties"].(map[string]any); !ok {
			schema["properties"] = map[string]any{}
		}
	}
	filterKiroRequired(schema)
}

func isAllowedKiroSchemaField(key string) bool {
	switch key {
	case "type", "description", "properties", "required", "items", "enum", "title":
		return true
	default:
		return false
	}
}

func normalizeKiroSchemaType(schema map[string]any) {
	if raw, exists := schema["type"]; exists {
		switch typed := raw.(type) {
		case string:
			lower := strings.ToLower(strings.TrimSpace(typed))
			if lower == "" || lower == "null" {
				lower = inferKiroSchemaType(schema)
			}
			schema["type"] = lower
		case []any:
			selected := ""
			nullable := false
			for _, item := range typed {
				part, ok := item.(string)
				if !ok {
					continue
				}
				lower := strings.ToLower(strings.TrimSpace(part))
				if lower == "null" {
					nullable = true
					continue
				}
				if selected == "" {
					selected = lower
				}
			}
			if selected == "" {
				selected = inferKiroSchemaType(schema)
			}
			schema["type"] = selected
			if nullable {
				appendKiroDescription(schema, "(nullable)")
			}
		default:
			schema["type"] = inferKiroSchemaType(schema)
		}
		return
	}
	schema["type"] = inferKiroSchemaType(schema)
}

func inferKiroSchemaType(schema map[string]any) string {
	if _, ok := schema["properties"]; ok {
		return "object"
	}
	if _, ok := schema["items"]; ok {
		return "array"
	}
	if _, ok := schema["enum"]; ok {
		return "string"
	}
	return "object"
}

func filterKiroRequired(schema map[string]any) {
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		delete(schema, "required")
		return
	}
	req, ok := schema["required"].([]any)
	if !ok {
		return
	}
	filtered := make([]any, 0, len(req))
	seen := map[string]struct{}{}
	for _, item := range req {
		key, ok := item.(string)
		if !ok {
			continue
		}
		if _, exists := props[key]; !exists {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		filtered = append(filtered, key)
	}
	if len(filtered) == 0 {
		delete(schema, "required")
		return
	}
	schema["required"] = filtered
}

func normalizeKiroEnum(schema map[string]any) {
	enumValues, ok := schema["enum"].([]any)
	if !ok {
		return
	}
	hasNonString := false
	for i, value := range enumValues {
		if _, ok := value.(string); ok {
			continue
		}
		hasNonString = true
		if value == nil {
			enumValues[i] = "null"
		} else {
			enumValues[i] = fmt.Sprintf("%v", value)
		}
	}
	if hasNonString {
		schema["type"] = "string"
	}
}

func migrateKiroSchemaConstraints(schema map[string]any) {
	constraints := []struct {
		key   string
		label string
	}{
		{"minLength", "minLen"},
		{"maxLength", "maxLen"},
		{"pattern", "pattern"},
		{"minimum", "min"},
		{"maximum", "max"},
		{"multipleOf", "multipleOf"},
		{"exclusiveMinimum", "exclMin"},
		{"exclusiveMaximum", "exclMax"},
		{"minItems", "minItems"},
		{"maxItems", "maxItems"},
		{"format", "format"},
	}
	var hints []string
	for _, constraint := range constraints {
		if value, ok := schema[constraint.key]; ok && value != nil {
			hints = append(hints, fmt.Sprintf("%s: %v", constraint.label, value))
		}
	}
	if len(hints) > 0 {
		appendKiroDescription(schema, "[Constraint: "+strings.Join(hints, ", ")+"]")
	}
}

func appendKiroDescription(schema map[string]any, suffix string) {
	desc, _ := schema["description"].(string)
	if strings.Contains(desc, suffix) {
		return
	}
	if desc != "" {
		desc += " "
	}
	schema["description"] = desc + suffix
}

func bestKiroUnionBranch(schema map[string]any) map[string]any {
	if branches, ok := schema["anyOf"].([]any); ok {
		if selected, ok := selectBestKiroSchema(branches).(map[string]any); ok {
			return selected
		}
	}
	if branches, ok := schema["oneOf"].([]any); ok {
		if selected, ok := selectBestKiroSchema(branches).(map[string]any); ok {
			return selected
		}
	}
	return nil
}

func selectBestKiroSchema(branches []any) any {
	var selected any
	bestScore := -1
	for _, branch := range branches {
		score := scoreKiroSchema(branch)
		if score > bestScore {
			bestScore = score
			selected = branch
		}
	}
	if selected == nil {
		return map[string]any{"type": "string"}
	}
	return selected
}

func scoreKiroSchema(value any) int {
	schema, ok := value.(map[string]any)
	if !ok {
		return 0
	}
	typeName, _ := schema["type"].(string)
	typeName = strings.ToLower(typeName)
	if _, ok := schema["properties"]; ok || typeName == "object" {
		return 3
	}
	if _, ok := schema["items"]; ok || typeName == "array" {
		return 2
	}
	if typeName != "" && typeName != "null" {
		return 1
	}
	return 0
}

func extractKiroSchemaDefs(schema map[string]any) map[string]any {
	defs := map[string]any{}
	for _, key := range []string{"$defs", "definitions"} {
		if raw, ok := schema[key].(map[string]any); ok {
			for name, value := range raw {
				defs[name] = value
			}
			delete(schema, key)
		}
	}
	return defs
}

func resolveLocalKiroRef(ref string, root map[string]any, defs map[string]any) (map[string]any, bool) {
	if !strings.HasPrefix(ref, "#/") {
		return nil, false
	}
	parts := strings.Split(strings.TrimPrefix(ref, "#/"), "/")
	if len(parts) >= 2 && (parts[0] == "$defs" || parts[0] == "definitions") {
		value, ok := defs[parts[1]]
		if !ok {
			return nil, false
		}
		for _, part := range parts[2:] {
			m, ok := value.(map[string]any)
			if !ok {
				return nil, false
			}
			value, ok = m[part]
			if !ok {
				return nil, false
			}
		}
		m, ok := value.(map[string]any)
		return m, ok
	}
	var current any = root
	for _, part := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = m[part]
		if !ok {
			return nil, false
		}
	}
	m, ok := current.(map[string]any)
	return m, ok
}

func deepCopyJSON(value any) any {
	switch v := value.(type) {
	case map[string]any:
		copied := make(map[string]any, len(v))
		for key, item := range v {
			copied[key] = deepCopyJSON(item)
		}
		return copied
	case []any:
		copied := make([]any, len(v))
		for i, item := range v {
			copied[i] = deepCopyJSON(item)
		}
		return copied
	default:
		return value
	}
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
