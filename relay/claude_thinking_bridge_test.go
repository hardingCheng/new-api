package relay

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
)

func bridgeInfo(on bool) *relaycommon.RelayInfo {
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	info.ChannelSetting.BridgeThinkingEffort = on
	return info
}

func claudeReqWithThinking(thinkingType string) *dto.ClaudeRequest {
	req := &dto.ClaudeRequest{Model: "kimi-k3"}
	if thinkingType != "" {
		req.Thinking = &dto.Thinking{Type: thinkingType}
	}
	return req
}

func TestClaudeThinkingBridge(t *testing.T) {
	t.Run("关闭时不动", func(t *testing.T) {
		converted := &dto.GeneralOpenAIRequest{ReasoningEffort: "low"}
		applyClaudeThinkingBridgeIfNeeded(bridgeInfo(false), claudeReqWithThinking("enabled"), converted)
		assert.Equal(t, "low", converted.ReasoningEffort)
	})
	t.Run("enabled 注入 high", func(t *testing.T) {
		converted := &dto.GeneralOpenAIRequest{}
		applyClaudeThinkingBridgeIfNeeded(bridgeInfo(true), claudeReqWithThinking("enabled"), converted)
		assert.Equal(t, "high", converted.ReasoningEffort)
	})
	t.Run("adaptive 注入 high", func(t *testing.T) {
		converted := &dto.GeneralOpenAIRequest{}
		applyClaudeThinkingBridgeIfNeeded(bridgeInfo(true), claudeReqWithThinking("adaptive"), converted)
		assert.Equal(t, "high", converted.ReasoningEffort)
	})
	t.Run("disabled 清空不下发", func(t *testing.T) {
		converted := &dto.GeneralOpenAIRequest{ReasoningEffort: "high"}
		applyClaudeThinkingBridgeIfNeeded(bridgeInfo(true), claudeReqWithThinking("disabled"), converted)
		assert.Empty(t, converted.ReasoningEffort)
	})
	t.Run("未开启 thinking 不下发", func(t *testing.T) {
		converted := &dto.GeneralOpenAIRequest{}
		applyClaudeThinkingBridgeIfNeeded(bridgeInfo(true), claudeReqWithThinking(""), converted)
		assert.Empty(t, converted.ReasoningEffort)
	})
	t.Run("客户已显式给值时保留", func(t *testing.T) {
		converted := &dto.GeneralOpenAIRequest{ReasoningEffort: "low"}
		applyClaudeThinkingBridgeIfNeeded(bridgeInfo(true), claudeReqWithThinking("enabled"), converted)
		assert.Equal(t, "low", converted.ReasoningEffort)
	})
	t.Run("非 chat 转换结果不动", func(t *testing.T) {
		applyClaudeThinkingBridgeIfNeeded(bridgeInfo(true), claudeReqWithThinking("enabled"), &dto.ClaudeRequest{})
	})
}
