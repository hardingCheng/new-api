package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

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

func TestRollbackModelQuotaPoolSkipsAlreadyFinalizedMatches(t *testing.T) {
	truncate(t)
	oldRedisEnabled, oldRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled, common.RDB = false, nil
	t.Cleanup(func() { common.RedisEnabled, common.RDB = oldRedisEnabled, oldRDB })
	info := &relaycommon.RelayInfo{
		RequestId: "partially-settled-pools",
		ModelQuotaPools: []ratio_setting.ModelQuotaPoolMatch{{
			ReservationID: 0,
			RedisKey:      "pool:already-finalized",
			Amount:        250,
		}},
	}

	require.NoError(t, RollbackModelQuotaPool(info))
	assert.True(t, info.ModelQuotaPoolSettled)
	assert.Empty(t, info.ModelQuotaPools)
	var adjustmentCount int64
	require.NoError(t, model.DB.Model(&model.QuotaPoolAdjustment{}).Count(&adjustmentCount).Error)
	assert.Zero(t, adjustmentCount)
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

func TestQuotaPoolReservationPersistsRecoveryBeforeRedisConsume(t *testing.T) {
	truncate(t)
	reservation, err := model.EnsureQuotaPoolReservation("relay:reserve:test-persisted", "pool:persisted", 25)
	require.NoError(t, err)
	assert.Equal(t, model.QuotaPoolAdjustmentStatusReserved, reservation.Status)
	assert.Equal(t, int64(25), reservation.ReservedAmount)
	assert.Equal(t, int64(-25), reservation.Delta)
	assert.Equal(t, model.QuotaPoolReservationGuardKey(reservation.ID), reservation.GuardKey)
	assert.Greater(t, reservation.NextRetryAt, common.GetTimestamp())

	adjustment, err := model.FinalizeQuotaPoolReservation(reservation.ID, 10)
	require.NoError(t, err)
	assert.Equal(t, model.QuotaPoolAdjustmentStatusPending, adjustment.Status)
	assert.Equal(t, int64(-15), adjustment.Delta)
}

func TestAbandonedQuotaPoolReservationReleasesRedisUsage(t *testing.T) {
	redisURL := os.Getenv("MODEL_QUOTA_POOL_REDIS_TEST_URL")
	if redisURL == "" {
		t.Skip("MODEL_QUOTA_POOL_REDIS_TEST_URL is not set")
	}
	truncate(t)
	options, err := redis.ParseURL(redisURL)
	require.NoError(t, err)
	client := redis.NewClient(options)
	require.NoError(t, client.Ping(context.Background()).Err())
	t.Cleanup(func() { _ = client.Close() })

	oldRedisEnabled, oldRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled, common.RDB = true, client
	t.Cleanup(func() { common.RedisEnabled, common.RDB = oldRedisEnabled, oldRDB })

	poolKey := "test:pool:abandoned-reservation"
	reservation, err := model.EnsureQuotaPoolReservation("relay:reserve:test-abandoned", poolKey, 30)
	require.NoError(t, err)
	require.NoError(t, client.Del(context.Background(), poolKey, reservation.GuardKey).Err())
	t.Cleanup(func() { _ = client.Del(context.Background(), poolKey, reservation.GuardKey).Err() })

	allowed, _, usedAfter, _, err := consumeModelQuotaPool(poolKey, reservation.GuardKey, 100, 30, time.Hour)
	require.NoError(t, err)
	require.True(t, allowed)
	assert.Equal(t, int64(30), usedAfter)
	require.NoError(t, model.DB.Model(&model.QuotaPoolAdjustment{}).
		Where("id = ?", reservation.ID).Update("next_retry_at", 0).Error)

	summary := ProcessPendingBillingAdjustments(context.Background(), 10)
	assert.Equal(t, 1, summary.PoolPending)
	assert.Equal(t, 1, summary.PoolSucceeded)
	assert.Zero(t, summary.PoolFailed)
	used, err := client.Get(context.Background(), poolKey).Int64()
	require.NoError(t, err)
	assert.Zero(t, used)
	assert.Zero(t, client.Exists(context.Background(), reservation.GuardKey).Val())
}

func TestGetVisibleModelQuotaPoolUsageScopesUserPools(t *testing.T) {
	previousConfig := ratio_setting.ModelQuotaPool2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelQuotaPoolByJSONString(previousConfig))
	})
	config := ratio_setting.ModelQuotaPoolConfig{Rules: []ratio_setting.ModelQuotaPoolRule{
		{ID: "global", Model: "*", Scope: ratio_setting.ModelQuotaPoolScopeGlobal, Metric: ratio_setting.ModelQuotaPoolMetricRequests, Period: ratio_setting.ModelQuotaPoolPeriodDay, Limit: 100},
		{ID: "user-1", Model: "*", Scope: ratio_setting.ModelQuotaPoolScopeUser, Metric: ratio_setting.ModelQuotaPoolMetricRequests, UserID: 1, Period: ratio_setting.ModelQuotaPoolPeriodDay, Limit: 10},
		{ID: "user-2", Model: "*", Scope: ratio_setting.ModelQuotaPoolScopeUser, Metric: ratio_setting.ModelQuotaPoolMetricRequests, UserID: 2, Period: ratio_setting.ModelQuotaPoolPeriodDay, Limit: 20},
	}}
	data, err := common.Marshal(config)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelQuotaPoolByJSONString(string(data)))

	selfItems := GetVisibleModelQuotaPoolUsage(1, false)
	require.Len(t, selfItems, 2)
	assert.Equal(t, "global", selfItems[0].Rule.ID)
	assert.Equal(t, "user-1", selfItems[1].Rule.ID)

	allItems := GetVisibleModelQuotaPoolUsage(1, true)
	require.Len(t, allItems, 3)
	assert.Equal(t, "global", allItems[0].Rule.ID)
	assert.Equal(t, "user-1", allItems[1].Rule.ID)
	assert.Equal(t, "user-2", allItems[2].Rule.ID)
}
