package common

import (
	"math"
	"strings"
	"sync/atomic"
)

var (
	channelDisableThresholdBits atomic.Uint64
	automaticDisableChannelFlag atomic.Bool
	automaticEnableChannelFlag  atomic.Bool
	channelBreakerFailureLimit  atomic.Int64
	channelBreakerCooldownSecs  atomic.Int64
	channelBreakerProbeCount    atomic.Int64
	channelBreakerProbeSuccess  atomic.Int64
	channelBreakerExcludePaths  atomic.Value
)

func init() {
	SetChannelDisableThreshold(5.0)
	SetAutomaticDisableChannelEnabled(false)
	SetAutomaticEnableChannelEnabled(false)
	SetChannelBreakerFailureLimit(5)
	SetChannelBreakerCooldownSeconds(60)
	SetChannelBreakerProbeCount(5)
	SetChannelBreakerProbeSuccessCount(3)
	SetChannelBreakerExcludePaths("/v1/videos")
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

func GetChannelBreakerFailureLimit() int {
	return int(channelBreakerFailureLimit.Load())
}

func SetChannelBreakerFailureLimit(value int) {
	if value <= 0 {
		value = 5
	}
	channelBreakerFailureLimit.Store(int64(value))
}

func GetChannelBreakerCooldownSeconds() int {
	return int(channelBreakerCooldownSecs.Load())
}

func SetChannelBreakerCooldownSeconds(value int) {
	if value <= 0 {
		value = 60
	}
	channelBreakerCooldownSecs.Store(int64(value))
}

func GetChannelBreakerProbeCount() int {
	return int(channelBreakerProbeCount.Load())
}

func SetChannelBreakerProbeCount(value int) {
	if value <= 0 {
		value = 5
	}
	channelBreakerProbeCount.Store(int64(value))
	if GetChannelBreakerProbeSuccessCount() > value {
		SetChannelBreakerProbeSuccessCount(value)
	}
}

func GetChannelBreakerProbeSuccessCount() int {
	return int(channelBreakerProbeSuccess.Load())
}

func SetChannelBreakerProbeSuccessCount(value int) {
	if value <= 0 {
		value = 3
	}
	probeCount := GetChannelBreakerProbeCount()
	if value > probeCount {
		value = probeCount
	}
	channelBreakerProbeSuccess.Store(int64(value))
}

func GetChannelBreakerExcludePaths() []string {
	paths, ok := channelBreakerExcludePaths.Load().([]string)
	if !ok {
		return nil
	}
	return append([]string(nil), paths...)
}

func SetChannelBreakerExcludePaths(value string) {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == '\n' || r == ',' || r == '，'
	})
	paths := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			paths = append(paths, part)
		}
	}
	channelBreakerExcludePaths.Store(paths)
}
