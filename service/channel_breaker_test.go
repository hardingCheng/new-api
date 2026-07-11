package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
)

func TestChannelBreakerOpensAfterFailureThreshold(t *testing.T) {
	common.SetChannelBreakerEnabled(true)
	disableRedisForBreakerTest(t)
	t.Cleanup(func() { common.SetChannelBreakerEnabled(false) })

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
	common.SetChannelBreakerEnabled(true)
	disableRedisForBreakerTest(t)
	t.Cleanup(func() { common.SetChannelBreakerEnabled(false) })

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

func TestChannelBreakerLateSuccessDoesNotCloseOpenState(t *testing.T) {
	common.SetChannelBreakerEnabled(true)
	disableRedisForBreakerTest(t)
	t.Cleanup(func() { common.SetChannelBreakerEnabled(false) })

	c := testBreakerContext("/v1/chat/completions")
	channelError := types.ChannelError{ChannelId: 1021, UsingKey: "key-a", AutoBan: true}
	ClearChannelBreaker(channelError)

	for i := 0; i < GetChannelBreakerFailureThreshold(); i++ {
		RecordChannelBreakerFailure(c, channelError, true)
	}
	RecordChannelBreakerSuccess(c, channelError)

	require.False(t, AllowChannelByBreaker(c, channelError))
}

func TestChannelBreakerSeparatesKeys(t *testing.T) {
	common.SetChannelBreakerEnabled(true)
	disableRedisForBreakerTest(t)
	t.Cleanup(func() { common.SetChannelBreakerEnabled(false) })

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

func TestChannelBreakerWholeChannelWhenKeyIsolationDisabled(t *testing.T) {
	common.SetChannelBreakerEnabled(true)
	common.SetChannelBreakerRules([]common.ChannelBreakerRule{{
		Id:             "whole-channel",
		Name:           "整渠道熔断",
		Enabled:        true,
		Scope:          common.ChannelBreakerScopeGlobal,
		FailureLimit:   1,
		OnlyKeyBreaker: false,
	}})
	disableRedisForBreakerTest(t)
	t.Cleanup(func() {
		common.SetChannelBreakerEnabled(false)
		common.SetChannelBreakerRules(nil)
	})

	c := testBreakerContext("/v1/chat/completions")
	keyA := types.ChannelError{ChannelId: 1027, UsingKey: "key-a", AutoBan: true}
	keyB := types.ChannelError{ChannelId: 1027, UsingKey: "key-b", AutoBan: true}
	opened, _ := RecordChannelBreakerFailure(c, keyA, true)
	require.True(t, opened)
	require.False(t, AllowChannelByBreaker(c, keyB))
}

func TestChannelBreakerGroupRuleStateIsolation(t *testing.T) {
	common.SetChannelBreakerEnabled(true)
	common.SetChannelBreakerRules([]common.ChannelBreakerRule{{
		Id:             "vip-isolated",
		Name:           "VIP 独立熔断",
		Enabled:        true,
		Scope:          common.ChannelBreakerScopeGroup,
		Targets:        []string{"vip"},
		FailureLimit:   1,
		OnlyKeyBreaker: true,
	}})
	disableRedisForBreakerTest(t)
	t.Cleanup(func() {
		common.SetChannelBreakerEnabled(false)
		common.SetChannelBreakerRules(nil)
	})

	vipCtx := testBreakerContext("/v1/chat/completions")
	common.SetContextKey(vipCtx, constant.ContextKeyUsingGroup, "vip")
	defaultCtx := testBreakerContext("/v1/chat/completions")
	common.SetContextKey(defaultCtx, constant.ContextKeyUsingGroup, "default")
	channelError := types.ChannelError{ChannelId: 1028, UsingKey: "key-a", AutoBan: true}

	opened, _ := RecordChannelBreakerFailure(vipCtx, channelError, true)
	require.True(t, opened)
	require.False(t, AllowChannelByBreaker(vipCtx, channelError))
	require.True(t, AllowChannelByBreaker(defaultCtx, channelError))
	ClearChannelBreaker(channelError)
	require.True(t, AllowChannelByBreaker(vipCtx, channelError))
}

func TestChannelBreakerModelRuleStateIsolation(t *testing.T) {
	common.SetChannelBreakerEnabled(true)
	common.SetChannelBreakerRules([]common.ChannelBreakerRule{{
		Id:             "video-isolated",
		Name:           "视频模型独立熔断",
		Enabled:        true,
		Scope:          common.ChannelBreakerScopeModel,
		Targets:        []string{"video-model"},
		FailureLimit:   1,
		OnlyKeyBreaker: true,
	}})
	disableRedisForBreakerTest(t)
	t.Cleanup(func() {
		common.SetChannelBreakerEnabled(false)
		common.SetChannelBreakerRules(nil)
	})

	videoCtx := testBreakerContext("/v1/chat/completions")
	common.SetContextKey(videoCtx, constant.ContextKeyOriginalModel, "video-model")
	chatCtx := testBreakerContext("/v1/chat/completions")
	common.SetContextKey(chatCtx, constant.ContextKeyOriginalModel, "chat-model")
	channelError := types.ChannelError{ChannelId: 1031, UsingKey: "key-a", AutoBan: true}

	opened, _ := RecordChannelBreakerFailure(videoCtx, channelError, true)
	require.True(t, opened)
	require.False(t, AllowChannelByBreaker(videoCtx, channelError))
	require.True(t, AllowChannelByBreaker(chatCtx, channelError))
}

func TestTypesChannelErrorIncludesSingleChannelKey(t *testing.T) {
	single := typesChannelError(&model.Channel{Id: 1029, Key: "key-a"})
	require.Equal(t, "key-a", single.UsingKey)

	multi := typesChannelError(&model.Channel{Id: 1030, Key: "key-a\nkey-b", ChannelInfo: model.ChannelInfo{IsMultiKey: true}})
	require.Empty(t, multi.UsingKey)
}

func TestChannelBreakerExcludesVideos(t *testing.T) {
	common.SetChannelBreakerEnabled(true)
	disableRedisForBreakerTest(t)
	t.Cleanup(func() { common.SetChannelBreakerEnabled(false) })

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
	common.SetChannelBreakerEnabled(true)
	disableRedisForBreakerTest(t)
	oldCooldown := channelBreakerCooldown
	channelBreakerCooldown = time.Millisecond
	t.Cleanup(func() {
		common.SetChannelBreakerEnabled(false)
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

func TestChannelBreakerHalfOpenWaitsForAllProbeResults(t *testing.T) {
	common.SetChannelBreakerEnabled(true)
	disableRedisForBreakerTest(t)
	oldCooldown := channelBreakerCooldown
	channelBreakerCooldown = time.Millisecond
	t.Cleanup(func() {
		common.SetChannelBreakerEnabled(false)
		channelBreakerCooldown = oldCooldown
	})

	channelError := types.ChannelError{ChannelId: 1022, UsingKey: "key-a", AutoBan: true}
	for i := 0; i < GetChannelBreakerFailureThreshold(); i++ {
		RecordChannelBreakerFailure(testBreakerContext("/v1/chat/completions"), channelError, true)
	}
	time.Sleep(2 * time.Millisecond)

	for i := 0; i < common.GetChannelBreakerProbeSuccessCount(); i++ {
		c := testBreakerContext("/v1/chat/completions")
		require.True(t, AllowChannelByBreaker(c, channelError))
		RecordChannelBreakerSuccess(c, channelError)
	}
	state, err := readChannelBreakerState(channelBreakerKey(channelError))
	require.NoError(t, err)
	require.Equal(t, ChannelBreakerStateHalfOpen, state.State)

	for i := common.GetChannelBreakerProbeSuccessCount(); i < common.GetChannelBreakerProbeCount(); i++ {
		c := testBreakerContext("/v1/chat/completions")
		require.True(t, AllowChannelByBreaker(c, channelError))
		RecordChannelBreakerSuccess(c, channelError)
	}
	state, err = readChannelBreakerState(channelBreakerKey(channelError))
	require.NoError(t, err)
	require.Nil(t, state)
}

func TestChannelBreakerHalfOpenNeutralErrorDoesNotCountAsSuccess(t *testing.T) {
	common.SetChannelBreakerEnabled(true)
	disableRedisForBreakerTest(t)
	oldCooldown := channelBreakerCooldown
	channelBreakerCooldown = time.Millisecond
	t.Cleanup(func() {
		common.SetChannelBreakerEnabled(false)
		channelBreakerCooldown = oldCooldown
	})

	channelError := types.ChannelError{ChannelId: 1026, UsingKey: "key-a", AutoBan: true}
	for i := 0; i < GetChannelBreakerFailureThreshold(); i++ {
		RecordChannelBreakerFailure(testBreakerContext("/v1/chat/completions"), channelError, true)
	}
	time.Sleep(2 * time.Millisecond)

	c := testBreakerContext("/v1/chat/completions")
	require.True(t, AllowChannelByBreaker(c, channelError))
	RecordChannelBreakerFailure(c, channelError, false)

	state, err := readChannelBreakerState(channelBreakerKey(channelError))
	require.NoError(t, err)
	require.Equal(t, ChannelBreakerStateHalfOpen, state.State)
	require.Zero(t, state.ProbeSuccess)
	require.Zero(t, state.ProbeTotal)
	require.Zero(t, state.ProbeInFlight)
}

func TestChannelBreakerSkipRetryChannelErrorDoesNotTrip(t *testing.T) {
	common.SetChannelBreakerEnabled(true)
	t.Cleanup(func() { common.SetChannelBreakerEnabled(false) })
	c := testBreakerContext("/v1/chat/completions")
	channelError := types.ChannelError{ChannelId: 1023, UsingKey: "key-a", AutoBan: true}
	err := types.NewError(errors.New("invalid model mapping"), types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())

	require.False(t, ShouldTripChannelBreakerWithRule(c, channelError, err))
}

func TestInstantDisableTargetKeepsMultiKeyIsolation(t *testing.T) {
	multiKey := instantDisableTarget(types.ChannelError{ChannelId: 1024, IsMultiKey: true, UsingKey: "key-a"})
	require.Equal(t, "key-a", multiKey.UsingKey)
	require.True(t, multiKey.AutoBan)
	require.Equal(t, ChannelBreakerKeyHash("key-a"), instantDisableKeyHash(multiKey))

	singleKey := instantDisableTarget(types.ChannelError{ChannelId: 1025, UsingKey: "key-a"})
	require.Empty(t, singleKey.UsingKey)
	require.True(t, singleKey.AutoBan)
	require.Empty(t, instantDisableKeyHash(singleKey))
}

func TestChannelBreakerRedisConcurrentStateTransitions(t *testing.T) {
	redisURL := os.Getenv("CHANNEL_BREAKER_REDIS_TEST_URL")
	if redisURL == "" {
		t.Skip("CHANNEL_BREAKER_REDIS_TEST_URL is not set")
	}
	options, err := redis.ParseURL(redisURL)
	require.NoError(t, err)
	client := redis.NewClient(options)
	require.NoError(t, client.Ping(context.Background()).Err())
	t.Cleanup(func() { _ = client.Close() })

	oldRedisEnabled, oldRDB := common.RedisEnabled, common.RDB
	oldCooldown := channelBreakerCooldown
	common.RedisEnabled, common.RDB = true, client
	common.SetChannelBreakerEnabled(true)
	channelBreakerCooldown = time.Millisecond
	t.Cleanup(func() {
		common.RedisEnabled, common.RDB = oldRedisEnabled, oldRDB
		common.SetChannelBreakerEnabled(false)
		channelBreakerCooldown = oldCooldown
	})

	channelError := types.ChannelError{ChannelId: 91001, UsingKey: "cluster-key", AutoBan: true}
	ClearChannelBreaker(channelError)
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			RecordChannelBreakerFailure(testBreakerContext("/v1/chat/completions"), channelError, true)
		}()
	}
	wg.Wait()
	state, err := readChannelBreakerState(channelBreakerKey(channelError))
	require.NoError(t, err)
	require.NotNil(t, state)
	require.Equal(t, ChannelBreakerStateOpen, state.State)
	redisNow, err := client.Time(context.Background()).Result()
	require.NoError(t, err)
	require.WithinDuration(t, redisNow, state.OpenedAt, time.Second)

	time.Sleep(2 * time.Millisecond)
	var allowed atomic.Int64
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if AcquireChannelBreakerProbe(testBreakerContext("/v1/chat/completions"), channelError) {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()
	require.Equal(t, int64(common.GetChannelBreakerProbeCount()), allowed.Load())
	ClearChannelBreaker(channelError)

	quarantined := types.ChannelError{ChannelId: 91002, UsingKey: "key-a", IsMultiKey: true, AutoBan: true}
	otherKey := types.ChannelError{ChannelId: 91002, UsingKey: "key-b", IsMultiKey: true, AutoBan: true}
	MarkChannelBreakerQuarantine(quarantined.ChannelId, quarantined.UsingKey, true)
	require.False(t, CanUseChannelByBreaker(testBreakerContext("/v1/chat/completions"), quarantined))
	require.True(t, CanUseChannelByBreaker(testBreakerContext("/v1/chat/completions"), otherKey))
	ClearChannelBreakerQuarantine(quarantined.ChannelId, quarantined.UsingKey, true)
	require.True(t, CanUseChannelByBreaker(testBreakerContext("/v1/chat/completions"), quarantined))

	channelDisableNotifyMemory.Delete(91002)
	require.NoError(t, client.Del(context.Background(), "channel_disable_notify:91002").Err())
	var notifications atomic.Int64
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if allowChannelDisableNotify(91002, 60) {
				notifications.Add(1)
			}
		}()
	}
	wg.Wait()
	require.Equal(t, int64(1), notifications.Load())
	require.NoError(t, client.Del(context.Background(), "channel_disable_notify:91002").Err())
}

func TestChannelBreakerRedisUnavailableFailsOpen(t *testing.T) {
	client := redis.NewClient(&redis.Options{
		Addr:         "127.0.0.1:1",
		DialTimeout:  10 * time.Millisecond,
		ReadTimeout:  10 * time.Millisecond,
		WriteTimeout: 10 * time.Millisecond,
		MaxRetries:   0,
	})
	t.Cleanup(func() { _ = client.Close() })

	oldRedisEnabled, oldRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled, common.RDB = true, client
	common.SetChannelBreakerEnabled(true)
	t.Cleanup(func() {
		common.RedisEnabled, common.RDB = oldRedisEnabled, oldRDB
		common.SetChannelBreakerEnabled(false)
	})

	c := testBreakerContext("/v1/chat/completions")
	channelError := types.ChannelError{ChannelId: 91003, UsingKey: "key-a", AutoBan: true}
	require.True(t, CanUseChannelByBreaker(c, channelError))
	require.True(t, AcquireChannelBreakerProbe(c, channelError))
	opened, message := RecordChannelBreakerFailure(c, channelError, true)
	require.False(t, opened)
	require.Empty(t, message)
}

func TestChannelBreakerQuarantineKeyScope(t *testing.T) {
	multiKey := types.ChannelError{ChannelId: 91004, UsingKey: "key-a", IsMultiKey: true}
	require.Equal(t, "channel_breaker_quarantine:91004:"+ChannelBreakerKeyHash("key-a"), channelBreakerQuarantineKey(multiKey))
	require.Empty(t, channelBreakerQuarantineKey(types.ChannelError{ChannelId: 91004, IsMultiKey: true}))
	require.Equal(t, "channel_breaker_quarantine:91004", channelBreakerQuarantineKey(types.ChannelError{ChannelId: 91004}))
	require.GreaterOrEqual(t, channelBreakerQuarantineTTL(), 130*time.Second)
}

func TestChannelDisableNotificationLocalDeduplication(t *testing.T) {
	oldRedisEnabled, oldRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled, common.RDB = false, nil
	channelDisableNotifyMemory.Delete(91005)
	t.Cleanup(func() {
		channelDisableNotifyMemory.Delete(91005)
		common.RedisEnabled, common.RDB = oldRedisEnabled, oldRDB
	})

	require.True(t, allowChannelDisableNotify(91005, 60))
	require.False(t, allowChannelDisableNotify(91005, 60))
}

func TestChannelBreakerHalfOpenRestoresWithMixedMajoritySuccess(t *testing.T) {
	common.SetChannelBreakerEnabled(true)
	disableRedisForBreakerTest(t)
	oldCooldown := channelBreakerCooldown
	channelBreakerCooldown = time.Millisecond
	t.Cleanup(func() {
		common.SetChannelBreakerEnabled(false)
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
	common.SetChannelBreakerEnabled(true)
	disableRedisForBreakerTest(t)
	oldCooldown := channelBreakerCooldown
	channelBreakerCooldown = time.Millisecond
	t.Cleanup(func() {
		common.SetChannelBreakerEnabled(false)
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

func TestChannelBreakerIndependentFromAutomaticDisableSwitch(t *testing.T) {
	common.SetAutomaticDisableChannelEnabled(false)
	common.SetChannelBreakerEnabled(true)
	disableRedisForBreakerTest(t)
	t.Cleanup(func() {
		common.SetAutomaticDisableChannelEnabled(false)
		common.SetChannelBreakerEnabled(false)
	})

	c := testBreakerContext("/v1/chat/completions")
	channelError := types.ChannelError{ChannelId: 1008, UsingKey: "key-a", AutoBan: true}
	ClearChannelBreaker(channelError)

	for i := 0; i < GetChannelBreakerFailureThreshold(); i++ {
		RecordChannelBreakerFailure(c, channelError, true)
	}

	require.False(t, AllowChannelByBreaker(c, channelError))
}

func TestChannelBreakerDisabledSwitchDoesNotBlockChannel(t *testing.T) {
	common.SetChannelBreakerEnabled(false)
	disableRedisForBreakerTest(t)

	c := testBreakerContext("/v1/chat/completions")
	channelError := types.ChannelError{ChannelId: 1009, UsingKey: "key-a", AutoBan: true}
	ClearChannelBreaker(channelError)

	for i := 0; i < GetChannelBreakerFailureThreshold(); i++ {
		opened, message := RecordChannelBreakerFailure(c, channelError, true)
		require.False(t, opened)
		require.Empty(t, message)
	}

	require.True(t, AllowChannelByBreaker(c, channelError))
}

func TestListAndClearChannelBreakerStatuses(t *testing.T) {
	common.SetChannelBreakerEnabled(true)
	disableRedisForBreakerTest(t)
	t.Cleanup(func() { common.SetChannelBreakerEnabled(false) })

	c := testBreakerContext("/v1/chat/completions")
	channelError := types.ChannelError{ChannelId: 1010, UsingKey: "key-a", AutoBan: true}
	ClearChannelBreaker(channelError)

	for i := 0; i < GetChannelBreakerFailureThreshold(); i++ {
		RecordChannelBreakerFailure(c, channelError, true)
	}

	var matched *ChannelBreakerStatus
	for _, status := range ListChannelBreakerStatuses() {
		if status.ChannelId == 1010 && status.KeyHash == ChannelBreakerKeyHash("key-a") {
			statusCopy := status
			matched = &statusCopy
			break
		}
	}
	require.NotNil(t, matched)
	require.Equal(t, ChannelBreakerStateOpen, matched.State)
	require.True(t, ClearChannelBreakerByStateKey(matched.StateKey))
	for _, status := range ListChannelBreakerStatuses() {
		require.NotEqual(t, matched.StateKey, status.StateKey)
	}
}

func TestChannelBreakerGroupRuleOverridesDefault(t *testing.T) {
	common.SetChannelBreakerEnabled(true)
	common.SetChannelBreakerRules([]common.ChannelBreakerRule{
		{
			Id:                "vip-rule",
			Name:              "VIP规则",
			Enabled:           true,
			Scope:             common.ChannelBreakerScopeGroup,
			Targets:           []string{"vip"},
			FailureLimit:      2,
			CooldownSeconds:   60,
			ProbeCount:        5,
			ProbeSuccessCount: 3,
		},
	})
	disableRedisForBreakerTest(t)
	t.Cleanup(func() {
		common.SetChannelBreakerEnabled(false)
		common.SetChannelBreakerRules(nil)
	})

	c := testBreakerContext("/v1/chat/completions")
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "vip")
	channelError := types.ChannelError{ChannelId: 1011, UsingKey: "key-a", AutoBan: true}
	ClearChannelBreaker(channelError)

	opened, _ := RecordChannelBreakerFailure(c, channelError, true)
	require.False(t, opened)
	opened, _ = RecordChannelBreakerFailure(c, channelError, true)
	require.True(t, opened)

	var matched *ChannelBreakerStatus
	for _, status := range ListChannelBreakerStatuses() {
		if status.ChannelId == 1011 {
			statusCopy := status
			matched = &statusCopy
			break
		}
	}
	require.NotNil(t, matched)
	require.Equal(t, "vip-rule", matched.RuleId)
	require.Equal(t, "vip", matched.Group)
}

func TestChannelBreakerProbeUsesStateRuleAfterConfigChange(t *testing.T) {
	common.SetChannelBreakerEnabled(true)
	common.SetChannelBreakerRules([]common.ChannelBreakerRule{
		{
			Id:                "vip-rule",
			Name:              "VIP规则",
			Enabled:           true,
			Scope:             common.ChannelBreakerScopeGroup,
			Targets:           []string{"vip"},
			FailureLimit:      1,
			CooldownSeconds:   1,
			ProbeCount:        2,
			ProbeSuccessCount: 2,
		},
	})
	disableRedisForBreakerTest(t)
	oldCooldown := channelBreakerCooldown
	channelBreakerCooldown = time.Millisecond
	t.Cleanup(func() {
		common.SetChannelBreakerEnabled(false)
		common.SetChannelBreakerRules(nil)
		channelBreakerCooldown = oldCooldown
	})

	c := testBreakerContext("/v1/chat/completions")
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "vip")
	channelError := types.ChannelError{ChannelId: 1013, UsingKey: "key-a", AutoBan: true}
	ClearChannelBreaker(channelError)

	opened, _ := RecordChannelBreakerFailure(c, channelError, true)
	require.True(t, opened)
	rule := resolveChannelBreakerRule(c, channelError)
	normalized, ok := normalizeChannelBreakerTarget(channelError, rule)
	require.True(t, ok)
	setBreakerOpenedAtForTest(channelBreakerStateKey(c, normalized, rule), time.Now().Add(-2*time.Second))

	common.SetChannelBreakerRules([]common.ChannelBreakerRule{
		{
			Id:                "vip-rule",
			Name:              "VIP规则",
			Enabled:           true,
			Scope:             common.ChannelBreakerScopeGroup,
			Targets:           []string{"vip"},
			FailureLimit:      1,
			CooldownSeconds:   1,
			ProbeCount:        5,
			ProbeSuccessCount: 5,
		},
	})

	for i := 0; i < 2; i++ {
		require.True(t, AllowChannelByBreaker(c, channelError))
		RecordChannelBreakerSuccess(c, channelError)
	}
	require.True(t, AllowChannelByBreaker(c, channelError))
}

func setBreakerOpenedAtForTest(key string, openedAt time.Time) {
	channelBreakerMu.Lock()
	defer channelBreakerMu.Unlock()

	state := loadChannelBreakerStateLocked(key)
	if state == nil {
		return
	}
	state.OpenedAt = openedAt
	saveChannelBreakerStateLocked(key, state)
}

func TestChannelBreakerStaleHalfOpenReopens(t *testing.T) {
	common.SetChannelBreakerEnabled(true)
	disableRedisForBreakerTest(t)
	oldCooldown := channelBreakerCooldown
	oldProbeTimeoutMin := channelBreakerProbeTimeoutMin
	channelBreakerCooldown = time.Millisecond
	channelBreakerProbeTimeoutMin = time.Millisecond
	t.Cleanup(func() {
		common.SetChannelBreakerEnabled(false)
		channelBreakerCooldown = oldCooldown
		channelBreakerProbeTimeoutMin = oldProbeTimeoutMin
	})

	c := testBreakerContext("/v1/chat/completions")
	channelError := types.ChannelError{ChannelId: 1014, UsingKey: "key-a", AutoBan: true}
	ClearChannelBreaker(channelError)

	for i := 0; i < GetChannelBreakerFailureThreshold(); i++ {
		RecordChannelBreakerFailure(c, channelError, true)
	}
	time.Sleep(2 * time.Millisecond)

	require.True(t, AllowChannelByBreaker(c, channelError))
	time.Sleep(2 * time.Millisecond)
	require.False(t, CanUseChannelByBreaker(c, channelError))

	var matched *ChannelBreakerStatus
	for _, status := range ListChannelBreakerStatuses() {
		if status.ChannelId == 1014 {
			statusCopy := status
			matched = &statusCopy
			break
		}
	}
	require.NotNil(t, matched)
	require.Equal(t, ChannelBreakerStateOpen, matched.State)
	require.Zero(t, matched.ProbeInFlight)
	require.Zero(t, matched.ProbeTotal)
}

func TestChannelBreakerModelRuleCanDisableBreaker(t *testing.T) {
	common.SetChannelBreakerEnabled(true)
	common.SetChannelBreakerRules([]common.ChannelBreakerRule{
		{
			Id:             "video-disabled",
			Name:           "视频模型不熔断",
			Enabled:        true,
			Scope:          common.ChannelBreakerScopeModel,
			Targets:        []string{"grok-imagine-video"},
			DisableBreaker: true,
		},
	})
	disableRedisForBreakerTest(t)
	t.Cleanup(func() {
		common.SetChannelBreakerEnabled(false)
		common.SetChannelBreakerRules(nil)
	})

	c := testBreakerContext("/v1/chat/completions")
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "grok-imagine-video")
	channelError := types.ChannelError{ChannelId: 1012, UsingKey: "key-a", AutoBan: true}
	ClearChannelBreaker(channelError)

	for i := 0; i < GetChannelBreakerFailureThreshold()+2; i++ {
		opened, message := RecordChannelBreakerFailure(c, channelError, true)
		require.False(t, opened)
		require.Empty(t, message)
	}
	require.True(t, AllowChannelByBreaker(c, channelError))
}

func TestBuildChannelBreakerBarkBodyIncludesRequestDetails(t *testing.T) {
	body := buildChannelBreakerBarkBody(ChannelBreakerNotificationContext{
		ChannelError: types.ChannelError{
			ChannelId:   1020,
			ChannelName: "primary-openai",
			ChannelType: constant.ChannelTypeOpenAI,
			UsingKey:    "sk-test-key",
		},
		Reason:      "upstream returned 429",
		ModelName:   "gpt-4o-mini",
		Group:       "vip",
		UserId:      88,
		Username:    "alice",
		TokenId:     99,
		RequestPath: "/v1/chat/completions",
		StatusCode:  429,
	})

	require.Contains(t, body, "渠道：primary-openai (#1020)")
	require.Contains(t, body, "类型：OpenAI")
	require.Contains(t, body, "模型：gpt-4o-mini")
	require.Contains(t, body, "分组：vip")
	require.Contains(t, body, "用户：alice (#88)")
	require.Contains(t, body, "令牌：#99")
	require.Contains(t, body, "路径：/v1/chat/completions")
	require.Contains(t, body, "状态码：429")
	require.Contains(t, body, "密钥哈希："+ChannelBreakerKeyHash("sk-test-key"))
	require.Contains(t, body, "原因：upstream returned 429")
}

func TestLowBalanceThresholdQuotaUsesCNYThreshold(t *testing.T) {
	oldQuotaPerUnit := common.QuotaPerUnit
	oldExchangeRate := operation_setting.USDExchangeRate
	common.QuotaPerUnit = 500000
	operation_setting.USDExchangeRate = 7.3
	t.Cleanup(func() {
		common.QuotaPerUnit = oldQuotaPerUnit
		operation_setting.USDExchangeRate = oldExchangeRate
	})

	require.Equal(t, 684932, lowBalanceThresholdQuota(10))
	require.Zero(t, lowBalanceThresholdQuota(0))
}

func TestShouldInstantDisableChannel(t *testing.T) {
	common.SetChannelBreakerEnabled(true)
	common.SetChannelBreakerRules([]common.ChannelBreakerRule{
		{
			Id:                        "instant-balance",
			Name:                      "上游余额不足立即禁用",
			Enabled:                   true,
			Scope:                     common.ChannelBreakerScopeGlobal,
			InstantDisableEnabled:     true,
			InstantDisableStatusCodes: "403",
			InstantDisableKeywords:    "insufficient account balance\n预扣费额度失败",
		},
	})
	disableRedisForBreakerTest(t)
	t.Cleanup(func() {
		common.SetChannelBreakerEnabled(false)
		common.SetChannelBreakerRules(nil)
	})

	c := testBreakerContext("/v1/chat/completions")
	channelError := types.ChannelError{ChannelId: 2001, UsingKey: "key-a", AutoBan: true}

	// 上游返回的 403 余额不足：状态码 AND 关键词命中 -> 立即禁用
	upstreamErr := types.NewErrorWithStatusCode(
		errors.New("Insufficient account balance"), types.ErrorCodeBadResponse, http.StatusForbidden)
	should, _, rule := shouldInstantDisableChannel(c, channelError, upstreamErr)
	require.True(t, should)
	require.Equal(t, "instant-balance", rule.Id)

	// 关键安全约束：我方预扣费/用户额度不足带 skipRetry，绝不能触发禁用
	ownQuotaErr := types.NewErrorWithStatusCode(
		errors.New("预扣费额度失败, 用户剩余额度: ¥0.06"), types.ErrorCodeInsufficientUserQuota,
		http.StatusForbidden, types.ErrOptionWithSkipRetry())
	should, _, _ = shouldInstantDisableChannel(c, channelError, ownQuotaErr)
	require.False(t, should)

	// AND 语义：状态码命中但关键词未命中 -> 不触发
	otherErr := types.NewErrorWithStatusCode(
		errors.New("Permission denied"), types.ErrorCodeBadResponse, http.StatusForbidden)
	should, _, _ = shouldInstantDisableChannel(c, channelError, otherErr)
	require.False(t, should)

	// 关键词命中但状态码不在范围 -> 不触发
	wrongCodeErr := types.NewErrorWithStatusCode(
		errors.New("Insufficient account balance"), types.ErrorCodeBadResponse, http.StatusTooManyRequests)
	should, _, _ = shouldInstantDisableChannel(c, channelError, wrongCodeErr)
	require.False(t, should)

	// 余额耗尽是终态：即使渠道未开启 AutoBan 也强制立即禁用
	noBan := types.ChannelError{ChannelId: 2001, UsingKey: "key-a", AutoBan: false}
	should, _, _ = shouldInstantDisableChannel(c, noBan, upstreamErr)
	require.True(t, should)
}

// TestInstantDisableBuiltinDefault 覆盖生产默认态：
// 全局熔断开关 OFF、且没有任何自定义规则时，纯内置规则仍应对 403 余额不足生效。
func TestInstantDisableBuiltinDefault(t *testing.T) {
	common.SetChannelBreakerEnabled(false)
	common.SetChannelBreakerRules(nil)
	disableRedisForBreakerTest(t)

	c := testBreakerContext("/v1/chat/completions")
	channelError := types.ChannelError{ChannelId: 3001, UsingKey: "key-a", AutoBan: false}

	upstreamErr := types.NewErrorWithStatusCode(
		errors.New("Insufficient account balance"), types.ErrorCodeBadResponse, http.StatusForbidden)
	should, _, rule := shouldInstantDisableChannel(c, channelError, upstreamErr)
	require.True(t, should)
	require.Equal(t, "global-default", rule.Id)

	// 我方扣费失败带 skipRetry，内置规则也不能误触发
	ownQuotaErr := types.NewErrorWithStatusCode(
		errors.New("预扣费额度失败, 用户剩余额度: ¥0.06"), types.ErrorCodeInsufficientUserQuota,
		http.StatusForbidden, types.ErrOptionWithSkipRetry())
	should, _, _ = shouldInstantDisableChannel(c, channelError, ownQuotaErr)
	require.False(t, should)

	// 结构化兜底：我方额度错误即使漏标 skipRetry（NewAPIError + 内部额度码），仍须排除
	ownQuotaNoSkip := types.NewErrorWithStatusCode(
		errors.New("预扣费额度失败, 用户剩余额度: ¥0.06"), types.ErrorCodeInsufficientUserQuota,
		http.StatusForbidden)
	should, _, _ = shouldInstantDisableChannel(c, channelError, ownQuotaNoSkip)
	require.False(t, should)

	// 上游本身是 new-api、账号没钱：错误码同为 insufficient_user_quota，但 type 是上游错误，
	// 不应被结构化兜底误伤，仍须正常立即禁用
	upstreamNewApiErr := types.WithOpenAIError(types.OpenAIError{
		Message: "预扣费额度失败, 用户剩余额度: ¥0",
		Type:    string(types.ErrorCodeInsufficientUserQuota),
		Code:    string(types.ErrorCodeInsufficientUserQuota),
	}, http.StatusForbidden)
	should, _, _ = shouldInstantDisableChannel(c, channelError, upstreamNewApiErr)
	require.True(t, should)
}

func TestInstantDisableTriggersOnFirstHit(t *testing.T) {
	common.SetChannelBreakerEnabled(true)
	common.SetChannelBreakerRules([]common.ChannelBreakerRule{
		{
			Id:                        "instant-balance",
			Name:                      "上游余额不足立即禁用",
			Enabled:                   true,
			Scope:                     common.ChannelBreakerScopeGlobal,
			FailureLimit:              5,
			InstantDisableEnabled:     true,
			InstantDisableStatusCodes: "403",
			InstantDisableKeywords:    "insufficient account balance",
		},
	})
	disableRedisForBreakerTest(t)
	t.Cleanup(func() {
		common.SetChannelBreakerEnabled(false)
		common.SetChannelBreakerRules(nil)
	})

	c := testBreakerContext("/v1/chat/completions")
	channelError := types.ChannelError{ChannelId: 2002, UsingKey: "key-a", AutoBan: true}
	ClearChannelBreaker(channelError)

	balanceErr := types.NewErrorWithStatusCode(
		errors.New("Insufficient account balance"), types.ErrorCodeBadResponse, http.StatusForbidden)

	// 第一次命中即判定立即禁用，无需累加到 FailureLimit(5)
	should, _, _ := shouldInstantDisableChannel(c, channelError, balanceErr)
	require.True(t, should, "余额不足应在第一次命中即触发立即禁用")

	// 对照：同样的错误若走普通熔断计数，第一次只会 pending、不会 opened
	opened, _ := RecordChannelBreakerFailure(c, channelError, true)
	require.False(t, opened, "普通熔断第一次不会打开，需累加到阈值")
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
