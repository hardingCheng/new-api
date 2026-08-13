package service

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func useChannelCapacityMiniRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	previousRedisEnabled := common.RedisEnabled
	previousRedisClient := common.RDB
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	require.NoError(t, redisClient.Ping(context.Background()).Err())
	common.RedisEnabled = true
	common.RDB = redisClient
	t.Cleanup(func() {
		_ = redisClient.Close()
		common.RedisEnabled = previousRedisEnabled
		common.RDB = previousRedisClient
	})
	return redisServer
}

func disableRedisForCapacityTest(t *testing.T) {
	t.Helper()
	previousRedisEnabled := common.RedisEnabled
	previousRedisClient := common.RDB
	common.RedisEnabled = false
	common.RDB = nil
	t.Cleanup(func() {
		common.RedisEnabled = previousRedisEnabled
		common.RDB = previousRedisClient
	})
}

func acquireCapacity(t *testing.T, channelID int, capacity *dto.ChannelCapacitySettings, mode string) ChannelCapacityDecision {
	t.Helper()
	attemptID := BuildChannelCapacityAttemptID(t.Name(), 0)
	return TryAcquireChannelCapacity(context.Background(), channelID, capacity, mode, attemptID)
}

// capacityTestBaseTime 固定测试时钟基准；容量窗口完全依赖 Redis TIME，
// miniredis 需用 SetTime 显式推进（FastForward 只影响 key TTL，不影响 TIME）。
var capacityTestBaseTime = time.Unix(1_700_000_000, 0)

func TestChannelCapacityRPMSlidingWindowEnforce(t *testing.T) {
	redisServer := useChannelCapacityMiniRedis(t)
	redisServer.SetTime(capacityTestBaseTime)
	capacity := &dto.ChannelCapacitySettings{RPM: 3}

	for i := 0; i < 3; i++ {
		decision := acquireCapacity(t, 101, capacity, operation_setting.ChannelCapacityModeEnforce)
		require.True(t, decision.Allowed, "attempt %d should pass", i+1)
		assert.False(t, decision.WouldBlock)
	}

	rejected := acquireCapacity(t, 101, capacity, operation_setting.ChannelCapacityModeEnforce)
	require.False(t, rejected.Allowed)
	assert.Equal(t, ChannelCapacityReasonRPM, rejected.Reason)
	assert.GreaterOrEqual(t, rejected.RetryAfterMs, int64(1000))
	assert.LessOrEqual(t, rejected.RetryAfterMs, int64(60000))
	assert.Equal(t, int64(3), rejected.RPMUsed)

	// 拒绝时不写入任何 member
	assert.Equal(t, 3, zsetLen(t, channelCapacityRPMKey(101)))

	// 最旧记录跨过 60 秒窗口后，下一次立即通过
	redisServer.SetTime(capacityTestBaseTime.Add(61 * time.Second))
	allowed := acquireCapacity(t, 101, capacity, operation_setting.ChannelCapacityModeEnforce)
	require.True(t, allowed.Allowed)
}

func TestChannelCapacityShadowAlwaysRecordsAndReportsWouldBlock(t *testing.T) {
	useChannelCapacityMiniRedis(t)
	capacity := &dto.ChannelCapacitySettings{RPM: 1}

	first := acquireCapacity(t, 102, capacity, operation_setting.ChannelCapacityModeShadow)
	require.True(t, first.Allowed)
	assert.False(t, first.WouldBlock)

	// shadow 达限后仍放行、仍写入真实计数，否则管理员看不到实际超量多少
	second := acquireCapacity(t, 102, capacity, operation_setting.ChannelCapacityModeShadow)
	require.True(t, second.Allowed)
	assert.True(t, second.WouldBlock)
	assert.Equal(t, ChannelCapacityReasonRPM, second.Reason)
	assert.Equal(t, int64(1), second.RPMUsed)

	third := acquireCapacity(t, 102, capacity, operation_setting.ChannelCapacityModeShadow)
	require.True(t, third.Allowed)
	assert.True(t, third.WouldBlock)
	assert.Equal(t, int64(2), third.RPMUsed, "shadow 计数必须继续增长，不能停在 limit")

	assert.Equal(t, 3, zsetLen(t, channelCapacityRPMKey(102)))
}

