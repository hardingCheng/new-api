package service

import (
	"context"

	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
)

type BillingReconciliationSummary struct {
	Pending       int `json:"pending"`
	Succeeded     int `json:"succeeded"`
	Failed        int `json:"failed"`
	PoolPending   int `json:"pool_pending"`
	PoolSucceeded int `json:"pool_succeeded"`
	PoolFailed    int `json:"pool_failed"`
}

func ProcessPendingBillingAdjustments(ctx context.Context, limit int) BillingReconciliationSummary {
	adjustments, err := model.FindPendingBillingAdjustments(limit)
	if err != nil {
		logger.LogError(ctx, "load pending billing adjustments failed: "+err.Error())
		return BillingReconciliationSummary{Failed: 1}
	}
	summary := BillingReconciliationSummary{Pending: len(adjustments)}
	for _, adjustment := range adjustments {
		if ctx != nil && ctx.Err() != nil {
			break
		}
		if adjustment.Status == model.BillingAdjustmentStatusReserved {
			adjustment, err = model.FinalizeBillingReservation(adjustment.ID, 0, false, true)
			if err != nil {
				summary.Failed++
				logger.LogError(ctx, "recover billing reservation failed: "+err.Error())
				continue
			}
		}
		if err := model.ApplyBillingAdjustment(adjustment.ID); err != nil {
			summary.Failed++
			logger.LogError(ctx, "retry billing adjustment failed: "+err.Error())
			continue
		}
		summary.Succeeded++
	}
	poolAdjustments, err := model.FindPendingQuotaPoolAdjustments(limit)
	if err != nil {
		summary.PoolFailed++
		logger.LogError(ctx, "load pending quota pool adjustments failed: "+err.Error())
		return summary
	}
	summary.PoolPending = len(poolAdjustments)
	for _, adjustment := range poolAdjustments {
		if ctx != nil && ctx.Err() != nil {
			break
		}
		if adjustment.Status == model.QuotaPoolAdjustmentStatusReserved {
			adjustment, err = model.FinalizeQuotaPoolReservation(adjustment.ID, 0)
			if err != nil {
				summary.PoolFailed++
				logger.LogError(ctx, "recover quota pool reservation failed: "+err.Error())
				continue
			}
		}
		if err := applyQuotaPoolAdjustment(adjustment); err != nil {
			summary.PoolFailed++
			logger.LogError(ctx, "retry quota pool adjustment failed: "+err.Error())
			continue
		}
		summary.PoolSucceeded++
	}
	return summary
}
