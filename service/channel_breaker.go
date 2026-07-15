package service

import (
	"context"
	"errors"
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

	channelBreakerMutationNone = iota
	channelBreakerMutationSave
	channelBreakerMutationDelete

	channelBreakerRedisMaxRetries = 32
)

var (
	channelBreakerMu               sync.Mutex
	channelBreakerStates           = map[string]*channelBreakerState{}
	channelBreakerCooldown         = time.Duration(0)
	channelBreakerProbeTimeoutMin  = 30 * time.Second
	errChannelBreakerRedisConflict = errors.New("channel breaker redis transaction conflict")
)

type channelBreakerState struct {
	State          string
	Generation     int64
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
	Key        string
	Generation int64
}

type channelBreakerMutation struct {
	Action int
	State  *channelBreakerState
	Value  any
	Event  *channelBreakerOpenEvent
}

type channelBreakerOpenEvent struct {
	Key    string
	State  channelBreakerState
	Reason string
}

type channelBreakerAllowResult struct {
	Allowed bool
	Probe   *ChannelBreakerProbe
}

type channelBreakerRecordResult struct {
	Opened  bool
	Message string
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
	Scope                     string
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
	if isChannelBreakerTargetQuarantined(channelError) {
		return false
	}
	rule := resolveChannelBreakerRule(c, channelError)
	if !rule.Enabled || shouldExcludeChannelBreakerByRule(c, rule) || !channelError.AutoBan || channelError.SkipBreaker {
		return true
	}
	channelError, ok := normalizeChannelBreakerTarget(channelError, rule)
	if !ok {
		return true
	}
	key := channelBreakerStateKey(c, channelError, rule)

	state, err := readChannelBreakerState(key)
	if err != nil {
		logChannelBreakerRedisFailure("read state", key, err)
		return true
	}
	if state == nil || state.State == ChannelBreakerStateClosed {
		return true
	}
	now, err := channelBreakerCurrentTime()
	if err != nil {
		logChannelBreakerRedisFailure("read server time", key, err)
		return true
	}
	if state.State == ChannelBreakerStateOpen {
		return now.Sub(state.OpenedAt) >= stateCooldown(state, rule)
	}
	if state.State != ChannelBreakerStateHalfOpen {
		return true
	}
	if isStaleHalfOpen(state, rule, now) {
		return false
	}
	return state.ProbeTotal+state.ProbeInFlight < stateProbeCount(state, rule)
}

func AcquireChannelBreakerProbe(c *gin.Context, channelError types.ChannelError) bool {
	if isChannelBreakerTargetQuarantined(channelError) {
		return false
	}
	rule := resolveChannelBreakerRule(c, channelError)
	if !rule.Enabled || shouldExcludeChannelBreakerByRule(c, rule) || !channelError.AutoBan || channelError.SkipBreaker {
		return true
	}
	channelError, ok := normalizeChannelBreakerTarget(channelError, rule)
	if !ok {
		return true
	}
	key := channelBreakerStateKey(c, channelError, rule)

	mutation, err := mutateChannelBreakerState(key, func(state *channelBreakerState, now time.Time) channelBreakerMutation {
		result := channelBreakerAllowResult{Allowed: true}
		if state == nil || state.State == ChannelBreakerStateClosed {
			return channelBreakerMutation{Action: channelBreakerMutationNone, Value: result}
		}
		var event *channelBreakerOpenEvent
		if isStaleHalfOpen(state, rule, now) {
			event = newChannelBreakerOpenEvent(key, state, fmt.Sprintf("channel breaker probe timed out (%d/%d successes, %d/%d completed)", state.ProbeSuccess, stateProbeSuccessCount(state, rule), state.ProbeTotal, stateProbeCount(state, rule)))
			openBreakerAt(state, now)
		}
		if state.State == ChannelBreakerStateOpen {
			if now.Sub(state.OpenedAt) < stateCooldown(state, rule) {
				result.Allowed = false
				return channelBreakerMutation{Action: channelBreakerMutationSave, State: state, Value: result, Event: event}
			}
			state.State = ChannelBreakerStateHalfOpen
			state.ProbeStartedAt = now
			state.ProbeInFlight = 0
			state.ProbeTotal = 0
			state.ProbeSuccess = 0
		}
		if state.State == ChannelBreakerStateHalfOpen {
			if state.ProbeTotal+state.ProbeInFlight >= stateProbeCount(state, rule) {
				result.Allowed = false
				return channelBreakerMutation{Action: channelBreakerMutationSave, State: state, Value: result, Event: event}
			}
			state.ProbeInFlight++
			result.Probe = &ChannelBreakerProbe{Key: key, Generation: state.Generation}
		}
		return channelBreakerMutation{Action: channelBreakerMutationSave, State: state, Value: result, Event: event}
	})
	if err != nil {
		logChannelBreakerRedisFailure("acquire probe", key, err)
		if errors.Is(err, errChannelBreakerRedisConflict) {
			return false
		}
		return true
	}
	recordChannelBreakerMutationEvent(mutation.Event)
	result, _ := mutation.Value.(channelBreakerAllowResult)
	if result.Probe != nil && c != nil {
		c.Set("channel_breaker_probe", *result.Probe)
	}
	return result.Allowed
}

