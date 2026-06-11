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

	res := ApplyUserPricingOverrides(42, "u", "vip", "sd2", "seedance-2.0-480p", false, -1, 1.5, 1)
	if !res.UsePrice {
		t.Fatalf("usePrice = false, want true")
	}
	if res.ModelPrice != 0.25 {
		t.Fatalf("modelPrice = %v, want 0.25", res.ModelPrice)
	}
	if res.ModelRatio != 0 {
		t.Fatalf("modelRatio = %v, want 0", res.ModelRatio)
	}
	if res.GroupRatio != 0.8 {
		t.Fatalf("groupRatio = %v, want 0.8", res.GroupRatio)
	}
	if len(res.Matches) != 2 {
		t.Fatalf("matches len = %d, want 2", len(res.Matches))
	}
}

func TestApplyUserPricingOverridesReferenceModes(t *testing.T) {
	defer func() {
		_ = UpdateUserPricingOverrideByJSONString("{}")
	}()

	if err := UpdateUserPricingOverrideByJSONString(`{
		"rules": [
			{"user_id":1,"model_pattern":"seedance-*","type":"video_ref_factor","value":0.5},
			{"user_id":2,"model_pattern":"seedance-*","type":"video_ref_price","value":0.02},
			{"user_id":3,"model_pattern":"seedance-*","type":"video_ref_flat","value":1},
			{"user_id":4,"model_pattern":"seedance-*","type":"video_ref_cap","value":5}
		]
	}`); err != nil {
		t.Fatalf("update user pricing override: %v", err)
	}

	cases := []struct {
		userID int
		mode   string
		value  float64
	}{
		{1, VideoRefModeFactor, 0.5},
		{2, VideoRefModePrice, 0.02},
		{3, VideoRefModeFlat, 1},
		{4, VideoRefModeCap, 5},
	}
	for _, tc := range cases {
		res := ApplyUserPricingOverrides(tc.userID, "u", "default", "default", "seedance-2.0-480p", true, 0.1, 0, 1)
		if res.VideoRefMode != tc.mode || res.VideoRefValue != tc.value {
			t.Fatalf("user %d: mode=%q value=%v, want mode=%q value=%v", tc.userID, res.VideoRefMode, res.VideoRefValue, tc.mode, tc.value)
		}
		if len(res.Matches) != 1 {
			t.Fatalf("user %d: matches=%d, want 1", tc.userID, len(res.Matches))
		}
	}

	// 无规则用户：不返回参考模式
	res := ApplyUserPricingOverrides(99, "u", "default", "default", "seedance-2.0-480p", true, 0.1, 0, 1)
	if res.VideoRefMode != VideoRefModeNone {
		t.Fatalf("no-rule user: mode=%q, want empty", res.VideoRefMode)
	}
}

func TestApplyUserPricingOverridesDefaultWhenNoMatch(t *testing.T) {
	defer func() {
		_ = UpdateUserPricingOverrideByJSONString("{}")
	}()
	if err := UpdateUserPricingOverrideByJSONString(`{"rules":[{"user_id":42,"type":"ratio","value":0.8}]}`); err != nil {
		t.Fatalf("update user pricing override: %v", err)
	}
	res := ApplyUserPricingOverrides(7, "u", "default", "default", "gpt-4o", false, -1, 1.25, 1)
	if res.UsePrice || res.ModelPrice != -1 || res.ModelRatio != 1.25 || res.GroupRatio != 1 || len(res.Matches) != 0 {
		t.Fatalf("unexpected override result: usePrice=%v modelPrice=%v modelRatio=%v groupRatio=%v matches=%d", res.UsePrice, res.ModelPrice, res.ModelRatio, res.GroupRatio, len(res.Matches))
	}
}
