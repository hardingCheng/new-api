package common

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestScaleTokenUsageJSONAcrossProtocols(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		checks map[string]int64
	}{
		{
			name:  "openai responses nested usage",
			input: `{"type":"response.completed","response":{"created_at":1710000000,"usage":{"input_tokens":1000,"output_tokens":250,"total_tokens":1250,"input_tokens_details":{"cached_tokens":200},"output_tokens_details":{"reasoning_tokens":50}},"max_output_tokens":4096}}`,
			checks: map[string]int64{
				"response.usage.input_tokens":                           2000,
				"response.usage.output_tokens":                          500,
				"response.usage.total_tokens":                           2500,
				"response.usage.input_tokens_details.cached_tokens":     400,
				"response.usage.output_tokens_details.reasoning_tokens": 100,
				"response.created_at":                                   1710000000,
				"response.max_output_tokens":                            4096,
			},
		},
		{
			name:  "anthropic usage",
			input: `{"type":"message_delta","usage":{"input_tokens":1000,"output_tokens":50,"cache_read_input_tokens":300,"cache_creation":{"ephemeral_5m_input_tokens":40}}}`,
			checks: map[string]int64{
				"usage.input_tokens":                             2000,
				"usage.output_tokens":                            100,
				"usage.cache_read_input_tokens":                  600,
				"usage.cache_creation.ephemeral_5m_input_tokens": 80,
			},
		},
		{
			name:  "gemini usage metadata",
			input: `{"usageMetadata":{"promptTokenCount":1000,"candidatesTokenCount":200,"totalTokenCount":1200,"promptTokensDetails":[{"modality":"TEXT","tokenCount":1000}]},"modelVersion":"gemini-test"}`,
			checks: map[string]int64{
				"usageMetadata.promptTokenCount":                 2000,
				"usageMetadata.candidatesTokenCount":             400,
				"usageMetadata.totalTokenCount":                  2400,
				"usageMetadata.promptTokensDetails.0.tokenCount": 2000,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := ScaleTokenUsageJSON([]byte(tt.input), 2)
			require.True(t, gjson.ValidBytes(output))
			for path, expected := range tt.checks {
				assert.Equal(t, expected, gjson.GetBytes(output, path).Int(), path)
			}
		})
	}
}

func TestScaleTokenUsageJSONLeavesNonUsagePayloadsUnchanged(t *testing.T) {
	input := []byte(`{"max_tokens":1000,"metadata":{"tokenCount":20},"message":"1000 tokens"}`)
	assert.Equal(t, string(input), string(ScaleTokenUsageJSON(input, 2)))
	assert.Equal(t, "[DONE]", string(ScaleTokenUsageJSON([]byte("[DONE]"), 2)))
}

func TestScaleTokenCountSaturatesAtDatabaseLimit(t *testing.T) {
	assert.Equal(t, 2000, ScaleTokenCount(1000, 2))
	assert.Equal(t, math.MaxInt32, ScaleTokenCount(math.MaxInt32, 2))
	assert.Equal(t, -1, ScaleTokenCount(-1, 2))
}
