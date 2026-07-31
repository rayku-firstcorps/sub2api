package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrepareUsageLogRequestContext_SanitizesAndTracksBytes(t *testing.T) {
	SetUsageLogRequestContextEnabled(true)
	raw := []byte(`{"model":"gpt-5.1","api_key":"secret","messages":[{"role":"user","content":"hello"}]}`)

	got, truncated, bytesLen := PrepareUsageLogRequestContext(raw)

	require.NotNil(t, got)
	require.False(t, truncated)
	require.NotNil(t, bytesLen)
	require.Equal(t, len(raw), *bytesLen)
	require.JSONEq(t, `{"model":"gpt-5.1","api_key":"[REDACTED]","messages":[{"role":"user","content":"hello"}]}`, *got)
}

func TestPrepareUsageLogRequestContext_DisabledSkipsSnapshot(t *testing.T) {
	SetUsageLogRequestContextEnabled(false)
	t.Cleanup(func() { SetUsageLogRequestContextEnabled(true) })

	got, truncated, bytesLen := PrepareUsageLogRequestContext([]byte(`{"model":"gpt-5.1"}`))

	require.Nil(t, got)
	require.False(t, truncated)
	require.Nil(t, bytesLen)
}

func TestPrepareUsageLogRequestContext_TruncatesLargeMessages(t *testing.T) {
	body := map[string]any{
		"model":    "gpt-5.1",
		"messages": []any{map[string]any{"role": "user", "content": strings.Repeat("x", usageLogRequestContextMaxBytes+1024)}},
	}
	raw, err := json.Marshal(body)
	require.NoError(t, err)

	got, truncated, bytesLen := PrepareUsageLogRequestContext(raw)

	require.NotNil(t, got)
	require.True(t, truncated)
	require.NotNil(t, bytesLen)
	require.Equal(t, len(raw), *bytesLen)
	require.LessOrEqual(t, len(*got), usageLogRequestContextMaxBytes)
}

func TestPrepareUsageLogRequestContext_TruncatesLargeResponsesInput(t *testing.T) {
	body := map[string]any{
		"model":  "gpt-5.5",
		"stream": true,
		"input": []any{
			map[string]any{
				"type":    "message",
				"role":    "developer",
				"content": []any{map[string]any{"type": "input_text", "text": strings.Repeat("d", usageLogRequestContextMaxBytes)}},
			},
			map[string]any{
				"type":    "message",
				"role":    "user",
				"content": []any{map[string]any{"type": "input_text", "text": "latest user prompt"}},
			},
		},
	}
	raw, err := json.Marshal(body)
	require.NoError(t, err)

	got, truncated, bytesLen := PrepareUsageLogRequestContext(raw)

	require.NotNil(t, got)
	require.True(t, truncated)
	require.NotNil(t, bytesLen)
	require.Equal(t, len(raw), *bytesLen)
	require.LessOrEqual(t, len(*got), usageLogRequestContextMaxBytes)
	require.JSONEq(t, `{"model":"gpt-5.5","stream":true,"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"latest user prompt"}]}]}`, *got)
}
