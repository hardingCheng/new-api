package controller

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCrossLoginCodeSingleUseAndHostBinding(t *testing.T) {
	code, err := issueCrossLoginCode(42, "z.open-api.ai")
	require.NoError(t, err)
	require.NotEmpty(t, code)

	// 域名不符:消费失败,且令牌已销毁不可重试
	userId, ok := consumeCrossLoginCode(code, "easymodelhub.com")
	require.False(t, ok)
	require.Zero(t, userId)
	_, ok = consumeCrossLoginCode(code, "z.open-api.ai")
	require.False(t, ok, "code must be destroyed after first consume attempt")

	// 域名相符(带端口/大小写归一化):消费成功,二次消费失败
	code, err = issueCrossLoginCode(42, "z.open-api.ai")
	require.NoError(t, err)
	userId, ok = consumeCrossLoginCode(code, "Z.Open-API.ai:443")
	require.True(t, ok)
	require.Equal(t, 42, userId)
	_, ok = consumeCrossLoginCode(code, "z.open-api.ai")
	require.False(t, ok, "code must be single-use")

	// 未知令牌与空令牌
	_, ok = consumeCrossLoginCode("no-such-code", "z.open-api.ai")
	require.False(t, ok)
	_, ok = consumeCrossLoginCode("", "z.open-api.ai")
	require.False(t, ok)
}

func TestCrossLoginCodeExpiry(t *testing.T) {
	code, err := issueCrossLoginCode(7, "z.open-api.ai")
	require.NoError(t, err)

	value, loaded := crossLoginTickets.Load(code)
	require.True(t, loaded)
	ticket := value.(crossLoginTicket)
	ticket.expireAt = time.Now().Add(-time.Second)
	crossLoginTickets.Store(code, ticket)

	_, ok := consumeCrossLoginCode(code, "z.open-api.ai")
	require.False(t, ok, "expired code must be rejected")
}
