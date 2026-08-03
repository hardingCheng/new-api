package vertex

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMetadataDurationRejectedByVertexBillingAndUpstreamRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader("{}"))
	ctx.Set("task_request", relaycommon.TaskSubmitReq{
		Model:  "veo-3.1-generate-preview",
		Prompt: "animate",
		Metadata: map[string]interface{}{
			"durationSeconds": relaycommon.MaxTaskDurationSeconds + 1,
		},
	})
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "veo-3.1-generate-preview",
		},
	}
	adaptor := &TaskAdaptor{}

	_, err := adaptor.EstimateBilling(ctx, info)
	require.ErrorContains(t, err, "durationSeconds must be between")

	_, err = adaptor.BuildRequestBody(ctx, info)
	require.ErrorContains(t, err, "durationSeconds must be between")
}
