package controller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func relayHandler(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	var err *types.NewAPIError
	switch info.RelayMode {
	case relayconstant.RelayModeImagesGenerations, relayconstant.RelayModeImagesEdits:
		err = relay.ImageHelper(c, info)
	case relayconstant.RelayModeAudioSpeech:
		fallthrough
	case relayconstant.RelayModeAudioTranslation:
		fallthrough
	case relayconstant.RelayModeAudioTranscription:
		err = relay.AudioHelper(c, info)
	case relayconstant.RelayModeRerank:
		err = relay.RerankHelper(c, info)
	case relayconstant.RelayModeEmbeddings:
		err = relay.EmbeddingHelper(c, info)
	case relayconstant.RelayModeResponses, relayconstant.RelayModeResponsesCompact:
		err = relay.ResponsesHelper(c, info)
	case relayconstant.RelayModeAlphaSearch:
		err = relay.AlphaSearchHelper(c, info)
	default:
		err = relay.TextHelper(c, info)
	}
	return err
}

func geminiRelayHandler(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	var err *types.NewAPIError
	if strings.Contains(c.Request.URL.Path, "embed") {
		err = relay.GeminiEmbeddingHandler(c, info)
	} else {
		err = relay.GeminiHelper(c, info)
	}
	return err
}

func Relay(c *gin.Context, relayFormat types.RelayFormat) {

	requestId := c.GetString(common.RequestIdKey)
	//group := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	//originalModel := common.GetContextKeyString(c, constant.ContextKeyOriginalModel)

	var (
		newAPIError  *types.NewAPIError
		ws           *websocket.Conn
		imageRequest *dto.ImageRequest
	)

	imageRelayMode := relayconstant.Path2RelayMode(c.Request.URL.Path)
	imagePayloadLogEnabled := relayFormat == types.RelayFormatOpenAIImage &&
		(imageRelayMode == relayconstant.RelayModeImagesGenerations || imageRelayMode == relayconstant.RelayModeImagesEdits)
	if imagePayloadLogEnabled {
		finishPayloadLog := relay.StartImageFailurePayloadLog(c)
		defer func() {
			finishPayloadLog(imageRequest)
		}()
	}

	if relayFormat == types.RelayFormatOpenAIRealtime {
		var err error
		ws, err = upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			helper.WssError(c, ws, types.NewError(err, types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry()).ToOpenAIError())
			return
		}
		defer ws.Close()
	}

	defer func() {
		if newAPIError != nil {
			service.RecordRequestPolicyTermination(c, newAPIError)
			logger.LogError(c, fmt.Sprintf("relay error: %s", common.LocalLogPreview(newAPIError.Error())))
			newAPIError = types.MaskUpstreamBillingError(newAPIError)
			newAPIError.SetMessage(common.MessageWithRequestId(newAPIError.Error(), requestId))
			switch relayFormat {
			case types.RelayFormatOpenAIRealtime:
				helper.WssError(c, ws, newAPIError.ToOpenAIError())
			case types.RelayFormatClaude:
				c.JSON(newAPIError.StatusCode, gin.H{
					"type":  "error",
					"error": newAPIError.ToClaudeError(),
				})
			default:
				c.JSON(newAPIError.StatusCode, gin.H{
					"error": newAPIError.ToOpenAIError(),
				})
			}
		}
	}()

	request, err := helper.GetAndValidateRequest(c, relayFormat)
	if imagePayloadLogEnabled {
		imageRequest, _ = request.(*dto.ImageRequest)
	}
	if err != nil {
		// Map "request body too large" to 413 so clients can handle it correctly
		if common.IsRequestBodyTooLargeError(err) || errors.Is(err, common.ErrRequestBodyTooLarge) {
			newAPIError = types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusRequestEntityTooLarge, types.ErrOptionWithSkipRetry())
		} else {
			// 请求体解析/校验失败是客户端错误,必须回 4xx:回 5xx 会让客户端 SDK 对
			// 永久性参数错误自动重试(NewError 不带状态码时默认 500)。
			newAPIError = types.NewError(err, types.ErrorCodeInvalidRequest, types.ErrOptionWithStatusCode(http.StatusBadRequest), types.ErrOptionWithSkipRetry())
		}
		return
	}

	relayInfo, err := relaycommon.GenRelayInfo(c, relayFormat, request, ws)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeGenRelayInfoFailed)
		return
	}
	if message, blocked := operation_setting.IsSeedanceResourcePoolGuardBlocked(relayInfo.UserId, relayInfo.EffectiveRoutingModelName()); blocked {
		recordSeedanceResourcePoolGuardErrorLog(c, relayInfo, message, "get_channel_failed", http.StatusTooManyRequests)
		newAPIError = types.NewErrorWithStatusCode(
			errors.New(message),
			types.ErrorCodeGetChannelFailed,
			http.StatusTooManyRequests,
			types.ErrOptionWithSkipRetry(),
		)
		return
	}

	defer func() {
		recovered := recover()
		resultErr := newAPIError
		if recovered != nil {
			resultErr = types.NewError(fmt.Errorf("relay panic: %v", recovered), types.ErrorCodeBadResponse)
		}
		if relayFormat != types.RelayFormatOpenAIRealtime {
			perfmetrics.RecordRelayResult(c.Request.Context(), relayInfo, resultErr)
		}
		if recovered != nil {
			panic(recovered)
		}
	}()

	if newAPIError = relay.PrepareRequestBilling(c, relayInfo); newAPIError != nil {
		return
	}
	defer func() {
		newAPIError = relay.RefundFailedRequestBilling(c, relayInfo, newAPIError)
	}()
	defer func() {
		// Safety net: a reservation that exits without being settled (e.g. panic or an
		// unexpected early return) would otherwise stay stuck in the pool until the period
		// resets. Roll it back so the cap reflects reality. Idempotent: no-op once settled.
		if !relayInfo.ModelQuotaPoolSettled && len(relayInfo.ModelQuotaPools) > 0 {
			logger.LogError(c, fmt.Sprintf("model quota pool reservation not settled, rolling back as safety net (model: %s, user: %d)", relayInfo.OriginModelName, relayInfo.UserId))
			service.RollbackModelQuotaPool(relayInfo)
		}
	}()

	retryParam := &service.RetryParam{
		Ctx:               c,
		TokenGroup:        relayInfo.TokenGroup,
		ModelName:         relayInfo.EffectiveRoutingModelName(),
		RequestPath:       c.Request.URL.Path,
		Retry:             common.GetPointer(0),
		ExcludeChannelIds: map[int]bool{},
	}
	relayInfo.RetryIndex = 0
	relayInfo.LastError = nil

	for ; retryParam.GetRetry() <= common.RetryTimes; retryParam.IncreaseRetry() {
		relayInfo.StreamStatus = nil
		relayInfo.PerformanceBusinessRejection = false
		relayInfo.PerformanceOutputTokens = 0
		relayInfo.RetryIndex = retryParam.GetRetry()
		channel, channelErr := getChannel(c, relayInfo, retryParam)
		if channelErr != nil {
			logger.LogError(c, channelErr.Error())
			newAPIError = channelErr
			break
		}
		// enforce 模式下选路层已预留容量；此处接管租约，派发前的任何本地失败都要撤销
		capacityLease := service.TakeReservedChannelCapacityLease(c)
		service.AppendUsedChannel(c, channel.Id)
		if billingErr := service.PrepareTieredBillingForSelectedGroup(c, relayInfo); billingErr != nil {
			if capacityLease != nil {
				_ = capacityLease.CancelBeforeDispatch(c)
			}
			newAPIError = billingErr
			break
		}

		bodyStorage, bodyErr := common.GetBodyStorage(c)
		if bodyErr != nil {
			if capacityLease != nil {
				_ = capacityLease.CancelBeforeDispatch(c)
			}
			// Ensure consistent 413 for oversized bodies even when error occurs later (e.g., retry path)
			if common.IsRequestBodyTooLargeError(bodyErr) || errors.Is(bodyErr, common.ErrRequestBodyTooLarge) {
				newAPIError = types.NewErrorWithStatusCode(bodyErr, types.ErrorCodeReadRequestBodyFailed, http.StatusRequestEntityTooLarge, types.ErrOptionWithSkipRetry())
			} else {
				newAPIError = types.NewErrorWithStatusCode(bodyErr, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
			}
			break
		}
		c.Request.Body = io.NopCloser(bodyStorage)

		if capacityLease == nil {
			// shadow 模式的观察采集（enforce 下由选路层负责，off 下返回 nil）
			capacityLease = acquireChannelCapacityForAttempt(c, relayFormat, channel, retryParam.GetRetry())
		}
		newAPIError = runRelayAttempt(c, relayInfo, relayFormat, capacityLease)

		if newAPIError == nil {
			service.MarkRequestPolicySuccess(c, relayInfo.StreamStatus)
			relayInfo.LastError = nil
			service.RecordChannelBreakerSuccess(c, *types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, common.GetContextKeyString(c, constant.ContextKeyChannelKey), channel.GetAutoBan(), common.IsChannelBreakerExemptChannel(channel.Id)))
			markRecoveredErrorLogs(c)
			return
		}

		newAPIError = service.NormalizeViolationFeeError(newAPIError)
		relayInfo.LastError = newAPIError
		// 这一条本次已经失败，重试选路时同层排除它，避免加权随机又抽回来
		retryParam.ExcludeChannelIds[channel.Id] = true
		service.ExcludeUpstreamAfterSessionBlock(c, channel.Id, newAPIError, retryParam)

		decision := service.DecideRelayRetry(c, newAPIError, common.RetryTimes-retryParam.GetRetry())
		service.RecordPolicyFailure(c, channel.Id, newAPIError, decision)
		processChannelError(c, *types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, common.GetContextKeyString(c, constant.ContextKeyChannelKey), channel.GetAutoBan(), common.IsChannelBreakerExemptChannel(channel.Id)), newAPIError, relayInfo)

		if decision.Action != "retry" {
			break
		}
	}

	useChannel := c.GetStringSlice("use_channel")
	if len(useChannel) > 1 {
		retryLogStr := fmt.Sprintf("重试：%s", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(useChannel)), "->"), "[]"))
		logger.LogInfo(c, retryLogStr)
	}
}

