package service

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMain(m *testing.M) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		panic("failed to open test db: " + err.Error())
	}
	sqlDB, err := db.DB()
	if err != nil {
		panic("failed to get sql.DB: " + err.Error())
	}
	sqlDB.SetMaxOpenConns(1)

	model.DB = db
	model.LOG_DB = db

	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.LogConsumeEnabled = true

	if err := db.AutoMigrate(
		&model.Task{},
		&model.User{},
		&model.Token{},
		&model.Log{},
		&model.Channel{},
		&model.TopUp{},
		&model.UserSubscription{},
		&model.SubscriptionPlan{},
		&model.SystemTask{},
		&model.SystemTaskLock{},
		&model.BillingAdjustment{},
		&model.QuotaPoolAdjustment{},
		&model.SubscriptionPreConsumeRecord{},
	); err != nil {
		panic("failed to migrate: " + err.Error())
	}

	os.Exit(m.Run())
}

// ---------------------------------------------------------------------------
// Seed helpers
// ---------------------------------------------------------------------------

func truncate(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		model.DB.Exec("DELETE FROM tasks")
		model.DB.Exec("DELETE FROM users")
		model.DB.Exec("DELETE FROM tokens")
		model.DB.Exec("DELETE FROM logs")
		model.DB.Exec("DELETE FROM channels")
		model.DB.Exec("DELETE FROM top_ups")
		model.DB.Exec("DELETE FROM user_subscriptions")
		model.DB.Exec("DELETE FROM subscription_plans")
		model.DB.Exec("DELETE FROM system_task_locks")
		model.DB.Exec("DELETE FROM system_tasks")
		model.DB.Exec("DELETE FROM billing_adjustments")
		model.DB.Exec("DELETE FROM quota_pool_adjustments")
		model.DB.Exec("DELETE FROM subscription_pre_consume_records")
	})
}

func setupConcurrentBillingFileDB(t *testing.T) {
	t.Helper()
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldBatchUpdateEnabled := common.BatchUpdateEnabled
	oldMainDatabaseType := common.MainDatabaseType()
	oldLogDatabaseType := common.LogDatabaseType()

	dsn := filepath.Join(t.TempDir(), "billing-concurrency.db") + "?_busy_timeout=30000&_journal_mode=WAL"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(16)
	require.NoError(t, db.AutoMigrate(
		&model.Task{},
		&model.User{},
		&model.Token{},
		&model.Log{},
		&model.Channel{},
		&model.UserSubscription{},
		&model.SubscriptionPlan{},
		&model.BillingAdjustment{},
		&model.QuotaPoolAdjustment{},
		&model.SubscriptionPreConsumeRecord{},
	))

	model.DB = db
	model.LOG_DB = db
	require.False(t, common.RedisEnabled)
	common.BatchUpdateEnabled = false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		_ = sqlDB.Close()
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.BatchUpdateEnabled = oldBatchUpdateEnabled
		common.SetDatabaseTypes(oldMainDatabaseType, oldLogDatabaseType)
	})
}

func seedUser(t *testing.T, id int, quota int) {
	t.Helper()
	user := &model.User{Id: id, Username: "test_user", Quota: quota, Status: common.UserStatusEnabled}
	require.NoError(t, model.DB.Create(user).Error)
}

func seedToken(t *testing.T, id int, userId int, key string, remainQuota int) {
	t.Helper()
	token := &model.Token{
		Id:          id,
		UserId:      userId,
		Key:         key,
		Name:        "test_token",
		Status:      common.TokenStatusEnabled,
		RemainQuota: remainQuota,
		UsedQuota:   0,
	}
	require.NoError(t, model.DB.Create(token).Error)
}

func seedSubscription(t *testing.T, id int, userId int, amountTotal int64, amountUsed int64) {
	t.Helper()
	sub := &model.UserSubscription{
		Id:          id,
		UserId:      userId,
		AmountTotal: amountTotal,
		AmountUsed:  amountUsed,
		Status:      "active",
		StartTime:   time.Now().Unix(),
		EndTime:     time.Now().Add(30 * 24 * time.Hour).Unix(),
	}
	require.NoError(t, model.DB.Create(sub).Error)
}

func seedChannel(t *testing.T, id int) {
	t.Helper()
	ch := &model.Channel{Id: id, Name: "test_channel", Key: "sk-test", Status: common.ChannelStatusEnabled}
	require.NoError(t, model.DB.Create(ch).Error)
}

func makeTask(userId, channelId, quota, tokenId int, billingSource string, subscriptionId int) *model.Task {
	return &model.Task{
		TaskID:    "task_" + time.Now().Format("150405.000"),
		UserId:    userId,
		ChannelId: channelId,
		Quota:     quota,
		Status:    model.TaskStatus(model.TaskStatusInProgress),
		Group:     "default",
		Data:      json.RawMessage(`{}`),
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
		Properties: model.Properties{
			OriginModelName: "test-model",
		},
		PrivateData: model.TaskPrivateData{
			BillingSource:  billingSource,
			SubscriptionId: subscriptionId,
			TokenId:        tokenId,
			BillingContext: &model.TaskBillingContext{
				ModelPrice:      0.02,
				GroupRatio:      1.0,
				OriginModelName: "test-model",
			},
		},
	}
}

func TestPriceDataOtherRatiosFilterAndSnapshot(t *testing.T) {
	priceData := types.PriceData{}

	priceData.AddOtherRatio("zero", 0)
	priceData.AddOtherRatio("negative", -0.5)
	priceData.AddOtherRatio("nan", math.NaN())
	priceData.AddOtherRatio("inf", math.Inf(1))
	priceData.AddOtherRatio("one", 1)
	priceData.AddOtherRatio("positive", 2.5)

	ratios := priceData.OtherRatios()
	require.Len(t, ratios, 2)
	assert.Equal(t, 1.0, ratios["one"])
	assert.Equal(t, 2.5, ratios["positive"])
	assert.True(t, priceData.HasOtherRatio("one"))
	assert.False(t, priceData.HasOtherRatio("zero"))

	ratios["positive"] = 99
	ratios["new"] = 3
	nextSnapshot := priceData.OtherRatios()
	assert.Equal(t, 2.5, nextSnapshot["positive"])
	assert.NotContains(t, nextSnapshot, "new")
}

func TestPriceDataReplaceAndApplyOtherRatios(t *testing.T) {
	priceData := types.PriceData{}

	replaced := priceData.ReplaceOtherRatios(map[string]float64{
		"zero":     0,
		"negative": -3,
		"nan":      math.NaN(),
		"inf":      math.Inf(1),
		"one":      1,
		"duration": 2,
		"size":     1.5,
	})

	require.True(t, replaced)
	assert.Equal(t, 3.0, priceData.OtherRatioMultiplier())
	assert.Equal(t, 30.0, priceData.ApplyOtherRatiosToFloat(10))
	assert.Equal(t, 10.0, priceData.RemoveOtherRatiosFromFloat(30))
	assert.True(t, decimal.NewFromInt(30).Equal(priceData.ApplyOtherRatiosToDecimal(decimal.NewFromInt(10))))

	replaced = priceData.ReplaceOtherRatios(map[string]float64{
		"zero": 0,
		"nan":  math.NaN(),
	})

	require.False(t, replaced)
	assert.Nil(t, priceData.OtherRatios())
	assert.Equal(t, 1.0, priceData.OtherRatioMultiplier())
}

func TestTaskBillingOtherFiltersHistoricalOtherRatios(t *testing.T) {
	task := makeTask(1, 1, 100, 0, BillingSourceWallet, 0)
	task.PrivateData.BillingContext.OtherRatios = map[string]float64{
		"seconds":  2,
		"identity": 1,
		"zero":     0,
		"negative": -1,
		"nan":      math.NaN(),
		"inf":      math.Inf(1),
	}

	other := taskBillingOther(task)

	assert.Equal(t, 2.0, other["seconds"])
	assert.Equal(t, 1.0, other["identity"])
	assert.NotContains(t, other, "zero")
	assert.NotContains(t, other, "negative")
	assert.NotContains(t, other, "nan")
	assert.NotContains(t, other, "inf")
}

func TestTaskBillingContextPriceDataFiltersMultiplier(t *testing.T) {
	priceData := taskBillingContextPriceData(&model.TaskBillingContext{
		OtherRatios: map[string]float64{
			"seconds":  2,
			"size":     3,
			"identity": 1,
			"zero":     0,
			"negative": -1,
			"nan":      math.NaN(),
			"inf":      math.Inf(1),
		},
	})

	require.NotNil(t, priceData)
	assert.Equal(t, 6.0, priceData.OtherRatioMultiplier())
	assert.Equal(t, map[string]float64{
		"seconds":  2,
		"size":     3,
		"identity": 1,
	}, priceData.OtherRatios())
}

// ---------------------------------------------------------------------------
// Read-back helpers
// ---------------------------------------------------------------------------

