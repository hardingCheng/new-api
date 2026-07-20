package types

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMaskUpstreamBillingError(t *testing.T) {
	t.Run("upstream group permission leak is masked to 429", func(t *testing.T) {
		err := WithOpenAIError(OpenAIError{
			Message: "无权访问 glm专用0.1 分组 (request id: 2026xxx)",
			Type:    "new_api_error",
		}, http.StatusForbidden)

		masked := MaskUpstreamBillingError(err)

		require.NotNil(t, masked)
		assert.Equal(t, http.StatusTooManyRequests, masked.StatusCode)
		assert.Equal(t, MaskedUpstreamBillingMessage, masked.Error())
		assert.Equal(t, MaskedUpstreamBillingMessage, masked.ToOpenAIError().Message)
		assert.Equal(t, MaskedUpstreamBillingMessage, masked.ToClaudeError().Message)
	})

	t.Run("upstream usage quota message is masked", func(t *testing.T) {
		err := WithOpenAIError(OpenAIError{
			Message: "You have exceeded the weekly usage quota. It will reset at 2026-07-20 00:00:00",
			Type:    "rate_limit_error",
		}, http.StatusTooManyRequests)

		masked := MaskUpstreamBillingError(err)

		assert.Equal(t, http.StatusTooManyRequests, masked.StatusCode)
		assert.Equal(t, MaskedUpstreamBillingMessage, masked.Error())
	})

	t.Run("upstream new-api balance error is masked", func(t *testing.T) {
		err := WithOpenAIError(OpenAIError{
			Message: "预扣费额度失败, 用户剩余额度: ＄0.000050, 需要预扣费额度: ＄0.028248",
			Type:    "new_api_error",
			Code:    "insufficient_user_quota",
		}, http.StatusForbidden)

		masked := MaskUpstreamBillingError(err)

		assert.Equal(t, http.StatusTooManyRequests, masked.StatusCode)
		assert.Equal(t, MaskedUpstreamBillingMessage, masked.Error())
	})

	t.Run("upstream 402 is masked even without keywords", func(t *testing.T) {
		err := WithOpenAIError(OpenAIError{
			Message: "please renew your plan",
			Type:    "upstream_error",
		}, http.StatusPaymentRequired)

		masked := MaskUpstreamBillingError(err)

		assert.Equal(t, http.StatusTooManyRequests, masked.StatusCode)
		assert.Equal(t, MaskedUpstreamBillingMessage, masked.Error())
	})

	t.Run("own customer quota error is untouched", func(t *testing.T) {
		original := NewErrorWithStatusCode(
			errors.New("预扣费额度失败, 用户剩余额度: ￥0.06, 需要预扣费额度: ￥1.20"),
			ErrorCodeInsufficientUserQuota,
			http.StatusForbidden,
			ErrOptionWithSkipRetry(),
		)

		result := MaskUpstreamBillingError(original)

		assert.Same(t, original, result)
		assert.Equal(t, http.StatusForbidden, result.StatusCode)
		assert.Contains(t, result.Error(), "用户剩余额度")
	})

	t.Run("own get_channel_failed error is untouched", func(t *testing.T) {
		original := NewError(
			errors.New("分组 glm 下模型 glm-5.2 的可用渠道不存在（retry）"),
			ErrorCodeGetChannelFailed,
			ErrOptionWithSkipRetry(),
		)

		result := MaskUpstreamBillingError(original)

		assert.Same(t, original, result)
	})

	t.Run("upstream non-billing error is untouched", func(t *testing.T) {
		original := WithOpenAIError(OpenAIError{
			Message: "This model's maximum context length is 128000 tokens",
			Type:    "invalid_request_error",
		}, http.StatusBadRequest)

		result := MaskUpstreamBillingError(original)

		assert.Same(t, original, result)
		assert.Equal(t, http.StatusBadRequest, result.StatusCode)
	})

	t.Run("nil error stays nil", func(t *testing.T) {
		assert.Nil(t, MaskUpstreamBillingError(nil))
	})
}

func TestMatchesUpstreamBillingLeak(t *testing.T) {
	assert.True(t, MatchesUpstreamBillingLeak("Insufficient Account Balance"))
	assert.True(t, MatchesUpstreamBillingLeak("账户余额不足，请充值"))
	assert.True(t, MatchesUpstreamBillingLeak("您购买的套餐已到期"))
	assert.False(t, MatchesUpstreamBillingLeak("model_not_found"))
	assert.False(t, MatchesUpstreamBillingLeak("request timeout"))
}
