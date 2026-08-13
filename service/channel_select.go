package service

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

type RetryParam struct {
	Ctx         *gin.Context
	TokenGroup  string
	ModelName   string
	RequestPath string
	Retry       *int
	// ReserveCapacity 只由 relay 的真实 attempt 选路置 true：enforce 模式下选中
	// 渠道时原子预留容量，满载渠道在本次选择内被排除并同层重选。distributor
	// 初选与其他调用方保持 false，不产生任何容量副作用。
	ReserveCapacity bool
	resetNextTry    bool
}

// ChannelCapacityExhaustedError 表示本次选择的全部候选渠道都因容量达限被排除。
// 它不代表渠道故障；调用方应转换为本地 429 + Retry-After，而不是当作上游错误。
// RedisError 为 true 表示存在因容量后端不可用而被 fail closed 排除的候选，
// 按 spec §7.4 应对外返回 503 而非 429。
type ChannelCapacityExhaustedError struct {
	RetryAfterMs int64
	RedisError   bool
}

func (e *ChannelCapacityExhaustedError) Error() string {
	return fmt.Sprintf("all eligible upstream channels are at capacity (retry after %dms)", e.RetryAfterMs)
}

// TakeReservedChannelCapacityLease 取走选路层预留的容量租约（取走即删，所有权转移）。
func TakeReservedChannelCapacityLease(c *gin.Context) ChannelCapacityLease {
	value, exists := common.GetContextKey(c, constant.ContextKeyChannelCapacityLease)
	if !exists {
		return nil
	}
	common.SetContextKey(c, constant.ContextKeyChannelCapacityLease, nil)
	lease, _ := value.(ChannelCapacityLease)
	return lease
}

func (p *RetryParam) GetRetry() int {
	if p.Retry == nil {
		return 0
	}
	return *p.Retry
}

func (p *RetryParam) SetRetry(retry int) {
	p.Retry = &retry
}

func (p *RetryParam) IncreaseRetry() {
	if p.resetNextTry {
		p.resetNextTry = false
		return
	}
	if p.Retry == nil {
		p.Retry = new(int)
	}
	*p.Retry++
}

func (p *RetryParam) ResetRetryNextTry() {
	p.resetNextTry = true
}

