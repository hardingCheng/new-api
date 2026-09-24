package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMultiKeyEnableRestoresOnlyExhaustedChannels(t *testing.T) {
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	previousMaster, previousCache, previousRedis, previousSQLite := common.IsMasterNode, common.MemoryCacheEnabled, common.RedisEnabled, common.SQLitePath
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousType, previousLogType)
		common.IsMasterNode, common.MemoryCacheEnabled, common.RedisEnabled, common.SQLitePath = previousMaster, previousCache, previousRedis, previousSQLite
	})
	t.Setenv("SQL_DSN", os.Getenv("TEST_CHANNEL_SQL_DSN"))
	t.Setenv("LOG_SQL_DSN", "")
	common.IsMasterNode, common.MemoryCacheEnabled, common.RedisEnabled = false, false, false
	common.SQLitePath = filepath.Join(t.TempDir(), "channel.db")
	require.NoError(t, model.InitDB())
	database := model.DB
	sqlDB, err := database.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	model.LOG_DB = database
	common.SetLogDatabaseType(common.MainDatabaseType())
	require.NoError(t, database.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.User{}, &model.Log{}, &model.AuditLog{}))
	root := &model.User{Username: "multi-key-review-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
	require.NoError(t, database.Create(root).Error)
	t.Cleanup(func() { require.NoError(t, database.Unscoped().Delete(root).Error) })
	versionQuery := "SELECT VERSION()"
	if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		versionQuery = "SELECT sqlite_version()"
	}
	var version string
	require.NoError(t, database.Raw(versionQuery).Scan(&version).Error)
	t.Logf("database=%s version=%s", common.MainDatabaseType(), version)

	for _, cacheEnabled := range []bool{false, true} {
		for _, action := range []string{"enable_key", "enable_all_keys"} {
			for _, tc := range []struct {
				name           string
				initialStatus  int
				manualOverride string
				wantStatus     int
			}{
				{name: "key exhaustion restores", initialStatus: common.ChannelStatusEnabled, wantStatus: common.ChannelStatusEnabled},
				{name: "manual disable is preserved", initialStatus: common.ChannelStatusManuallyDisabled, wantStatus: common.ChannelStatusManuallyDisabled},
				{name: "manual disable after exhaustion is preserved", initialStatus: common.ChannelStatusEnabled, manualOverride: "status", wantStatus: common.ChannelStatusManuallyDisabled},
				{name: "tag disable after exhaustion is preserved", initialStatus: common.ChannelStatusEnabled, manualOverride: "tag", wantStatus: common.ChannelStatusManuallyDisabled},
			} {
				t.Run(fmt.Sprintf("cache=%t/%s/%s", cacheEnabled, action, tc.name), func(t *testing.T) {
					common.MemoryCacheEnabled = cacheEnabled
					tag := t.Name()
					channel := &model.Channel{Name: t.Name(), Type: 1, Key: "key-one\nkey-two", Status: tc.initialStatus, Models: "test-model", Group: "default", Tag: &tag,
						ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeySize: 2, MultiKeyStatusList: map[int]int{1: common.ChannelStatusManuallyDisabled}},
					}
					require.NoError(t, channel.Insert())
					t.Cleanup(func() {
						require.NoError(t, channel.Delete())
						model.InitChannelCache()
					})
					for _, operation := range []string{"disable_key", action} {
						if operation == action {
							if tc.manualOverride == "status" {
								model.UpdateChannelStatus(channel.Id, "", common.ChannelStatusManuallyDisabled, "manual operation")
							} else if tc.manualOverride == "tag" {
								require.NoError(t, model.DisableChannelByTag(tag))
							}
						}
						payload, err := common.Marshal(MultiKeyManageRequest{ChannelId: channel.Id, Action: operation, KeyIndex: common.GetPointer(0)})
						require.NoError(t, err)
						recorder := httptest.NewRecorder()
						c, _ := gin.CreateTestContext(recorder)
						c.Set("id", root.Id)
						c.Set("role", common.RoleRootUser)
						c.Request = httptest.NewRequest(http.MethodPost, "/api/channel/multi_key", bytes.NewReader(payload))
						c.Request.Header.Set("Content-Type", "application/json")
						ManageMultiKeys(c)
						var result struct {
							Success bool `json:"success"`
						}
						require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &result))
						require.True(t, result.Success, recorder.Body.String())
					}
					loaded, err := model.GetChannelById(channel.Id, true)
					require.NoError(t, err)
					assert.Equal(t, tc.wantStatus, loaded.Status)
					assert.NotContains(t, loaded.ChannelInfo.MultiKeyStatusList, 0)
					assert.NotContains(t, loaded.ChannelInfo.MultiKeyDisabledReason, 0)
					assert.NotContains(t, loaded.ChannelInfo.MultiKeyDisabledTime, 0)
					var ability model.Ability
					require.NoError(t, database.Where("channel_id = ?", channel.Id).First(&ability).Error)
					assert.Equal(t, tc.wantStatus == common.ChannelStatusEnabled, ability.Enabled)
				})
			}
		}
	}
}

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

func TestManageMultiKeysPreservesChannelLevelManualDisable(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	channel := &model.Channel{
		Type:   1,
		Name:   "manual-channel-key-update",
		Key:    "key-a\nkey-b",
		Status: common.ChannelStatusEnabled,
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 2,
		},
	}
	require.NoError(t, db.Create(channel).Error)
	require.True(t, model.UpdateChannelStatus(channel.Id, "", common.ChannelStatusManuallyDisabled, "manual operation"))
	keyIndex := 0

	manageMultiKeyForTest(t, channel.Id, "disable_key", &keyIndex)

	var updated model.Channel
	require.NoError(t, db.First(&updated, channel.Id).Error)
	require.Equal(t, common.ChannelStatusManuallyDisabled, updated.Status)
	require.True(t, updated.ChannelInfo.ChannelManuallyDisabled)
	require.Equal(t, common.ChannelStatusManuallyDisabled, updated.ChannelInfo.MultiKeyStatusList[0])
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
