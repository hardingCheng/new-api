package service

import (
	"context"
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

const (
	ChannelBreakerStateClosed   = "closed"
	ChannelBreakerStateOpen     = "open"
	ChannelBreakerStateHalfOpen = "half-open"
)

var (
	channelBreakerMu       sync.Mutex
	channelBreakerStates   = map[string]*channelBreakerState{}
	channelBreakerCooldown = time.Duration(0)
)

type channelBreakerState struct {
	State          string
	Failures       int
	OpenedAt       time.Time
	ProbeStartedAt time.Time
	ProbeInFlight  int
	ProbeTotal     int
	ProbeSuccess   int
	RuleId         string
	RuleName       string
	Group          string
	Model          string
	CooldownSecs   int
	RuleProbeCount int
	RuleProbeNeed  int
}

type ChannelBreakerProbe struct {
	Key string
}

type ChannelBreakerStatus struct {
	StateKey              string     `json:"state_key"`
	ChannelId             int        `json:"channel_id"`
	KeyHash               string     `json:"key_hash,omitempty"`
	State                 string     `json:"state"`
	Failures              int        `json:"failures"`
	OpenedAt              *time.Time `json:"opened_at,omitempty"`
	ProbeStartedAt        *time.Time `json:"probe_started_at,omitempty"`
	ProbeInFlight         int        `json:"probe_in_flight"`
	ProbeTotal            int        `json:"probe_total"`
	ProbeSuccess          int        `json:"probe_success"`
	CooldownSeconds       int        `json:"cooldown_seconds"`
	CooldownRemainingSecs int        `json:"cooldown_remaining_seconds"`
	RuleId                string     `json:"rule_id,omitempty"`
	RuleName              string     `json:"rule_name,omitempty"`
	Group                 string     `json:"group,omitempty"`
	Model                 string     `json:"model,omitempty"`
	RuleProbeCount        int        `json:"rule_probe_count,omitempty"`
	RuleProbeSuccessCount int        `json:"rule_probe_success_count,omitempty"`
}

type channelBreakerRuntimeRule struct {
	Id                        string
	Name                      string
	Enabled                   bool
	FailureLimit              int
	Cooldown                  time.Duration
	ProbeCount                int
	ProbeSuccessCount         int
	FailureStatusCodes        []operation_setting.StatusCodeRange
	FailureKeywords           []string
	ExcludePaths              []string
	DisableBreaker            bool
	OnlyKeyBreaker            bool
	IgnoreClientError4xx      bool
	InstantDisableEnabled     bool
	InstantDisableStatusCodes []operation_setting.StatusCodeRange
	InstantDisableKeywords    []string
}

func ShouldExcludeChannelBreaker(c *gin.Context) bool {
	return shouldExcludeChannelBreakerByRule(c, resolveChannelBreakerRule(c, types.ChannelError{}))
}

func CanUseChannelByBreaker(c *gin.Context, channelError types.ChannelError) bool {
	rule := resolveChannelBreakerRule(c, channelError)
	if !rule.Enabled || shouldExcludeChannelBreakerByRule(c, rule) || !channelError.AutoBan {
		return true
	}
	channelError, ok := normalizeChannelBreakerTarget(channelError, rule)
	if !ok {
		return true
	}
	key := channelBreakerKey(channelError)

	channelBreakerMu.Lock()
	defer channelBreakerMu.Unlock()

	state := loadChannelBreakerStateLocked(key)
	if state == nil || state.State == ChannelBreakerStateClosed {
		return true
	}
	if state.State == ChannelBreakerStateOpen {
		return time.Since(state.OpenedAt) >= rule.Cooldown
	}
	if state.State != ChannelBreakerStateHalfOpen {
		return true
	}
	return state.ProbeTotal+state.ProbeInFlight < rule.ProbeCount
}

func AcquireChannelBreakerProbe(c *gin.Context, channelError types.ChannelError) bool {
	rule := resolveChannelBreakerRule(c, channelError)
	if !rule.Enabled || shouldExcludeChannelBreakerByRule(c, rule) || !channelError.AutoBan {
		return true
	}
	channelError, ok := normalizeChannelBreakerTarget(channelError, rule)
	if !ok {
		return true
	}
	key := channelBreakerKey(channelError)

	channelBreakerMu.Lock()
	defer channelBreakerMu.Unlock()

	state := loadChannelBreakerStateLocked(key)
	if state == nil || state.State == ChannelBreakerStateClosed {
		return true
	}
	if state.State == ChannelBreakerStateOpen {
		if time.Since(state.OpenedAt) < rule.Cooldown {
			return false
		}
		state.State = ChannelBreakerStateHalfOpen
		state.ProbeStartedAt = time.Now()
		state.ProbeInFlight = 0
		state.ProbeTotal = 0
		state.ProbeSuccess = 0
		saveChannelBreakerStateLocked(key, state)
	}
	if state.State == ChannelBreakerStateHalfOpen {
		if state.ProbeTotal+state.ProbeInFlight >= rule.ProbeCount {
			return false
		}
		state.ProbeInFlight++
		saveChannelBreakerStateLocked(key, state)
		if c != nil {
			c.Set("channel_breaker_probe", ChannelBreakerProbe{Key: key})
		}
	}
	return true
}

func AllowChannelByBreaker(c *gin.Context, channelError types.ChannelError) bool {
	if !CanUseChannelByBreaker(c, channelError) {
		return false
	}
	return AcquireChannelBreakerProbe(c, channelError)
}

func RecordChannelBreakerFailure(c *gin.Context, channelError types.ChannelError, shouldTrip bool) (bool, string) {
	rule := resolveChannelBreakerRule(c, channelError)
	if !rule.Enabled || shouldExcludeChannelBreakerByRule(c, rule) || !channelError.AutoBan {
		return false, ""
	}
	channelError, ok := normalizeChannelBreakerTarget(channelError, rule)
	if !ok {
		return false, ""
	}
	key := channelBreakerKey(channelError)

	channelBreakerMu.Lock()
	defer channelBreakerMu.Unlock()

	state := getOrCreateChannelBreakerState(key, c, rule)
	if !shouldTrip {
		if state.State == ChannelBreakerStateHalfOpen {
			recordProbeLocked(state, true)
			return evaluateProbeLocked(key, state, rule)
		}
		return false, ""
	}
	if state.State == ChannelBreakerStateHalfOpen {
		recordProbeLocked(state, false)
		return evaluateProbeLocked(key, state, rule)
	}
	state.Failures++
	failureLimit := rule.FailureLimit
	if state.Failures < failureLimit {
		saveChannelBreakerStateLocked(key, state)
		return false, fmt.Sprintf("channel breaker pending (failures: %d/%d)", state.Failures, failureLimit)
	}
	recordChannelBreakerOpenLog(key, state, "channel breaker opened")
	openBreakerLocked(state)
	saveChannelBreakerStateLocked(key, state)
	return true, "channel breaker opened"
}

func RecordChannelBreakerSuccess(c *gin.Context, channelError types.ChannelError) {
	rule := resolveChannelBreakerRule(c, channelError)
	if !rule.Enabled || shouldExcludeChannelBreakerByRule(c, rule) {
		return
	}
	channelError, ok := normalizeChannelBreakerTarget(channelError, rule)
	if !ok {
		return
	}
	key := channelBreakerKey(channelError)

	channelBreakerMu.Lock()
	defer channelBreakerMu.Unlock()

	state := loadChannelBreakerStateLocked(key)
	if state == nil {
		return
	}
	if state.State == ChannelBreakerStateHalfOpen {
		recordProbeLocked(state, true)
		_, _ = evaluateProbeLocked(key, state, rule)
		return
	}
	deleteChannelBreakerStateLocked(key)
}

func IsChannelBreakerOpenForKey(channelId int, usingKey string) bool {
	key := channelBreakerKey(types.ChannelError{ChannelId: channelId, UsingKey: usingKey})
	channelBreakerMu.Lock()
	defer channelBreakerMu.Unlock()

	state := loadChannelBreakerStateLocked(key)
	if state == nil || state.State == ChannelBreakerStateClosed {
		return false
	}
	if state.State == ChannelBreakerStateOpen && time.Since(state.OpenedAt) >= getChannelBreakerCooldown() {
		return false
	}
	if state.State == ChannelBreakerStateHalfOpen && state.ProbeTotal+state.ProbeInFlight < common.GetChannelBreakerProbeCount() {
		return false
	}
	return true
}

func ShouldTripChannelBreakerWithRule(c *gin.Context, channelError types.ChannelError, err *types.NewAPIError) bool {
	if err == nil {
		return false
	}
	rule := resolveChannelBreakerRule(c, channelError)
	if !rule.Enabled || rule.DisableBreaker {
		return false
	}
	if types.IsChannelError(err) {
		return true
	}
	if types.IsSkipRetryError(err) {
		return false
	}
	if rule.IgnoreClientError4xx && err.StatusCode >= 400 && err.StatusCode <= 499 {
		return false
	}
	if shouldMatchStatusCodeRanges(rule.FailureStatusCodes, err.StatusCode) {
		return true
	}
	lowerMessage := strings.ToLower(err.Error())
	search, _ := AcSearch(lowerMessage, rule.FailureKeywords, true)
	return search
}

// shouldInstantDisableChannel 判断错误是否命中"立即禁用"规则。
// 立即禁用为开箱即用的内置保护，独立于全局熔断开关。
// 命中条件（AND 语义）：InstantDisableEnabled && 状态码命中 && 关键词命中。
// 关键安全约束：
//   - 我方预扣费 / 用户额度不足等错误均带 skipRetry，绝不能据此禁用上游渠道；
//   - 余额耗尽是终态（充值前会持续 403），无视渠道 AutoBan 设置强制禁用，
//     与普通失败计数熔断（仍遵循 AutoBan）不同。
func shouldInstantDisableChannel(c *gin.Context, channelError types.ChannelError, err *types.NewAPIError) (bool, string, channelBreakerRuntimeRule) {
	var emptyRule channelBreakerRuntimeRule
	if err == nil {
		return false, "", emptyRule
	}
	// 我方扣费失败 / 用户额度不足（insufficient_user_quota）均带 skipRetry，排除之
	if types.IsSkipRetryError(err) {
		return false, "", emptyRule
	}
	// 结构化兜底：即使将来某处我方额度错误漏标 skipRetry，只要它是 NewAPIError 且
	// 携带我方内部额度码，也一律排除。注意必须 type+码"双中"：上游若为 new-api，其
	// 余额错误码同为 insufficient_user_quota，但 errorType 为 OpenAIError/ClaudeError，
	// 不会命中这里，因此不会误伤"上游账号没钱"这一最该禁用的场景。
	if isOwnQuotaError(err) {
		return false, "", emptyRule
	}
	// 立即禁用独立于全局熔断开关，使用专用解析；DisableBreaker/排除路径仍是退出口
	rule := resolveInstantDisableRule(c, channelError)
	if !rule.InstantDisableEnabled || shouldExcludeChannelBreakerByRule(c, rule) {
		return false, "", emptyRule
	}
	// AND 语义：状态码与关键词都必须配置且都命中，避免误伤
	if len(rule.InstantDisableStatusCodes) == 0 || len(rule.InstantDisableKeywords) == 0 {
		return false, "", emptyRule
	}
	if !shouldMatchStatusCodeRanges(rule.InstantDisableStatusCodes, err.StatusCode) {
		return false, "", emptyRule
	}
	matched, keywords := AcSearch(strings.ToLower(err.Error()), rule.InstantDisableKeywords, true)
	if !matched {
		return false, "", emptyRule
	}
	reason := fmt.Sprintf("命中立即禁用规则「%s」(status_code=%d, keyword=%s)", rule.Name, err.StatusCode, strings.Join(keywords, ","))
	return true, reason, rule
}

// isOwnQuotaError 判断错误是否为"我方自己生成的额度/扣费错误"。
// 仅当 errorType 为我方 NewAPIError 且错误码是内部额度码时才成立，
// 以避免误伤上游（尤其上游本身是 new-api）返回的同名错误码。
func isOwnQuotaError(err *types.NewAPIError) bool {
	if err.GetErrorType() != types.ErrorTypeNewAPIError {
		return false
	}
	switch err.GetErrorCode() {
	case types.ErrorCodeInsufficientUserQuota, types.ErrorCodePreConsumeTokenQuotaFailed:
		return true
	}
	return false
}

// HandleInstantDisableChannel 命中立即禁用规则时执行：永久禁用整个渠道 + 记录熔断历史。
// 返回是否已处理；已处理时调用方应跳过普通熔断失败计数。
func HandleInstantDisableChannel(c *gin.Context, channelError types.ChannelError, err *types.NewAPIError) (bool, string) {
	should, reason, rule := shouldInstantDisableChannel(c, channelError, err)
	if !should {
		return false, ""
	}
	// 余额是账号级，始终禁用整个渠道（UsingKey 置空）。
	// 余额耗尽是终态，强制 AutoBan=true 以绕过 DisableChannel 内部的 AutoBan 拦截。
	wholeChannel := channelError
	wholeChannel.UsingKey = ""
	wholeChannel.AutoBan = true
	DisableChannel(wholeChannel, reason)

	model.RecordChannelBreakerLog(&model.ChannelBreakerLog{
		ChannelId:  channelError.ChannelId,
		RuleId:     rule.Id,
		RuleName:   rule.Name,
		UsingGroup: channelBreakerContextGroup(c),
		ModelName:  channelBreakerContextModel(c),
		Reason:     reason,
	})
	return true, reason
}

func ClearChannelBreaker(channelError types.ChannelError) {
	channelBreakerMu.Lock()
	defer channelBreakerMu.Unlock()
	deleteChannelBreakerStateLocked(channelBreakerKey(channelError))
}

func ClearChannelBreakerByStateKey(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	channelBreakerMu.Lock()
	defer channelBreakerMu.Unlock()
	if loadChannelBreakerStateLocked(key) == nil {
		return false
	}
	deleteChannelBreakerStateLocked(key)
	return true
}

func ListChannelBreakerStatuses() []ChannelBreakerStatus {
	channelBreakerMu.Lock()
	defer channelBreakerMu.Unlock()

	keys := listChannelBreakerKeysLocked()
	statuses := make([]ChannelBreakerStatus, 0, len(keys))
	for _, key := range keys {
		state := loadChannelBreakerStateLocked(key)
		if state == nil || state.State == ChannelBreakerStateClosed {
			continue
		}
		statuses = append(statuses, buildChannelBreakerStatus(key, state))
	}
	return statuses
}

func GetChannelBreakerFailureThreshold() int {
	return common.GetChannelBreakerFailureLimit()
}

func getOrCreateChannelBreakerState(key string, c *gin.Context, rule channelBreakerRuntimeRule) *channelBreakerState {
	state := loadChannelBreakerStateLocked(key)
	if state == nil {
		state = &channelBreakerState{State: ChannelBreakerStateClosed}
		saveChannelBreakerStateLocked(key, state)
	}
	state.RuleId = rule.Id
	state.RuleName = rule.Name
	state.Group = channelBreakerContextGroup(c)
	state.Model = channelBreakerContextModel(c)
	state.CooldownSecs = int(rule.Cooldown.Seconds())
	state.RuleProbeCount = rule.ProbeCount
	state.RuleProbeNeed = rule.ProbeSuccessCount
	return state
}

func recordProbeLocked(state *channelBreakerState, success bool) {
	if state.ProbeInFlight > 0 {
		state.ProbeInFlight--
	}
	state.ProbeTotal++
	if success {
		state.ProbeSuccess++
	}
}

func evaluateProbeLocked(key string, state *channelBreakerState, rule channelBreakerRuntimeRule) (bool, string) {
	probeSuccesses := rule.ProbeSuccessCount
	probeRequests := rule.ProbeCount
	if state.ProbeSuccess >= probeSuccesses {
		deleteChannelBreakerStateLocked(key)
		return false, "channel breaker closed after probe"
	}
	if state.ProbeTotal >= probeRequests {
		recordChannelBreakerOpenLog(key, state, fmt.Sprintf("channel breaker remains open after probe (%d/%d successes)", state.ProbeSuccess, state.ProbeTotal))
		openBreakerLocked(state)
		saveChannelBreakerStateLocked(key, state)
		return true, fmt.Sprintf("channel breaker remains open after probe (%d/%d successes)", state.ProbeSuccess, state.ProbeTotal)
	}
	saveChannelBreakerStateLocked(key, state)
	return false, fmt.Sprintf("channel breaker probing (%d/%d successes, %d/%d completed)", state.ProbeSuccess, probeSuccesses, state.ProbeTotal, probeRequests)
}

// recordChannelBreakerOpenLog 在熔断器打开时异步记录一条历史日志。
// 必须在 openBreakerLocked 之前调用，以便捕获被重置前的 Failures。
func recordChannelBreakerOpenLog(key string, state *channelBreakerState, reason string) {
	channelId, keyHash := parseChannelBreakerKey(key)
	model.RecordChannelBreakerLog(&model.ChannelBreakerLog{
		ChannelId:    channelId,
		KeyHash:      keyHash,
		RuleId:       state.RuleId,
		RuleName:     state.RuleName,
		UsingGroup:   state.Group,
		ModelName:    state.Model,
		Failures:     state.Failures,
		CooldownSecs: state.CooldownSecs,
		Reason:       reason,
	})
}

func openBreakerLocked(state *channelBreakerState) {
	state.State = ChannelBreakerStateOpen
	state.Failures = 0
	state.OpenedAt = time.Now()
	state.ProbeStartedAt = time.Time{}
	state.ProbeInFlight = 0
	state.ProbeTotal = 0
	state.ProbeSuccess = 0
}

func channelBreakerKey(channelError types.ChannelError) string {
	if channelError.UsingKey == "" {
		return fmt.Sprintf("%d", channelError.ChannelId)
	}
	return fmt.Sprintf("%d:%s", channelError.ChannelId, ChannelBreakerKeyHash(channelError.UsingKey))
}

func ChannelBreakerKeyHash(key string) string {
	return fmt.Sprintf("%x", hashString64(key))
}

func hashString64(s string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return h.Sum64()
}

func getChannelBreakerCooldown() time.Duration {
	if channelBreakerCooldown > 0 {
		return channelBreakerCooldown
	}
	return time.Duration(common.GetChannelBreakerCooldownSeconds()) * time.Second
}

func channelBreakerRedisEnabled() bool {
	return common.RedisEnabled && common.RDB != nil
}

func channelBreakerRedisKey(key string) string {
	return "channel_breaker:" + key
}

func listChannelBreakerKeysLocked() []string {
	keySet := make(map[string]struct{})
	for key := range channelBreakerStates {
		keySet[key] = struct{}{}
	}
	if channelBreakerRedisEnabled() {
		iter := common.RDB.Scan(context.Background(), 0, "channel_breaker:*", 100).Iterator()
		for iter.Next(context.Background()) {
			key := strings.TrimPrefix(iter.Val(), "channel_breaker:")
			if key != "" {
				keySet[key] = struct{}{}
			}
		}
		if err := iter.Err(); err != nil {
			common.SysError("failed to scan channel breaker keys from redis: " + err.Error())
		}
	}
	keys := make([]string, 0, len(keySet))
	for key := range keySet {
		keys = append(keys, key)
	}
	return keys
}

func buildChannelBreakerStatus(key string, state *channelBreakerState) ChannelBreakerStatus {
	channelId, keyHash := parseChannelBreakerKey(key)
	cooldown := getChannelBreakerCooldown()
	if state.CooldownSecs > 0 {
		cooldown = time.Duration(state.CooldownSecs) * time.Second
	}
	status := ChannelBreakerStatus{
		StateKey:              key,
		ChannelId:             channelId,
		KeyHash:               keyHash,
		State:                 state.State,
		Failures:              state.Failures,
		ProbeInFlight:         state.ProbeInFlight,
		ProbeTotal:            state.ProbeTotal,
		ProbeSuccess:          state.ProbeSuccess,
		CooldownSeconds:       int(cooldown.Seconds()),
		RuleId:                state.RuleId,
		RuleName:              state.RuleName,
		Group:                 state.Group,
		Model:                 state.Model,
		RuleProbeCount:        state.RuleProbeCount,
		RuleProbeSuccessCount: state.RuleProbeNeed,
	}
	if !state.OpenedAt.IsZero() {
		openedAt := state.OpenedAt
		status.OpenedAt = &openedAt
		remaining := cooldown - time.Since(state.OpenedAt)
		if remaining > 0 {
			status.CooldownRemainingSecs = int(remaining.Seconds())
			if remaining%time.Second != 0 {
				status.CooldownRemainingSecs++
			}
		}
	}
	if !state.ProbeStartedAt.IsZero() {
		probeStartedAt := state.ProbeStartedAt
		status.ProbeStartedAt = &probeStartedAt
	}
	return status
}

func parseChannelBreakerKey(key string) (int, string) {
	parts := strings.SplitN(key, ":", 2)
	channelId, _ := strconv.Atoi(parts[0])
	if len(parts) == 2 {
		return channelId, parts[1]
	}
	return channelId, ""
}

func resolveChannelBreakerRule(c *gin.Context, channelError types.ChannelError) channelBreakerRuntimeRule {
	fallback := defaultChannelBreakerRuntimeRule()
	if !fallback.Enabled {
		return fallback
	}
	return resolveBestChannelBreakerRule(c, channelError, fallback)
}

// resolveInstantDisableRule 解析"立即禁用"使用的规则。
// 立即禁用是开箱即用的内置保护，独立于全局熔断开关：
// 即使全局熔断关闭，余额不足类错误仍会命中内置规则并立即禁用渠道。
func resolveInstantDisableRule(c *gin.Context, channelError types.ChannelError) channelBreakerRuntimeRule {
	return resolveBestChannelBreakerRule(c, channelError, defaultChannelBreakerRuntimeRule())
}

func resolveBestChannelBreakerRule(c *gin.Context, channelError types.ChannelError, fallback channelBreakerRuntimeRule) channelBreakerRuntimeRule {
	rules := common.GetChannelBreakerRules()
	if len(rules) == 0 {
		return fallback
	}
	group := channelBreakerContextGroup(c)
	modelName := channelBreakerContextModel(c)
	channelId := strconv.Itoa(channelError.ChannelId)
	bestPriority := -1
	bestIndex := -1
	for i, rule := range rules {
		if !rule.Enabled {
			continue
		}
		priority := channelBreakerRulePriority(rule, group, modelName, channelId)
		if priority > bestPriority {
			bestPriority = priority
			bestIndex = i
		}
	}
	if bestIndex < 0 {
		return fallback
	}
	return runtimeRuleFromConfig(rules[bestIndex], fallback)
}

func channelBreakerRulePriority(rule common.ChannelBreakerRule, group, modelName, channelId string) int {
	switch rule.Scope {
	case common.ChannelBreakerScopeChannel:
		if channelId != "" && containsString(rule.Targets, channelId) {
			return 4
		}
	case common.ChannelBreakerScopeModel:
		if modelName != "" && containsString(rule.Targets, modelName) {
			return 3
		}
	case common.ChannelBreakerScopeGroup:
		if group != "" && containsString(rule.Targets, group) {
			return 2
		}
	case common.ChannelBreakerScopeGlobal:
		return 1
	}
	return -1
}

func runtimeRuleFromConfig(rule common.ChannelBreakerRule, fallback channelBreakerRuntimeRule) channelBreakerRuntimeRule {
	runtimeRule := fallback
	runtimeRule.Id = rule.Id
	runtimeRule.Name = rule.Name
	runtimeRule.Enabled = rule.Enabled
	runtimeRule.FailureLimit = rule.FailureLimit
	runtimeRule.Cooldown = time.Duration(rule.CooldownSeconds) * time.Second
	runtimeRule.ProbeCount = rule.ProbeCount
	runtimeRule.ProbeSuccessCount = rule.ProbeSuccessCount
	runtimeRule.DisableBreaker = rule.DisableBreaker
	runtimeRule.OnlyKeyBreaker = rule.OnlyKeyBreaker
	runtimeRule.IgnoreClientError4xx = rule.IgnoreClientError4xx
	// 立即禁用：自定义规则未显式开启时，继承内置（fallback）的开箱即用保护；
	// 显式开启后才用规则自带的状态码/关键词覆盖，未填则仍沿用内置默认。
	if rule.InstantDisableEnabled {
		runtimeRule.InstantDisableEnabled = true
		if ranges, err := operation_setting.ParseHTTPStatusCodeRanges(rule.InstantDisableStatusCodes); err == nil && len(ranges) > 0 {
			runtimeRule.InstantDisableStatusCodes = ranges
		}
		if keywords := common.ParseChannelBreakerList(rule.InstantDisableKeywords); len(keywords) > 0 {
			runtimeRule.InstantDisableKeywords = normalizeKeywords(keywords)
		}
	}
	if ranges, err := operation_setting.ParseHTTPStatusCodeRanges(rule.FailureStatusCodes); err == nil && len(ranges) > 0 {
		runtimeRule.FailureStatusCodes = ranges
	}
	if keywords := common.ParseChannelBreakerList(rule.FailureKeywords); len(keywords) > 0 {
		runtimeRule.FailureKeywords = normalizeKeywords(keywords)
	}
	if paths := common.ParseChannelBreakerList(rule.ExcludePaths); len(paths) > 0 {
		runtimeRule.ExcludePaths = paths
	}
	return runtimeRule
}

// defaultInstantDisableKeywordList 内置"立即禁用"关键词：上游渠道账号余额耗尽时的典型返回。
// 与状态码 403 取 AND 语义，开箱即用，无需管理员配置。
var defaultInstantDisableKeywordList = []string{
	"insufficient account balance",
	"insufficient user quota",
	"insufficient_user_quota",
	"预扣费额度失败",
}

func defaultInstantDisableStatusRanges() []operation_setting.StatusCodeRange {
	ranges, _ := operation_setting.ParseHTTPStatusCodeRanges("403")
	return ranges
}

func defaultInstantDisableKeywords() []string {
	return normalizeKeywords(defaultInstantDisableKeywordList)
}

func defaultChannelBreakerRuntimeRule() channelBreakerRuntimeRule {
	return channelBreakerRuntimeRule{
		Id:                        "global-default",
		Name:                      "全局默认规则",
		Enabled:                   common.IsChannelBreakerEnabled(),
		FailureLimit:              common.GetChannelBreakerFailureLimit(),
		Cooldown:                  getChannelBreakerCooldown(),
		ProbeCount:                common.GetChannelBreakerProbeCount(),
		ProbeSuccessCount:         common.GetChannelBreakerProbeSuccessCount(),
		FailureStatusCodes:        operation_setting.GetChannelBreakerFailureStatusCodeRanges(),
		FailureKeywords:           operation_setting.GetAutomaticDisableKeywords(),
		ExcludePaths:              common.GetChannelBreakerExcludePaths(),
		InstantDisableEnabled:     true,
		InstantDisableStatusCodes: defaultInstantDisableStatusRanges(),
		InstantDisableKeywords:    defaultInstantDisableKeywords(),
	}
}

func shouldExcludeChannelBreakerByRule(c *gin.Context, rule channelBreakerRuntimeRule) bool {
	if rule.DisableBreaker {
		return true
	}
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return false
	}
	path := c.Request.URL.Path
	for _, excludePath := range rule.ExcludePaths {
		if excludePath != "" && strings.HasPrefix(path, excludePath) {
			return true
		}
	}
	return false
}

