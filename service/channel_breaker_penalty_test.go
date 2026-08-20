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

func penaltyChannelError(channelId int) types.ChannelError {
	return types.ChannelError{ChannelId: channelId, ChannelName: fmt.Sprintf("penalty-ch-%d", channelId), AutoBan: true}
}

func TestChannelBreakerPenaltyDisabledDoesNothing(t *testing.T) {
	db := setupChannelBreakerPenaltyTest(t)
	common.SetChannelBreakerPenaltyEnabled(false)

	seedPenaltyChannel(t, db, 1, common.ChannelStatusEnabled)
	seedBreakerOpens(t, db, 1, "", model.ChannelBreakerLogReasonOpened, 0, 50)
	seedBreakerOpens(t, db, 1, "", model.ChannelBreakerLogReasonOpened, 1, 50)

	decision, err := decideChannelBreakerPenalty(penaltyChannelError(1), penaltyTestNow())
	require.NoError(t, err)
	assert.False(t, decision.Alert)
	assert.False(t, decision.Offline)
}

func TestChannelBreakerPenaltyBelowThresholdByOne(t *testing.T) {
	db := setupChannelBreakerPenaltyTest(t)
	seedPenaltyChannel(t, db, 1, common.ChannelStatusEnabled)
	seedBreakerOpens(t, db, 1, "", model.ChannelBreakerLogReasonOpened, 0, 9)

	decision, err := decideChannelBreakerPenalty(penaltyChannelError(1), penaltyTestNow())
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

	decision, err := decideChannelBreakerPenalty(penaltyChannelError(1), penaltyTestNow())
	require.NoError(t, err)
	assert.True(t, decision.Alert)
	assert.False(t, decision.Offline)
	assert.Empty(t, decision.OfflineBlockedBy)
}

func TestChannelBreakerPenaltyOfflineAfterConsecutiveHours(t *testing.T) {
	db := setupChannelBreakerPenaltyTest(t)
	seedPenaltyChannel(t, db, 1, common.ChannelStatusEnabled)
	seedBreakerOpens(t, db, 1, "", model.ChannelBreakerLogReasonOpened, 0, 10)
	seedBreakerOpens(t, db, 1, "", model.ChannelBreakerLogReasonOpened, 1, 10)

	decision, err := decideChannelBreakerPenalty(penaltyChannelError(1), penaltyTestNow())
	require.NoError(t, err)
	assert.True(t, decision.Alert)
	assert.True(t, decision.Offline)
	assert.Empty(t, decision.OfflineBlockedBy)
}

func TestChannelBreakerPenaltyCountsProbeReopenReasons(t *testing.T) {
	db := setupChannelBreakerPenaltyTest(t)
	seedPenaltyChannel(t, db, 1, common.ChannelStatusEnabled)
	// 慢性坏渠道的打开事件多为探测失败/超时重开，三类都要计入
	seedBreakerOpens(t, db, 1, "", model.ChannelBreakerLogReasonOpened, 0, 3)
	seedBreakerOpens(t, db, 1, "", model.ChannelBreakerLogReasonReopenedPrefix+" (0/5 successes)", 0, 4)
	seedBreakerOpens(t, db, 1, "", model.ChannelBreakerLogReasonProbeTimeoutPrefix+" (0/3 successes, 2/5 completed)", 0, 3)
	// 立即禁用的日志行不属于打开事件，不计入
	seedBreakerOpens(t, db, 1, "", "命中立即禁用规则「全局默认规则」(status_code=403)", 0, 20)

	decision, err := decideChannelBreakerPenalty(penaltyChannelError(1), penaltyTestNow())
	require.NoError(t, err)
	assert.Equal(t, int64(10), decision.CurrentHourOpens)
	assert.True(t, decision.Alert)
}

func TestChannelBreakerPenaltyOfflineSwitchOffBlocksOffline(t *testing.T) {
	db := setupChannelBreakerPenaltyTest(t)
	common.SetChannelBreakerPenaltyOfflineEnabled(false)
	seedPenaltyChannel(t, db, 1, common.ChannelStatusEnabled)
	seedBreakerOpens(t, db, 1, "", model.ChannelBreakerLogReasonOpened, 0, 10)
	seedBreakerOpens(t, db, 1, "", model.ChannelBreakerLogReasonOpened, 1, 10)

	decision, err := decideChannelBreakerPenalty(penaltyChannelError(1), penaltyTestNow())
	require.NoError(t, err)
	assert.True(t, decision.Alert)
	assert.False(t, decision.Offline)
	assert.NotEmpty(t, decision.OfflineBlockedBy)
}