// CountClaudeTokens implements Anthropic's token-counting utility endpoint.
// It deliberately skips upstream generation and billing; callers use this
// endpoint to size prompts before creating a Message.
func CountClaudeTokens(c *gin.Context) {
	request, err := helper.GetAndValidateClaudeRequest(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"type": "error",
			"error": gin.H{
				"type":    "invalid_request_error",
				"message": common.MessageWithRequestId(err.Error(), c.GetString(common.RequestIdKey)),
			},
		})
		return
	}

	info := relaycommon.GenRelayInfoClaude(c, request)
	inputTokens, err := service.CountRequestToken(c, request.GetTokenCountMeta(), info)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"type": "error",
			"error": gin.H{
				"type":    "api_error",
				"message": common.MessageWithRequestId(err.Error(), c.GetString(common.RequestIdKey)),
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"input_tokens": inputTokens})
}

var upgrader = websocket.Upgrader{
	Subprotocols: []string{"realtime", "responses"}, // WS 握手支持的协议，如果有使用 Sec-WebSocket-Protocol，则必须在此声明对应的 Protocol
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许跨域
	},
}

// runRelayAttempt 是一次真实上游尝试的独立作用域：handler 返回（流式响应此时已
// 完全结束）或 panic 展开时，都先释放本次尝试的并发名额，再回到外层做错误处理
// 和下一次选路。释放绝不能挂在整个 Relay() 上，否则重试期间旧渠道的名额会一直
// 占到整条客户请求结束。
func runRelayAttempt(c *gin.Context, relayInfo *relaycommon.RelayInfo, relayFormat types.RelayFormat, capacityLease service.ChannelCapacityLease) *types.NewAPIError {
	if capacityLease != nil {
		// 跨过派发边界：此后 Release 只释放并发名额，RPM 记录保留到窗口自然过期
		capacityLease.MarkDispatched()
		defer func() { _ = capacityLease.Release(c.Request.Context()) }()
	}
	switch relayFormat {
	case types.RelayFormatOpenAIRealtime:
		return relay.WssHelper(c, relayInfo)
	case types.RelayFormatClaude:
		return relay.ClaudeHelper(c, relayInfo)
	case types.RelayFormatGemini:
		return geminiRelayHandler(c, relayInfo)
	default:
		return relayHandler(c, relayInfo)
	}
}

