package openai

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func structuredInfo(on bool, format string, withTools bool) *relaycommon.RelayInfo {
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	info.ChannelSetting.StructuredToolCallToContent = on
	request := &dto.GeneralOpenAIRequest{Model: "kimi-k3"}
	if format != "" {
		request.ResponseFormat = &dto.ResponseFormat{Type: format}
	}
	if withTools {
		request.Tools = []dto.ToolCallRequest{{Type: "function", Function: dto.FunctionRequest{Name: "f"}}}
	}
	info.Request = request
	return info
}

func syntheticToolCallChoice() dto.OpenAITextResponseChoice {
	return dto.OpenAITextResponseChoice{
		FinishReason: "tool_calls",
		Message: dto.Message{
			Role: "assistant",
			ToolCalls: json.RawMessage(`[{"id":"structured_output_weather_0","type":"function",` +
				`"function":{"name":"structured_output_weather","arguments":"{\"city\":\"Shanghai\"}"}}]`),
		},
	}
}

func TestShouldFixStructuredToolCall(t *testing.T) {
	assert.True(t, shouldFixStructuredToolCall(structuredInfo(true, "json_schema", false)))
	assert.True(t, shouldFixStructuredToolCall(structuredInfo(true, "json_object", false)))
	assert.False(t, shouldFixStructuredToolCall(structuredInfo(false, "json_schema", false)), "开关关着不动")
	assert.False(t, shouldFixStructuredToolCall(structuredInfo(true, "text", false)), "非 json 系格式不动")
	assert.False(t, shouldFixStructuredToolCall(structuredInfo(true, "", false)), "无 response_format 不动")
	assert.False(t, shouldFixStructuredToolCall(structuredInfo(true, "json_schema", true)), "客户带了真 tools 不动")
}

func TestFixStructuredToolCallChoices(t *testing.T) {
	t.Run("合成调用搬回 content", func(t *testing.T) {
		choices := []dto.OpenAITextResponseChoice{syntheticToolCallChoice()}
		fixed := fixStructuredToolCallChoices(choices)
		require.Equal(t, []int{0}, fixed)
		assert.Equal(t, `{"city":"Shanghai"}`, choices[0].Message.StringContent())
		assert.Nil(t, choices[0].Message.ToolCalls)
		assert.Equal(t, "stop", choices[0].FinishReason)
	})
	t.Run("多个工具调用不动", func(t *testing.T) {
		choice := syntheticToolCallChoice()
		choice.Message.ToolCalls = json.RawMessage(`[{"type":"function","function":{"name":"a","arguments":"{}"}},` +
			`{"type":"function","function":{"name":"b","arguments":"{}"}}]`)
		assert.Empty(t, fixStructuredToolCallChoices([]dto.OpenAITextResponseChoice{choice}))
	})
	t.Run("arguments 非法 JSON 不动", func(t *testing.T) {
		choice := syntheticToolCallChoice()
		choice.Message.ToolCalls = json.RawMessage(`[{"type":"function","function":{"name":"s","arguments":"not-json"}}]`)
		assert.Empty(t, fixStructuredToolCallChoices([]dto.OpenAITextResponseChoice{choice}))
	})
	t.Run("正常 stop 响应不动", func(t *testing.T) {
		choice := dto.OpenAITextResponseChoice{FinishReason: "stop", Message: dto.Message{Content: "hi"}}
		assert.Empty(t, fixStructuredToolCallChoices([]dto.OpenAITextResponseChoice{choice}))
	})
}

func TestApplyStructuredToolCallFixToBody(t *testing.T) {
	body := []byte(`{"id":"x","choices":[{"index":0,"finish_reason":"tool_calls","message":{"role":"assistant",` +
		`"content":"","reasoning_content":"thought","tool_calls":[{"id":"structured_output_w_0","type":"function",` +
		`"function":{"name":"structured_output_w","arguments":"{\"city\":\"Shanghai\"}"}}]}}],"usage":{"total_tokens":9}}`)
	var bodyMap map[string]any
	require.NoError(t, common.Unmarshal(body, &bodyMap))
	choices := []dto.OpenAITextResponseChoice{syntheticToolCallChoice()}
	fixed := fixStructuredToolCallChoices(choices)
	applyStructuredToolCallFixToBody(bodyMap, choices, fixed)
	out, err := common.Marshal(bodyMap)
	require.NoError(t, err)
	text := string(out)
	assert.Contains(t, text, `"content":"{\"city\":\"Shanghai\"}"`)
	assert.NotContains(t, text, "tool_calls")
	assert.Contains(t, text, `"finish_reason":"stop"`)
	assert.Contains(t, text, `"reasoning_content":"thought"`, "未建模字段应原样保留")
	assert.Contains(t, text, `"total_tokens":9`)
}