func TestChannelBreakerPenaltyExemptChannelNotOffline(t *testing.T) {
	db := setupChannelBreakerPenaltyTest(t)
	common.SetChannelBreakerExemptChannels([]int{1})
	seedPenaltyChannel(t, db, 1, common.ChannelStatusEnabled)
	seedBreakerOpens(t, db, 1, "", model.ChannelBreakerLogReasonOpened, 0, 10)
	seedBreakerOpens(t, db, 1, "", model.ChannelBreakerLogReasonOpened, 1, 10)

	decision, err := decideChannelBreakerPenalty(penaltyChannelError(1), penaltyTestNow())
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

	decision, err := decideChannelBreakerPenalty(penaltyChannelError(1), penaltyTestNow())
	require.NoError(t, err)
	assert.True(t, decision.Alert)
	assert.False(t, decision.Offline)
	assert.Contains(t, decision.OfflineBlockedBy, "启用状态")
}

func TestChannelBreakerPenaltyMultiKeyCountsPerKey(t *testing.T) {
	db := setupChannelBreakerPenaltyTest(t)
	seedPenaltyChannel(t, db, 1, common.ChannelStatusEnabled)
	hashA := ChannelBreakerKeyHash("key-a")
	hashB := ChannelBreakerKeyHash("key-b")
	seedBreakerOpens(t, db, 1, hashA, model.ChannelBreakerLogReasonOpened, 0, 10)
	seedBreakerOpens(t, db, 1, hashA, model.ChannelBreakerLogReasonOpened, 1, 10)
	seedBreakerOpens(t, db, 1, hashB, model.ChannelBreakerLogReasonOpened, 0, 3)

	ceA := penaltyChannelError(1)
	ceA.IsMultiKey = true
	ceA.UsingKey = "key-a"
	decisionA, err := decideChannelBreakerPenalty(ceA, penaltyTestNow())
	require.NoError(t, err)
	assert.True(t, decisionA.Offline)

	ceB := penaltyChannelError(1)
	ceB.IsMultiKey = true
	ceB.UsingKey = "key-b"
	decisionB, err := decideChannelBreakerPenalty(ceB, penaltyTestNow())
	require.NoError(t, err)
	assert.False(t, decisionB.Alert)
	assert.False(t, decisionB.Offline)
}

func TestChannelBreakerPenaltyPoolAudit(t *testing.T) {
	db := setupChannelBreakerPenaltyTest(t)
	for id := 1; id <= 3; id++ {
		seedPenaltyChannel(t, db, id, common.ChannelStatusEnabled)
	}

	// 池 3 条、下限 2：摘 1 剩 2，放行
	seedPenaltyAbility(t, db, "default", "gpt-test", 1, true)
	seedPenaltyAbility(t, db, "default", "gpt-test", 2, true)
	seedPenaltyAbility(t, db, "default", "gpt-test", 3, true)
	reason, err := channelBreakerPenaltyPoolBlockReason(1, 2)
	require.NoError(t, err)
	assert.Empty(t, reason)

	// 池 2 条、下限 2：摘 1 剩 1 < 2，拦截（剩余量口径，不是摘除前口径）
	seedPenaltyAbility(t, db, "vip", "claude-x", 1, true)
	seedPenaltyAbility(t, db, "vip", "claude-x", 2, true)
	reason, err = channelBreakerPenaltyPoolBlockReason(1, 2)
	require.NoError(t, err)
	assert.Contains(t, reason, "vip")
	assert.Contains(t, reason, "claude-x")
	assert.Contains(t, reason, "仅剩 1 条")

	// 任一池不达标即拦截：default 池充足救不了 vip 池
	reason, err = channelBreakerPenaltyPoolBlockReason(2, 2)
	require.NoError(t, err)
	assert.NotEmpty(t, reason)
}

func TestChannelBreakerPenaltyPoolAuditDisabledByZero(t *testing.T) {
	db := setupChannelBreakerPenaltyTest(t)
	seedPenaltyChannel(t, db, 1, common.ChannelStatusEnabled)
	seedPenaltyAbility(t, db, "default", "gpt-test", 1, true)

	reason, err := channelBreakerPenaltyPoolBlockReason(1, 0)
	require.NoError(t, err)
	assert.Empty(t, reason)
}

func TestChannelBreakerPenaltyPoolAuditIgnoresDisabledAbilities(t *testing.T) {
	db := setupChannelBreakerPenaltyTest(t)
	seedPenaltyChannel(t, db, 1, common.ChannelStatusEnabled)
	seedPenaltyChannel(t, db, 2, common.ChannelStatusEnabled)
	seedPenaltyAbility(t, db, "default", "gpt-test", 1, false)
	seedPenaltyAbility(t, db, "default", "gpt-test", 2, true)

	// 渠道 1 没有启用中的能力，无池可保护，放行
	reason, err := channelBreakerPenaltyPoolBlockReason(1, 2)
	require.NoError(t, err)
	assert.Empty(t, reason)
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
