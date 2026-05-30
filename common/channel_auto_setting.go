package common

import (
	"math"
	"sync/atomic"
)

var (
	channelDisableThresholdBits atomic.Uint64
	automaticDisableChannelFlag atomic.Bool
	automaticEnableChannelFlag  atomic.Bool
)

func init() {
	SetChannelDisableThreshold(5.0)
	SetAutomaticDisableChannelEnabled(false)
	SetAutomaticEnableChannelEnabled(false)
}

func GetChannelDisableThreshold() float64 {
	return math.Float64frombits(channelDisableThresholdBits.Load())
}

func SetChannelDisableThreshold(value float64) {
	channelDisableThresholdBits.Store(math.Float64bits(value))
}

func IsAutomaticDisableChannelEnabled() bool {
	return automaticDisableChannelFlag.Load()
}

func SetAutomaticDisableChannelEnabled(enabled bool) {
	automaticDisableChannelFlag.Store(enabled)
}

func IsAutomaticEnableChannelEnabled() bool {
	return automaticEnableChannelFlag.Load()
}

func SetAutomaticEnableChannelEnabled(enabled bool) {
	automaticEnableChannelFlag.Store(enabled)
}
