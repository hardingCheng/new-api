package model

import (
	"time"

	"github.com/bytedance/gopkg/util/gopool"
)

// 熔断打开事件的三类 reason。惩罚计数覆盖全部三类：慢性坏渠道的状态机长期
// 停留在 open↔half-open 循环里，打开事件多以探测失败/超时重开的形式出现，
// 只数第一类会永久漏计（立即禁用的日志行 reason 不同，天然不计入）。
const (
	ChannelBreakerLogReasonOpened             = "channel breaker opened"
	ChannelBreakerLogReasonReopenedPrefix     = "channel breaker remains open after probe"
	ChannelBreakerLogReasonProbeTimeoutPrefix = "channel breaker probe timed out"
)

// ChannelBreakerLog 记录每一次熔断器打开（OPEN）的历史，便于管理员排查。
type ChannelBreakerLog struct {
	Id           int    `json:"id"`
	CreatedAt    int64  `json:"created_at" gorm:"bigint;index"`
	ChannelId    int    `json:"channel_id" gorm:"index"`
	ChannelName  string `json:"channel_name" gorm:"->"`
	KeyHash      string `json:"key_hash" gorm:"type:varchar(64);default:''"`
	RuleId       string `json:"rule_id" gorm:"type:varchar(64);default:''"`
	RuleName     string `json:"rule_name" gorm:"default:''"`
	UsingGroup   string `json:"using_group" gorm:"column:using_group;default:''"`
	ModelName    string `json:"model_name" gorm:"default:''"`
	Failures     int    `json:"failures" gorm:"default:0"`
	CooldownSecs int    `json:"cooldown_secs" gorm:"default:0"`
	Reason       string `json:"reason" gorm:"default:''"`
}

// RecordChannelBreakerLog 异步写入一条熔断历史记录，避免阻塞熔断主流程。
func RecordChannelBreakerLog(log *ChannelBreakerLog) {
	if log == nil {
		return
	}
	if log.CreatedAt == 0 {
		log.CreatedAt = time.Now().Unix()
	}
	gopool.Go(func() {
		_ = DB.Create(log).Error
	})
}

// CountChannelBreakerOpens 统计渠道在 [from, to) 秒级时间窗内熔断打开的次数
// （三类打开事件都算，见上方常量说明）。keyHash 非空时只统计该 Key
// （多 Key 渠道按 Key 惩罚，比按渠道聚合更宽松）。
func CountChannelBreakerOpens(channelId int, keyHash string, from int64, to int64) (int64, error) {
	query := DB.Model(&ChannelBreakerLog{}).
		Where("channel_id = ? AND created_at >= ? AND created_at < ?", channelId, from, to).
		Where("(reason = ? OR reason LIKE ? OR reason LIKE ?)",
			ChannelBreakerLogReasonOpened,
			ChannelBreakerLogReasonReopenedPrefix+"%",
			ChannelBreakerLogReasonProbeTimeoutPrefix+"%")
	if keyHash != "" {
		query = query.Where("key_hash = ?", keyHash)
	}
	var count int64
	err := query.Count(&count).Error
	return count, err
}

// GetChannelBreakerLogs 分页获取熔断历史，按时间倒序，并回填渠道名称。
func GetChannelBreakerLogs(startIdx int, num int) ([]*ChannelBreakerLog, int64, error) {
	var logs []*ChannelBreakerLog
	var total int64
	if err := DB.Model(&ChannelBreakerLog{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := DB.Model(&ChannelBreakerLog{}).
		Order("id desc").
		Limit(num).
		Offset(startIdx).
		Find(&logs).Error
	if err != nil {
		return nil, 0, err
	}
	if len(logs) > 0 {
		if nameMap, mapErr := GetChannelIdNameMap(); mapErr == nil {
			for i := range logs {
				if name, ok := nameMap[logs[i].ChannelId]; ok && name != "" {
					logs[i].ChannelName = name
				}
			}
		}
	}
	return logs, total, nil
}
