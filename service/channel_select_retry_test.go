package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 同档优先:重试把失败渠道移出档内候选,同档还有健康渠道时绝不降档。
// (旧行为是"第 N 次重试从第 N 档起步",本用例在旧行为下会选到 P5 的 403)
func TestRetrySelectionPrefersSameTierBeforeDescending(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	const modelName = "same-tier-retry-model"
	createCapacityTestChannel(t, db, 401, "default", modelName, 10, nil)
	createCapacityTestChannel(t, db, 402, "default", modelName, 10, nil)
	createCapacityTestChannel(t, db, 403, "default", modelName, 5, nil)
	model.InitChannelCache()

	// 加权随机选路,多跑几轮确认不是碰巧
	for i := 0; i < 8; i++ {
		ctx := newCapacitySelectContext(t)
		retry := 1
		param := &RetryParam{
			Ctx: ctx, TokenGroup: "default", ModelName: modelName,
			RequestPath: "/v1/chat/completions", Retry: &retry,
			ExcludeChannelIds: map[int]bool{401: true},
		}
		channel, _, err := CacheGetRandomSatisfiedChannel(param)
		require.NoError(t, err)
		require.NotNil(t, channel)
		assert.Equal(t, 402, channel.Id, "同档还有 402 可用,不许降档选 403")
	}
}

// 档内候选全部失败后,才降到下一优先级档
func TestRetrySelectionDescendsWhenTierExhausted(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	const modelName = "tier-exhausted-retry-model"
	createCapacityTestChannel(t, db, 411, "default", modelName, 10, nil)
	createCapacityTestChannel(t, db, 412, "default", modelName, 10, nil)
	createCapacityTestChannel(t, db, 413, "default", modelName, 5, nil)
	model.InitChannelCache()

	ctx := newCapacitySelectContext(t)
	retry := 2
	param := &RetryParam{
		Ctx: ctx, TokenGroup: "default", ModelName: modelName,
		RequestPath: "/v1/chat/completions", Retry: &retry,
		ExcludeChannelIds: map[int]bool{411: true, 412: true},
	}
	channel, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 413, channel.Id, "P10 档剔空后应降到 P5")
}

// 全池都失败:选不出渠道(channel=nil),由调用方(relay getChannel)放开排除集兜底
func TestRetrySelectionAllExcludedReturnsNoChannel(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	const modelName = "all-excluded-retry-model"
	createCapacityTestChannel(t, db, 421, "default", modelName, 10, nil)
	createCapacityTestChannel(t, db, 422, "default", modelName, 5, nil)
	model.InitChannelCache()

	ctx := newCapacitySelectContext(t)
	retry := 2
	param := &RetryParam{
		Ctx: ctx, TokenGroup: "default", ModelName: modelName,
		RequestPath: "/v1/chat/completions", Retry: &retry,
		ExcludeChannelIds: map[int]bool{421: true, 422: true},
	}
	channel, _, _ := CacheGetRandomSatisfiedChannel(param)
	require.Nil(t, channel)
}

// 首次请求(无排除集)仍从最高档选起,行为与改动前一致
func TestFirstAttemptStillPicksTopTier(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	const modelName = "first-attempt-model"
	createCapacityTestChannel(t, db, 431, "default", modelName, 10, nil)
	createCapacityTestChannel(t, db, 432, "default", modelName, 5, nil)
	model.InitChannelCache()

	for i := 0; i < 8; i++ {
		ctx := newCapacitySelectContext(t)
		retry := 0
		param := &RetryParam{
			Ctx: ctx, TokenGroup: "default", ModelName: modelName,
			RequestPath: "/v1/chat/completions", Retry: &retry,
		}
		channel, _, err := CacheGetRandomSatisfiedChannel(param)
		require.NoError(t, err)
		require.NotNil(t, channel)
		assert.Equal(t, 431, channel.Id, "首选必须是最高优先级档")
	}
}
