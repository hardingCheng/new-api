package model

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	BillingAdjustmentStatusReserved      = "reserved"
	BillingAdjustmentStatusPending       = "pending"
	BillingAdjustmentStatusSucceeded     = "succeeded"
	BillingAdjustmentFundingWallet       = "wallet"
	BillingAdjustmentFundingSubscription = "subscription"
	BillingReservationOutcomeSettlement  = "settlement"
	BillingReservationOutcomeRefund      = "refund"
)

const billingReservationRecoveryDelay = 6 * time.Hour

var billingSQLiteWriteMu sync.Mutex

func runBillingTransaction(fn func(*gorm.DB) error) error {
	if !common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		return DB.Transaction(fn)
	}
	billingSQLiteWriteMu.Lock()
	defer billingSQLiteWriteMu.Unlock()
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		err = DB.Transaction(fn)
		if err == nil || (!strings.Contains(err.Error(), "database is locked") && !strings.Contains(err.Error(), "SQLITE_BUSY")) {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * 5 * time.Millisecond)
	}
	return err
}

func lockBillingAdjustmentWrite() func() {
	if !common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		return func() {}
	}
	billingSQLiteWriteMu.Lock()
	return billingSQLiteWriteMu.Unlock
}

type BillingAdjustment struct {
	ID                              int64  `json:"id" gorm:"primary_key"`
	AdjustmentKey                   string `json:"adjustment_key" gorm:"type:varchar(191);uniqueIndex"`
	UserID                          int    `json:"user_id" gorm:"index"`
	TokenID                         int    `json:"token_id" gorm:"index"`
	SubscriptionID                  int    `json:"subscription_id" gorm:"index"`
	SubscriptionPreConsumeRequestID string `json:"subscription_pre_consume_request_id" gorm:"type:varchar(64);index"`
	SubscriptionPreConsumed         int    `json:"subscription_pre_consumed"`
	SubscriptionExtraReserved       int    `json:"subscription_extra_reserved"`
	FundingSource                   string `json:"funding_source" gorm:"type:varchar(32)"`
	FundingDelta                    int    `json:"funding_delta"`
	TokenDelta                      int    `json:"token_delta"`
	ReservationOutcome              string `json:"reservation_outcome" gorm:"type:varchar(32)"`
	Status                          string `json:"status" gorm:"type:varchar(32);index"`
	Attempts                        int    `json:"attempts"`
	LastError                       string `json:"last_error" gorm:"type:text"`
	NextRetryAt                     int64  `json:"next_retry_at" gorm:"bigint;index"`
	CompletedAt                     int64  `json:"completed_at" gorm:"bigint;index"`
	CacheSynced                     bool   `json:"cache_synced" gorm:"index"`
	CreatedAt                       int64  `json:"created_at" gorm:"bigint;index"`
	UpdatedAt                       int64  `json:"updated_at" gorm:"bigint;index"`
}

type BillingAdjustmentParams struct {
	AdjustmentKey                   string
	UserID                          int
	TokenID                         int
	SubscriptionID                  int
	SubscriptionPreConsumeRequestID string
	SubscriptionPreConsumed         int
	SubscriptionExtraReserved       int
	FundingSource                   string
	FundingDelta                    int
	TokenDelta                      int
}

type BillingReservationParams struct {
	RequestID             string
	UserID                int
	TokenID               int
	TokenKey              string
	TokenUnlimited        bool
	TokenBillingEnabled   bool
	FundingSource         string
	Amount                int
	SubscriptionModelName string
}

type BillingReservationResult struct {
	Adjustment   *BillingAdjustment
	Subscription *SubscriptionPreConsumeResult
}

func GetBillingAdjustmentByKey(adjustmentKey string) (*BillingAdjustment, bool, error) {
	adjustmentKey = strings.TrimSpace(adjustmentKey)
	if adjustmentKey == "" {
		return nil, false, nil
	}
	var adjustment BillingAdjustment
	err := DB.Where("adjustment_key = ?", adjustmentKey).First(&adjustment).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &adjustment, true, nil
}

