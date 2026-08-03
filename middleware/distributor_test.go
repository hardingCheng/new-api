package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSetupContextForSelectedChannelKeyCanProbeDisabledKey(t *testing.T) {
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	common.SetChannelBreakerEnabled(false)
	t.Cleanup(func() {
		common.RedisEnabled = oldRedisEnabled
		common.SetChannelBreakerEnabled(false)
	})
	channel := &model.Channel{
		Id:     901,
		Type:   constant.ChannelTypeOpenAI,
		Name:   "recoverable-multi-key",
		Key:    "terminal-key\nrecoverable-key",
		Status: common.ChannelStatusAutoDisabled,
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:         true,
			MultiKeySize:       2,
			MultiKeyStatusList: map[int]int{0: common.ChannelStatusAutoDisabled, 1: common.ChannelStatusAutoDisabled},
		},
	}

	regularCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	regularCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	require.Error(t, SetupContextForSelectedChannel(regularCtx, channel, "gpt-4o-mini"))

	probeCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	probeCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	setupErr := SetupContextForSelectedChannelKey(probeCtx, channel, "gpt-4o-mini", 1)
	require.Nil(t, setupErr)
	require.Equal(t, "recoverable-key", common.GetContextKeyString(probeCtx, constant.ContextKeyChannelKey))
	require.Equal(t, 1, common.GetContextKeyInt(probeCtx, constant.ContextKeyChannelMultiKeyIndex))
}

func TestDistributeRoutesUserAliasAndAllowsDirectTarget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalViewJSON, err := common.Marshal(model_setting.GetUserModelViewCopy())
	require.NoError(t, err)
	originalDB := model.DB
	originalRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	common.SetChannelBreakerEnabled(false)
	t.Cleanup(func() {
		require.NoError(t, model_setting.UpdateUserModelViewByJSONString(string(originalViewJSON)))
		model.DB = originalDB
		common.RedisEnabled = originalRedisEnabled
		common.SetChannelBreakerEnabled(false)
	})
	require.NoError(t, model_setting.UpdateUserModelViewByJSONString(`{
		"rules": [{
			"user_id": 521,
			"aliases": [
				{"public_model":"521ai-2.0-720p","target_model":"seedance-2.0-720p","reference_video":"forbidden"}
			]
		}]
	}`))

	database, err := gorm.Open(sqlite.Open("file:middleware_user_model_view?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = database
	require.NoError(t, database.AutoMigrate(&model.Channel{}))
	require.NoError(t, database.Create(&model.Channel{
		Id:     1,
		Type:   constant.ChannelTypeOpenAI,
		Key:    "test-key",
		Status: common.ChannelStatusEnabled,
		Name:   "seedance-test-channel",
		Group:  "sd2",
		Models: "seedance-2.0-720p",
	}).Error)

	var publicModel string
	var routingModel string
	var referencePolicy string
	router := gin.New()
	router.Use(func(context *gin.Context) {
		common.SetContextKey(context, constant.ContextKeyUserId, 521)
		common.SetContextKey(context, constant.ContextKeyTokenSpecificChannelId, "1")
		context.Next()
	})
	router.Use(Distribute())
	router.POST("/v1/video/generations", func(context *gin.Context) {
		publicModel = common.GetContextKeyString(context, constant.ContextKeyOriginalModel)
		routingModel = common.GetContextKeyString(context, constant.ContextKeyRoutingModel)
		referencePolicy = common.GetContextKeyString(context, constant.ContextKeyReferenceVideoPolicy)
		context.Status(http.StatusNoContent)
	})

	aliasRecorder := httptest.NewRecorder()
	aliasRequest := httptest.NewRequest(
		http.MethodPost,
		"/v1/video/generations",
		strings.NewReader(`{"model":"521ai-2.0-720p","prompt":"animate"}`),
	)
	aliasRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(aliasRecorder, aliasRequest)

	assert.Equal(t, http.StatusNoContent, aliasRecorder.Code)
	assert.Equal(t, "521ai-2.0-720p", publicModel)
	assert.Equal(t, "seedance-2.0-720p", routingModel)
	assert.Equal(t, model_setting.ReferenceVideoForbidden, referencePolicy)

	directRecorder := httptest.NewRecorder()
	directRequest := httptest.NewRequest(
		http.MethodPost,
		"/v1/video/generations",
		strings.NewReader(`{"model":"seedance-2.0-720p","prompt":"animate"}`),
	)
	directRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(directRecorder, directRequest)

	assert.Equal(t, http.StatusNoContent, directRecorder.Code)
	assert.Equal(t, "seedance-2.0-720p", publicModel)
	assert.Empty(t, routingModel)
	assert.Empty(t, referencePolicy)
}
