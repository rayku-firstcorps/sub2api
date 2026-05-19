package kiro

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

const (
	thinkingStartTag = "<thinking>"
	thinkingEndTag   = "</thinking>"
)

type ParsedContentBlock struct {
	Type      string
	Text      string
	Thinking  string
	ToolUseID string
	ToolName  string
	ToolInput any
}

func ParseKiroContentBlocks(text string, toolNameMap map[string]string) []ParsedContentBlock {
	if text == "" {
		return nil
	}
	segments := parseThinkingSegments(text, toolNameMap)
	var blocks []ParsedContentBlock
	for _, segment := range segments {
		if segment.Type != "text" {
			blocks = append(blocks, segment)
			continue
		}
		blocks = append(blocks, parseBracketToolCallBlocks(segment.Text, toolNameMap)...)
	}
	if len(blocks) == 0 {
		return []ParsedContentBlock{{Type: "text", Text: text}}
	}
	return blocks
}

func parseThinkingSegments(text string, toolNameMap map[string]string) []ParsedContentBlock {
	start := findRealTag(text, thinkingStartTag, 0)
	if start < 0 {
		return []ParsedContentBlock{{Type: "text", Text: text}}
	}

	var blocks []ParsedContentBlock
	before := text[:start]
	if strings.TrimSpace(before) != "" {
		blocks = append(blocks, ParsedContentBlock{Type: "text", Text: before})
	}

	rest := text[start+len(thinkingStartTag):]
	rest = strings.TrimPrefix(strings.TrimPrefix(rest, "\r\n"), "\n")

	end := findRealThinkingEndTag(rest, 0)
	if end < 0 {
		end = findRealThinkingEndTagAtBufferEnd(rest, 0)
	}
	if end < 0 {
		if strings.TrimSpace(rest) != "" {
			blocks = append(blocks, ParsedContentBlock{Type: "thinking", Thinking: rest})
		}
		return blocks
	}

	thinking := rest[:end]
	after := rest[end+len(thinkingEndTag):]
	after = strings.TrimPrefix(after, "\n\n")
	blocks = append(blocks, ParsedContentBlock{Type: "thinking", Thinking: thinking})
	if strings.TrimSpace(after) != "" {
		blocks = append(blocks, ParseKiroContentBlocks(after, toolNameMap)...)
	}
	return blocks
}

func parseBracketToolCallBlocks(text string, toolNameMap map[string]string) []ParsedContentBlock {
	if !strings.Contains(text, "[Called") {
		if strings.TrimSpace(text) == "" {
			return nil
		}
		return []ParsedContentBlock{{Type: "text", Text: text}}
	}

	var blocks []ParsedContentBlock
	cursor := 0
	for cursor < len(text) {
		start := strings.Index(text[cursor:], "[Called")
		if start < 0 {
			remaining := text[cursor:]
			if strings.TrimSpace(remaining) != "" {
				blocks = append(blocks, ParsedContentBlock{Type: "text", Text: remaining})
			}
			break
		}
		start += cursor
		prefix := text[cursor:start]
		if strings.TrimSpace(prefix) != "" {
			blocks = append(blocks, ParsedContentBlock{Type: "text", Text: prefix})
		}

		end := findMatchingBracket(text, start, '[', ']')
		if end < 0 {
			remaining := text[start:]
			if strings.TrimSpace(remaining) != "" {
				blocks = append(blocks, ParsedContentBlock{Type: "text", Text: remaining})
			}
			break
		}

		callText := text[start : end+1]
		if block, ok := parseSingleBracketToolCall(callText, toolNameMap); ok {
			blocks = append(blocks, block)
		} else if strings.TrimSpace(callText) != "" {
			blocks = append(blocks, ParsedContentBlock{Type: "text", Text: callText})
		}
		cursor = end + 1
	}
	return blocks
}

var bracketToolNamePattern = regexp.MustCompile(`(?i)^\[Called\s+(\w+)\s+with\s+args:`)