func getUserQuota(t *testing.T, id int) int {
	t.Helper()
	var user model.User
	require.NoError(t, model.DB.Select("quota").Where("id = ?", id).First(&user).Error)
	return user.Quota
}

func getTokenRemainQuota(t *testing.T, id int) int {
	t.Helper()
	var token model.Token
	require.NoError(t, model.DB.Select("remain_quota").Where("id = ?", id).First(&token).Error)
	return token.RemainQuota
}

func getTokenUsedQuota(t *testing.T, id int) int {
	t.Helper()
	var token model.Token
	require.NoError(t, model.DB.Select("used_quota").Where("id = ?", id).First(&token).Error)
	return token.UsedQuota
}

func getSubscriptionUsed(t *testing.T, id int) int64 {
	t.Helper()
	var sub model.UserSubscription
	require.NoError(t, model.DB.Select("amount_used").Where("id = ?", id).First(&sub).Error)
	return sub.AmountUsed
}

func getLastLog(t *testing.T) *model.Log {
	t.Helper()
	var log model.Log
	err := model.LOG_DB.Order("id desc").First(&log).Error
	if err != nil {
		return nil
	}
	return &log
}

func countLogs(t *testing.T) int64 {
	t.Helper()
	var count int64
	model.LOG_DB.Model(&model.Log{}).Count(&count)
	return count
}

// ===========================================================================
// RefundTaskQuota tests
// ===========================================================================

func TestRefundTaskQuota_Wallet(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 1, 1, 1
	const initQuota, preConsumed = 10000, 3000
	const tokenRemain = 5000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-test-key", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)

	RefundTaskQuota(ctx, task, "task failed: upstream error")

	// User quota should increase by preConsumed
	assert.Equal(t, initQuota+preConsumed, getUserQuota(t, userID))

	// Token remain_quota should increase, used_quota should decrease
	assert.Equal(t, tokenRemain+preConsumed, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, -preConsumed, getTokenUsedQuota(t, tokenID))

	// A refund log should be created
	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
	assert.Equal(t, preConsumed, log.Quota)
	assert.Equal(t, "test-model", log.ModelName)
}

func TestRefundTaskQuota_Subscription(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID, subID = 2, 2, 2, 1
	const preConsumed = 2000
	const subTotal, subUsed int64 = 100000, 50000
	const tokenRemain = 8000

	seedUser(t, userID, 0)
	seedToken(t, tokenID, userID, "sk-sub-key", tokenRemain)
	seedChannel(t, channelID)
	seedSubscription(t, subID, userID, subTotal, subUsed)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceSubscription, subID)

	RefundTaskQuota(ctx, task, "subscription task failed")

	// Subscription used should decrease by preConsumed
	assert.Equal(t, subUsed-int64(preConsumed), getSubscriptionUsed(t, subID))

	// Token should also be refunded
	assert.Equal(t, tokenRemain+preConsumed, getTokenRemainQuota(t, tokenID))

	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
}

func TestRefundTaskQuota_ZeroQuota(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID = 3
	seedUser(t, userID, 5000)

	task := makeTask(userID, 0, 0, 0, BillingSourceWallet, 0)

	RefundTaskQuota(ctx, task, "zero quota task")

	// No change to user quota
	assert.Equal(t, 5000, getUserQuota(t, userID))

	// No log created
	assert.Equal(t, int64(0), countLogs(t))
}

func TestRefundTaskQuota_NoToken(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, channelID = 4, 4
	const initQuota, preConsumed = 10000, 1500

	seedUser(t, userID, initQuota)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, 0, BillingSourceWallet, 0) // TokenId=0

	RefundTaskQuota(ctx, task, "no token task failed")

	// User quota refunded
	assert.Equal(t, initQuota+preConsumed, getUserQuota(t, userID))

	// Log created
	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
}

func TestRefundTaskQuota_IsFinanciallyIdempotent(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 5, 5, 5
	const initQuota, preConsumed, tokenRemain = 10000, 2500, 7000
	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-idempotent", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	task.TaskID = "task_refund_idempotent"
	require.NoError(t, model.DB.Create(task).Error)

	require.NoError(t, RefundTaskQuota(ctx, task, "first failure"))
	require.NoError(t, RefundTaskQuota(ctx, task, "duplicate worker"))

	assert.Equal(t, initQuota+preConsumed, getUserQuota(t, userID))
	assert.Equal(t, tokenRemain+preConsumed, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, -preConsumed, getTokenUsedQuota(t, tokenID))
	var adjustments int64
	require.NoError(t, model.DB.Model(&model.BillingAdjustment{}).
		Where("adjustment_key = ?", "task-refund:"+task.TaskID).
		Count(&adjustments).Error)
	assert.Equal(t, int64(1), adjustments)
}

