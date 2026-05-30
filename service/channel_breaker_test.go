package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestChannelBreakerOpensAfterFailureThreshold(t *testing.T) {
	common.SetAutomaticDisableChannelEnabled(true)
	disableRedisForBreakerTest(t)
	t.Cleanup(func() { common.SetAutomaticDisableChannelEnabled(false) })

	c := testBreakerContext("/v1/chat/completions")
	channelError := types.ChannelError{ChannelId: 1001, UsingKey: "key-a", AutoBan: true}
	ClearChannelBreaker(channelError)

	for i := 1; i < GetChannelBreakerFailureThreshold(); i++ {
		opened, _ := RecordChannelBreakerFailure(c, channelError, true)
		require.False(t, opened)
		require.True(t, AllowChannelByBreaker(c, channelError))
	}

	opened, _ := RecordChannelBreakerFailure(c, channelError, true)
	require.True(t, opened)
	require.False(t, AllowChannelByBreaker(c, channelError))
}

func TestChannelBreakerSuccessClearsFailures(t *testing.T) {
	common.SetAutomaticDisableChannelEnabled(true)
	disableRedisForBreakerTest(t)
	t.Cleanup(func() { common.SetAutomaticDisableChannelEnabled(false) })

	c := testBreakerContext("/v1/chat/completions")
	channelError := types.ChannelError{ChannelId: 1002, UsingKey: "key-a", AutoBan: true}
	ClearChannelBreaker(channelError)

	opened, _ := RecordChannelBreakerFailure(c, channelError, true)
	require.False(t, opened)
	RecordChannelBreakerSuccess(c, channelError)

	for i := 1; i < GetChannelBreakerFailureThreshold(); i++ {
		opened, _ = RecordChannelBreakerFailure(c, channelError, true)
		require.False(t, opened)
	}
}

func TestChannelBreakerSeparatesKeys(t *testing.T) {
	common.SetAutomaticDisableChannelEnabled(true)
	disableRedisForBreakerTest(t)
	t.Cleanup(func() { common.SetAutomaticDisableChannelEnabled(false) })

	c := testBreakerContext("/v1/chat/completions")
	keyA := types.ChannelError{ChannelId: 1003, UsingKey: "key-a", AutoBan: true}
	keyB := types.ChannelError{ChannelId: 1003, UsingKey: "key-b", AutoBan: true}
	ClearChannelBreaker(keyA)
	ClearChannelBreaker(keyB)

	for i := 0; i < GetChannelBreakerFailureThreshold(); i++ {
		RecordChannelBreakerFailure(c, keyA, true)
	}

	require.False(t, AllowChannelByBreaker(c, keyA))
	require.True(t, AllowChannelByBreaker(c, keyB))
}

func TestChannelBreakerExcludesVideos(t *testing.T) {
	common.SetAutomaticDisableChannelEnabled(true)
	disableRedisForBreakerTest(t)
	t.Cleanup(func() { common.SetAutomaticDisableChannelEnabled(false) })

	c := testBreakerContext("/v1/videos")
	channelError := types.ChannelError{ChannelId: 1004, UsingKey: "key-a", AutoBan: true}
	ClearChannelBreaker(channelError)

	for i := 0; i < GetChannelBreakerFailureThreshold(); i++ {
		opened, _ := RecordChannelBreakerFailure(c, channelError, true)
		require.False(t, opened)
	}
	require.True(t, AllowChannelByBreaker(c, channelError))
}

func TestChannelBreakerHalfOpenRestoresAfterMajoritySuccess(t *testing.T) {
	common.SetAutomaticDisableChannelEnabled(true)
	disableRedisForBreakerTest(t)
	oldCooldown := channelBreakerCooldown
	channelBreakerCooldown = time.Millisecond
	t.Cleanup(func() {
		common.SetAutomaticDisableChannelEnabled(false)
		channelBreakerCooldown = oldCooldown
	})

	c := testBreakerContext("/v1/chat/completions")
	channelError := types.ChannelError{ChannelId: 1005, UsingKey: "key-a", AutoBan: true}
	ClearChannelBreaker(channelError)

	for i := 0; i < GetChannelBreakerFailureThreshold(); i++ {
		RecordChannelBreakerFailure(c, channelError, true)
	}
	time.Sleep(2 * time.Millisecond)

	for i := 0; i < common.GetChannelBreakerProbeSuccessCount(); i++ {
		require.True(t, AllowChannelByBreaker(c, channelError))
		RecordChannelBreakerSuccess(c, channelError)
	}

	require.True(t, AllowChannelByBreaker(c, channelError))
}

func TestChannelBreakerHalfOpenRestoresWithMixedMajoritySuccess(t *testing.T) {
	common.SetAutomaticDisableChannelEnabled(true)
	disableRedisForBreakerTest(t)
	oldCooldown := channelBreakerCooldown
	channelBreakerCooldown = time.Millisecond
	t.Cleanup(func() {
		common.SetAutomaticDisableChannelEnabled(false)
		channelBreakerCooldown = oldCooldown
	})

	c := testBreakerContext("/v1/chat/completions")
	channelError := types.ChannelError{ChannelId: 1006, UsingKey: "key-a", AutoBan: true}
	ClearChannelBreaker(channelError)

	for i := 0; i < GetChannelBreakerFailureThreshold(); i++ {
		RecordChannelBreakerFailure(c, channelError, true)
	}
	time.Sleep(2 * time.Millisecond)

	for i := 0; i < 2; i++ {
		require.True(t, AllowChannelByBreaker(c, channelError))
		RecordChannelBreakerFailure(c, channelError, true)
	}
	for i := 0; i < 3; i++ {
		require.True(t, AllowChannelByBreaker(c, channelError))
		RecordChannelBreakerSuccess(c, channelError)
	}

	require.True(t, AllowChannelByBreaker(c, channelError))
}

func TestChannelBreakerCanUseDoesNotConsumeProbeSlot(t *testing.T) {
	common.SetAutomaticDisableChannelEnabled(true)
	disableRedisForBreakerTest(t)
	oldCooldown := channelBreakerCooldown
	channelBreakerCooldown = time.Millisecond
	t.Cleanup(func() {
		common.SetAutomaticDisableChannelEnabled(false)
		channelBreakerCooldown = oldCooldown
	})

	c := testBreakerContext("/v1/chat/completions")
	channelError := types.ChannelError{ChannelId: 1007, UsingKey: "key-a", AutoBan: true}
	ClearChannelBreaker(channelError)

	for i := 0; i < GetChannelBreakerFailureThreshold(); i++ {
		RecordChannelBreakerFailure(c, channelError, true)
	}
	time.Sleep(2 * time.Millisecond)

	for i := 0; i < common.GetChannelBreakerProbeCount()*2; i++ {
		require.True(t, CanUseChannelByBreaker(c, channelError))
	}
	for i := 0; i < common.GetChannelBreakerProbeCount(); i++ {
		require.True(t, AcquireChannelBreakerProbe(c, channelError))
	}
	require.False(t, AcquireChannelBreakerProbe(c, channelError))
}

func testBreakerContext(path string) *gin.Context {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, path, nil)
	return c
}

func disableRedisForBreakerTest(t *testing.T) {
	t.Helper()
	oldRedisEnabled := common.RedisEnabled
	oldRDB := common.RDB
	common.RedisEnabled = false
	common.RDB = nil
	t.Cleanup(func() {
		common.RedisEnabled = oldRedisEnabled
		common.RDB = oldRDB
	})
}
