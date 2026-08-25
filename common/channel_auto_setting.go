package common

import (
	"fmt"
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
	channelBreakerExemptChannel atomic.Value // map[int]struct{}：报错不触发熔断与自动禁用的渠道

	channelBreakerPenaltyEnabledFlag  atomic.Bool
	channelBreakerPenaltyOfflineFlag  atomic.Bool
	channelBreakerPenaltyAlertOpens   atomic.Int64
	channelBreakerPenaltyOfflineHours atomic.Int64
	channelBreakerPenaltyMinPoolSize  atomic.Int64

	channelBreakerBackoffEnabledFlag atomic.Bool

	// ---- 2026-08-25 新增三个开关，默认全关 ----
	// 上游把 OpenAI 的 400 参数校验错误包成 5xx 时，归正为客户端错误，
	// 不计入熔断、不跨渠道重试（实测同一条渠道 84% 被包成 502）。
	upstreamClientErrNormalizeFlag atomic.Bool
	// 某分组所有档位的渠道都被熔断挡住时，降级再选一次（忽略熔断状态）。
	// 没有这条时选路直接返回"无可用渠道"，用户拿到硬报错。
	breakerAllOpenFallbackFlag atomic.Bool
	// chat/completions → Responses 转换只对非流式生效。
	// 流式要额外把上游 Responses SSE 翻译回 chat SSE，实测大量 500。
	chatToResponsesNonStreamOnlyFlag atomic.Bool
	channelBreakerBackoffMultiplier  atomic.Value // []int：连续熔断的冷却倍率阶梯
	channelBreakerBackoffMaxCooldown atomic.Int64
	channelBreakerBackoffDecaySecs   atomic.Int64
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
	SetChannelBreakerExemptChannels(nil)
	SetChannelBreakerPenaltyEnabled(false)
	SetChannelBreakerPenaltyOfflineEnabled(false)
	SetChannelBreakerPenaltyAlertOpensPerHour(10)
	SetChannelBreakerPenaltyOfflineConsecutiveHours(2)
	SetChannelBreakerPenaltyMinPoolSize(2)
	SetChannelBreakerBackoffEnabled(false)
	SetUpstreamClientErrNormalize(false)
	SetBreakerAllOpenFallback(false)
	SetChatToResponsesNonStreamOnly(false)
	SetChannelBreakerBackoffMultipliers("1,2,5,15,60")
	SetChannelBreakerBackoffMaxCooldownSeconds(3600)
	SetChannelBreakerBackoffDecaySeconds(600)
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

// ---- 熔断惩罚：反复熔断 → 告警 / 自动下线。判据是打开频次，不是错误文本；
// 阈值默认值来自 30 天生产数据校准（CHANNEL_BREAKER_SPEC.md §6），调参前先复跑校准 SQL ----

func IsChannelBreakerPenaltyEnabled() bool {
	return channelBreakerPenaltyEnabledFlag.Load()
}

func SetChannelBreakerPenaltyEnabled(enabled bool) {
	channelBreakerPenaltyEnabledFlag.Store(enabled)
}

func IsChannelBreakerPenaltyOfflineEnabled() bool {
	return channelBreakerPenaltyOfflineFlag.Load()
}

func SetChannelBreakerPenaltyOfflineEnabled(enabled bool) {
	channelBreakerPenaltyOfflineFlag.Store(enabled)
}

func GetChannelBreakerPenaltyAlertOpensPerHour() int {
	return int(channelBreakerPenaltyAlertOpens.Load())
}

func SetChannelBreakerPenaltyAlertOpensPerHour(value int) {
	if value <= 0 {
		value = 10
	}
	channelBreakerPenaltyAlertOpens.Store(int64(value))
}

func GetChannelBreakerPenaltyOfflineConsecutiveHours() int {
	return int(channelBreakerPenaltyOfflineHours.Load())
}

func SetChannelBreakerPenaltyOfflineConsecutiveHours(value int) {
	if value <= 0 {
		value = 2
	}
	channelBreakerPenaltyOfflineHours.Store(int64(value))
}

func GetChannelBreakerPenaltyMinPoolSize() int {
	return int(channelBreakerPenaltyMinPoolSize.Load())
}

// SetChannelBreakerPenaltyMinPoolSize 0 表示关闭池保护，负值视为未配置回退默认值。
func SetChannelBreakerPenaltyMinPoolSize(value int) {
	if value < 0 {
		value = 2
	}
	channelBreakerPenaltyMinPoolSize.Store(int64(value))
}

// ---- 冷却退避：连续熔断冷却按倍率阶梯递增，稳定超过衰减窗后回到初值 ----

var defaultChannelBreakerBackoffMultipliers = []int{1, 2, 5, 15, 60}

func IsChannelBreakerBackoffEnabled() bool {
	return channelBreakerBackoffEnabledFlag.Load()
}

func SetChannelBreakerBackoffEnabled(enabled bool) {
	channelBreakerBackoffEnabledFlag.Store(enabled)
}

func GetChannelBreakerBackoffMultipliers() []int {
	multipliers, ok := channelBreakerBackoffMultiplier.Load().([]int)
	if !ok || len(multipliers) == 0 {
		return append([]int(nil), defaultChannelBreakerBackoffMultipliers...)
	}
	return append([]int(nil), multipliers...)
}

// parseChannelBreakerBackoffMultipliers 解析逗号/换行分隔的正整数阶梯；
// 空串按默认阶梯，任一项非法返回错误。
func parseChannelBreakerBackoffMultipliers(value string) ([]int, error) {
	parts := ParseChannelBreakerList(value)
	if len(parts) == 0 {
		return append([]int(nil), defaultChannelBreakerBackoffMultipliers...), nil
	}
	multipliers := make([]int, 0, len(parts))
	for _, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("冷却退避阶梯必须是正整数列表，非法项：%q", part)
		}
		multipliers = append(multipliers, n)
	}
	return multipliers, nil
}

