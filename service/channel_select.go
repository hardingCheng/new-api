package service

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

func GetChannelConstraints(c *gin.Context) *dto.ChannelConstraints {
	if c == nil {
		return &dto.ChannelConstraints{}
	}
	if existing, ok := common.GetContextKeyType[*dto.ChannelConstraints](c, constant.ContextKeyChannelConstraints); ok && existing != nil {
		return existing
	}
	constraints := &dto.ChannelConstraints{}
	common.SetContextKey(c, constant.ContextKeyChannelConstraints, constraints)
	return constraints
}

func AppendTaskPluginIdentityFilter(c *gin.Context, pluginKey string) {
	if c == nil {
		return
	}
	channelTypes, pluginKeys := pinnedTaskPluginIdentities(c, pluginKey)
	GetChannelConstraints(c).AddFilter(dto.ChannelFilter{
		Kind:                   dto.FilterTaskPluginIdentity,
		TaskPluginKey:          pluginKey,
		TaskPluginChannelTypes: channelTypes,
		TaskPluginKeys:         pluginKeys,
	})
}

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
	// ExcludeChannelIds 是本次请求中已经失败过的渠道，重试选路时在同优先级层内把它们
	// 移出候选。与 capacity 排除集不同，它必须跨重试保持：一条渠道这次已经证明不行，
	// 本请求就不该再抽回它。调用方（controller）在排除后无候选时负责放开排除集兜底。
	ExcludeChannelIds map[int]bool
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
// 选路口径:同档优先(2026-08-21 起)。每次选择(含重试)都从最高优先级档
// 开始,已失败渠道(ExcludeChannelIds)、熔断中、容量满载的渠道从档内候选
// 剔除;某档被剔空才降到下一档。此前的行为是"第 N 次重试从第 N 档起步只往
// 下走",同档未被抽中的健康渠道从此没有机会,实测会让重试链跳过大档撞进
// 空池/兜底渠道(2026-08-20 事故:14→48→52→70,P5 档三条健康渠道未被尝试)。
//
// For "auto" tokenGroup with cross-group Retry enabled:
// 对于启用了跨分组重试的 "auto" tokenGroup:
//
//   - 每个分组用完 RetryTimes 次重试(或选不出渠道)后才切换到下一个分组;
//     组内每次重试同样按"同档优先"选路。
//
//   - Uses ContextKeyAutoGroupIndex to track current group index.
//     使用 ContextKeyAutoGroupIndex 跟踪当前分组索引。
//
//   - Uses ContextKeyAutoGroupRetryIndex to track the global Retry count when current group started.
//     使用 ContextKeyAutoGroupRetryIndex 跟踪当前分组开始时的全局重试次数。
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
			// priorityRetry 只用于"本组重试预算是否用完"的换组判定与日志,
			// 不再决定选路起始档位(选路一律同档优先,见 CacheGetRandomSatisfiedChannel 注释)
			priorityRetry := param.GetRetry()
			// If moved to a new group, reset priorityRetry and update startRetryIndex
			// 如果切换到新分组，重置 priorityRetry 并更新 startRetryIndex
			if i > startGroupIndex {
				priorityRetry = 0
			}
			logger.LogDebug(param.Ctx, "Auto selecting group: %s, priorityRetry: %d", autoGroup, priorityRetry)

			channel, err = getRandomSatisfiedChannelByBreaker(param, autoGroup)
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
		channel, err = getRandomSatisfiedChannelByBreaker(param, param.TokenGroup)
		if err != nil {
			return nil, param.TokenGroup, err
		}
	}
	if channel == nil && pendingCapacityErr != nil {
		return nil, selectGroup, pendingCapacityErr
	}
	return channel, selectGroup, nil
}

func getRandomSatisfiedChannelByBreaker(param *RetryParam, group string) (*model.Channel, error) {
	decision := resolveUserChannelRouting(param.Ctx, group, param.ModelName)
	channel, err := getRandomSatisfiedChannelByBreakerAndRouting(param, group, decision)
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

	channel, err = getRandomSatisfiedChannelByBreakerAndRouting(param, group, userChannelRoutingDecision{})
	setUserChannelRoutingLogInfo(param.Ctx, decision, group, param.ModelName, 0, true, "assigned_pool_unavailable")
	if channel != nil {
		setUserChannelRoutingLogInfo(param.Ctx, decision, group, param.ModelName, channel.Id, true, "assigned_pool_unavailable")
	}
	return channel, err
}

