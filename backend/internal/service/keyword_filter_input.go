package service

import (
	"strings"

	"github.com/tidwall/gjson"
)

func ExtractKeywordFilterTexts(protocol string, body []byte) []string {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return nil
	}
	var parts []string
	switch protocol {
	case ContentModerationProtocolAnthropicMessages:
		collectAllAnthropicUserMessages(gjson.GetBytes(body, "messages"), &parts)
	case ContentModerationProtocolOpenAIChat:
		collectAllRoleMessages(gjson.GetBytes(body, "messages"), "user", &parts)
	case ContentModerationProtocolOpenAIResponses:
		collectAllResponsesInput(gjson.GetBytes(body, "input"), &parts)
	case ContentModerationProtocolGemini:
		collectAllGeminiUserContent(gjson.GetBytes(body, "contents"), &parts)
	case ContentModerationProtocolOpenAIImages:
		return nil
	default:
		collectAllResponsesInput(gjson.GetBytes(body, "input"), &parts)
		collectAllRoleMessages(gjson.GetBytes(body, "messages"), "user", &parts)
		collectAllGeminiUserContent(gjson.GetBytes(body, "contents"), &parts)
	}
	return normalizeKeywordFilterExtractedTexts(parts)
}

func collectAllRoleMessages(messages gjson.Result, role string, parts *[]string) {
	if !messages.IsArray() {
		return
	}
	messages.ForEach(func(_, msg gjson.Result) bool {
		if strings.ToLower(strings.TrimSpace(msg.Get("role").String())) == role {
			collectKeywordTextContent(msg.Get("content"), parts)
		}
		return true
	})
}

func collectAllAnthropicUserMessages(messages gjson.Result, parts *[]string) {
	if !messages.IsArray() {
		return
	}
	messages.ForEach(func(_, msg gjson.Result) bool {
		if strings.ToLower(strings.TrimSpace(msg.Get("role").String())) == "user" {
			collectAnthropicKeywordContent(msg.Get("content"), parts)
		}
		return true
	})
}

func collectAnthropicKeywordContent(value gjson.Result, parts *[]string) {
	switch {
	case !value.Exists():
		return
	case value.Type == gjson.String:
		if !isAnthropicSystemReminderText(value.String()) {
			addKeywordFilterText(parts, value.String())
		}
	case value.IsArray():
		value.ForEach(func(_, item gjson.Result) bool {
			collectAnthropicKeywordContent(item, parts)
			return true
		})
	case value.IsObject():
		typ := strings.ToLower(strings.TrimSpace(value.Get("type").String()))
		switch typ {
		case "", "text", "input_text", "message":
			if value.Get("text").Exists() && !isAnthropicSystemReminderText(value.Get("text").String()) {
				addKeywordFilterText(parts, value.Get("text").String())
			}
			if value.Get("content").Exists() {
				collectAnthropicKeywordContent(value.Get("content"), parts)
			}
		}
	}
}

func collectAllResponsesInput(input gjson.Result, parts *[]string) {
	switch {
	case !input.Exists():
		return
	case input.Type == gjson.String:
		addKeywordFilterText(parts, input.String())
	case input.IsArray():
		input.ForEach(func(_, item gjson.Result) bool {
			if isResponsesUserTextItem(item) {
				collectResponsesKeywordItemText(item, parts)
			}
			return true
		})
	case input.IsObject():
		if isResponsesUserTextItem(input) {
			collectResponsesKeywordItemText(input, parts)
		}
	}
}

func collectResponsesKeywordItemText(item gjson.Result, parts *[]string) {
	if item.Get("type").String() == "input_text" || item.Get("text").Exists() {
		collectKeywordTextContent(item, parts)
		return
	}
	collectKeywordTextContent(item.Get("content"), parts)
}

func collectAllGeminiUserContent(contents gjson.Result, parts *[]string) {
	if !contents.IsArray() {
		return
	}
	contents.ForEach(func(_, content gjson.Result) bool {
		role := strings.ToLower(strings.TrimSpace(content.Get("role").String()))
		if role == "" || role == "user" {
			if arr := content.Get("parts"); arr.IsArray() {
				arr.ForEach(func(_, part gjson.Result) bool {
					addKeywordFilterText(parts, part.Get("text").String())
					return true
				})
			}
		}
		return true
	})
}

func collectKeywordTextContent(value gjson.Result, parts *[]string) {
	switch {
	case !value.Exists():
		return
	case value.Type == gjson.String:
		addKeywordFilterText(parts, value.String())
	case value.IsArray():
		value.ForEach(func(_, item gjson.Result) bool {
			collectKeywordTextContent(item, parts)
			return true
		})
	case value.IsObject():
		typ := strings.ToLower(strings.TrimSpace(value.Get("type").String()))
		switch typ {
		case "", "text", "input_text", "message":
			if value.Get("text").Exists() {
				addKeywordFilterText(parts, value.Get("text").String())
			}
			if value.Get("content").Exists() {
				collectKeywordTextContent(value.Get("content"), parts)
			}
		}
	}
}

func addKeywordFilterText(parts *[]string, text string) {
	text = strings.TrimSpace(text)
	if text == "" || strings.Contains(text, "<system-reminder>") {
		return
	}
	*parts = append(*parts, text)
}

func normalizeKeywordFilterExtractedTexts(parts []string) []string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
