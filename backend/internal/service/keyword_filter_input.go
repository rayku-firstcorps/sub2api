package service

import (
	"strings"

	"github.com/tidwall/gjson"
)

type KeywordFilterTextSegment struct {
	Text         string `json:"text"`
	Protocol     string `json:"protocol"`
	SegmentIndex int    `json:"segment_index"`
	MessageIndex int    `json:"message_index"`
	PartIndex    int    `json:"part_index"`
}

func ExtractKeywordFilterTexts(protocol string, body []byte) []string {
	segments := ExtractKeywordFilterSegments(protocol, body)
	out := make([]string, 0, len(segments))
	for _, segment := range segments {
		out = append(out, segment.Text)
	}
	return out
}

func ExtractKeywordFilterSegments(protocol string, body []byte) []KeywordFilterTextSegment {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return nil
	}
	collector := &keywordFilterSegmentCollector{protocol: protocol}
	switch protocol {
	case ContentModerationProtocolAnthropicMessages:
		collectAllAnthropicUserMessageSegments(gjson.GetBytes(body, "messages"), collector)
	case ContentModerationProtocolOpenAIChat:
		collectAllRoleMessageSegments(gjson.GetBytes(body, "messages"), "user", collector)
	case ContentModerationProtocolOpenAIResponses:
		collectAllResponsesInputSegments(gjson.GetBytes(body, "input"), collector)
	case ContentModerationProtocolGemini:
		collectAllGeminiUserContentSegments(gjson.GetBytes(body, "contents"), collector)
	case ContentModerationProtocolOpenAIImages:
		return nil
	default:
		collectAllResponsesInputSegments(gjson.GetBytes(body, "input"), collector)
		collectAllRoleMessageSegments(gjson.GetBytes(body, "messages"), "user", collector)
		collectAllGeminiUserContentSegments(gjson.GetBytes(body, "contents"), collector)
	}
	return collector.segments
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

type keywordFilterSegmentCollector struct {
	protocol string
	segments []KeywordFilterTextSegment
}

func (c *keywordFilterSegmentCollector) add(text string, messageIndex int, partIndex int) {
	if c == nil {
		return
	}
	text = strings.TrimSpace(text)
	if text == "" || strings.Contains(text, "<system-reminder>") {
		return
	}
	c.segments = append(c.segments, KeywordFilterTextSegment{
		Text:         text,
		Protocol:     c.protocol,
		SegmentIndex: len(c.segments),
		MessageIndex: messageIndex,
		PartIndex:    partIndex,
	})
}

func collectAllRoleMessageSegments(messages gjson.Result, role string, collector *keywordFilterSegmentCollector) {
	if !messages.IsArray() {
		return
	}
	messageIndex := 0
	messages.ForEach(func(_, msg gjson.Result) bool {
		currentIndex := messageIndex
		messageIndex++
		if strings.ToLower(strings.TrimSpace(msg.Get("role").String())) == role {
			collectKeywordTextContentSegments(msg.Get("content"), collector, currentIndex, -1)
		}
		return true
	})
}

func collectAllAnthropicUserMessageSegments(messages gjson.Result, collector *keywordFilterSegmentCollector) {
	if !messages.IsArray() {
		return
	}
	messageIndex := 0
	messages.ForEach(func(_, msg gjson.Result) bool {
		currentIndex := messageIndex
		messageIndex++
		if strings.ToLower(strings.TrimSpace(msg.Get("role").String())) == "user" {
			collectAnthropicKeywordContentSegments(msg.Get("content"), collector, currentIndex, -1)
		}
		return true
	})
}

func collectAnthropicKeywordContentSegments(value gjson.Result, collector *keywordFilterSegmentCollector, messageIndex int, partIndex int) {
	switch {
	case !value.Exists():
		return
	case value.Type == gjson.String:
		if !isAnthropicSystemReminderText(value.String()) {
			collector.add(value.String(), messageIndex, partIndex)
		}
	case value.IsArray():
		index := 0
		value.ForEach(func(_, item gjson.Result) bool {
			currentPart := partIndex
			if partIndex < 0 {
				currentPart = index
			}
			index++
			collectAnthropicKeywordContentSegments(item, collector, messageIndex, currentPart)
			return true
		})
	case value.IsObject():
		typ := strings.ToLower(strings.TrimSpace(value.Get("type").String()))
		switch typ {
		case "", "text", "input_text", "message":
			if value.Get("text").Exists() && !isAnthropicSystemReminderText(value.Get("text").String()) {
				collector.add(value.Get("text").String(), messageIndex, partIndex)
			}
			if value.Get("content").Exists() {
				collectAnthropicKeywordContentSegments(value.Get("content"), collector, messageIndex, partIndex)
			}
		}
	}
}

func collectAllResponsesInputSegments(input gjson.Result, collector *keywordFilterSegmentCollector) {
	switch {
	case !input.Exists():
		return
	case input.Type == gjson.String:
		collector.add(input.String(), -1, -1)
	case input.IsArray():
		messageIndex := 0
		input.ForEach(func(_, item gjson.Result) bool {
			currentIndex := messageIndex
			messageIndex++
			if isResponsesUserTextItem(item) {
				collectResponsesKeywordItemTextSegments(item, collector, currentIndex)
			}
			return true
		})
	case input.IsObject():
		if isResponsesUserTextItem(input) {
			collectResponsesKeywordItemTextSegments(input, collector, 0)
		}
	}
}

func collectResponsesKeywordItemTextSegments(item gjson.Result, collector *keywordFilterSegmentCollector, messageIndex int) {
	if item.Get("type").String() == "input_text" || item.Get("text").Exists() {
		collectKeywordTextContentSegments(item, collector, messageIndex, -1)
		return
	}
	collectKeywordTextContentSegments(item.Get("content"), collector, messageIndex, -1)
}

func collectAllGeminiUserContentSegments(contents gjson.Result, collector *keywordFilterSegmentCollector) {
	if !contents.IsArray() {
		return
	}
	messageIndex := 0
	contents.ForEach(func(_, content gjson.Result) bool {
		currentIndex := messageIndex
		messageIndex++
		role := strings.ToLower(strings.TrimSpace(content.Get("role").String()))
		if role == "" || role == "user" {
			if arr := content.Get("parts"); arr.IsArray() {
				partIndex := 0
				arr.ForEach(func(_, part gjson.Result) bool {
					collector.add(part.Get("text").String(), currentIndex, partIndex)
					partIndex++
					return true
				})
			}
		}
		return true
	})
}

func collectKeywordTextContentSegments(value gjson.Result, collector *keywordFilterSegmentCollector, messageIndex int, partIndex int) {
	switch {
	case !value.Exists():
		return
	case value.Type == gjson.String:
		collector.add(value.String(), messageIndex, partIndex)
	case value.IsArray():
		index := 0
		value.ForEach(func(_, item gjson.Result) bool {
			currentPart := partIndex
			if partIndex < 0 {
				currentPart = index
			}
			index++
			collectKeywordTextContentSegments(item, collector, messageIndex, currentPart)
			return true
		})
	case value.IsObject():
		typ := strings.ToLower(strings.TrimSpace(value.Get("type").String()))
		switch typ {
		case "", "text", "input_text", "message":
			if value.Get("text").Exists() {
				collector.add(value.Get("text").String(), messageIndex, partIndex)
			}
			if value.Get("content").Exists() {
				collectKeywordTextContentSegments(value.Get("content"), collector, messageIndex, partIndex)
			}
		}
	}
}