// acquireChannelCapacityForAttempt 是 shadow 模式的观察采集：在派发边界按 shadow
// 记录真实计数并上报 would-block，绝不拒绝流量。enforce 的预留在选路层完成（本函数
// 返回 nil），off 模式零开销。Realtime WebSocket 与任务类入口生命周期不同，
// 留待 Stage C 单独接入。
func acquireChannelCapacityForAttempt(c *gin.Context, relayFormat types.RelayFormat, channel *model.Channel, attemptIndex int) service.ChannelCapacityLease {
	if relayFormat == types.RelayFormatOpenAIRealtime {
		return nil
	}
	if operation_setting.GetChannelCapacityMode() != operation_setting.ChannelCapacityModeShadow {
		return nil
	}
	capacity := channel.GetSetting().Capacity
	if !capacity.HasLimit() {
		return nil
	}
	attemptID := service.BuildChannelCapacityAttemptID(c.GetString(common.RequestIdKey), attemptIndex)
	decision := service.TryAcquireChannelCapacity(c.Request.Context(), channel.Id, capacity, operation_setting.ChannelCapacityModeShadow, attemptID)
	if decision.WouldBlock && service.ChannelCapacityShouldLog(channel.Id) {
		logger.LogWarn(c, fmt.Sprintf("event=channel_capacity_shadow_block channel_id=%d reason=%s rpm_used=%d inflight_used=%d retry_after_ms=%d",
			channel.Id, decision.Reason, decision.RPMUsed, decision.InflightUsed, decision.RetryAfterMs))
	}
	return decision.Lease
}

// markRecoveredErrorLogs 在重试后最终成功时,把本请求此前失败尝试落下的错误
// 日志标记为已挽救。错误日志在 processChannelError 里同步落库,此时行已提交。
func markRecoveredErrorLogs(c *gin.Context) {
	if !constant.ErrorLogEnabled || len(c.GetStringSlice("use_channel")) <= 1 {
		return
	}
	requestId := c.GetString(common.RequestIdKey)
	gopool.Go(func() {
		model.MarkErrorLogsRecovered(requestId)
	})
}

// newChannelCapacityError 把"全部候选满载"转成对客户端可识别的本地回压响应：
// 纯容量达限 → 429 + Retry-After；容量后端不可用（fail closed）→ 503。
// 不暴露渠道 ID、名称或配置值。
func newChannelCapacityError(c *gin.Context, capacityErr *service.ChannelCapacityExhaustedError) *types.NewAPIError {
	if capacityErr.RedisError {
		c.Header("Retry-After", "1")
		return types.NewErrorWithStatusCode(
			errors.New("capacity backend unavailable; retry later"),
			types.ErrorCodeChannelCapacityBackendUnavailable, http.StatusServiceUnavailable,
			types.ErrOptionWithSkipRetry())
	}
	seconds := (capacityErr.RetryAfterMs + 999) / 1000
	if seconds < 1 {
		seconds = 1
	}
	if seconds > 60 {
		seconds = 60
	}
	c.Header("Retry-After", strconv.FormatInt(seconds, 10))
	return types.NewErrorWithStatusCode(
		errors.New("all eligible upstream channels are at capacity; retry later"),
		types.ErrorCodeChannelCapacityExceeded, http.StatusTooManyRequests,
		types.ErrOptionWithSkipRetry())
}

// tryReserveInitialChannelCapacity 为 attempt 0（distributor 已选定的渠道）补上
// enforce 准入。返回 (decision, isFull)；预留成功时租约暂存 context 等待主循环取走。
func tryReserveInitialChannelCapacity(c *gin.Context, channelID int, attemptIndex int) (service.ChannelCapacityDecision, bool) {
	cached, err := model.CacheGetChannel(channelID)
	if err != nil || cached == nil {
		return service.ChannelCapacityDecision{Allowed: true}, false
	}
	capacity := cached.GetSetting().Capacity
	if !capacity.HasLimit() {
		return service.ChannelCapacityDecision{Allowed: true}, false
	}
	attemptID := service.BuildChannelCapacityAttemptID(c.GetString(common.RequestIdKey), attemptIndex)
	decision := service.TryAcquireChannelCapacity(c, channelID, capacity, operation_setting.ChannelCapacityModeEnforce, attemptID)
	if decision.Allowed {
		common.SetContextKey(c, constant.ContextKeyChannelCapacityLease, decision.Lease)
		return decision, false
	}
	return decision, true
}