func TestChannelCapacityConcurrencyLeaseAcquireRelease(t *testing.T) {
	useChannelCapacityMiniRedis(t)
	capacity := &dto.ChannelCapacitySettings{MaxConcurrency: 2}

	first := acquireCapacity(t, 103, capacity, operation_setting.ChannelCapacityModeEnforce)
	require.True(t, first.Allowed)
	second := acquireCapacity(t, 103, capacity, operation_setting.ChannelCapacityModeEnforce)
	require.True(t, second.Allowed)

	rejected := acquireCapacity(t, 103, capacity, operation_setting.ChannelCapacityModeEnforce)
	require.False(t, rejected.Allowed)
	assert.Equal(t, ChannelCapacityReasonConcurrency, rejected.Reason)
	assert.Equal(t, int64(2), rejected.InflightUsed)

	// 只配并发时不应产生 RPM 记录
	assert.Equal(t, 0, zsetLen(t, channelCapacityRPMKey(103)))

	first.Lease.MarkDispatched()
	require.NoError(t, first.Lease.Release(context.Background()))

	next := acquireCapacity(t, 103, capacity, operation_setting.ChannelCapacityModeEnforce)
	require.True(t, next.Allowed, "释放一个并发名额后应立即可通过")
}

func TestChannelCapacityCancelBeforeDispatchRollsBackBothDimensions(t *testing.T) {
	useChannelCapacityMiniRedis(t)
	capacity := &dto.ChannelCapacitySettings{RPM: 1, MaxConcurrency: 1}

	decision := acquireCapacity(t, 104, capacity, operation_setting.ChannelCapacityModeEnforce)
	require.True(t, decision.Allowed)
	require.NoError(t, decision.Lease.CancelBeforeDispatch(context.Background()))

	assert.Equal(t, 0, zsetLen(t, channelCapacityRPMKey(104)))
	assert.Equal(t, 0, zsetLen(t, channelCapacityInflightKey(104)))

	again := acquireCapacity(t, 104, capacity, operation_setting.ChannelCapacityModeEnforce)
	require.True(t, again.Allowed, "撤销后的名额必须完整回退")
}

func TestChannelCapacityReleaseAfterDispatchKeepsRPM(t *testing.T) {
	useChannelCapacityMiniRedis(t)
	capacity := &dto.ChannelCapacitySettings{RPM: 2, MaxConcurrency: 2}

	decision := acquireCapacity(t, 105, capacity, operation_setting.ChannelCapacityModeEnforce)
	require.True(t, decision.Allowed)
	decision.Lease.MarkDispatched()
	require.NoError(t, decision.Lease.Release(context.Background()))

	assert.Equal(t, 1, zsetLen(t, channelCapacityRPMKey(105)), "Release 不撤销 RPM")
	assert.Equal(t, 0, zsetLen(t, channelCapacityInflightKey(105)))

	// 已派发后 CancelBeforeDispatch 必须降级为 Release，不得撤销 RPM
	dispatched := acquireCapacity(t, 105, capacity, operation_setting.ChannelCapacityModeEnforce)
	require.True(t, dispatched.Allowed)
	dispatched.Lease.MarkDispatched()
	require.NoError(t, dispatched.Lease.CancelBeforeDispatch(context.Background()))
	assert.Equal(t, 2, zsetLen(t, channelCapacityRPMKey(105)))

	rejected := acquireCapacity(t, 105, capacity, operation_setting.ChannelCapacityModeEnforce)
	require.False(t, rejected.Allowed)
	assert.Equal(t, ChannelCapacityReasonRPM, rejected.Reason)
}

