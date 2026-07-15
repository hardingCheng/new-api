package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
)

// channelDisableNotifyMemory 记录每个渠道上次禁用告警时间，用于按冷却窗口去重，
// 避免同一渠道反复禁用/抖动时刷屏。key=channelId，value=time.Time。
var channelDisableNotifyMemory sync.Map

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
		if err := SendSystemBarkNotify(subject, body, "new-api 熔断告警", "critical", operation_setting.GetMonitorSetting().ChannelBreakerAlertSound); err != nil {
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
	disableChannel(channelError, reason, false)
}

func DisableChannelWithoutAutoRecovery(channelError types.ChannelError, reason string) {
	disableChannel(channelError, reason, true)
}

func disableChannel(channelError types.ChannelError, reason string, disableAutoRecovery bool) {
	common.SysLog(fmt.Sprintf("通道「%s」（#%d）准备禁用，原因：%s", channelError.ChannelName, channelError.ChannelId, common.LocalLogPreview(reason)))

	if !channelError.AutoBan {
		common.SysLog(fmt.Sprintf("通道「%s」（#%d）未启用自动禁用功能，跳过禁用操作", channelError.ChannelName, channelError.ChannelId))
		return
	}

	var success bool
	if disableAutoRecovery {
		success = model.UpdateChannelStatusWithoutAutoRecovery(channelError.ChannelId, channelError.UsingKey, common.ChannelStatusAutoDisabled, reason)
	} else {
		success = model.UpdateChannelStatus(channelError.ChannelId, channelError.UsingKey, common.ChannelStatusAutoDisabled, reason)
	}
	if success {
		ClearChannelBreaker(channelError)
		subject := fmt.Sprintf("通道「%s」（#%d）已被禁用", channelError.ChannelName, channelError.ChannelId)
		content := fmt.Sprintf("通道「%s」（#%d）已被禁用，原因：%s", channelError.ChannelName, channelError.ChannelId, reason)
		if channelError.IsMultiKey && channelError.UsingKey != "" {
			keyHash := ChannelBreakerKeyHash(channelError.UsingKey)
			subject = fmt.Sprintf("通道「%s」（#%d）的故障密钥已被禁用", channelError.ChannelName, channelError.ChannelId)
			content = fmt.Sprintf("通道「%s」（#%d）的故障密钥已被禁用，密钥哈希：%s，原因：%s", channelError.ChannelName, channelError.ChannelId, keyHash, reason)
		}
		if disableAutoRecovery {
			content += "；已关闭自动探测恢复，请处理后手动启用"
		}
		NotifyRootUser(formatNotifyType(channelError.ChannelId, common.ChannelStatusAutoDisabled), subject, content)
		notifyChannelDisabledBark(channelError, reason, disableAutoRecovery)
	}
}

// notifyChannelDisabledBark 渠道被自动禁用时发送系统级 Bark 严重告警（独立于 NotifyRootUser，
// 用于"渠道没钱被禁用"这类需要立即感知的场景）。受 BarkAlertEnabled 与 ChannelDisableAlertEnabled
// 控制，并按 ChannelDisableAlertCooldownSecond 做每渠道去重。仅在禁用状态或终态标记真正变更时调用。
func notifyChannelDisabledBark(channelError types.ChannelError, reason string, disableAutoRecovery bool) {
	monitorSetting := operation_setting.GetMonitorSetting()
	if !monitorSetting.BarkAlertEnabled || !monitorSetting.ChannelDisableAlertEnabled {
		return
	}
	if !allowChannelDisableNotify(channelError.ChannelId, monitorSetting.ChannelDisableAlertCooldownSecond) {
		return
	}
	subject := fmt.Sprintf("通道「%s」（#%d）已被禁用", channelError.ChannelName, channelError.ChannelId)
	body := buildChannelDisabledBarkBody(channelError, reason, disableAutoRecovery)
	if err := SendSystemBarkNotify(subject, body, "new-api 渠道禁用告警", "critical", monitorSetting.ChannelDisableAlertSound); err != nil {
		common.SysError(fmt.Sprintf("failed to send channel disabled bark notify for channel %d: %s", channelError.ChannelId, err.Error()))
	}
}

// allowChannelDisableNotify 判断该渠道是否已过冷却窗口，过则记录本次时间并放行。
func allowChannelDisableNotify(channelId int, cooldownSecond int) bool {
	if cooldownSecond <= 0 {
		return true
	}
	if channelBreakerRedisEnabled() {
		key := fmt.Sprintf("channel_disable_notify:%d", channelId)
		allowed, err := common.RDB.SetNX(context.Background(), key, "1", time.Duration(cooldownSecond)*time.Second).Result()
		if err == nil {
			return allowed
		}
		common.SysError(fmt.Sprintf("failed to deduplicate channel disable notification for channel %d: %s", channelId, err.Error()))
	}
	now := time.Now()
	cooldown := time.Duration(cooldownSecond) * time.Second
	if last, ok := channelDisableNotifyMemory.Load(channelId); ok {
		if lastTime, ok := last.(time.Time); ok && now.Sub(lastTime) < cooldown {
			return false
		}
	}
	channelDisableNotifyMemory.Store(channelId, now)
	return true
}

func buildChannelDisabledBarkBody(channelError types.ChannelError, reason string, disableAutoRecovery bool) string {
	channelTypeName := constant.GetChannelTypeName(channelError.ChannelType)
	lines := []string{
		fmt.Sprintf("渠道：%s (#%d)", channelError.ChannelName, channelError.ChannelId),
		fmt.Sprintf("类型：%s (%d)", channelTypeName, channelError.ChannelType),
		fmt.Sprintf("原因：%s", reason),
	}
	if channelError.IsMultiKey && channelError.UsingKey != "" {
		lines = append(lines, fmt.Sprintf("密钥哈希：%s", ChannelBreakerKeyHash(channelError.UsingKey)))
	}
	if disableAutoRecovery {
		lines = append(lines, "自动恢复：已关闭（请处理后手动启用）")
	}
	return strings.Join(lines, "\n")
}

func EnableChannel(channelId int, usingKey string, channelName string) {
	success := model.UpdateChannelStatus(channelId, usingKey, common.ChannelStatusEnabled, "")
	if success {
		ClearChannelBreakerQuarantine(channelId, usingKey, usingKey != "")
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
