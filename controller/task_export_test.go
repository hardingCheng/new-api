package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type taskExportResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		Items []struct {
			TaskID      string `json:"task_id"`
			RefundQuota int    `json:"refund_quota"`
		} `json:"items"`
		HasMore    bool   `json:"has_more"`
		NextCursor string `json:"next_cursor"`
	} `json:"data"`
}

func runTaskExportRequest(t *testing.T, target string) taskExportResponse {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, target, nil)
	GetAllTaskExport(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response taskExportResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func runTaskListRequest(t *testing.T, target string, userID int) taskExportResponse {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, target, nil)
	if userID > 0 {
		ctx.Set("id", userID)
		GetUserTask(ctx)
	} else {
		GetAllTask(ctx)
	}

	require.Equal(t, http.StatusOK, recorder.Code)
	var response taskExportResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func TestGetAllTaskExportRequiresBoundedTimeRange(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		message string
	}{
		{name: "missing start", target: "/api/task/export?end_timestamp=200", message: "invalid start_timestamp"},
		{name: "missing end", target: "/api/task/export?start_timestamp=100", message: "invalid end_timestamp"},
		{name: "inverted", target: "/api/task/export?start_timestamp=200&end_timestamp=100", message: "invalid time range"},
		{name: "over 31 days", target: "/api/task/export?start_timestamp=100&end_timestamp=2678501", message: "task export time range cannot exceed 31 days"},
		{name: "invalid limit", target: "/api/task/export?start_timestamp=100&end_timestamp=200&limit=5001", message: "invalid export limit; must be between 1 and 5000"},
		{name: "invalid cursor", target: "/api/task/export?start_timestamp=100&end_timestamp=200&before_id=-1", message: "invalid before_id"},
		{name: "invalid channels", target: "/api/task/export?start_timestamp=100&end_timestamp=200&channel_ids=1,invalid", message: "invalid channel_ids"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := runTaskExportRequest(t, test.target)
			assert.False(t, response.Success)
			assert.Equal(t, test.message, response.Message)
		})
	}
}

func TestGetAllTaskExportAppliesTaskLogFilters(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Task{}))

	users := []*model.User{
		{Username: "filter-alice", Password: "password1", AffCode: "filter-alice"},
		{Username: "filter-bob", Password: "password1", AffCode: "filter-bob"},
		{Username: "filter-charlie", Password: "password1", AffCode: "filter-charlie"},
	}
	for _, user := range users {
		require.NoError(t, db.Create(user).Error)
	}

	tasks := []*model.Task{
		{TaskID: "task_filter_alice", UserId: users[0].Id, ChannelId: 10, Action: "textGenerate", Status: model.TaskStatusSuccess, SubmitTime: 150, Properties: model.Properties{OriginModelName: "seedance-filter-model"}},
		{TaskID: "task_filter_bob", UserId: users[1].Id, ChannelId: 20, Action: "textGenerate", Status: model.TaskStatusSuccess, SubmitTime: 150, Properties: model.Properties{OriginModelName: "seedance-filter-model"}},
		{TaskID: "task_wrong_user", UserId: users[2].Id, ChannelId: 10, Action: "textGenerate", Status: model.TaskStatusSuccess, SubmitTime: 150, Properties: model.Properties{OriginModelName: "seedance-filter-model"}},
		{TaskID: "task_wrong_channel", UserId: users[0].Id, ChannelId: 30, Action: "textGenerate", Status: model.TaskStatusSuccess, SubmitTime: 150, Properties: model.Properties{OriginModelName: "seedance-filter-model"}},
		{TaskID: "task_wrong_action", UserId: users[0].Id, ChannelId: 10, Action: "imageToVideo", Status: model.TaskStatusSuccess, SubmitTime: 150, Properties: model.Properties{OriginModelName: "seedance-filter-model"}},
		{TaskID: "task_wrong_status", UserId: users[0].Id, ChannelId: 10, Action: "textGenerate", Status: model.TaskStatusFailure, SubmitTime: 150, Properties: model.Properties{OriginModelName: "seedance-filter-model"}},
		{TaskID: "task_wrong_model", UserId: users[0].Id, ChannelId: 10, Action: "textGenerate", Status: model.TaskStatusSuccess, SubmitTime: 150, Properties: model.Properties{OriginModelName: "other-model"}},
	}
	for _, task := range tasks {
		require.NoError(t, db.Create(task).Error)
	}

	response := runTaskExportRequest(t, "/api/task/export?start_timestamp=100&end_timestamp=200&usernames=filter-alice,filter-bob&channel_ids=10,20&action=textGenerate&model_name=seedance-filter&status=SUCCESS")
	require.True(t, response.Success, response.Message)
	require.Len(t, response.Data.Items, 2)
	assert.ElementsMatch(t, []string{"task_filter_alice", "task_filter_bob"}, []string{
		response.Data.Items[0].TaskID,
		response.Data.Items[1].TaskID,
	})

	listResponse := runTaskListRequest(t, "/api/task?p=1&page_size=10&start_timestamp=100&end_timestamp=200&usernames=filter-alice,filter-bob&channel_ids=10,20&action=textGenerate&model_name=seedance-filter&status=SUCCESS", 0)
	require.True(t, listResponse.Success, listResponse.Message)
	require.Len(t, listResponse.Data.Items, 2)
	assert.ElementsMatch(t, []string{"task_filter_alice", "task_filter_bob"}, []string{
		listResponse.Data.Items[0].TaskID,
		listResponse.Data.Items[1].TaskID,
	})

	selfResponse := runTaskListRequest(t, "/api/task?p=1&page_size=10&start_timestamp=100&end_timestamp=200&action=textGenerate&model_name=other-model&status=SUCCESS", users[0].Id)
	require.True(t, selfResponse.Success, selfResponse.Message)
	require.Len(t, selfResponse.Data.Items, 1)
	assert.Equal(t, "task_wrong_model", selfResponse.Data.Items[0].TaskID)
}