func TestChannelCapacityLeaseExpiryReclaimsSlot(t *testing.T) {
	redisServer := useChannelCapacityMiniRedis(t)
	redisServer.SetTime(capacityTestBaseTime)
	capacity := &dto.ChannelCapacitySettings{MaxConcurrency: 1}

	held := acquireCapacity(t, 106, capacity, operation_setting.ChannelCapacityModeEnforce)
	require.True(t, held.Allowed)

	blocked := acquireCapacity(t, 106, capacity, operation_setting.ChannelCapacityModeEnforce)
	require.False(t, blocked.Allowed)

	// 持有者崩溃（不续租不释放）：租约 90 秒后自动过期
	redisServer.SetTime(capacityTestBaseTime.Add(91 * time.Second))
	reclaimed := acquireCapacity(t, 106, capacity, operation_setting.ChannelCapacityModeEnforce)
	require.True(t, reclaimed.Allowed, "崩溃残留的名额必须在租约 TTL 后回收")
}

func TestChannelCapacityRenewDoesNotResurrectReleasedMember(t *testing.T) {
	useChannelCapacityMiniRedis(t)
	capacity := &dto.ChannelCapacitySettings{MaxConcurrency: 1}

	attemptID := BuildChannelCapacityAttemptID("renew-race", 0)
	decision := TryAcquireChannelCapacity(context.Background(), 107, capacity, operation_setting.ChannelCapacityModeEnforce, attemptID)
	require.True(t, decision.Allowed)
	decision.Lease.MarkDispatched()
	require.NoError(t, decision.Lease.Release(context.Background()))

	// 模拟迟到的续租：member 已释放时必须返回 0 且不重建
	alive, err := channelCapacityRenewScript.Run(context.Background(), common.RDB,
		[]string{channelCapacityInflightKey(107)},
		attemptID, channelCapacityLeaseTTLMs, channelCapacityInflightTTLMs).Int64()
	require.NoError(t, err)
	assert.Equal(t, int64(0), alive)
	assert.Equal(t, 0, zsetLen(t, channelCapacityInflightKey(107)))
}

func TestChannelCapacityRenewExtendsLease(t *testing.T) {
	redisServer := useChannelCapacityMiniRedis(t)
	redisServer.SetTime(capacityTestBaseTime)
	capacity := &dto.ChannelCapacitySettings{MaxConcurrency: 1}

	attemptID := BuildChannelCapacityAttemptID("renew-extend", 0)
	decision := TryAcquireChannelCapacity(context.Background(), 108, capacity, operation_setting.ChannelCapacityModeEnforce, attemptID)
	require.True(t, decision.Allowed)

	// 长流场景：60 秒后续租一次，租约应从当前时间重新延长 90 秒
	redisServer.SetTime(capacityTestBaseTime.Add(60 * time.Second))
	alive, err := channelCapacityRenewScript.Run(context.Background(), common.RDB,
		[]string{channelCapacityInflightKey(108)},
		attemptID, channelCapacityLeaseTTLMs, channelCapacityInflightTTLMs).Int64()
	require.NoError(t, err)
	require.Equal(t, int64(1), alive)

	// 原始租约 90 秒边界已过，但续租后的名额仍被占用
	redisServer.SetTime(capacityTestBaseTime.Add(100 * time.Second))
	blocked := acquireCapacity(t, 108, capacity, operation_setting.ChannelCapacityModeEnforce)
	require.False(t, blocked.Allowed, "续租后的租约在原 TTL 过后必须仍然有效")
}

func TestChannelCapacityNoLimitSkipsRedis(t *testing.T) {
	useChannelCapacityMiniRedis(t)

	for _, capacity := range []*dto.ChannelCapacitySettings{nil, {}} {
		decision := acquireCapacity(t, 109, capacity, operation_setting.ChannelCapacityModeEnforce)
		require.True(t, decision.Allowed)
		require.NotNil(t, decision.Lease)
	}

	keys := common.RDB.Keys(context.Background(), "new-api:channel-capacity:*").Val()
	assert.Empty(t, keys, "未配置限额的渠道必须走零 Redis 开销快路径")
}

