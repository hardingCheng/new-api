package middleware

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestChannelMatchesExpectedTaskPluginUsesGenericChannelSetting(t *testing.T) {
	channel := &model.Channel{Type: constant.ChannelTypeTaskPlugin}
	channel.SetSetting(dto.ChannelSettings{TaskPluginKey: "generic-alpha"})

	assert.True(t, channelMatchesExpectedTaskPlugin(nil, channel, "generic-alpha"))
	assert.False(t, channelMatchesExpectedTaskPlugin(nil, channel, "generic-beta"))
	assert.False(t, channelMatchesExpectedTaskPlugin(nil, channel, ""))
}

func TestChannelMatchesExpectedTaskPluginUsesPinnedLegacyIndex(t *testing.T) {
	registry := jsplugin.NewRegistry()
	alpha, err := registry.Register(distributorTaskPluginSource("legacy-alpha", constant.ChannelTypeKling), jsplugin.Options{})
	require.NoError(t, err)
	pinnedGeneration := registry.Generation()

	require.NoError(t, registry.Unregister("legacy-alpha"))
	_, err = registry.Register(distributorTaskPluginSource("legacy-beta", constant.ChannelTypeKling), jsplugin.Options{})
	require.NoError(t, err)

	c, _ := gin.CreateTestContext(nil)
	c.Set(jsplugin.ContextKeyPinnedPlugin, jsplugin.PinnedPlugin{
		Generation: pinnedGeneration,
		Plugin:     alpha,
	})
	channel := &model.Channel{Type: constant.ChannelTypeKling}

	assert.True(t, channelMatchesExpectedTaskPlugin(c, channel, "legacy-alpha"))
	assert.False(t, channelMatchesExpectedTaskPlugin(c, channel, "legacy-beta"))
	assert.False(t, channelMatchesExpectedTaskPlugin(c, &model.Channel{Type: constant.ChannelTypeJimeng}, "legacy-alpha"))
}

func TestChannelMatchesExpectedTaskPluginRejectsUnindexedLegacyChannel(t *testing.T) {
	registry := jsplugin.NewRegistry()
	plugin, err := registry.Register(distributorTaskPluginSource("legacy-alpha", constant.ChannelTypeKling), jsplugin.Options{})
	require.NoError(t, err)

	c, _ := gin.CreateTestContext(nil)
	c.Set(jsplugin.ContextKeyPinnedPlugin, jsplugin.PinnedPlugin{
		Generation: registry.Generation(),
		Plugin:     plugin,
	})

	assert.False(t, channelMatchesExpectedTaskPlugin(c, &model.Channel{Type: constant.ChannelTypeJimeng}, "legacy-alpha"))
	assert.False(t, channelMatchesExpectedTaskPlugin(c, &model.Channel{Type: 0}, "legacy-alpha"))
	assert.True(t, channelMatchesExpectedTaskPlugin(c, &model.Channel{Type: constant.ChannelTypeJimeng}, ""))
	assert.False(t, channelMatchesExpectedTaskPlugin(nil, &model.Channel{Type: constant.ChannelTypeKling}, "legacy-alpha"))

	c.Set("expected_task_plugin_key", "legacy-alpha")
	setupErr := SetupContextForSelectedChannel(c, &model.Channel{Type: constant.ChannelTypeJimeng}, "task-model")
	require.NotNil(t, setupErr)
	assert.Contains(t, setupErr.Error(), "does not match")
}