// maxTierWalk 是单次选路允许下探的优先级档数上限。同档优先后每次选路都从
// 第 0 档起步,靠排除集清空档位自然下探;默认路径(无用户路由规则)的 model
// 层不上报 exhausted 而是钳到最低档,需要这个硬上限终止循环。取值远大于任何
// 真实分组的档数,不构成实际限制。
const maxTierWalk = 32

// getRandomSatisfiedChannelByBreakerAndRouting 正常选路；若全部候选都被熔断
// 挡住（选不出任何渠道且没有其他错误），在开关打开时降级再选一次，这次忽略
// 熔断状态。
//
// 为什么要这条：熔断在选路层是硬过滤，某档被剔空就降档，所有档剔空就直接
// 返回「无可用渠道」，用户拿到硬报错。而熔断的本意是「有更好的选择时别用坏
// 的」——当一个可选的都没有时，坏的也比确定失败强：一条 50% 成功率的渠道
// 远好过 100% 报错。开了冷却退避后最长冷却可达 1 小时，没有这条兜底就是
// 最长 1 小时全站无路。
func getRandomSatisfiedChannelByBreakerAndRouting(param *RetryParam, group string, decision userChannelRoutingDecision) (*model.Channel, error) {
	channel, err := selectChannelOnce(param, group, decision, false)
	if channel != nil || err != nil {
		return channel, err
	}
	if !common.IsBreakerAllOpenFallbackEnabled() {
		return nil, nil
	}
	logger.LogWarn(param.Ctx, fmt.Sprintf("all candidates blocked by breaker in group %s for model %s, falling back to breaker-ignored selection", group, param.ModelName))
	return selectChannelOnce(param, group, decision, true)
}

