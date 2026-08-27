package relay

import (
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// Claude thinking → reasoning_effort 桥接（ChannelSettings.BridgeThinkingEffort，默认关）：
// messages→chat 转换会把 thinking 字段整个丢掉（非 OpenRouter 分支），到上游时
// enabled 和 disabled 长得一模一样。对只认 reasoning_effort 的上游，enabled 因此
// 不出思考块；而参数覆盖层跑在转换之后同样看不到 thinking，无条件注入又会让
// disabled 关不掉。这里在转换后、覆盖层之前按原始 thinking 语义桥接：
// enabled/adaptive → reasoning_effort=high（客户已显式给出的值优先），
// disabled 或未开启 → 不下发（与 Anthropic 协议"默认不思考"一致）。
func applyClaudeThinkingBridgeIfNeeded(info *relaycommon.RelayInfo, claudeRequest *dto.ClaudeRequest, convertedRequest any) {
	if info == nil || claudeRequest == nil || !info.ChannelSetting.BridgeThinkingEffort {
		return
	}
	openAIRequest, ok := convertedRequest.(*dto.GeneralOpenAIRequest)
	if !ok {
		return
	}
	thinkingOn := claudeRequest.Thinking != nil &&
		(claudeRequest.Thinking.Type == "enabled" || claudeRequest.Thinking.Type == "adaptive")
	if !thinkingOn {
		openAIRequest.ReasoningEffort = ""
		return
	}
	if openAIRequest.ReasoningEffort == "" {
		openAIRequest.ReasoningEffort = "high"
	}
}
