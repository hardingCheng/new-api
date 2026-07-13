package service

import (
	"context"

	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
)

type BillingReconciliationSummary struct {
	Pending   int `json:"pending"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
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
		if err := model.ApplyBillingAdjustment(adjustment.ID); err != nil {
			summary.Failed++
			logger.LogError(ctx, "retry billing adjustment failed: "+err.Error())
			continue
		}
		summary.Succeeded++
	}
	return summary
}