// selectChannelOnce 是原来的选路主体。ignoreBreaker 为 true 时不按熔断状态
// 过滤候选，也不再消耗探测名额。
func selectChannelOnce(param *RetryParam, group string, decision userChannelRoutingDecision, ignoreBreaker bool) (*model.Channel, error) {
	enforceCapacity := param.ReserveCapacity &&
		operation_setting.GetChannelCapacityMode() == operation_setting.ChannelCapacityModeEnforce
	// 容量排除集只在本次选择调用内有效：重试之间租约会释放、窗口会滑动，不能跨调用记忆。
	var capacityExcluded map[int]bool
	var capacityRetryAfterMs int64
	capacityRedisError := false
	var lastErr error
outer:
	for offset := 0; decision.Matched || offset < maxTierWalk; offset++ {
		// 同档优先:每次选路(含重试)都从最高优先级档(offset=0)开始,
		// 已失败/熔断/满载的渠道被过滤器移出档内候选,某档剔空才降档。
		// 同层重选：满载渠道仅移出本层候选重新加权抽取，不消耗降层机会。
		// 每轮排除集严格增大，循环必然收敛。
		for {
			channel, exhausted, err := model.GetRandomSatisfiedChannelWithFilters(group, param.ModelName, offset, GetChannelConstraints(param.Ctx).Filters, userChannelRoutingCandidateFilter(decision), func(channel *model.Channel) bool {
				if param.ExcludeChannelIds[channel.Id] {
					logger.LogDebug(param.Ctx, "channel #%d already failed in this request, excluded from reselection", channel.Id)
					return false
				}
				if capacityExcluded[channel.Id] {
					return false
				}
				if ignoreBreaker {
					return true
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

			if !ignoreBreaker && !channel.ChannelInfo.IsMultiKey && !AcquireChannelBreakerProbe(param.Ctx, typesChannelError(channel)) {
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

func pinnedTaskPluginIdentities(c *gin.Context, expected string) ([]int, []string) {
	if c == nil || expected == "" {
		return nil, nil
	}
	if value, exists := c.Get(jsplugin.ContextKeyPinnedEndpoint); exists {
		pinned, ok := value.(jsplugin.PinnedEndpoint)
		if ok && pinned.Generation != nil && len(pinned.Candidates) > 1 {
			expectedFound := false
			channelTypes := make([]int, 0, len(pinned.Candidates))
			pluginKeys := make([]string, 0, len(pinned.Candidates))
			seen := make(map[int]struct{}, len(pinned.Candidates))
			for _, candidate := range pinned.Candidates {
				if candidate.Plugin == nil {
					continue
				}
				if candidate.Plugin.Meta.Key == expected {
					expectedFound = true
				}
				pluginKeys = append(pluginKeys, candidate.Plugin.Meta.Key)
				for _, channelType := range candidate.Plugin.Meta.ChannelTypes {
					if channelType == 0 || channelType == constant.ChannelTypeTaskPlugin {
						continue
					}
					if _, duplicate := seen[channelType]; duplicate {
						continue
					}
					if plugin, indexed := pinned.Generation.GetByChannelType(channelType); indexed && plugin == candidate.Plugin {
						seen[channelType] = struct{}{}
						channelTypes = append(channelTypes, channelType)
					}
				}
			}
			if expectedFound {
				return channelTypes, pluginKeys
			}
		}
	}
	value, exists := c.Get(jsplugin.ContextKeyPinnedPlugin)
	pinned, ok := value.(jsplugin.PinnedPlugin)
	if !exists || !ok || pinned.Generation == nil || pinned.Plugin == nil || pinned.Plugin.Meta.Key != expected {
		return nil, nil
	}
	channelTypes := make([]int, 0, len(pinned.Plugin.Meta.ChannelTypes))
	for _, channelType := range pinned.Plugin.Meta.ChannelTypes {
		if channelType == 0 || channelType == constant.ChannelTypeTaskPlugin {
			continue
		}
		channelTypes = append(channelTypes, channelType)
	}
	return channelTypes, []string{expected}
}

// ChannelSelectError explains why SelectChannelForRequest found no channel.
// Callers render it for their transport: the HTTP distributor localizes
// MessageID with its own helpers and the Responses WebSocket relay wraps it in
// a NewAPIError. Message is set instead of MessageID when the text is a fixed
// error code that clients match on.
type ChannelSelectError struct {
	StatusCode int
	Code       types.ErrorCode
	MessageID  string
	Params     map[string]any
	Message    string
	// FilterKind and Channel identify a candidate rejected by request filters.
	FilterKind dto.ChannelFilterKind
	Channel    *model.Channel
	// NoAvailableChannel marks the "no channel for this group and model"
	// outcome so the distributor can name the claiming task plugin.
	NoAvailableChannel bool
}

// SelectChannelForRequest resolves the channel for one attempt with the rules
// shared by the HTTP distributor and the Responses WebSocket relay: a pinned
// channel wins, then session affinity (first attempt only), then a random
// eligible channel; every candidate must satisfy the request's channel
// filters. The group the channel was chosen from is returned for auto-group
// callers. The caller still applies SetupContextForSelectedChannel.
func SelectChannelForRequest(c *gin.Context, modelName string, retry *RetryParam) (*model.Channel, string, *ChannelSelectError) {
	constraints := GetChannelConstraints(c)
	if pin, found, overridden := constraints.ResolvedPin(); found {
		for _, lost := range overridden {
			logger.LogWarn(c, fmt.Sprintf(
				"channel pin overridden: winning_source=%s winning_channel_id=%d overridden_source=%s overridden_channel_id=%d",
				pin.Source, pin.ChannelId, lost.Source, lost.ChannelId,
			))
		}
		channel, err := model.CacheGetChannel(pin.ChannelId)
		if err != nil {
			return nil, "", pinnedChannelUnavailable(pin, http.StatusBadRequest, i18n.MsgDistributorInvalidChannelId)
		}
		if channel.Status != common.ChannelStatusEnabled {
			return nil, "", pinnedChannelUnavailable(pin, http.StatusForbidden, i18n.MsgDistributorChannelDisabled)
		}
		if ok, kind := model.ChannelSatisfiesFilters(channel, modelName, constraints.Filters); !ok {
			return nil, "", &ChannelSelectError{
				StatusCode: http.StatusBadRequest, Code: types.ErrorCode(kind), MessageID: i18n.MsgDistributorNoAvailableChannel,
				Params:     map[string]any{"Group": common.GetContextKeyString(c, constant.ContextKeyUsingGroup), "Model": modelName},
				FilterKind: kind, Channel: channel,
			}
		}
		// 令牌指定渠道同样受用户路由池与熔断约束；续作任务钉回原渠道不受限。
		if pin.Source != dto.PinSourceOriginTask {
			pinGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
			if !AllowSelectedChannelByUserRouting(c, pinGroup, modelName, channel) || !AllowSelectedChannelByBreaker(c, channel) {
				return nil, "", &ChannelSelectError{
					StatusCode: http.StatusServiceUnavailable, Code: types.ErrorCodeModelNotFound, MessageID: i18n.MsgDistributorNoAvailableChannel,
					Params: map[string]any{"Group": pinGroup, "Model": modelName},
				}
			}
		}
		return channel, "", nil
	}

	usingGroup := retry.TokenGroup
	var channel *model.Channel
	var selectGroup string
	if retry.GetRetry() == 0 {
		if preferredChannelID, found := GetPreferredChannelByAffinity(c, modelName, usingGroup); found {
			affinityUsable := false
			preferred, err := model.CacheGetChannel(preferredChannelID)
			affinitySatisfied := false
			if err == nil && preferred != nil && preferred.Status == common.ChannelStatusEnabled {
				affinitySatisfied, _ = model.ChannelSatisfiesFilters(preferred, modelName, constraints.Filters)
			}
			if affinitySatisfied {
				if usingGroup == "auto" {
					userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
					for _, g := range GetRequestAutoGroups(c, userGroup) {
						if model.IsChannelEnabledForGroupModel(g, modelName, preferred.Id) &&
							AllowSelectedChannelByUserRouting(c, g, modelName, preferred) && AllowSelectedChannelByBreaker(c, preferred) {
							selectGroup = g
							common.SetContextKey(c, constant.ContextKeyAutoGroup, g)
							channel = preferred
							affinityUsable = true
							MarkChannelAffinityUsed(c, g, preferred.Id)
							break
						}
					}
				} else if model.IsChannelEnabledForGroupModel(usingGroup, modelName, preferred.Id) &&
					AllowSelectedChannelByUserRouting(c, usingGroup, modelName, preferred) && AllowSelectedChannelByBreaker(c, preferred) {
					channel = preferred
					selectGroup = usingGroup
					affinityUsable = true
					MarkChannelAffinityUsed(c, usingGroup, preferred.Id)
				}
			}
			if !affinityUsable && !ShouldKeepChannelAffinityOnChannelDisabled() {
				ClearCurrentChannelAffinityCache(c)
			}
			if !affinityUsable && RequestPolicy(c).SessionMode == "strict" {
				return nil, "", &ChannelSelectError{StatusCode: http.StatusServiceUnavailable, Message: "strict_session_binding_unavailable"}
			}
		}
	}

	if channel == nil {
		var err error
		channel, selectGroup, err = CacheGetRandomSatisfiedChannel(retry)
		if err != nil {
			showGroup := usingGroup
			if usingGroup == "auto" {
				showGroup = fmt.Sprintf("auto(%s)", selectGroup)
			}
			return nil, selectGroup, &ChannelSelectError{
				StatusCode: http.StatusServiceUnavailable, Code: types.ErrorCodeModelNotFound, MessageID: i18n.MsgDistributorGetChannelFailed,
				Params: map[string]any{"Group": showGroup, "Model": modelName, "Error": err.Error()},
			}
		}
		if channel == nil {
			return nil, selectGroup, &ChannelSelectError{
				StatusCode: http.StatusServiceUnavailable, Code: types.ErrorCodeModelNotFound, MessageID: i18n.MsgDistributorNoAvailableChannel,
				Params: map[string]any{"Group": usingGroup, "Model": modelName}, NoAvailableChannel: true,
			}
		}
	}
	if ok, kind := model.ChannelSatisfiesFilters(channel, modelName, constraints.Filters); !ok {
		return nil, selectGroup, &ChannelSelectError{
			StatusCode: http.StatusServiceUnavailable, Code: types.ErrorCodeModelNotFound, MessageID: i18n.MsgDistributorNoAvailableChannel,
			Params:     map[string]any{"Group": common.GetContextKeyString(c, constant.ContextKeyUsingGroup), "Model": modelName},
			FilterKind: kind, Channel: channel, NoAvailableChannel: true,
		}
	}
	return channel, selectGroup, nil
}

// Origin-task pins report a fixed code so task polling can tell a retired
// channel from a malformed request.
func pinnedChannelUnavailable(pin dto.ChannelPin, statusCode int, messageID string) *ChannelSelectError {
	if pin.Source == dto.PinSourceOriginTask {
		return &ChannelSelectError{StatusCode: http.StatusBadRequest, Code: "origin_task_channel_disabled", Message: "origin_task_channel_disabled"}
	}
	return &ChannelSelectError{StatusCode: statusCode, MessageID: messageID}
}

// AppendUsedChannel records an attempted channel in the request's channel
// trail, which the retry log and the consume log's admin_info both read.
func AppendUsedChannel(c *gin.Context, channelID int) {
	c.Set("use_channel", append(c.GetStringSlice("use_channel"), fmt.Sprintf("%d", channelID)))
}
