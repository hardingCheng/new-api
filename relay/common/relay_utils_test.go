package common

import "testing"

func TestIsSeedanceVideoModelIncludesPrism(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{model: "seedance-2.0-fast-480p", want: true},
		{model: "doubao-seedance-2-0-fast-260128", want: true},
		{model: "prism-3.0-fast-480p", want: true},
		{model: "grok-imagine-video", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			if got := IsSeedanceVideoModel(tt.model); got != tt.want {
				t.Fatalf("IsSeedanceVideoModel(%q) = %v, want %v", tt.model, got, tt.want)
			}
		})
	}
}
