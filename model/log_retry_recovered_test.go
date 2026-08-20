package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupRetryRecoveredLogDB(t *testing.T) {
	t.Helper()
	previousDB, previousLogDB := DB, LOG_DB
	previousMainDatabaseType, previousLogDatabaseType := common.MainDatabaseType(), common.LogDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	DB, LOG_DB = db, db
	require.NoError(t, db.AutoMigrate(&Log{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
		_ = sqlDB.Close()
	})
}

func TestMarkErrorLogsRecoveredMarksOnlyMatchingErrorLogs(t *testing.T) {
	setupRetryRecoveredLogDB(t)

	requestId := "req-recovered"
	require.NoError(t, createLog(&Log{RequestId: requestId, Type: LogTypeError, ChannelId: 65,
		Content: "status_code=502", Other: common.MapToJsonStr(map[string]interface{}{"status_code": 502})}))
	require.NoError(t, createLog(&Log{RequestId: requestId, Type: LogTypeError, ChannelId: 66,
		Content: "status_code=502", Other: common.MapToJsonStr(map[string]interface{}{"status_code": 502})}))
	// 同 request_id 的消费日志和其他请求的错误日志都不能被动到
	require.NoError(t, createLog(&Log{RequestId: requestId, Type: LogTypeConsume, ChannelId: 52, Other: "{}"}))
	require.NoError(t, createLog(&Log{RequestId: "req-other", Type: LogTypeError, ChannelId: 65, Other: "{}"}))

	MarkErrorLogsRecovered(requestId)

	var marked []*Log
	require.NoError(t, LOG_DB.Where("request_id = ? AND type = ?", requestId, LogTypeError).Find(&marked).Error)
	require.Len(t, marked, 2)
	for _, entry := range marked {
		otherMap, err := common.StrToMap(entry.Other)
		require.NoError(t, err)
		assert.Equal(t, true, otherMap["retry_recovered"])
		// 原有字段保留
		assert.EqualValues(t, 502, otherMap["status_code"])
		assert.Equal(t, "status_code=502", entry.Content)
	}

	var consumeRow Log
	require.NoError(t, LOG_DB.Where("request_id = ? AND type = ?", requestId, LogTypeConsume).First(&consumeRow).Error)
	assert.Equal(t, "{}", consumeRow.Other)
	var otherRequestRow Log
	require.NoError(t, LOG_DB.Where("request_id = ?", "req-other").First(&otherRequestRow).Error)
	assert.Equal(t, "{}", otherRequestRow.Other)
}

func TestMarkErrorLogsRecoveredNoopOnEmptyRequestId(t *testing.T) {
	setupRetryRecoveredLogDB(t)
	require.NoError(t, createLog(&Log{RequestId: "req-a", Type: LogTypeError, Other: "{}"}))

	MarkErrorLogsRecovered("")

	var entry Log
	require.NoError(t, LOG_DB.Where("request_id = ?", "req-a").First(&entry).Error)
	assert.Equal(t, "{}", entry.Other)
}

func TestGetAllLogsErrorOutcomeFilter(t *testing.T) {
	setupRetryRecoveredLogDB(t)

	require.NoError(t, createLog(&Log{RequestId: "req-visible", Type: LogTypeError, Other: "{}"}))
	require.NoError(t, createLog(&Log{RequestId: "req-null-other", Type: LogTypeError}))
	require.NoError(t, createLog(&Log{RequestId: "req-saved", Type: LogTypeError,
		Other: common.MapToJsonStr(map[string]interface{}{"retry_recovered": true})}))

	collectRequestIds := func(outcome string) []string {
		logs, _, err := GetAllLogs(LogTypeError, 0, 0, "", "", nil, "", 0, 100, 0, "", "", "", outcome)
		require.NoError(t, err)
		ids := make([]string, 0, len(logs))
		for _, entry := range logs {
			ids = append(ids, entry.RequestId)
		}
		return ids
	}

	assert.ElementsMatch(t, []string{"req-visible", "req-null-other", "req-saved"}, collectRequestIds(""))
	assert.ElementsMatch(t, []string{"req-visible", "req-null-other"}, collectRequestIds(ErrorOutcomeVisible))
	assert.ElementsMatch(t, []string{"req-saved"}, collectRequestIds(ErrorOutcomeRecovered))
}
