package operation_setting

import (
	"os"
	"strconv"

	"github.com/QuantumNous/new-api/setting/config"
)

type MonitorSetting struct {
	AutoTestChannelEnabled            bool    `json:"auto_test_channel_enabled"`
	AutoTestChannelMinutes            float64 `json:"auto_test_channel_minutes"`
	ChannelTestMode                   string  `json:"channel_test_mode"`
	BarkAlertEnabled                  bool    `json:"bark_alert_enabled"`
	BarkAlertUrl                      string  `json:"bark_alert_url"`
	BarkAlertVolume                   int     `json:"bark_alert_volume"`
	BarkAlertIcon                     string  `json:"bark_alert_icon"`
	LowBalanceAlertEnabled            bool    `json:"low_balance_alert_enabled"`
	LowBalanceThresholdCny            float64 `json:"low_balance_threshold_cny"`
	LowBalanceAlertSound              string  `json:"low_balance_alert_sound"`
	ChannelBalanceCheckEnabled        bool    `json:"channel_balance_check_enabled"`
	ChannelBalanceCheckMinutes        float64 `json:"channel_balance_check_minutes"`
	ChannelBalanceAlertThreshold      float64 `json:"channel_balance_alert_threshold"`
	ChannelBalanceAlertSound          string  `json:"channel_balance_alert_sound"`
	ChannelBreakerAlertEnabled        bool    `json:"channel_breaker_alert_enabled"`
	ChannelBreakerAlertSound          string  `json:"channel_breaker_alert_sound"`
	ChannelDisableAlertEnabled        bool    `json:"channel_disable_alert_enabled"`
	ChannelDisableAlertSound          string  `json:"channel_disable_alert_sound"`
	ChannelDisableAlertCooldownSecond int     `json:"channel_disable_alert_cooldown_second"`
	RetestDisabledChannelEnabled      bool    `json:"retest_disabled_channel_enabled"`
	RetestDisabledChannelSeconds      int     `json:"retest_disabled_channel_seconds"`
}

const (
	ChannelTestModeScheduledAll    = "scheduled_all"
	ChannelTestModePassiveRecovery = "passive_recovery"
)

// 默认配置
var monitorSetting = MonitorSetting{
	AutoTestChannelEnabled:            false,
	AutoTestChannelMinutes:            10,
	ChannelTestMode:                   ChannelTestModeScheduledAll,
	BarkAlertEnabled:                  true,
	BarkAlertUrl:                      "https://bark.aigod.one/kFRNZMUXcuQ6c4ccrUgQ3W/",
	BarkAlertVolume:                   5,
	BarkAlertIcon:                     "",
	LowBalanceAlertEnabled:            true,
	LowBalanceThresholdCny:            10,
	LowBalanceAlertSound:              "alarm",
	ChannelBalanceCheckEnabled:        true,
	ChannelBalanceCheckMinutes:        360,
	ChannelBalanceAlertThreshold:      10,
	ChannelBalanceAlertSound:          "alarm",
	ChannelBreakerAlertEnabled:        true,
	ChannelBreakerAlertSound:          "alarm",
	ChannelDisableAlertEnabled:        true,
	ChannelDisableAlertSound:          "alarm",
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
			monitorSetting.ChannelTestMode = ChannelTestModeScheduledAll
		}
	}
	if enabled, ok := os.LookupEnv("CHANNEL_TEST_ENABLED"); ok {
		parsed, err := strconv.ParseBool(enabled)
		if err == nil {
			monitorSetting.AutoTestChannelEnabled = parsed
		}
	}
	if monitorSetting.ChannelTestMode != ChannelTestModePassiveRecovery {
		monitorSetting.ChannelTestMode = ChannelTestModeScheduledAll
	}
	return &monitorSetting
}
