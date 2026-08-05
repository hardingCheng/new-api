package service

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func preserveUserChannelRoutingForServiceTest(t *testing.T) {
	t.Helper()
	original := model_setting.UserChannelRouting2JSONString()
	originalRetryTimes := common.RetryTimes
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	originalBreakerEnabled := common.IsChannelBreakerEnabled()
	t.Cleanup(func() {
		require.NoError(t, model_setting.UpdateUserChannelRoutingByJSONString(original))
		model.DB.Exec("DELETE FROM abilities")
		model.DB.Exec("DELETE FROM channels")
		model.InitChannelCache()
		common.RetryTimes = originalRetryTimes
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		common.SetChannelBreakerEnabled(originalBreakerEnabled)
	})
	common.RetryTimes = 2
	common.MemoryCacheEnabled = true
	common.SetChannelBreakerEnabled(false)
	require.NoError(t, model.DB.AutoMigrate(&model.Ability{}))
	require.NoError(t, model.DB.Exec("DELETE FROM abilities").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM channels").Error)
}

func createUserChannelRoutingFixtures(t *testing.T) map[int]*model.Channel {
	t.Helper()
	priorities := map[int]int64{1: 100, 2: 10, 3: 200}
	channels := make(map[int]*model.Channel, len(priorities))
	for id, priority := range priorities {
		channel := &model.Channel{
			Id:       id,
			Type:     1,
			Name:     "channel",
			Key:      "sk-test",
			Status:   common.ChannelStatusEnabled,
			Group:    "sd2",
			Models:   "video",
			Priority: common.GetPointer(priority),
		}
		require.NoError(t, model.DB.Create(channel).Error)
		require.NoError(t, model.DB.Create(&model.Ability{
			Group:     "sd2",
			Model:     "video",
			ChannelId: id,
			Enabled:   true,
			Priority:  common.GetPointer(priority),
			Weight:    100,
		}).Error)
		channels[id] = channel
	}
	model.InitChannelCache()
	return channels
}

func newUserChannelRoutingContext(userID int) *gin.Context {
	c, _ := gin.CreateTestContext(nil)
	common.SetContextKey(c, constant.ContextKeyUserId, userID)
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "sd2")
	return c
}

func TestUserChannelRoutingSelectsEachUsersAssignedPool(t *testing.T) {
	preserveUserChannelRoutingForServiceTest(t)
	createUserChannelRoutingFixtures(t)
	require.NoError(t, model_setting.UpdateUserChannelRoutingByJSONString(`{"rules":[
		{"id":"user-a","name":"User A","user_id":10,"group_pattern":"sd2","model_pattern":"*","channel_ids":[1,2],"fallback":"strict"},
		{"id":"user-b","name":"User B","user_id":20,"group_pattern":"sd2","model_pattern":"*","channel_ids":[3],"fallback":"strict"}
	]}`))

	userA := newUserChannelRoutingContext(10)
	selected, _, err := CacheGetRandomSatisfiedChannel(&RetryParam{Ctx: userA, TokenGroup: "sd2", ModelName: "video", Retry: common.GetPointer(0)})
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Contains(t, []int{1, 2}, selected.Id)

	userB := newUserChannelRoutingContext(20)
	selected, _, err = CacheGetRandomSatisfiedChannel(&RetryParam{Ctx: userB, TokenGroup: "sd2", ModelName: "video", Retry: common.GetPointer(0)})
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, 3, selected.Id)

	unconfigured := newUserChannelRoutingContext(30)
	selected, _, err = CacheGetRandomSatisfiedChannel(&RetryParam{Ctx: unconfigured, TokenGroup: "sd2", ModelName: "video", Retry: common.GetPointer(0)})
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, 3, selected.Id)
}

