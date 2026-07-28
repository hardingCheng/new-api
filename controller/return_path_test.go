package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPaymentReturnPathUsesDefaultDashboardRoutes(t *testing.T) {
	previousAddress := system_setting.ServerAddress
	system_setting.ServerAddress = "https://dashboard.example.com/"
	t.Cleanup(func() { system_setting.ServerAddress = previousAddress })

	assert.Equal(
		t,
		"https://dashboard.example.com/wallet?pay=success",
		paymentReturnPath("", "/wallet?pay=success"),
	)
	assert.Equal(
		t,
		"https://dashboard.example.com/usage-logs",
		paymentReturnPath("", "/usage-logs"),
	)
}

func TestPaymentReturnPathStaysOnConfiguredStationHost(t *testing.T) {
	previousAddress := system_setting.ServerAddress
	system_setting.ServerAddress = "https://dashboard.example.com/"
	previousStations := setting.StationConfigs2JsonString()
	t.Cleanup(func() {
		system_setting.ServerAddress = previousAddress
		require.NoError(t, setting.UpdateStationConfigsByJsonString(previousStations))
	})
	require.NoError(t, setting.UpdateStationConfigsByJsonString(
		`{"station.example.com":{"group":"station"}}`,
	))

	// 请求来自已配置站点域名:留在该域名,跨站会丢登录态
	assert.Equal(
		t,
		"https://station.example.com/wallet",
		paymentReturnPath("station.example.com", "/wallet"),
	)
	// 未配置的域名回退全局地址
	assert.Equal(
		t,
		"https://dashboard.example.com/wallet",
		paymentReturnPath("unknown.example.com", "/wallet"),
	)
}
