package sora

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTaskRequestContext(t *testing.T, body []byte, contentType string) *gin.Context {
	t.Helper()

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", contentType)
	t.Cleanup(func() {
		common.CleanupBodyStorage(ctx)
	})
	return ctx
}

func TestValidateRemixRequestRejectsOutOfRangeEffectiveDuration(t *testing.T) {
	ctx := newTaskRequestContext(t, []byte(`{"prompt":"test","seconds":"999999","duration":5}`), "application/json")
	info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
	info.Action = constant.TaskActionRemix

	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(ctx, info)

	require.NotNil(t, taskErr)
	assert.Equal(t, "invalid_seconds", taskErr.Code)
	assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
}

func TestBuildRequestBodyJSONDurationCompatibility(t *testing.T) {
	tests := []struct {
		name             string
		body             string
		originModel      string
		upstreamModel    string
		expectedSeconds  string
		expectedDuration int
		hasDuration      bool
		autoDuration     bool
	}{
		{
			name:             "seedance 2.5 preserves automatic duration",
			body:             `{"model":"seedance-2.5-fast-720p","prompt":"test","duration":-1}`,
			originModel:      "seedance-2.5-fast-720p",
			upstreamModel:    "provider-video-model",
			expectedSeconds:  "-1",
			expectedDuration: -1,
			hasDuration:      true,
			autoDuration:     true,
		},
		{
			name:             "prism adds numeric duration",
			body:             `{"model":"prism-3.0-fast-480p","prompt":"test","seconds":"5"}`,
			originModel:      "prism-3.0-fast-480p",
			upstreamModel:    "provider-video-model",
			expectedSeconds:  "5",
			expectedDuration: 5,
			hasDuration:      true,
		},
		{
			name:             "prism uses larger conflicting duration",
			body:             `{"model":"prism-3.0-fast-480p","prompt":"test","seconds":"5","duration":8}`,
			originModel:      "prism-3.0-fast-480p",
			upstreamModel:    "provider-video-model",
			expectedSeconds:  "8",
			expectedDuration: 8,
			hasDuration:      true,
		},
		{
			name:             "prism applies upper duration bound",
			body:             `{"model":"prism-3.0-fast-480p","prompt":"test","seconds":"20"}`,
			originModel:      "prism-3.0-fast-480p",
			upstreamModel:    "provider-video-model",
			expectedSeconds:  "15",
			expectedDuration: 15,
			hasDuration:      true,
		},
		{
			name:             "prism applies lower duration bound",
			body:             `{"model":"prism-3.0-fast-480p","prompt":"test","seconds":"2"}`,
			originModel:      "prism-3.0-fast-480p",
			upstreamModel:    "provider-video-model",
			expectedSeconds:  "4",
			expectedDuration: 4,
			hasDuration:      true,
		},
		{
			name:            "standard sora keeps seconds protocol",
			body:            `{"model":"sora-2","prompt":"test","seconds":"5"}`,
			originModel:     "sora-2",
			upstreamModel:   "sora-2",
			expectedSeconds: "5",
			hasDuration:     false,
		},
		{
			name:             "standard sora preserves explicit duration protocol",
			body:             `{"model":"sora-2","prompt":"test","seconds":"5","duration":8}`,
			originModel:      "sora-2",
			upstreamModel:    "sora-2",
			expectedSeconds:  "8",
			expectedDuration: 8,
			hasDuration:      true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setting := operation_setting.GetGeneralSetting()
			original := setting.Seedance25AutoDurationEnabled
			setting.Seedance25AutoDurationEnabled = test.autoDuration
			defer func() { setting.Seedance25AutoDurationEnabled = original }()

			ctx := newTaskRequestContext(t, []byte(test.body), "application/json")
			info := &relaycommon.RelayInfo{
				OriginModelName: test.originModel,
				ChannelMeta: &relaycommon.ChannelMeta{
					UpstreamModelName: test.upstreamModel,
				},
				TaskRelayInfo: &relaycommon.TaskRelayInfo{},
			}
			adaptor := &TaskAdaptor{}

			require.Nil(t, adaptor.ValidateRequestAndSetAction(ctx, info))
			requestBody, err := adaptor.BuildRequestBody(ctx, info)
			require.NoError(t, err)
			body, err := io.ReadAll(requestBody)
			require.NoError(t, err)

			var payload struct {
				Seconds  string `json:"seconds"`
				Duration *int   `json:"duration"`
			}
			require.NoError(t, common.Unmarshal(body, &payload))
			assert.Equal(t, test.expectedSeconds, payload.Seconds)
			if test.hasDuration {
				require.NotNil(t, payload.Duration)
				assert.Equal(t, test.expectedDuration, *payload.Duration)
			} else {
				assert.Nil(t, payload.Duration)
			}
		})
	}
}