func TestSharedEndpointRebindsToSelectedLegacyProvider(t *testing.T) {
	registry := jsplugin.NewRegistry()
	_, err := registry.Register(distributorEndpointPluginSource("gemini-shared", constant.ChannelTypeGemini), jsplugin.Options{})
	require.NoError(t, err)
	_, err = registry.Register(distributorEndpointPluginSource("vertex-shared", constant.ChannelTypeVertexAi), jsplugin.Options{})
	require.NoError(t, err)
	candidates := registry.Generation().LookupEndpointCandidates("POST", "/v1/responses", "task-model")
	require.Len(t, candidates, 2)

	c, _ := gin.CreateTestContext(nil)
	c.Set(jsplugin.ContextKeyPinnedPlugin, jsplugin.PinnedPlugin{Generation: registry.Generation(), Plugin: candidates[0].Plugin})
	c.Set(jsplugin.ContextKeyPinnedEndpoint, jsplugin.PinnedEndpoint{
		Generation: registry.Generation(),
		Plugin:     candidates[0].Plugin,
		Protocol:   candidates[0].Protocol,
		Operation:  candidates[0].Operation,
		Model:      "task-model",
		Candidates: candidates,
	})
	c.Set("expected_task_plugin_key", candidates[0].Plugin.Meta.Key)

	geminiChannel := &model.Channel{Id: 1, Type: constant.ChannelTypeGemini}
	vertexChannel := &model.Channel{Id: 2, Type: constant.ChannelTypeVertexAi}
	assert.True(t, channelMatchesExpectedTaskPlugin(c, geminiChannel, candidates[0].Plugin.Meta.Key))
	assert.True(t, channelMatchesExpectedTaskPlugin(c, vertexChannel, candidates[0].Plugin.Meta.Key))
	assert.False(t, channelMatchesExpectedTaskPlugin(c, &model.Channel{Type: constant.ChannelTypeKling}, candidates[0].Plugin.Meta.Key))

	require.Nil(t, SetupContextForSelectedChannel(c, vertexChannel, "task-model"))
	pinnedValue, exists := c.Get(jsplugin.ContextKeyPinnedEndpoint)
	require.True(t, exists)
	pinned, ok := pinnedValue.(jsplugin.PinnedEndpoint)
	require.True(t, ok)
	assert.Equal(t, "vertex-shared", pinned.Plugin.Meta.Key)
	assert.Equal(t, "vertex-shared", c.GetString("expected_task_plugin_key"))
	assert.Equal(t, "vertex-shared", c.GetString("task_plugin_key"))
	assert.True(t, channelMatchesExpectedTaskPlugin(c, geminiChannel, "vertex-shared"), "a retry may select another declared provider")
}

func distributorTaskPluginSource(key string, channelType int) string {
	return fmt.Sprintf(`
export const meta = {
  apiVersion: 1,
  key: %q,
  name: %q,
  version: "1.0.0",
  author: {name: "Test"},
  channelTypes: [%d],
  models: ["task-model"],
  fetchMode: "per_task",
};
export function buildSubmitRequest() { return {}; }
export function parseSubmitResponse() { return {taskId: "task"}; }
export function buildQueryRequest() { return {}; }
export function parseTaskResult() { return {status: "SUCCESS"}; }
`, key, key, channelType)
}

func distributorEndpointPluginSource(key string, channelType int) string {
	return fmt.Sprintf(`
export const meta = {
  apiVersion: 1,
  key: %q,
  name: %q,
  version: "1.0.0",
  author: {name: "Test"},
  channelTypes: [%d],
  models: ["task-model"],
  fetchMode: "per_task",
  protocols: [{name: "openai_responses", supports: ["stream", "sync", "background"]}],
};
export function buildSubmitRequest() { return {}; }
export function parseSubmitResponse() { return {taskId: "task"}; }
export function buildQueryRequest() { return {}; }
export function parseTaskResult() { return {status: "SUCCESS"}; }
export const protocols = {openai_responses: {
  decodeRequest: function(ctx) { return {kind: "submit", model: "task-model", requestBody: ctx.body.value}; },
  renderEvents: function() { return {events: [], state: null, done: false}; },
  renderFinal: function() { return {output: []}; },
}};
`, key, key, channelType)
}

func TestTokenModelLimitAllowsLegacyAliasAndModifierVariant(t *testing.T) {
	aliasOnly := map[string]bool{"claude-3-7-sonnet-thinking": true}
	assert.True(t, TokenModelLimitAllows(aliasOnly, "claude-3-7-sonnet-thinking"))
	assert.False(t, TokenModelLimitAllows(aliasOnly, "claude-3-7-sonnet"))

	baseOnly := map[string]bool{"claude-3-7-sonnet": true}
	assert.True(t, TokenModelLimitAllows(baseOnly, "claude-3-7-sonnet@thinking:on"))
	assert.True(t, TokenModelLimitAllows(baseOnly, "claude-3-7-sonnet-thinking"))

	wildcard := map[string]bool{"gemini-2.5-flash-thinking-*": true}
	assert.True(t, TokenModelLimitAllows(wildcard, "gemini-2.5-flash-thinking-8192"))
}

