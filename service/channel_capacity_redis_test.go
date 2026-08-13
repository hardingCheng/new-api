package service

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 真实 Redis 集成测试：验证 Lua 原子性、TIME 语义与 EVALSHA fallback。
// 通过 CHANNEL_CAPACITY_REDIS_TEST_URL 显式开启，例如：
//
//	CHANNEL_CAPACITY_REDIS_TEST_URL=redis://127.0.0.1:6379/15 go test ./service/ -run TestChannelCapacityRealRedis
func useChannelCapacityRealRedis(t *testing.T, channelIDs ...int) {
	t.Helper()
	rawURL := os.Getenv("CHANNEL_CAPACITY_REDIS_TEST_URL")
	if rawURL == "" {
		t.Skip("CHANNEL_CAPACITY_REDIS_TEST_URL not set")
	}
	opts, err := redis.ParseURL(rawURL)
	require.NoError(t, err)
	client := redis.NewClient(opts)
	require.NoError(t, client.Ping(context.Background()).Err())

	cleanupKeys := func() {
		for _, id := range channelIDs {
			client.Del(context.Background(),
				channelCapacityRPMKey(id), channelCapacityInflightKey(id), channelCapacityStatsKey(id))
		}
	}
	cleanupKeys()

	previousRedisEnabled := common.RedisEnabled
	previousRedisClient := common.RDB
	common.RedisEnabled = true
	common.RDB = client
	t.Cleanup(func() {
		cleanupKeys()
		_ = client.Close()
		common.RedisEnabled = previousRedisEnabled
		common.RDB = previousRedisClient
	})
}

func TestChannelCapacityRealRedisConcurrentNoOversell(t *testing.T) {
	const channelID = 987001
	useChannelCapacityRealRedis(t, channelID)

	const limit = 40
	const contenders = 200
	capacity := &dto.ChannelCapacitySettings{RPM: 100, MaxConcurrency: limit}

	var wg sync.WaitGroup
	results := make([]ChannelCapacityDecision, contenders)
	for i := 0; i < contenders; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			attemptID := BuildChannelCapacityAttemptID(fmt.Sprintf("real-storm-%d", idx), 0)
			results[idx] = TryAcquireChannelCapacity(context.Background(), channelID, capacity, operation_setting.ChannelCapacityModeEnforce, attemptID)
		}(i)
	}
	wg.Wait()

	allowedCount := 0
	for _, decision := range results {
		if decision.Allowed {
			allowedCount++
		}
	}
	assert.Equal(t, limit, allowedCount, "真实 Redis 下也必须恰好 limit 个成功")

	inflight, err := common.RDB.ZCard(context.Background(), channelCapacityInflightKey(channelID)).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(limit), inflight)

	// 释放全部租约后名额应完全归零
	for _, decision := range results {
		if decision.Allowed {
			decision.Lease.MarkDispatched()
			require.NoError(t, decision.Lease.Release(context.Background()))
		}
	}
	inflight, err = common.RDB.ZCard(context.Background(), channelCapacityInflightKey(channelID)).Result()
	require.NoError(t, err)
	assert.Zero(t, inflight)
}

func TestChannelCapacityRealRedisScriptCacheFlush(t *testing.T) {
	const channelID = 987002
	useChannelCapacityRealRedis(t, channelID)
	capacity := &dto.ChannelCapacitySettings{RPM: 10}

	first := acquireCapacity(t, channelID, capacity, operation_setting.ChannelCapacityModeEnforce)
	require.True(t, first.Allowed)

	// SCRIPT FLUSH 后 EVALSHA 必须自动 fallback 到 EVAL，不影响业务
	require.NoError(t, common.RDB.ScriptFlush(context.Background()).Err())
	second := acquireCapacity(t, channelID, capacity, operation_setting.ChannelCapacityModeEnforce)
	require.True(t, second.Allowed)
	assert.Equal(t, int64(1), second.RPMUsed)
}
