package doubao

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildRequestBodyPreservesSeedance25AutoDuration(t *testing.T) {
	setting := operation_setting.GetGeneralSetting()
	original := setting.Seedance25AutoDurationEnabled
	setting.Seedance25AutoDurationEnabled = true
	defer func() { setting.Seedance25AutoDurationEnabled = original }()

	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(
		http.MethodPost,
		"/v1/video/generations",
		bytes.NewBufferString(`{"model":"seedance-2.5-fast-720p","prompt":"test","duration":-1}`),
	)
	context.Request.Header.Set("Content-Type", "application/json")
	info := &relaycommon.RelayInfo{
		OriginModelName: "seedance-2.5-fast-720p",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "doubao-seedance-2-5-pro-250715",
			IsModelMapped:     true,
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}
	adaptor := &TaskAdaptor{}

	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))
	requestBody, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	body, err := io.ReadAll(requestBody)
	require.NoError(t, err)

	var payload struct {
		Model    string `json:"model"`
		Duration *int   `json:"duration"`
	}
	require.NoError(t, common.Unmarshal(body, &payload))
	assert.Equal(t, "doubao-seedance-2-5-pro-250715", payload.Model)
	require.NotNil(t, payload.Duration)
	assert.Equal(t, -1, *payload.Duration)
}

func TestEstimateBillingUsesMinimumForSeedance25AutoDuration(t *testing.T) {
	setting := operation_setting.GetGeneralSetting()
	original := setting.Seedance25AutoDurationEnabled
	setting.Seedance25AutoDurationEnabled = true
	defer func() { setting.Seedance25AutoDurationEnabled = original }()

	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(
		http.MethodPost,
		"/v1/video/generations",
		bytes.NewBufferString(`{"model":"seedance-2.5-fast-720p","prompt":"test","duration":-1}`),
	)
	context.Request.Header.Set("Content-Type", "application/json")
	info := &relaycommon.RelayInfo{
		OriginModelName: "seedance-2.5-fast-720p",
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
	}
	adaptor := &TaskAdaptor{}

	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))
	ratios, err := adaptor.EstimateBilling(context, info)
	require.NoError(t, err)
	assert.Equal(t, 4.0, ratios["seconds"])
}

func TestEstimateBillingRecognizesSeedance25RoutingAlias(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(
		http.MethodPost,
		"/v1/video/generations",
		bytes.NewBufferString(`{"model":"public-video-alias","prompt":"test","duration":20}`),
	)
	context.Request.Header.Set("Content-Type", "application/json")
	info := &relaycommon.RelayInfo{
		OriginModelName:  "public-video-alias",
		RoutingModelName: "seedance-2.5-fast-720p",
		TaskRelayInfo:    &relaycommon.TaskRelayInfo{},
	}
	adaptor := &TaskAdaptor{}

	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))
	ratios, err := adaptor.EstimateBilling(context, info)
	require.NoError(t, err)
	assert.Equal(t, 20.0, ratios["seconds"])
}