func normalizeChannelBreakerTarget(channelError types.ChannelError, rule channelBreakerRuntimeRule) (types.ChannelError, bool) {
	if rule.OnlyKeyBreaker && channelError.UsingKey == "" {
		return channelError, false
	}
	return channelError, true
}

func channelBreakerContextGroup(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if group := common.GetContextKeyString(c, constant.ContextKeyUsingGroup); group != "" {
		return group
	}
	if group := common.GetContextKeyString(c, constant.ContextKeyTokenGroup); group != "" {
		return group
	}
	return common.GetContextKeyString(c, constant.ContextKeyUserGroup)
}

func channelBreakerContextModel(c *gin.Context) string {
	if c == nil {
		return ""
	}
	return common.GetContextKeyString(c, constant.ContextKeyOriginalModel)
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func normalizeKeywords(keywords []string) []string {
	normalized := make([]string, 0, len(keywords))
	for _, keyword := range keywords {
		keyword = strings.ToLower(strings.TrimSpace(keyword))
		if keyword != "" {
			normalized = append(normalized, keyword)
		}
	}
	return normalized
}

func shouldMatchStatusCodeRanges(ranges []operation_setting.StatusCodeRange, code int) bool {
	if code < 100 || code > 599 {
		return false
	}
	for _, r := range ranges {
		if code < r.Start {
			return false
		}
		if code <= r.End {
			return true
		}
	}
	return false
}

func loadChannelBreakerStateLocked(key string) *channelBreakerState {
	if channelBreakerRedisEnabled() {
		var state channelBreakerState
		data, err := common.RedisGet(channelBreakerRedisKey(key))
		if err == nil {
			if unmarshalErr := common.UnmarshalJsonStr(data, &state); unmarshalErr == nil {
				return &state
			}
		} else if err != redis.Nil {
			common.SysError("failed to load channel breaker from redis: " + err.Error())
		}
	}
	return channelBreakerStates[key]
}

func saveChannelBreakerStateLocked(key string, state *channelBreakerState) {
	if state == nil {
		deleteChannelBreakerStateLocked(key)
		return
	}
	channelBreakerStates[key] = state
	if !channelBreakerRedisEnabled() {
		return
	}
	data, err := common.Marshal(state)
	if err != nil {
		common.SysError("failed to marshal channel breaker: " + err.Error())
		return
	}
	ttl := getChannelBreakerCooldown() + time.Duration(common.GetChannelBreakerProbeCount()+common.GetChannelBreakerFailureLimit()+60)*time.Second
	if err := common.RDB.Set(context.Background(), channelBreakerRedisKey(key), string(data), ttl).Err(); err != nil {
		common.SysError("failed to save channel breaker to redis: " + err.Error())
	}
}

func deleteChannelBreakerStateLocked(key string) {
	delete(channelBreakerStates, key)
	if channelBreakerRedisEnabled() {
		if err := common.RDB.Del(context.Background(), channelBreakerRedisKey(key)).Err(); err != nil {
			common.SysError("failed to delete channel breaker from redis: " + err.Error())
		}
	}
}