func parseSingleBracketToolCall(text string, toolNameMap map[string]string) (ParsedContentBlock, bool) {
	nameMatch := bracketToolNamePattern.FindStringSubmatch(text)
	if len(nameMatch) < 2 {
		return ParsedContentBlock{}, false
	}
	argsMarker := "with args:"
	argsStart := strings.Index(strings.ToLower(text), argsMarker)
	argsEnd := strings.LastIndex(text, "]")
	if argsStart < 0 || argsEnd <= argsStart {
		return ParsedContentBlock{}, false
	}
	args := strings.TrimSpace(text[argsStart+len(argsMarker) : argsEnd])
	var input any
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		repaired := repairKiroJSON(args)
		if repairErr := json.Unmarshal([]byte(repaired), &input); repairErr != nil {
			return ParsedContentBlock{}, false
		}
	}
	if _, ok := input.(map[string]any); !ok {
		return ParsedContentBlock{}, false
	}
	name := RestoreToolName(nameMatch[1], toolNameMap)
	return ParsedContentBlock{
		Type:      "tool_use",
		ToolUseID: "call_" + strings.ReplaceAll(uuid.New().String(), "-", "")[:8],
		ToolName:  name,
		ToolInput: input,
	}, true
}

func repairKiroJSON(value string) string {
	trailingComma := regexp.MustCompile(`,\s*([}\]])`)
	unquotedKey := regexp.MustCompile(`([{,]\s*)([a-zA-Z0-9_]+?)\s*:`)
	value = trailingComma.ReplaceAllString(value, `$1`)
	value = unquotedKey.ReplaceAllString(value, `$1"$2":`)
	return quoteBareKiroJSONValues(value)
}

func quoteBareKiroJSONValues(value string) string {
	var b strings.Builder
	for i := 0; i < len(value); i++ {
		ch := value[i]
		b.WriteByte(ch)
		if ch != ':' {
			continue
		}
		for i+1 < len(value) && (value[i+1] == ' ' || value[i+1] == '\t' || value[i+1] == '\n' || value[i+1] == '\r') {
			i++
			b.WriteByte(value[i])
		}
		start := i + 1
		if start >= len(value) || !isBareJSONValueChar(value[start]) {
			continue
		}
		end := start
		for end < len(value) && isBareJSONValueChar(value[end]) {
			end++
		}
		next := end
		for next < len(value) && (value[next] == ' ' || value[next] == '\t' || value[next] == '\n' || value[next] == '\r') {
			next++
		}
		word := value[start:end]
		if next < len(value) && (value[next] == ',' || value[next] == '}' || value[next] == ']') && !isJSONLiteral(word) {
			b.WriteByte('"')
			b.WriteString(word)
			b.WriteByte('"')
			i = end - 1
		}
	}
	return b.String()
}

func isBareJSONValueChar(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '-' || ch == '.'
}

func isJSONLiteral(value string) bool {
	return value == "true" || value == "false" || value == "null" || regexp.MustCompile(`^-?\d+(\.\d+)?$`).MatchString(value)
}

func findRealTag(text, tag string, startIndex int) int {
	searchStart := startIndex
	if searchStart < 0 {
		searchStart = 0
	}
	for {
		pos := strings.Index(text[searchStart:], tag)
		if pos < 0 {
			return -1
		}
		pos += searchStart
		if !isQuoteCharAt(text, pos-1) && !isQuoteCharAt(text, pos+len(tag)) {
			return pos
		}
		searchStart = pos + 1
	}
}

func isQuoteCharAt(text string, index int) bool {
	if index < 0 || index >= len(text) {
		return false
	}
	return text[index] == '"' || text[index] == '\'' || text[index] == '`'
}

func findRealThinkingEndTag(text string, startIndex int) int {
	searchStart := startIndex
	if searchStart < 0 {
		searchStart = 0
	}
	for {
		pos := findRealTag(text, thinkingEndTag, searchStart)
		if pos < 0 {
			return -1
		}
		if strings.HasPrefix(text[pos+len(thinkingEndTag):], "\n\n") {
			return pos
		}
		searchStart = pos + 1
	}
}

func findRealThinkingEndTagAtBufferEnd(text string, startIndex int) int {
	searchStart := startIndex
	if searchStart < 0 {
		searchStart = 0
	}
	for {
		pos := findRealTag(text, thinkingEndTag, searchStart)
		if pos < 0 {
			return -1
		}
		if strings.TrimSpace(text[pos+len(thinkingEndTag):]) == "" {
			return pos
		}
		searchStart = pos + 1
	}
}

func findMatchingBracket(text string, startPos int, openChar, closeChar byte) int {
	if text == "" || startPos >= len(text) || text[startPos] != openChar {
		return -1
	}
	depth := 1
	inString := false
	escaped := false
	for i := startPos + 1; i < len(text); i++ {
		ch := text[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && inString {
			escaped = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if ch == openChar {
			depth++
		} else if ch == closeChar {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}
