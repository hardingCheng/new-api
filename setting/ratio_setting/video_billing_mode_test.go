package ratio_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
)

func TestVideoBillingModeMatching(t *testing.T) {
	oldPatches := constant.TaskPricePatches
	defer func() {
		constant.TaskPricePatches = oldPatches
		_ = UpdateVideoBillingModeByJSONString("{}")
	}()

	constant.TaskPricePatches = []string{"legacy-fixed-video"}
	if err := UpdateVideoBillingModeByJSONString(`{
		"seedance-*": "per_second",
		"grok-imagine-*": "per_call",
		"grok-imagine-1.0-video": "per_second"
	}`); err != nil {
		t.Fatalf("update video billing mode: %v", err)
	}

	tests := []struct {
		name  string
		model string
		want  string
	}{
		{name: "wildcard per second", model: "seedance-2.0-480p", want: VideoBillingModePerSecond},
		{name: "wildcard per call", model: "grok-imagine-video", want: VideoBillingModePerCall},
		{name: "exact beats wildcard", model: "grok-imagine-1.0-video", want: VideoBillingModePerSecond},
		{name: "legacy patch default", model: "legacy-fixed-video", want: VideoBillingModePerCall},
		{name: "default per second", model: "other-video-model", want: VideoBillingModePerSecond},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetVideoBillingMode(tt.model); got != tt.want {
				t.Fatalf("GetVideoBillingMode(%q) = %q, want %q", tt.model, got, tt.want)
			}
		})
	}
}

func TestUpdateVideoBillingModeNormalizesValues(t *testing.T) {
	defer func() {
		_ = UpdateVideoBillingModeByJSONString("{}")
	}()

	if err := UpdateVideoBillingModeByJSONString(`{
		"a": "按秒计费",
		"b": "per-request",
		"c": "invalid"
	}`); err != nil {
		t.Fatalf("update video billing mode: %v", err)
	}

	modes := GetVideoBillingModeCopy()
	if modes["a"] != VideoBillingModePerSecond {
		t.Fatalf("a mode = %q, want %q", modes["a"], VideoBillingModePerSecond)
	}
	if modes["b"] != VideoBillingModePerCall {
		t.Fatalf("b mode = %q, want %q", modes["b"], VideoBillingModePerCall)
	}
	if _, ok := modes["c"]; ok {
		t.Fatalf("invalid mode should be dropped")
	}
}
