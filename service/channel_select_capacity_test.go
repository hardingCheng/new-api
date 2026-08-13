package service

import (
	"errors"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setCapacityModeForSelectTest(t *testing.T, mode string) {
	t.Helper()
	cfg := config.GlobalConfig.Get("channel_capacity_setting")
	require.NotNil(t, cfg)
	config.UpdateConfigFromMap(cfg, map[string]string{"mode": mode})
	t.Cleanup(func() {
		config.UpdateConfigFromMap(cfg, map[string]string{"mode": "off"})
	})
}

func createCapacityTestChannel(t *testing.T, db *gorm.DB, id int, group, modelName string, priority int64, capacity *dto.ChannelCapacitySettings) {
	t.Helper()
	weight := uint(100)
	channel := &model.Channel{
		Id:       id,
		Type:     constant.ChannelTypeOpenAI,
		Key:      fmt.Sprintf("key-%d", id),
		Status:   common.ChannelStatusEnabled,
		Name:     fmt.Sprintf("channel-%d", id),
		Weight:   &weight,
		Models:   modelName,
		Group:    group,
		Priority: &priority,
	}
	if capacity != nil {
		channel.SetSetting(dto.ChannelSettings{Capacity: capacity})
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     group,
		Model:     modelName,
		ChannelId: id,
		Enabled:   true,
		Priority:  &priority,
		Weight:    weight,
	}).Error)
}

func newCapacitySelectContext(t *testing.T) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	return ctx
}

// fillChannelCapacity 直接占满渠道的容量名额，模拟满载状态
func fillChannelCapacity(t *testing.T, channelID int, capacity *dto.ChannelCapacitySettings, slots int) {
	t.Helper()
	for i := 0; i < slots; i++ {
		attemptID := BuildChannelCapacityAttemptID(fmt.Sprintf("filler-%d", channelID), i)
		decision := TryAcquireChannelCapacity(nil, channelID, capacity, operation_setting.ChannelCapacityModeEnforce, attemptID)
		require.True(t, decision.Allowed, "预填充第 %d 个名额应成功", i+1)
	}
}

func TestEnforceSelectionSkipsFullChannelWithinSameTier(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	useChannelCapacityMiniRedis(t)
	setCapacityModeForSelectTest(t, "enforce")
	const modelName = "capacity-same-tier-model"

	limited := &dto.ChannelCapacitySettings{RPM: 1}
	createCapacityTestChannel(t, db, 301, "default", modelName, 0, limited)
	createCapacityTestChannel(t, db, 302, "default", modelName, 0, &dto.ChannelCapacitySettings{RPM: 5})
	model.InitChannelCache()

	fillChannelCapacity(t, 301, limited, 1)

	ctx := newCapacitySelectContext(t)
	retry := 0
	param := &RetryParam{
		Ctx: ctx, TokenGroup: "default", ModelName: modelName,
		RequestPath: "/v1/chat/completions", Retry: &retry, ReserveCapacity: true,
	}

	channel, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 302, channel.Id, "同层满载渠道必须被跳过")
	assert.Equal(t, 0, param.GetRetry(), "容量跳过不消耗重试次数")

	lease := TakeReservedChannelCapacityLease(ctx)
	require.NotNil(t, lease, "选中的限额渠道应携带已预留的租约")
	assert.Nil(t, TakeReservedChannelCapacityLease(ctx), "租约取走即删")
	require.NoError(t, lease.CancelBeforeDispatch(ctx))
}

func TestEnforceSelectionOverflowsToLowerPriorityTier(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	useChannelCapacityMiniRedis(t)
	setCapacityModeForSelectTest(t, "enforce")
	originalRetryTimes := common.RetryTimes
	common.RetryTimes = 3
	t.Cleanup(func() { common.RetryTimes = originalRetryTimes })
	const modelName = "capacity-overflow-model"

	limited := &dto.ChannelCapacitySettings{RPM: 1, MaxConcurrency: 1}
	createCapacityTestChannel(t, db, 311, "default", modelName, 10, limited)
	createCapacityTestChannel(t, db, 312, "default", modelName, 0, nil)
	model.InitChannelCache()

	fillChannelCapacity(t, 311, limited, 1)

	ctx := newCapacitySelectContext(t)
	retry := 0
	param := &RetryParam{
		Ctx: ctx, TokenGroup: "default", ModelName: modelName,
		RequestPath: "/v1/chat/completions", Retry: &retry, ReserveCapacity: true,
	}

	channel, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 312, channel.Id, "高优先级层满载时必须溢出到低优先级兜底渠道")
	assert.Equal(t, 0, param.GetRetry())
	assert.Nil(t, TakeReservedChannelCapacityLease(ctx), "无限额渠道不产生租约")
}

