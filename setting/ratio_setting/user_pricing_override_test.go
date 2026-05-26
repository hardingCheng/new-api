package ratio_setting

import "testing"

func TestApplyUserPricingOverrides(t *testing.T) {
	defer func() {
		_ = UpdateUserPricingOverrideByJSONString("{}")
	}()

	err := UpdateUserPricingOverrideByJSONString(`{
		"rules": [
			{"user_id":42,"type":"ratio","value":0.9},
			{"user_id":42,"group_pattern":"sd2","type":"ratio","value":0.8},
			{"user_id":42,"group_pattern":"sd2","model_pattern":"seedance-2.0-*","type":"model_price","value":0.25}
		]
	}`)
	if err != nil {
		t.Fatalf("update user pricing override: %v", err)
	}

	usePrice, modelPrice, modelRatio, groupRatio, matches := ApplyUserPricingOverrides(42, "u", "vip", "sd2", "seedance-2.0-480p", false, -1, 1.5, 1)
	if !usePrice {
		t.Fatalf("usePrice = false, want true")
	}
	if modelPrice != 0.25 {
		t.Fatalf("modelPrice = %v, want 0.25", modelPrice)
	}
	if modelRatio != 0 {
		t.Fatalf("modelRatio = %v, want 0", modelRatio)
	}
	if groupRatio != 0.8 {
		t.Fatalf("groupRatio = %v, want 0.8", groupRatio)
	}
	if len(matches) != 2 {
		t.Fatalf("matches len = %d, want 2", len(matches))
	}
}

func TestApplyUserPricingOverridesDefaultWhenNoMatch(t *testing.T) {
	defer func() {
		_ = UpdateUserPricingOverrideByJSONString("{}")
	}()
	if err := UpdateUserPricingOverrideByJSONString(`{"rules":[{"user_id":42,"type":"ratio","value":0.8}]}`); err != nil {
		t.Fatalf("update user pricing override: %v", err)
	}
	usePrice, modelPrice, modelRatio, groupRatio, matches := ApplyUserPricingOverrides(7, "u", "default", "default", "gpt-4o", false, -1, 1.25, 1)
	if usePrice || modelPrice != -1 || modelRatio != 1.25 || groupRatio != 1 || len(matches) != 0 {
		t.Fatalf("unexpected override result: usePrice=%v modelPrice=%v modelRatio=%v groupRatio=%v matches=%d", usePrice, modelPrice, modelRatio, groupRatio, len(matches))
	}
}
