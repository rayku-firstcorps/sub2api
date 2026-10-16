//go:build unit

package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const monitorCandyAnswer = `最少取出 **21个**：利用手感，取 **9个圆形糖果和12个五角星形糖果**。

### 为什么一定能满足要求？
- 圆形西瓜味只有8个，所以9个圆形糖果中，至少有1个是苹果味或桃子味。
- 五角星形西瓜味只有4个，所以12个糖果中，苹果味和桃子味一定都有。

### 为什么20个不够？
设取出的五角星形糖果有 s 个，圆形糖果有 20-s 个。`

func TestMonitorQualityAnswerMatching(t *testing.T) {
	for _, tc := range []struct {
		name, expected, response string
		matches                  bool
	}{
		{"exact", "21", " 21\n", true},
		{"reported candy false positive", "21", monitorCandyAnswer, true},
		{"bold", "21", "**21**", true},
		{"inline code", "21", "`21`", true},
		{"count unit", "21", "21个", true},
		{"count unit and explanation", "21", "21个糖果。\n取9个圆形和12个五角星形。", true},
		{"explicit answer", "21", "答案是 **21**。因为9+12=21。", true},
		{"conclusion after reasoning", "21", "9个圆形和12个五角星形可以保证。\n最终答案：21", true},
		{"markdown heading", "21", "### 最终答案：**21个**", true},
		{"english", "21", "The answer is **21**. Choose 9 round and 12 star-shaped candies.", true},
		{"english units", "21", "Final answer: 21 candies.", true},
		{"equal decimal", "21", "答案是21.0。", true},
		{"negative", "-21", "答案：-21", true},
		{"text exact", "答案", "答案", true},
		{"text bold", "答案", "**答案**", true},
		{"text substring", "答案", "这是答案", false},
		{"wrong answer", "21", "最少取出 **22个**：21个不够。", false},
		{"number substring", "21", "答案：121", false},
		{"decimal substring", "21", "答案：21.5", false},
		{"percent", "21", "答案：21%", false},
		{"fraction", "21", "答案：21/2", false},
		{"exponent", "21", "答案：21e3", false},
		{"range", "21", "答案：21-22", false},
		{"alternatives", "21", "答案是21或22", false},
		{"expression", "21", "答案：21+1", false},
		{"negated", "21", "答案不是21，而是22。", false},
		{"mentioned in reasoning", "21", "取21个不够，最少需要22个。", false},
		{"quoted expectation", "21", "预期答案是21，但这是错误的。", false},
		{"contradictory conclusions", "21", "答案：21\n最终答案：22", false},
		{"contradictory ambiguous conclusion", "21", "21\n最终答案：21或22", false},
		{"empty", "21", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.matches, matchMonitorQualityAnswer(tc.response, tc.expected))
		})
	}
}

func TestMonitorQualityCandyAnswerClassification(t *testing.T) {
	original := monitorHTTPClient
	t.Cleanup(func() { monitorHTTPClient = original })
	for _, tc := range []struct {
		name, answer, finishReason, status string
	}{
		{"correct with explanation", monitorCandyAnswer, "stop", MonitorStatusOperational},
		{"wrong with expected number in explanation", "最少取出 **22个**：21个不够。", "stop", MonitorStatusQualityFailed},
		{"truncated is still inconclusive", monitorCandyAnswer, "length", MonitorStatusFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(map[string]any{"choices": []any{map[string]any{
				"message": map[string]any{"content": tc.answer}, "finish_reason": tc.finishReason,
			}}})
			require.NoError(t, err)
			monitorHTTPClient = &http.Client{Transport: feishuRoundTripper(func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
			})}
			result := runCheckForModel(context.Background(), "openai", "https://example.com", "secret", "model", &CheckOptions{
				Prompt: "最少取出多少个糖果？", ExpectedAnswer: "21",
			})
			require.Equal(t, tc.status, result.Status)
		})
	}
}
