package model

import (
	"fmt"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"gorm.io/gorm"
)

// customChannelTypeMigrationKey 标记自研渠道类型编号已迁移。
// 迁移只能跑一次：迁移之后 61/62/63 属于官方类型（Task Plugin / vLLM / SGLang），
// 再跑会把官方渠道误改成自研类型。
const customChannelTypeMigrationKey = "CustomChannelTypeMigrationV1"

// legacyCustomChannelTypes 是自研类型在合入官方 rc.40 之前使用的编号 → 现编号。
var legacyCustomChannelTypes = map[int]int{
	61: constant.ChannelTypeDMC,
	62: constant.ChannelTypeTokenMart,
	63: constant.ChannelTypeTencentMPS,
}

// migrateCustomChannelTypes 把老库里 DMC/TokenMart/腾讯 MPS 的渠道类型与任务平台号
// 从 61/62/63 挪到现编号。渠道与任务在同一事务里改完再写标记。
// 全新库（没有渠道）也会写标记，之后建的 61/62/63 渠道永远不会被改动。
func migrateCustomChannelTypes(db *gorm.DB) error {
	var marker Option
	err := db.Where(commonKeyCol+" = ?", customChannelTypeMigrationKey).Limit(1).Find(&marker).Error
	if err != nil {
		return fmt.Errorf("read channel type migration marker: %w", err)
	}
	if marker.Value == "true" {
		return nil
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for oldType, newType := range legacyCustomChannelTypes {
			result := tx.Model(&Channel{}).Where("type = ?", oldType).Update("type", newType)
			if result.Error != nil {
				return fmt.Errorf("migrate channel type %d -> %d: %w", oldType, newType, result.Error)
			}
			if result.RowsAffected > 0 {
				common.SysLog(fmt.Sprintf("migrated %d channels from type %d to %d", result.RowsAffected, oldType, newType))
			}
			taskResult := tx.Model(&Task{}).Where("platform = ?", strconv.Itoa(oldType)).Update("platform", strconv.Itoa(newType))
			if taskResult.Error != nil {
				return fmt.Errorf("migrate task platform %d -> %d: %w", oldType, newType, taskResult.Error)
			}
			if taskResult.RowsAffected > 0 {
				common.SysLog(fmt.Sprintf("migrated %d tasks from platform %d to %d", taskResult.RowsAffected, oldType, newType))
			}
		}
		return tx.Save(&Option{Key: customChannelTypeMigrationKey, Value: "true"}).Error
	})
}
