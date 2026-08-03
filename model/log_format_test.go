package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/require"
)

// TestFormatUserLogsStripsAdminOnlyFields verifies that non-admin log views
// remove routing, pricing-rule, and group details while retaining public
// billing fields.
func TestFormatUserLogsStripsAdminOnlyFields(t *testing.T) {
	other := common.MapToJsonStr(map[string]interface{}{
		"model_price": 0.004,
		"group":       "vip",
		"user_pricing_overrides": []interface{}{
			map[string]interface{}{"rule": map[string]interface{}{"value": 0.5}},
		},
		"model_quota_pools": []interface{}{
			map[string]interface{}{"redis_key": "model_quota_pool:secret"},
		},
		"admin_info": map[string]interface{}{
			"quota_saturation": map[string]interface{}{
				"op":      "QuotaFromDecimal",
				"kind":    "overflow",
				"clamped": common.MaxQuota,
			},
		},
	})
	logs := []*Log{{Group: "vip", Other: other}}

	formatUserLogs(logs, 0)

	parsed, err := common.StrToMap(logs[0].Other)
	require.NoError(t, err)
	_, hasAdminInfo := parsed["admin_info"]
	require.False(t, hasAdminInfo, "admin_info (and nested quota_saturation) must be stripped for non-admin views")
	require.NotContains(t, parsed, "user_pricing_overrides")
	require.NotContains(t, parsed, "model_quota_pools")
	require.NotContains(t, parsed, "group")
	require.Empty(t, logs[0].Group)
	// Non-admin billing fields remain visible.
	require.Contains(t, parsed, "model_price")
}