func TestFinalizeTaskFailureConcurrentWorkersRefundOnce(t *testing.T) {
	setupConcurrentBillingFileDB(t)
	const workers = 16
	const userID, tokenID, channelID = 51, 51, 51
	const initQuota, preConsumed, tokenRemain = 10000, 2400, 8000
	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-concurrent-refund", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	task.TaskID = "task_concurrent_refund_once"
	require.NoError(t, model.DB.Create(task).Error)

	copies := make([]*model.Task, workers)
	for i := range copies {
		copies[i] = &model.Task{}
		require.NoError(t, model.DB.First(copies[i], task.ID).Error)
	}

	start := make(chan struct{})
	errs := make(chan error, workers)
	var wins atomic.Int32
	var wg sync.WaitGroup
	wg.Add(workers)
	for _, taskCopy := range copies {
		go func(taskCopy *model.Task) {
			defer wg.Done()
			<-start
			won, err := FinalizeTaskFailure(context.Background(), taskCopy, model.TaskStatusInProgress, "concurrent upstream failure")
			if won {
				wins.Add(1)
			}
			errs <- err
		}(taskCopy)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	assert.Equal(t, int32(1), wins.Load())
	// SQLite may return SQLITE_BUSY while losing CAS workers still release their
	// read transactions. The durable pending state must finish on the next pass.
	retryPendingTaskRefunds(context.Background())
	assert.Equal(t, initQuota+preConsumed, getUserQuota(t, userID))
	assert.Equal(t, tokenRemain+preConsumed, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, -preConsumed, getTokenUsedQuota(t, tokenID))
	assert.Equal(t, int64(1), countLogs(t))

	var adjustments int64
	require.NoError(t, model.DB.Model(&model.BillingAdjustment{}).
		Where("adjustment_key = ?", "task-refund:"+task.TaskID).
		Count(&adjustments).Error)
	assert.Equal(t, int64(1), adjustments)
}

func TestApplyBillingAdjustmentConcurrentWorkersChargesOnce(t *testing.T) {
	setupConcurrentBillingFileDB(t)
	const workers = 16
	const userID, tokenID = 52, 52
	const initialQuota, charge = 10000, 1750
	seedUser(t, userID, initialQuota)
	seedToken(t, tokenID, userID, "sk-concurrent-charge", initialQuota)

	adjustment, err := model.EnsureBillingAdjustment(model.BillingAdjustmentParams{
		AdjustmentKey: "relay-settle:concurrent-charge-once",
		UserID:        userID,
		TokenID:       tokenID,
		FundingSource: BillingSourceWallet,
		FundingDelta:  charge,
		TokenDelta:    charge,
	})
	require.NoError(t, err)

	start := make(chan struct{})
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			<-start
			errs <- model.ApplyBillingAdjustment(adjustment.ID)
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	assert.Equal(t, initialQuota-charge, getUserQuota(t, userID))
	assert.Equal(t, initialQuota-charge, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, charge, getTokenUsedQuota(t, tokenID))
	var reloaded model.BillingAdjustment
	require.NoError(t, model.DB.First(&reloaded, adjustment.ID).Error)
	assert.Equal(t, model.BillingAdjustmentStatusSucceeded, reloaded.Status)
	assert.Equal(t, 1, reloaded.Attempts)
}

func TestBillingSessionRefundPersistsAndRefundsWalletOnce(t *testing.T) {
	truncate(t)
	const userID, tokenID, preConsumed = 81, 81, 2400
	seedUser(t, userID, 7600)
	seedToken(t, tokenID, userID, "sk-session-refund", 2600)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	session := &BillingSession{
		relayInfo: &relaycommon.RelayInfo{
			RequestId: "session-refund-once",
			UserId:    userID,
			TokenId:   tokenID,
		},
		funding:          &WalletFunding{},
		preConsumedQuota: preConsumed,
		tokenConsumed:    preConsumed,
	}

	require.NoError(t, session.Refund(ctx))
	require.NoError(t, session.Refund(ctx))
	assert.Equal(t, 10000, getUserQuota(t, userID))
	assert.Equal(t, 5000, getTokenRemainQuota(t, tokenID))
	var count int64
	require.NoError(t, model.DB.Model(&model.BillingAdjustment{}).
		Where("adjustment_key = ?", "relay-refund:session-refund-once").Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestBillingSessionUsesDurableReservationForWalletSettlement(t *testing.T) {
	truncate(t)
	const userID, tokenID = 91, 91
	seedUser(t, userID, 10000)
	seedToken(t, tokenID, userID, "sk-durable-session", 10000)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{
		RequestId:       "durable-wallet-session",
		UserId:          userID,
		TokenId:         tokenID,
		TokenKey:        "sk-durable-session",
		ForcePreConsume: true,
	}
	info.UserSetting.BillingPreference = "wallet_only"

	session, apiErr := NewBillingSession(ctx, info, 2000)
	require.Nil(t, apiErr)
	require.NotNil(t, session)
	assert.Equal(t, 8000, getUserQuota(t, userID))
	assert.Equal(t, 8000, getTokenRemainQuota(t, tokenID))

	var reserved model.BillingAdjustment
	require.NoError(t, model.DB.Where("adjustment_key = ?", "relay-billing:"+info.RequestId).First(&reserved).Error)
	assert.Equal(t, model.BillingAdjustmentStatusReserved, reserved.Status)
	assert.Equal(t, -2000, reserved.FundingDelta)
	assert.Equal(t, -2000, reserved.TokenDelta)

	require.NoError(t, session.Settle(1500))
	assert.Equal(t, 8500, getUserQuota(t, userID))
	assert.Equal(t, 8500, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, 1500, getTokenUsedQuota(t, tokenID))
	require.NoError(t, model.DB.First(&reserved, reserved.ID).Error)
	assert.Equal(t, model.BillingAdjustmentStatusSucceeded, reserved.Status)
	assert.Equal(t, -500, reserved.FundingDelta)
	assert.Equal(t, -500, reserved.TokenDelta)
}

func TestAbandonedBillingReservationIsRefundedWhenDue(t *testing.T) {
	truncate(t)
	const userID, tokenID = 92, 92
	seedUser(t, userID, 5000)
	seedToken(t, tokenID, userID, "sk-abandoned-session", 5000)
	reservation, err := model.ReserveBillingQuota(model.BillingReservationParams{
		RequestID:           "abandoned-wallet-session",
		UserID:              userID,
		TokenID:             tokenID,
		TokenKey:            "sk-abandoned-session",
		TokenBillingEnabled: true,
		FundingSource:       BillingSourceWallet,
		Amount:              1200,
	})
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.BillingAdjustment{}).
		Where("id = ?", reservation.Adjustment.ID).Update("next_retry_at", 0).Error)

	summary := ProcessPendingBillingAdjustments(context.Background(), 10)
	assert.Equal(t, 1, summary.Succeeded)
	assert.Equal(t, 5000, getUserQuota(t, userID))
	assert.Equal(t, 5000, getTokenRemainQuota(t, tokenID))
	assert.Zero(t, getTokenUsedQuota(t, tokenID))
	var adjustment model.BillingAdjustment
	require.NoError(t, model.DB.First(&adjustment, reservation.Adjustment.ID).Error)
	assert.Equal(t, model.BillingAdjustmentStatusSucceeded, adjustment.Status)
}

func TestBillingReservationFailureDoesNotPartiallyDeductToken(t *testing.T) {
	truncate(t)
	const userID, tokenID = 93, 93
	seedUser(t, userID, 100)
	seedToken(t, tokenID, userID, "sk-atomic-failure", 1000)

	_, err := model.ReserveBillingQuota(model.BillingReservationParams{
		RequestID:           "atomic-reservation-failure",
		UserID:              userID,
		TokenID:             tokenID,
		TokenKey:            "sk-atomic-failure",
		TokenBillingEnabled: true,
		FundingSource:       BillingSourceWallet,
		Amount:              200,
	})
	require.Error(t, err)
	assert.Equal(t, 100, getUserQuota(t, userID))
	assert.Equal(t, 1000, getTokenRemainQuota(t, tokenID))
	assert.Zero(t, getTokenUsedQuota(t, tokenID))
	var count int64
	require.NoError(t, model.DB.Model(&model.BillingAdjustment{}).
		Where("adjustment_key = ?", "relay-billing:atomic-reservation-failure").Count(&count).Error)
	assert.Zero(t, count)
}

func TestBillingReservationExtensionFailureDoesNotDeductFunding(t *testing.T) {
	truncate(t)
	const userID, tokenID = 96, 96
	seedUser(t, userID, 1000)
	seedToken(t, tokenID, userID, "sk-extension-atomic", 150)
	reservation, err := model.ReserveBillingQuota(model.BillingReservationParams{
		RequestID:           "atomic-extension-failure",
		UserID:              userID,
		TokenID:             tokenID,
		TokenKey:            "sk-extension-atomic",
		TokenBillingEnabled: true,
		FundingSource:       BillingSourceWallet,
		Amount:              100,
	})
	require.NoError(t, err)

	_, err = model.ExtendBillingReservation(reservation.Adjustment.ID, 100, true, false, "sk-extension-atomic")
	require.Error(t, err)
	assert.Equal(t, 900, getUserQuota(t, userID))
	assert.Equal(t, 50, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, 100, getTokenUsedQuota(t, tokenID))
	var adjustment model.BillingAdjustment
	require.NoError(t, model.DB.First(&adjustment, reservation.Adjustment.ID).Error)
	assert.Equal(t, -100, adjustment.FundingDelta)
	assert.Equal(t, -100, adjustment.TokenDelta)
}

func TestSubscriptionReservationAndRefundShareOneDurableLifecycle(t *testing.T) {
	truncate(t)
	const userID, tokenID, subscriptionID, planID = 97, 97, 97, 97
	seedUser(t, userID, 0)
	seedToken(t, tokenID, userID, "sk-subscription-lifecycle", 5000)
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{
		Id:               planID,
		Title:            "test plan",
		Enabled:          true,
		TotalAmount:      10000,
		QuotaResetPeriod: model.SubscriptionResetNever,
	}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{
		Id:          subscriptionID,
		UserId:      userID,
		PlanId:      planID,
		AmountTotal: 10000,
		Status:      "active",
		StartTime:   time.Now().Add(-time.Hour).Unix(),
		EndTime:     time.Now().Add(24 * time.Hour).Unix(),
	}).Error)

	reservation, err := model.ReserveBillingQuota(model.BillingReservationParams{
		RequestID:             "subscription-durable-lifecycle",
		UserID:                userID,
		TokenID:               tokenID,
		TokenKey:              "sk-subscription-lifecycle",
		TokenBillingEnabled:   true,
		FundingSource:         BillingSourceSubscription,
		Amount:                1400,
		SubscriptionModelName: "display-model",
	})
	require.NoError(t, err)
	require.NotNil(t, reservation.Subscription)
	assert.Equal(t, int64(1400), getSubscriptionUsed(t, subscriptionID))
	assert.Equal(t, 3600, getTokenRemainQuota(t, tokenID))

	adjustment, err := model.FinalizeBillingReservation(reservation.Adjustment.ID, 0, true, true)
	require.NoError(t, err)
	require.NoError(t, model.ApplyBillingAdjustment(adjustment.ID))
	assert.Zero(t, getSubscriptionUsed(t, subscriptionID))
	assert.Equal(t, 5000, getTokenRemainQuota(t, tokenID))
	assert.Zero(t, getTokenUsedQuota(t, tokenID))
	var record model.SubscriptionPreConsumeRecord
	require.NoError(t, model.DB.Where("request_id = ?", "subscription-durable-lifecycle").First(&record).Error)
	assert.Equal(t, "refunded", record.Status)
}

func TestConcurrentBillingReservationDeductsSameRequestOnce(t *testing.T) {
	setupConcurrentBillingFileDB(t)
	const workers = 12
	const userID, tokenID, amount = 94, 94, 1250
	seedUser(t, userID, 10000)
	seedToken(t, tokenID, userID, "sk-concurrent-reservation", 10000)

	start := make(chan struct{})
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			<-start
			_, err := model.ReserveBillingQuota(model.BillingReservationParams{
				RequestID:           "concurrent-reservation-once",
				UserID:              userID,
				TokenID:             tokenID,
				TokenKey:            "sk-concurrent-reservation",
				TokenBillingEnabled: true,
				FundingSource:       BillingSourceWallet,
				Amount:              amount,
			})
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	successes := 0
	for err := range errs {
		if err == nil {
			successes++
		}
	}
	assert.Greater(t, successes, 0)
	assert.Equal(t, 10000-amount, getUserQuota(t, userID))
	assert.Equal(t, 10000-amount, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, amount, getTokenUsedQuota(t, tokenID))
	var count int64
	require.NoError(t, model.DB.Model(&model.BillingAdjustment{}).
		Where("adjustment_key = ?", "relay-billing:concurrent-reservation-once").Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestBillingReconciliationDueChecksRespectBackoff(t *testing.T) {
	truncate(t)
	now := common.GetTimestamp()
	adjustment, err := model.EnsureBillingAdjustment(model.BillingAdjustmentParams{
		AdjustmentKey: "relay-settle:not-due-yet",
		UserID:        95,
		FundingSource: BillingSourceWallet,
		FundingDelta:  10,
	})
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.BillingAdjustment{}).
		Where("id = ?", adjustment.ID).Update("next_retry_at", now+3600).Error)
	pool, err := model.EnsureQuotaPoolAdjustment("pool:not-due-yet", "pool:key", 10)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.QuotaPoolAdjustment{}).
		Where("id = ?", pool.ID).Update("next_retry_at", now+3600).Error)

	assert.True(t, model.HasPendingBillingAdjustments())
	assert.True(t, model.HasPendingQuotaPoolAdjustments())
	assert.False(t, model.HasDueBillingAdjustments(now))
	assert.False(t, model.HasDueQuotaPoolAdjustments(now))
	require.NoError(t, model.DB.Model(&model.BillingAdjustment{}).
		Where("id = ?", adjustment.ID).Update("next_retry_at", now).Error)
	require.NoError(t, model.DB.Model(&model.QuotaPoolAdjustment{}).
		Where("id = ?", pool.ID).Update("next_retry_at", now).Error)
	assert.True(t, model.HasDueBillingAdjustments(now))
	assert.True(t, model.HasDueQuotaPoolAdjustments(now))
}