// CacheGetRandomSatisfiedChannel tries to get a random channel that satisfies the requirements.
// 尝试获取一个满足要求的随机渠道。
//
// For "auto" tokenGroup with cross-group Retry enabled:
// 对于启用了跨分组重试的 "auto" tokenGroup：
//
//   - Each group will exhaust all its priorities before moving to the next group.
//     每个分组会用完所有优先级后才会切换到下一个分组。
//
//   - Uses ContextKeyAutoGroupIndex to track current group index.
//     使用 ContextKeyAutoGroupIndex 跟踪当前分组索引。
//
//   - Uses ContextKeyAutoGroupRetryIndex to track the global Retry count when current group started.
//     使用 ContextKeyAutoGroupRetryIndex 跟踪当前分组开始时的全局重试次数。
//
//   - priorityRetry = Retry - startRetryIndex, represents the priority level within current group.
//     priorityRetry = Retry - startRetryIndex，表示当前分组内的优先级级别。
//
//   - When GetRandomSatisfiedChannel returns nil (priorities exhausted), moves to next group.
//     当 GetRandomSatisfiedChannel 返回 nil（优先级用完）时，切换到下一个分组。
//
// Example flow (2 groups, each with 2 priorities, RetryTimes=3):
// 示例流程（2个分组，每个有2个优先级，RetryTimes=3）：
//
//	Retry=0: GroupA, priority0 (startRetryIndex=0, priorityRetry=0)
//	         分组A, 优先级0
//
//	Retry=1: GroupA, priority1 (startRetryIndex=0, priorityRetry=1)
//	         分组A, 优先级1
//
//	Retry=2: GroupA exhausted → GroupB, priority0 (startRetryIndex=2, priorityRetry=0)
//	         分组A用完 → 分组B, 优先级0
//
//	Retry=3: GroupB, priority1 (startRetryIndex=2, priorityRetry=1)
//	         分组B, 优先级1
func CacheGetRandomSatisfiedChannel(param *RetryParam) (*model.Channel, string, error) {
	var channel *model.Channel
	var err error
	selectGroup := param.TokenGroup
	userGroup := common.GetContextKeyString(param.Ctx, constant.ContextKeyUserGroup)
	// auto 跨分组时，某组全因容量满不该终止扫组；记住容量错误，全部组都无渠道时再上抛
	var pendingCapacityErr *ChannelCapacityExhaustedError

	if param.TokenGroup == "auto" {
		autoGroups := GetRequestAutoGroups(param.Ctx, userGroup)
		if len(autoGroups) == 0 {
			return nil, selectGroup, errors.New("auto groups is not enabled")
		}

		// startGroupIndex: the group index to start searching from
		// startGroupIndex: 开始搜索的分组索引
		startGroupIndex := 0
		crossGroupRetry := common.GetContextKeyBool(param.Ctx, constant.ContextKeyTokenCrossGroupRetry)

		if lastGroupIndex, exists := common.GetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex); exists {
			if idx, ok := lastGroupIndex.(int); ok {
				startGroupIndex = idx
			}
		}

		for i := startGroupIndex; i < len(autoGroups); i++ {
			autoGroup := autoGroups[i]
			// Calculate priorityRetry for current group
			// 计算当前分组的 priorityRetry
			priorityRetry := param.GetRetry()
			// If moved to a new group, reset priorityRetry and update startRetryIndex
			// 如果切换到新分组，重置 priorityRetry 并更新 startRetryIndex
			if i > startGroupIndex {
				priorityRetry = 0
			}
			logger.LogDebug(param.Ctx, "Auto selecting group: %s, priorityRetry: %d", autoGroup, priorityRetry)

			channel, err = getRandomSatisfiedChannelByBreaker(param, autoGroup, priorityRetry)
			if err != nil {
				var capacityErr *ChannelCapacityExhaustedError
				if !errors.As(err, &capacityErr) {
					return nil, autoGroup, err
				}
				// 本组全因容量满：视同无渠道，继续尝试下一组；取各组最小 Retry-After
				if pendingCapacityErr == nil {
					pendingCapacityErr = capacityErr
				} else {
					if capacityErr.RetryAfterMs > 0 &&
						(pendingCapacityErr.RetryAfterMs <= 0 || capacityErr.RetryAfterMs < pendingCapacityErr.RetryAfterMs) {
						pendingCapacityErr.RetryAfterMs = capacityErr.RetryAfterMs
					}
					pendingCapacityErr.RedisError = pendingCapacityErr.RedisError || capacityErr.RedisError
				}
				err = nil
				channel = nil
			}
			if channel == nil {
				// Current group has no available channel for this model, try next group
				// 当前分组没有该模型的可用渠道，尝试下一个分组
				logger.LogDebug(param.Ctx, "No available channel in group %s for model %s at priorityRetry %d, trying next group", autoGroup, param.ModelName, priorityRetry)
				// 重置状态以尝试下一个分组
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupRetryIndex, 0)
				// Reset retry counter so outer loop can continue for next group
				// 重置重试计数器，以便外层循环可以为下一个分组继续
				param.SetRetry(0)
				continue
			}
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, autoGroup)
			selectGroup = autoGroup
			logger.LogDebug(param.Ctx, "Auto selected group: %s", autoGroup)

			// Prepare state for next retry
			// 为下一次重试准备状态
			if crossGroupRetry && priorityRetry >= common.RetryTimes {
				// Current group has exhausted all retries, prepare to switch to next group
				// This request still uses current group, but next retry will use next group
				// 当前分组已用完所有重试次数，准备切换到下一个分组
				// 本次请求仍使用当前分组，但下次重试将使用下一个分组
				logger.LogDebug(param.Ctx, "Current group %s retries exhausted (priorityRetry=%d >= RetryTimes=%d), preparing switch to next group for next retry", autoGroup, priorityRetry, common.RetryTimes)
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
				// Reset retry counter so outer loop can continue for next group
				// 重置重试计数器，以便外层循环可以为下一个分组继续
				param.SetRetry(0)
				param.ResetRetryNextTry()
			} else {
				// Stay in current group, save current state
				// 保持在当前分组，保存当前状态
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i)
			}
			break
		}
	} else {
		channel, err = getRandomSatisfiedChannelByBreaker(param, param.TokenGroup, param.GetRetry())
		if err != nil {
			return nil, param.TokenGroup, err
		}
	}
	if channel == nil && pendingCapacityErr != nil {
		return nil, selectGroup, pendingCapacityErr
	}
	return channel, selectGroup, nil
}

