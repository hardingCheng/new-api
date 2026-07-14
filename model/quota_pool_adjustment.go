package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	QuotaPoolAdjustmentStatusPending   = "pending"
	QuotaPoolAdjustmentStatusSucceeded = "succeeded"
)

type QuotaPoolAdjustment struct {
	ID           int64  `json:"id" gorm:"primary_key"`
	OperationKey string `json:"operation_key" gorm:"type:varchar(191);uniqueIndex"`
	RedisKey     string `json:"redis_key" gorm:"type:varchar(512)"`
	Delta        int64  `json:"delta" gorm:"bigint"`
	Status       string `json:"status" gorm:"type:varchar(32);index"`
	Attempts     int    `json:"attempts"`
	LastError    string `json:"last_error" gorm:"type:text"`
	NextRetryAt  int64  `json:"next_retry_at" gorm:"bigint;index"`
	CompletedAt  int64  `json:"completed_at" gorm:"bigint;index"`
	CreatedAt    int64  `json:"created_at" gorm:"bigint;index"`
	UpdatedAt    int64  `json:"updated_at" gorm:"bigint;index"`
}

func EnsureQuotaPoolAdjustment(operationKey string, redisKey string, delta int64) (*QuotaPoolAdjustment, error) {
	operationKey = strings.TrimSpace(operationKey)
	redisKey = strings.TrimSpace(redisKey)
	if operationKey == "" || redisKey == "" || delta == 0 {
		return nil, errors.New("invalid quota pool adjustment")
	}
	var existing QuotaPoolAdjustment
	if err := DB.Where("operation_key = ?", operationKey).First(&existing).Error; err == nil {
		if existing.RedisKey != redisKey || existing.Delta != delta {
			return nil, fmt.Errorf("quota pool adjustment key %s was reused with different values", operationKey)
		}
		return &existing, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	now := common.GetTimestamp()
	adjustment := &QuotaPoolAdjustment{
		OperationKey: operationKey,
		RedisKey:     redisKey,
		Delta:        delta,
		Status:       QuotaPoolAdjustmentStatusPending,
		NextRetryAt:  now,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := DB.Create(adjustment).Error; err != nil {
		if findErr := DB.Where("operation_key = ?", operationKey).First(&existing).Error; findErr == nil {
			if existing.RedisKey != redisKey || existing.Delta != delta {
				return nil, fmt.Errorf("quota pool adjustment key %s was reused with different values", operationKey)
			}
			return &existing, nil
		}
		return nil, err
	}
	return adjustment, nil
}

func MarkQuotaPoolAdjustmentSucceeded(id int64) error {
	now := common.GetTimestamp()
	return DB.Model(&QuotaPoolAdjustment{}).
		Where("id = ? AND status = ?", id, QuotaPoolAdjustmentStatusPending).
		Updates(map[string]interface{}{
			"status":        QuotaPoolAdjustmentStatusSucceeded,
			"attempts":      gorm.Expr("attempts + 1"),
			"last_error":    "",
			"next_retry_at": int64(0),
			"completed_at":  now,
			"updated_at":    now,
		}).Error
}

func MarkQuotaPoolAdjustmentFailed(id int64, applyErr error) {
	if id <= 0 || applyErr == nil {
		return
	}
	var adjustment QuotaPoolAdjustment
	if err := DB.Select("attempts").Where("id = ?", id).First(&adjustment).Error; err != nil {
		return
	}
	attempts := adjustment.Attempts + 1
	_ = DB.Model(&QuotaPoolAdjustment{}).Where("id = ?", id).Updates(map[string]interface{}{
		"attempts":      attempts,
		"last_error":    applyErr.Error(),
		"next_retry_at": common.GetTimestamp() + billingRetryDelaySeconds(attempts),
		"updated_at":    common.GetTimestamp(),
	}).Error
}

func FindPendingQuotaPoolAdjustments(limit int) ([]*QuotaPoolAdjustment, error) {
	if limit <= 0 {
		limit = 100
	}
	var adjustments []*QuotaPoolAdjustment
	err := DB.Where("status = ? AND next_retry_at <= ?", QuotaPoolAdjustmentStatusPending, common.GetTimestamp()).
		Order("next_retry_at, id").Limit(limit).Find(&adjustments).Error
	return adjustments, err
}

func HasPendingQuotaPoolAdjustments() bool {
	var count int64
	if err := DB.Model(&QuotaPoolAdjustment{}).Where("status = ?", QuotaPoolAdjustmentStatusPending).
		Limit(1).Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}

func HasDueQuotaPoolAdjustments(now int64) bool {
	var count int64
	if err := DB.Model(&QuotaPoolAdjustment{}).
		Where("status = ? AND next_retry_at <= ?", QuotaPoolAdjustmentStatusPending, now).
		Limit(1).Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}

func CleanupCompletedQuotaPoolAdjustments(olderThan int64, limit int) (int64, error) {
	if olderThan <= 0 || limit <= 0 {
		return 0, nil
	}
	var ids []int64
	if err := DB.Model(&QuotaPoolAdjustment{}).
		Where("status = ? AND completed_at > 0 AND completed_at < ?", QuotaPoolAdjustmentStatusSucceeded, olderThan).
		Order("id").Limit(limit).Pluck("id", &ids).Error; err != nil || len(ids) == 0 {
		return 0, err
	}
	result := DB.Where("id IN ?", ids).Delete(&QuotaPoolAdjustment{})
	return result.RowsAffected, result.Error
}
