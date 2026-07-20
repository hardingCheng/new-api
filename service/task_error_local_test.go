package service

import (
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TaskErrorFromAPIError 的 LocalError 语义决定了任务错误在出口处是否会被
// 当作上游错误脱敏（respondTaskError）：我方自身错误必须保留原文给客户。
func TestTaskErrorFromAPIErrorLocality(t *testing.T) {
	local := types.NewErrorWithStatusCode(
		errors.New("预扣费额度失败, 用户剩余额度: ￥0.06"),
		types.ErrorCodeInsufficientUserQuota,
		http.StatusForbidden,
	)
	taskErr := TaskErrorFromAPIError(local)
	require.NotNil(t, taskErr)
	assert.True(t, taskErr.LocalError, "hub-generated billing errors must stay local")

	upstream := types.WithOpenAIError(types.OpenAIError{
		Message: "insufficient account balance",
	}, http.StatusForbidden)
	taskErr = TaskErrorFromAPIError(upstream)
	require.NotNil(t, taskErr)
	assert.False(t, taskErr.LocalError, "upstream-parsed errors must not be marked local")
}