func TestApplySubscriptionRefundAdjustmentConcurrentWorkersRefundOnce(t *testing.T) {
	setupConcurrentBillingFileDB(t)
	const userID, tokenID, subscriptionID = 82, 82, 82
	const preConsumed, extraReserved = 2000, 500
	seedUser(t, userID, 0)
	seedToken(t, tokenID, userID, "sk-subscription-refund", 2500)
	seedSubscription(t, subscriptionID, userID, 10000, preConsumed+extraReserved)
	require.NoError(t, model.DB.Create(&model.SubscriptionPreConsumeRecord{
		RequestId:          "subscription-refund-once",
		UserId:             userID,
		UserSubscriptionId: subscriptionID,
		PreConsumed:        preConsumed,
		Status:             "consumed",
	}).Error)
	adjustment, err := model.EnsureBillingAdjustment(model.BillingAdjustmentParams{
		AdjustmentKey:                   "relay-refund:subscription-refund-once",
		UserID:                          userID,
		TokenID:                         tokenID,
		SubscriptionID:                  subscriptionID,
		SubscriptionPreConsumeRequestID: "subscription-refund-once",
		SubscriptionPreConsumed:         preConsumed,
		SubscriptionExtraReserved:       extraReserved,
		FundingSource:                   BillingSourceSubscription,
		FundingDelta:                    -(preConsumed + extraReserved),
		TokenDelta:                      -(preConsumed + extraReserved),
	})
	require.NoError(t, err)

	const workers = 20
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			<-start
			_ = model.ApplyBillingAdjustment(adjustment.ID)
		}()
	}
	close(start)
	wg.Wait()
	var reloaded model.BillingAdjustment
	require.NoError(t, model.DB.First(&reloaded, adjustment.ID).Error)
	if reloaded.Status != model.BillingAdjustmentStatusSucceeded {
		require.NoError(t, model.DB.Model(&model.BillingAdjustment{}).Where("id = ?", adjustment.ID).Update("next_retry_at", 0).Error)
		summary := ProcessPendingBillingAdjustments(context.Background(), 10)
		require.Equal(t, 1, summary.Succeeded)
	}
	assert.Zero(t, getSubscriptionUsed(t, subscriptionID))
	assert.Equal(t, 5000, getTokenRemainQuota(t, tokenID))
	var record model.SubscriptionPreConsumeRecord
	require.NoError(t, model.DB.Where("request_id = ?", "subscription-refund-once").First(&record).Error)
	assert.Equal(t, "refunded", record.Status)
}

func TestFinalizeTaskFailureRefundsFinancialQuotaWhenPoolReleaseIsPending(t *testing.T) {
	truncate(t)
	oldRedisEnabled, oldRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled, common.RDB = false, nil
	t.Cleanup(func() {
		common.RedisEnabled, common.RDB = oldRedisEnabled, oldRDB
	})

	const userID, tokenID, channelID = 8, 8, 8
	const initQuota, preConsumed, tokenRemain = 10000, 1800, 6000
	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-pool-pending", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	task.TaskID = "task_pool_release_pending"
	task.PrivateData.BillingContext.ModelQuotaPools = []ratio_setting.ModelQuotaPoolMatch{
		{RedisKey: "pool:pending", Amount: 1},
	}
	require.NoError(t, model.DB.Create(task).Error)

	won, err := FinalizeTaskFailure(context.Background(), task, model.TaskStatusInProgress, "upstream failed")
	require.NoError(t, err)
	require.True(t, won)

	assert.Equal(t, initQuota+preConsumed, getUserQuota(t, userID))
	assert.Equal(t, tokenRemain+preConsumed, getTokenRemainQuota(t, tokenID))
	var reloaded model.Task
	require.NoError(t, model.DB.First(&reloaded, task.ID).Error)
	assert.Equal(t, model.TaskBillingStatusRefunded, reloaded.BillingStatus)
	var poolAdjustment model.QuotaPoolAdjustment
	require.NoError(t, model.DB.Where("operation_key LIKE ?", "task:rollback:%").First(&poolAdjustment).Error)
	assert.Equal(t, model.QuotaPoolAdjustmentStatusPending, poolAdjustment.Status)
}

func TestProcessPendingBillingAdjustmentsRetriesAfterDependencyRecovers(t *testing.T) {
	truncate(t)
	adjustment, err := model.EnsureBillingAdjustment(model.BillingAdjustmentParams{
		AdjustmentKey: "relay-settle:retry-after-user-created",
		UserID:        88,
		FundingSource: BillingSourceWallet,
		FundingDelta:  300,
	})
	require.NoError(t, err)
	require.Error(t, model.ApplyBillingAdjustment(adjustment.ID))
	var waiting model.BillingAdjustment
	require.NoError(t, model.DB.First(&waiting, adjustment.ID).Error)
	assert.Greater(t, waiting.NextRetryAt, common.GetTimestamp())
	due, err := model.FindPendingBillingAdjustments(10)
	require.NoError(t, err)
	assert.Empty(t, due)

	seedUser(t, 88, 1000)
	require.NoError(t, model.DB.Model(&model.BillingAdjustment{}).Where("id = ?", adjustment.ID).Update("next_retry_at", 0).Error)
	summary := ProcessPendingBillingAdjustments(context.Background(), 10)
	assert.Equal(t, 1, summary.Pending)
	assert.Equal(t, 1, summary.Succeeded)
	assert.Zero(t, summary.Failed)
	assert.Equal(t, 700, getUserQuota(t, 88))

	summary = ProcessPendingBillingAdjustments(context.Background(), 10)
	assert.Zero(t, summary.Pending)
	assert.Equal(t, 700, getUserQuota(t, 88))
}

func TestPendingSuccessfulTaskSettlementRetriesAfterBillingRecovers(t *testing.T) {
	truncate(t)
	task := makeTask(89, 0, 2000, 0, BillingSourceWallet, 0)
	task.TaskID = "task_success_settlement_retry"
	task.Status = model.TaskStatusSuccess
	task.Progress = "100%"
	task.BillingStatus = model.TaskBillingStatusSettlementPending
	task.PrivateData.BillingContext.SettlementHasActualQuota = true
	task.PrivateData.BillingContext.SettlementActualQuota = 3000
	task.PrivateData.BillingContext.SettlementReason = "retry regression"
	require.NoError(t, model.DB.Create(task).Error)

	// The adjustment row is durable even though applying it fails while the user
	// row is absent, so the task marker can be cleared and reconciliation owns it.
	require.NoError(t, ApplyPendingTaskSettlement(context.Background(), task))
	var pending model.Task
	require.NoError(t, model.DB.First(&pending, task.ID).Error)
	assert.Equal(t, model.TaskBillingStatusSettled, pending.BillingStatus)

	seedUser(t, 89, 10000)
	require.NoError(t, model.DB.Model(&model.BillingAdjustment{}).
		Where("adjustment_key = ?", "task-settle:task_success_settlement_retry").
		Update("next_retry_at", 0).Error)
	summary := ProcessPendingBillingAdjustments(context.Background(), 10)
	require.Equal(t, 1, summary.Succeeded)

	require.NoError(t, model.DB.First(&pending, task.ID).Error)
	assert.Equal(t, model.TaskBillingStatusSettled, pending.BillingStatus)
	assert.Equal(t, 3000, pending.Quota)
	assert.Equal(t, 9000, getUserQuota(t, 89))
}

