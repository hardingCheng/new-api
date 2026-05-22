package model_setting

import "testing"

func TestIsTaskDurationBillingModel(t *testing.T) {
	original := taskBillingSettings
	defer func() {
		taskBillingSettings = original
	}()

	taskBillingSettings = TaskBillingSettings{
		DurationBillingModelPatterns: []string{
			"sora-2*",
			"seedance-*",
			"doubao-seedance-*",
		},
		DurationBillingExcludeModelPatterns: []string{
			"grok-imagine-video",
			"grok-imagine-1.0-video",
		},
		ReferenceVideoBillingModelPatterns: []string{
			"seedance-*",
			"doubao-seedance-*",
		},
	}

	tests := []struct {
		name     string
		models   []string
		expected bool
	}{
		{
			name:     "seedance prefix matches",
			models:   []string{"seedance-1-0-pro-250528"},
			expected: true,
		},
		{
			name:     "doubao seedance prefix matches",
			models:   []string{"doubao-seedance-2-0-260128"},
			expected: true,
		},
		{
			name:     "sora wildcard matches",
			models:   []string{"sora-2-pro"},
			expected: true,
		},
		{
			name:     "grok exact exclusion wins",
			models:   []string{"grok-imagine-video"},
			expected: false,
		},
		{
			name:     "exclusion wins across mapped names",
			models:   []string{"grok-imagine-1.0-video", "sora-2"},
			expected: false,
		},
		{
			name:     "unknown model does not match",
			models:   []string{"other-video-model"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTaskDurationBillingModel(tt.models...); got != tt.expected {
				t.Fatalf("IsTaskDurationBillingModel() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestIsTaskReferenceVideoBillingModel(t *testing.T) {
	original := taskBillingSettings
	defer func() {
		taskBillingSettings = original
	}()

	taskBillingSettings = TaskBillingSettings{
		ReferenceVideoBillingModelPatterns: []string{
			"seedance-*",
			"doubao-seedance-*",
		},
		DurationBillingExcludeModelPatterns: []string{
			"grok-imagine-video",
		},
	}

	tests := []struct {
		name     string
		models   []string
		expected bool
	}{
		{
			name:     "seedance matches reference video billing",
			models:   []string{"seedance-2.0"},
			expected: true,
		},
		{
			name:     "doubao seedance matches reference video billing",
			models:   []string{"doubao-seedance-2-0-260128"},
			expected: true,
		},
		{
			name:     "sora not enabled by default",
			models:   []string{"sora-2"},
			expected: false,
		},
		{
			name:     "exclude wins",
			models:   []string{"grok-imagine-video", "seedance-2.0"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTaskReferenceVideoBillingModel(tt.models...); got != tt.expected {
				t.Fatalf("IsTaskReferenceVideoBillingModel() = %v, want %v", got, tt.expected)
			}
		})
	}
}