func TestTokenModelLimitAllowsExemptAtNameByFullName(t *testing.T) {
	settings := model_setting.GetGlobalSettings()
	original := append([]string(nil), settings.ThinkingModelBlacklist...)
	t.Cleanup(func() { settings.ThinkingModelBlacklist = original })
	settings.ThinkingModelBlacklist = append(original, "re:.*@sha256:.*")

	fullOnly := map[string]bool{"opaque@sha256:deadbeef": true}
	assert.True(t, TokenModelLimitAllows(fullOnly, "opaque@sha256:deadbeef"))

	baseOnly := map[string]bool{"opaque": true}
	assert.False(t, TokenModelLimitAllows(baseOnly, "opaque@sha256:deadbeef"))
}

func TestDistributeHidesTaskPluginDetailsButLogsDiagnostics(t *testing.T) {
	require.NoError(t, i18n.Init())
	previousCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	t.Cleanup(func() { common.MemoryCacheEnabled = previousCacheEnabled })

	const group = "private-plugin-error-test-group"
	for _, locale := range []struct{ language, message string }{
		{"en", "No available channel for model task-model under group " + group + ": the model is claimed by a task plugin, which has no enabled channel serving it (distributor)"},
		{"zh-CN", "分组 " + group + " 下模型 task-model 无可用渠道：该模型由任务插件认领，但当前没有启用的渠道可服务此模型（distributor）"},
		{"zh-TW", "分組 " + group + " 下模型 task-model 無可用管道：該模型由任務插件認領，但目前沒有啟用的管道可服務此模型（distributor）"},
	} {
		for _, providerCount := range []int{1, 2} {
			t.Run(fmt.Sprintf("%s/%d_providers", locale.language, providerCount), func(t *testing.T) {
				registry := jsplugin.NewRegistry()
				keys := []string{"private-provider-alpha", "private-provider-beta"}[:providerCount]
				for index, key := range keys {
					_, err := registry.Register(distributorEndpointPluginSource(key, constant.ChannelTypeKling+index), jsplugin.Options{})
					require.NoError(t, err)
				}
				generation := registry.Generation()
				candidates := generation.LookupEndpointCandidates("POST", "/v1/responses", "task-model")
				require.Len(t, candidates, providerCount)

				var logs bytes.Buffer
				common.LogWriterMu.Lock()
				previousWriter := gin.DefaultErrorWriter
				gin.DefaultErrorWriter = &logs
				common.LogWriterMu.Unlock()
				t.Cleanup(func() {
					common.LogWriterMu.Lock()
					gin.DefaultErrorWriter = previousWriter
					common.LogWriterMu.Unlock()
				})

				router := gin.New()
				router.POST("/v1/responses", RequestId(), func(c *gin.Context) {
					common.SetContextKey(c, constant.ContextKeyUsingGroup, group)
					c.Set("resolved_task_model", "task-model")
					c.Set("expected_task_plugin_key", keys[0])
					c.Set(jsplugin.ContextKeyPinnedPlugin, jsplugin.PinnedPlugin{Generation: generation, Plugin: candidates[0].Plugin})
					if providerCount > 1 {
						c.Set(jsplugin.ContextKeyPinnedEndpoint, jsplugin.PinnedEndpoint{
							Generation: generation, Plugin: candidates[0].Plugin,
							Protocol: candidates[0].Protocol, Operation: candidates[0].Operation,
							Model: "task-model", Candidates: candidates,
						})
					}
				}, Distribute(), func(c *gin.Context) {
					t.Error("unavailable requests must stop before the relay handler")
					c.Status(http.StatusNoContent)
				})
				request := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
				request.Header.Set("Accept-Language", locale.language)
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, request)

				require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
				requestID := recorder.Header().Get(common.RequestIdKey)
				require.NotEmpty(t, requestID)
				assert.JSONEq(t, fmt.Sprintf(`{"error":{"message":%q,"type":"new_api_error","code":"model_not_found"}}`,
					locale.message+" (request id: "+requestID+")"), recorder.Body.String())
				assert.NotContains(t, recorder.Body.String(), "disable or override")
				for _, key := range keys {
					assert.NotContains(t, recorder.Body.String(), key)
					assert.Contains(t, logs.String(), key)
				}
				assert.Contains(t, logs.String(), requestID)
				assert.Contains(t, logs.String(), `group="`+group+`"`)
				assert.Contains(t, logs.String(), `model="task-model"`)
				assert.Contains(t, logs.String(), "reason=no_eligible_channel")
			})
		}
	}
}

