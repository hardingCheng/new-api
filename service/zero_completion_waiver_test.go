package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func waiverStreamStatus(reason relaycommon.StreamEndReason) *relaycommon.StreamStatus {
	status := relaycommon.NewStreamStatus()
	status.SetEndReason(reason, nil)
	return status
}

func TestShouldWaiveZeroCompletionQuota(t *testing.T) {
	common.SetZeroCompletionNoChargeEnabled(true)
	t.Cleanup(func() { common.SetZeroCompletionNoChargeEnabled(false) })

	usage := &dto.Usage{PromptTokens: 100}

	tests := []struct {
		name        string
		relayInfo   *relaycommon.RelayInfo
		summary     textQuotaSummary
		originUsage *dto.Usage
		want        bool
	}{
		{
			name:      "流式上游超时零补全_免除",
			relayInfo: &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeChatCompletions, IsStream: true, StreamStatus: waiverStreamStatus(relaycommon.StreamEndReasonTimeout)},
			want:      true,
		},
		{
			name:      "流式扫描错误零补全_免除",
			relayInfo: &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeResponses, IsStream: true, StreamStatus: waiverStreamStatus(relaycommon.StreamEndReasonScannerErr)},
			want:      true,
		},
		{
			name:      "客户端主动断开_不免",
			relayInfo: &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeChatCompletions, IsStream: true, StreamStatus: waiverStreamStatus(relaycommon.StreamEndReasonClientGone)},
			want:      false,
		},
		{
			name:      "流式正常结束_不免",
			relayInfo: &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeChatCompletions, IsStream: true, StreamStatus: waiverStreamStatus(relaycommon.StreamEndReasonDone)},
			want:      false,
		},
		{
			name:      "流式无状态记录_不免",
			relayInfo: &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeChatCompletions, IsStream: true},
			want:      false,
		},
		{
			name:      "非流式上游未返回usage_免除",
			relayInfo: &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeChatCompletions},
			want:      true,
		},
		{
			name:        "非流式上游返回了usage_不免",
			relayInfo:   &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeChatCompletions},
			originUsage: usage,
			want:        false,
		},
		{
			name:      "有补全输出_不免",
			relayInfo: &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeChatCompletions, IsStream: true, StreamStatus: waiverStreamStatus(relaycommon.StreamEndReasonTimeout)},
			summary:   textQuotaSummary{CompletionTokens: 5},
			want:      false,
		},
		{
			name:      "有工具附加费_不免",
			relayInfo: &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeChatCompletions, IsStream: true, StreamStatus: waiverStreamStatus(relaycommon.StreamEndReasonTimeout)},
			summary:   textQuotaSummary{ToolCallSurchargeQuota: decimal.NewFromInt(5)},
			want:      false,
		},
		{
			name:      "embeddings请求_不免",
			relayInfo: &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeEmbeddings},
			want:      false,
		},
		{
			name:      "rerank请求_不免",
			relayInfo: &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeRerank},
			want:      false,
		},
		{
			name:      "Claude消息请求_免除",
			relayInfo: &relaycommon.RelayInfo{RelayFormat: types.RelayFormatClaude},
			want:      true,
		},
		{
			name:      "Gemini生成请求_免除",
			relayInfo: &relaycommon.RelayInfo{RelayFormat: types.RelayFormatGemini, RequestURLPath: "/v1beta/models/gemini-pro:generateContent"},
			want:      true,
		},
		{
			name:      "Gemini向量请求_不免",
			relayInfo: &relaycommon.RelayInfo{RelayFormat: types.RelayFormatGemini, RequestURLPath: "/v1beta/models/gemini-embedding:embedContent"},
			want:      false,
		},
		{
			name:      "Gemini批量向量请求_不免",
			relayInfo: &relaycommon.RelayInfo{RelayFormat: types.RelayFormatGemini, RequestURLPath: "/v1beta/models/gemini-embedding:batchEmbedContents"},
			want:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, shouldWaiveZeroCompletionQuota(tt.relayInfo, &tt.summary, tt.originUsage))
		})
	}
}

func TestShouldWaiveZeroCompletionQuotaDisabledByDefault(t *testing.T) {
	common.SetZeroCompletionNoChargeEnabled(false)
	relayInfo := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeChatCompletions, IsStream: true, StreamStatus: waiverStreamStatus(relaycommon.StreamEndReasonTimeout)}
	summary := textQuotaSummary{}
	assert.False(t, shouldWaiveZeroCompletionQuota(relayInfo, &summary, nil))
}

func TestAttachZeroCompletionWaiverNestsUnderAdminInfo(t *testing.T) {
	other := map[string]interface{}{}
	attachZeroCompletionWaiver(other, 1234)

	adminInfo, ok := other["admin_info"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, true, adminInfo["zero_completion_no_charge"])
	assert.Equal(t, 1234, adminInfo["zero_completion_waived_quota"])

	// 已存在 admin_info 时并入而非覆盖
	other2 := map[string]interface{}{"admin_info": map[string]interface{}{"use_channel": []string{"7"}}}
	attachZeroCompletionWaiver(other2, 5)
	adminInfo2 := other2["admin_info"].(map[string]interface{})
	assert.Equal(t, []string{"7"}, adminInfo2["use_channel"])
	assert.Equal(t, true, adminInfo2["zero_completion_no_charge"])
}
