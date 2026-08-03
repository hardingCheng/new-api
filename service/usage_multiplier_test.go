package service

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/require"
)

func TestContractualBillingUsageScalesEffectiveProviderUsage(t *testing.T) {
	usage := &dto.Usage{
		PromptTokens:     1,
		CompletionTokens: 1,
		BillingUsage: dto.NewClaudeMessagesBillingUsage(&dto.ClaudeUsage{
			InputTokens:              1000,
			OutputTokens:             200,
			CacheReadInputTokens:     300,
			CacheCreationInputTokens: 40,
		}),
	}

	scaled := contractualBillingUsage(usage, 2)
	require.NotNil(t, scaled)
	require.Equal(t, 2000, scaled.PromptTokens)
	require.Equal(t, 400, scaled.CompletionTokens)
	require.Equal(t, 600, scaled.PromptTokensDetails.CachedTokens)
	require.Equal(t, 80, scaled.PromptTokensDetails.CachedCreationTokens)

	// Provider usage remains available at its real value for affinity/diagnostics.
	require.Equal(t, 1000, usage.BillingUsage.ClaudeUsage.InputTokens)
}
