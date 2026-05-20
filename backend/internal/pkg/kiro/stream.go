package kiro

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"strings"
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
	buffer                 []byte
	depth                  int
	inStr                  bool
	escaped                bool
	start                  int
	invalidJSONSkipped     int
	eventStreamFrames      int
	eventStreamFrameErrors int
	eventStreamSkipped     int
	eventStreamPayloadErrs int
}

func NewStreamParser() *StreamParser {
	return &StreamParser{}
}

func (p *StreamParser) Feed(data []byte) []StreamEvent {
	var events []StreamEvent
	p.buffer = append(p.buffer, data...)

	for {
		if status, msg, frameLen := parseEventStreamFrame(p.buffer); status == eventStreamFrameComplete {
			p.eventStreamFrames++
			if msg.shouldParsePayload() {
				if !appendPayloadEvent(msg.payload, &events) {
					p.eventStreamPayloadErrs++
				}
			} else {
				p.eventStreamSkipped++
			}
			p.buffer = p.buffer[frameLen:]
			continue
		} else if status == eventStreamFrameIncomplete {
			return events
		} else if status == eventStreamFrameInvalid {
			p.eventStreamFrameErrors++
		}

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
		if appendPayloadEvent(obj, &events) {
			p.buffer = p.buffer[start+end+1:]
			continue
		}

		// AWS event-stream frames contain binary headers. If a header byte happens
		// to look like "{", skip it and continue searching for the JSON payload.
		p.invalidJSONSkipped++
		p.buffer = p.buffer[start+1:]
	}
}

func appendPayloadEvent(payload []byte, events *[]StreamEvent) bool {
	var evt StreamEvent
	if err := json.Unmarshal(payload, &evt); err != nil {
		return false
	}
	*events = append(*events, evt)
	return true
}

func (p *StreamParser) Reset() {
	p.buffer = nil
	p.depth = 0
	p.inStr = false
	p.escaped = false
	p.start = 0
	p.invalidJSONSkipped = 0
	p.eventStreamFrames = 0
	p.eventStreamFrameErrors = 0
	p.eventStreamSkipped = 0
	p.eventStreamPayloadErrs = 0
}

func (p *StreamParser) BufferedBytes() int {
	if p == nil {
		return 0
	}
	return len(p.buffer)
}

func (p *StreamParser) InvalidJSONSkipped() int {
	if p == nil {
		return 0
	}
	return p.invalidJSONSkipped
}

func (p *StreamParser) EventStreamFrames() int {
	if p == nil {
		return 0
	}
	return p.eventStreamFrames
}

func (p *StreamParser) EventStreamFrameErrors() int {
	if p == nil {
		return 0
	}
	return p.eventStreamFrameErrors
}

func (p *StreamParser) EventStreamSkippedFrames() int {
	if p == nil {
		return 0
	}
	return p.eventStreamSkipped
}

func (p *StreamParser) EventStreamPayloadErrors() int {
	if p == nil {
		return 0
	}
	return p.eventStreamPayloadErrs
}

type eventStreamFrameStatus int

const (
	eventStreamFrameNotPresent eventStreamFrameStatus = iota
	eventStreamFrameComplete
	eventStreamFrameIncomplete
	eventStreamFrameInvalid
)

const maxKiroEventStreamFrameBytes = 16 * 1024 * 1024

var eventStreamCRCTable = crc32.MakeTable(crc32.IEEE)

type eventStreamMessage struct {
	eventType     string
	messageType   string
	exceptionType string
	payload       []byte
}

func (m eventStreamMessage) shouldParsePayload() bool {
	if len(m.payload) == 0 {
		return false
	}
	if m.messageType == "exception" || m.messageType == "error" || m.exceptionType != "" {
		return true
	}
	if m.eventType == "" {
		return true
	}
	return m.eventType == "assistantResponseEvent"
}

func parseEventStreamFrame(data []byte) (eventStreamFrameStatus, eventStreamMessage, int) {
	if len(data) == 0 {
		return eventStreamFrameNotPresent, eventStreamMessage{}, 0
	}
	if len(data) < 12 {
		if looksLikeEventStreamPrefix(data) {
			return eventStreamFrameIncomplete, eventStreamMessage{}, 0
		}
		return eventStreamFrameNotPresent, eventStreamMessage{}, 0
	}

	totalLength := int(binary.BigEndian.Uint32(data[0:4]))
	headersLength := int(binary.BigEndian.Uint32(data[4:8]))
	if totalLength < 16 || totalLength > maxKiroEventStreamFrameBytes || headersLength < 0 || headersLength > totalLength-16 {
		return eventStreamFrameNotPresent, eventStreamMessage{}, 0
	}

	preludeCRC := binary.BigEndian.Uint32(data[8:12])
	if crc32.Checksum(data[0:8], eventStreamCRCTable) != preludeCRC {
		return eventStreamFrameInvalid, eventStreamMessage{}, 0
	}
	if len(data) < totalLength {
		return eventStreamFrameIncomplete, eventStreamMessage{}, 0
	}

	messageCRC := binary.BigEndian.Uint32(data[totalLength-4 : totalLength])
	if crc32.Checksum(data[:totalLength-4], eventStreamCRCTable) != messageCRC {
		return eventStreamFrameInvalid, eventStreamMessage{}, 0
	}

	payloadStart := 12 + headersLength
	payloadEnd := totalLength - 4
	if payloadStart > payloadEnd {
		return eventStreamFrameInvalid, eventStreamMessage{}, 0
	}
	headers := data[12:payloadStart]
	msg := eventStreamMessage{
		eventType:     extractEventStreamStringHeader(headers, ":event-type"),
		messageType:   extractEventStreamStringHeader(headers, ":message-type"),
		exceptionType: extractEventStreamStringHeader(headers, ":exception-type"),
		payload:       data[payloadStart:payloadEnd],
	}
	return eventStreamFrameComplete, msg, totalLength
}