func AllowChannelByBreaker(c *gin.Context, channelError types.ChannelError) bool {
	if !CanUseChannelByBreaker(c, channelError) {
		return false
	}
	return AcquireChannelBreakerProbe(c, channelError)
}

func RecordChannelBreakerFailure(c *gin.Context, channelError types.ChannelError, shouldTrip bool) (bool, string) {
	rule := resolveChannelBreakerRule(c, channelError)
	if !rule.Enabled || shouldExcludeChannelBreakerByRule(c, rule) || !channelError.AutoBan || channelError.SkipBreaker {
		return false, ""
	}
	channelError, ok := normalizeChannelBreakerTarget(channelError, rule)
	if !ok {
		return false, ""
	}
	key := channelBreakerStateKey(c, channelError, rule)

	probe, hasProbe := channelBreakerProbeFromContext(c, key)
	mutation, err := mutateChannelBreakerState(key, func(state *channelBreakerState, now time.Time) channelBreakerMutation {
		if state == nil {
			if !shouldTrip {
				return channelBreakerMutation{Action: channelBreakerMutationNone, Value: channelBreakerRecordResult{}}
			}
			state = &channelBreakerState{State: ChannelBreakerStateClosed}
		}
		applyChannelBreakerRuleContext(state, c, rule)
		if state.State == ChannelBreakerStateOpen {
			return channelBreakerMutation{Action: channelBreakerMutationNone, Value: channelBreakerRecordResult{}}
		}
		if state.State == ChannelBreakerStateHalfOpen {
			if !hasProbe || probe.Generation != state.Generation {
				return channelBreakerMutation{Action: channelBreakerMutationNone, Value: channelBreakerRecordResult{}}
			}
			if !shouldTrip {
				if state.ProbeInFlight > 0 {
					state.ProbeInFlight--
				}
				return channelBreakerMutation{Action: channelBreakerMutationSave, State: state, Value: channelBreakerRecordResult{}}
			}
			recordProbeLocked(state, false)
			return evaluateProbeMutation(key, state, rule, now)
		}
		if !shouldTrip {
			return channelBreakerMutation{Action: channelBreakerMutationNone, Value: channelBreakerRecordResult{}}
		}
		state.Failures++
		if state.Failures < rule.FailureLimit {
			message := fmt.Sprintf("channel breaker pending (failures: %d/%d)", state.Failures, rule.FailureLimit)
			return channelBreakerMutation{Action: channelBreakerMutationSave, State: state, Value: channelBreakerRecordResult{Message: message}}
		}
		event := newChannelBreakerOpenEvent(key, state, "channel breaker opened")
		openBreakerAt(state, now)
		return channelBreakerMutation{Action: channelBreakerMutationSave, State: state, Value: channelBreakerRecordResult{Opened: true, Message: "channel breaker opened"}, Event: event}
	})
	if err != nil {
		logChannelBreakerRedisFailure("record failure", key, err)
		return false, ""
	}
	recordChannelBreakerMutationEvent(mutation.Event)
	result, _ := mutation.Value.(channelBreakerRecordResult)
	return result.Opened, result.Message
}

