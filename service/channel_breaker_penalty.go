package service

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
)

// 熔断惩罚：熔断只有短期记忆（每次打开都清零重来），本文件补上长期记忆 ——
// 反复打开的渠道先告警、连续超阈值则自动下线（DisableChannelWithoutAutoRecovery，
// 定时重测不复活）。判据是打开频次，不解析错误文本。

// channelBreakerPenaltyEvent 是一次熔断打开事件的上下文快照。
// gin.Context 在请求结束后会被复用，异步评估前必须在这里落值。
type channelBreakerPenaltyEvent struct {
	ChannelError types.ChannelError
	Group        string
	Model        string
}

type channelBreakerPenaltyDecision struct {
	Alert            bool
	Offline          bool
	OfflineBlockedBy string // 达到下线频次但被拦下的原因；空 = 未触线或已放行下线
	CurrentHourOpens int64
	HourBucket       int64
}

var channelBreakerPenaltyAlertMemory sync.Map

// EvaluateChannelBreakerPenaltyAsync 在熔断打开后触发惩罚评估。
// 熔断日志异步落库，本次打开可能尚未计入；慢性渠道会在随后的打开事件补上，判定仍收敛。
func EvaluateChannelBreakerPenaltyAsync(c *gin.Context, channelError types.ChannelError) {
	if !common.IsChannelBreakerPenaltyEnabled() {
		return
	}
	ev := channelBreakerPenaltyEvent{
		ChannelError: channelError,
		Group:        channelBreakerContextGroup(c),
		Model:        channelBreakerContextModel(c),
	}
	gopool.Go(func() {
		decision, err := decideChannelBreakerPenalty(ev, time.Now())
		if err != nil {
			common.SysError(fmt.Sprintf("channel breaker penalty evaluation failed for channel %d: %s", ev.ChannelError.ChannelId, err.Error()))
		}
		executeChannelBreakerPenalty(ev, decision)
	})
}

// decideChannelBreakerPenalty 只判定不动作。小时桶用 FLOOR(unix/3600) 固定桶，
// 与阈值校准 SQL 同口径；当前桶达标即触发，不等桶结束。
// 出错时保守处理：能告警就告警，绝不带着不确定的数据下线渠道。
func decideChannelBreakerPenalty(ev channelBreakerPenaltyEvent, now time.Time) (channelBreakerPenaltyDecision, error) {
	decision := channelBreakerPenaltyDecision{}
	if !common.IsChannelBreakerPenaltyEnabled() {
		return decision, nil
	}
	channelId := ev.ChannelError.ChannelId
	if channelId <= 0 {
		return decision, nil
	}
	keyHash := ""
	if ev.ChannelError.IsMultiKey && ev.ChannelError.UsingKey != "" {
		keyHash = ChannelBreakerKeyHash(ev.ChannelError.UsingKey)
	}
	threshold := int64(common.GetChannelBreakerPenaltyAlertOpensPerHour())
	bucket := now.Unix() / 3600
	decision.HourBucket = bucket
	current, err := model.CountChannelBreakerOpens(channelId, keyHash, bucket*3600, now.Unix()+1)
	if err != nil {
		return decision, err
	}
	decision.CurrentHourOpens = current
	if current < threshold {
		return decision, nil
	}
	decision.Alert = true

	needHours := common.GetChannelBreakerPenaltyOfflineConsecutiveHours()
	for i := 1; i < needHours; i++ {
		hourStart := (bucket - int64(i)) * 3600
		n, countErr := model.CountChannelBreakerOpens(channelId, keyHash, hourStart, hourStart+3600)
		if countErr != nil {
			return decision, countErr
		}
		if n < threshold {
			return decision, nil
		}
	}

	if !common.IsChannelBreakerPenaltyOfflineEnabled() {
		decision.OfflineBlockedBy = "自动下线开关未启用"
		return decision, nil
	}
	if common.IsChannelBreakerExemptChannel(channelId) {
		decision.OfflineBlockedBy = "渠道在熔断豁免名单"
		return decision, nil
	}
	channel, err := model.GetChannelById(channelId, false)
	if err != nil {
		decision.OfflineBlockedBy = "渠道信息读取失败"
		return decision, err
	}
	if channel.Status != common.ChannelStatusEnabled {
		decision.OfflineBlockedBy = "渠道已不在启用状态"
		return decision, nil
	}
	minPool := common.GetChannelBreakerPenaltyMinPoolSize()
	if minPool > 0 {
		if ev.Group == "" || ev.Model == "" {
			decision.OfflineBlockedBy = "缺少分组/模型上下文"
			return decision, nil
		}
		poolSize, poolErr := model.CountEnabledChannelsForGroupModel(ev.Group, ev.Model)
		if poolErr != nil {
			decision.OfflineBlockedBy = "池保护查询失败"
			return decision, poolErr
		}
		if poolSize < int64(minPool) {
			decision.OfflineBlockedBy = fmt.Sprintf("分组「%s」模型「%s」启用渠道仅 %d 条（下限 %d）", ev.Group, ev.Model, poolSize, minPool)
			return decision, nil
		}
	}
	decision.Offline = true
	return decision, nil
}

