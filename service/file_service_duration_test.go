package service

import (
	"math"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVideoDurationSecondsForBilling(t *testing.T) {
	tests := []struct {
		name     string
		duration float64
		want     int
		wantErr  bool
	}{
		{name: "rounds up", duration: 1.2, want: 2},
		{name: "allows maximum", duration: relaycommon.MaxTaskDurationSeconds, want: relaycommon.MaxTaskDurationSeconds},
		{name: "rejects infinity", duration: math.Inf(1), wantErr: true},
		{name: "rejects NaN", duration: math.NaN(), wantErr: true},
		{name: "rejects oversized duration", duration: relaycommon.MaxTaskDurationSeconds + 0.1, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			seconds, err := videoDurationSecondsForBilling(test.duration)
			if test.wantErr {
				require.Error(t, err)
				assert.Zero(t, seconds)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, seconds)
		})
	}
}

func TestSumReferenceVideoDurationSecondsRejectsUnreadableMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/videos", nil)

	seconds, err := SumReferenceVideoDurationSeconds(ctx, []string{"data:video/mp4;base64,%%%"})

	require.Error(t, err)
	assert.Zero(t, seconds)
}
