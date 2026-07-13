package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
)

const (
	BillingAdjustmentStatusPending       = "pending"
	BillingAdjustmentStatusSucceeded     = "succeeded"
	BillingAdjustmentFundingWallet       = "wallet"
	BillingAdjustmentFundingSubscription = "subscription"
)

type BillingAdjustment struct {
	ID             int64  `json:"id" gorm:"primary_key"`
	AdjustmentKey  string `json:"adjustment_key" gorm:"type:varchar(191);uniqueIndex"`
	UserID         int    `json:"user_id" gorm:"index"`
	TokenID        int    `json:"token_id" gorm:"index"`
	SubscriptionID int    `json:"subscription_id" gorm:"index"`
	FundingSource  string `json:"funding_source" gorm:"type:varchar(32)"`
	FundingDelta   int    `json:"funding_delta"`
	TokenDelta     int    `json:"token_delta"`
	Status         string `json:"status" gorm:"type:varchar(32);index"`
	Attempts       int    `json:"attempts"`
	LastError      string `json:"last_error" gorm:"type:text"`
	CreatedAt      int64  `json:"created_at" gorm:"bigint;index"`
	UpdatedAt      int64  `json:"updated_at" gorm:"bigint;index"`
}

type BillingAdjustmentParams struct {
	AdjustmentKey  string
	UserID         int
	TokenID        int
	SubscriptionID int
	FundingSource  string
	FundingDelta   int
	TokenDelta     int
}