// ReserveBillingQuota atomically records the recovery action and deducts all
// pre-consumed quota. A process crash leaves a due reserved row whose default
// action is a full refund.
func ReserveBillingQuota(params BillingReservationParams) (*BillingReservationResult, error) {
	params.RequestID = strings.TrimSpace(params.RequestID)
	params.TokenKey = strings.TrimSpace(params.TokenKey)
	params.FundingSource = strings.TrimSpace(params.FundingSource)
	if params.RequestID == "" || params.UserID <= 0 || params.Amount < 0 {
		return nil, errors.New("invalid billing reservation")
	}
	if params.FundingSource != BillingAdjustmentFundingWallet && params.FundingSource != BillingAdjustmentFundingSubscription {
		return nil, fmt.Errorf("invalid billing reservation funding source %q", params.FundingSource)
	}
	if params.FundingSource == BillingAdjustmentFundingSubscription && params.Amount <= 0 {
		return nil, errors.New("subscription billing reservation amount must be positive")
	}
	if params.TokenBillingEnabled && params.TokenID <= 0 {
		return nil, errors.New("billing reservation token is required")
	}

	adjustmentKey := "relay-billing:" + params.RequestID
	reservationNow := GetDBTimestamp()
	result := &BillingReservationResult{}
	err := runBillingTransaction(func(tx *gorm.DB) error {
		var existing BillingAdjustment
		query := tx.Where("adjustment_key = ?", adjustmentKey).Limit(1).Find(&existing)
		if query.Error != nil {
			return query.Error
		}
		if query.RowsAffected > 0 {
			if !billingReservationMatches(existing, params) {
				return fmt.Errorf("billing reservation key %s was reused with different values", adjustmentKey)
			}
			result.Adjustment = &existing
			if existing.SubscriptionID > 0 {
				result.Subscription = subscriptionResultFromAdjustment(tx, existing)
			}
			return nil
		}

		var subscription *SubscriptionPreConsumeResult
		switch params.FundingSource {
		case BillingAdjustmentFundingSubscription:
			var err error
			subscription, err = preConsumeUserSubscriptionTx(tx, params.RequestID, params.UserID, params.SubscriptionModelName, 0, int64(params.Amount), reservationNow)
			if err != nil {
				return err
			}
		case BillingAdjustmentFundingWallet:
			if params.Amount > 0 {
				var user User
				if err := lockForUpdate(tx).Where("id = ?", params.UserID).First(&user).Error; err != nil {
					return err
				}
				if user.Quota < params.Amount {
					return fmt.Errorf("user quota is not enough, user remain quota: %d, need quota: %d", user.Quota, params.Amount)
				}
				if err := tx.Model(&User{}).Where("id = ?", params.UserID).
					Update("quota", gorm.Expr("quota - ?", params.Amount)).Error; err != nil {
					return err
				}
			}
		}

		tokenAmount := 0
		if params.TokenBillingEnabled {
			var token Token
			if err := lockForUpdate(tx).Where("id = ?", params.TokenID).First(&token).Error; err != nil {
				return err
			}
			if token.UserId != params.UserID || (params.TokenKey != "" && token.Key != params.TokenKey) {
				return errors.New("billing reservation token does not match request")
			}
			if !params.TokenUnlimited && token.RemainQuota < params.Amount {
				return fmt.Errorf("token quota is not enough, token remain quota: %d, need quota: %d", token.RemainQuota, params.Amount)
			}
			if params.Amount > 0 {
				if err := tx.Model(&Token{}).Where("id = ?", token.Id).Updates(map[string]interface{}{
					"remain_quota":  gorm.Expr("remain_quota - ?", params.Amount),
					"used_quota":    gorm.Expr("used_quota + ?", params.Amount),
					"accessed_time": common.GetTimestamp(),
				}).Error; err != nil {
					return err
				}
				tokenAmount = params.Amount
			}
		}

		now := common.GetTimestamp()
		adjustment := &BillingAdjustment{
			AdjustmentKey: adjustmentKey,
			UserID:        params.UserID,
			TokenID:       params.TokenID,
			FundingSource: params.FundingSource,
			FundingDelta:  -params.Amount,
			TokenDelta:    -tokenAmount,
			Status:        BillingAdjustmentStatusReserved,
			NextRetryAt:   now + int64(billingReservationRecoveryDelay/time.Second),
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if subscription != nil {
			adjustment.SubscriptionID = subscription.UserSubscriptionId
			adjustment.SubscriptionPreConsumeRequestID = params.RequestID
			adjustment.SubscriptionPreConsumed = params.Amount
		}
		if err := tx.Create(adjustment).Error; err != nil {
			return err
		}
		result.Adjustment = adjustment
		result.Subscription = subscription
		return nil
	})
	if err != nil {
		// A concurrent transaction with the same request ID may have committed
		// after our initial lookup. Its failed transaction did not retain deductions.
		var existing BillingAdjustment
		if findErr := DB.Where("adjustment_key = ?", adjustmentKey).First(&existing).Error; findErr == nil && billingReservationMatches(existing, params) {
			result.Adjustment = &existing
			if existing.SubscriptionID > 0 {
				result.Subscription = subscriptionResultFromAdjustment(DB, existing)
			}
			err = nil
		}
	}
	if err != nil {
		return nil, err
	}
	refreshBillingReservationCaches(params.UserID, params.TokenKey, params.FundingSource)
	return result, nil
}

func billingReservationMatches(adjustment BillingAdjustment, params BillingReservationParams) bool {
	tokenDelta := 0
	if params.TokenBillingEnabled {
		tokenDelta = -params.Amount
	}
	return adjustment.UserID == params.UserID &&
		adjustment.TokenID == params.TokenID &&
		adjustment.FundingSource == params.FundingSource &&
		adjustment.FundingDelta == -params.Amount &&
		adjustment.TokenDelta == tokenDelta
}

func subscriptionResultFromAdjustment(tx *gorm.DB, adjustment BillingAdjustment) *SubscriptionPreConsumeResult {
	result := &SubscriptionPreConsumeResult{
		UserSubscriptionId: adjustment.SubscriptionID,
		PreConsumed:        int64(adjustment.SubscriptionPreConsumed),
	}
	var subscription UserSubscription
	if err := tx.Where("id = ?", adjustment.SubscriptionID).First(&subscription).Error; err == nil {
		result.AmountTotal = subscription.AmountTotal
		result.AmountUsedBefore = subscription.AmountUsed
		result.AmountUsedAfter = subscription.AmountUsed
	}
	return result
}

func ExtendBillingReservation(adjustmentID int64, amount int, tokenBillingEnabled bool, tokenUnlimited bool, tokenKey string) (*BillingAdjustment, error) {
	if adjustmentID <= 0 || amount <= 0 {
		return nil, errors.New("invalid billing reservation extension")
	}
	var updated BillingAdjustment
	err := runBillingTransaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("id = ?", adjustmentID).First(&updated).Error; err != nil {
			return err
		}
		if updated.Status != BillingAdjustmentStatusReserved {
			return errors.New("billing reservation is already finalized")
		}
		switch updated.FundingSource {
		case BillingAdjustmentFundingSubscription:
			if err := applySubscriptionBillingDeltaTx(tx, updated.SubscriptionID, int64(amount)); err != nil {
				return err
			}
			updated.SubscriptionExtraReserved += amount
		case BillingAdjustmentFundingWallet:
			var user User
			if err := lockForUpdate(tx).Where("id = ?", updated.UserID).First(&user).Error; err != nil {
				return err
			}
			if user.Quota < amount {
				return fmt.Errorf("user quota is not enough, user remain quota: %d, need quota: %d", user.Quota, amount)
			}
			if err := tx.Model(&User{}).Where("id = ?", updated.UserID).
				Update("quota", gorm.Expr("quota - ?", amount)).Error; err != nil {
				return err
			}
		default:
			return errors.New("invalid billing reservation funding source")
		}
		if tokenBillingEnabled {
			var token Token
			if err := lockForUpdate(tx).Where("id = ?", updated.TokenID).First(&token).Error; err != nil {
				return err
			}
			if !tokenUnlimited && token.RemainQuota < amount {
				return fmt.Errorf("token quota is not enough, token remain quota: %d, need quota: %d", token.RemainQuota, amount)
			}
			if err := tx.Model(&Token{}).Where("id = ?", token.Id).Updates(map[string]interface{}{
				"remain_quota":  gorm.Expr("remain_quota - ?", amount),
				"used_quota":    gorm.Expr("used_quota + ?", amount),
				"accessed_time": common.GetTimestamp(),
			}).Error; err != nil {
				return err
			}
		}
		updated.FundingDelta -= amount
		if tokenBillingEnabled {
			updated.TokenDelta -= amount
		}
		updated.NextRetryAt = common.GetTimestamp() + int64(billingReservationRecoveryDelay/time.Second)
		updated.UpdatedAt = common.GetTimestamp()
		return tx.Model(&BillingAdjustment{}).Where("id = ?", updated.ID).Updates(map[string]interface{}{
			"funding_delta":               updated.FundingDelta,
			"token_delta":                 updated.TokenDelta,
			"subscription_extra_reserved": updated.SubscriptionExtraReserved,
			"next_retry_at":               updated.NextRetryAt,
			"updated_at":                  updated.UpdatedAt,
		}).Error
	})
	if err != nil {
		return nil, err
	}
	refreshBillingReservationCaches(updated.UserID, tokenKey, updated.FundingSource)
	return &updated, nil
}

