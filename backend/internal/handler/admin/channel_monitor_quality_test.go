//go:build unit

package admin

import (
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestChannelMonitorQualityRequestAndResponse(t *testing.T) {
	var create channelMonitorCreateRequest
	require.NoError(t, json.Unmarshal([]byte(`{"prompt":"问题","expected_answer":"答案","reasoning_effort":"high","notify_feishu":true}`), &create))
	require.Equal(t, "问题", create.Prompt)
	require.Equal(t, "答案", create.ExpectedAnswer)
	require.Equal(t, "high", create.ReasoningEffort)
	require.True(t, create.NotifyFeishu)
	var update channelMonitorUpdateRequest
	require.NoError(t, json.Unmarshal([]byte(`{"prompt":"","expected_answer":"","reasoning_effort":"","notify_feishu":false}`), &update))
	require.NotNil(t, update.Prompt)
	require.NotNil(t, update.ExpectedAnswer)
	require.NotNil(t, update.ReasoningEffort)
	require.NotNil(t, update.NotifyFeishu)
	require.False(t, *update.NotifyFeishu)
	response := channelMonitorToResponse(&service.ChannelMonitor{Prompt: create.Prompt, ExpectedAnswer: create.ExpectedAnswer, ReasoningEffort: create.ReasoningEffort, NotifyFeishu: true})
	raw, err := json.Marshal(response)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"expected_answer":"答案"`)
	require.Contains(t, string(raw), `"notify_feishu":true`)
}
