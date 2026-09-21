package service

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	hosttypes "github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCalculateTextQuotaSummaryAppliesEffortRatio(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	usage := &dto.Usage{PromptTokens: 1000, CompletionTokens: 500}
	newInfo := func(priceData hosttypes.PriceData) *relaycommon.RelayInfo {
		return &relaycommon.RelayInfo{
			RelayFormat:             types.RelayFormatClaude,
			FinalRequestRelayFormat: types.RelayFormatClaude,
			OriginModelName:         "claude-opus-5",
			PriceData:               priceData,
			StartTime:               time.Now(),
		}
	}
	base := hosttypes.PriceData{
		ModelRatio:      1,
		CompletionRatio: 1,
		GroupRatioInfo:  hosttypes.GroupRatioInfo{GroupRatio: 1},
	}
	baseline := calculateTextQuotaSummary(ctx, newInfo(base), usage)
	require.Equal(t, 1500, baseline.Quota)

	surcharged := base
	surcharged.AddOtherRatio(hosttypes.OtherRatioKeyEffort, 2)
	doubled := calculateTextQuotaSummary(ctx, newInfo(surcharged), usage)
	require.Equal(t, baseline.Quota*2, doubled.Quota)
	// 分组倍率照旧,日志里的 group_ratio 不会被倍率污染
	require.Equal(t, baseline.GroupRatio, doubled.GroupRatio)
}

func TestGenerateTextOtherInfoRecordsEffortRatio(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	newInfo := func(priceData hosttypes.PriceData, reasoningEffort string) *relaycommon.RelayInfo {
		return &relaycommon.RelayInfo{
			RelayFormat:             types.RelayFormatClaude,
			FinalRequestRelayFormat: types.RelayFormatClaude,
			OriginModelName:         "claude-opus-5",
			PriceData:               priceData,
			StartTime:               time.Now(),
			ReasoningEffort:         reasoningEffort,
			ChannelMeta:             &relaycommon.ChannelMeta{},
			Request: &dto.ClaudeRequest{
				Model:        "claude-opus-5",
				OutputConfig: json.RawMessage(`{"effort":"max"}`),
			},
		}
	}
	base := hosttypes.PriceData{
		ModelRatio:      1,
		CompletionRatio: 1,
		GroupRatioInfo:  hosttypes.GroupRatioInfo{GroupRatio: 1},
	}

	t.Run("no surcharge → no field", func(t *testing.T) {
		other := GenerateTextOtherInfo(ctx, newInfo(base, "max"), 1, 1, 1, 0, 0, -1, -1)
		_, ok := other["effort_ratio"]
		require.False(t, ok)
		require.Equal(t, "max", other["reasoning_effort"])
	})

	surcharged := base
	surcharged.AddOtherRatio(hosttypes.OtherRatioKeyEffort, 2)

	t.Run("surcharge is logged next to the effort", func(t *testing.T) {
		other := GenerateTextOtherInfo(ctx, newInfo(surcharged, "max"), 1, 1, 1, 0, 0, -1, -1)
		require.Equal(t, 2.0, other["effort_ratio"])
		require.Equal(t, "max", other["reasoning_effort"])
	})

	t.Run("pass-through blanked effort is backfilled from the request", func(t *testing.T) {
		other := GenerateTextOtherInfo(ctx, newInfo(surcharged, ""), 1, 1, 1, 0, 0, -1, -1)
		require.Equal(t, 2.0, other["effort_ratio"])
		require.Equal(t, "max", other["reasoning_effort"])
	})
}