func executeChannelBreakerPenalty(ev channelBreakerPenaltyEvent, decision channelBreakerPenaltyDecision) {
	if !decision.Alert {
		return
	}
	channelError := ev.ChannelError
	if decision.Offline {
		target := channelError
		if !target.IsMultiKey {
			target.UsingKey = ""
		}
		// 熔断事件只产生于 AutoBan 渠道，这里固化以免快照期间渠道配置变更
		target.AutoBan = true
		reason := fmt.Sprintf("熔断惩罚：连续 %d 小时每小时打开熔断 ≥ %d 次（本小时 %d 次），已自动下线，处理后需手动启用",
			common.GetChannelBreakerPenaltyOfflineConsecutiveHours(),
			common.GetChannelBreakerPenaltyAlertOpensPerHour(),
			decision.CurrentHourOpens)
		markChannelBreakerTargetQuarantined(target)
		// disableChannel 自带 NotifyRootUser + Bark 告警，此处不再重复发一级告警
		DisableChannelWithoutAutoRecovery(target, reason)
		return
	}
	if !allowChannelBreakerPenaltyAlert(channelError, decision.HourBucket) {
		return
	}
	subject := fmt.Sprintf("通道「%s」（#%d）熔断频次超标", channelError.ChannelName, channelError.ChannelId)
	content := fmt.Sprintf("通道「%s」（#%d）本小时已打开熔断 %d 次（告警阈值 %d 次/小时）",
		channelError.ChannelName, channelError.ChannelId, decision.CurrentHourOpens,
		common.GetChannelBreakerPenaltyAlertOpensPerHour())
	if decision.OfflineBlockedBy != "" {
		content += "；已达到自动下线条件，但" + decision.OfflineBlockedBy + "，未下线"
	}
	common.SysLog(content)
	NotifyRootUser("channel_breaker_penalty_"+strconv.Itoa(channelError.ChannelId), subject, content)
}

// allowChannelBreakerPenaltyAlert 每（渠道/Key × 小时桶）只发一次一级告警。
func allowChannelBreakerPenaltyAlert(channelError types.ChannelError, bucket int64) bool {
	dedupKey := strconv.Itoa(channelError.ChannelId)
	if channelError.IsMultiKey && channelError.UsingKey != "" {
		dedupKey += ":" + ChannelBreakerKeyHash(channelError.UsingKey)
	}
	if channelBreakerRedisEnabled() {
		redisKey := "channel_breaker_penalty_alert:" + dedupKey + ":" + strconv.FormatInt(bucket, 10)
		allowed, err := common.RDB.SetNX(context.Background(), redisKey, "1", 2*time.Hour).Result()
		if err == nil {
			return allowed
		}
		common.SysError(fmt.Sprintf("failed to deduplicate channel breaker penalty alert for channel %d: %s", channelError.ChannelId, err.Error()))
	}
	if last, ok := channelBreakerPenaltyAlertMemory.Load(dedupKey); ok {
		if lastBucket, ok := last.(int64); ok && lastBucket == bucket {
			return false
		}
	}
	channelBreakerPenaltyAlertMemory.Store(dedupKey, bucket)
	return true
}
