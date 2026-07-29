package dto

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScaleUsageCopyDoublesBillingCountersWithoutMutatingProviderUsage(t *testing.T) {
	inputDetails := &InputTokenDetails{CachedTokens: 9, AudioTokens: 3}
	original := &Usage{
		PromptTokens:     1000,
		CompletionTokens: 200,
		TotalTokens:      1200,
		InputTokens:      1000,
		OutputTokens:     200,
		PromptTokensDetails: InputTokenDetails{
			CachedTokens:         100,
			CachedCreationTokens: 20,
			ImageTokens:          10,
		},
		CompletionTokenDetails: OutputTokenDetails{ReasoningTokens: 50},
		InputTokensDetails:     inputDetails,
	}

	scaled := ScaleUsageCopy(original, 2)
	require.NotSame(t, original, scaled)
	assert.Equal(t, 2000, scaled.PromptTokens)
	assert.Equal(t, 400, scaled.CompletionTokens)
	assert.Equal(t, 2400, scaled.TotalTokens)
	assert.Equal(t, 200, scaled.PromptTokensDetails.CachedTokens)
	assert.Equal(t, 40, scaled.PromptTokensDetails.CachedCreationTokens)
	assert.Equal(t, 20, scaled.PromptTokensDetails.ImageTokens)
	assert.Equal(t, 100, scaled.CompletionTokenDetails.ReasoningTokens)
	require.NotNil(t, scaled.InputTokensDetails)
	assert.Equal(t, 18, scaled.InputTokensDetails.CachedTokens)
	assert.Equal(t, 6, scaled.InputTokensDetails.AudioTokens)

	assert.Equal(t, 1000, original.PromptTokens)
	assert.Equal(t, 100, original.PromptTokensDetails.CachedTokens)
	assert.Equal(t, 9, original.InputTokensDetails.CachedTokens)
}

func TestScaleRealtimeUsageCopy(t *testing.T) {
	original := &RealtimeUsage{
		TotalTokens:  120,
		InputTokens:  100,
		OutputTokens: 20,
		InputTokenDetails: InputTokenDetails{
			TextTokens:  80,
			AudioTokens: 20,
		},
		OutputTokenDetails: OutputTokenDetails{
			TextTokens:  10,
			AudioTokens: 10,
		},
	}

	scaled := ScaleRealtimeUsageCopy(original, 2)
	require.NotSame(t, original, scaled)
	assert.Equal(t, 240, scaled.TotalTokens)
	assert.Equal(t, 200, scaled.InputTokens)
	assert.Equal(t, 40, scaled.OutputTokens)
	assert.Equal(t, 160, scaled.InputTokenDetails.TextTokens)
	assert.Equal(t, 20, scaled.OutputTokenDetails.AudioTokens)
	assert.Equal(t, 120, original.TotalTokens)
}