func RecordChannelBreakerSuccess(c *gin.Context, channelError types.ChannelError) {
	rule := resolveChannelBreakerRule(c, channelError)
	if !rule.Enabled || shouldExcludeChannelBreakerByRule(c, rule) || channelError.SkipBreaker {
		return
	}
	channelError, ok := normalizeChannelBreakerTarget(channelError, rule)
	if !ok {
		return
	}
	key := channelBreakerStateKey(c, channelError, rule)

	probe, hasProbe := channelBreakerProbeFromContext(c, key)
	mutation, err := mutateChannelBreakerState(key, func(state *channelBreakerState, now time.Time) channelBreakerMutation {
		if state == nil {
			return channelBreakerMutation{Action: channelBreakerMutationNone}
		}
		switch state.State {
		case ChannelBreakerStateClosed:
			return channelBreakerMutation{Action: channelBreakerMutationDelete}
		case ChannelBreakerStateHalfOpen:
			if !hasProbe || probe.Generation != state.Generation {
				return channelBreakerMutation{Action: channelBreakerMutationNone}
			}
			recordProbeLocked(state, true)
			return evaluateProbeMutation(key, state, rule, now)
		default:
			// A request that started before OPEN must not close a newer breaker.
			return channelBreakerMutation{Action: channelBreakerMutationNone}
		}
	})
	if err != nil {
		logChannelBreakerRedisFailure("record success", key, err)
		return
	}
	recordChannelBreakerMutationEvent(mutation.Event)
}