func TestNoAvailableChannelMessageWithoutPlugin(t *testing.T) {
	require.NoError(t, i18n.Init())
	plain, _ := gin.CreateTestContext(nil)
	plain.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	plain.Request.Header.Set("Accept-Language", "en")
	generic := noAvailableChannelMessage(plain, "default", "gpt-4o")
	assert.NotContains(t, generic, "task plugin")
	assert.Contains(t, generic, "gpt-4o")
}

func TestSharedEndpointRebindsToSelectedType61Plugin(t *testing.T) {
	registry := jsplugin.NewRegistry()
	for _, key := range []string{"alpha", "beta"} {
		source := strings.Replace(distributorEndpointPluginSource(key, 0), "channelTypes: [0],", "", 1)
		_, err := registry.Register(source, jsplugin.Options{})
		require.NoError(t, err)
	}
	generation := registry.Generation()
	candidates := generation.LookupEndpointCandidates("POST", "/v1/responses", "task-model")
	require.Len(t, candidates, 2)
	c, _ := gin.CreateTestContext(nil)
	c.Set(jsplugin.ContextKeyPinnedPlugin, jsplugin.PinnedPlugin{Generation: generation, Plugin: candidates[0].Plugin})
	c.Set(jsplugin.ContextKeyPinnedEndpoint, jsplugin.PinnedEndpoint{Generation: generation, Plugin: candidates[0].Plugin, Protocol: candidates[0].Protocol, Operation: candidates[0].Operation, Model: "task-model", Candidates: candidates})
	c.Set("expected_task_plugin_key", "alpha")
	channel := &model.Channel{Id: 2, Type: constant.ChannelTypeTaskPlugin}
	channel.SetSetting(dto.ChannelSettings{TaskPluginKey: "unrelated"})
	assert.False(t, channelMatchesExpectedTaskPlugin(c, channel, "alpha"))
	channel.SetSetting(dto.ChannelSettings{TaskPluginKey: "beta"})
	require.Nil(t, SetupContextForSelectedChannel(c, channel, "task-model"))
	assert.Equal(t, "beta", c.GetString("task_plugin_key"))
	assert.Equal(t, "beta", c.GetString("expected_task_plugin_key"))
	assert.Equal(t, "beta", c.MustGet(jsplugin.ContextKeyPinnedEndpoint).(jsplugin.PinnedEndpoint).Plugin.Meta.Key)
}

func TestChannelMatchesExpectedTaskPluginUsesNewAPIExtensions(t *testing.T) {
	channel := &model.Channel{Type: constant.ChannelTypeNewAPI}
	channel.SetSetting(dto.ChannelSettings{TaskPluginKey: "single", TaskExtendPluginKeys: []string{"ext-alpha", "ext-beta"}})

	assert.True(t, channelMatchesExpectedTaskPlugin(nil, channel, "ext-alpha"))
	assert.True(t, channelMatchesExpectedTaskPlugin(nil, channel, "single"), "the single key stays valid on a New API channel")
	assert.False(t, channelMatchesExpectedTaskPlugin(nil, channel, "ext-gamma"))
	assert.True(t, channelMatchesExpectedTaskPlugin(nil, channel, ""), "requests without a pinned plugin still use the gateway")
	assert.False(t, channelMatchesExpectedTaskPlugin(nil, &model.Channel{Type: constant.ChannelTypeNewAPI}, "ext-alpha"))
}

