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
//
// 判定与执行分两段：decideChannelBreakerPenalty 只读判定频次与渠道状态；
// 池保护是终审，放在 runChannelBreakerPenaltyOffline 的互斥临界区里，
// 防止同池多条渠道并发通过初审后一起下线把池摘穿。

type channelBreakerPenaltyDecision struct {
	Alert            bool
	Offline          bool   // 频次触线且初审通过；终审（池保护）在执行期
	OfflineBlockedBy string // 达到下线频次但被拦下的原因；空 = 未触线或初审放行
	CurrentHourOpens int64
	HourBucket       int64
}

var (
	channelBreakerPenaltyAlertMemory sync.Map
	channelBreakerPenaltyOfflineMu   sync.Mutex
)

// EvaluateChannelBreakerPenaltyAsync 在熔断打开后触发惩罚评估。
func EvaluateChannelBreakerPenaltyAsync(c *gin.Context, channelError types.ChannelError) {
	if !common.IsChannelBreakerPenaltyEnabled() {
		return
	}
	gopool.Go(func() {
		// 熔断日志经 gopool 异步落库，等一拍再计数，把本次打开也算进去
		time.Sleep(time.Second)
		decision, err := decideChannelBreakerPenalty(channelError, time.Now())
		if err != nil {
			common.SysError(fmt.Sprintf("channel breaker penalty evaluation failed for channel %d: %s", channelError.ChannelId, err.Error()))
		}
		executeChannelBreakerPenalty(channelError, decision)
	})
}

// decideChannelBreakerPenalty 只判定不动作。小时桶用 FLOOR(unix/3600) 固定桶，
// 与阈值校准 SQL 同口径；当前桶达标即触发，不等桶结束。
// 出错时保守处理：能告警就告警，绝不带着不确定的数据下线渠道。
func decideChannelBreakerPenalty(channelError types.ChannelError, now time.Time) (channelBreakerPenaltyDecision, error) {
	decision := channelBreakerPenaltyDecision{}
	if !common.IsChannelBreakerPenaltyEnabled() {
		return decision, nil
	}
	channelId := channelError.ChannelId
	if channelId <= 0 {
		return decision, nil
	}
	keyHash := ""
	if channelError.IsMultiKey && channelError.UsingKey != "" {
		keyHash = ChannelBreakerKeyHash(channelError.UsingKey)
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
	decision.Offline = true
	return decision, nil
}

func executeChannelBreakerPenalty(channelError types.ChannelError, decision channelBreakerPenaltyDecision) {
	if !decision.Alert {
		return
	}
	if decision.Offline {
		blockReason := runChannelBreakerPenaltyOffline(channelError, decision)
		if blockReason == "" {
			// disableChannel 自带 NotifyRootUser + Bark 告警，不再重复发一级告警
			return
		}
		decision.OfflineBlockedBy = blockReason
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

// runChannelBreakerPenaltyOffline 终审并执行下线。互斥串行化：并发的下线评估
// 逐个通过池保护，前一个下线产生的池变化对后一个可见（UpdateChannelStatus 同步
// 更新 abilities），单实例内不会把池摘穿；跨实例的数据库层竞态见计划文档。
// 返回空串 = 已执行下线；非空 = 拦截原因。
func runChannelBreakerPenaltyOffline(channelError types.ChannelError, decision channelBreakerPenaltyDecision) string {
	channelBreakerPenaltyOfflineMu.Lock()
	defer channelBreakerPenaltyOfflineMu.Unlock()

	blockReason, err := channelBreakerPenaltyPoolBlockReason(channelError.ChannelId, common.GetChannelBreakerPenaltyMinPoolSize())
	if err != nil {
		common.SysError(fmt.Sprintf("channel breaker penalty pool audit failed for channel %d: %s", channelError.ChannelId, err.Error()))
		return "池保护查询失败"
	}
	if blockReason != "" {
		return blockReason
	}

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
	DisableChannelWithoutAutoRecovery(target, reason)
	return ""
}

// channelBreakerPenaltyPoolBlockReason 下线前审该渠道承载的全部启用 (分组, 模型) 池：
// 摘除后任一池剩余低于下限即拦截 —— 整渠道下线影响的是它所有的池，不只是触发
// 请求的那一个。多 Key 渠道同按整渠道口径审（最后一把 Key 被禁时渠道整体停用，
// 宁可保守）。返回空串 = 放行。
func channelBreakerPenaltyPoolBlockReason(channelId int, minPool int) (string, error) {
	if minPool <= 0 {
		return "", nil
	}
	abilities, err := model.GetEnabledAbilityGroupModels(channelId)
	if err != nil {
		return "", err
	}
	for _, ability := range abilities {
		poolSize, countErr := model.CountEnabledChannelsForGroupModel(ability.Group, ability.Model)
		if countErr != nil {
			return "", countErr
		}
		if poolSize-1 < int64(minPool) {
			return fmt.Sprintf("下线后分组「%s」模型「%s」仅剩 %d 条启用渠道（下限 %d）",
				ability.Group, ability.Model, poolSize-1, minPool), nil
		}
	}
	return "", nil
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
