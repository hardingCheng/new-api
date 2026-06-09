package common

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
)

const (
	ChannelBreakerScopeGlobal  = "global"
	ChannelBreakerScopeGroup   = "group"
	ChannelBreakerScopeModel   = "model"
	ChannelBreakerScopeChannel = "channel"
)

type ChannelBreakerRule struct {
	Id                   string   `json:"id"`
	Name                 string   `json:"name"`
	Enabled              bool     `json:"enabled"`
	Scope                string   `json:"scope"`
	Targets              []string `json:"targets"`
	FailureLimit         int      `json:"failure_limit"`
	CooldownSeconds      int      `json:"cooldown_seconds"`
	ProbeCount           int      `json:"probe_count"`
	ProbeSuccessCount    int      `json:"probe_success_count"`
	FailureStatusCodes   string   `json:"failure_status_codes"`
	FailureKeywords      string   `json:"failure_keywords"`
	ExcludePaths         string   `json:"exclude_paths"`
	DisableBreaker       bool     `json:"disable_breaker"`
	OnlyKeyBreaker       bool     `json:"only_key_breaker"`
	IgnoreClientError4xx bool     `json:"ignore_client_error_4xx"`
	// 立即禁用：命中（状态码 AND 关键词）时直接永久禁用整个渠道，不走失败计数熔断。
	// 典型场景：上游渠道账号余额耗尽返回 403。
	InstantDisableEnabled     bool   `json:"instant_disable_enabled"`
	InstantDisableStatusCodes string `json:"instant_disable_status_codes"`
	InstantDisableKeywords    string `json:"instant_disable_keywords"`
}

var (
	channelDisableThresholdBits atomic.Uint64
	automaticDisableChannelFlag atomic.Bool
	automaticEnableChannelFlag  atomic.Bool
	channelBreakerEnabledFlag   atomic.Bool
	channelBreakerFailureLimit  atomic.Int64
	channelBreakerCooldownSecs  atomic.Int64
	channelBreakerProbeCount    atomic.Int64
	channelBreakerProbeSuccess  atomic.Int64
	channelBreakerExcludePaths  atomic.Value
	channelBreakerRules         atomic.Value
)

func init() {
	SetChannelDisableThreshold(5.0)
	SetAutomaticDisableChannelEnabled(false)
	SetAutomaticEnableChannelEnabled(false)
	SetChannelBreakerEnabled(false)
	SetChannelBreakerFailureLimit(5)
	SetChannelBreakerCooldownSeconds(60)
	SetChannelBreakerProbeCount(5)
	SetChannelBreakerProbeSuccessCount(3)
	SetChannelBreakerExcludePaths("/v1/videos")
	SetChannelBreakerRules(nil)
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

func IsChannelBreakerEnabled() bool {
	return channelBreakerEnabledFlag.Load()
}

func SetChannelBreakerEnabled(enabled bool) {
	channelBreakerEnabledFlag.Store(enabled)
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
	channelBreakerExcludePaths.Store(ParseChannelBreakerList(value))
}

func GetChannelBreakerRules() []ChannelBreakerRule {
	rules, ok := channelBreakerRules.Load().([]ChannelBreakerRule)
	if !ok {
		return nil
	}
	return cloneChannelBreakerRules(rules)
}

func ChannelBreakerRulesToJSONString() string {
	data, err := Marshal(GetChannelBreakerRules())
	if err != nil {
		return "[]"
	}
	return string(data)
}

func UpdateChannelBreakerRulesByJSONString(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		SetChannelBreakerRules(nil)
		return nil
	}
	var rules []ChannelBreakerRule
	if err := UnmarshalJsonStr(value, &rules); err != nil {
		return err
	}
	SetChannelBreakerRules(rules)
	return nil
}

func SetChannelBreakerRules(rules []ChannelBreakerRule) {
	normalized := make([]ChannelBreakerRule, 0, len(rules))
	for _, rule := range rules {
		rule = NormalizeChannelBreakerRule(rule)
		if rule.Id == "" || rule.Scope == "" {
			continue
		}
		normalized = append(normalized, rule)
	}
	channelBreakerRules.Store(normalized)
}

func NormalizeChannelBreakerRule(rule ChannelBreakerRule) ChannelBreakerRule {
	rule.Id = strings.TrimSpace(rule.Id)
	rule.Name = strings.TrimSpace(rule.Name)
	rule.Scope = strings.ToLower(strings.TrimSpace(rule.Scope))
	switch rule.Scope {
	case ChannelBreakerScopeGroup, ChannelBreakerScopeModel, ChannelBreakerScopeChannel:
	default:
		rule.Scope = ChannelBreakerScopeGlobal
	}
	targets := make([]string, 0, len(rule.Targets))
	for _, target := range rule.Targets {
		target = strings.TrimSpace(target)
		if rule.Scope == ChannelBreakerScopeChannel {
			if id, err := strconv.Atoi(target); err == nil && id > 0 {
				target = strconv.Itoa(id)
			}
		}
		if target != "" {
			targets = append(targets, target)
		}
	}
	slices.Sort(targets)
	rule.Targets = slices.Compact(targets)
	if rule.FailureLimit <= 0 {
		rule.FailureLimit = GetChannelBreakerFailureLimit()
	}
	if rule.CooldownSeconds <= 0 {
		rule.CooldownSeconds = GetChannelBreakerCooldownSeconds()
	}
	if rule.ProbeCount <= 0 {
		rule.ProbeCount = GetChannelBreakerProbeCount()
	}
	if rule.ProbeSuccessCount <= 0 {
		rule.ProbeSuccessCount = GetChannelBreakerProbeSuccessCount()
	}
	if rule.ProbeSuccessCount > rule.ProbeCount {
		rule.ProbeSuccessCount = rule.ProbeCount
	}
	return rule
}

func ParseChannelBreakerList(value string) []string {
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
	return paths
}

func cloneChannelBreakerRules(rules []ChannelBreakerRule) []ChannelBreakerRule {
	if len(rules) == 0 {
		return []ChannelBreakerRule{}
	}
	cloned := make([]ChannelBreakerRule, len(rules))
	for i, rule := range rules {
		cloned[i] = rule
		cloned[i].Targets = append([]string(nil), rule.Targets...)
	}
	return cloned
}