func getChannel(c *gin.Context, info *relaycommon.RelayInfo, retryParam *service.RetryParam) (*model.Channel, *types.NewAPIError) {
	// Realtime WebSocket 生命周期不同，容量保护留待 Stage C 接入
	enforceCapacity := info.RelayFormat != types.RelayFormatOpenAIRealtime &&
		operation_setting.GetChannelCapacityMode() == operation_setting.ChannelCapacityModeEnforce
	if info.ChannelMeta == nil {
		autoBan := c.GetBool("auto_ban")
		autoBanInt := 1
		if !autoBan {
			autoBanInt = 0
		}
		initialChannel := &model.Channel{
			Id:      c.GetInt("channel_id"),
			Type:    c.GetInt("channel_type"),
			Name:    c.GetString("channel_name"),
			AutoBan: &autoBanInt,
		}
		if !enforceCapacity {
			service.RequestPolicy(c).BeginAttempt(initialChannel, info.UsingGroup)
			return initialChannel, nil
		}
		decision, isFull := tryReserveInitialChannelCapacity(c, initialChannel.Id, retryParam.GetRetry())
		if !isFull {
			service.RequestPolicy(c).BeginAttempt(initialChannel, info.UsingGroup)
			return initialChannel, nil
		}
		if _, isSpecific, _ := service.GetChannelConstraints(c).ResolvedPin(); isSpecific {
			// 指定渠道满载：不 fallback、不突破上限，直接本地回压
			return nil, newChannelCapacityError(c, &service.ChannelCapacityExhaustedError{
				RetryAfterMs: decision.RetryAfterMs,
				RedisError:   decision.Reason == service.ChannelCapacityReasonRedisError,
			})
		}
		// 初选渠道满载：落入下方加权重选，同一逻辑尝试内换渠道，不消耗 retry
		logger.LogDebug(c, "initial channel #%d at capacity, reselecting", initialChannel.Id)
	}
	retryParam.ReserveCapacity = enforceCapacity
	channel, selectGroup, err := service.CacheGetRandomSatisfiedChannel(retryParam)
	if channel == nil && len(retryParam.ExcludeChannelIds) > 0 {
		// 已失败渠道被排除后没有候选了：放开排除集再选一次。宁可重打同一条，
		// 也不能因为排除本身把「还能重试一次」退化成直接失败。
		// 选路未成功时不会残留容量租约（租约只在选中渠道时写入 context），二次选路安全。
		excluded := retryParam.ExcludeChannelIds
		retryParam.ExcludeChannelIds = nil
		channel, selectGroup, err = service.CacheGetRandomSatisfiedChannel(retryParam)
		retryParam.ExcludeChannelIds = excluded
	}
	if err != nil {
		var capacityErr *service.ChannelCapacityExhaustedError
		if errors.As(err, &capacityErr) {
			return nil, newChannelCapacityError(c, capacityErr)
		}
	}
	if channel == nil && len(retryParam.ExcludeUpstreamHosts) > 0 && info.LastError != nil {
		// 封禁请求的上游被整家排除后没有别家可选：把上游原文还给客户，而不是「无可用渠道」
		return nil, info.LastError
	}
	if err != nil {
		return nil, types.NewError(fmt.Errorf("获取分组 %s 下模型 %s 的可用渠道失败: %s", selectGroup, info.OriginModelName, err.Error()), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}
	if channel == nil {
		return nil, types.NewError(fmt.Errorf("分组 %s 下模型 %s 的可用渠道不存在", selectGroup, info.OriginModelName), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}

	// 重算分组倍率必须连带重新套用户价格覆盖,否则重试后客户的个性折扣会静默失效
	// 并按原价扣费(日志里却仍显示折扣命中)。详见 RefreshGroupRatioForRetry 的注释。
	helper.RefreshGroupRatioForRetry(c, info)

	service.RequestPolicy(c).BeginAttempt(channel, selectGroup)
	newAPIError := middleware.SetupContextForSelectedChannel(c, channel, retryParam.ModelName)
	if newAPIError != nil {
		// 渠道上下文装配失败，请求未派发：撤销选路层的容量预留（RPM 与并发都回退）
		if lease := service.TakeReservedChannelCapacityLease(c); lease != nil {
			_ = lease.CancelBeforeDispatch(c)
		}
		return nil, newAPIError
	}
	if retryParam.ModelName != info.OriginModelName {
		common.SetContextKey(c, constant.ContextKeyOriginalModel, info.OriginModelName)
	}
	return channel, nil
}

func processChannelError(c *gin.Context, channelError types.ChannelError, err *types.NewAPIError, relayInfo *relaycommon.RelayInfo) {
	service.ProcessChannelError(c, channelError, err, relayInfo)
}

func recordSeedanceResourcePoolGuardErrorLog(c *gin.Context, relayInfo *relaycommon.RelayInfo, message string, errorCode string, statusCode int) {
	if !constant.ErrorLogEnabled || relayInfo == nil {
		return
	}
	other := model.NewLogOther()
	if c.Request != nil && c.Request.URL != nil {
		other.SetPublic("request_path", c.Request.URL.Path)
	}
	other.SetPublic("error_type", "new_api_error")
	other.SetPublic("error_code", errorCode)
	other.SetPublic("status_code", statusCode)
	other.SetAdmin("channel_id", 0)
	other.SetPublic("guard", "seedance_resource_pool")
	startTime := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime)
	if startTime.IsZero() {
		startTime = time.Now()
	}
	useTimeSeconds := int(time.Since(startTime).Seconds())
	model.RecordErrorLog(
		c,
		relayInfo.UserId,
		0,
		relayInfo.OriginModelName,
		c.GetString("token_name"),
		message,
		relayInfo.TokenId,
		useTimeSeconds,
		relayInfo.IsStream,
		// 与标准错误日志一致，记录请求实际使用的分组而非用户权限分组
		common.GetContextKeyString(c, constant.ContextKeyUsingGroup),
		other,
	)
}

func RelayMidjourney(c *gin.Context) {
	policy := service.RequestPolicy(c)
	defer func() {
		if policy.Attempts > 0 && !policy.Successful {
			service.RecordRequestPolicyTermination(c, types.NewErrorWithStatusCode(errors.New("Midjourney submission failed"), types.ErrorCodeBadResponseStatusCode, http.StatusBadGateway, types.ErrOptionWithSkipRetry()))
		}
	}()
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatMjProxy, nil, nil)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"description": fmt.Sprintf("failed to generate relay info: %s", err.Error()),
			"type":        "upstream_error",
			"code":        4,
		})
		return
	}

	var mjErr *taskdto.MidjourneyResponse
	switch relayInfo.RelayMode {
	case relayconstant.RelayModeMidjourneyNotify:
		mjErr = relay.RelayMidjourneyNotify(c)
	case relayconstant.RelayModeMidjourneyTaskFetch, relayconstant.RelayModeMidjourneyTaskFetchByCondition:
		mjErr = relay.RelayMidjourneyTask(c, relayInfo.RelayMode)
	case relayconstant.RelayModeMidjourneyTaskImageSeed:
		mjErr = relay.RelayMidjourneyTaskImageSeed(c)
	case relayconstant.RelayModeSwapFace:
		mjErr = relay.RelaySwapFace(c, relayInfo)
	default:
		mjErr = relay.RelayMidjourneySubmit(c, relayInfo)
	}
	//err = relayMidjourneySubmit(c, relayMode)
	log.Println(mjErr)
	if mjErr != nil {
		policy.Successful = false
		statusCode := http.StatusBadRequest
		if mjErr.Code == 30 {
			mjErr.Result = "当前分组负载已饱和，请稍后再试，或升级账户以提升服务质量。"
			statusCode = http.StatusTooManyRequests
		}
		c.JSON(statusCode, gin.H{
			"description": fmt.Sprintf("%s %s", mjErr.Description, mjErr.Result),
			"type":        "upstream_error",
			"code":        mjErr.Code,
		})
		channelId := c.GetInt("channel_id")
		logger.LogError(c, fmt.Sprintf("relay error (channel #%d, status code %d): %s", channelId, statusCode, fmt.Sprintf("%s %s", mjErr.Description, mjErr.Result)))
	}
}

