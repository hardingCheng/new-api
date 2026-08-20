package service

import (
	"testing"
	"time"

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

// waiverStreamInfo 构造流式请求的 relayInfo；delivered=true 表示已有上游数据块送达客户端。
func waiverStreamInfo(mode int, reason relaycommon.StreamEndReason, delivered bool) *relaycommon.RelayInfo {
	start := time.Unix(1700000000, 0)
	info := &relaycommon.RelayInfo{
		RelayMode:    mode,
		IsStream:     true,
		StartTime:    start,
		StreamStatus: waiverStreamStatus(reason),
	}
	if delivered {
		info.FirstResponseTime = start.Add(time.Second)
	}
	return info
}

func TestShouldWaiveZeroCompletionQuota(t *testing.T) {
	common.SetZeroCompletionNoChargeEnabled(true)
	t.Cleanup(func() { common.SetZeroCompletionNoChargeEnabled(false) })

	usage := &dto.Usage{PromptTokens: 100}
	chat := relayconstant.RelayModeChatCompletions

	tests := []struct {
		name        string
		relayInfo   *relaycommon.RelayInfo
		summary     textQuotaSummary
		originUsage *dto.Usage
		want        bool
	}{
		{
			name:      "流式上游超时且无内容送达_免除",
			relayInfo: waiverStreamInfo(chat, relaycommon.StreamEndReasonTimeout, false),
			want:      true,
		},
		{
			name:      "流式扫描错误且无内容送达_免除",
			relayInfo: waiverStreamInfo(relayconstant.RelayModeResponses, relaycommon.StreamEndReasonScannerErr, false),
			want:      true,
		},
		{
			name:      "流式HandlerStop且无内容送达_免除",
			relayInfo: waiverStreamInfo(chat, relaycommon.StreamEndReasonHandlerStop, false),
			want:      true,
		},
		{
			name:      "流式空白完成_正常收流但零输出_免除",
			relayInfo: waiverStreamInfo(chat, relaycommon.StreamEndReasonDone, false),
			want:      true,
		},
		{
			name:      "流式错误但已送达部分内容_不免",
			relayInfo: waiverStreamInfo(chat, relaycommon.StreamEndReasonTimeout, true),
			want:      false,
		},
		{
			name:      "客户端主动断开_不免",
			relayInfo: waiverStreamInfo(chat, relaycommon.StreamEndReasonClientGone, false),
			want:      false,
		},
		{
			name:      "流式无状态记录_不免",
			relayInfo: &relaycommon.RelayInfo{RelayMode: chat, IsStream: true, StartTime: time.Unix(1700000000, 0)},
			want:      false,
		},
		{
			name:      "非流式上游未返回usage_免除",
			relayInfo: &relaycommon.RelayInfo{RelayMode: chat},
			want:      true,
		},
		{
			name:        "非流式上游返回了usage_不免",
			relayInfo:   &relaycommon.RelayInfo{RelayMode: chat},
			originUsage: usage,
			want:        false,
		},
		{
			name:      "有补全输出_不免",
			relayInfo: waiverStreamInfo(chat, relaycommon.StreamEndReasonTimeout, false),
			summary:   textQuotaSummary{CompletionTokens: 5},
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
	relayInfo := waiverStreamInfo(relayconstant.RelayModeChatCompletions, relaycommon.StreamEndReasonTimeout, false)
	summary := textQuotaSummary{}
	assert.False(t, shouldWaiveZeroCompletionQuota(relayInfo, &summary, nil))
}

func TestSplitZeroCompletionWaiver(t *testing.T) {
	// 无附加费：推理费全免
	keep, waived := splitZeroCompletionWaiver(1000, decimal.Zero)
	assert.Equal(t, 0, keep)
	assert.Equal(t, 1000, waived)

	// 有附加费：附加费保留，只免推理费部分
	keep, waived = splitZeroCompletionWaiver(1000, decimal.NewFromInt(300))
	assert.Equal(t, 300, keep)
	assert.Equal(t, 700, waived)

	// 附加费超过总额（异常防御）：保留不超过总额，免除为 0
	keep, waived = splitZeroCompletionWaiver(100, decimal.NewFromInt(300))
	assert.Equal(t, 100, keep)
	assert.Equal(t, 0, waived)

	// 负附加费（异常防御）：按 0 处理
	keep, waived = splitZeroCompletionWaiver(100, decimal.NewFromInt(-5))
	assert.Equal(t, 0, keep)
	assert.Equal(t, 100, waived)
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
