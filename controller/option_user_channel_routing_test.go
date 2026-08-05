package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateOptionValidatesUserChannelRoutingChannelIDs(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	require.NoError(t, db.Create(&model.Channel{Id: 1, Name: "channel-1"}).Error)

	original := model_setting.UserChannelRouting2JSONString()
	originalOptionMap := common.OptionMap
	common.OptionMap = map[string]string{}
	t.Cleanup(func() {
		require.NoError(t, model_setting.UpdateUserChannelRoutingByJSONString(original))
		common.OptionMap = originalOptionMap
	})

	t.Run("rejects a missing channel", func(t *testing.T) {
		response := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(response)
		context.Request = httptest.NewRequest(
			http.MethodPut,
			"/api/option/",
			strings.NewReader(`{"key":"UserChannelRouting","value":"{\"rules\":[{\"id\":\"route\",\"name\":\"Route\",\"user_id\":7,\"group_pattern\":\"sd2\",\"channel_ids\":[1,999],\"fallback\":\"strict\"}]}"}`),
		)

		UpdateOption(context)

		assert.Equal(t, http.StatusOK, response.Code)
		var payload struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
		assert.False(t, payload.Success)
		assert.Equal(t, "one or more channel_ids do not exist", payload.Message)
	})

	t.Run("persists and activates valid routing", func(t *testing.T) {
		response := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(response)
		context.Request = httptest.NewRequest(
			http.MethodPut,
			"/api/option/",
			strings.NewReader(`{"key":"UserChannelRouting","value":"{\"rules\":[{\"id\":\"route\",\"name\":\"Route\",\"user_id\":7,\"group_pattern\":\"sd2\",\"channel_ids\":[1],\"fallback\":\"strict\"}]}"}`),
		)

		UpdateOption(context)

		assert.Equal(t, http.StatusOK, response.Code)
		var payload struct {
			Success bool `json:"success"`
		}
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
		assert.True(t, payload.Success)
		match, ok := model_setting.MatchUserChannelRouting(7, "sd2", "video")
		require.True(t, ok)
		assert.Equal(t, []int{1}, match.Rule.ChannelIDs)
	})
}