func IsChannelBreakerOpenForKey(channelId int, usingKey string) bool {
	key := channelBreakerKey(types.ChannelError{ChannelId: channelId, UsingKey: usingKey})
	state, err := readChannelBreakerState(key)
	if err != nil {
		logChannelBreakerRedisFailure("read key state", key, err)
		return false
	}
	if state == nil || state.State == ChannelBreakerStateClosed {
		return false
	}
	now, err := channelBreakerCurrentTime()
	if err != nil {
		logChannelBreakerRedisFailure("read server time", key, err)
		return false
	}
	if state.State == ChannelBreakerStateOpen && now.Sub(state.OpenedAt) >= stateCooldown(state, defaultChannelBreakerRuntimeRule()) {
		return false
	}
	if state.State == ChannelBreakerStateHalfOpen && state.ProbeTotal+state.ProbeInFlight < stateProbeCount(state, defaultChannelBreakerRuntimeRule()) {
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
	if types.IsSkipRetryError(err) {
		return false
	}
	if types.IsChannelError(err) {
		return true
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
// 命中条件：InstantDisableEnabled && 状态码命中 &&（上游额度错误码命中 || 关键词命中）。
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
	if len(rule.InstantDisableStatusCodes) == 0 {
		return false, "", emptyRule
	}
	if !shouldMatchStatusCodeRanges(rule.InstantDisableStatusCodes, err.StatusCode) {
		return false, "", emptyRule
	}
	if isUpstreamQuotaError(err) {
		reason := fmt.Sprintf("命中立即禁用规则「%s」(status_code=%d, error_code=%s)", rule.Name, err.StatusCode, err.GetErrorCode())
		return true, reason, rule
	}
	if len(rule.InstantDisableKeywords) == 0 {
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
	return err != nil && err.GetErrorType() == types.ErrorTypeNewAPIError && isQuotaErrorCode(err.GetErrorCode())
}

func isUpstreamQuotaError(err *types.NewAPIError) bool {
	return err != nil && err.GetErrorType() != types.ErrorTypeNewAPIError && isQuotaErrorCode(err.GetErrorCode())
}

func isQuotaErrorCode(errorCode types.ErrorCode) bool {
	switch errorCode {
	case types.ErrorCodeInsufficientUserQuota, types.ErrorCodePreConsumeTokenQuotaFailed:
		return true
	default:
		return false
	}
}

// HandleInstantDisableChannel 命中立即禁用规则时执行：单 Key 渠道禁用渠道，
// 多 Key 渠道只禁用发生确定性故障的 Key。
// 返回是否已处理；已处理时调用方应跳过普通熔断失败计数。
func HandleInstantDisableChannel(c *gin.Context, channelError types.ChannelError, err *types.NewAPIError) (bool, string) {
	should, reason, rule := shouldInstantDisableChannel(c, channelError, err)
	if !should {
		return false, ""
	}
	// 多 Key 可能属于不同上游账号，一个 Key 余额耗尽不能误伤其他 Key。
	// UpdateChannelStatus 会在最后一个可用 Key 被禁用后自动禁用整个渠道。
	disableTarget := instantDisableTarget(channelError)
	markChannelBreakerTargetQuarantined(disableTarget)
	DisableChannelWithoutAutoRecovery(disableTarget, reason)

	model.RecordChannelBreakerLog(&model.ChannelBreakerLog{
		ChannelId:  channelError.ChannelId,
		KeyHash:    instantDisableKeyHash(channelError),
		RuleId:     rule.Id,
		RuleName:   rule.Name,
		UsingGroup: channelBreakerContextGroup(c),
		ModelName:  channelBreakerContextModel(c),
		Reason:     reason,
	})
	return true, reason
}

func instantDisableKeyHash(channelError types.ChannelError) string {
	if !channelError.IsMultiKey || channelError.UsingKey == "" {
		return ""
	}
	return ChannelBreakerKeyHash(channelError.UsingKey)
}

func instantDisableTarget(channelError types.ChannelError) types.ChannelError {
	if !channelError.IsMultiKey {
		channelError.UsingKey = ""
	}
	channelError.AutoBan = true
	return channelError
}

func channelBreakerQuarantineKey(channelError types.ChannelError) string {
	if channelError.ChannelId <= 0 {
		return ""
	}
	if channelError.IsMultiKey {
		if channelError.UsingKey == "" {
			return ""
		}
		return fmt.Sprintf("channel_breaker_quarantine:%d:%s", channelError.ChannelId, ChannelBreakerKeyHash(channelError.UsingKey))
	}
	return fmt.Sprintf("channel_breaker_quarantine:%d", channelError.ChannelId)
}

func channelBreakerQuarantineTTL() time.Duration {
	seconds := common.RedisKeyCacheSeconds()*2 + 10
	if seconds < 130 {
		seconds = 130
	}
	return time.Duration(seconds) * time.Second
}

func markChannelBreakerTargetQuarantined(channelError types.ChannelError) {
	key := channelBreakerQuarantineKey(channelError)
	if key == "" || !channelBreakerRedisEnabled() {
		return
	}
	if err := common.RDB.Set(context.Background(), key, "1", channelBreakerQuarantineTTL()).Err(); err != nil {
		common.SysError(fmt.Sprintf("failed to quarantine channel breaker target %s: %s", key, err.Error()))
	}
}

func MarkChannelBreakerQuarantine(channelId int, usingKey string, isMultiKey bool) {
	markChannelBreakerTargetQuarantined(types.ChannelError{ChannelId: channelId, UsingKey: usingKey, IsMultiKey: isMultiKey})
}

func isChannelBreakerTargetQuarantined(channelError types.ChannelError) bool {
	key := channelBreakerQuarantineKey(channelError)
	if key == "" || !channelBreakerRedisEnabled() {
		return false
	}
	exists, err := common.RDB.Exists(context.Background(), key).Result()
	if err != nil {
		common.SysError(fmt.Sprintf("failed to read channel breaker quarantine %s: %s", key, err.Error()))
		return false
	}
	return exists > 0
}

func ClearChannelBreakerQuarantine(channelId int, usingKey string, isMultiKey bool) {
	key := channelBreakerQuarantineKey(types.ChannelError{ChannelId: channelId, UsingKey: usingKey, IsMultiKey: isMultiKey})
	if key == "" || !channelBreakerRedisEnabled() {
		return
	}
	if err := common.RDB.Del(context.Background(), key).Err(); err != nil {
		common.SysError(fmt.Sprintf("failed to clear channel breaker quarantine %s: %s", key, err.Error()))
	}
}

func ClearChannelBreaker(channelError types.ChannelError) {
	keys := listChannelBreakerKeys()
	for _, key := range keys {
		if !channelBreakerStateMatchesTarget(key, channelError) {
			continue
		}
		if _, err := mutateChannelBreakerState(key, func(state *channelBreakerState, _ time.Time) channelBreakerMutation {
			if state == nil {
				return channelBreakerMutation{Action: channelBreakerMutationNone}
			}
			return channelBreakerMutation{Action: channelBreakerMutationDelete}
		}); err != nil {
			logChannelBreakerRedisFailure("clear state", key, err)
		}
	}
}

func channelBreakerStateMatchesTarget(stateKey string, channelError types.ChannelError) bool {
	baseKey := channelBreakerKey(channelError)
	if stateKey == baseKey || strings.HasPrefix(stateKey, baseKey+"|") {
		return true
	}
	if channelError.UsingKey != "" {
		return false
	}
	channelPrefix := strconv.Itoa(channelError.ChannelId)
	return strings.HasPrefix(stateKey, channelPrefix+":")
}

func ClearChannelBreakerByStateKey(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	mutation, err := mutateChannelBreakerState(key, func(state *channelBreakerState, _ time.Time) channelBreakerMutation {
		if state == nil {
			return channelBreakerMutation{Action: channelBreakerMutationNone, Value: false}
		}
		return channelBreakerMutation{Action: channelBreakerMutationDelete, Value: true}
	})
	if err != nil {
		logChannelBreakerRedisFailure("clear state", key, err)
		return false
	}
	cleared, _ := mutation.Value.(bool)
	return cleared
}

func ListChannelBreakerStatuses() []ChannelBreakerStatus {
	keys := listChannelBreakerKeys()
	displayNow, timeErr := channelBreakerCurrentTime()
	if timeErr != nil {
		common.SysError("failed to read Redis time for channel breaker statuses: " + timeErr.Error())
		displayNow = time.Now()
	}
	statuses := make([]ChannelBreakerStatus, 0, len(keys))
	for _, key := range keys {
		mutation, err := mutateChannelBreakerState(key, func(state *channelBreakerState, now time.Time) channelBreakerMutation {
			if state == nil {
				return channelBreakerMutation{Action: channelBreakerMutationNone}
			}
			rule := defaultChannelBreakerRuntimeRule()
			if !isStaleHalfOpen(state, rule, now) {
				return channelBreakerMutation{Action: channelBreakerMutationNone, Value: cloneChannelBreakerState(state)}
			}
			reason := fmt.Sprintf("channel breaker probe timed out (%d/%d successes, %d/%d completed)", state.ProbeSuccess, stateProbeSuccessCount(state, rule), state.ProbeTotal, stateProbeCount(state, rule))
			event := newChannelBreakerOpenEvent(key, state, reason)
			openBreakerAt(state, now)
			return channelBreakerMutation{Action: channelBreakerMutationSave, State: state, Value: cloneChannelBreakerState(state), Event: event}
		})
		if err != nil {
			logChannelBreakerRedisFailure("list state", key, err)
			continue
		}
		recordChannelBreakerMutationEvent(mutation.Event)
		state, _ := mutation.Value.(*channelBreakerState)
		if state == nil || state.State == ChannelBreakerStateClosed {
			continue
		}
		statuses = append(statuses, buildChannelBreakerStatus(key, state, displayNow))
	}
	return statuses
}

func GetChannelBreakerFailureThreshold() int {
	return common.GetChannelBreakerFailureLimit()
}

func applyChannelBreakerRuleContext(state *channelBreakerState, c *gin.Context, rule channelBreakerRuntimeRule) {
	state.RuleId = rule.Id
	state.RuleName = rule.Name
	state.Group = channelBreakerContextGroup(c)
	state.Model = channelBreakerContextModel(c)
	state.CooldownSecs = int(rule.Cooldown.Seconds())
	state.RuleProbeCount = rule.ProbeCount
	state.RuleProbeNeed = rule.ProbeSuccessCount
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

func evaluateProbeMutation(key string, state *channelBreakerState, rule channelBreakerRuntimeRule, now time.Time) channelBreakerMutation {
	probeSuccesses := stateProbeSuccessCount(state, rule)
	probeRequests := stateProbeCount(state, rule)
	if state.ProbeTotal < probeRequests {
		message := fmt.Sprintf("channel breaker probing (%d/%d successes, %d/%d completed)", state.ProbeSuccess, probeSuccesses, state.ProbeTotal, probeRequests)
		return channelBreakerMutation{Action: channelBreakerMutationSave, State: state, Value: channelBreakerRecordResult{Message: message}}
	}
	if state.ProbeSuccess >= probeSuccesses {
		return channelBreakerMutation{Action: channelBreakerMutationDelete, Value: channelBreakerRecordResult{Message: "channel breaker closed after probe"}}
	}
	message := fmt.Sprintf("channel breaker remains open after probe (%d/%d successes)", state.ProbeSuccess, state.ProbeTotal)
	event := newChannelBreakerOpenEvent(key, state, message)
	openBreakerAt(state, now)
	return channelBreakerMutation{Action: channelBreakerMutationSave, State: state, Value: channelBreakerRecordResult{Opened: true, Message: message}, Event: event}
}

func stateCooldown(state *channelBreakerState, rule channelBreakerRuntimeRule) time.Duration {
	if state != nil && state.CooldownSecs > 0 {
		return time.Duration(state.CooldownSecs) * time.Second
	}
	return rule.Cooldown
}

func stateProbeCount(state *channelBreakerState, rule channelBreakerRuntimeRule) int {
	if state != nil && state.RuleProbeCount > 0 {
		return state.RuleProbeCount
	}
	return rule.ProbeCount
}

func stateProbeSuccessCount(state *channelBreakerState, rule channelBreakerRuntimeRule) int {
	if state != nil && state.RuleProbeNeed > 0 {
		return state.RuleProbeNeed
	}
	return rule.ProbeSuccessCount
}

func isStaleHalfOpen(state *channelBreakerState, rule channelBreakerRuntimeRule, now time.Time) bool {
	if state == nil || state.State != ChannelBreakerStateHalfOpen || state.ProbeStartedAt.IsZero() {
		return false
	}
	timeout := stateCooldown(state, rule)
	if timeout < channelBreakerProbeTimeoutMin {
		timeout = channelBreakerProbeTimeoutMin
	}
	return now.Sub(state.ProbeStartedAt) >= timeout
}

// recordChannelBreakerOpenLog 在熔断器打开时异步记录一条历史日志。
// 必须在 openBreakerAt 之前捕获状态，以便保留被重置前的失败数据。
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

func newChannelBreakerOpenEvent(key string, state *channelBreakerState, reason string) *channelBreakerOpenEvent {
	return &channelBreakerOpenEvent{Key: key, State: *state, Reason: reason}
}

func recordChannelBreakerMutationEvent(event *channelBreakerOpenEvent) {
	if event == nil {
		return
	}
	recordChannelBreakerOpenLog(event.Key, &event.State, event.Reason)
}

func openBreakerAt(state *channelBreakerState, now time.Time) {
	state.State = ChannelBreakerStateOpen
	state.Generation++
	state.Failures = 0
	state.OpenedAt = now
	state.ProbeStartedAt = time.Time{}
	state.ProbeInFlight = 0
	state.ProbeTotal = 0
	state.ProbeSuccess = 0
}

func channelBreakerProbeFromContext(c *gin.Context, key string) (ChannelBreakerProbe, bool) {
	if c == nil {
		return ChannelBreakerProbe{}, false
	}
	value, ok := c.Get("channel_breaker_probe")
	if !ok {
		return ChannelBreakerProbe{}, false
	}
	probe, ok := value.(ChannelBreakerProbe)
	return probe, ok && probe.Key == key
}

func channelBreakerKey(channelError types.ChannelError) string {
	if channelError.UsingKey == "" {
		return fmt.Sprintf("%d", channelError.ChannelId)
	}
	return fmt.Sprintf("%d:%s", channelError.ChannelId, ChannelBreakerKeyHash(channelError.UsingKey))
}

func channelBreakerStateKey(c *gin.Context, channelError types.ChannelError, rule channelBreakerRuntimeRule) string {
	key := channelBreakerKey(channelError)
	var scopeType, scopeValue string
	switch rule.Scope {
	case common.ChannelBreakerScopeGroup:
		scopeType = "group"
		scopeValue = channelBreakerContextGroup(c)
	case common.ChannelBreakerScopeModel:
		scopeType = "model"
		scopeValue = channelBreakerContextModel(c)
	}
	if scopeValue == "" {
		return key
	}
	return fmt.Sprintf("%s|%s:%x", key, scopeType, hashString64(scopeValue))
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

func listChannelBreakerKeys() []string {
	if channelBreakerRedisEnabled() {
		keys := make([]string, 0)
		iter := common.RDB.Scan(context.Background(), 0, "channel_breaker:*", 100).Iterator()
		for iter.Next(context.Background()) {
			key := strings.TrimPrefix(iter.Val(), "channel_breaker:")
			if key != "" {
				keys = append(keys, key)
			}
		}
		if err := iter.Err(); err != nil {
			common.SysError("failed to scan channel breaker keys from redis: " + err.Error())
		}
		return keys
	}

	channelBreakerMu.Lock()
	defer channelBreakerMu.Unlock()
	keys := make([]string, 0, len(channelBreakerStates))
	for key := range channelBreakerStates {
		keys = append(keys, key)
	}
	return keys
}

func buildChannelBreakerStatus(key string, state *channelBreakerState, now time.Time) ChannelBreakerStatus {
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
		remaining := cooldown - now.Sub(state.OpenedAt)
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
	baseKey := strings.SplitN(key, "|", 2)[0]
	parts := strings.SplitN(baseKey, ":", 2)
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
	runtimeRule.Scope = rule.Scope
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
// 供未返回结构化额度错误码的上游与状态码 403 配合兜底，开箱即用。
var defaultInstantDisableKeywordList = []string{
	"insufficient account balance",
	"insufficient user quota",
	"insufficient_user_quota",
	"用户额度不足",
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
		Scope:                     common.ChannelBreakerScopeGlobal,
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
		OnlyKeyBreaker:            true,
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
	if rule.OnlyKeyBreaker {
		if channelError.UsingKey == "" {
			return channelError, false
		}
		return channelError, true
	}
	channelError.UsingKey = ""
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

func readChannelBreakerState(key string) (*channelBreakerState, error) {
	if channelBreakerRedisEnabled() {
		data, err := common.RDB.Get(context.Background(), channelBreakerRedisKey(key)).Result()
		if err == redis.Nil {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		var state channelBreakerState
		if err := common.UnmarshalJsonStr(data, &state); err != nil {
			return nil, err
		}
		return &state, nil
	}

	channelBreakerMu.Lock()
	defer channelBreakerMu.Unlock()
	state := channelBreakerStates[key]
	if state == nil {
		return nil, nil
	}
	copy := *state
	return &copy, nil
}

func mutateChannelBreakerState(key string, mutate func(*channelBreakerState, time.Time) channelBreakerMutation) (channelBreakerMutation, error) {
	if !channelBreakerRedisEnabled() {
		channelBreakerMu.Lock()
		defer channelBreakerMu.Unlock()
		state := cloneChannelBreakerState(channelBreakerStates[key])
		mutation := mutate(state, time.Now())
		applyLocalChannelBreakerMutation(key, mutation)
		return mutation, nil
	}

	redisKey := channelBreakerRedisKey(key)
	ctx := context.Background()
	var committed channelBreakerMutation
	for attempt := 0; attempt < channelBreakerRedisMaxRetries; attempt++ {
		err := common.RDB.Watch(ctx, func(tx *redis.Tx) error {
			now, err := tx.Time(ctx).Result()
			if err != nil {
				return err
			}
			state, err := readChannelBreakerStateFromTx(ctx, tx, redisKey)
			if err != nil {
				return err
			}
			mutation := mutate(state, now)
			if mutation.Action == channelBreakerMutationNone {
				committed = mutation
				return nil
			}
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				switch mutation.Action {
				case channelBreakerMutationSave:
					data, marshalErr := common.Marshal(mutation.State)
					if marshalErr != nil {
						return marshalErr
					}
					pipe.Set(ctx, redisKey, string(data), channelBreakerStateTTL(mutation.State))
				case channelBreakerMutationDelete:
					pipe.Del(ctx, redisKey)
				}
				return nil
			})
			if err == nil {
				committed = mutation
			}
			return err
		}, redisKey)
		if err == redis.TxFailedErr {
			time.Sleep(time.Duration(attempt+1) * 100 * time.Microsecond)
			continue
		}
		return committed, err
	}
	return channelBreakerMutation{}, fmt.Errorf("%w after %d retries", errChannelBreakerRedisConflict, channelBreakerRedisMaxRetries)
}

func channelBreakerCurrentTime() (time.Time, error) {
	if !channelBreakerRedisEnabled() {
		return time.Now(), nil
	}
	return common.RDB.Time(context.Background()).Result()
}

func readChannelBreakerStateFromTx(ctx context.Context, tx *redis.Tx, key string) (*channelBreakerState, error) {
	data, err := tx.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var state channelBreakerState
	if err := common.UnmarshalJsonStr(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func cloneChannelBreakerState(state *channelBreakerState) *channelBreakerState {
	if state == nil {
		return nil
	}
	copy := *state
	return &copy
}

func applyLocalChannelBreakerMutation(key string, mutation channelBreakerMutation) {
	switch mutation.Action {
	case channelBreakerMutationSave:
		channelBreakerStates[key] = mutation.State
	case channelBreakerMutationDelete:
		delete(channelBreakerStates, key)
	}
}

func channelBreakerStateTTL(state *channelBreakerState) time.Duration {
	cooldown := getChannelBreakerCooldown()
	if state != nil && state.CooldownSecs > 0 {
		cooldown = time.Duration(state.CooldownSecs) * time.Second
	}
	probeCount := common.GetChannelBreakerProbeCount()
	if state != nil && state.RuleProbeCount > 0 {
		probeCount = state.RuleProbeCount
	}
	return cooldown + time.Duration(probeCount+common.GetChannelBreakerFailureLimit()+60)*time.Second
}

func logChannelBreakerRedisFailure(operation, key string, err error) {
	if err == nil || !channelBreakerRedisEnabled() {
		return
	}
	common.SysError(fmt.Sprintf("channel breaker %s failed for %s: %s", operation, key, err.Error()))
}

// These helpers are kept for local-state tests that need to adjust timestamps.
func loadChannelBreakerStateLocked(key string) *channelBreakerState {
	return channelBreakerStates[key]
}

func saveChannelBreakerStateLocked(key string, state *channelBreakerState) {
	if state == nil {
		delete(channelBreakerStates, key)
		return
	}
	channelBreakerStates[key] = state
}
