package service

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
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

// 上游按会话封禁请求时，同一上游的其他渠道也会拦下同一个会话：重试必须整家换掉，
// 放开「已失败渠道」排除集兜底时也不能回到这家；文案不匹配的错误不受影响。
func TestSessionBlockExcludesWholeUpstream(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	const modelName = "session-block-model"
	for id, baseURL := range map[int]string{431: "https://vip.block.test", 432: "https://VIP.block.test/v1", 433: "https://other.test"} {
		createCapacityTestChannel(t, db, id, "default", modelName, 0, nil)
		require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", id).Update("base_url", baseURL).Error)
	}
	model.InitChannelCache()
	common.SetUpstreamSessionBlockKeywords("cyber-security policy")
	t.Cleanup(func() { common.SetUpstreamSessionBlockKeywords("") })

	blockErr := types.NewOpenAIError(errors.New("该会话已被网络安全策略屏蔽，请开启新会话 / This session is blocked by cyber-security policy, please start a new session"), types.ErrorCodeBadResponseStatusCode, 403)
	otherErr := types.NewOpenAIError(errors.New("upstream overloaded"), types.ErrorCodeBadResponseStatusCode, 503)
	newParam := func() *RetryParam {
		retry := 1
		return &RetryParam{
			Ctx: newCapacitySelectContext(t), TokenGroup: "default", ModelName: modelName,
			RequestPath: "/v1/chat/completions", Retry: &retry,
			ExcludeChannelIds: map[int]bool{431: true},
		}
	}

	param := newParam()
	assert.False(t, ExcludeUpstreamAfterSessionBlock(param.Ctx, 431, otherErr, param))
	assert.Empty(t, param.ExcludeUpstreamHosts)

	require.True(t, ExcludeUpstreamAfterSessionBlock(param.Ctx, 431, blockErr, param))
	for range 20 {
		channel, _, err := CacheGetRandomSatisfiedChannel(param)
		require.NoError(t, err)
		require.NotNil(t, channel)
		require.Equal(t, 433, channel.Id, "同一上游的 432 也会封禁这个会话，不应被选中")
	}
	param.ExcludeChannelIds = map[int]bool{431: true, 433: true}
	channel, _, _ := CacheGetRandomSatisfiedChannel(param)
	assert.Nil(t, channel, "别家都失败后只剩封禁上游时不应选出渠道")
	param.ExcludeChannelIds = nil
	channel, _, _ = CacheGetRandomSatisfiedChannel(param)
	require.NotNil(t, channel, "放开已失败渠道后仍应排除封禁上游")
	assert.Equal(t, 433, channel.Id)
}