func TestTaskSubmissionRecordLifecycle(t *testing.T) {
	truncate(t)
	info := &relaycommon.RelayInfo{
		UserId:                6,
		UsingGroup:            "default",
		OriginModelName:       "public-video-model",
		RequestId:             "task-submission-lifecycle",
		FinalPreConsumedQuota: 1200,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId:         9,
			UpstreamModelName: "upstream-video-model",
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{
			PublicTaskID: "task_submission_lifecycle",
			Action:       "generate",
		},
		PriceData: types.PriceData{Quota: 1200},
	}

	created, err := EnsureTaskSubmissionRecord(nil, info, "video")
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusSubmitting, created.Status)
	assert.Equal(t, model.TaskBillingStatusActive, created.BillingStatus)

	err = CompleteTaskSubmissionRecord(nil, info, "video", "upstream-123", []byte(`{"status":"queued"}`), 1100)
	require.NoError(t, err)
	reloaded, found, err := model.GetByTaskId(info.UserId, info.PublicTaskID)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, model.TaskStatusNotStart, reloaded.Status)
	assert.Equal(t, "upstream-123", reloaded.PrivateData.UpstreamTaskID)
	assert.True(t, reloaded.PrivateData.UsesPublicTaskID)
	assert.Equal(t, 1100, reloaded.Quota)
	assert.Equal(t, model.TaskBillingStatusSettlementPending, reloaded.BillingStatus)
	require.NotNil(t, reloaded.PrivateData.BillingContext)
	assert.True(t, reloaded.PrivateData.BillingContext.SubmissionSettlementPending)
	assert.Equal(t, 1200, reloaded.PrivateData.BillingContext.SubmissionPreConsumedQuota)
	assert.Equal(t, 1100, reloaded.PrivateData.BillingContext.SubmissionActualQuota)
	assert.JSONEq(t, `{"status":"queued"}`, string(reloaded.Data))
}

func TestPendingTaskSubmissionSettlementReusesPreConsumeReservation(t *testing.T) {
	truncate(t)
	const userID, tokenID = 101, 101
	seedUser(t, userID, 10000)
	seedToken(t, tokenID, userID, "sk-task-submission-recovery", 10000)
	reservation, err := model.ReserveBillingQuota(model.BillingReservationParams{
		RequestID:           "task-submission-recovery",
		UserID:              userID,
		TokenID:             tokenID,
		TokenKey:            "sk-task-submission-recovery",
		TokenBillingEnabled: true,
		FundingSource:       BillingSourceWallet,
		Amount:              2000,
	})
	require.NoError(t, err)

	task := makeTask(userID, 0, 1500, tokenID, BillingSourceWallet, 0)
	task.TaskID = "task_submission_recovery"
	task.Status = model.TaskStatusNotStart
	task.BillingStatus = model.TaskBillingStatusSettlementPending
	task.PrivateData.BillingContext.SubmissionRequestID = "task-submission-recovery"
	task.PrivateData.BillingContext.SubmissionPreConsumedQuota = 2000
	task.PrivateData.BillingContext.SubmissionActualQuota = 1500
	task.PrivateData.BillingContext.SubmissionSettlementPending = true
	task.PrivateData.BillingContext.SubmissionTokenBilling = true
	require.NoError(t, model.DB.Create(task).Error)

	require.NoError(t, ApplyPendingTaskSettlement(context.Background(), task))
	assert.Equal(t, 8500, getUserQuota(t, userID))
	assert.Equal(t, 8500, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, 1500, getTokenUsedQuota(t, tokenID))

	var adjustment model.BillingAdjustment
	require.NoError(t, model.DB.First(&adjustment, reservation.Adjustment.ID).Error)
	assert.Equal(t, model.BillingAdjustmentStatusSucceeded, adjustment.Status)
	assert.Equal(t, -500, adjustment.FundingDelta)
	assert.Equal(t, -500, adjustment.TokenDelta)
	var legacyCount int64
	require.NoError(t, model.DB.Model(&model.BillingAdjustment{}).
		Where("adjustment_key = ?", "relay-settle:task-submission-recovery").Count(&legacyCount).Error)
	assert.Zero(t, legacyCount)
}

func TestPendingTaskSubmissionSettlementIsIdempotentAfterReservationApplied(t *testing.T) {
	truncate(t)
	const userID, tokenID = 102, 102
	seedUser(t, userID, 10000)
	seedToken(t, tokenID, userID, "sk-task-submission-applied", 10000)
	reservation, err := model.ReserveBillingQuota(model.BillingReservationParams{
		RequestID:           "task-submission-applied",
		UserID:              userID,
		TokenID:             tokenID,
		TokenKey:            "sk-task-submission-applied",
		TokenBillingEnabled: true,
		FundingSource:       BillingSourceWallet,
		Amount:              2000,
	})
	require.NoError(t, err)
	adjustment, err := model.FinalizeBillingReservation(reservation.Adjustment.ID, 1500, true, false)
	require.NoError(t, err)
	require.NoError(t, model.ApplyBillingAdjustment(adjustment.ID))

	task := makeTask(userID, 0, 1500, tokenID, BillingSourceWallet, 0)
	task.TaskID = "task_submission_applied"
	task.Status = model.TaskStatusNotStart
	task.BillingStatus = model.TaskBillingStatusSettlementPending
	task.PrivateData.BillingContext.SubmissionRequestID = "task-submission-applied"
	task.PrivateData.BillingContext.SubmissionPreConsumedQuota = 2000
	task.PrivateData.BillingContext.SubmissionActualQuota = 1500
	task.PrivateData.BillingContext.SubmissionSettlementPending = true
	task.PrivateData.BillingContext.SubmissionTokenBilling = true
	require.NoError(t, model.DB.Create(task).Error)

	require.NoError(t, ApplyPendingTaskSettlement(context.Background(), task))
	var reloaded model.Task
	require.NoError(t, model.DB.First(&reloaded, task.ID).Error)
	require.NoError(t, ApplyPendingTaskSettlement(context.Background(), &reloaded))
	assert.Equal(t, 8500, getUserQuota(t, userID))
	assert.Equal(t, 8500, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, 1500, getTokenUsedQuota(t, tokenID))
}

func TestTaskSubmissionPersistsZeroDeltaSettlementMarker(t *testing.T) {
	truncate(t)
	const userID, tokenID = 103, 103
	seedUser(t, userID, 10000)
	seedToken(t, tokenID, userID, "sk-task-submission-zero-delta", 10000)
	reservation, err := model.ReserveBillingQuota(model.BillingReservationParams{
		RequestID:           "task-submission-zero-delta",
		UserID:              userID,
		TokenID:             tokenID,
		TokenKey:            "sk-task-submission-zero-delta",
		TokenBillingEnabled: true,
		FundingSource:       BillingSourceWallet,
		Amount:              2000,
	})
	require.NoError(t, err)
	info := &relaycommon.RelayInfo{
		UserId:                userID,
		TokenId:               tokenID,
		UsingGroup:            "default",
		OriginModelName:       "public-video-model",
		RequestId:             "task-submission-zero-delta",
		FinalPreConsumedQuota: 2000,
		BillingSource:         BillingSourceWallet,
		Billing:               &BillingSession{},
		ChannelMeta:           &relaycommon.ChannelMeta{},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{
			PublicTaskID: "task_submission_zero_delta",
			Action:       "generate",
		},
		PriceData: types.PriceData{Quota: 2000},
	}
	_, err = EnsureTaskSubmissionRecord(nil, info, "video")
	require.NoError(t, err)
	require.NoError(t, CompleteTaskSubmissionRecord(nil, info, "video", "upstream-zero", nil, 2000))

	task, found, err := model.GetByTaskId(userID, info.PublicTaskID)
	require.NoError(t, err)
	require.True(t, found)
	require.True(t, task.PrivateData.BillingContext.SubmissionSettlementPending)
	require.NoError(t, ApplyPendingTaskSettlement(context.Background(), task))

	var adjustment model.BillingAdjustment
	require.NoError(t, model.DB.First(&adjustment, reservation.Adjustment.ID).Error)
	assert.Equal(t, model.BillingAdjustmentStatusSucceeded, adjustment.Status)
	assert.Zero(t, adjustment.FundingDelta)
	assert.Zero(t, adjustment.TokenDelta)
	assert.Equal(t, 8000, getUserQuota(t, userID))
	assert.Equal(t, 8000, getTokenRemainQuota(t, tokenID))
}

