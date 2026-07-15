package setting

import (
	"encoding/json"
	"net"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
)

// StationOAuthClient 单个 OAuth 提供方在某分站下的客户端凭据
type StationOAuthClient struct {
	ClientId     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// StationBrand 分站品牌与展示内容覆盖;空字段回退全局配置
type StationBrand struct {
	SystemName      string                   `json:"system_name,omitempty"`
	Logo            string                   `json:"logo,omitempty"`
	Footer          string                   `json:"footer,omitempty"`
	HomePageContent string                   `json:"home_page_content,omitempty"`
	Notice          string                   `json:"notice,omitempty"`
	About           string                   `json:"about,omitempty"`
	Announcements   []map[string]interface{} `json:"announcements,omitempty"`
}

// StationConfig 一个分站(以域名为 key)的完整配置
type StationConfig struct {
	Group string                        `json:"group,omitempty"`
	OAuth map[string]StationOAuthClient `json:"oauth,omitempty"`
	Brand StationBrand                  `json:"brand,omitempty"`
}

var stationConfigs = map[string]StationConfig{}
var stationConfigsMutex sync.RWMutex

func UpdateStationConfigsByJsonString(jsonString string) error {
	newConfigs := map[string]StationConfig{}
	if strings.TrimSpace(jsonString) != "" {
		if err := json.Unmarshal([]byte(jsonString), &newConfigs); err != nil {
			return err
		}
	}
	normalized := make(map[string]StationConfig, len(newConfigs))
	for host, cfg := range newConfigs {
		normalized[strings.ToLower(strings.TrimSpace(host))] = cfg
	}
	stationConfigsMutex.Lock()
	defer stationConfigsMutex.Unlock()
	stationConfigs = normalized
	return nil
}

func StationConfigs2JsonString() string {
	stationConfigsMutex.RLock()
	defer stationConfigsMutex.RUnlock()
	jsonBytes, err := json.Marshal(stationConfigs)
	if err != nil {
		common.SysLog("error marshalling station configs: " + err.Error())
		return "{}"
	}
	return string(jsonBytes)
}

// GetStationByHost 按请求 Host(可含端口)返回命中的分站配置;
// 未命中返回 nil,调用方回退全局默认。
func GetStationByHost(host string) *StationConfig {
	if host == "" {
		return nil
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.ToLower(strings.TrimSpace(host))
	stationConfigsMutex.RLock()
	defer stationConfigsMutex.RUnlock()
	if cfg, ok := stationConfigs[host]; ok {
		cfgCopy := cfg
		return &cfgCopy
	}
	return nil
}

// GetStationDomainByGroup 返回注册分组归属的分站域名;
// 多个域名命中同一分组时取字典序最小的,分组无分站返回空串。
func GetStationDomainByGroup(group string) string {
	if group == "" {
		return ""
	}
	stationConfigsMutex.RLock()
	defer stationConfigsMutex.RUnlock()
	domain := ""
	for host, cfg := range stationConfigs {
		if cfg.Group != group {
			continue
		}
		if domain == "" || host < domain {
			domain = host
		}
	}
	return domain
}

// GetStationOAuthClient 返回某分站下指定 OAuth 提供方的凭据;
// 分站未配置(或凭据不完整)返回 false,调用方回退全局凭据。
func GetStationOAuthClient(host string, provider string) (StationOAuthClient, bool) {
	station := GetStationByHost(host)
	if station == nil {
		return StationOAuthClient{}, false
	}
	client, ok := station.OAuth[provider]
	if !ok || client.ClientId == "" || client.ClientSecret == "" {
		return StationOAuthClient{}, false
	}
	return client, true
}