func TestGetAllTaskExportReturnsRowsWithinBoundedRange(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Task{}))
	require.NoError(t, db.Create(&model.Task{
		TaskID:     "task_export_inside",
		SubmitTime: 150,
		Status:     model.TaskStatusSuccess,
		Progress:   "100%",
	}).Error)
	require.NoError(t, db.Create(&model.Task{
		TaskID:     "task_export_outside",
		SubmitTime: 250,
		Status:     model.TaskStatusSuccess,
		Progress:   "100%",
	}).Error)

	response := runTaskExportRequest(t, "/api/task/export?start_timestamp=100&end_timestamp=200")
	require.True(t, response.Success, response.Message)
	assert.Len(t, response.Data.Items, 1)
}

func TestGetAllTaskExportPaginatesWithPrimaryKeyCursor(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Task{}))
	for _, task := range []*model.Task{
		{TaskID: "task_export_cursor_1", SubmitTime: 150, Status: model.TaskStatusSuccess, Progress: "100%"},
		{TaskID: "task_export_cursor_2", SubmitTime: 150, Status: model.TaskStatusSuccess, Progress: "100%"},
		{TaskID: "task_export_cursor_3", SubmitTime: 150, Status: model.TaskStatusSuccess, Progress: "100%"},
	} {
		require.NoError(t, db.Create(task).Error)
	}

	first := runTaskExportRequest(t, "/api/task/export?start_timestamp=100&end_timestamp=200&limit=2")
	require.True(t, first.Success, first.Message)
	require.Len(t, first.Data.Items, 2)
	assert.True(t, first.Data.HasMore)
	require.NotEmpty(t, first.Data.NextCursor)

	second := runTaskExportRequest(t, "/api/task/export?start_timestamp=100&end_timestamp=200&limit=2&before_id="+first.Data.NextCursor)
	require.True(t, second.Success, second.Message)
	assert.Len(t, second.Data.Items, 1)
	assert.False(t, second.Data.HasMore)
	assert.Empty(t, second.Data.NextCursor)
	for _, item := range first.Data.Items {
		assert.NotEqual(t, item.TaskID, second.Data.Items[0].TaskID)
	}
}

func TestGetAllTaskExportDoesNotLoadLargeTaskPayloads(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Task{}))
	require.NoError(t, db.Create(&model.Task{
		TaskID:      "task_export_large_payload",
		SubmitTime:  150,
		Status:      model.TaskStatusFailure,
		Progress:    "100%",
		Data:        []byte(`{"large":"payload"}`),
		PrivateData: model.TaskPrivateData{Key: "must-not-be-loaded", RefundQuota: 123456},
	}).Error)

	items, err := model.TaskGetAllTasksForExport(10, 0, model.SyncTaskQueryParams{StartTimestamp: 100, EndTimestamp: 200})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Empty(t, items[0].Data)
	assert.Empty(t, items[0].PrivateData.Key)
	assert.Equal(t, 123456, items[0].PrivateData.RefundQuota)

	response := runTaskExportRequest(t, "/api/task/export?start_timestamp=100&end_timestamp=200")
	require.True(t, response.Success, response.Message)
	require.Len(t, response.Data.Items, 1)
	assert.Equal(t, 123456, response.Data.Items[0].RefundQuota)
}
