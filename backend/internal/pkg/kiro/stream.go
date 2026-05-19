package kiro

import (
	"bytes"
	"encoding/json"
	"fmt"
)

type EventType int

const (
	EventText EventType = iota
	EventToolStart
	EventToolInput
	EventStop
	EventError
	EventThinkingStart
	EventThinkingDelta
	EventThinkingStop
)

type ParsedEvent struct {
	Type      EventType
	Content   string
	ToolUseID string
	ToolName  string
	Error     string
}

type StreamParser struct {
	buffer  []byte
	depth   int
	inStr   bool
	escaped bool
	start   int
}

func NewStreamParser() *StreamParser {
	return &StreamParser{}
}

func (p *StreamParser) Feed(data []byte) []StreamEvent {
	var events []StreamEvent
	p.buffer = append(p.buffer, data...)

	for {
		start := bytes.IndexByte(p.buffer, '{')
		if start < 0 {
			p.buffer = nil
			return events
		}

		end, complete := findJSONObjectEnd(p.buffer[start:])
		if !complete {
			p.buffer = p.buffer[start:]
			return events
		}

		obj := p.buffer[start : start+end+1]
		var evt StreamEvent
		if err := json.Unmarshal(obj, &evt); err == nil {
			events = append(events, evt)
			p.buffer = p.buffer[start+end+1:]
			continue
		}

		// AWS event-stream frames contain binary headers. If a header byte happens
		// to look like "{", skip it and continue searching for the JSON payload.
		p.buffer = p.buffer[start+1:]
	}
}

func (p *StreamParser) Reset() {
	p.buffer = nil
	p.depth = 0
	p.inStr = false
	p.escaped = false
	p.start = 0
}

