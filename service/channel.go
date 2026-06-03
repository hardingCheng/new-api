package service

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
)

type ChannelBreakerNotificationContext struct {
	ChannelError types.ChannelError
	Reason       string
	ModelName    string
	Group        string
	UserId       int
	Username     string
	TokenId      int
	RequestPath  string
	StatusCode   int
}

func formatNotifyType(channelId int, status int) string {
	return fmt.Sprintf("%s_%d_%d", dto.NotifyTypeChannelUpdate, channelId, status)
}

func NotifyChannelBreakerOpen(ctx ChannelBreakerNotificationContext) {
	channelError := ctx.ChannelError
	reason := ctx.Reason
	common.SysLog(fmt.Sprintf("通道「%s」（#%d）发生错误，进入熔断，原因：%s", channelError.ChannelName, channelError.ChannelId, common.LocalLogPreview(reason)))
	subject := fmt.Sprintf("通道「%s」（#%d）已进入熔断", channelError.ChannelName, channelError.ChannelId)
	content := fmt.Sprintf("通道「%s」（#%d）已进入熔断，原因：%s", channelError.ChannelName, channelError.ChannelId, reason)
	if operation_setting.GetMonitorSetting().ChannelBreakerAlertEnabled {
		body := buildChannelBreakerBarkBody(ctx)
		if err := SendSystemBarkNotify(subject, body, "new-api 熔断告警", "critical"); err != nil {
			common.SysError(fmt.Sprintf("failed to send channel breaker bark notify for channel %d: %s", channelError.ChannelId, err.Error()))
		}
	}
	NotifyRootUser(formatNotifyType(channelError.ChannelId, common.ChannelStatusAutoDisabled), subject, content)
}

func buildChannelBreakerBarkBody(ctx ChannelBreakerNotificationContext) string {
	channelTypeName := constant.GetChannelTypeName(ctx.ChannelError.ChannelType)
	lines := []string{
		fmt.Sprintf("渠道：%s (#%d)", ctx.ChannelError.ChannelName, ctx.ChannelError.ChannelId),
		fmt.Sprintf("类型：%s (%d)", channelTypeName, ctx.ChannelError.ChannelType),
	}
	if ctx.ModelName != "" {
		lines = append(lines, fmt.Sprintf("模型：%s", ctx.ModelName))
	}
	if ctx.Group != "" {
		lines = append(lines, fmt.Sprintf("分组：%s", ctx.Group))
	}
	if ctx.Username != "" {
		lines = append(lines, fmt.Sprintf("用户：%s (#%d)", ctx.Username, ctx.UserId))
	} else if ctx.UserId != 0 {
		lines = append(lines, fmt.Sprintf("用户：#%d", ctx.UserId))
	}
	if ctx.TokenId != 0 {
		lines = append(lines, fmt.Sprintf("令牌：#%d", ctx.TokenId))
	}
	if ctx.RequestPath != "" {
		lines = append(lines, fmt.Sprintf("路径：%s", ctx.RequestPath))
	}
	if ctx.StatusCode != 0 {
		lines = append(lines, fmt.Sprintf("状态码：%d", ctx.StatusCode))
	}
	if ctx.ChannelError.UsingKey != "" {
		lines = append(lines, fmt.Sprintf("密钥哈希：%s", ChannelBreakerKeyHash(ctx.ChannelError.UsingKey)))
	}
	lines = append(lines, fmt.Sprintf("原因：%s", ctx.Reason))
	return strings.Join(lines, "\n")
}

// disable & notify
func DisableChannel(channelError types.ChannelError, reason string) {
	common.SysLog(fmt.Sprintf("通道「%s」（#%d）准备禁用，原因：%s", channelError.ChannelName, channelError.ChannelId, common.LocalLogPreview(reason)))

	if !channelError.AutoBan {
		common.SysLog(fmt.Sprintf("通道「%s」（#%d）未启用自动禁用功能，跳过禁用操作", channelError.ChannelName, channelError.ChannelId))
		return
	}

	success := model.UpdateChannelStatus(channelError.ChannelId, channelError.UsingKey, common.ChannelStatusAutoDisabled, reason)
	if success {
		ClearChannelBreaker(channelError)
		subject := fmt.Sprintf("通道「%s」（#%d）已被禁用", channelError.ChannelName, channelError.ChannelId)
		content := fmt.Sprintf("通道「%s」（#%d）已被禁用，原因：%s", channelError.ChannelName, channelError.ChannelId, reason)
		NotifyRootUser(formatNotifyType(channelError.ChannelId, common.ChannelStatusAutoDisabled), subject, content)
	}
}

func EnableChannel(channelId int, usingKey string, channelName string) {
	success := model.UpdateChannelStatus(channelId, usingKey, common.ChannelStatusEnabled, "")
	if success {
		ClearChannelBreaker(types.ChannelError{ChannelId: channelId, UsingKey: usingKey})
		subject := fmt.Sprintf("通道「%s」（#%d）已被启用", channelName, channelId)
		content := fmt.Sprintf("通道「%s」（#%d）已被启用", channelName, channelId)
		NotifyRootUser(formatNotifyType(channelId, common.ChannelStatusEnabled), subject, content)
	}
}

func ShouldDisableChannel(err *types.NewAPIError) bool {
	if !common.IsAutomaticDisableChannelEnabled() {
		return false
	}
	return ShouldTripChannelBreaker(err)
}

func ShouldTripChannelBreaker(err *types.NewAPIError) bool {
	if err == nil {
		return false
	}
	if types.IsChannelError(err) {
		return true
	}
	if types.IsSkipRetryError(err) {
		return false
	}
	if operation_setting.ShouldDisableByStatusCode(err.StatusCode) {
		return true
	}

	lowerMessage := strings.ToLower(err.Error())
	search, _ := AcSearch(lowerMessage, operation_setting.GetAutomaticDisableKeywords(), true)
	return search
}

func ShouldEnableChannel(newAPIError *types.NewAPIError, status int) bool {
	if !common.IsAutomaticEnableChannelEnabled() {
		return false
	}
	if newAPIError != nil {
		return false
	}
	if status != common.ChannelStatusAutoDisabled {
		return false
	}
	return true
}
