package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChannelBreakerBackoffMultipliersParsing(t *testing.T) {
	t.Cleanup(func() { SetChannelBreakerBackoffMultipliers("1,2,5,15,60") })

	tests := []struct {
		name  string
		value string
		want  []int
	}{
		{"标准阶梯", "1,2,5,15,60", []int{1, 2, 5, 15, 60}},
		{"换行分隔", "1\n3\n9", []int{1, 3, 9}},
		{"空串回退默认", "", []int{1, 2, 5, 15, 60}},
		{"非数字整体回退", "1,abc,5", []int{1, 2, 5, 15, 60}},
		{"非正数整体回退", "1,0,5", []int{1, 2, 5, 15, 60}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetChannelBreakerBackoffMultipliers(tt.value)
			assert.Equal(t, tt.want, GetChannelBreakerBackoffMultipliers())
		})
	}
}
