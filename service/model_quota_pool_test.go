package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