func RelayNotImplemented(c *gin.Context) {
	err := types.OpenAIError{
		Message: "API not implemented",
		Type:    "new_api_error",
		Param:   "",
		Code:    "api_not_implemented",
	}
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": err,
	})
}

func RelayNotFound(c *gin.Context) {
	// The web fallback may already have applied static-asset cache headers.
	// A missing API or asset can appear after an upgrade; never cache its 404.
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, private, max-age=0")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")
	err := types.OpenAIError{
		Message: fmt.Sprintf("Invalid URL (%s %s)", c.Request.Method, c.Request.URL.Path),
		Type:    "invalid_request_error",
		Param:   "",
		Code:    "",
	}
	c.JSON(http.StatusNotFound, gin.H{
		"error": err,
	})
}

// RelayTaskPluginEndpoint keeps unclaimed shared-endpoint traffic on its
// existing handler while claimed requests enter the generation-pinned
// host-owned protocol bridge.
func RelayTaskPluginEndpoint(c *gin.Context, fallback gin.HandlerFunc) {
	pinnedValue, exists := c.Get(pluginruntime.ContextKeyPinnedEndpoint)
	if !exists {
		fallback(c)
		return
	}
	pinned, ok := pinnedValue.(pluginruntime.PinnedEndpoint)
	if !ok || pinned.Plugin == nil || pinned.Generation == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": "Task protocol request failed",
				"type":    "new_api_error",
				"code":    "task_protocol_error",
			},
		})
		return
	}
	switch pinned.Protocol {
	case "openai_responses":
		serveTaskPluginProtocol(c, pinned, defaultPluginProtocolBridgeDeps())
	case pluginruntime.ProtocolOpenAIImage:
		serveTaskPluginImageProtocol(c, pinned, defaultPluginProtocolBridgeDeps())
	default:
		fallback(c)
	}
}

func RelayTaskFetch(c *gin.Context) {
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatTask, nil, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, &taskdto.TaskError{
			Code:       "gen_relay_info_failed",
			Message:    err.Error(),
			StatusCode: http.StatusInternalServerError,
		})
		return
	}
	if taskErr := relay.RelayTaskFetch(c, relayInfo.RelayMode); taskErr != nil {
		respondTaskError(c, taskErr)
	}
}

type taskSubmissionOutcome struct {
	Result    *relay.TaskSubmitResult
	Task      *model.Task
	RelayInfo *relaycommon.RelayInfo
}

func RelayTask(c *gin.Context) {
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatTask, nil, nil)
	if err != nil {
		respondTaskSubmissionError(c, &taskdto.TaskError{
			Code:       "gen_relay_info_failed",
			Message:    err.Error(),
			StatusCode: http.StatusInternalServerError,
		})
		return
	}
	if action := c.GetString("task_action"); action != "" {
		relayInfo.Action = action
	}

	if taskErr := relay.ResolveOriginTask(c, relayInfo); taskErr != nil {
		respondTaskSubmissionError(c, taskErr)
		return
	}
	if taskErr := relay.ApplyOriginTaskAffinity(c, relayInfo); taskErr != nil {
		respondTaskSubmissionError(c, taskErr)
		return
	}
	if message, blocked := operation_setting.IsSeedanceResourcePoolGuardBlocked(relayInfo.UserId, relayInfo.EffectiveRoutingModelName()); blocked {
		recordSeedanceResourcePoolGuardErrorLog(c, relayInfo, message, "model_resource_pool_exhausted", http.StatusTooManyRequests)
		respondTaskError(c, service.TaskErrorWrapperLocal(errors.New(message), "model_resource_pool_exhausted", http.StatusTooManyRequests))
		return
	}

	outcome, taskErr := executeTaskSubmission(c, relayInfo)
	if taskErr != nil {
		respondTaskSubmissionError(c, taskErr)
		return
	}
	presentTaskSubmission(c, outcome)
}

// executeTaskSubmission owns the retry, billing, and persistence lifecycle.
// It deliberately performs no client response writes so JSON and protocol
// presenters share the same durable task barrier. Its cancellation semantics
// come from c.Request.Context: native task endpoints use the client context,
// while the Responses bridge supplies an independently bounded context.
func executeTaskSubmission(c *gin.Context, relayInfo *relaycommon.RelayInfo) (*taskSubmissionOutcome, *taskdto.TaskError) {
	return executeTaskSubmissionWith(c, relayInfo, relay.RelayTaskSubmit)
}

type taskSubmitAttempt func(*gin.Context, *relaycommon.RelayInfo) (*relay.TaskSubmitResult, *taskdto.TaskError)