func TestFailedUnsettledTaskSubmissionRefundsReservationWithoutCredit(t *testing.T) {
	truncate(t)
	const userID, tokenID = 104, 104
	seedUser(t, userID, 10000)
	seedToken(t, tokenID, userID, "sk-task-submission-failure", 10000)
	reservation, err := model.ReserveBillingQuota(model.BillingReservationParams{
		RequestID:           "task-submission-failure",
		UserID:              userID,
		TokenID:             tokenID,
		TokenKey:            "sk-task-submission-failure",
		TokenBillingEnabled: true,
		FundingSource:       BillingSourceWallet,
		Amount:              2000,
	})
	require.NoError(t, err)

	task := makeTask(userID, 0, 2000, tokenID, BillingSourceWallet, 0)
	task.TaskID = "task_submission_failure"
	task.Status = model.TaskStatusSubmitting
	task.BillingStatus = model.TaskBillingStatusActive
	task.PrivateData.BillingContext.SubmissionRequestID = "task-submission-failure"
	task.PrivateData.BillingContext.SubmissionPreConsumedQuota = 2000
	task.PrivateData.BillingContext.SubmissionTokenBilling = true
	require.NoError(t, model.DB.Create(task).Error)

	won, err := FinalizeTaskFailure(context.Background(), task, model.TaskStatusSubmitting, "submission failed")
	require.NoError(t, err)
	require.True(t, won)
	assert.Equal(t, 10000, getUserQuota(t, userID))
	assert.Equal(t, 10000, getTokenRemainQuota(t, tokenID))
	assert.Zero(t, getTokenUsedQuota(t, tokenID))

	var reloaded model.Task
	require.NoError(t, model.DB.First(&reloaded, task.ID).Error)
	assert.Equal(t, model.TaskBillingStatusRefunded, reloaded.BillingStatus)
	require.NoError(t, RefundTaskQuota(context.Background(), &reloaded, "duplicate retry"))
	assert.Equal(t, 10000, getUserQuota(t, userID))
	assert.Equal(t, 10000, getTokenRemainQuota(t, tokenID))

	var adjustment model.BillingAdjustment
	require.NoError(t, model.DB.First(&adjustment, reservation.Adjustment.ID).Error)
	assert.Equal(t, model.BillingAdjustmentStatusSucceeded, adjustment.Status)
	var taskRefundCount int64
	require.NoError(t, model.DB.Model(&model.BillingAdjustment{}).
		Where("adjustment_key = ?", "task-refund:task_submission_failure").Count(&taskRefundCount).Error)
	assert.Zero(t, taskRefundCount)
}

func TestFailedTaskRefundsChargeWhenSubmissionReservationAlreadySettled(t *testing.T) {
	truncate(t)
	const userID, tokenID = 105, 105
	seedUser(t, userID, 10000)
	seedToken(t, tokenID, userID, "sk-task-settled-before-failure", 10000)
	reservation, err := model.ReserveBillingQuota(model.BillingReservationParams{
		RequestID:           "task-settled-before-failure",
		UserID:              userID,
		TokenID:             tokenID,
		TokenKey:            "sk-task-settled-before-failure",
		TokenBillingEnabled: true,
		FundingSource:       BillingSourceWallet,
		Amount:              2000,
	})
	require.NoError(t, err)
	adjustment, err := model.FinalizeBillingReservation(reservation.Adjustment.ID, 1500, true, false)
	require.NoError(t, err)
	require.NoError(t, model.ApplyBillingAdjustment(adjustment.ID))

	task := makeTask(userID, 0, 1500, tokenID, BillingSourceWallet, 0)
	task.TaskID = "task_settled_before_failure"
	task.Status = model.TaskStatusFailure
	task.BillingStatus = model.TaskBillingStatusRefundPending
	task.PrivateData.BillingContext.SubmissionRequestID = "task-settled-before-failure"
	task.PrivateData.BillingContext.SubmissionPreConsumedQuota = 2000
	task.PrivateData.BillingContext.SubmissionTokenBilling = true
	require.NoError(t, model.DB.Create(task).Error)

	require.NoError(t, RefundTaskQuota(context.Background(), task, "upstream task failed"))
	assert.Equal(t, 10000, getUserQuota(t, userID))
	assert.Equal(t, 10000, getTokenRemainQuota(t, tokenID))
	assert.Zero(t, getTokenUsedQuota(t, tokenID))
	var taskRefund model.BillingAdjustment
	require.NoError(t, model.DB.Where("adjustment_key = ?", "task-refund:task_settled_before_failure").First(&taskRefund).Error)
	assert.Equal(t, model.BillingAdjustmentStatusSucceeded, taskRefund.Status)
}

func TestTaskSubmissionWithoutUpstreamIDDoesNotFallbackToPublicID(t *testing.T) {
	truncate(t)
	info := &relaycommon.RelayInfo{
		UserId:      9,
		UsingGroup:  "default",
		ChannelMeta: &relaycommon.ChannelMeta{},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{
			PublicTaskID: "task_missing_upstream_id",
		},
	}
	_, err := EnsureTaskSubmissionRecord(nil, info, "video")
	require.NoError(t, err)
	require.NoError(t, CompleteTaskSubmissionRecord(nil, info, "video", "", nil, 0))

	reloaded, found, err := model.GetByTaskId(info.UserId, info.PublicTaskID)
	require.NoError(t, err)
	require.True(t, found)
	assert.Empty(t, reloaded.GetUpstreamTaskID())
}

func TestMarkTaskSubmissionFailedWritesTerminalState(t *testing.T) {
	truncate(t)
	info := &relaycommon.RelayInfo{
		UserId:      7,
		UsingGroup:  "default",
		ChannelMeta: &relaycommon.ChannelMeta{},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{
			PublicTaskID: "task_submission_failed",
		},
		PriceData: types.PriceData{Quota: 0},
	}
	_, err := EnsureTaskSubmissionRecord(nil, info, "video")
	require.NoError(t, err)

	handled, err := MarkTaskSubmissionFailed(context.Background(), info.UserId, info.PublicTaskID, "request build failed")
	require.NoError(t, err)
	require.True(t, handled)

	reloaded, found, err := model.GetByTaskId(info.UserId, info.PublicTaskID)
	require.NoError(t, err)
	require.True(t, found)
	assert.EqualValues(t, model.TaskStatusFailure, reloaded.Status)
	assert.Equal(t, "100%", reloaded.Progress)
	assert.Equal(t, "request build failed", reloaded.FailReason)
	assert.Equal(t, model.TaskBillingStatusRefunded, reloaded.BillingStatus)
	assert.NotZero(t, reloaded.FinishTime)
}

// ===========================================================================
// RecalculateTaskQuota tests
// ===========================================================================

func TestRecalculate_PositiveDelta(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 10, 10, 10
	const initQuota, preConsumed = 10000, 2000
	const actualQuota = 3000 // under-charged by 1000
	const tokenRemain = 5000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-recalc-pos", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)

	RecalculateTaskQuota(ctx, task, actualQuota, "adaptor adjustment")

	// User quota should decrease by the delta (1000 additional charge)
	assert.Equal(t, initQuota-(actualQuota-preConsumed), getUserQuota(t, userID))

	// Token should also be charged the delta
	assert.Equal(t, tokenRemain-(actualQuota-preConsumed), getTokenRemainQuota(t, tokenID))

	// task.Quota should be updated to actualQuota
	assert.Equal(t, actualQuota, task.Quota)

	// Log type should be Consume (additional charge)
	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeConsume, log.Type)
	assert.Equal(t, actualQuota-preConsumed, log.Quota)
}

func TestRecalculate_NegativeDelta(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 11, 11, 11
	const initQuota, preConsumed = 10000, 5000
	const actualQuota = 3000 // over-charged by 2000
	const tokenRemain = 5000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-recalc-neg", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)

	RecalculateTaskQuota(ctx, task, actualQuota, "adaptor adjustment")

	// User quota should increase by abs(delta) = 2000 (refund overpayment)
	assert.Equal(t, initQuota+(preConsumed-actualQuota), getUserQuota(t, userID))

	// Token should be refunded the difference
	assert.Equal(t, tokenRemain+(preConsumed-actualQuota), getTokenRemainQuota(t, tokenID))

	// task.Quota updated
	assert.Equal(t, actualQuota, task.Quota)

	// Log type should be Refund
	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
	assert.Equal(t, preConsumed-actualQuota, log.Quota)
}

func TestRecalculate_ZeroDelta(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID = 12
	const initQuota, preConsumed = 10000, 3000

	seedUser(t, userID, initQuota)

	task := makeTask(userID, 0, preConsumed, 0, BillingSourceWallet, 0)

	RecalculateTaskQuota(ctx, task, preConsumed, "exact match")

	// No change to user quota
	assert.Equal(t, initQuota, getUserQuota(t, userID))

	// No log created (delta is zero)
	assert.Equal(t, int64(0), countLogs(t))
}

func TestRecalculate_ActualQuotaZero(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID = 13
	const initQuota = 10000

	seedUser(t, userID, initQuota)

	task := makeTask(userID, 0, 5000, 0, BillingSourceWallet, 0)

	RecalculateTaskQuota(ctx, task, 0, "zero actual")

	// No change (early return)
	assert.Equal(t, initQuota, getUserQuota(t, userID))
	assert.Equal(t, int64(0), countLogs(t))
}

