package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGroupRatioInfoEffectiveUserRatio(t *testing.T) {
	tests := []struct {
		name string
		info GroupRatioInfo
		want float64
	}{
		{
			name: "override wins over special ratio",
			info: GroupRatioInfo{
				GroupRatio:        0.17,
				GroupSpecialRatio: 0.4,
				HasSpecialRatio:   true,
				UserOverrideRatio: 0.17,
				HasUserOverride:   true,
			},
			want: 0.17,
		},
		{
			name: "override without special ratio",
			info: GroupRatioInfo{
				GroupRatio:        0.1,
				GroupSpecialRatio: -1,
				UserOverrideRatio: 0.1,
				HasUserOverride:   true,
			},
			want: 0.1,
		},
		{
			name: "special ratio only",
			info: GroupRatioInfo{
				GroupRatio:        0.4,
				GroupSpecialRatio: 0.4,
				HasSpecialRatio:   true,
			},
			want: 0.4,
		},
		{
			name: "free override is a valid ratio",
			info: GroupRatioInfo{
				GroupRatio:        0,
				GroupSpecialRatio: 0.4,
				HasSpecialRatio:   true,
				UserOverrideRatio: 0,
				HasUserOverride:   true,
			},
			want: 0,
		},
		{
			name: "neither returns sentinel",
			info: GroupRatioInfo{GroupRatio: 1, GroupSpecialRatio: -1},
			want: -1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.info.EffectiveUserRatio())
		})
	}
}
