package dto

import "github.com/QuantumNous/new-api/common"

// ScaleUsageCopy returns an independently scalable usage value so contractual
// billing/display adjustments never mutate the provider's original usage.
func ScaleUsageCopy(usage *Usage, multiplier int) *Usage {
	if usage == nil || multiplier <= 1 {
		return usage
	}

	scaled := *usage
	scaled.PromptTokens = common.ScaleTokenCount(scaled.PromptTokens, multiplier)
	scaled.CompletionTokens = common.ScaleTokenCount(scaled.CompletionTokens, multiplier)
	scaled.TotalTokens = common.ScaleTokenCount(scaled.TotalTokens, multiplier)
	scaled.PromptCacheHitTokens = common.ScaleTokenCount(scaled.PromptCacheHitTokens, multiplier)
	scaled.InputTokens = common.ScaleTokenCount(scaled.InputTokens, multiplier)
	scaled.OutputTokens = common.ScaleTokenCount(scaled.OutputTokens, multiplier)
	scaled.ClaudeCacheCreation5mTokens = common.ScaleTokenCount(scaled.ClaudeCacheCreation5mTokens, multiplier)
	scaled.ClaudeCacheCreation1hTokens = common.ScaleTokenCount(scaled.ClaudeCacheCreation1hTokens, multiplier)

	scaleInputTokenDetails(&scaled.PromptTokensDetails, multiplier)
	scaleOutputTokenDetails(&scaled.CompletionTokenDetails, multiplier)
	if usage.InputTokensDetails != nil {
		inputDetails := *usage.InputTokensDetails
		scaleInputTokenDetails(&inputDetails, multiplier)
		scaled.InputTokensDetails = &inputDetails
	}
	return &scaled
}

func ScaleRealtimeUsageCopy(usage *RealtimeUsage, multiplier int) *RealtimeUsage {
	if usage == nil || multiplier <= 1 {
		return usage
	}

	scaled := *usage
	scaled.TotalTokens = common.ScaleTokenCount(scaled.TotalTokens, multiplier)
	scaled.InputTokens = common.ScaleTokenCount(scaled.InputTokens, multiplier)
	scaled.OutputTokens = common.ScaleTokenCount(scaled.OutputTokens, multiplier)
	scaleInputTokenDetails(&scaled.InputTokenDetails, multiplier)
	scaleOutputTokenDetails(&scaled.OutputTokenDetails, multiplier)
	return &scaled
}

func scaleInputTokenDetails(details *InputTokenDetails, multiplier int) {
	if details == nil {
		return
	}
	details.CachedTokens = common.ScaleTokenCount(details.CachedTokens, multiplier)
	details.CachedCreationTokens = common.ScaleTokenCount(details.CachedCreationTokens, multiplier)
	details.CacheWriteTokens = common.ScaleTokenCount(details.CacheWriteTokens, multiplier)
	details.TextTokens = common.ScaleTokenCount(details.TextTokens, multiplier)
	details.AudioTokens = common.ScaleTokenCount(details.AudioTokens, multiplier)
	details.ImageTokens = common.ScaleTokenCount(details.ImageTokens, multiplier)
}

func scaleOutputTokenDetails(details *OutputTokenDetails, multiplier int) {
	if details == nil {
		return
	}
	details.TextTokens = common.ScaleTokenCount(details.TextTokens, multiplier)
	details.AudioTokens = common.ScaleTokenCount(details.AudioTokens, multiplier)
	details.ImageTokens = common.ScaleTokenCount(details.ImageTokens, multiplier)
	details.ReasoningTokens = common.ScaleTokenCount(details.ReasoningTokens, multiplier)
}
