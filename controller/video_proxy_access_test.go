package controller

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVideoProxyScopesCrossUserAccessToAdminDashboardSessions(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Task{}))
	require.NoError(t, db.Create(&model.Task{
		TaskID:   "task_owned_by_user_42",
		UserId:   42,
		Status:   model.TaskStatusInProgress,
		Progress: "50%",
	}).Error)

	tests := []struct {
		name          string
		viewerID      int
		role          int
		dashboardAuth bool
		targetUserID  int
		wantStatus    int
		wantMessage   string
	}{
		{
			name:        "task owner using API token",
			viewerID:    42,
			wantStatus:  http.StatusBadRequest,
			wantMessage: "Task is not completed yet",
		},
		{
			name:         "different regular user",
			viewerID:     7,
			role:         common.RoleCommonUser,
			targetUserID: 42,
			wantStatus:   http.StatusNotFound,
			wantMessage:  "Task not found",
		},
		{
			name:          "administrator dashboard session without owner scope",
			viewerID:      1,
			role:          common.RoleAdminUser,
			dashboardAuth: true,
			wantStatus:    http.StatusNotFound,
			wantMessage:   "Task not found",
		},
		{
			name:          "administrator dashboard session",
			viewerID:      1,
			role:          common.RoleAdminUser,
			dashboardAuth: true,
			targetUserID:  42,
			wantStatus:    http.StatusBadRequest,
			wantMessage:   "Task is not completed yet",
		},
		{
			name:         "administrator API token",
			viewerID:     1,
			role:         common.RoleAdminUser,
			targetUserID: 42,
			wantStatus:   http.StatusNotFound,
			wantMessage:  "Task not found",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			target := "/v1/videos/task_owned_by_user_42/content"
			if test.targetUserID > 0 {
				target = fmt.Sprintf("%s?user_id=%d", target, test.targetUserID)
			}
			ctx.Request = httptest.NewRequest(http.MethodGet, target, nil)
			ctx.Params = gin.Params{{Key: "task_id", Value: "task_owned_by_user_42"}}
			ctx.Set("id", test.viewerID)
			ctx.Set("role", test.role)
			if test.dashboardAuth {
				ctx.Set("session_id", fmt.Sprintf("session-%d", test.viewerID))
				ctx.Set("auth_version", int64(1))
				ctx.Set("session_version", int64(1))
			}

			VideoProxy(ctx)

			assert.Equal(t, test.wantStatus, recorder.Code)
			assert.True(t, strings.Contains(recorder.Body.String(), test.wantMessage), recorder.Body.String())
		})
	}
}

func TestWriteVideoDataURLDisablesSharedCaching(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	data := base64.StdEncoding.EncodeToString([]byte("video"))

	require.NoError(t, writeVideoDataURL(ctx, "data:video/mp4;base64,"+data))

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "private, no-store", recorder.Header().Get("Cache-Control"))
	assert.Equal(t, "video/mp4", recorder.Header().Get("Content-Type"))
}
