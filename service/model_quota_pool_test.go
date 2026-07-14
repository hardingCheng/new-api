package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSettleRelayModelQuotaPoolUsesActualQuotaAndTokens(t *testing.T) {
	info := &relaycommon.RelayInfo{
		ModelQuotaPools: []ratio_setting.ModelQuotaPoolMatch{
			{
				Metric:    ratio_setting.ModelQuotaPoolMetricQuota,
				RedisKey:  "pool:quota",
				Amount:    800,
				UsedAfter: 800,
				Remaining: 200,
			},
			{
				Metric:    ratio_setting.ModelQuotaPoolMetricTotalTokens,
				RedisKey:  "pool:tokens",
				Amount:    1000,
				UsedAfter: 1000,
				Remaining: 9000,
			},
			{
				Metric:    ratio_setting.ModelQuotaPoolMetricRequests,
				RedisKey:  "pool:requests",
				Amount:    1,
				UsedAfter: 1,
				Remaining: 9,
			},
		},
	}
	deltas := make(map[string]int64)

	settleRelayModelQuotaPoolWithAdjuster(info, ModelQuotaPoolSettlement{
		ActualQuota:          300,
		ActualTotalTokens:    120,
		HasActualQuota:       true,
		HasActualTotalTokens: true,
	}, func(key string, delta int64) bool {
		deltas[key] = delta
		return true
	})

	require.Len(t, deltas, 2)
	assert.Equal(t, int64(-500), deltas["pool:quota"])
	assert.Equal(t, int64(-880), deltas["pool:tokens"])
	assert.Equal(t, int64(300), info.ModelQuotaPools[0].Amount)
	assert.Equal(t, int64(120), info.ModelQuotaPools[1].Amount)
	assert.Equal(t, int64(1), info.ModelQuotaPools[2].Amount)
}

func TestSettleModelQuotaPoolDoesNotClaimSuccessWithoutDurability(t *testing.T) {
	oldDB := model.DB
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "closed.db")), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	model.DB = db
	t.Cleanup(func() { model.DB = oldDB })
	oldRedisEnabled, oldRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled, common.RDB = false, nil
	t.Cleanup(func() { common.RedisEnabled, common.RDB = oldRedisEnabled, oldRDB })

	info := &relaycommon.RelayInfo{
		RequestId: "pool-settlement-no-durability",
		ModelQuotaPools: []ratio_setting.ModelQuotaPoolMatch{{
			Metric:   ratio_setting.ModelQuotaPoolMetricTotalTokens,
			RedisKey: "pool:closed-db",
			Amount:   1000,
		}},
	}
	err = SettleModelQuotaPool(info, ModelQuotaPoolSettlement{
		ActualTotalTokens:    250,
		HasActualTotalTokens: true,
	})
	require.Error(t, err)
	assert.False(t, info.ModelQuotaPoolSettled)
	assert.Equal(t, int64(1000), info.ModelQuotaPools[0].Amount)
}

func TestSettleModelQuotaPoolFallsBackToIdempotentRedisApply(t *testing.T) {
	redisURL := os.Getenv("MODEL_QUOTA_POOL_REDIS_TEST_URL")
	if redisURL == "" {
		t.Skip("MODEL_QUOTA_POOL_REDIS_TEST_URL is not set")
	}
	options, err := redis.ParseURL(redisURL)
	require.NoError(t, err)
	client := redis.NewClient(options)
	require.NoError(t, client.Ping(context.Background()).Err())
	t.Cleanup(func() { _ = client.Close() })

	oldDB := model.DB
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "closed-fallback.db")), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	model.DB = db
	t.Cleanup(func() { model.DB = oldDB })
	oldRedisEnabled, oldRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled, common.RDB = true, client
	t.Cleanup(func() { common.RedisEnabled, common.RDB = oldRedisEnabled, oldRDB })

	poolKey := "test:pool:settlement-direct-fallback"
	poolDigest := sha256.Sum256([]byte(poolKey))
	operationKey := fmt.Sprintf("relay:settle:pool-direct-fallback:%s:%d:%x", ratio_setting.ModelQuotaPoolMetricTotalTokens, 0, poolDigest[:12])
	operationDigest := sha256.Sum256([]byte(operationKey))
	markerKey := fmt.Sprintf("%s:adjustment:%x", modelQuotaPoolRedisPrefix, operationDigest[:16])
	require.NoError(t, client.Del(context.Background(), poolKey, markerKey).Err())
	require.NoError(t, client.Set(context.Background(), poolKey, 1000, 0).Err())
	t.Cleanup(func() { _ = client.Del(context.Background(), poolKey, markerKey).Err() })
	info := &relaycommon.RelayInfo{
		RequestId: "pool-direct-fallback",
		ModelQuotaPools: []ratio_setting.ModelQuotaPoolMatch{{
			Metric:   ratio_setting.ModelQuotaPoolMetricTotalTokens,
			RedisKey: poolKey,
			Amount:   1000,
		}},
	}

	require.NoError(t, SettleModelQuotaPool(info, ModelQuotaPoolSettlement{
		ActualTotalTokens:    250,
		HasActualTotalTokens: true,
	}))
	assert.True(t, info.ModelQuotaPoolSettled)
	value, err := client.Get(context.Background(), poolKey).Int64()
	require.NoError(t, err)
	assert.Equal(t, int64(250), value)
}

func TestSettleRelayModelQuotaPoolKeepsTokenEstimateWithoutActualUsage(t *testing.T) {
	info := &relaycommon.RelayInfo{
		ModelQuotaPools: []ratio_setting.ModelQuotaPoolMatch{
			{
				Metric:    ratio_setting.ModelQuotaPoolMetricTotalTokens,
				RedisKey:  "pool:tokens",
				Amount:    1000,
				UsedAfter: 1000,
				Remaining: 9000,
			},
		},
	}
	called := false

	settleRelayModelQuotaPoolWithAdjuster(info, ModelQuotaPoolSettlement{
		ActualTotalTokens:    0,
		HasActualTotalTokens: false,
	}, func(string, int64) bool {
		called = true
		return true
	})

	assert.False(t, called)
	assert.Equal(t, int64(1000), info.ModelQuotaPools[0].Amount)
}

func TestAppendAdminBillingRulesOmitsRedisKeyWithoutMutatingReservation(t *testing.T) {
	reservation := ratio_setting.ModelQuotaPoolMatch{
		RedisKey: "model_quota_pool:global:secret",
		Amount:   10,
	}
	other := make(map[string]interface{})

	appendAdminBillingRules(other, nil, []ratio_setting.ModelQuotaPoolMatch{reservation})

	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	logPools, ok := adminInfo["model_quota_pools"].([]ratio_setting.ModelQuotaPoolMatch)
	require.True(t, ok)
	require.Len(t, logPools, 1)
	assert.Empty(t, logPools[0].RedisKey)
	assert.Equal(t, "model_quota_pool:global:secret", reservation.RedisKey)
	assert.NotContains(t, other, "model_quota_pools")
	serialized, err := common.Marshal(other)
	require.NoError(t, err)
	assert.NotContains(t, string(serialized), "redis_key")
}