func TestRecalculateByTokensUsesFrozenBillingContext(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	t.Cleanup(func() {
		_ = ratio_setting.UpdateModelRatioByJSONString("{}")
	})
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"test-model":2}`))

	const userID, tokenID, channelID = 15, 15, 15
	const initQuota, preConsumed = 10000, 1000
	const tokenRemain = 5000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-frozen-ratio", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	task.PrivateData.BillingContext.ModelRatio = 0.5
	task.PrivateData.BillingContext.GroupRatio = 0.8
	task.PrivateData.BillingContext.ModelPrice = -1

	RecalculateTaskQuotaByTokens(ctx, task, 1000)

	const actualQuota = 400
	assert.Equal(t, actualQuota, task.Quota)
	assert.Equal(t, initQuota+(preConsumed-actualQuota), getUserQuota(t, userID))
	assert.Equal(t, tokenRemain+(preConsumed-actualQuota), getTokenRemainQuota(t, tokenID))
}

func TestRecalculateByTokensSkipsFrozenFixedPrice(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	t.Cleanup(func() {
		_ = ratio_setting.UpdateModelRatioByJSONString("{}")
	})
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"test-model":2}`))

	const userID, tokenID, channelID = 16, 16, 16
	const initQuota, preConsumed = 10000, 1000
	const tokenRemain = 5000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-frozen-price", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	task.PrivateData.BillingContext.ModelPrice = 0.25
	task.PrivateData.BillingContext.ModelRatio = 0

	RecalculateTaskQuotaByTokens(ctx, task, 1000)

	assert.Equal(t, preConsumed, task.Quota)
	assert.Equal(t, initQuota, getUserQuota(t, userID))
	assert.Equal(t, tokenRemain, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, int64(0), countLogs(t))
}

func TestRecalculate_Subscription_NegativeDelta(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID, subID = 14, 14, 14, 2
	const preConsumed = 5000
	const actualQuota = 2000 // over-charged by 3000
	const subTotal, subUsed int64 = 100000, 50000
	const tokenRemain = 8000

	seedUser(t, userID, 0)
	seedToken(t, tokenID, userID, "sk-sub-recalc", tokenRemain)
	seedChannel(t, channelID)
	seedSubscription(t, subID, userID, subTotal, subUsed)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceSubscription, subID)

	RecalculateTaskQuota(ctx, task, actualQuota, "subscription over-charge")

	// Subscription used should decrease by delta (refund 3000)
	assert.Equal(t, subUsed-int64(preConsumed-actualQuota), getSubscriptionUsed(t, subID))

	// Token refunded
	assert.Equal(t, tokenRemain+(preConsumed-actualQuota), getTokenRemainQuota(t, tokenID))

	assert.Equal(t, actualQuota, task.Quota)

	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
}

// ===========================================================================
// CAS + Billing integration tests
// Simulates the flow in updateVideoSingleTask (service/task_polling.go)
// ===========================================================================

// simulatePollBilling reproduces the CAS + billing logic from updateVideoSingleTask.
// It takes a persisted task (already in DB), applies the new status, and performs
// the conditional update + billing exactly as the polling loop does.
func simulatePollBilling(ctx context.Context, task *model.Task, newStatus model.TaskStatus, actualQuota int) {
	snap := task.Snapshot()

	shouldSettle := false

	task.Status = newStatus
	switch string(newStatus) {
	case model.TaskStatusSuccess:
		task.Progress = "100%"
		task.FinishTime = 9999
		shouldSettle = true
	case model.TaskStatusFailure:
		_, _ = FinalizeTaskFailure(ctx, task, snap.Status, "upstream error")
		return
	default:
		task.Progress = "50%"
	}

	isDone := task.Status == model.TaskStatus(model.TaskStatusSuccess) || task.Status == model.TaskStatus(model.TaskStatusFailure)
	if isDone && snap.Status != task.Status {
		won, err := task.UpdateWithStatus(snap.Status)
		if err != nil {
			shouldSettle = false
		} else if !won {
			shouldSettle = false
		}
	} else if !snap.Equal(task.Snapshot()) {
		_, _ = task.UpdateWithStatus(snap.Status)
	}

	if shouldSettle && actualQuota > 0 {
		RecalculateTaskQuota(ctx, task, actualQuota, "test settle")
	}
}

func TestCASGuardedRefund_Win(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 20, 20, 20
	const initQuota, preConsumed = 10000, 4000
	const tokenRemain = 6000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-cas-refund-win", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	task.Status = model.TaskStatus(model.TaskStatusInProgress)
	require.NoError(t, model.DB.Create(task).Error)

	simulatePollBilling(ctx, task, model.TaskStatus(model.TaskStatusFailure), 0)

	// CAS wins: task in DB should now be FAILURE
	var reloaded model.Task
	require.NoError(t, model.DB.First(&reloaded, task.ID).Error)
	assert.EqualValues(t, model.TaskStatusFailure, reloaded.Status)

	// Refund should have happened
	assert.Equal(t, initQuota+preConsumed, getUserQuota(t, userID))
	assert.Equal(t, tokenRemain+preConsumed, getTokenRemainQuota(t, tokenID))

	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
}

func TestCASGuardedRefund_Lose(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 21, 21, 21
	const initQuota, preConsumed = 10000, 4000
	const tokenRemain = 6000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-cas-refund-lose", tokenRemain)
	seedChannel(t, channelID)

	// Create task with IN_PROGRESS in DB
	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	task.Status = model.TaskStatus(model.TaskStatusInProgress)
	require.NoError(t, model.DB.Create(task).Error)

	// Simulate another process already transitioning to FAILURE
	model.DB.Model(&model.Task{}).Where("id = ?", task.ID).Update("status", model.TaskStatusFailure)

	// Our process still has the old in-memory state (IN_PROGRESS) and tries to transition
	// task.Status is still IN_PROGRESS in the snapshot
	simulatePollBilling(ctx, task, model.TaskStatus(model.TaskStatusFailure), 0)

	// CAS lost: user quota should NOT change (no double refund)
	assert.Equal(t, initQuota, getUserQuota(t, userID))
	assert.Equal(t, tokenRemain, getTokenRemainQuota(t, tokenID))

	// No billing log should be created
	assert.Equal(t, int64(0), countLogs(t))
}

func TestCASGuardedCleanupZeroQuotaQueuesPoolRollbackWhenRedisUnavailable(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	oldRedisEnabled, oldRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled, common.RDB = false, nil
	t.Cleanup(func() {
		common.RedisEnabled, common.RDB = oldRedisEnabled, oldRDB
	})

	task := makeTask(1, 1, 0, 0, BillingSourceWallet, 0)
	task.Status = model.TaskStatus(model.TaskStatusInProgress)
	task.PrivateData.BillingContext.ModelQuotaPools = []ratio_setting.ModelQuotaPoolMatch{
		{RedisKey: "pool:requests", Amount: 1},
	}
	require.NoError(t, model.DB.Create(task).Error)

	simulatePollBilling(ctx, task, model.TaskStatus(model.TaskStatusFailure), 0)

	assert.Empty(t, task.PrivateData.BillingContext.ModelQuotaPools)
	var reloaded model.Task
	require.NoError(t, model.DB.First(&reloaded, task.ID).Error)
	assert.Equal(t, model.TaskBillingStatusRefunded, reloaded.BillingStatus)
	var poolAdjustment model.QuotaPoolAdjustment
	require.NoError(t, model.DB.Where("operation_key LIKE ?", "task:rollback:%").First(&poolAdjustment).Error)
	assert.Equal(t, model.QuotaPoolAdjustmentStatusPending, poolAdjustment.Status)
}

func TestCASGuardedSettle_Win(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 22, 22, 22
	const initQuota, preConsumed = 10000, 5000
	const actualQuota = 3000 // over-charged, should get partial refund
	const tokenRemain = 8000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-cas-settle-win", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	task.Status = model.TaskStatus(model.TaskStatusInProgress)
	require.NoError(t, model.DB.Create(task).Error)

	simulatePollBilling(ctx, task, model.TaskStatus(model.TaskStatusSuccess), actualQuota)

	// CAS wins: task should be SUCCESS
	var reloaded model.Task
	require.NoError(t, model.DB.First(&reloaded, task.ID).Error)
	assert.EqualValues(t, model.TaskStatusSuccess, reloaded.Status)

	// Settlement should refund the over-charge (5000 - 3000 = 2000 back to user)
	assert.Equal(t, initQuota+(preConsumed-actualQuota), getUserQuota(t, userID))
	assert.Equal(t, tokenRemain+(preConsumed-actualQuota), getTokenRemainQuota(t, tokenID))

	// task.Quota should be updated to actualQuota
	assert.Equal(t, actualQuota, task.Quota)
}