func executeTaskSubmissionWith(
	c *gin.Context,
	relayInfo *relaycommon.RelayInfo,
	submit taskSubmitAttempt,
) (*taskSubmissionOutcome, *taskdto.TaskError) {
	policy := service.RequestPolicy(c)
	diagnostics := newTaskPluginSubmitDiagnostics(c)
	diagnostics.start(relayInfo)
	var result *relay.TaskSubmitResult
	var taskErr *taskdto.TaskError
	durable := false
	stage := "start"
	defer func() {
		if durable {
			return
		}
		reason := "task submission aborted"
		if taskErr != nil {
			reason = taskErr.Message
		}
		diagnostics.refund(stage)
		// 未落成正式任务：把 submitting 记录标记失败并退款；记录不存在时直接退预扣与限量池。
		handled, finalizeErr := service.MarkTaskSubmissionFailed(context.WithoutCancel(c.Request.Context()), relayInfo.UserId, relayInfo.PublicTaskID, reason)
		if finalizeErr != nil {
			common.SysError("finalize failed task submission error: " + finalizeErr.Error())
		}
		if !handled {
			if relayInfo.Billing != nil {
				if refundErr := relayInfo.Billing.Refund(c); refundErr != nil {
					common.SysError("persist task refund error: " + refundErr.Error())
				}
			}
			service.RollbackModelQuotaPool(relayInfo)
		}
	}()
	stage = "before_attempt"
	if requestErr := c.Request.Context().Err(); requestErr != nil {
		diagnostics.cancelled("before_attempt", 0)
		return nil, service.TaskErrorWrapperLocal(requestErr, "request_cancelled", http.StatusRequestTimeout)
	}

	retryParam := &service.RetryParam{
		Ctx:               c,
		TokenGroup:        relayInfo.TokenGroup,
		ModelName:         relayInfo.EffectiveRoutingModelName(),
		RequestPath:       c.Request.URL.Path,
		Retry:             common.GetPointer(0),
		ExcludeChannelIds: map[int]bool{},
	}

	for ; retryParam.GetRetry() <= common.RetryTimes; retryParam.IncreaseRetry() {
		stage = "select_channel"
		if requestErr := c.Request.Context().Err(); requestErr != nil {
			diagnostics.cancelled("before_attempt", retryParam.GetRetry()+1)
			taskErr = service.TaskErrorWrapperLocal(requestErr, "request_cancelled", http.StatusRequestTimeout)
			break
		}
		var channel *model.Channel

		if lockedCh, ok := relayInfo.LockedChannel.(*model.Channel); ok && lockedCh != nil {
			channel = lockedCh
			if !service.AllowSelectedChannelByUserRouting(c, retryParam.TokenGroup, retryParam.ModelName, channel) {
				taskErr = service.TaskErrorWrapperLocal(fmt.Errorf("locked channel is not allowed by user channel routing"), "user_channel_routing_denied", http.StatusServiceUnavailable)
				break
			}
			if !service.AllowSelectedChannelByBreaker(c, channel) {
				taskErr = service.TaskErrorWrapperLocal(fmt.Errorf("channel breaker is open"), "channel_breaker_open", http.StatusServiceUnavailable)
				break
			}
			policy.BeginAttempt(channel, relayInfo.UsingGroup)
			if retryParam.GetRetry() > 0 {
				if setupErr := middleware.SetupContextForSelectedChannel(c, channel, relayInfo.EffectiveRoutingModelName()); setupErr != nil {
					taskErr = service.TaskErrorWrapperLocal(setupErr.Err, "setup_locked_channel_failed", http.StatusInternalServerError)
					break
				}
				if relayInfo.EffectiveRoutingModelName() != relayInfo.OriginModelName {
					common.SetContextKey(c, constant.ContextKeyOriginalModel, relayInfo.OriginModelName)
				}
			}
		} else {
			var channelErr *types.NewAPIError
			channel, channelErr = getChannel(c, relayInfo, retryParam)
			if channelErr != nil {
				logger.LogError(c, channelErr.Error())
				taskErr = service.TaskErrorWrapperLocal(channelErr.Err, "get_channel_failed", channelErr.StatusCode)
				break
			}
		}
		diagnostics.attempt(retryParam.GetRetry()+1, channel, relayInfo.LockedChannel != nil)

		service.AppendUsedChannel(c, channel.Id)
		bodyStorage, bodyErr := common.GetBodyStorage(c)
		if bodyErr != nil {
			stage = "read_body"
			if common.IsRequestBodyTooLargeError(bodyErr) || errors.Is(bodyErr, common.ErrRequestBodyTooLarge) {
				taskErr = service.TaskErrorWrapperLocal(bodyErr, "read_request_body_failed", http.StatusRequestEntityTooLarge)
			} else {
				taskErr = service.TaskErrorWrapperLocal(bodyErr, "read_request_body_failed", http.StatusBadRequest)
			}
			break
		}
		c.Request.Body = io.NopCloser(bodyStorage)

		stage = "submit"
		result, taskErr = submit(c, relayInfo)
		if requestErr := c.Request.Context().Err(); requestErr != nil {
			diagnostics.cancelled("after_submit", retryParam.GetRetry()+1)
			taskErr = service.TaskErrorWrapperLocal(requestErr, "request_cancelled", http.StatusRequestTimeout)
			break
		}
		if taskErr == nil {
			service.RecordChannelBreakerSuccess(c, *types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, common.GetContextKeyString(c, constant.ContextKeyChannelKey), channel.GetAutoBan(), common.IsChannelBreakerExemptChannel(channel.Id)))
			markRecoveredErrorLogs(c)
			diagnostics.attemptSucceeded(retryParam.GetRetry()+1, result)
			break
		}

		// 这一条本次已经失败，重试选路时同层排除它，避免加权随机又抽回来
		retryParam.ExcludeChannelIds[channel.Id] = true

		taskAPIError := taskSubmissionAPIError(taskErr)
		relayInfo.LastError = taskAPIError
		decision := decideTaskRetry(c, taskErr, common.RetryTimes-retryParam.GetRetry())
		service.RecordPolicyFailure(c, channel.Id, taskAPIError, decision)
		if !taskErr.LocalError {
			processChannelError(c,
				*types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey,
					common.GetContextKeyString(c, constant.ContextKeyChannelKey), channel.GetAutoBan(), common.IsChannelBreakerExemptChannel(channel.Id)),
				taskAPIError,
				relayInfo)
		}

		willRetry := decision.Action == "retry"
		diagnostics.attemptFailed(retryParam.GetRetry()+1, channel, taskErr, willRetry)
		if !willRetry {
			break
		}
	}

	useChannel := c.GetStringSlice("use_channel")
	if len(useChannel) > 1 {
		retryLogStr := fmt.Sprintf("重试：%s", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(useChannel)), "->"), "[]"))
		logger.LogInfo(c, retryLogStr)
	}

	if taskErr != nil {
		diagnostics.failed(stage, "task_error", taskErr, false)
		return nil, taskErr
	}
	if result == nil {
		taskErr = service.TaskErrorWrapperLocal(errors.New("task submission returned no result"), "task_submit_failed", http.StatusInternalServerError)
		diagnostics.failed("submit", "missing_result", taskErr, false)
		return nil, taskErr
	}
	if requestErr := c.Request.Context().Err(); requestErr != nil {
		diagnostics.cancelled("before_reserve", retryParam.GetRetry()+1)
		return nil, service.TaskErrorWrapperLocal(requestErr, "request_cancelled", http.StatusRequestTimeout)
	}

	// Reserve any submit-time upward billing adjustment before persistence.
	// This keeps insertion failures fully refundable while ensuring settlement
	// after the barrier normally has a zero positive delta.
	if relayInfo.Billing != nil {
		stage = "reserve"
		diagnostics.reserve("reserve_start", result.Quota)
		if reserveErr := relayInfo.Billing.Reserve(result.Quota); reserveErr != nil {
			common.SysError("reserve adjusted task billing error: " + reserveErr.Error())
			taskErr = service.TaskErrorWrapperLocal(errors.New("insufficient quota for adjusted task cost"), string(types.ErrorCodeInsufficientUserQuota), http.StatusForbidden)
			diagnostics.failed("reserve", "insufficient_quota", taskErr, false)
			return nil, taskErr
		}
		diagnostics.reserve("reserve_complete", result.Quota)
	}
	if requestErr := c.Request.Context().Err(); requestErr != nil {
		diagnostics.cancelled("before_insert", retryParam.GetRetry()+1)
		return nil, service.TaskErrorWrapperLocal(requestErr, "request_cancelled", http.StatusRequestTimeout)
	}

	stage = "insert"
	execution := service.TaskExecutionSnapshotFromContext(c)
	// A native submit route may declare retainResult: false. It applies only
	// to immediate terminal results: the client receives the complete response
	// once, the upstream snapshot is never persisted, and the task is not
	// retrievable afterwards. An asynchronous result on such a route keeps its
	// snapshot because polling and retrieval need it. The OpenAI Images
	// protocol delivers its images inline in the same HTTP response, so its
	// immediate results follow the same rule; an asynchronous image task is
	// expected there and is polled inside the request.
	resultDiscarded := false
	immediateTerminal := result.Immediate != nil && (result.Immediate.Status == model.TaskStatusSuccess || result.Immediate.Status == model.TaskStatusFailure)
	if pinnedValue, exists := c.Get(pluginruntime.ContextKeyPinnedRoute); exists {
		pinned, ok := pinnedValue.(pluginruntime.PinnedRoute)
		if ok && pinned.Route.RetainResult != nil && !*pinned.Route.RetainResult {
			if immediateTerminal {
				resultDiscarded = true
			} else {
				logger.LogWarn(c, fmt.Sprintf("task plugin route %s %s declares retainResult: false but returned an asynchronous result; retaining task %s", pinned.Route.Method, pinned.Route.Path, relayInfo.PublicTaskID))
			}
		}
	}
	if pinnedValue, exists := c.Get(pluginruntime.ContextKeyPinnedEndpoint); exists && immediateTerminal {
		if pinned, ok := pinnedValue.(pluginruntime.PinnedEndpoint); ok && pinned.Protocol == pluginruntime.ProtocolOpenAIImage {
			resultDiscarded = true
		}
	}
	// 提交前已写下 submitting 记录（崩溃可恢复），这里把它推进为正式任务。
	task, updateErr := service.CompleteTaskSubmissionRecordWith(c, relayInfo, result.Platform, result.UpstreamTaskID, result.TaskData, result.Quota, func(task *model.Task) {
		task.PrivateData.Execution = execution
		if len(result.PluginState) > 0 {
			task.PrivateData.PluginState = result.PluginState
		}
		if immediate := result.Immediate; immediate != nil {
			task.Status = model.TaskStatus(immediate.Status)
			task.Progress = immediate.Progress
			if immediate.Status == model.TaskStatusSuccess || immediate.Status == model.TaskStatusFailure {
				task.FinishTime = time.Now().Unix()
			}
			if immediate.Status == model.TaskStatusFailure {
				task.FailReason = immediate.Reason
			}
			if immediate.Url != "" {
				task.PrivateData.ResultURL = immediate.Url
			} else if immediate.Status == model.TaskStatusSuccess {
				task.PrivateData.ResultURL = taskcommon.BuildProxyURL(task.TaskID)
			}
		}
		if resultDiscarded {
			task.PrivateData.ResultDiscarded = true
			task.Data = nil
		}
		diagnostics.insertStart(task)
	})
	if updateErr != nil {
		common.SysError("complete task submission record error: " + updateErr.Error())
		taskErr = service.TaskErrorWrapperLocal(errors.New("failed to persist task"), "task_insert_failed", http.StatusInternalServerError)
		diagnostics.failed("insert", "database_error", taskErr, false)
		return nil, taskErr
	}
	if resultDiscarded {
		// 上游快照不落库，但本次响应仍要把完整结果交给客户端
		task.Data = result.TaskData
	}
	durable = true
	stage = "settle"
	diagnostics.durable(task)
	diagnostics.settleStart(task, result.Quota)

	// 任务已经提交到上游，结算失败不能让客户以为没提交：记录后由提交结算标记驱动后台补做。
	settleErr := service.SettleBilling(c, relayInfo, result.Quota)
	if settleErr != nil {
		common.SysError("settle task billing error: " + settleErr.Error())
	}
	poolErr := service.SettleModelQuotaPool(relayInfo, service.ModelQuotaPoolSettlement{
		ActualQuota:    result.Quota,
		HasActualQuota: true,
	})
	if poolErr != nil {
		common.SysError("settle task model quota pool error: " + poolErr.Error())
	}
	if syncErr := service.SyncTaskSubmissionBillingContext(relayInfo); syncErr != nil {
		common.SysError("sync task submission billing context error: " + syncErr.Error())
	} else if settleErr == nil && poolErr == nil {
		if completeErr := service.CompleteTaskSubmissionSettlement(relayInfo.UserId, relayInfo.PublicTaskID); completeErr != nil {
			common.SysError("complete task submission settlement marker error: " + completeErr.Error())
		}
	}
	if task.Status != model.TaskStatusFailure {
		service.MarkRequestPolicySuccess(c, nil)
	} else {
		policy.AddEvent(service.PolicyEvent{Decision: service.PolicyDecision{Action: "stop", Reason: "task_failed", Source: "upstream"}})
	}
	service.LogTaskConsumption(c, relayInfo, task)
	diagnostics.complete(task, result.Quota)

	return &taskSubmissionOutcome{Result: result, Task: task, RelayInfo: relayInfo}, nil
}

