//go:build unit

package repository

import (
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestChannelMonitorQualityFieldsMapping(t *testing.T) {
	row := &dbent.ChannelMonitor{ReasoningEffort: "high", Prompt: "问题", ExpectedAnswer: "答案", NotifyFeishu: true}
	m := entToServiceMonitor(row)
	require.Equal(t, row.ReasoningEffort, m.ReasoningEffort)
	require.Equal(t, row.Prompt, m.Prompt)
	require.Equal(t, row.ExpectedAnswer, m.ExpectedAnswer)
	require.True(t, m.NotifyFeishu)
}