func extractEventStreamStringHeader(headers []byte, targetName string) string {
	pos := 0
	for pos < len(headers) {
		nameLen := int(headers[pos])
		pos++
		if pos+nameLen > len(headers) {
			return ""
		}
		name := string(headers[pos : pos+nameLen])
		pos += nameLen
		if pos >= len(headers) {
			return ""
		}
		valueType := headers[pos]
		pos++
		switch valueType {
		case 0, 1:
			if name == targetName {
				return fmt.Sprintf("%t", valueType == 0)
			}
		case 2:
			pos++
		case 3:
			pos += 2
		case 4:
			pos += 4
		case 5:
			pos += 8
		case 6, 7:
			if pos+2 > len(headers) {
				return ""
			}
			valueLen := int(binary.BigEndian.Uint16(headers[pos : pos+2]))
			pos += 2
			if pos+valueLen > len(headers) {
				return ""
			}
			value := string(headers[pos : pos+valueLen])
			pos += valueLen
			if name == targetName {
				return value
			}
		case 8:
			pos += 8
		case 9:
			pos += 16
		default:
			return ""
		}
		if pos > len(headers) {
			return ""
		}
	}
	return ""
}

func looksLikeEventStreamPrefix(data []byte) bool {
	if len(data) == 0 || data[0] != 0 {
		return false
	}
	if len(data) >= 2 && data[1] != 0 {
		return false
	}
	if len(data) >= 4 {
		totalLength := int(binary.BigEndian.Uint32(data[0:4]))
		return totalLength >= 16 && totalLength <= maxKiroEventStreamFrameBytes
	}
	return true
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
	toolNameMap    map[string]string
	nextIdx        int
	contentIdx     int
	toolIdx        int
	thinkingIdx    int
	inThinking     bool
	inContent      bool
	inTool         bool
	inputAccum     string
	currentToolID  string
	outputTokens   int
	stopped        bool
	emittedTool    bool
	hasVisibleText bool
	hasThinking    bool
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
	if stopReason == "end_turn" && c.hasThinking && !c.hasVisibleText && !c.emittedTool {
		c.appendParsedContentBlock(&events, ParsedContentBlock{Type: "text", Text: " "})
		c.closeContent(&events)
		stopReason = "max_tokens"
	}
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
		c.flushToolInput(events)
		*events = append(*events, AnthropicSSEEvent{
			Event: "content_block_stop",
			Data:  fmt.Sprintf(`{"type":"content_block_stop","index":%d}`, c.toolIdx),
		})
		c.inTool = false
	}
}

func (c *StreamConverter) flushToolInput(events *[]AnthropicSSEEvent) {
	if strings.TrimSpace(c.inputAccum) == "" {
		return
	}
	input := normalizeToolInputJSON(c.inputAccum)
	*events = append(*events, AnthropicSSEEvent{
		Event: "content_block_delta",
		Data:  fmt.Sprintf(`{"type":"content_block_delta","index":%d,"delta":{"type":"input_json_delta","partial_json":%s}}`, c.toolIdx, jsonStr(input)),
	})
	c.inputAccum = ""
}

func (c *StreamConverter) appendParsedContentBlock(events *[]AnthropicSSEEvent, block ParsedContentBlock) {
	switch block.Type {
	case "thinking":
		if block.Thinking == "" {
			return
		}
		c.hasThinking = true
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
		if strings.TrimSpace(block.Text) != "" {
			c.hasVisibleText = true
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

func normalizeToolInputJSON(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	if normalized, ok := normalizeToolInputJSONObject(input); ok {
		return normalized
	}
	if normalized, ok := normalizeToolInputJSONObject(repairKiroJSON(input)); ok {
		return normalized
	}
	return input
}

func normalizeToolInputJSONObject(input string) (string, bool) {
	var obj map[string]any
	if err := json.Unmarshal([]byte(input), &obj); err != nil {
		return "", false
	}
	if obj == nil {
		return "{}", true
	}
	for key := range obj {
		if strings.TrimSpace(key) == "" {
			delete(obj, key)
		}
	}
	normalized, err := json.Marshal(obj)
	if err != nil {
		return "", false
	}
	return string(normalized), true
}

func jsonStr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func FormatSSE(event AnthropicSSEEvent) []byte {
	return []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", event.Event, event.Data))
}