func getRandomSatisfiedChannelByBreaker(param *RetryParam, group string, priorityRetry int) (*model.Channel, error) {
	decision := resolveUserChannelRouting(param.Ctx, group, param.ModelName)
	channel, err := getRandomSatisfiedChannelByBreakerAndRouting(param, group, priorityRetry, decision)
	var assignedCapacityErr *ChannelCapacityExhaustedError
	if err != nil && !errors.As(err, &assignedCapacityErr) {
		return channel, err
	}
	if channel != nil || !decision.Matched || decision.Match.Rule.Fallback != model_setting.UserChannelRoutingFallbackDefault {
		// 指定池全因容量满且不允许默认回退：本地回压，不突破上限
		if channel == nil && assignedCapacityErr != nil {
			return nil, assignedCapacityErr
		}
		return channel, err
	}

	channel, err = getRandomSatisfiedChannelByBreakerAndRouting(param, group, priorityRetry, userChannelRoutingDecision{})
	setUserChannelRoutingLogInfo(param.Ctx, decision, group, param.ModelName, 0, true, "assigned_pool_unavailable")
	if channel != nil {
		setUserChannelRoutingLogInfo(param.Ctx, decision, group, param.ModelName, channel.Id, true, "assigned_pool_unavailable")
	}
	return channel, err
}

