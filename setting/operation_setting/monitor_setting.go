package operation_setting

import (
	"os"
	"strconv"

	"github.com/QuantumNous/new-api/setting/config"
)

type MonitorSetting struct {
	AutoTestChannelEnabled            bool    `json:"auto_test_channel_enabled"`
	AutoTestChannelMinutes            float64 `json:"auto_test_channel_minutes"`
	BarkAlertEnabled                  bool    `json:"bark_alert_enabled"`
	BarkAlertUrl                      string  `json:"bark_alert_url"`
	LowBalanceAlertEnabled            bool    `json:"low_balance_alert_enabled"`
	LowBalanceThresholdCny            float64 `json:"low_balance_threshold_cny"`
	ChannelBreakerAlertEnabled        bool    `json:"channel_breaker_alert_enabled"`
	ChannelDisableAlertEnabled        bool    `json:"channel_disable_alert_enabled"`
	ChannelDisableAlertCooldownSecond int     `json:"channel_disable_alert_cooldown_second"`
	RetestDisabledChannelEnabled      bool    `json:"retest_disabled_channel_enabled"`
	RetestDisabledChannelSeconds      int     `json:"retest_disabled_channel_seconds"`
}

// 默认配置
var monitorSetting = MonitorSetting{
	AutoTestChannelEnabled:            false,
	AutoTestChannelMinutes:            10,
	BarkAlertEnabled:                  true,
	BarkAlertUrl:                      "https://bark.aigod.one/kFRNZMUXcuQ6c4ccrUgQ3W/",
	LowBalanceAlertEnabled:            true,
	LowBalanceThresholdCny:            10,
	ChannelBreakerAlertEnabled:        true,
	ChannelDisableAlertEnabled:        true,
	ChannelDisableAlertCooldownSecond: 300,
	RetestDisabledChannelEnabled:      false,
	RetestDisabledChannelSeconds:      15,
}

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("monitor_setting", &monitorSetting)
}

func GetMonitorSetting() *MonitorSetting {
	if os.Getenv("CHANNEL_TEST_FREQUENCY") != "" {
		frequency, err := strconv.Atoi(os.Getenv("CHANNEL_TEST_FREQUENCY"))
		if err == nil && frequency > 0 {
			monitorSetting.AutoTestChannelEnabled = true
			monitorSetting.AutoTestChannelMinutes = float64(frequency)
		}
	}
	return &monitorSetting
}
