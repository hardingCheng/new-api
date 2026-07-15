package service

import (
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// GetCallbackAddress 返回支付回调(notify/return)的基址。
// 请求来自已配置分站域名时用该域名(各分站域名指向同一后端,
// 加站/迁移无需改支付配置),否则回退全局 CustomCallbackAddress / ServerAddress。
func GetCallbackAddress(host string) string {
	if host != "" && setting.GetStationByHost(host) != nil {
		return "https://" + host
	}
	if operation_setting.CustomCallbackAddress == "" {
		return system_setting.ServerAddress
	}
	return operation_setting.CustomCallbackAddress
}
