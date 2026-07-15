package controller

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// paymentReturnPath 构造支付完成后用户落地的控制台地址。
// 请求来自已配置分站域名时留在该域名上(跨站会丢登录态),
// 否则回退全局 ServerAddress;host 传空 = 强制走全局。
func paymentReturnPath(host string, suffix string) string {
	base := strings.TrimRight(system_setting.ServerAddress, "/")
	if host != "" && setting.GetStationByHost(host) != nil {
		base = "https://" + host
	}
	return base + common.ThemeAwarePath(suffix)
}
