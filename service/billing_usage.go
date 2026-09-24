package service

import (
	"github.com/QuantumNous/new-api/common"
	hostdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

const (
	usageBillingPathLocal              = "local"
	usageBillingPathUpstream           = "upstream"
	usageBillingPathOpenAI             = "billing-usage-openai"
	usageBillingPathOpenAIEstimated    = "billing-usage-openai-estimated"
	usageBillingPathAnthropic          = "billing-usage-anthropic"
	usageBillingPathAnthropicEstimated = "billing-usage-anthropic-estimated"
	usageBillingPathGemini             = "billing-usage-gemini"
	usageBillingPathGeminiEstimated    = "billing-usage-gemini-estimated"
)

func effectiveBillingUsage(usage *dto.Usage) *dto.Usage {
	if billingUsage, ok := usageFromBillingUsage(usage); ok {
		return billingUsage
	}
	return usage
}

func contractualBillingUsage(usage *dto.Usage, multiplier int) *dto.Usage {
	return hostdto.ScaleUsageCopy(effectiveBillingUsage(usage), multiplier)
}

func attachUsageTokenMultiplier(other *model.LogOther) {
	if other == nil || common.UsageTokenMultiplier <= 1 {
		return
	}
	other.SetAdmin("usage_token_multiplier", common.UsageTokenMultiplier)
}

func hasActualTokenUsage(isLocalCountTokens bool, usage *dto.Usage) bool {
	if isLocalCountTokens || usage == nil {
		return false
	}
	if usage.BillingUsage != nil && usage.BillingUsage.Estimated {
		return false
	}
	return dto.HasOpenAIUsageTokens(effectiveBillingUsage(usage))
}

func usageBillingPathForLog(isLocalCountTokens bool, usage *dto.Usage) string {
	effectiveUsage, ok := usageFromBillingUsage(usage)
	if !ok {
		if isLocalCountTokens {
			return usageBillingPathLocal
		}
		return usageBillingPathUpstream
	}

	switch effectiveUsage.UsageSemantic {
	case dto.BillingUsageSemanticOpenAI:
		if usage.BillingUsage.Estimated {
			return usageBillingPathOpenAIEstimated
		}
		return usageBillingPathOpenAI
	case dto.BillingUsageSemanticAnthropic:
		if usage.BillingUsage.Estimated {
			return usageBillingPathAnthropicEstimated
		}
		return usageBillingPathAnthropic
	case dto.BillingUsageSemanticGemini:
		if usage.BillingUsage.Estimated {
			return usageBillingPathGeminiEstimated
		}
		return usageBillingPathGemini
	}

	return usageBillingPathUpstream
}

func appendUsageBillingPathForLog(other *model.LogOther, isLocalCountTokens bool, usage *dto.Usage) {
	if other == nil {
		return
	}
	other.SetAdmin("usage_billing_path", usageBillingPathForLog(isLocalCountTokens, usage))
}

func usageFromBillingUsage(usage *dto.Usage) (*dto.Usage, bool) {
	if usage == nil || usage.BillingUsage == nil {
		return nil, false
	}
	return usage.BillingUsage.CanonicalUsage()
}