func TestChannelCapacityModeOffBypasses(t *testing.T) {
	useChannelCapacityMiniRedis(t)
	capacity := &dto.ChannelCapacitySettings{RPM: 1, MaxConcurrency: 1}

	for i := 0; i < 3; i++ {
		decision := acquireCapacity(t, 110, capacity, operation_setting.ChannelCapacityModeOff)
		require.True(t, decision.Allowed)
		assert.False(t, decision.WouldBlock)
	}
	keys := common.RDB.Keys(context.Background(), "new-api:channel-capacity:*").Val()
	assert.Empty(t, keys)
}

func TestChannelCapacityRedisUnavailableFailModes(t *testing.T) {
	disableRedisForCapacityTest(t)
	capacity := &dto.ChannelCapacitySettings{RPM: 1}

	// shadow：fail open，不影响客户请求
	shadow := acquireCapacity(t, 111, capacity, operation_setting.ChannelCapacityModeShadow)
	require.True(t, shadow.Allowed)
	assert.Equal(t, ChannelCapacityReasonRedisError, shadow.Reason)

	// enforce：fail closed，否则 Redis 故障时全部流量直接打到上游
	enforce := acquireCapacity(t, 111, capacity, operation_setting.ChannelCapacityModeEnforce)
	require.False(t, enforce.Allowed)
	assert.Equal(t, ChannelCapacityReasonRedisError, enforce.Reason)

	// 拿到的 noop lease 三个方法都必须安全幂等
	require.NoError(t, shadow.Lease.Release(context.Background()))
	require.NoError(t, shadow.Lease.CancelBeforeDispatch(context.Background()))
}

func TestChannelCapacityConcurrentAcquireNoOversell(t *testing.T) {
	useChannelCapacityMiniRedis(t)
	const limit = 40
	const contenders = 200
	capacity := &dto.ChannelCapacitySettings{MaxConcurrency: limit}

	var wg sync.WaitGroup
	results := make([]bool, contenders)
	for i := 0; i < contenders; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			attemptID := BuildChannelCapacityAttemptID(fmt.Sprintf("storm-%d", idx), 0)
			decision := TryAcquireChannelCapacity(context.Background(), 112, capacity, operation_setting.ChannelCapacityModeEnforce, attemptID)
			results[idx] = decision.Allowed
		}(i)
	}
	wg.Wait()

	allowedCount := 0
	for _, allowed := range results {
		if allowed {
			allowedCount++
		}
	}
	assert.Equal(t, limit, allowedCount, "恰好 limit 个成功，不能超卖")
	assert.Equal(t, limit, zsetLen(t, channelCapacityInflightKey(112)))
}

func TestChannelCapacityLeaseMethodsAreIdempotent(t *testing.T) {
	useChannelCapacityMiniRedis(t)
	capacity := &dto.ChannelCapacitySettings{RPM: 5, MaxConcurrency: 5}

	decision := acquireCapacity(t, 113, capacity, operation_setting.ChannelCapacityModeEnforce)
	require.True(t, decision.Allowed)
	decision.Lease.MarkDispatched()
	require.NoError(t, decision.Lease.Release(context.Background()))
	require.NoError(t, decision.Lease.Release(context.Background()))
	require.NoError(t, decision.Lease.CancelBeforeDispatch(context.Background()))

	assert.Equal(t, 1, zsetLen(t, channelCapacityRPMKey(113)))
	assert.Equal(t, 0, zsetLen(t, channelCapacityInflightKey(113)))
}

func zsetLen(t *testing.T, key string) int {
	t.Helper()
	n, err := common.RDB.ZCard(context.Background(), key).Result()
	require.NoError(t, err)
	return int(n)
}
