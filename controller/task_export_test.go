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
		Items []any `json:"items"`
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
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := runTaskExportRequest(t, test.target)
			assert.False(t, response.Success)
			assert.Equal(t, test.message, response.Message)
		})
	}
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
