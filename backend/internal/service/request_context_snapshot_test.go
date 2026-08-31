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

func TestPrepareUsageLogRequestContextForAPIKey_SkipWhitelist(t *testing.T) {
	SetUsageLogRequestContextEnabled(true)
	SetUsageLogRequestContextSkipAPIKeyIDs([]int64{42, 7})
	t.Cleanup(func() { SetUsageLogRequestContextSkipAPIKeyIDs(nil) })

	raw := []byte(`{"model":"gpt-5.1"}`)
	skipped, truncated, bytesLen := PrepareUsageLogRequestContextForAPIKey(42, raw)
	require.Nil(t, skipped)
	require.False(t, truncated)
	require.Nil(t, bytesLen)

	recorded, truncated, bytesLen := PrepareUsageLogRequestContextForAPIKey(41, raw)
	require.NotNil(t, recorded)
	require.False(t, truncated)
	require.NotNil(t, bytesLen)
}

func TestNormalizeUsageLogRequestContextSkipAPIKeyIDs(t *testing.T) {
	got, err := NormalizeUsageLogRequestContextSkipAPIKeyIDs([]int64{3, 0, 1, 3, -2, 2})
	require.NoError(t, err)
	require.Equal(t, []int64{1, 2, 3}, got)

	empty, err := NormalizeUsageLogRequestContextSkipAPIKeyIDs(nil)
	require.NoError(t, err)
	require.NotNil(t, empty)
	require.Empty(t, empty)

	tooMany := make([]int64, usageLogRequestContextSkipAPIKeyIDsMax+1)
	for i := range tooMany {
		tooMany[i] = int64(i + 1)
	}
	_, err = NormalizeUsageLogRequestContextSkipAPIKeyIDs(tooMany)
	require.Error(t, err)
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