func findJSONObjectEnd(data []byte) (int, bool) {
	depth := 0
	inStr := false
	escaped := false

	for i, ch := range data {
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && inStr {
			escaped = true
			continue
		}
		if ch == '"' {
			inStr = !inStr
			continue
		}
		if inStr {
			continue
		}

		switch ch {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}

	return 0, false
}

type AnthropicSSEEvent struct {
	Event string
	Data  string
}

type StreamConverter struct {
	toolNameMap   map[string]string
	nextIdx       int
	contentIdx    int
	toolIdx       int
	thinkingIdx   int
	inThinking    bool
	inContent     bool
	inTool        bool
	inputAccum    string
	currentToolID string
	outputTokens  int
	stopped       bool
	emittedTool   bool
}

func NewStreamConverter(toolNameMap map[string]string) *StreamConverter {
	return &StreamConverter{
		toolNameMap: toolNameMap,
		contentIdx:  -1,
		toolIdx:     -1,
		thinkingIdx: -1,
	}
}

func (c *StreamConverter) Convert(evt StreamEvent) []AnthropicSSEEvent {
	var events []AnthropicSSEEvent
	consumedInput := false

	if evt.Error != "" || evt.ErrorCode != "" {
		msg := evt.Error
		if msg == "" {
			msg = evt.ErrorMessage
		}
		events = append(events, AnthropicSSEEvent{
			Event: "error",
			Data:  fmt.Sprintf(`{"type":"error","error":{"type":"api_error","message":%s}}`, jsonStr(msg)),
		})
		return events
	}

	if evt.Content != "" {
		for _, block := range ParseKiroContentBlocks(evt.Content, c.toolNameMap) {
			c.appendParsedContentBlock(&events, block)
		}
	}

	if evt.Name != "" && evt.ToolUseID != "" {
		if !c.inTool || c.currentToolID != evt.ToolUseID {
			c.closeContent(&events)
			c.closeThinking(&events)
			c.closeTool(&events)
			c.toolIdx = c.nextIdx
			c.nextIdx++
			c.inTool = true
			c.emittedTool = true
			c.currentToolID = evt.ToolUseID
			c.inputAccum = ""
			toolName := RestoreToolName(evt.Name, c.toolNameMap)
			events = append(events, AnthropicSSEEvent{
				Event: "content_block_start",
				Data:  fmt.Sprintf(`{"type":"content_block_start","index":%d,"content_block":{"type":"tool_use","id":%s,"name":%s,"input":{}}}`, c.toolIdx, jsonStr(evt.ToolUseID), jsonStr(toolName)),
			})
		}
		if evt.Input != "" && c.inTool {
			c.appendToolInput(&events, string(evt.Input))
			consumedInput = true
		}
	}

	if evt.Input != "" && c.inTool && !consumedInput {
		c.appendToolInput(&events, string(evt.Input))
	}

	if evt.Stop != nil && *evt.Stop {
		stopReason := "end_turn"
		if c.inTool {
			stopReason = "tool_use"
		}
		events = append(events, c.Finish(stopReason)...)
	}

	return events
}

func (c *StreamConverter) appendToolInput(events *[]AnthropicSSEEvent, input string) {
	c.inputAccum += input
	*events = append(*events, AnthropicSSEEvent{
		Event: "content_block_delta",
		Data:  fmt.Sprintf(`{"type":"content_block_delta","index":%d,"delta":{"type":"input_json_delta","partial_json":%s}}`, c.toolIdx, jsonStr(input)),
	})
}

func (c *StreamConverter) SetOutputTokens(tokens int) {
	c.outputTokens = tokens
}

func (c *StreamConverter) Finish(stopReason string) []AnthropicSSEEvent {
	if c.stopped {
		return nil
	}
	if stopReason == "" {
		stopReason = "end_turn"
	}
	if stopReason == "end_turn" && c.emittedTool {
		stopReason = "tool_use"
	}

	var events []AnthropicSSEEvent
	c.closeContent(&events)
	c.closeThinking(&events)
	c.closeTool(&events)
	events = append(events, AnthropicSSEEvent{
		Event: "message_delta",
		Data:  fmt.Sprintf(`{"type":"message_delta","delta":{"stop_reason":"%s","stop_sequence":null},"usage":{"output_tokens":%d}}`, stopReason, c.outputTokens),
	})
	events = append(events, AnthropicSSEEvent{
		Event: "message_stop",
		Data:  `{"type":"message_stop"}`,
	})
	c.stopped = true
	return events
}

func (c *StreamConverter) closeContent(events *[]AnthropicSSEEvent) {
	if c.inContent {
		*events = append(*events, AnthropicSSEEvent{
			Event: "content_block_stop",
			Data:  fmt.Sprintf(`{"type":"content_block_stop","index":%d}`, c.contentIdx),
		})
		c.inContent = false
	}
}

func (c *StreamConverter) closeThinking(events *[]AnthropicSSEEvent) {
	if c.inThinking {
		*events = append(*events, AnthropicSSEEvent{
			Event: "content_block_stop",
			Data:  fmt.Sprintf(`{"type":"content_block_stop","index":%d}`, c.thinkingIdx),
		})
		c.inThinking = false
	}
}

func (c *StreamConverter) closeTool(events *[]AnthropicSSEEvent) {
	if c.inTool {
		*events = append(*events, AnthropicSSEEvent{
			Event: "content_block_stop",
			Data:  fmt.Sprintf(`{"type":"content_block_stop","index":%d}`, c.toolIdx),
		})
		c.inTool = false
	}
}

func (c *StreamConverter) appendParsedContentBlock(events *[]AnthropicSSEEvent, block ParsedContentBlock) {
	switch block.Type {
	case "thinking":
		if block.Thinking == "" {
			return
		}
		c.closeContent(events)
		c.closeTool(events)
		c.thinkingIdx = c.nextIdx
		c.nextIdx++
		c.inThinking = true
		*events = append(*events, AnthropicSSEEvent{
			Event: "content_block_start",
			Data:  fmt.Sprintf(`{"type":"content_block_start","index":%d,"content_block":{"type":"thinking","thinking":""}}`, c.thinkingIdx),
		})
		*events = append(*events, AnthropicSSEEvent{
			Event: "content_block_delta",
			Data:  fmt.Sprintf(`{"type":"content_block_delta","index":%d,"delta":{"type":"thinking_delta","thinking":%s}}`, c.thinkingIdx, jsonStr(block.Thinking)),
		})
		c.closeThinking(events)
	case "tool_use":
		c.closeContent(events)
		c.closeThinking(events)
		c.closeTool(events)
		c.toolIdx = c.nextIdx
		c.nextIdx++
		c.inTool = true
		c.emittedTool = true
		c.currentToolID = block.ToolUseID
		c.inputAccum = ""
		inputJSON, _ := json.Marshal(block.ToolInput)
		*events = append(*events, AnthropicSSEEvent{
			Event: "content_block_start",
			Data:  fmt.Sprintf(`{"type":"content_block_start","index":%d,"content_block":{"type":"tool_use","id":%s,"name":%s,"input":{}}}`, c.toolIdx, jsonStr(block.ToolUseID), jsonStr(block.ToolName)),
		})
		*events = append(*events, AnthropicSSEEvent{
			Event: "content_block_delta",
			Data:  fmt.Sprintf(`{"type":"content_block_delta","index":%d,"delta":{"type":"input_json_delta","partial_json":%s}}`, c.toolIdx, jsonStr(string(inputJSON))),
		})
		c.closeTool(events)
	default:
		if block.Text == "" {
			return
		}
		if !c.inContent {
			c.closeThinking(events)
			c.closeTool(events)
			c.contentIdx = c.nextIdx
			c.nextIdx++
			c.inContent = true
			*events = append(*events, AnthropicSSEEvent{
				Event: "content_block_start",
				Data:  fmt.Sprintf(`{"type":"content_block_start","index":%d,"content_block":{"type":"text","text":""}}`, c.contentIdx),
			})
		}
		*events = append(*events, AnthropicSSEEvent{
			Event: "content_block_delta",
			Data:  fmt.Sprintf(`{"type":"content_block_delta","index":%d,"delta":{"type":"text_delta","text":%s}}`, c.contentIdx, jsonStr(block.Text)),
		})
	}
}

func (c *StreamConverter) StopReason() string {
	if c.emittedTool || (c.toolIdx >= 0 && c.inTool) {
		return "tool_use"
	}
	return "end_turn"
}

func jsonStr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func FormatSSE(event AnthropicSSEEvent) []byte {
	return []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", event.Event, event.Data))
}