func TestBuildRequestBodyMultipartDurationCompatibility(t *testing.T) {
	tests := []struct {
		name             string
		model            string
		seconds          string
		expectedSeconds  string
		expectedDuration string
		autoDuration     bool
	}{
		{
			name:             "seedance 2.5 sends automatic duration in both fields",
			model:            "seedance-2.5-fast-720p",
			seconds:          "-1",
			expectedSeconds:  "-1",
			expectedDuration: "-1",
			autoDuration:     true,
		},
		{
			name:             "prism sends both duration fields",
			model:            "prism-3.0-fast-480p",
			seconds:          "5",
			expectedSeconds:  "5",
			expectedDuration: "5",
		},
		{
			name:             "standard sora keeps seconds protocol",
			model:            "sora-2",
			seconds:          "5",
			expectedSeconds:  "5",
			expectedDuration: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setting := operation_setting.GetGeneralSetting()
			original := setting.Seedance25AutoDurationEnabled
			setting.Seedance25AutoDurationEnabled = test.autoDuration
			defer func() { setting.Seedance25AutoDurationEnabled = original }()

			var input bytes.Buffer
			writer := multipart.NewWriter(&input)
			require.NoError(t, writer.WriteField("model", test.model))
			require.NoError(t, writer.WriteField("prompt", "test"))
			require.NoError(t, writer.WriteField("seconds", test.seconds))
			require.NoError(t, writer.Close())

			ctx := newTaskRequestContext(t, input.Bytes(), writer.FormDataContentType())
			info := &relaycommon.RelayInfo{
				OriginModelName: test.model,
				ChannelMeta: &relaycommon.ChannelMeta{
					UpstreamModelName: "provider-video-model",
				},
				TaskRelayInfo: &relaycommon.TaskRelayInfo{},
			}
			adaptor := &TaskAdaptor{}

			require.Nil(t, adaptor.ValidateRequestAndSetAction(ctx, info))
			requestBody, err := adaptor.BuildRequestBody(ctx, info)
			require.NoError(t, err)
			output, err := io.ReadAll(requestBody)
			require.NoError(t, err)

			outputRequest := httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(output))
			outputRequest.Header.Set("Content-Type", ctx.GetHeader("Content-Type"))
			require.NoError(t, outputRequest.ParseMultipartForm(1<<20))
			t.Cleanup(func() {
				if outputRequest.MultipartForm != nil {
					_ = outputRequest.MultipartForm.RemoveAll()
				}
			})
			assert.Equal(t, test.expectedSeconds, outputRequest.FormValue("seconds"))
			assert.Equal(t, test.expectedDuration, outputRequest.FormValue("duration"))
		})
	}
}

func TestSoraBuildRequestBodyReturnsReplayablePassThroughBody(t *testing.T) {
	payload := []byte("opaque-sora-request-body")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(payload))
	c.Request.Header.Set("Content-Type", "application/octet-stream")
	defer common.CleanupBodyStorage(c)

	info := &relaycommon.RelayInfo{}
	body, err := (&TaskAdaptor{}).BuildRequestBody(c, info)
	require.NoError(t, err)
	replayable, ok := body.(common.ReplayableBody)
	require.True(t, ok)

	sent, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.Equal(t, payload, sent)
	assert.EqualValues(t, len(payload), replayable.Size())

	replayBody, err := replayable.NewReader()
	require.NoError(t, err)
	replay, err := io.ReadAll(replayBody)
	require.NoError(t, err)
	require.NoError(t, replayBody.Close())
	assert.Equal(t, payload, replay)
}

func TestBuildRequestBodyMergesReferenceImages(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		originModel   string
		expectedImage interface{}
		expectMerged  bool
	}{
		{
			name:          "single image stays a string",
			body:          `{"model":"seedance-2.0-mini-480p","prompt":"test","image":"https://example.com/a.png","seconds":"4"}`,
			originModel:   "seedance-2.0-mini-480p",
			expectedImage: "https://example.com/a.png",
			expectMerged:  true,
		},
		{
			name:          "image array reaches upstream as array",
			body:          `{"model":"seedance-2.0-mini-480p","prompt":"test","image":["https://example.com/a.png","https://example.com/b.png"],"seconds":"4"}`,
			originModel:   "seedance-2.0-mini-480p",
			expectedImage: []interface{}{"https://example.com/a.png", "https://example.com/b.png"},
			expectMerged:  true,
		},
		{
			name:          "images array is folded into image",
			body:          `{"model":"seedance-2.0-mini-480p","prompt":"test","images":["https://example.com/c.png","https://example.com/d.png"],"seconds":"4"}`,
			originModel:   "seedance-2.0-mini-480p",
			expectedImage: []interface{}{"https://example.com/c.png", "https://example.com/d.png"},
			expectMerged:  true,
		},
		{
			name:          "input_reference array is folded into image",
			body:          `{"model":"seedance-2.0-mini-480p","prompt":"test","input_reference":["https://example.com/e.png","https://example.com/f.png"],"seconds":"4"}`,
			originModel:   "seedance-2.0-mini-480p",
			expectedImage: []interface{}{"https://example.com/e.png", "https://example.com/f.png"},
			expectMerged:  true,
		},
		{
			name:         "non-seedance body is left untouched",
			body:         `{"model":"omni-fast","prompt":"test","images":["https://example.com/g.png","https://example.com/h.png"],"seconds":"4"}`,
			originModel:  "omni-fast",
			expectMerged: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := newTaskRequestContext(t, []byte(test.body), "application/json")
			info := &relaycommon.RelayInfo{
				OriginModelName: test.originModel,
				ChannelMeta: &relaycommon.ChannelMeta{
					UpstreamModelName: "provider-video-model",
				},
				TaskRelayInfo: &relaycommon.TaskRelayInfo{},
			}
			adaptor := &TaskAdaptor{}

			require.Nil(t, adaptor.ValidateRequestAndSetAction(ctx, info))
			requestBody, err := adaptor.BuildRequestBody(ctx, info)
			require.NoError(t, err)
			body, err := io.ReadAll(requestBody)
			require.NoError(t, err)

			var payload map[string]interface{}
			require.NoError(t, common.Unmarshal(body, &payload))

			if !test.expectMerged {
				// 其他视频渠道各有自己的参考图字段协议，不能被 seedance 的归并改写
				assert.NotContains(t, payload, "image")
				assert.Equal(t, []interface{}{"https://example.com/g.png", "https://example.com/h.png"}, payload["images"])
				return
			}

			assert.Equal(t, test.expectedImage, payload["image"])
			assert.NotContains(t, payload, "images")
			assert.NotContains(t, payload, "input_reference")
		})
	}
}
