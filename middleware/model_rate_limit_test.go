package middleware

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelRedisRateLimitUsesUTCRegardlessOfLocalTimezone(t *testing.T) {
	redisServer, redisClient := useRateLimitMiniRedis(t)
	previousLocation := time.Local
	time.Local = time.FixedZone("test-utc-plus-eight", 8*60*60)
	t.Cleanup(func() { time.Local = previousLocation })

	ctx := context.Background()
	recordKey := "rateLimit:model-utc-record"
	recordRedisRequest(ctx, redisClient, recordKey, 2)
	recorded, err := redisClient.LIndex(ctx, recordKey, 0).Result()
	require.NoError(t, err)
	recordedAt, err := time.Parse(modelRateLimitTimeFormat, recorded)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now().UTC(), recordedAt, 2*time.Second)

	checkKey := "rateLimit:model-utc-check"
	withinWindow := time.Now().UTC().Add(-30 * time.Second).Format(modelRateLimitTimeFormat)
	_, err = redisServer.Push(checkKey, withinWindow, withinWindow)
	require.NoError(t, err)
	allowed, err := checkRedisRateLimit(ctx, redisClient, checkKey, 2, 60)
	require.NoError(t, err)
	assert.False(t, allowed, "an existing UTC timestamp inside the window must remain limited on a non-UTC host")
}

// 2026-07-30 生产事故的回归测试。
//
// 老版本按容器本地时区（Asia/Shanghai）取时间却仍写 Z 后缀，升级到按 UTC
// 记录的版本后，遗留条目会被解析成"未来 8 小时"，窗口计算得负值 → 永远判为
// 超限。而拒绝路径不写入新时间戳、每次拒绝还刷新 TTL，列表既不轮换也不过期，
// 于是列表满了的用户被永久拒死（当时主力客户 402/403 请求被打 429）。
func TestModelRedisRateLimitHealsLegacyLocalTimeEntries(t *testing.T) {
	redisServer, redisClient := useRateLimitMiniRedis(t)
	ctx := context.Background()
	key := "rateLimit:MRRLS:legacy"

	// 老版本写下的条目：本地时间（UTC+8）却带 Z 后缀，看起来像 8 小时后的未来
	legacy := time.Now().UTC().Add(8 * time.Hour).Format(modelRateLimitTimeFormat)
	_, err := redisServer.Push(key, legacy, legacy)
	require.NoError(t, err)
	require.NoError(t, redisClient.Expire(ctx, key, time.Minute).Err())

	allowed, err := checkRedisRateLimit(ctx, redisClient, key, 2, 60)
	require.NoError(t, err)
	assert.True(t, allowed, "遗留时间戳必须放行，否则用户被永久拒死")

	remaining, err := redisClient.LLen(ctx, key).Result()
	require.NoError(t, err)
	assert.Zero(t, remaining, "遗留列表要被丢弃，让它按新格式重建")
}

var modelRateLimitTestUsers atomic.Int64

func TestModelRateLimitStreamFailuresDoNotConsumeSuccessLimit(t *testing.T) {
	for _, backend := range []string{"memory", "redis"} {
		for _, totalLimit := range []int{0, 2} {
			t.Run(fmt.Sprintf("%s/total=%d", backend, totalLimit), func(t *testing.T) {
				userID := 7200000 + int(modelRateLimitTestUsers.Add(1))
				handler := memoryRateLimitHandler(60, totalLimit, 1)
				if backend == "redis" {
					useRateLimitMiniRedis(t)
					handler = redisRateLimitHandler(60, totalLimit, 1)
				}
				router := gin.New()
				router.GET("/:outcome", func(c *gin.Context) { c.Set("id", userID) }, handler, func(c *gin.Context) {
					status := relaycommon.NewStreamStatus()
					if c.Param("outcome") == "failed" {
						status.MarkFailed("server_error", "", 0)
					} else {
						status.MarkCompleted()
					}
					common.SetContextKey(c, constant.ContextKeyResponseStreamStatus, status)
					c.Status(http.StatusOK)
				})
				assert.Equal(t, http.StatusOK, performRateLimitRequest(router, "/failed", "127.0.0.1:1000").Code)
				if totalLimit > 0 {
					assert.Equal(t, http.StatusOK, performRateLimitRequest(router, "/failed", "127.0.0.1:1000").Code)
				} else {
					assert.Equal(t, http.StatusOK, performRateLimitRequest(router, "/completed", "127.0.0.1:1000").Code)
				}
				assert.Equal(t, http.StatusTooManyRequests, performRateLimitRequest(router, "/completed", "127.0.0.1:1000").Code)
			})
		}
	}
}

func TestModelMemoryRateLimitReservesConcurrentSuccessAdmission(t *testing.T) {
	userID := 7200000 + int(modelRateLimitTestUsers.Add(1))
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	router := gin.New()
	router.GET("/:outcome", func(c *gin.Context) { c.Set("id", userID) }, memoryRateLimitHandler(60, 0, 1), func(c *gin.Context) {
		if c.Param("outcome") == "failed" {
			close(entered)
			<-release
			status := relaycommon.NewStreamStatus()
			status.MarkFailed("server_error", "", 0)
			common.SetContextKey(c, constant.ContextKeyResponseStreamStatus, status)
		}
		c.Status(http.StatusOK)
	})
	go func() {
		defer close(finished)
		assert.Equal(t, http.StatusOK, performRateLimitRequest(router, "/failed", "127.0.0.1:1000").Code)
	}()
	<-entered
	assert.Equal(t, http.StatusTooManyRequests, performRateLimitRequest(router, "/completed", "127.0.0.1:1000").Code)
	close(release)
	<-finished
	assert.Equal(t, http.StatusOK, performRateLimitRequest(router, "/completed", "127.0.0.1:1000").Code)
	assert.Equal(t, http.StatusTooManyRequests, performRateLimitRequest(router, "/completed", "127.0.0.1:1000").Code)
}
