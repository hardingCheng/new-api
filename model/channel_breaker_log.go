package model

import (
	"time"

	"github.com/bytedance/gopkg/util/gopool"
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
