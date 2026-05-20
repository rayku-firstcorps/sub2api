package kiro

import (
	"encoding/binary"
	"hash/crc32"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStreamParserSkipsInvalidAWSFrameHeaderJSONStarts(t *testing.T) {
	parser := NewStreamParser()

	events := parser.Feed([]byte("binary {not-json} header {\"content\":\"Hel"))
	require.Empty(t, events)
	require.Equal(t, 1, parser.InvalidJSONSkipped())
	require.Greater(t, parser.BufferedBytes(), 0)

	events = parser.Feed([]byte("lo\"}{\"stop\":true}"))
	require.Len(t, events, 2)
	require.Equal(t, "Hello", events[0].Content)
	require.NotNil(t, events[1].Stop)
	require.True(t, *events[1].Stop)
	require.Equal(t, 0, parser.BufferedBytes())
}

func TestStreamParserParsesAWSEventStreamFramesAcrossFeeds(t *testing.T) {
	parser := NewStreamParser()
	stream := append(
		kiroEventStreamFrame([]byte(`{"content":"Hello"}`)),
		kiroEventStreamFrame([]byte(`{"stop":true}`))...,
	)

	events := parser.Feed(stream[:7])
	require.Empty(t, events)
	require.Equal(t, 7, parser.BufferedBytes())

	events = parser.Feed(stream[7 : len(stream)-3])
	require.Len(t, events, 1)
	require.Equal(t, "Hello", events[0].Content)
	require.Equal(t, 1, parser.EventStreamFrames())
	require.Greater(t, parser.BufferedBytes(), 0)

	events = parser.Feed(stream[len(stream)-3:])
	require.Len(t, events, 1)
	require.NotNil(t, events[0].Stop)
	require.True(t, *events[0].Stop)
	require.Equal(t, 2, parser.EventStreamFrames())
	require.Equal(t, 0, parser.EventStreamFrameErrors())
	require.Equal(t, 0, parser.BufferedBytes())
}

func TestStreamParserSkipsNonAssistantEventStreamFrames(t *testing.T) {
	parser := NewStreamParser()
	stream := append(
		kiroEventStreamFrameWithEventType("initial-response", []byte(`not-json`)),
		kiroEventStreamFrame([]byte(`{"content":"Hello"}`))...,
	)

	events := parser.Feed(stream)

	require.Len(t, events, 1)
	require.Equal(t, "Hello", events[0].Content)
	require.Equal(t, 2, parser.EventStreamFrames())
	require.Equal(t, 1, parser.EventStreamSkippedFrames())
	require.Equal(t, 0, parser.EventStreamPayloadErrors())
	require.Equal(t, 0, parser.InvalidJSONSkipped())
}

func TestStreamParserAcceptsStructuredToolInput(t *testing.T) {
	parser := NewStreamParser()

	events := parser.Feed([]byte(`{"name":"read_file","toolUseId":"toolu_1","input":{"path":"README.md"}}`))

	require.Len(t, events, 1)
	require.Equal(t, "read_file", events[0].Name)
	require.Equal(t, "toolu_1", events[0].ToolUseID)
	require.Equal(t, `{"path":"README.md"}`, string(events[0].Input))
}

func TestStreamParserNormalizesNullToolInput(t *testing.T) {
	parser := NewStreamParser()

	events := parser.Feed([]byte(`{"name":"read_file","toolUseId":"toolu_1","input":null}`))

	require.Len(t, events, 1)
	require.Equal(t, "read_file", events[0].Name)
	require.Equal(t, "toolu_1", events[0].ToolUseID)
	require.Empty(t, string(events[0].Input))
}

func TestStreamConverterUsesMonotonicBlockIndexesAndUsage(t *testing.T) {
	converter := NewStreamConverter(nil)

	events := converter.Convert(StreamEvent{Content: "hello"})
	require.Len(t, events, 2)
	require.Contains(t, events[0].Data, `"index":0`)

	events = converter.Convert(StreamEvent{Name: "read_file", ToolUseID: "toolu_1"})
	require.Len(t, events, 2)
	require.Equal(t, "content_block_stop", events[0].Event)
	require.Contains(t, events[1].Data, `"index":1`)

	stop := true
	converter.SetOutputTokens(7)
	events = converter.Convert(StreamEvent{Stop: &stop})
	require.Len(t, events, 3)
	require.Equal(t, "content_block_stop", events[0].Event)
	require.Contains(t, events[1].Data, `"output_tokens":7`)
	require.Equal(t, "message_stop", events[2].Event)
}

func TestStreamConverterMergesRepeatedToolUseEventsWithSameID(t *testing.T) {
	converter := NewStreamConverter(nil)

	events := converter.Convert(StreamEvent{Name: "WebFetch", ToolUseID: "toolu_1"})
	require.Len(t, events, 1)
	require.Equal(t, "content_block_start", events[0].Event)

	events = converter.Convert(StreamEvent{Name: "WebFetch", ToolUseID: "toolu_1", Input: JSONText(`{"url": "`)})
	require.Empty(t, events)

	events = converter.Convert(StreamEvent{Name: "WebFetch", ToolUseID: "toolu_1", Input: JSONText(`https://example.com", "prompt": "read"}`)})
	require.Empty(t, events)

	stop := true
	events = converter.Convert(StreamEvent{Stop: &stop})
	require.Len(t, events, 4)
	require.Equal(t, "content_block_delta", events[0].Event)
	require.Contains(t, events[0].Data, `"partial_json":"{\"prompt\":\"read\",\"url\":\"https://example.com\"}"`)
	require.Equal(t, "content_block_stop", events[1].Event)
	require.Contains(t, events[2].Data, `"stop_reason":"tool_use"`)
}

func TestParseKiroContentBlocksExtractsThinkingAndBracketToolCall(t *testing.T) {
	blocks := ParseKiroContentBlocks(`<thinking>
plan
</thinking>

Answer [Called Read with args: {path:"README.md",}] done`, nil)

	require.Len(t, blocks, 4)
	require.Equal(t, "thinking", blocks[0].Type)
	require.Contains(t, blocks[0].Thinking, "plan")
	require.Equal(t, "text", blocks[1].Type)
	require.Contains(t, blocks[1].Text, "Answer")
	require.Equal(t, "tool_use", blocks[2].Type)
	require.Equal(t, "Read", blocks[2].ToolName)
	require.Equal(t, map[string]any{"path": "README.md"}, blocks[2].ToolInput)
	require.Equal(t, "text", blocks[3].Type)
	require.Contains(t, blocks[3].Text, "done")
}

func TestStreamConverterConvertsBracketToolCallTextToToolUse(t *testing.T) {
	converter := NewStreamConverter(nil)

	events := converter.Convert(StreamEvent{Content: `[Called Read with args: {"path":"README.md"}]`})

	require.Len(t, events, 3)
	require.Equal(t, "content_block_start", events[0].Event)
	require.Contains(t, events[0].Data, `"type":"tool_use"`)
	require.Contains(t, events[0].Data, `"name":"Read"`)
	require.Equal(t, "content_block_delta", events[1].Event)
	require.Contains(t, events[1].Data, `"partial_json":"{\"path\":\"README.md\"}"`)
	require.Equal(t, "content_block_stop", events[2].Event)

	stop := true
	events = converter.Convert(StreamEvent{Stop: &stop})
	require.Contains(t, events[0].Data, `"stop_reason":"tool_use"`)
}

func TestStreamConverterConvertsThinkingTextToThinkingBlock(t *testing.T) {
	converter := NewStreamConverter(nil)

	events := converter.Convert(StreamEvent{Content: "<thinking>\nplan\n</thinking>\n\nfinal"})

	require.Len(t, events, 5)
	require.Contains(t, events[0].Data, `"type":"thinking"`)
	require.Contains(t, events[1].Data, `"type":"thinking_delta"`)
	require.Contains(t, events[1].Data, `"thinking":"plan\n"`)
	require.Contains(t, events[3].Data, `"type":"text"`)
	require.Contains(t, events[4].Data, `"text":"final"`)
}

func kiroEventStreamFrame(payload []byte) []byte {
	return kiroEventStreamFrameWithEventType("assistantResponseEvent", payload)
}

func kiroEventStreamFrameWithEventType(eventType string, payload []byte) []byte {
	headers := appendKiroEventStreamHeader(nil, ":event-type", eventType)
	headers = appendKiroEventStreamHeader(headers, ":content-type", "application/json")
	headers = appendKiroEventStreamHeader(headers, ":message-type", "event")

	totalLength := 12 + len(headers) + len(payload) + 4
	frame := make([]byte, totalLength)
	binary.BigEndian.PutUint32(frame[0:4], uint32(totalLength))
	binary.BigEndian.PutUint32(frame[4:8], uint32(len(headers)))
	binary.BigEndian.PutUint32(frame[8:12], crc32.ChecksumIEEE(frame[0:8]))
	copy(frame[12:], headers)
	copy(frame[12+len(headers):], payload)
	binary.BigEndian.PutUint32(frame[totalLength-4:], crc32.ChecksumIEEE(frame[:totalLength-4]))
	return frame
}

func appendKiroEventStreamHeader(dst []byte, name, value string) []byte {
	dst = append(dst, byte(len(name)))
	dst = append(dst, name...)
	dst = append(dst, 7)
	dst = binary.BigEndian.AppendUint16(dst, uint16(len(value)))
	dst = append(dst, value...)
	return dst
}