func TestUserChannelRoutingStrictAndDefaultFallback(t *testing.T) {
	preserveUserChannelRoutingForServiceTest(t)
	channels := createUserChannelRoutingFixtures(t)
	resetStatuses := func(t *testing.T) {
		t.Helper()
		for _, channel := range channels {
			require.NoError(t, model.DB.Model(channel).Update("status", common.ChannelStatusEnabled).Error)
		}
		model.InitChannelCache()
	}

	t.Run("strict stops when assigned channels are unavailable", func(t *testing.T) {
		resetStatuses(t)
		require.NoError(t, model.DB.Model(channels[1]).Update("status", common.ChannelStatusAutoDisabled).Error)
		require.NoError(t, model.DB.Model(channels[2]).Update("status", common.ChannelStatusAutoDisabled).Error)
		model.InitChannelCache()
		require.NoError(t, model_setting.UpdateUserChannelRoutingByJSONString(`{"rules":[{"id":"strict","name":"Strict","user_id":10,"group_pattern":"sd2","model_pattern":"*","channel_ids":[1,2],"fallback":"strict"}]}`))

		c := newUserChannelRoutingContext(10)
		selected, _, err := CacheGetRandomSatisfiedChannel(&RetryParam{Ctx: c, TokenGroup: "sd2", ModelName: "video", Retry: common.GetPointer(0)})
		require.NoError(t, err)
		assert.Nil(t, selected)
	})

	t.Run("default uses a lower assigned priority before group fallback", func(t *testing.T) {
		resetStatuses(t)
		require.NoError(t, model.DB.Model(channels[1]).Update("status", common.ChannelStatusAutoDisabled).Error)
		require.NoError(t, model.DB.Model(channels[2]).Update("status", common.ChannelStatusEnabled).Error)
		model.InitChannelCache()
		require.NoError(t, model_setting.UpdateUserChannelRoutingByJSONString(`{"rules":[{"id":"default","name":"Default","user_id":10,"group_pattern":"sd2","model_pattern":"*","channel_ids":[1,2],"fallback":"default"}]}`))

		c := newUserChannelRoutingContext(10)
		selected, _, err := CacheGetRandomSatisfiedChannel(&RetryParam{Ctx: c, TokenGroup: "sd2", ModelName: "video", Retry: common.GetPointer(0)})
		require.NoError(t, err)
		require.NotNil(t, selected)
		assert.Equal(t, 2, selected.Id)

		adminInfo := map[string]interface{}{}
		AppendUserChannelRoutingAdminInfo(c, adminInfo)
		routingInfo := adminInfo["user_channel_routing"].(map[string]interface{})
		assert.Equal(t, false, routingInfo["fallback_used"])
		assert.Equal(t, 2, routingInfo["selected_channel_id"])
	})

	t.Run("default falls back after all assigned priorities are unavailable", func(t *testing.T) {
		resetStatuses(t)
		require.NoError(t, model.DB.Model(channels[1]).Update("status", common.ChannelStatusAutoDisabled).Error)
		require.NoError(t, model.DB.Model(channels[2]).Update("status", common.ChannelStatusAutoDisabled).Error)
		model.InitChannelCache()
		require.NoError(t, model_setting.UpdateUserChannelRoutingByJSONString(`{"rules":[{"id":"default","name":"Default","user_id":10,"group_pattern":"sd2","model_pattern":"*","channel_ids":[1,2],"fallback":"default"}]}`))

		c := newUserChannelRoutingContext(10)
		selected, _, err := CacheGetRandomSatisfiedChannel(&RetryParam{Ctx: c, TokenGroup: "sd2", ModelName: "video", Retry: common.GetPointer(0)})
		require.NoError(t, err)
		require.NotNil(t, selected)
		assert.Equal(t, 3, selected.Id)

		adminInfo := map[string]interface{}{}
		AppendUserChannelRoutingAdminInfo(c, adminInfo)
		routingInfo := adminInfo["user_channel_routing"].(map[string]interface{})
		assert.Equal(t, "default", routingInfo["rule_id"])
		assert.Equal(t, true, routingInfo["fallback_used"])
		assert.Equal(t, 3, routingInfo["selected_channel_id"])
		assert.Equal(t, "assigned_pool_unavailable", routingInfo["fallback_reason"])
	})
}

func TestUserChannelRoutingScansEveryAssignedPriorityBeforeFallback(t *testing.T) {
	preserveUserChannelRoutingForServiceTest(t)
	channels := createUserChannelRoutingFixtures(t)
	common.RetryTimes = 0
	require.NoError(t, model.DB.Model(channels[1]).Update("status", common.ChannelStatusAutoDisabled).Error)
	model.InitChannelCache()

	for _, fallback := range []string{
		model_setting.UserChannelRoutingFallbackStrict,
		model_setting.UserChannelRoutingFallbackDefault,
	} {
		t.Run(fallback, func(t *testing.T) {
			require.NoError(t, model_setting.UpdateUserChannelRoutingByJSONString(fmt.Sprintf(`{"rules":[{"id":"%s","name":"Assigned priorities","user_id":10,"group_pattern":"sd2","model_pattern":"*","channel_ids":[1,2],"fallback":"%s"}]}`, fallback, fallback)))

			c := newUserChannelRoutingContext(10)
			selected, _, err := CacheGetRandomSatisfiedChannel(&RetryParam{
				Ctx:        c,
				TokenGroup: "sd2",
				ModelName:  "video",
				Retry:      common.GetPointer(0),
			})

			require.NoError(t, err)
			require.NotNil(t, selected)
			assert.Equal(t, channels[2].Id, selected.Id)
			adminInfo := map[string]interface{}{}
			AppendUserChannelRoutingAdminInfo(c, adminInfo)
			routingInfo := adminInfo["user_channel_routing"].(map[string]interface{})
			assert.Equal(t, false, routingInfo["fallback_used"])
		})
	}
}

