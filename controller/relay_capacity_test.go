package controller

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setChannelCapacityModeForTest(t *testing.T, mode string) {
	t.Helper()
	cfg := config.GlobalConfig.Get("channel_capacity_setting")
	require.NotNil(t, cfg, "channel_capacity_setting 必须已注册")
	config.UpdateConfigFromMap(cfg, map[string]string{"mode": mode})
	t.Cleanup(func() {
		config.UpdateConfigFromMap(cfg, map[string]string{"mode": "off"})
	})
}

func useCapacityMiniRedisForRelayTest(t *testing.T) {
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
}

func newCapacityTestChannel(id int, capacity *dto.ChannelCapacitySettings) *model.Channel {
	channel := &model.Channel{Id: id}
	channel.SetSetting(dto.ChannelSettings{Capacity: capacity})
	return channel
}

func TestAcquireChannelCapacityForAttempt(t *testing.T) {
	useCapacityMiniRedisForRelayTest(t)
	gin.SetMode(gin.TestMode)
	newTestContext := func() *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
		return c
	}
	limited := &dto.ChannelCapacitySettings{RPM: 10, MaxConcurrency: 5}

	t.Run("off 模式不产生任何 Redis 记录", func(t *testing.T) {
		setChannelCapacityModeForTest(t, "off")
		lease := acquireChannelCapacityForAttempt(newTestContext(), types.RelayFormatOpenAI, newCapacityTestChannel(201, limited), 0)
		assert.Nil(t, lease)
		assert.Empty(t, common.RDB.Keys(context.Background(), "new-api:channel-capacity:*").Val())
	})

	t.Run("Realtime WebSocket 在 Stage C 前不接入", func(t *testing.T) {
		setChannelCapacityModeForTest(t, "shadow")
		lease := acquireChannelCapacityForAttempt(newTestContext(), types.RelayFormatOpenAIRealtime, newCapacityTestChannel(202, limited), 0)
		assert.Nil(t, lease)
	})

	t.Run("未配置限额的渠道走快路径", func(t *testing.T) {
		setChannelCapacityModeForTest(t, "shadow")
		lease := acquireChannelCapacityForAttempt(newTestContext(), types.RelayFormatOpenAI, newCapacityTestChannel(203, nil), 0)
		assert.Nil(t, lease)
		assert.Empty(t, common.RDB.Keys(context.Background(), "new-api:channel-capacity:*").Val())
	})

	t.Run("shadow 模式记录真实计数并返回租约", func(t *testing.T) {
		setChannelCapacityModeForTest(t, "shadow")
		c := newTestContext()
		lease := acquireChannelCapacityForAttempt(c, types.RelayFormatOpenAI, newCapacityTestChannel(204, limited), 0)
		require.NotNil(t, lease)

		rpm := common.RDB.ZCard(context.Background(), "new-api:channel-capacity:v1:{channel:204}:rpm").Val()
		inflight := common.RDB.ZCard(context.Background(), "new-api:channel-capacity:v1:{channel:204}:inflight").Val()
		assert.Equal(t, int64(1), rpm)
		assert.Equal(t, int64(1), inflight)

		require.NoError(t, lease.Release(c.Request.Context()))
		inflight = common.RDB.ZCard(context.Background(), "new-api:channel-capacity:v1:{channel:204}:inflight").Val()
		assert.Zero(t, inflight, "释放后并发名额归零")
		rpm = common.RDB.ZCard(context.Background(), "new-api:channel-capacity:v1:{channel:204}:rpm").Val()
		assert.Equal(t, int64(1), rpm, "RPM 记录保留到窗口自然过期")
	})

	t.Run("enforce 在 Stage B 前仍按 shadow 记录且不拒绝", func(t *testing.T) {
		setChannelCapacityModeForTest(t, "enforce")
		tight := &dto.ChannelCapacitySettings{RPM: 1}
		for i := 0; i < 3; i++ {
			c := newTestContext()
			lease := acquireChannelCapacityForAttempt(c, types.RelayFormatOpenAI, newCapacityTestChannel(205, tight), i)
			require.NotNil(t, lease, "Stage A 不允许出现任何拒绝路径")
			_ = lease.Release(c.Request.Context())
		}
		rpm := common.RDB.ZCard(context.Background(), "new-api:channel-capacity:v1:{channel:205}:rpm").Val()
		assert.Equal(t, int64(3), rpm)
	})
}
