package openai

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeClientUsage(t *testing.T) {
	t.Run("裸 usage 补齐缓存计量字段", func(t *testing.T) {
		got, ok := normalizeClientUsage(map[string]any{
			"prompt_tokens": 1450.0, "completion_tokens": 9.0, "total_tokens": 1459.0,
		}, 0)
		require.True(t, ok)
		assert.Equal(t, 1450.0, got["prompt_tokens"])
		details := got["prompt_tokens_details"].(map[string]any)
		assert.Equal(t, 0, details["cached_tokens"])
	})

	t.Run("剥掉非官方顶层字段", func(t *testing.T) {
		got, ok := normalizeClientUsage(map[string]any{
			"prompt_tokens": 1301.0, "completion_tokens": 16.0, "total_tokens": 1317.0,
			"estimated_cost": 0.0039,
			"prompt_tokens_details": map[string]any{
				"cache_write_tokens": nil, "cached_tokens": 5.0,
			},
		}, 0)
		require.True(t, ok)
		assert.NotContains(t, got, "estimated_cost")
		details := got["prompt_tokens_details"].(map[string]any)
		assert.Equal(t, 5.0, details["cached_tokens"], "上游已报的缓存值不该被覆盖")
	})

	t.Run("moonshot 方言缓存字段保留", func(t *testing.T) {
		got, ok := normalizeClientUsage(map[string]any{
			"prompt_tokens": 10.0, "completion_tokens": 1.0, "total_tokens": 11.0,
			"prompt_cache_hit_tokens": 8.0,
		}, 8)
		require.True(t, ok)
		assert.Equal(t, 8.0, got["prompt_cache_hit_tokens"])
		details := got["prompt_tokens_details"].(map[string]any)
		assert.Equal(t, 8, details["cached_tokens"], "billing 侧从方言位置提取的值应回填到官方位置")
	})

	t.Run("hub 自建 usage 结构体也规范化", func(t *testing.T) {
		usage := dto.Usage{PromptTokens: 100, CompletionTokens: 20, TotalTokens: 120}
		got, ok := normalizeClientUsage(usage, 0)
		require.True(t, ok)
		assert.NotContains(t, got, "claude_cache_creation_5_m_tokens")
		assert.NotContains(t, got, "input_tokens")
		details := got["prompt_tokens_details"].(map[string]any)
		assert.Contains(t, details, "cached_tokens")
	})

	t.Run("usage 缺失或非对象跳过", func(t *testing.T) {
		_, ok := normalizeClientUsage(nil, 0)
		assert.False(t, ok)
		_, ok = normalizeClientUsage("oops", 0)
		assert.False(t, ok)
	})
}