func EnsureBillingAdjustment(params BillingAdjustmentParams) (*BillingAdjustment, error) {
	params.AdjustmentKey = strings.TrimSpace(params.AdjustmentKey)
	if params.AdjustmentKey == "" {
		return nil, errors.New("billing adjustment key is required")
	}
	params.FundingSource = strings.TrimSpace(params.FundingSource)
	if params.FundingSource == "" {
		params.FundingSource = BillingAdjustmentFundingWallet
	}
	if params.FundingSource != BillingAdjustmentFundingWallet && params.FundingSource != BillingAdjustmentFundingSubscription {
		return nil, fmt.Errorf("invalid billing adjustment funding source %q", params.FundingSource)
	}
	var existing BillingAdjustment
	if err := DB.Where("adjustment_key = ?", params.AdjustmentKey).First(&existing).Error; err == nil {
		if !billingAdjustmentMatches(existing, params) {
			return nil, fmt.Errorf("billing adjustment key %s was reused with different values", params.AdjustmentKey)
		}
		return &existing, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	now := common.GetTimestamp()
	adjustment := &BillingAdjustment{
		AdjustmentKey:  params.AdjustmentKey,
		UserID:         params.UserID,
		TokenID:        params.TokenID,
		SubscriptionID: params.SubscriptionID,
		FundingSource:  params.FundingSource,
		FundingDelta:   params.FundingDelta,
		TokenDelta:     params.TokenDelta,
		Status:         BillingAdjustmentStatusPending,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := DB.Create(adjustment).Error; err != nil {
		// A concurrent request may have created the same idempotency key.
		if findErr := DB.Where("adjustment_key = ?", params.AdjustmentKey).First(&existing).Error; findErr == nil {
			if !billingAdjustmentMatches(existing, params) {
				return nil, fmt.Errorf("billing adjustment key %s was reused with different values", params.AdjustmentKey)
			}
			return &existing, nil
		}
		return nil, err
	}
	return adjustment, nil
}

func billingAdjustmentMatches(adjustment BillingAdjustment, params BillingAdjustmentParams) bool {
	return adjustment.UserID == params.UserID &&
		adjustment.TokenID == params.TokenID &&
		adjustment.SubscriptionID == params.SubscriptionID &&
		adjustment.FundingSource == params.FundingSource &&
		adjustment.FundingDelta == params.FundingDelta &&
		adjustment.TokenDelta == params.TokenDelta
}

func ApplyBillingAdjustment(adjustmentID int64) error {
	if adjustmentID <= 0 {
		return errors.New("invalid billing adjustment id")
	}
	var applied *BillingAdjustment
	err := DB.Transaction(func(tx *gorm.DB) error {
		var adjustment BillingAdjustment
		if err := lockForUpdate(tx).Where("id = ?", adjustmentID).First(&adjustment).Error; err != nil {
			return err
		}
		if adjustment.Status == BillingAdjustmentStatusSucceeded {
			return nil
		}

		if adjustment.FundingDelta != 0 {
			switch adjustment.FundingSource {
			case BillingAdjustmentFundingSubscription:
				if err := applySubscriptionBillingDeltaTx(tx, adjustment.SubscriptionID, int64(adjustment.FundingDelta)); err != nil {
					return err
				}
			default:
				if adjustment.UserID <= 0 {
					return errors.New("invalid billing adjustment user")
				}
				var user User
				if err := lockForUpdate(tx).Where("id = ?", adjustment.UserID).First(&user).Error; err != nil {
					return err
				}
				if err := tx.Model(&User{}).Where("id = ?", adjustment.UserID).
					Update("quota", gorm.Expr("quota - ?", adjustment.FundingDelta)).Error; err != nil {
					return err
				}
			}
		}

		if adjustment.TokenDelta != 0 && adjustment.TokenID > 0 {
			var token Token
			if err := lockForUpdate(tx).Where("id = ?", adjustment.TokenID).First(&token).Error; err != nil {
				return err
			}
			if err := tx.Model(&Token{}).Where("id = ?", adjustment.TokenID).Updates(map[string]interface{}{
				"remain_quota":  gorm.Expr("remain_quota - ?", adjustment.TokenDelta),
				"used_quota":    gorm.Expr("used_quota + ?", adjustment.TokenDelta),
				"accessed_time": common.GetTimestamp(),
			}).Error; err != nil {
				return err
			}
		}

		now := common.GetTimestamp()
		if err := tx.Model(&BillingAdjustment{}).Where("id = ?", adjustment.ID).Updates(map[string]interface{}{
			"status":     BillingAdjustmentStatusSucceeded,
			"attempts":   gorm.Expr("attempts + 1"),
			"last_error": "",
			"updated_at": now,
		}).Error; err != nil {
			return err
		}
		applied = &adjustment
		return nil
	})
	if err != nil {
		_ = DB.Model(&BillingAdjustment{}).Where("id = ?", adjustmentID).Updates(map[string]interface{}{
			"attempts":   gorm.Expr("attempts + 1"),
			"last_error": err.Error(),
			"updated_at": common.GetTimestamp(),
		}).Error
		return err
	}
	if applied != nil {
		refreshBillingAdjustmentCaches(*applied)
	}
	return nil
}

func applySubscriptionBillingDeltaTx(tx *gorm.DB, subscriptionID int, delta int64) error {
	if subscriptionID <= 0 {
		return errors.New("invalid billing adjustment subscription")
	}
	var subscription UserSubscription
	if err := lockForUpdate(tx).Where("id = ?", subscriptionID).First(&subscription).Error; err != nil {
		return err
	}
	newUsed := subscription.AmountUsed + delta
	if newUsed < 0 {
		newUsed = 0
	}
	if subscription.AmountTotal > 0 && newUsed > subscription.AmountTotal {
		return fmt.Errorf("subscription used exceeds total, used=%d total=%d", newUsed, subscription.AmountTotal)
	}
	return tx.Model(&UserSubscription{}).Where("id = ?", subscriptionID).Update("amount_used", newUsed).Error
}

func refreshBillingAdjustmentCaches(adjustment BillingAdjustment) {
	if adjustment.FundingSource != BillingAdjustmentFundingSubscription && adjustment.UserID > 0 && adjustment.FundingDelta != 0 {
		gopool.Go(func() {
			var err error
			if adjustment.FundingDelta > 0 {
				err = cacheDecrUserQuota(adjustment.UserID, int64(adjustment.FundingDelta))
			} else {
				err = cacheIncrUserQuota(adjustment.UserID, int64(-adjustment.FundingDelta))
			}
			if err != nil {
				common.SysLog("failed to update user cache after billing adjustment: " + err.Error())
			}
		})
	}
	if common.RedisEnabled && adjustment.TokenID > 0 && adjustment.TokenDelta != 0 {
		gopool.Go(func() {
			token, err := GetTokenById(adjustment.TokenID)
			if err != nil {
				common.SysLog("failed to load token after billing adjustment: " + err.Error())
				return
			}
			if adjustment.TokenDelta > 0 {
				err = cacheDecrTokenQuota(token.Key, int64(adjustment.TokenDelta))
			} else {
				err = cacheIncrTokenQuota(token.Key, int64(-adjustment.TokenDelta))
			}
			if err != nil {
				common.SysLog("failed to update token cache after billing adjustment: " + err.Error())
			}
		})
	}
}

func FindPendingBillingAdjustments(limit int) ([]*BillingAdjustment, error) {
	if limit <= 0 {
		limit = 100
	}
	var adjustments []*BillingAdjustment
	err := DB.Where("status = ?", BillingAdjustmentStatusPending).Order("id").Limit(limit).Find(&adjustments).Error
	return adjustments, err
}

func HasPendingBillingAdjustments() bool {
	var count int64
	if err := DB.Model(&BillingAdjustment{}).Where("status = ?", BillingAdjustmentStatusPending).Limit(1).Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}
