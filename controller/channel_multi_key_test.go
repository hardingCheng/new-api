package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func manageMultiKeyForTest(t *testing.T, channelId int, action string, keyIndex *int) {
	t.Helper()
	payload, err := common.Marshal(MultiKeyManageRequest{
		ChannelId: channelId,
		Action:    action,
		KeyIndex:  keyIndex,
	})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/multi_key/manage", bytes.NewReader(payload))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", 1)
	ctx.Set("role", common.RoleRootUser)

	ManageMultiKeys(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success, response.Message)
}

func TestManageMultiKeysEnableAllClearsTerminalState(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	channel := &model.Channel{
		Type:   1,
		Name:   "enable-terminal-keys",
		Key:    "key-a\nkey-b",
		Status: common.ChannelStatusAutoDisabled,
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:                   true,
			MultiKeySize:                 2,
			MultiKeyStatusList:           map[int]int{0: common.ChannelStatusAutoDisabled, 1: common.ChannelStatusAutoDisabled},
			MultiKeyAutoRecoveryDisabled: map[int]bool{0: true, 1: true},
			AutoRecoveryDisabled:         true,
		},
	}
	require.NoError(t, db.Create(channel).Error)

	manageMultiKeyForTest(t, channel.Id, "enable_all_keys", nil)

	var updated model.Channel
	require.NoError(t, db.First(&updated, channel.Id).Error)
	require.Equal(t, common.ChannelStatusEnabled, updated.Status)
	require.Empty(t, updated.ChannelInfo.MultiKeyStatusList)
	require.Empty(t, updated.ChannelInfo.MultiKeyAutoRecoveryDisabled)
	require.False(t, updated.ChannelInfo.AutoRecoveryDisabled)
}

func TestManageMultiKeysDeleteKeyReindexesTerminalState(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	channel := &model.Channel{
		Type:   1,
		Name:   "delete-before-terminal-key",
		Key:    "healthy-key\nterminal-key",
		Status: common.ChannelStatusEnabled,
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:                   true,
			MultiKeySize:                 2,
			MultiKeyStatusList:           map[int]int{1: common.ChannelStatusAutoDisabled},
			MultiKeyAutoRecoveryDisabled: map[int]bool{1: true},
		},
	}
	require.NoError(t, db.Create(channel).Error)
	keyIndex := 0

	manageMultiKeyForTest(t, channel.Id, "delete_key", &keyIndex)

	var updated model.Channel
	require.NoError(t, db.First(&updated, channel.Id).Error)
	require.Equal(t, "terminal-key", updated.Key)
	require.Equal(t, common.ChannelStatusAutoDisabled, updated.Status)
	require.Equal(t, common.ChannelStatusAutoDisabled, updated.ChannelInfo.MultiKeyStatusList[0])
	require.True(t, updated.ChannelInfo.MultiKeyAutoRecoveryDisabled[0])
	require.True(t, updated.ChannelInfo.AutoRecoveryDisabled)
	_, staleMarker := updated.ChannelInfo.MultiKeyAutoRecoveryDisabled[1]
	require.False(t, staleMarker)
}

func TestManageMultiKeysEnableKeyClearsOnlyItsTerminalState(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	channel := &model.Channel{
		Type:   1,
		Name:   "enable-one-terminal-key",
		Key:    "key-a\nkey-b",
		Status: common.ChannelStatusAutoDisabled,
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:                   true,
			MultiKeySize:                 2,
			MultiKeyStatusList:           map[int]int{0: common.ChannelStatusAutoDisabled, 1: common.ChannelStatusAutoDisabled},
			MultiKeyAutoRecoveryDisabled: map[int]bool{0: true, 1: true},
			AutoRecoveryDisabled:         true,
		},
	}
	require.NoError(t, db.Create(channel).Error)
	keyIndex := 1

	manageMultiKeyForTest(t, channel.Id, "enable_key", &keyIndex)

	var updated model.Channel
	require.NoError(t, db.First(&updated, channel.Id).Error)
	require.Equal(t, common.ChannelStatusEnabled, updated.Status)
	require.Equal(t, common.ChannelStatusAutoDisabled, updated.ChannelInfo.MultiKeyStatusList[0])
	_, enabledKeyStatus := updated.ChannelInfo.MultiKeyStatusList[1]
	require.False(t, enabledKeyStatus)
	require.True(t, updated.ChannelInfo.MultiKeyAutoRecoveryDisabled[0])
	_, enabledKeyTerminal := updated.ChannelInfo.MultiKeyAutoRecoveryDisabled[1]
	require.False(t, enabledKeyTerminal)
	require.False(t, updated.ChannelInfo.AutoRecoveryDisabled)
}

func TestManageMultiKeysDeleteDisabledKeysRemovesTerminalMarkers(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	channel := &model.Channel{
		Type:   1,
		Name:   "delete-terminal-keys",
		Key:    "terminal-key\nhealthy-key\nmanual-key",
		Status: common.ChannelStatusEnabled,
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:                   true,
			MultiKeySize:                 3,
			MultiKeyStatusList:           map[int]int{0: common.ChannelStatusAutoDisabled, 2: common.ChannelStatusManuallyDisabled},
			MultiKeyAutoRecoveryDisabled: map[int]bool{0: true},
		},
	}
	require.NoError(t, db.Create(channel).Error)

	manageMultiKeyForTest(t, channel.Id, "delete_disabled_keys", nil)

	var updated model.Channel
	require.NoError(t, db.First(&updated, channel.Id).Error)
	require.Equal(t, "healthy-key\nmanual-key", updated.Key)
	require.Equal(t, common.ChannelStatusEnabled, updated.Status)
	require.Equal(t, common.ChannelStatusManuallyDisabled, updated.ChannelInfo.MultiKeyStatusList[1])
	require.Empty(t, updated.ChannelInfo.MultiKeyAutoRecoveryDisabled)
	require.False(t, updated.ChannelInfo.AutoRecoveryDisabled)
}
