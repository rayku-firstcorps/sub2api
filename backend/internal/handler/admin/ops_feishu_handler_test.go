package admin

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpsFeishuRulePayloadValidation(t *testing.T) {
	for _, tc := range []struct {
		value                 string
		want, provided, valid bool
	}{
		{"", true, false, true}, {"true", true, true, true}, {"false", false, true, true},
		{"null", false, true, false}, {"1", false, true, false}, {"\"yes\"", false, true, false},
	} {
		t.Run(tc.value, func(t *testing.T) {
			raw := map[string]json.RawMessage{
				"name": json.RawMessage("\"Test\""), "metric_type": json.RawMessage("\"error_rate\""),
				"operator": json.RawMessage("\">\""), "threshold": json.RawMessage("5"),
			}
			if tc.value != "" {
				raw["notify_feishu"] = json.RawMessage(tc.value)
			}
			result, err := validateOpsAlertRulePayload(raw)
			if !tc.valid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, result.NotifyFeishu)
			require.Equal(t, tc.provided, result.FeishuProvided)
		})
	}
}
