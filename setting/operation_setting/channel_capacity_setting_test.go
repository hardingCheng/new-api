package operation_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// off 是紧急回滚开关：存量配置值被污染时必须回退到 off，而不是意外启用限流。
func TestGetChannelCapacityModeCoercesInvalidValueToOff(t *testing.T) {
	original := channelCapacitySetting.Mode
	t.Cleanup(func() { channelCapacitySetting.Mode = original })

	channelCapacitySetting.Mode = "enforce"
	assert.Equal(t, ChannelCapacityModeEnforce, GetChannelCapacityMode())

	channelCapacitySetting.Mode = "shadow"
	assert.Equal(t, ChannelCapacityModeShadow, GetChannelCapacityMode())

	for _, polluted := range []string{"", "on", "ENFORCE", "true"} {
		channelCapacitySetting.Mode = polluted
		assert.Equal(t, ChannelCapacityModeOff, GetChannelCapacityMode())
	}
}

func TestIsValidChannelCapacityMode(t *testing.T) {
	assert.True(t, IsValidChannelCapacityMode("off"))
	assert.True(t, IsValidChannelCapacityMode("shadow"))
	assert.True(t, IsValidChannelCapacityMode("enforce"))
	assert.False(t, IsValidChannelCapacityMode(""))
	assert.False(t, IsValidChannelCapacityMode("Shadow"))
}
