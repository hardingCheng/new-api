package openai

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// 结构化输出形状归位（ChannelSettings.StructuredToolCallToContent，默认关）：
// 部分上游用"强制工具调用"实现 response_format——对 json_schema/json_object 请求
// 返回 finish_reason=tool_calls，JSON 在 tool_calls[0].function.arguments 里，
// content 为空。数据是对的，但客户端按官方形状读 content 拿到空串。
// 这里在响应侧搬回官方形状：arguments → content，剥掉合成的 tool_calls，
// finish_reason 归为 stop。仅当客户请求带 json 系 response_format 且没带任何
// tools 时才动——带 tools 的请求里 tool_calls 是真实调用，原样透传。

// shouldFixStructuredToolCall 判断本次请求是否属于"结构化请求被上游转成工具调用"的场景。
func shouldFixStructuredToolCall(info *relaycommon.RelayInfo) bool {
	if info == nil || !info.ChannelSetting.StructuredToolCallToContent {
		return false
	}
	request, ok := info.Request.(*dto.GeneralOpenAIRequest)
	if !ok || request == nil || len(request.Tools) > 0 {
		return false
	}
	if request.ResponseFormat == nil {
		return false
	}
	return request.ResponseFormat.Type == "json_schema" || request.ResponseFormat.Type == "json_object"
}

// fixStructuredToolCallChoices 把合成工具调用搬回 content。返回被改动的 choice 下标。
func fixStructuredToolCallChoices(choices []dto.OpenAITextResponseChoice) []int {
	var fixed []int
	for i := range choices {
		choice := &choices[i]
		if choice.FinishReason != "tool_calls" {
			continue
		}
		toolCalls := choice.Message.ParseToolCalls()
		if len(toolCalls) != 1 {
			continue
		}
		arguments := strings.TrimSpace(toolCalls[0].Function.Arguments)
		var parsed any
		if arguments == "" || common.UnmarshalJsonStr(arguments, &parsed) != nil {
			continue
		}
		choice.Message.Content = arguments
		choice.Message.ToolCalls = nil
		choice.FinishReason = "stop"
		fixed = append(fixed, i)
	}
	return fixed
}

// applyStructuredToolCallFixToBody 把同样的改动落到原始响应 JSON 上（只动被修过的
// 下标），响应体里 DTO 未建模的字段原样不动。
func applyStructuredToolCallFixToBody(bodyMap map[string]any, choices []dto.OpenAITextResponseChoice, fixedIndexes []int) {
	rawChoices, ok := bodyMap["choices"].([]any)
	if !ok {
		return
	}
	for _, i := range fixedIndexes {
		if i >= len(rawChoices) || i >= len(choices) {
			continue
		}
		rawChoice, ok := rawChoices[i].(map[string]any)
		if !ok {
			continue
		}
		rawMessage, ok := rawChoice["message"].(map[string]any)
		if !ok {
			continue
		}
		rawMessage["content"] = choices[i].Message.StringContent()
		delete(rawMessage, "tool_calls")
		rawChoice["finish_reason"] = "stop"
	}
}