func TestSharedEndpointRebindsToBoundNewAPIExtension(t *testing.T) {
	registry := jsplugin.NewRegistry()
	for _, key := range []string{"alpha", "beta"} {
		source := strings.Replace(distributorEndpointPluginSource(key, 0), "channelTypes: [0],", "", 1)
		_, err := registry.Register(source, jsplugin.Options{})
		require.NoError(t, err)
	}
	generation := registry.Generation()
	candidates := generation.LookupEndpointCandidates("POST", "/v1/responses", "task-model")
	require.Len(t, candidates, 2)
	pin := func(expected string) *gin.Context {
		pinned := candidates[0]
		if candidates[1].Plugin.Meta.Key == expected {
			pinned = candidates[1]
		}
		c, _ := gin.CreateTestContext(nil)
		c.Set(jsplugin.ContextKeyPinnedPlugin, jsplugin.PinnedPlugin{Generation: generation, Plugin: pinned.Plugin})
		c.Set(jsplugin.ContextKeyPinnedEndpoint, jsplugin.PinnedEndpoint{Generation: generation, Plugin: pinned.Plugin, Protocol: pinned.Protocol, Operation: pinned.Operation, Model: "task-model", Candidates: candidates})
		c.Set("expected_task_plugin_key", expected)
		return c
	}

	channel := &model.Channel{Id: 3, Type: constant.ChannelTypeNewAPI}
	channel.SetSetting(dto.ChannelSettings{TaskExtendPluginKeys: []string{"unrelated"}})
	assert.False(t, channelMatchesExpectedTaskPlugin(pin("alpha"), channel, "alpha"))

	channel.SetSetting(dto.ChannelSettings{TaskExtendPluginKeys: []string{"beta"}})
	c := pin("alpha")
	require.Nil(t, SetupContextForSelectedChannel(c, channel, "task-model"))
	assert.Equal(t, "beta", c.GetString("task_plugin_key"), "the only bound candidate executes")
	assert.Equal(t, "beta", c.GetString("expected_task_plugin_key"))
	assert.Equal(t, "beta", c.MustGet(jsplugin.ContextKeyPinnedEndpoint).(jsplugin.PinnedEndpoint).Plugin.Meta.Key)

	channel.SetSetting(dto.ChannelSettings{TaskExtendPluginKeys: []string{"alpha", "beta"}})
	c = pin("beta")
	require.Nil(t, SetupContextForSelectedChannel(c, channel, "task-model"))
	assert.Equal(t, "alpha", c.GetString("task_plugin_key"), "the first bound candidate executes regardless of the earlier pin")
	assert.Equal(t, "alpha", c.MustGet(jsplugin.ContextKeyPinnedEndpoint).(jsplugin.PinnedEndpoint).Plugin.Meta.Key)
}

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
		service.GetChannelConstraints(context).AddPin(taskdto.ChannelPin{ChannelId: 1, Source: taskdto.PinSourceToken, Rank: taskdto.PinRankToken, RetryMode: taskdto.PinRetrySingleAttempt})
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