func getRandomSatisfiedChannelByBreakerAndRouting(param *RetryParam, group string, priorityRetry int, decision userChannelRoutingDecision) (*model.Channel, error) {
	maxAttempts := common.RetryTimes + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	enforceCapacity := param.ReserveCapacity &&
		operation_setting.GetChannelCapacityMode() == operation_setting.ChannelCapacityModeEnforce
	// 容量排除集只在本次选择调用内有效：重试之间租约会释放、窗口会滑动，不能跨调用记忆。
	var capacityExcluded map[int]bool
	var capacityRetryAfterMs int64
	capacityRedisError := false
	var lastErr error
outer:
	for offset := 0; decision.Matched || offset < maxAttempts; offset++ {
		// 同层重选：满载渠道仅移出本层候选重新加权抽取，不消耗降层机会。
		// 每轮排除集严格增大，循环必然收敛。
		for {
			channel, exhausted, err := model.GetRandomSatisfiedChannelWithFilters(group, param.ModelName, priorityRetry+offset, param.RequestPath, userChannelRoutingCandidateFilter(decision), func(channel *model.Channel) bool {
				if capacityExcluded[channel.Id] {
					return false
				}
				allowed := CanUseChannelByBreaker(param.Ctx, typesChannelError(channel))
				if !allowed {
					logger.LogWarn(param.Ctx, fmt.Sprintf("channel breaker skipped channel #%d", channel.Id))
				}
				return allowed
			})
			if exhausted {
				break outer
			}
			if err != nil {
				lastErr = err
				if decision.Matched {
					break outer
				}
				continue outer
			}
			if channel == nil {
				continue outer
			}

			// 容量准入先于有副作用的 probe 获取（spec §6.2）；满载只写排除集，无副作用
			var capacityLease ChannelCapacityLease
			if enforceCapacity {
				capacity := channel.GetSetting().Capacity
				if capacity.HasLimit() {
					attemptID := BuildChannelCapacityAttemptID(param.Ctx.GetString(common.RequestIdKey), param.GetRetry())
					capacityDecision := TryAcquireChannelCapacity(param.Ctx, channel.Id, capacity, operation_setting.ChannelCapacityModeEnforce, attemptID)
					if !capacityDecision.Allowed {
						if capacityExcluded == nil {
							capacityExcluded = make(map[int]bool)
						}
						capacityExcluded[channel.Id] = true
						if capacityDecision.Reason == ChannelCapacityReasonRedisError {
							capacityRedisError = true
						} else if capacityDecision.RetryAfterMs > 0 &&
							(capacityRetryAfterMs == 0 || capacityDecision.RetryAfterMs < capacityRetryAfterMs) {
							capacityRetryAfterMs = capacityDecision.RetryAfterMs
						}
						// 单个候选满载不打 warn，避免饱和时日志风暴；精确计数在容量 stats 里
						logger.LogDebug(param.Ctx, "channel capacity skipped channel #%d (%s)", channel.Id, capacityDecision.Reason)
						continue
					}
					capacityLease = capacityDecision.Lease
				}
			}

			if !channel.ChannelInfo.IsMultiKey && !AcquireChannelBreakerProbe(param.Ctx, typesChannelError(channel)) {
				logger.LogWarn(param.Ctx, fmt.Sprintf("channel breaker probe limit reached for channel #%d", channel.Id))
				// probe 不放行时撤销已预留的容量（尚未派发，RPM 与并发都回退），沿用现状降层
				if capacityLease != nil {
					_ = capacityLease.CancelBeforeDispatch(param.Ctx)
				}
				continue outer
			}
			if decision.Matched {
				setUserChannelRoutingLogInfo(param.Ctx, decision, group, param.ModelName, channel.Id, false, "")
			}
			if capacityLease != nil {
				common.SetContextKey(param.Ctx, constant.ContextKeyChannelCapacityLease, capacityLease)
			}
			return channel, nil
		}
	}
	// 容量错误优先于普通"无可用渠道"：候选被容量排除后 model 层会报 no channel
	// found，那只是排除的结果；对客户可行动的信息是 Retry-After。
	if len(capacityExcluded) > 0 {
		retryAfterMs := capacityRetryAfterMs
		if retryAfterMs <= 0 {
			retryAfterMs = 1000
		}
		return nil, &ChannelCapacityExhaustedError{RetryAfterMs: retryAfterMs, RedisError: capacityRedisError}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, nil
}

func AllowSelectedChannelByBreaker(c *gin.Context, channel *model.Channel) bool {
	channelError := typesChannelError(channel)
	if !CanUseChannelByBreaker(c, channelError) {
		return false
	}
	if channel.ChannelInfo.IsMultiKey {
		return true
	}
	return AcquireChannelBreakerProbe(c, channelError)
}

func CanUseSelectedChannelByBreaker(c *gin.Context, channel *model.Channel) bool {
	if channel == nil {
		return false
	}
	allowed := CanUseChannelByBreaker(c, typesChannelError(channel))
	if !allowed {
		logger.LogWarn(c, fmt.Sprintf("channel breaker skipped channel #%d", channel.Id))
	}
	return allowed
}

func AllowSelectedChannelKeyByBreaker(c *gin.Context, channel *model.Channel, key string) bool {
	channelError := typesChannelError(channel)
	channelError.UsingKey = key
	if !CanUseChannelByBreaker(c, channelError) {
		return false
	}
	return AcquireChannelBreakerProbe(c, channelError)
}

func CanUseSelectedChannelKeyByBreaker(c *gin.Context, channel *model.Channel, key string) bool {
	channelError := typesChannelError(channel)
	channelError.UsingKey = key
	allowed := CanUseChannelByBreaker(c, channelError)
	if !allowed {
		logger.LogWarn(c, fmt.Sprintf("channel breaker skipped channel #%d key", channel.Id))
	}
	return allowed
}

func typesChannelError(channel *model.Channel) types.ChannelError {
	usingKey := ""
	if !channel.ChannelInfo.IsMultiKey {
		usingKey = channel.Key
	}
	return types.ChannelError{
		ChannelId:   channel.Id,
		ChannelType: channel.Type,
		ChannelName: channel.Name,
		IsMultiKey:  channel.ChannelInfo.IsMultiKey,
		UsingKey:    usingKey,
		AutoBan:     channel.GetAutoBan(),
		SkipBreaker: common.IsChannelBreakerExemptChannel(channel.Id),
	}
}
