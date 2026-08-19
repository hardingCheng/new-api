package service

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelBreakerPenaltyTest(t *testing.T) *gorm.DB {
	t.Helper()

	originalDB := model.DB
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelBreakerLog{}))
	model.DB = db

	common.SetChannelBreakerPenaltyEnabled(true)
	common.SetChannelBreakerPenaltyOfflineEnabled(true)
	common.SetChannelBreakerPenaltyAlertOpensPerHour(10)
	common.SetChannelBreakerPenaltyOfflineConsecutiveHours(2)
	common.SetChannelBreakerPenaltyMinPoolSize(2)
	common.SetChannelBreakerExemptChannels(nil)

	t.Cleanup(func() {
		model.DB = originalDB
		common.SetChannelBreakerPenaltyEnabled(false)
		common.SetChannelBreakerPenaltyOfflineEnabled(false)
		common.SetChannelBreakerPenaltyAlertOpensPerHour(10)
		common.SetChannelBreakerPenaltyOfflineConsecutiveHours(2)
		common.SetChannelBreakerPenaltyMinPoolSize(2)
		common.SetChannelBreakerExemptChannels(nil)
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			require.NoError(t, sqlDB.Close())
		}
	})

	return db
}

// penaltyTestNow 固定在一个小时桶的中段，避免用例踩到桶边界。
func penaltyTestNow() time.Time {
	return time.Unix((500000*3600)+1800, 0)
}

func seedBreakerOpens(t *testing.T, db *gorm.DB, channelId int, keyHash string, reason string, hourOffset int64, count int) {
	t.Helper()
	bucket := penaltyTestNow().Unix() / 3600
	base := (bucket - hourOffset) * 3600
	for i := 0; i < count; i++ {
		require.NoError(t, db.Create(&model.ChannelBreakerLog{
			CreatedAt: base + int64(i),
			ChannelId: channelId,
			KeyHash:   keyHash,
			Reason:    reason,
		}).Error)
	}
}

func seedPenaltyChannel(t *testing.T, db *gorm.DB, id int, status int) {
	t.Helper()
	require.NoError(t, db.Create(&model.Channel{Id: id, Name: fmt.Sprintf("penalty-ch-%d", id), Status: status}).Error)
}

func seedPenaltyAbility(t *testing.T, db *gorm.DB, group, modelName string, channelId int, enabled bool) {
	t.Helper()
	require.NoError(t, db.Create(&model.Ability{Group: group, Model: modelName, ChannelId: channelId, Enabled: enabled}).Error)
}

func penaltyEvent(channelId int) channelBreakerPenaltyEvent {
	return channelBreakerPenaltyEvent{
		ChannelError: types.ChannelError{ChannelId: channelId, ChannelName: fmt.Sprintf("penalty-ch-%d", channelId), AutoBan: true},
		Group:        "default",
		Model:        "gpt-test",
	}
}

func TestChannelBreakerPenaltyDisabledDoesNothing(t *testing.T) {
	db := setupChannelBreakerPenaltyTest(t)
	common.SetChannelBreakerPenaltyEnabled(false)

	seedPenaltyChannel(t, db, 1, common.ChannelStatusEnabled)
	seedBreakerOpens(t, db, 1, "", model.ChannelBreakerLogReasonOpened, 0, 50)
	seedBreakerOpens(t, db, 1, "", model.ChannelBreakerLogReasonOpened, 1, 50)

	decision, err := decideChannelBreakerPenalty(penaltyEvent(1), penaltyTestNow())
	require.NoError(t, err)
	assert.False(t, decision.Alert)
	assert.False(t, decision.Offline)
}

func TestChannelBreakerPenaltyBelowThresholdByOne(t *testing.T) {
	db := setupChannelBreakerPenaltyTest(t)
	seedPenaltyChannel(t, db, 1, common.ChannelStatusEnabled)
	seedBreakerOpens(t, db, 1, "", model.ChannelBreakerLogReasonOpened, 0, 9)

	decision, err := decideChannelBreakerPenalty(penaltyEvent(1), penaltyTestNow())
	require.NoError(t, err)
	assert.False(t, decision.Alert)
	assert.False(t, decision.Offline)
	assert.Equal(t, int64(9), decision.CurrentHourOpens)
}

func TestChannelBreakerPenaltyAlertOnlyWhenPreviousHourQuiet(t *testing.T) {
	db := setupChannelBreakerPenaltyTest(t)
	seedPenaltyChannel(t, db, 1, common.ChannelStatusEnabled)
	seedBreakerOpens(t, db, 1, "", model.ChannelBreakerLogReasonOpened, 0, 10)
	seedBreakerOpens(t, db, 1, "", model.ChannelBreakerLogReasonOpened, 1, 9)

	decision, err := decideChannelBreakerPenalty(penaltyEvent(1), penaltyTestNow())
	require.NoError(t, err)
	assert.True(t, decision.Alert)
	assert.False(t, decision.Offline)
	assert.Empty(t, decision.OfflineBlockedBy)
}