func TestDistributeRejectsSpecificChannelOutsideUserRoutingPool(t *testing.T) {
	gin.SetMode(gin.TestMode)
	require.NoError(t, i18n.Init())
	originalRouting := model_setting.UserChannelRouting2JSONString()
	originalDB := model.DB
	originalRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	common.SetChannelBreakerEnabled(false)
	t.Cleanup(func() {
		require.NoError(t, model_setting.UpdateUserChannelRoutingByJSONString(originalRouting))
		model.DB = originalDB
		common.RedisEnabled = originalRedisEnabled
		common.SetChannelBreakerEnabled(false)
	})
	require.NoError(t, model_setting.UpdateUserChannelRoutingByJSONString(`{
		"rules": [{"id":"route","name":"Route","user_id":521,"group_pattern":"sd2","model_pattern":"*","channel_ids":[2],"fallback":"strict"}]
	}`))

	database, err := gorm.Open(sqlite.Open("file:middleware_user_channel_routing?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = database
	require.NoError(t, database.AutoMigrate(&model.Channel{}))
	require.NoError(t, database.Create(&model.Channel{
		Id:     1,
		Type:   constant.ChannelTypeOpenAI,
		Key:    "test-key",
		Status: common.ChannelStatusEnabled,
		Name:   "forbidden-channel",
		Group:  "sd2",
		Models: "video",
	}).Error)

	router := gin.New()
	router.Use(func(context *gin.Context) {
		common.SetContextKey(context, constant.ContextKeyUserId, 521)
		common.SetContextKey(context, constant.ContextKeyUsingGroup, "sd2")
		service.GetChannelConstraints(context).AddPin(taskdto.ChannelPin{ChannelId: 1, Source: taskdto.PinSourceToken, Rank: taskdto.PinRankToken, RetryMode: taskdto.PinRetrySingleAttempt})
		context.Next()
	})
	router.Use(Distribute())
	router.POST("/v1/video/generations", func(context *gin.Context) {
		context.Status(http.StatusNoContent)
	})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/video/generations",
		strings.NewReader(`{"model":"video","prompt":"animate"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusServiceUnavailable, response.Code)
}

func TestDistributeExpandsAutoGroupBeforeCheckingSpecificChannelRouting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	require.NoError(t, i18n.Init())
	originalRouting := model_setting.UserChannelRouting2JSONString()
	originalAutoGroups := setting.AutoGroups2JsonString()
	originalUsableGroups := setting.UserUsableGroups2JSONString()
	originalRatios := ratio_setting.GroupRatio2JSONString()
	originalDB := model.DB
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	originalRedisEnabled := common.RedisEnabled
	common.MemoryCacheEnabled = true
	common.RedisEnabled = false
	common.SetChannelBreakerEnabled(false)
	t.Cleanup(func() {
		require.NoError(t, model_setting.UpdateUserChannelRoutingByJSONString(originalRouting))
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsableGroups))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalRatios))
		model.DB = originalDB
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		common.RedisEnabled = originalRedisEnabled
		common.SetChannelBreakerEnabled(false)
		model.InitChannelCache()
	})
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["sd2"]`))
	// 上游 #6590 起，auto 分组必须同时「用户可选」且「配了分组倍率」才会被展开。
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"sd2":"SD2"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"sd2":1}`))
	require.NoError(t, model_setting.UpdateUserChannelRoutingByJSONString(`{
		"rules": [{"id":"route","name":"Route","user_id":521,"group_pattern":"sd2","model_pattern":"*","channel_ids":[1],"fallback":"strict"}]
	}`))

	database, err := gorm.Open(sqlite.Open("file:middleware_auto_user_channel_routing?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = database
	require.NoError(t, database.AutoMigrate(&model.Channel{}, &model.Ability{}))
	priority := int64(10)
	require.NoError(t, database.Create(&model.Channel{
		Id:       1,
		Type:     constant.ChannelTypeOpenAI,
		Key:      "test-key",
		Status:   common.ChannelStatusEnabled,
		Name:     "allowed-channel",
		Group:    "sd2",
		Models:   "video",
		Priority: &priority,
	}).Error)
	require.NoError(t, database.Create(&model.Ability{
		Group: "sd2", Model: "video", ChannelId: 1, Enabled: true, Priority: &priority, Weight: 100,
	}).Error)
	model.InitChannelCache()

	var selectedAutoGroup string
	router := gin.New()
	router.Use(func(context *gin.Context) {
		common.SetContextKey(context, constant.ContextKeyUserId, 521)
		common.SetContextKey(context, constant.ContextKeyUserGroup, "sd2")
		common.SetContextKey(context, constant.ContextKeyUsingGroup, "auto")
		service.GetChannelConstraints(context).AddPin(taskdto.ChannelPin{ChannelId: 1, Source: taskdto.PinSourceToken, Rank: taskdto.PinRankToken, RetryMode: taskdto.PinRetrySingleAttempt})
		context.Next()
	})
	router.Use(Distribute())
	router.POST("/v1/video/generations", func(context *gin.Context) {
		selectedAutoGroup = common.GetContextKeyString(context, constant.ContextKeyAutoGroup)
		context.Status(http.StatusNoContent)
	})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(`{"model":"video"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusNoContent, response.Code)
	assert.Equal(t, "sd2", selectedAutoGroup)
}
