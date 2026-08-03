package controller

import (
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

func TestUpdateOptionRejectsPublicModelConflictingWithEnabledModel(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "video-a",
		ChannelId: 1,
		Enabled:   true,
	}).Error)

	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(
		http.MethodPut,
		"/api/option/",
		strings.NewReader(`{"key":"UserModelView","value":"{\"rules\":[{\"user_id\":42,\"aliases\":[{\"public_model\":\"video-a\",\"target_model\":\"video-b\",\"reference_video\":\"allowed\"}]}]}"}`),
	)

	UpdateOption(context)

	assert.Equal(t, http.StatusOK, response.Code)
	var payload struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
	assert.False(t, payload.Success)
	assert.Equal(t, `user_id 42 public model "video-a" conflicts with an existing model`, payload.Message)
}
