package service

import (
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
)

// DecideRelayRetry is the single retry decision for relay attempts. The reason
// is recorded in the request policy decision events of the log details.
func DecideRelayRetry(c *gin.Context, err *types.NewAPIError, retryTimes int) PolicyDecision {
	if err == nil {
		return PolicyDecision{Action: "stop", Reason: "request_completed", Source: "system"}
	}
	if ShouldSkipRetryAfterChannelAffinityFailure(c) {
		source := RequestPolicy(c).SessionModeSource
		if source == "" {
			source = "session_rule"
		}
		return PolicyDecision{Action: "stop", Reason: "strict_session", Source: source}
	}
	if GetChannelConstraints(c).SuppressesRetry() {
		return PolicyDecision{Action: "stop", Reason: "pinned_channel", Source: "channel_constraint"}
	}
	if types.IsChannelError(err) {
		return PolicyDecision{Action: "retry", Reason: "channel_error", Source: "system"}
	}
	if types.IsSkipRetryError(err) {
		return PolicyDecision{Action: "stop", Reason: "non_retryable_error", Source: "system"}
	}
	if retryTimes <= 0 {
		return PolicyDecision{Action: "stop", Reason: "attempt_budget_exhausted", Source: "global"}
	}
	code := err.StatusCode
	if code >= 200 && code < 300 {
		return PolicyDecision{Action: "stop", Reason: "system_retry_exclusion", Source: "system"}
	}
	if code < 100 || code > 599 {
		return PolicyDecision{Action: "retry", Reason: "unrecognized_status", Source: "system"}
	}
	if operation_setting.IsAlwaysSkipRetryCode(err.GetErrorCode()) || operation_setting.IsAlwaysSkipRetryStatusCode(code) {
		return PolicyDecision{Action: "stop", Reason: "system_retry_exclusion", Source: "system"}
	}
	if operation_setting.ShouldRetryByStatusCode(code) {
		return PolicyDecision{Action: "retry", Reason: "retry_status_matched", Source: "global"}
	}
	if code == http.StatusBadRequest && isRetryableUpstream400(c, err) {
		return PolicyDecision{Action: "retry", Reason: "upstream_error_retry_matched", Source: "global"}
	}
	return PolicyDecision{Action: "stop", Reason: "status_not_retryable", Source: "global"}
}

// upstream400RetryLimit 每个请求因上游 400 换渠道重试的次数上限。
// 这类 400 有一部分是上游对请求内容的确定性拒绝，多试几条渠道只会空转，
// 所以只给一次机会，不吃满全局重试次数。
const upstream400RetryLimit = 1