func presentTaskSubmission(c *gin.Context, outcome *taskSubmissionOutcome) {
	diagnostics := newTaskPluginSubmitDiagnostics(c)
	otherRatios := outcome.RelayInfo.PriceData.OtherRatios()
	if otherRatios == nil {
		otherRatios = map[string]float64{}
	}
	if ratiosJSON, err := common.Marshal(otherRatios); err == nil {
		c.Header("X-New-Api-Other-Ratios", string(ratiosJSON))
	}
	if pinnedValue, exists := c.Get(pluginruntime.ContextKeyPinnedRoute); exists {
		if pinned, ok := pinnedValue.(pluginruntime.PinnedRoute); ok && pinned.Plugin != nil && pinned.Route.Render != "" {
			view, err := service.BuildTaskPluginView(outcome.Task)
			requestValue, _ := c.Get(pluginruntime.ContextKeyRouteRequest)
			requestContext, _ := requestValue.(pluginruntime.RouteRequestContext)
			if err == nil {
				viewValue, valueErr := taskPluginProtocolJSONValue(view)
				if valueErr == nil {
					if body, callErr := pinned.Plugin.Engine.CallPath(c.Request.Context(), "native", []string{pinned.Route.Render}, requestContext.JSValue(), viewValue); callErr == nil {
						diagnostics.present(outcome.Task, "native_presenter")
						c.JSON(http.StatusOK, body)
						return
					} else {
						logger.LogError(c, "task plugin native submit presenter failed: "+callErr.Error())
					}
				} else {
					logger.LogError(c, "encode task plugin native submit view failed: "+valueErr.Error())
				}
			} else {
				logger.LogError(c, "build task plugin native submit view failed: "+err.Error())
			}
		}
	}
	if pinnedValue, exists := c.Get(pluginruntime.ContextKeyPinnedEndpoint); exists {
		if pinned, ok := pinnedValue.(pluginruntime.PinnedEndpoint); ok && pinned.Protocol == "openai_video" && pinned.Operation.Name == "create" {
			diagnostics.present(outcome.Task, "openai_video_create")
			c.JSON(http.StatusOK, outcome.Task.ToOpenAIVideo())
			return
		}
	}
	// 自研 Go 适配器沿用原先写给客户端的响应体，保证对外格式不变。
	if outcome.Result != nil && outcome.Result.ClientResponse != nil {
		diagnostics.present(outcome.Task, "adaptor_response")
		c.JSON(http.StatusOK, outcome.Result.ClientResponse)
		return
	}
	createdAt := outcome.Task.CreatedAt
	if createdAt == 0 {
		createdAt = outcome.Task.SubmitTime
	}
	diagnostics.present(outcome.Task, "host_fallback")
	c.JSON(http.StatusOK, map[string]any{
		"id":         outcome.Task.TaskID,
		"task_id":    outcome.Task.TaskID,
		"status":     "queued",
		"model":      outcome.RelayInfo.OriginModelName,
		"created_at": createdAt,
	})
}

