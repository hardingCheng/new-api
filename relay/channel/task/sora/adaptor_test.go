package sora

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
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

func TestBuildRequestBodyJSONDurationCompatibility(t *testing.T) {
	tests := []struct {
		name             string
		body             string
		originModel      string
		upstreamModel    string
		expectedSeconds  string
		expectedDuration int
		hasDuration      bool
	}{
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
	}{
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