func TestChannelBreakerPenaltyOfflineAfterConsecutiveHours(t *testing.T) {
	db := setupChannelBreakerPenaltyTest(t)
	seedPenaltyChannel(t, db, 1, common.ChannelStatusEnabled)
	seedPenaltyChannel(t, db, 2, common.ChannelStatusEnabled)
	seedPenaltyAbility(t, db, "default", "gpt-test", 1, true)
	seedPenaltyAbility(t, db, "default", "gpt-test", 2, true)
	seedBreakerOpens(t, db, 1, "", model.ChannelBreakerLogReasonOpened, 0, 10)
	seedBreakerOpens(t, db, 1, "", model.ChannelBreakerLogReasonOpened, 1, 10)

	decision, err := decideChannelBreakerPenalty(penaltyEvent(1), penaltyTestNow())
	require.NoError(t, err)
	assert.True(t, decision.Alert)
	assert.True(t, decision.Offline)
	assert.Empty(t, decision.OfflineBlockedBy)
}

func TestChannelBreakerPenaltyOfflineSwitchOffBlocksOffline(t *testing.T) {
	db := setupChannelBreakerPenaltyTest(t)
	common.SetChannelBreakerPenaltyOfflineEnabled(false)
	seedPenaltyChannel(t, db, 1, common.ChannelStatusEnabled)
	seedBreakerOpens(t, db, 1, "", model.ChannelBreakerLogReasonOpened, 0, 10)
	seedBreakerOpens(t, db, 1, "", model.ChannelBreakerLogReasonOpened, 1, 10)

	decision, err := decideChannelBreakerPenalty(penaltyEvent(1), penaltyTestNow())
	require.NoError(t, err)
	assert.True(t, decision.Alert)
	assert.False(t, decision.Offline)
	assert.NotEmpty(t, decision.OfflineBlockedBy)
}

func TestChannelBreakerPenaltyIgnoresProbeReopenReasons(t *testing.T) {
	db := setupChannelBreakerPenaltyTest(t)
	seedPenaltyChannel(t, db, 1, common.ChannelStatusEnabled)
	seedBreakerOpens(t, db, 1, "", model.ChannelBreakerLogReasonOpened, 0, 10)
	seedBreakerOpens(t, db, 1, "", model.ChannelBreakerLogReasonOpened, 1, 5)
	// probe 失败重开与超时不计入，与阈值校准口径一致
	seedBreakerOpens(t, db, 1, "", "channel breaker remains open after probe (0/5 successes)", 1, 20)

	decision, err := decideChannelBreakerPenalty(penaltyEvent(1), penaltyTestNow())
	require.NoError(t, err)
	assert.True(t, decision.Alert)
	assert.False(t, decision.Offline)
}

func TestChannelBreakerPenaltyPoolProtection(t *testing.T) {
	db := setupChannelBreakerPenaltyTest(t)
	seedPenaltyChannel(t, db, 1, common.ChannelStatusEnabled)
	seedPenaltyAbility(t, db, "default", "gpt-test", 1, true)
	seedBreakerOpens(t, db, 1, "", model.ChannelBreakerLogReasonOpened, 0, 10)
	seedBreakerOpens(t, db, 1, "", model.ChannelBreakerLogReasonOpened, 1, 10)

	decision, err := decideChannelBreakerPenalty(penaltyEvent(1), penaltyTestNow())
	require.NoError(t, err)
	assert.True(t, decision.Alert)
	assert.False(t, decision.Offline)
	assert.Contains(t, decision.OfflineBlockedBy, "启用渠道仅 1 条")
}

func TestChannelBreakerPenaltyPoolProtectionDisabledByZero(t *testing.T) {
	db := setupChannelBreakerPenaltyTest(t)
	common.SetChannelBreakerPenaltyMinPoolSize(0)
	seedPenaltyChannel(t, db, 1, common.ChannelStatusEnabled)
	seedBreakerOpens(t, db, 1, "", model.ChannelBreakerLogReasonOpened, 0, 10)
	seedBreakerOpens(t, db, 1, "", model.ChannelBreakerLogReasonOpened, 1, 10)

	decision, err := decideChannelBreakerPenalty(penaltyEvent(1), penaltyTestNow())
	require.NoError(t, err)
	assert.True(t, decision.Offline)
}

