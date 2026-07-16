package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
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
