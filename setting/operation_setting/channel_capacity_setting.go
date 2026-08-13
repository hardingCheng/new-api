package operation_setting

import (
	"github.com/QuantumNous/new-api/setting/config"
)

// ChannelCapacitySetting 渠道容量保护的全局运行模式。
// 渠道级限额（RPM / 最大并发）配置在 channels.setting 的 capacity 字段，
// 见 relaykit/dto.ChannelCapacitySettings。
type ChannelCapacitySetting struct {
	// Mode: off（默认，完全绕过，也是紧急回滚开关）/ shadow（正常放行但记录
	// 达限情况）/ enforce（真正跳过满载渠道并本地 429）。
	Mode string `json:"mode"`
}

const (
	ChannelCapacityModeOff     = "off"
	ChannelCapacityModeShadow  = "shadow"
	ChannelCapacityModeEnforce = "enforce"
)

var channelCapacitySetting = ChannelCapacitySetting{
	Mode: ChannelCapacityModeOff,
}

func init() {
	config.GlobalConfig.Register("channel_capacity_setting", &channelCapacitySetting)
}

// GetChannelCapacityMode 返回当前模式；非法存量值一律按 off 处理，保证
// 配置被污染时行为与当前版本一致。
func GetChannelCapacityMode() string {
	switch channelCapacitySetting.Mode {
	case ChannelCapacityModeShadow, ChannelCapacityModeEnforce:
		return channelCapacitySetting.Mode
	default:
		return ChannelCapacityModeOff
	}
}

func IsValidChannelCapacityMode(mode string) bool {
	switch mode {
	case ChannelCapacityModeOff, ChannelCapacityModeShadow, ChannelCapacityModeEnforce:
		return true
	}
	return false
}