// isRetryableUpstream400 判断上游返回的 400 是否属于渠道自身的问题、值得换渠道再试：
// 错误内容为空（上游没说明原因），或错误文案命中配置的关键词（如渠道能力差异）。
// 本地参数校验产生的 400、不在生效分组内的请求、已经因此重试过的请求都不算。
func isRetryableUpstream400(c *gin.Context, err *types.NewAPIError) bool {
	if !common.IsUpstream400RetryEnabled() {
		return false
	}
	if err.GetErrorType() == types.ErrorTypeNewAPIError {
		return false
	}
	state := RequestPolicy(c)
	if groups := common.GetUpstream400RetryGroups(); len(groups) > 0 && !slices.Contains(groups, state.SelectedGroup) {
		return false
	}
	retried := 0
	for _, event := range state.Events() {
		if event.Decision.Reason == "upstream_error_retry_matched" {
			retried++
		}
	}
	if retried >= upstream400RetryLimit {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	if message == "" {
		return true
	}
	matched, _ := AcSearch(message, common.GetUpstream400RetryKeywords(), true)
	return matched
}

// ChannelUpstreamHost 返回渠道上游地址的主机名，用来判断几条渠道是否接的同一家上游。
func ChannelUpstreamHost(channel *model.Channel) string {
	if channel == nil {
		return ""
	}
	parsed, err := url.Parse(channel.GetBaseURL())
	if err != nil {
		return ""
	}
	return strings.ToLower(parsed.Hostname())
}

// ExcludeUpstreamAfterSessionBlock 处理上游按会话封禁请求的错误（如网络安全策略屏蔽）。
// 同一上游的各条渠道共用一套封禁判定，所以命中关键词后本请求排除失败渠道所在上游的全部渠道，
// 重试只去别家；同时清掉该会话的渠道亲和，后续请求重新选路，不再粘在封禁它的上游上。
// 返回是否命中。
func ExcludeUpstreamAfterSessionBlock(c *gin.Context, channelID int, err *types.NewAPIError, param *RetryParam) bool {
	if err == nil || param == nil || err.GetErrorType() == types.ErrorTypeNewAPIError {
		return false
	}
	keywords := common.GetUpstreamSessionBlockKeywords()
	if len(keywords) == 0 {
		return false
	}
	if matched, _ := AcSearch(strings.ToLower(err.Error()), keywords, true); !matched {
		return false
	}
	ClearCurrentChannelAffinityCache(c)
	channel, cacheErr := model.CacheGetChannel(channelID)
	if cacheErr != nil {
		return true
	}
	host := ChannelUpstreamHost(channel)
	if host == "" {
		return true
	}
	if param.ExcludeUpstreamHosts == nil {
		param.ExcludeUpstreamHosts = map[string]bool{}
	}
	param.ExcludeUpstreamHosts[host] = true
	logger.LogWarn(c, fmt.Sprintf("upstream %s blocked this session on channel #%d, retrying on other upstreams", host, channelID))
	return true
}

func ShouldRetryRelayError(c *gin.Context, openaiErr *types.NewAPIError, retryTimes int) bool {
	return DecideRelayRetry(c, openaiErr, retryTimes).Action == "retry"
}

func ProcessChannelError(c *gin.Context, channelError types.ChannelError, err *types.NewAPIError, relayInfo *relaycommon.RelayInfo) {
	if err == nil {
		return
	}
	logger.LogError(c, fmt.Sprintf("channel error (channel #%d, status code: %d): %s", channelError.ChannelId, err.StatusCode, common.LocalLogPreview(err.MaskSensitiveErrorWithStatusCode())))
	// 不要使用context获取渠道信息，异步处理时可能会出现渠道信息不一致的情况
	if handled, reason := HandleInstantDisableChannel(c, channelError, err); handled {
		if channelError.IsMultiKey && channelError.UsingKey != "" {
			logger.LogWarn(c, fmt.Sprintf("%s，已立即禁用渠道 #%d 的故障密钥", reason, channelError.ChannelId))
		} else {
			logger.LogWarn(c, fmt.Sprintf("%s，已立即禁用整个渠道 #%d", reason, channelError.ChannelId))
		}
	} else if opened, message := RecordChannelBreakerFailure(c, channelError, ShouldTripChannelBreakerWithRule(c, channelError, err)); message != "" {
		if opened {
			logger.LogWarn(c, fmt.Sprintf("%s (channel #%d)", message, channelError.ChannelId))
			group := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
			if group == "" {
				group = common.GetContextKeyString(c, constant.ContextKeyTokenGroup)
			}
			if group == "" {
				group = common.GetContextKeyString(c, constant.ContextKeyUserGroup)
			}
			requestPath := ""
			if c.Request != nil && c.Request.URL != nil {
				requestPath = c.Request.URL.Path
			}
			notifyCtx := ChannelBreakerNotificationContext{
				ChannelError: channelError,
				Reason:       err.ErrorWithStatusCode(),
				ModelName:    common.GetContextKeyString(c, constant.ContextKeyOriginalModel),
				Group:        group,
				UserId:       common.GetContextKeyInt(c, constant.ContextKeyUserId),
				Username:     common.GetContextKeyString(c, constant.ContextKeyUserName),
				TokenId:      common.GetContextKeyInt(c, constant.ContextKeyTokenId),
				RequestPath:  requestPath,
				StatusCode:   err.StatusCode,
			}
			gopool.Go(func() {
				NotifyChannelBreakerOpen(notifyCtx)
			})
		} else {
			logger.LogInfo(c, fmt.Sprintf("%s (channel #%d)", message, channelError.ChannelId))
		}
	}

	if constant.ErrorLogEnabled && types.IsRecordErrorLog(err) {
		userId := c.GetInt("id")
		tokenName := c.GetString("token_name")
		modelName := c.GetString("original_model")
		tokenId := c.GetInt("token_id")
		userGroup := c.GetString("group")
		other := model.NewLogOther()
		if c.Request != nil && c.Request.URL != nil {
			other.SetPublic("request_path", c.Request.URL.Path)
		}
		other.SetPublic("error_type", err.GetErrorType())
		other.SetPublic("error_code", err.GetErrorCode())
		other.SetPublic("status_code", err.StatusCode)
		// 渠道标识只给管理端看（日志行本身带 channel_id 列）
		other.SetAdmin("channel_name", c.GetString("channel_name"))
		other.SetAdmin("channel_type", c.GetInt("channel_type"))
		AppendRelayLogAdminInfo(c, relayInfo, other)
		AppendResponseModelLogInfo(relayInfo, other)
		AppendTaskPluginContextAuditInfo(c, other)
		startTime := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime)
		if startTime.IsZero() {
			startTime = time.Now()
		}
		useTimeSeconds := int(time.Since(startTime).Seconds())
		model.RecordErrorLog(c, userId, channelError.ChannelId, modelName, tokenName, err.MaskSensitiveErrorWithStatusCode(), tokenId, useTimeSeconds, common.GetContextKeyBool(c, constant.ContextKeyIsStream), userGroup, other)
	}
}