func TestEnforceSelectionAllFullReturnsCapacityError(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	useChannelCapacityMiniRedis(t)
	setCapacityModeForSelectTest(t, "enforce")
	const modelName = "capacity-all-full-model"

	limitedA := &dto.ChannelCapacitySettings{RPM: 1}
	limitedB := &dto.ChannelCapacitySettings{MaxConcurrency: 1}
	createCapacityTestChannel(t, db, 321, "default", modelName, 0, limitedA)
	createCapacityTestChannel(t, db, 322, "default", modelName, 0, limitedB)
	model.InitChannelCache()

	fillChannelCapacity(t, 321, limitedA, 1)
	fillChannelCapacity(t, 322, limitedB, 1)

	ctx := newCapacitySelectContext(t)
	retry := 0
	param := &RetryParam{
		Ctx: ctx, TokenGroup: "default", ModelName: modelName,
		RequestPath: "/v1/chat/completions", Retry: &retry, ReserveCapacity: true,
	}

	channel, _, err := CacheGetRandomSatisfiedChannel(param)
	require.Nil(t, channel)
	require.Error(t, err)
	var capacityErr *ChannelCapacityExhaustedError
	require.True(t, errors.As(err, &capacityErr), "全满时必须返回专用容量错误，实际: %v", err)
	assert.False(t, capacityErr.RedisError)
	assert.GreaterOrEqual(t, capacityErr.RetryAfterMs, int64(1000))
	assert.LessOrEqual(t, capacityErr.RetryAfterMs, int64(60000))
	assert.Nil(t, TakeReservedChannelCapacityLease(ctx), "拒绝路径不得遗留租约")
}

func TestSelectionWithoutReserveCapacityIgnoresLimits(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	useChannelCapacityMiniRedis(t)
	setCapacityModeForSelectTest(t, "enforce")
	const modelName = "capacity-no-reserve-model"

	limited := &dto.ChannelCapacitySettings{RPM: 1}
	createCapacityTestChannel(t, db, 331, "default", modelName, 0, limited)
	model.InitChannelCache()

	fillChannelCapacity(t, 331, limited, 1)

	// distributor 初选等调用方不置 ReserveCapacity：满载渠道照常可选、零容量副作用
	ctx := newCapacitySelectContext(t)
	retry := 0
	param := &RetryParam{
		Ctx: ctx, TokenGroup: "default", ModelName: modelName,
		RequestPath: "/v1/chat/completions", Retry: &retry,
	}

	channel, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 331, channel.Id)
	assert.Nil(t, TakeReservedChannelCapacityLease(ctx))
}

func TestShadowModeSelectionUnaffectedByFullChannels(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	useChannelCapacityMiniRedis(t)
	setCapacityModeForSelectTest(t, "shadow")
	const modelName = "capacity-shadow-select-model"

	limited := &dto.ChannelCapacitySettings{RPM: 1}
	createCapacityTestChannel(t, db, 341, "default", modelName, 0, limited)
	model.InitChannelCache()

	fillChannelCapacity(t, 341, limited, 1)

	ctx := newCapacitySelectContext(t)
	retry := 0
	param := &RetryParam{
		Ctx: ctx, TokenGroup: "default", ModelName: modelName,
		RequestPath: "/v1/chat/completions", Retry: &retry, ReserveCapacity: true,
	}

	channel, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 341, channel.Id, "shadow 模式选路不排除满载渠道")
	assert.Nil(t, TakeReservedChannelCapacityLease(ctx))
}
