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
	"github.com/QuantumNous/new-api/types"
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
		PriceData:       types.PriceData{UsePrice: true},
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
	}
	adaptor := &TaskAdaptor{}

	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))
	ratios, err := adaptor.EstimateBillingValidated(context, info)
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
		PriceData:        types.PriceData{UsePrice: true},
		TaskRelayInfo:    &relaycommon.TaskRelayInfo{},
	}
	adaptor := &TaskAdaptor{}

	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))
	ratios, err := adaptor.EstimateBillingValidated(context, info)
	require.NoError(t, err)
	assert.Equal(t, 20.0, ratios["seconds"])
}

// 按 token 定价的官方线：预扣不带 seconds（结算按上游 token 重算会连乘 OtherRatios），
// 只带参考视频折扣；分辨率按模型名后缀下发；对客模型名去掉档位后缀查价表。
func TestEstimateBillingPerTokenPricingSkipsSecondsAndAppliesVideoInputRatio(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, testCase := range []struct {
		name           string
		body           string
		wantSeconds    bool
		wantVideoRatio float64
		wantResolution string
	}{
		{
			name:           "text to video keeps base price and sends the suffix resolution",
			body:           `{"model":"seedance-2.0-mini-720p","prompt":"海边","duration":5}`,
			wantResolution: "720p",
		},
		{
			name:           "reference video in content gets the video-input price",
			body:           `{"model":"seedance-2.0-fast-480p","prompt":"海边","duration":5,"content":[{"type":"text","text":"海边"},{"type":"video_url","video_url":{"url":"https://media.example/ref.mp4"}}]}`,
			wantVideoRatio: 22.0 / 37.0,
			wantResolution: "480p",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			context.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", bytes.NewBufferString(testCase.body))
			context.Request.Header.Set("Content-Type", "application/json")
			var request struct {
				Model string `json:"model"`
			}
			require.NoError(t, common.Unmarshal([]byte(testCase.body), &request))
			info := &relaycommon.RelayInfo{
				OriginModelName: request.Model,
				PriceData:       types.PriceData{UsePrice: false, ModelRatio: 11.5},
				ChannelMeta: &relaycommon.ChannelMeta{
					UpstreamModelName: "ep-20261028221749-67rht",
					IsModelMapped:     true,
				},
				TaskRelayInfo: &relaycommon.TaskRelayInfo{},
			}
			adaptor := &TaskAdaptor{}
			require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))

			ratios, err := adaptor.EstimateBillingValidated(context, info)
			require.NoError(t, err)
			_, hasSeconds := ratios["seconds"]
			assert.Equal(t, testCase.wantSeconds, hasSeconds)
			if testCase.wantVideoRatio > 0 {
				assert.InDelta(t, testCase.wantVideoRatio, ratios["video_input"], 1e-9)
			} else {
				assert.NotContains(t, ratios, "video_input")
			}

			requestBody, err := adaptor.BuildRequestBody(context, info)
			require.NoError(t, err)
			body, err := io.ReadAll(requestBody)
			require.NoError(t, err)
			var payload struct {
				Resolution string `json:"resolution"`
			}
			require.NoError(t, common.Unmarshal(body, &payload))
			assert.Equal(t, testCase.wantResolution, payload.Resolution)
		})
	}
}