func TestUserChannelRoutingAdminInfoTracksFinalDecisionAndAlias(t *testing.T) {
	preserveUserChannelRoutingForServiceTest(t)
	require.NoError(t, model_setting.UpdateUserChannelRoutingByJSONString(`{"rules":[{"id":"sd2","name":"SD2","user_id":10,"group_pattern":"sd2","model_pattern":"video","channel_ids":[1],"fallback":"strict"}]}`))

	c := newUserChannelRoutingContext(10)
	decision := resolveUserChannelRouting(c, "sd2", "video")
	require.True(t, decision.Matched)
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "public-video")
	adminInfo := map[string]interface{}{}
	AppendUserChannelRoutingAdminInfo(c, adminInfo)
	routingInfo := adminInfo["user_channel_routing"].(map[string]interface{})
	assert.Equal(t, "public-video", routingInfo["requested_model"])

	decision = resolveUserChannelRouting(c, "default", "video")
	assert.False(t, decision.Matched)
	adminInfo = map[string]interface{}{}
	AppendUserChannelRoutingAdminInfo(c, adminInfo)
	assert.NotContains(t, adminInfo, "user_channel_routing")

	decision = resolveUserChannelRouting(c, "sd2", "video")
	require.True(t, decision.Matched)
	adminInfo = map[string]interface{}{}
	AppendUserChannelRoutingAdminInfo(c, adminInfo)
	routingInfo = adminInfo["user_channel_routing"].(map[string]interface{})
	assert.Equal(t, "sd2", routingInfo["rule_id"])
	assert.Equal(t, "public-video", routingInfo["requested_model"])
}

func TestUserChannelRoutingRejectsAffinityOutsideAssignedPool(t *testing.T) {
	preserveUserChannelRoutingForServiceTest(t)
	channels := createUserChannelRoutingFixtures(t)
	require.NoError(t, model_setting.UpdateUserChannelRoutingByJSONString(`{"rules":[{"id":"user-a","name":"User A","user_id":10,"group_pattern":"sd2","model_pattern":"*","channel_ids":[1,2],"fallback":"strict"}]}`))

	c := newUserChannelRoutingContext(10)
	assert.False(t, AllowSelectedChannelByUserRouting(c, "sd2", "video", channels[3]))
	assert.True(t, AllowSelectedChannelByUserRouting(c, "sd2", "video", channels[1]))
}

func TestUserChannelRoutingChecksSelectedChannelAgainstExpandedAutoGroups(t *testing.T) {
	preserveUserChannelRoutingForServiceTest(t)
	channels := createUserChannelRoutingFixtures(t)
	originalAutoGroups := setting.AutoGroups2JsonString()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
	})
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["sd2"]`))
	require.NoError(t, model_setting.UpdateUserChannelRoutingByJSONString(`{"rules":[
		{"id":"sd2","name":"SD2","user_id":10,"group_pattern":"sd2","model_pattern":"*","channel_ids":[2],"fallback":"strict"}
	]}`))

	c := newUserChannelRoutingContext(10)
	common.SetContextKey(c, constant.ContextKeyUserGroup, "sd2")
	assert.False(t, AllowSelectedChannelByUserRouting(c, "auto", "video", channels[1]))
	assert.True(t, AllowSelectedChannelByUserRouting(c, "auto", "video", channels[2]))
	assert.Equal(t, "sd2", common.GetContextKeyString(c, constant.ContextKeyAutoGroup))

	adminInfo := map[string]interface{}{}
	AppendUserChannelRoutingAdminInfo(c, adminInfo)
	routingInfo := adminInfo["user_channel_routing"].(map[string]interface{})
	assert.Equal(t, "sd2", routingInfo["using_group"])
	assert.Equal(t, channels[2].Id, routingInfo["selected_channel_id"])
}

func TestUserChannelRoutingKeepsLegacySpecificChannelBehaviorWithoutAutoRule(t *testing.T) {
	preserveUserChannelRoutingForServiceTest(t)
	channels := createUserChannelRoutingFixtures(t)
	originalAutoGroups := setting.AutoGroups2JsonString()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
	})
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default"]`))
	require.NoError(t, model_setting.UpdateUserChannelRoutingByJSONString(`{"rules":[]}`))

	c := newUserChannelRoutingContext(10)
	common.SetContextKey(c, constant.ContextKeyUserGroup, "sd2")
	assert.True(t, AllowSelectedChannelByUserRouting(c, "auto", "video", channels[1]))
}