// SetChannelBreakerBackoffMultipliers 内部初始化用：非法输入回退默认阶梯。
// 管理端配置入口请走 UpdateChannelBreakerBackoffMultipliersByString（拒绝非法值）。
func SetChannelBreakerBackoffMultipliers(value string) {
	multipliers, err := parseChannelBreakerBackoffMultipliers(value)
	if err != nil {
		multipliers = append([]int(nil), defaultChannelBreakerBackoffMultipliers...)
	}
	channelBreakerBackoffMultiplier.Store(multipliers)
}

// UpdateChannelBreakerBackoffMultipliersByString 校验并更新阶梯；
// 非法输入返回错误且不改动现值，避免坏配置静默生效。
func UpdateChannelBreakerBackoffMultipliersByString(value string) error {
	multipliers, err := parseChannelBreakerBackoffMultipliers(value)
	if err != nil {
		return err
	}
	channelBreakerBackoffMultiplier.Store(multipliers)
	return nil
}

func ChannelBreakerBackoffMultipliersToString() string {
	multipliers := GetChannelBreakerBackoffMultipliers()
	parts := make([]string, 0, len(multipliers))
	for _, m := range multipliers {
		parts = append(parts, strconv.Itoa(m))
	}
	return strings.Join(parts, ",")
}

func GetChannelBreakerBackoffMaxCooldownSeconds() int {
	return int(channelBreakerBackoffMaxCooldown.Load())
}

func SetChannelBreakerBackoffMaxCooldownSeconds(value int) {
	if value <= 0 {
		value = 3600
	}
	channelBreakerBackoffMaxCooldown.Store(int64(value))
}

func GetChannelBreakerBackoffDecaySeconds() int {
	return int(channelBreakerBackoffDecaySecs.Load())
}

func SetChannelBreakerBackoffDecaySeconds(value int) {
	if value <= 0 {
		value = 600
	}
	channelBreakerBackoffDecaySecs.Store(int64(value))
}

// ---- 熔断豁免渠道：集中名单（报错不触发熔断与自动禁用，余额不足仍会禁用）----

func GetChannelBreakerExemptChannels() []int {
	m, ok := channelBreakerExemptChannel.Load().(map[int]struct{})
	if !ok || len(m) == 0 {
		return []int{}
	}
	ids := make([]int, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

func IsChannelBreakerExemptChannel(channelId int) bool {
	if channelId <= 0 {
		return false
	}
	m, ok := channelBreakerExemptChannel.Load().(map[int]struct{})
	if !ok {
		return false
	}
	_, exist := m[channelId]
	return exist
}

func SetChannelBreakerExemptChannels(ids []int) {
	m := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		if id > 0 {
			m[id] = struct{}{}
		}
	}
	channelBreakerExemptChannel.Store(m)
}

func ChannelBreakerExemptChannelsToJSONString() string {
	data, err := Marshal(GetChannelBreakerExemptChannels())
	if err != nil {
		return "[]"
	}
	return string(data)
}

func UpdateChannelBreakerExemptChannelsByJSONString(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		SetChannelBreakerExemptChannels(nil)
		return nil
	}
	var ids []int
	if err := UnmarshalJsonStr(value, &ids); err != nil {
		return err
	}
	SetChannelBreakerExemptChannels(ids)
	return nil
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

// ---- 2026-08-25 新增：客户端错误归正 / 全熔断兜底 / 转换仅非流式 ----

// IsUpstreamClientErrNormalizeEnabled 上游把 400 包成 5xx 时是否归正。
func IsUpstreamClientErrNormalizeEnabled() bool {
	return upstreamClientErrNormalizeFlag.Load()
}

func SetUpstreamClientErrNormalize(enabled bool) {
	upstreamClientErrNormalizeFlag.Store(enabled)
}

// IsBreakerAllOpenFallbackEnabled 所有候选都被熔断挡住时是否降级重选。
func IsBreakerAllOpenFallbackEnabled() bool {
	return breakerAllOpenFallbackFlag.Load()
}

func SetBreakerAllOpenFallback(enabled bool) {
	breakerAllOpenFallbackFlag.Store(enabled)
}

// IsChatToResponsesNonStreamOnly chat→Responses 转换是否只对非流式生效。
func IsChatToResponsesNonStreamOnly() bool {
	return chatToResponsesNonStreamOnlyFlag.Load()
}

func SetChatToResponsesNonStreamOnly(enabled bool) {
	chatToResponsesNonStreamOnlyFlag.Store(enabled)
}
