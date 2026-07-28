package gemini

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newVeoTaskContext(req relaycommon.TaskSubmitReq) *gin.Context {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader("{}"))
	ctx.Set("task_request", req)
	return ctx
}

func veoRelayInfo() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "veo-3.1-generate-preview",
		},
	}
}

func TestVeoMetadataParametersRejectBillingAndUpstreamRequest(t *testing.T) {
	tests := []struct {
		name      string
		metadata  map[string]interface{}
		errorText string
	}{
		{
			name:      "zero duration",
			metadata:  map[string]interface{}{"durationSeconds": 0},
			errorText: "durationSeconds must be between",
		},
		{
			name:      "negative duration",
			metadata:  map[string]interface{}{"durationSeconds": -1},
			errorText: "durationSeconds must be between",
		},
		{
			name:      "duration above billing bound",
			metadata:  map[string]interface{}{"durationSeconds": relaycommon.MaxTaskDurationSeconds + 1},
			errorText: "durationSeconds must be between",
		},
		{
			name:      "unknown resolution",
			metadata:  map[string]interface{}{"durationSeconds": 8, "resolution": "2160p"},
			errorText: "invalid resolution",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := relaycommon.TaskSubmitReq{
				Model:    "veo-3.1-generate-preview",
				Prompt:   "animate",
				Metadata: test.metadata,
			}
			adaptor := &TaskAdaptor{}
			ctx := newVeoTaskContext(req)

			_, err := adaptor.EstimateBilling(ctx, veoRelayInfo())
			require.ErrorContains(t, err, test.errorText)

			_, err = adaptor.BuildRequestBody(ctx, veoRelayInfo())
			require.ErrorContains(t, err, test.errorText)
		})
	}
}

func TestVeoMetadataParametersMatchBillingAndUpstreamRequest(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:  "veo-3.1-generate-preview",
		Prompt: "animate",
		Metadata: map[string]interface{}{
			"durationSeconds": 8,
			"resolution":      "1080P",
			"negativePrompt":  "blur",
		},
	}
	adaptor := &TaskAdaptor{}
	ctx := newVeoTaskContext(req)

	ratios, err := adaptor.EstimateBilling(ctx, veoRelayInfo())
	require.NoError(t, err)
	assert.Equal(t, 8.0, ratios["seconds"])

	requestBody, err := adaptor.BuildRequestBody(ctx, veoRelayInfo())
	require.NoError(t, err)
	body, err := io.ReadAll(requestBody)
	require.NoError(t, err)
	var payload VeoRequestPayload
	require.NoError(t, common.Unmarshal(body, &payload))
	require.NotNil(t, payload.Parameters)
	assert.Equal(t, 8, payload.Parameters.DurationSeconds)
	assert.Equal(t, "1080p", payload.Parameters.Resolution)
	assert.Equal(t, "blur", payload.Parameters.NegativePrompt)
	assert.Equal(t, 1, payload.Parameters.SampleCount)
}