// FinalizeBillingReservation turns the pre-existing recovery row into either
// the actual settlement delta or an immediate full refund. The first durable
// finalization wins; retries only apply that recorded outcome.
func FinalizeBillingReservation(adjustmentID int64, actualQuota int, tokenBillingEnabled bool, refund bool) (*BillingAdjustment, error) {
	if adjustmentID <= 0 || actualQuota < 0 {
		return nil, errors.New("invalid billing reservation finalization")
	}
	var adjustment BillingAdjustment
	err := runBillingTransaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("id = ?", adjustmentID).First(&adjustment).Error; err != nil {
			return err
		}
		if adjustment.Status != BillingAdjustmentStatusReserved {
			return nil
		}
		if !refund {
			reservedFunding := -adjustment.FundingDelta
			reservedToken := -adjustment.TokenDelta
			adjustment.FundingDelta = actualQuota - reservedFunding
			if tokenBillingEnabled {
				adjustment.TokenDelta = actualQuota - reservedToken
			} else {
				adjustment.TokenDelta = 0
			}
			adjustment.SubscriptionPreConsumeRequestID = ""
			adjustment.SubscriptionPreConsumed = 0
			adjustment.SubscriptionExtraReserved = 0
			adjustment.ReservationOutcome = BillingReservationOutcomeSettlement
		} else {
			adjustment.ReservationOutcome = BillingReservationOutcomeRefund
		}
		adjustment.Status = BillingAdjustmentStatusPending
		adjustment.NextRetryAt = common.GetTimestamp()
		adjustment.UpdatedAt = common.GetTimestamp()
		return tx.Model(&BillingAdjustment{}).Where("id = ? AND status = ?", adjustment.ID, BillingAdjustmentStatusReserved).Updates(map[string]interface{}{
			"funding_delta":                       adjustment.FundingDelta,
			"token_delta":                         adjustment.TokenDelta,
			"subscription_pre_consume_request_id": adjustment.SubscriptionPreConsumeRequestID,
			"subscription_pre_consumed":           adjustment.SubscriptionPreConsumed,
			"subscription_extra_reserved":         adjustment.SubscriptionExtraReserved,
			"reservation_outcome":                 adjustment.ReservationOutcome,
			"status":                              adjustment.Status,
			"next_retry_at":                       adjustment.NextRetryAt,
			"updated_at":                          adjustment.UpdatedAt,
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return &adjustment, nil
}

func refreshBillingReservationCaches(userID int, tokenKey string, fundingSource string) {
	if !common.RedisEnabled {
		return
	}
	if fundingSource == BillingAdjustmentFundingWallet && userID > 0 {
		if err := invalidateUserCache(userID); err != nil {
			common.SysLog("failed to invalidate billing reservation user cache: " + err.Error())
		}
	}
	if strings.TrimSpace(tokenKey) != "" {
		if err := cacheDeleteToken(tokenKey); err != nil {
			common.SysLog("failed to invalidate billing reservation token cache: " + err.Error())
		}
	}
}

func EnsureBillingAdjustment(params BillingAdjustmentParams) (*BillingAdjustment, error) {
	unlock := lockBillingAdjustmentWrite()
	defer unlock()
	params.AdjustmentKey = strings.TrimSpace(params.AdjustmentKey)
	if params.AdjustmentKey == "" {
		return nil, errors.New("billing adjustment key is required")
	}
	params.FundingSource = strings.TrimSpace(params.FundingSource)
	if params.SubscriptionPreConsumed < 0 || params.SubscriptionExtraReserved < 0 {
		return nil, errors.New("subscription refund amounts cannot be negative")
	}
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
		AdjustmentKey:                   params.AdjustmentKey,
		UserID:                          params.UserID,
		TokenID:                         params.TokenID,
		SubscriptionID:                  params.SubscriptionID,
		SubscriptionPreConsumeRequestID: strings.TrimSpace(params.SubscriptionPreConsumeRequestID),
		SubscriptionPreConsumed:         params.SubscriptionPreConsumed,
		SubscriptionExtraReserved:       params.SubscriptionExtraReserved,
		FundingSource:                   params.FundingSource,
		FundingDelta:                    params.FundingDelta,
		TokenDelta:                      params.TokenDelta,
		Status:                          BillingAdjustmentStatusPending,
		NextRetryAt:                     now,
		CreatedAt:                       now,
		UpdatedAt:                       now,
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
		adjustment.SubscriptionPreConsumeRequestID == strings.TrimSpace(params.SubscriptionPreConsumeRequestID) &&
		adjustment.SubscriptionPreConsumed == params.SubscriptionPreConsumed &&
		adjustment.SubscriptionExtraReserved == params.SubscriptionExtraReserved &&
		adjustment.FundingSource == params.FundingSource &&
		adjustment.FundingDelta == params.FundingDelta &&
		adjustment.TokenDelta == params.TokenDelta
}

func ApplyBillingAdjustment(adjustmentID int64) error {
	if adjustmentID <= 0 {
		return errors.New("invalid billing adjustment id")
	}
	var applied *BillingAdjustment
	err := runBillingTransaction(func(tx *gorm.DB) error {
		var adjustment BillingAdjustment
		if err := lockForUpdate(tx).Where("id = ?", adjustmentID).First(&adjustment).Error; err != nil {
			return err
		}
		if adjustment.Status == BillingAdjustmentStatusSucceeded {
			if adjustment.CacheSynced {
				return nil
			}
			applied = &adjustment
			return nil
		}

		if adjustment.FundingDelta != 0 {
			switch adjustment.FundingSource {
			case BillingAdjustmentFundingSubscription:
				if adjustment.SubscriptionPreConsumeRequestID != "" {
					if err := applySubscriptionRefundAdjustmentTx(tx, &adjustment); err != nil {
						return err
					}
				} else if err := applySubscriptionBillingDeltaTx(tx, adjustment.SubscriptionID, int64(adjustment.FundingDelta)); err != nil {
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
			"status":        BillingAdjustmentStatusSucceeded,
			"attempts":      gorm.Expr("attempts + 1"),
			"last_error":    "",
			"next_retry_at": int64(0),
			"completed_at":  now,
			"cache_synced":  !common.RedisEnabled,
			"updated_at":    now,
		}).Error; err != nil {
			return err
		}
		applied = &adjustment
		return nil
	})
	if err != nil {
		scheduleBillingAdjustmentRetry(adjustmentID, err)
		return err
	}
	if applied != nil {
		if err := refreshBillingAdjustmentCaches(*applied); err != nil {
			scheduleBillingAdjustmentRetry(adjustmentID, err)
			return err
		}
	}
	return nil
}

func applySubscriptionRefundAdjustmentTx(tx *gorm.DB, adjustment *BillingAdjustment) error {
	if adjustment == nil || adjustment.FundingDelta >= 0 {
		return errors.New("invalid subscription refund adjustment")
	}
	var record SubscriptionPreConsumeRecord
	if err := lockForUpdate(tx).Where("request_id = ?", adjustment.SubscriptionPreConsumeRequestID).First(&record).Error; err != nil {
		return err
	}
	if record.UserId != adjustment.UserID || record.UserSubscriptionId != adjustment.SubscriptionID {
		return errors.New("subscription refund adjustment does not match pre-consume record")
	}
	if adjustment.SubscriptionPreConsumed > 0 && record.PreConsumed != int64(adjustment.SubscriptionPreConsumed) {
		return errors.New("subscription refund adjustment amount does not match pre-consume record")
	}
	if record.Status == "refunded" {
		return nil
	}
	refundAmount := record.PreConsumed + int64(adjustment.SubscriptionExtraReserved)
	if refundAmount != int64(-adjustment.FundingDelta) {
		return fmt.Errorf("subscription refund amount mismatch, adjustment=%d actual=%d", -adjustment.FundingDelta, refundAmount)
	}
	if refundAmount > 0 {
		if err := applySubscriptionBillingDeltaTx(tx, adjustment.SubscriptionID, -refundAmount); err != nil {
			return err
		}
	}
	if record.Status != "refunded" {
		if err := tx.Model(&SubscriptionPreConsumeRecord{}).Where("id = ?", record.Id).Updates(map[string]interface{}{
			"status":     "refunded",
			"updated_at": common.GetTimestamp(),
		}).Error; err != nil {
			return err
		}
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

func refreshBillingAdjustmentCaches(adjustment BillingAdjustment) error {
	if !common.RedisEnabled {
		return DB.Model(&BillingAdjustment{}).Where("id = ?", adjustment.ID).Update("cache_synced", true).Error
	}
	if common.RDB == nil {
		return errors.New("Redis is enabled but unavailable")
	}
	var cacheErr error
	if adjustment.FundingSource != BillingAdjustmentFundingSubscription && adjustment.UserID > 0 && adjustment.FundingDelta != 0 {
		if err := invalidateUserCache(adjustment.UserID); err != nil {
			cacheErr = errors.Join(cacheErr, fmt.Errorf("invalidate user quota cache: %w", err))
		}
	}
	if adjustment.TokenID > 0 && adjustment.TokenDelta != 0 {
		var token Token
		err := DB.Select(commonKeyCol).Where("id = ?", adjustment.TokenID).First(&token).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = nil
		} else if err == nil {
			err = cacheDeleteToken(token.Key)
		}
		if err != nil {
			cacheErr = errors.Join(cacheErr, fmt.Errorf("invalidate token quota cache: %w", err))
		}
	}
	if cacheErr != nil {
		return cacheErr
	}
	return DB.Model(&BillingAdjustment{}).Where("id = ?", adjustment.ID).Updates(map[string]interface{}{
		"cache_synced":  true,
		"last_error":    "",
		"next_retry_at": int64(0),
		"updated_at":    common.GetTimestamp(),
	}).Error
}

func scheduleBillingAdjustmentRetry(adjustmentID int64, applyErr error) {
	if adjustmentID <= 0 || applyErr == nil {
		return
	}
	var adjustment BillingAdjustment
	if err := DB.Select("attempts").Where("id = ?", adjustmentID).First(&adjustment).Error; err != nil {
		return
	}
	attempts := adjustment.Attempts + 1
	_ = DB.Model(&BillingAdjustment{}).Where("id = ?", adjustmentID).Updates(map[string]interface{}{
		"attempts":      attempts,
		"last_error":    applyErr.Error(),
		"next_retry_at": common.GetTimestamp() + billingRetryDelaySeconds(attempts),
		"updated_at":    common.GetTimestamp(),
	}).Error
}

func billingRetryDelaySeconds(attempts int) int64 {
	if attempts < 1 {
		attempts = 1
	}
	delay := 15 * time.Second
	for i := 1; i < attempts && delay < time.Hour; i++ {
		delay *= 2
	}
	if delay > time.Hour {
		delay = time.Hour
	}
	return int64(delay / time.Second)
}

func FindPendingBillingAdjustments(limit int) ([]*BillingAdjustment, error) {
	if limit <= 0 {
		limit = 100
	}
	var adjustments []*BillingAdjustment
	now := common.GetTimestamp()
	err := DB.Where("(status = ? OR status = ? OR (status = ? AND cache_synced = ?)) AND next_retry_at <= ?",
		BillingAdjustmentStatusReserved, BillingAdjustmentStatusPending, BillingAdjustmentStatusSucceeded, false, now).
		Order("next_retry_at, id").Limit(limit).Find(&adjustments).Error
	return adjustments, err
}

func HasPendingBillingAdjustments() bool {
	var count int64
	if err := DB.Model(&BillingAdjustment{}).
		Where("status = ? OR status = ? OR (status = ? AND cache_synced = ?)", BillingAdjustmentStatusReserved, BillingAdjustmentStatusPending, BillingAdjustmentStatusSucceeded, false).
		Limit(1).Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}

func HasDueBillingAdjustments(now int64) bool {
	var count int64
	if err := DB.Model(&BillingAdjustment{}).
		Where("(status = ? OR status = ? OR (status = ? AND cache_synced = ?)) AND next_retry_at <= ?",
			BillingAdjustmentStatusReserved, BillingAdjustmentStatusPending, BillingAdjustmentStatusSucceeded, false, now).
		Limit(1).Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}

func CleanupCompletedBillingAdjustments(olderThan int64, limit int) (int64, error) {
	if olderThan <= 0 || limit <= 0 {
		return 0, nil
	}
	var ids []int64
	if err := DB.Model(&BillingAdjustment{}).
		Where("status = ? AND cache_synced = ? AND completed_at > 0 AND completed_at < ?", BillingAdjustmentStatusSucceeded, true, olderThan).
		Order("id").Limit(limit).Pluck("id", &ids).Error; err != nil || len(ids) == 0 {
		return 0, err
	}
	result := DB.Where("id IN ?", ids).Delete(&BillingAdjustment{})
	return result.RowsAffected, result.Error
}