func TestNonTerminalUpdate_NoBilling(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, channelID = 23, 23
	const initQuota, preConsumed = 10000, 3000

	seedUser(t, userID, initQuota)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, 0, BillingSourceWallet, 0)
	task.Status = model.TaskStatus(model.TaskStatusInProgress)
	task.Progress = "20%"
	require.NoError(t, model.DB.Create(task).Error)

	// Simulate a non-terminal poll update (still IN_PROGRESS, progress changed)
	simulatePollBilling(ctx, task, model.TaskStatus(model.TaskStatusInProgress), 0)

	// User quota should NOT change
	assert.Equal(t, initQuota, getUserQuota(t, userID))

	// No billing log
	assert.Equal(t, int64(0), countLogs(t))

	// Task progress should be updated in DB
	var reloaded model.Task
	require.NoError(t, model.DB.First(&reloaded, task.ID).Error)
	assert.Equal(t, "50%", reloaded.Progress)
}

// ===========================================================================
// Mock adaptor for settleTaskBillingOnComplete tests
// ===========================================================================

type mockAdaptor struct {
	adjustReturn int
}

func (m *mockAdaptor) Init(_ *relaycommon.RelayInfo) {}
func (m *mockAdaptor) FetchTask(string, string, map[string]any, string) (*http.Response, error) {
	return nil, nil
}
func (m *mockAdaptor) ParseTaskResult([]byte) (*relaycommon.TaskInfo, error) { return nil, nil }
func (m *mockAdaptor) AdjustBillingOnComplete(_ *model.Task, _ *relaycommon.TaskInfo) int {
	return m.adjustReturn
}

// ===========================================================================
// PerCallBilling tests — settleTaskBillingOnComplete
// ===========================================================================

func TestSettle_PerCallBilling_SkipsAdaptorAdjust(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 30, 30, 30
	const initQuota, preConsumed = 10000, 5000
	const tokenRemain = 8000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-percall-adaptor", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	task.PrivateData.BillingContext.PerCallBilling = true

	adaptor := &mockAdaptor{adjustReturn: 2000}
	taskResult := &relaycommon.TaskInfo{Status: model.TaskStatusSuccess}

	settleTaskBillingOnComplete(ctx, adaptor, task, taskResult)

	// Per-call: no adjustment despite adaptor returning 2000
	assert.Equal(t, initQuota, getUserQuota(t, userID))
	assert.Equal(t, tokenRemain, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, preConsumed, task.Quota)
	assert.Equal(t, int64(0), countLogs(t))
}

func TestSettle_PerCallBilling_SkipsTotalTokens(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 31, 31, 31
	const initQuota, preConsumed = 10000, 4000
	const tokenRemain = 7000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-percall-tokens", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	task.PrivateData.BillingContext.PerCallBilling = true

	adaptor := &mockAdaptor{adjustReturn: 0}
	taskResult := &relaycommon.TaskInfo{Status: model.TaskStatusSuccess, TotalTokens: 9999}

	settleTaskBillingOnComplete(ctx, adaptor, task, taskResult)

	// Per-call: no recalculation by tokens
	assert.Equal(t, initQuota, getUserQuota(t, userID))
	assert.Equal(t, tokenRemain, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, preConsumed, task.Quota)
	assert.Equal(t, int64(0), countLogs(t))
}

func TestSettle_NonPerCallBilling_AppliesAdaptorAdjustment(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 32, 32, 32
	const initQuota, preConsumed = 10000, 5000
	const adaptorQuota = 3000
	const tokenRemain = 8000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-nonpercall-adj", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	// PerCallBilling defaults to false

	adaptor := &mockAdaptor{adjustReturn: adaptorQuota}
	taskResult := &relaycommon.TaskInfo{Status: model.TaskStatusSuccess}

	settleTaskBillingOnComplete(ctx, adaptor, task, taskResult)

	// Non-per-call: adaptor adjustment applies (refund 2000)
	assert.Equal(t, initQuota+(preConsumed-adaptorQuota), getUserQuota(t, userID))
	assert.Equal(t, tokenRemain+(preConsumed-adaptorQuota), getTokenRemainQuota(t, tokenID))
	assert.Equal(t, adaptorQuota, task.Quota)

	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
}

func TestRollbackModelQuotaPoolFromRelayInfo(t *testing.T) {
	info := &relaycommon.RelayInfo{
		ModelQuotaPools: []ratio_setting.ModelQuotaPoolMatch{
			{RedisKey: "pool:a", Amount: 2},
			{RedisKey: "pool:a", Amount: 3},
			{RedisKey: "pool:b", Amount: 4},
			{RedisKey: "pool:ignored", Amount: 0},
		},
	}

	consumed := modelQuotaPoolConsumedFromMatches(info.ModelQuotaPools)
	assert.Equal(t, int64(5), consumed["pool:a"])
	assert.Equal(t, int64(4), consumed["pool:b"])
	assert.NotContains(t, consumed, "pool:ignored")
}

func TestRollbackTaskModelQuotaPoolQueuesDurablyWithoutRedis(t *testing.T) {
	truncate(t)
	oldRedisEnabled := common.RedisEnabled
	oldRDB := common.RDB
	common.RedisEnabled = false
	common.RDB = nil
	t.Cleanup(func() {
		common.RedisEnabled = oldRedisEnabled
		common.RDB = oldRDB
	})

	task := makeTask(1, 1, 0, 0, BillingSourceWallet, 0)
	task.PrivateData.BillingContext.ModelQuotaPools = []ratio_setting.ModelQuotaPoolMatch{
		{RedisKey: "pool:a", Amount: 1},
	}

	err := RollbackTaskModelQuotaPool(task)
	require.NoError(t, err)
	assert.Empty(t, task.PrivateData.BillingContext.ModelQuotaPools)
	var adjustment model.QuotaPoolAdjustment
	require.NoError(t, model.DB.Where("operation_key LIKE ?", "task:rollback:%").First(&adjustment).Error)
	assert.Equal(t, int64(-1), adjustment.Delta)
	assert.Equal(t, model.QuotaPoolAdjustmentStatusPending, adjustment.Status)
}

func TestAdjustTaskModelQuotaPoolWithAdjusterUpdatesOnlySuccessfulAdjustments(t *testing.T) {
	task := makeTask(1, 1, 0, 0, BillingSourceWallet, 0)
	task.PrivateData.BillingContext.ModelQuotaPools = []ratio_setting.ModelQuotaPoolMatch{
		{
			Metric:    ratio_setting.ModelQuotaPoolMetricQuota,
			RedisKey:  "pool:quota",
			Amount:    2000,
			UsedAfter: 2000,
			Remaining: 8000,
		},
		{
			Metric:    ratio_setting.ModelQuotaPoolMetricQuota,
			RedisKey:  "pool:failed",
			Amount:    2000,
			UsedAfter: 2000,
			Remaining: 8000,
		},
		{
			Metric:   ratio_setting.ModelQuotaPoolMetricRequests,
			RedisKey: "pool:requests",
			Amount:   1,
		},
	}

	adjusted := map[string]int64{}
	adjustTaskModelQuotaPoolWithAdjuster(task, ratio_setting.ModelQuotaPoolMetricQuota, 3000, func(key string, delta int64) bool {
		adjusted[key] = delta
		return key != "pool:failed"
	})

	require.Equal(t, int64(1000), adjusted["pool:quota"])
	require.Equal(t, int64(1000), adjusted["pool:failed"])
	require.NotContains(t, adjusted, "pool:requests")

	matches := task.PrivateData.BillingContext.ModelQuotaPools
	require.Equal(t, int64(3000), matches[0].Amount)
	require.Equal(t, int64(3000), matches[0].UsedAfter)
	require.Equal(t, int64(7000), matches[0].Remaining)

	require.Equal(t, int64(2000), matches[1].Amount)
	require.Equal(t, int64(2000), matches[1].UsedAfter)
	require.Equal(t, int64(8000), matches[1].Remaining)
}

func TestTaskModelQuotaPoolQueuesDuplicateRedisKeyRulesSeparately(t *testing.T) {
	truncate(t)
	oldRedisEnabled, oldRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled, common.RDB = false, nil
	t.Cleanup(func() {
		common.RedisEnabled, common.RDB = oldRedisEnabled, oldRDB
	})
	task := makeTask(1, 1, 100, 0, BillingSourceWallet, 0)
	task.TaskID = "task_duplicate_pool_rules"
	task.PrivateData.BillingContext.ModelQuotaPools = []ratio_setting.ModelQuotaPoolMatch{
		{Metric: ratio_setting.ModelQuotaPoolMetricQuota, RedisKey: "pool:shared", Amount: 100},
		{Metric: ratio_setting.ModelQuotaPoolMetricQuota, RedisKey: "pool:shared", Amount: 100},
	}

	require.NoError(t, AdjustTaskModelQuotaPoolQuota(task, 50))
	var adjustments []model.QuotaPoolAdjustment
	require.NoError(t, model.DB.Order("id").Find(&adjustments).Error)
	require.Len(t, adjustments, 2)
	assert.Equal(t, int64(-50), adjustments[0].Delta)
	assert.Equal(t, int64(-50), adjustments[1].Delta)
	assert.NotEqual(t, adjustments[0].OperationKey, adjustments[1].OperationKey)
}