func respondTaskSubmissionError(c *gin.Context, taskErr *taskdto.TaskError) {
	service.RecordRequestPolicyTermination(c, taskSubmissionAPIError(taskErr))
	newTaskPluginSubmitDiagnostics(c).presentError(taskErr)
	if middleware.RespondTaskPluginError(c, taskErr) {
		return
	}
	respondTaskError(c, taskErr)
}

// respondTaskError 统一输出 Task 错误响应（含 429 限流提示改写）
func respondTaskError(c *gin.Context, taskErr *taskdto.TaskError) {
	if !taskErr.LocalError && types.MatchesUpstreamBillingLeak(taskErr.Message) {
		taskErr.StatusCode = http.StatusTooManyRequests
	}
	if taskErr.StatusCode == http.StatusTooManyRequests {
		taskErr.Message = "当前分组上游负载已饱和，请稍后再试"
	}
	c.JSON(taskErr.StatusCode, taskErr)
}

// taskSubmissionAPIError adapts a task error for the shared relay error paths.
// TaskError.Error is nil for many local rejections, so fall back to the message.
func taskSubmissionAPIError(taskErr *taskdto.TaskError) *types.NewAPIError {
	err := taskErr.Error
	if err == nil {
		err = errors.New(taskErr.Message)
	}
	return types.NewOpenAIError(err, types.ErrorCodeBadResponseStatusCode, taskErr.StatusCode)
}

// decideTaskRetry is the single retry decision for task submissions. The
// reason is recorded in the request policy decision events of the log details.
func decideTaskRetry(c *gin.Context, taskErr *taskdto.TaskError, retryTimes int) service.PolicyDecision {
	stop := service.PolicyDecision{Action: "stop", Source: "system"}
	retry := service.PolicyDecision{Action: "retry", Reason: "retry_status_matched", Source: "system"}
	switch {
	case taskErr == nil:
		stop.Reason = "request_completed"
	case taskErr.NoRetry:
		stop.Reason = "task_accepted"
	case service.ShouldSkipRetryAfterChannelAffinityFailure(c):
		stop.Reason, stop.Source = "strict_session", "session_rule"
		if source := service.RequestPolicy(c).SessionModeSource; source != "" {
			stop.Source = source
		}
	case retryTimes <= 0:
		stop.Reason, stop.Source = "attempt_budget_exhausted", "global"
	case service.GetChannelConstraints(c).SuppressesRetry():
		stop.Reason, stop.Source = "pinned_channel", "channel_constraint"
	case taskErr.StatusCode == http.StatusTooManyRequests, taskErr.StatusCode == 307:
		return retry
	case taskErr.StatusCode/100 == 5:
		// 超时不重试
		if operation_setting.IsAlwaysSkipRetryStatusCode(taskErr.StatusCode) {
			stop.Reason = "system_retry_exclusion"
			break
		}
		return retry
	case taskErr.StatusCode == http.StatusBadRequest, taskErr.StatusCode == 408:
		// azure处理超时不重试
		stop.Reason = "status_not_retryable"
	case taskErr.LocalError:
		stop.Reason = "local_rejection"
	case taskErr.StatusCode/100 == 2:
		stop.Reason = "system_retry_exclusion"
	default:
		return retry
	}
	return stop
}