func TestChannelBreakerPenaltyExemptChannelNotOffline(t *testing.T) {
	db := setupChannelBreakerPenaltyTest(t)
	common.SetChannelBreakerExemptChannels([]int{1})
	seedPenaltyChannel(t, db, 1, common.ChannelStatusEnabled)
	seedBreakerOpens(t, db, 1, "", model.ChannelBreakerLogReasonOpened, 0, 10)
	seedBreakerOpens(t, db, 1, "", model.ChannelBreakerLogReasonOpened, 1, 10)

	decision, err := decideChannelBreakerPenalty(penaltyEvent(1), penaltyTestNow())
	require.NoError(t, err)
	assert.True(t, decision.Alert)
	assert.False(t, decision.Offline)
	assert.Contains(t, decision.OfflineBlockedBy, "豁免")
}

func TestChannelBreakerPenaltyManuallyDisabledChannelNotOffline(t *testing.T) {
	db := setupChannelBreakerPenaltyTest(t)
	seedPenaltyChannel(t, db, 1, common.ChannelStatusManuallyDisabled)
	seedBreakerOpens(t, db, 1, "", model.ChannelBreakerLogReasonOpened, 0, 10)
	seedBreakerOpens(t, db, 1, "", model.ChannelBreakerLogReasonOpened, 1, 10)

	decision, err := decideChannelBreakerPenalty(penaltyEvent(1), penaltyTestNow())
	require.NoError(t, err)
	assert.True(t, decision.Alert)
	assert.False(t, decision.Offline)
	assert.Contains(t, decision.OfflineBlockedBy, "启用状态")
}

func TestChannelBreakerPenaltyMissingContextBlocksOffline(t *testing.T) {
	db := setupChannelBreakerPenaltyTest(t)
	seedPenaltyChannel(t, db, 1, common.ChannelStatusEnabled)
	seedBreakerOpens(t, db, 1, "", model.ChannelBreakerLogReasonOpened, 0, 10)
	seedBreakerOpens(t, db, 1, "", model.ChannelBreakerLogReasonOpened, 1, 10)

	ev := penaltyEvent(1)
	ev.Group = ""
	decision, err := decideChannelBreakerPenalty(ev, penaltyTestNow())
	require.NoError(t, err)
	assert.True(t, decision.Alert)
	assert.False(t, decision.Offline)
	assert.Contains(t, decision.OfflineBlockedBy, "上下文")
}

func TestChannelBreakerPenaltyMultiKeyCountsPerKey(t *testing.T) {
	db := setupChannelBreakerPenaltyTest(t)
	seedPenaltyChannel(t, db, 1, common.ChannelStatusEnabled)
	seedPenaltyChannel(t, db, 2, common.ChannelStatusEnabled)
	seedPenaltyAbility(t, db, "default", "gpt-test", 1, true)
	seedPenaltyAbility(t, db, "default", "gpt-test", 2, true)
	hashA := ChannelBreakerKeyHash("key-a")
	hashB := ChannelBreakerKeyHash("key-b")
	seedBreakerOpens(t, db, 1, hashA, model.ChannelBreakerLogReasonOpened, 0, 10)
	seedBreakerOpens(t, db, 1, hashA, model.ChannelBreakerLogReasonOpened, 1, 10)
	seedBreakerOpens(t, db, 1, hashB, model.ChannelBreakerLogReasonOpened, 0, 3)

	evA := penaltyEvent(1)
	evA.ChannelError.IsMultiKey = true
	evA.ChannelError.UsingKey = "key-a"
	decisionA, err := decideChannelBreakerPenalty(evA, penaltyTestNow())
	require.NoError(t, err)
	assert.True(t, decisionA.Offline)

	evB := penaltyEvent(1)
	evB.ChannelError.IsMultiKey = true
	evB.ChannelError.UsingKey = "key-b"
	decisionB, err := decideChannelBreakerPenalty(evB, penaltyTestNow())
	require.NoError(t, err)
	assert.False(t, decisionB.Alert)
	assert.False(t, decisionB.Offline)
}

func TestChannelBreakerPenaltyAlertDeduplicatesPerHourBucket(t *testing.T) {
	setupChannelBreakerPenaltyTest(t)
	disableRedisForBreakerTest(t)

	channelError := types.ChannelError{ChannelId: 42}
	bucket := penaltyTestNow().Unix() / 3600
	assert.True(t, allowChannelBreakerPenaltyAlert(channelError, bucket))
	assert.False(t, allowChannelBreakerPenaltyAlert(channelError, bucket))
	assert.True(t, allowChannelBreakerPenaltyAlert(channelError, bucket+1))
}
