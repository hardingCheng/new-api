package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPrepareRelayErrorForResponseRedactsUpstreamForbiddenDetails(t *testing.T) {
	apiErr := types.WithOpenAIError(types.OpenAIError{
		Message: "User has been banned by upstream account provider",
		Type:    "upstream_error",
		Code:    "account_banned",
	}, http.StatusForbidden)

	publicErr := prepareRelayErrorForResponse(apiErr, "req-403")
	openAIError := publicErr.ToOpenAIError()

	require.Equal(t, http.StatusForbidden, publicErr.StatusCode)
	require.Equal(t, "Insufficient account balance (request id: req-403)", openAIError.Message)
	require.Equal(t, types.ErrorCodeBadResponseStatusCode, publicErr.GetErrorCode())
	require.NotContains(t, openAIError.Message, "banned")
}

func TestPrepareRelayErrorForResponseKeepsLocalForbiddenMessage(t *testing.T) {
	apiErr := types.NewErrorWithStatusCode(
		errors.New("token has no access to this model"),
		types.ErrorCodeAccessDenied,
		http.StatusForbidden,
	)

	publicErr := prepareRelayErrorForResponse(apiErr, "req-local")

	require.Equal(t, "token has no access to this model (request id: req-local)", publicErr.ToOpenAIError().Message)
}

func TestRespondTaskErrorRedactsUpstreamForbiddenDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	taskErr := &taskdto.TaskError{
		Code:       "fail_to_fetch_task",
		Message:    `{"error":{"message":"upstream account secret"}}`,
		StatusCode: http.StatusForbidden,
	}

	respondTaskError(ctx, taskErr)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.JSONEq(t, `{"code":"fail_to_fetch_task","message":"Insufficient account balance","data":null}`, recorder.Body.String())
}

func TestRespondTaskErrorKeepsLocalForbiddenMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	taskErr := &taskdto.TaskError{
		Code:       "access_denied",
		Message:    "token has no access to this model",
		StatusCode: http.StatusForbidden,
		LocalError: true,
	}

	respondTaskError(ctx, taskErr)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Contains(t, recorder.Body.String(), "token has no access to this model")
	require.NotContains(t, recorder.Body.String(), "Insufficient account balance")
}