func TestUserChannelRoutingMatchesExpandedAutoGroup(t *testing.T) {
	preserveUserChannelRoutingForServiceTest(t)
	channels := createUserChannelRoutingFixtures(t)
	originalAutoGroups := setting.AutoGroups2JsonString()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
	})
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["sd2","default"]`))

	defaultPriority := int64(50)
	defaultChannel := &model.Channel{
		Id:       4,
		Type:     1,
		Name:     "default-channel",
		Key:      "sk-default",
		Status:   common.ChannelStatusEnabled,
		Group:    "default",
		Models:   "video",
		Priority: common.GetPointer(defaultPriority),
	}
	require.NoError(t, model.DB.Create(defaultChannel).Error)
	require.NoError(t, model.DB.Create(&model.Ability{
		Group:     "default",
		Model:     "video",
		ChannelId: defaultChannel.Id,
		Enabled:   true,
		Priority:  common.GetPointer(defaultPriority),
		Weight:    100,
	}).Error)
	require.NoError(t, model.DB.Model(channels[1]).Update("status", common.ChannelStatusAutoDisabled).Error)
	model.InitChannelCache()
	require.NoError(t, model_setting.UpdateUserChannelRoutingByJSONString(`{"rules":[
		{"id":"sd2","name":"SD2","user_id":10,"group_pattern":"sd2","model_pattern":"*","channel_ids":[1],"fallback":"strict"},
		{"id":"default","name":"Default","user_id":10,"group_pattern":"default","model_pattern":"*","channel_ids":[4],"fallback":"strict"}
	]}`))

	c := newUserChannelRoutingContext(10)
	common.SetContextKey(c, constant.ContextKeyUserGroup, "sd2")
	selected, selectedGroup, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:        c,
		TokenGroup: "auto",
		ModelName:  "video",
		Retry:      common.GetPointer(0),
	})

	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, defaultChannel.Id, selected.Id)
	assert.Equal(t, "default", selectedGroup)
	assert.Equal(t, "default", common.GetContextKeyString(c, constant.ContextKeyAutoGroup))
	adminInfo := map[string]interface{}{}
	AppendUserChannelRoutingAdminInfo(c, adminInfo)
	routingInfo := adminInfo["user_channel_routing"].(map[string]interface{})
	assert.Equal(t, "default", routingInfo["rule_id"])
	assert.Equal(t, "default", routingInfo["using_group"])
}

func TestUserChannelRoutingAutoGroupReturnsDatabaseErrors(t *testing.T) {
	preserveUserChannelRoutingForServiceTest(t)
	originalAutoGroups := setting.AutoGroups2JsonString()
	originalDB := model.DB
	t.Cleanup(func() {
		model.DB = originalDB
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
	})
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["sd2"]`))
	require.NoError(t, model_setting.UpdateUserChannelRoutingByJSONString(`{"rules":[
		{"id":"sd2","name":"SD2","user_id":10,"group_pattern":"sd2","model_pattern":"*","channel_ids":[1],"fallback":"strict"}
	]}`))

	database, err := gorm.Open(sqlite.Open("file:service_user_channel_routing_closed?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	model.DB = database
	common.MemoryCacheEnabled = false

	c := newUserChannelRoutingContext(10)
	common.SetContextKey(c, constant.ContextKeyUserGroup, "sd2")
	selected, selectedGroup, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx: c, TokenGroup: "auto", ModelName: "video", Retry: common.GetPointer(0),
	})

	assert.Nil(t, selected)
	assert.Equal(t, "sd2", selectedGroup)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database is closed")
}
