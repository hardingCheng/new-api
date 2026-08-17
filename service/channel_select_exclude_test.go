package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 重试选路必须在同优先级层内排除本次已失败的渠道：同层是加权随机抽取，
// 不排除的话重试会以 1/N 的概率打回刚刚失败的那条，池子只剩一条时是必然。
func TestSelectionExcludesFailedChannelWithinSameTier(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	const modelName = "exclude-same-tier-model"

	createCapacityTestChannel(t, db, 401, "default", modelName, 0, nil)
	createCapacityTestChannel(t, db, 402, "default", modelName, 0, nil)
	model.InitChannelCache()

	ctx := newCapacitySelectContext(t)
	retry := 1
	param := &RetryParam{
		Ctx: ctx, TokenGroup: "default", ModelName: modelName,
		RequestPath:       "/v1/chat/completions",
		Retry:             &retry,
		ExcludeChannelIds: map[int]bool{401: true},
	}

	for i := 0; i < 20; i++ {
		channel, _, err := CacheGetRandomSatisfiedChannel(param)
		require.NoError(t, err)
		require.NotNil(t, channel)
		require.Equal(t, 402, channel.Id, "已失败的渠道 401 不应被重新抽中")
	}
	assert.Equal(t, 1, param.GetRetry(), "同层换渠道不消耗降层机会")
}

// 排除集吃掉全部候选时选路选不出渠道，调用方放开排除集后必须还能选出来 ——
// controller 的兜底依赖这个行为，否则「还能重试一次」会退化成直接失败。
func TestSelectionRecoversAfterClearingExclusions(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	const modelName = "exclude-all-model"

	createCapacityTestChannel(t, db, 411, "default", modelName, 0, nil)
	createCapacityTestChannel(t, db, 412, "default", modelName, 0, nil)
	model.InitChannelCache()

	ctx := newCapacitySelectContext(t)
	retry := 2
	param := &RetryParam{
		Ctx: ctx, TokenGroup: "default", ModelName: modelName,
		RequestPath:       "/v1/chat/completions",
		Retry:             &retry,
		ExcludeChannelIds: map[int]bool{411: true, 412: true},
	}

	channel, _, _ := CacheGetRandomSatisfiedChannel(param)
	require.Nil(t, channel, "候选被排除干净时不应选出渠道")

	param.ExcludeChannelIds = nil
	channel, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, channel, "放开排除集后必须能重新选出渠道")
	assert.Contains(t, []int{411, 412}, channel.Id)
}

// 空排除集不改变原有选路行为，保证改动对未失败的首次请求零影响。
func TestSelectionWithEmptyExclusionsIsUnchanged(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	const modelName = "exclude-empty-model"

	createCapacityTestChannel(t, db, 421, "default", modelName, 0, nil)
	model.InitChannelCache()

	ctx := newCapacitySelectContext(t)
	retry := 0
	param := &RetryParam{
		Ctx: ctx, TokenGroup: "default", ModelName: modelName,
		RequestPath:       "/v1/chat/completions",
		Retry:             &retry,
		ExcludeChannelIds: map[int]bool{},
	}

	channel, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 421, channel.Id)
}
