package relay

import (
	"encoding/json"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func infoWith(jsonSchemaPrompt, validateTools bool) *relaycommon.RelayInfo {
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	info.ChannelSetting.JsonSchemaPrompt = jsonSchemaPrompt
	info.ChannelSetting.ValidateToolSchema = validateTools
	return info
}

func chatReqWithSchema(schema string) *dto.GeneralOpenAIRequest {
	return &dto.GeneralOpenAIRequest{
		Model:          "kimi-k3",
		Messages:       []dto.Message{{Role: "user", Content: "北京今天25度"}},
		ResponseFormat: &dto.ResponseFormat{Type: "json_schema", JsonSchema: json.RawMessage(schema)},
	}
}

func TestJsonSchemaPromptInjection(t *testing.T) {
	wrapped := `{"name":"w","strict":true,"schema":{"type":"object","properties":{"city":{"type":"string"}}}}`

	t.Run("关闭时不注入", func(t *testing.T) {
		req := chatReqWithSchema(wrapped)
		applyJsonSchemaPromptIfNeeded(infoWith(false, false), req)
		if len(req.Messages) != 1 {
			t.Fatalf("不该改动消息，实际 %d 条", len(req.Messages))
		}
	})

	t.Run("开启时注入 system 且含 schema 原文", func(t *testing.T) {
		req := chatReqWithSchema(wrapped)
		applyJsonSchemaPromptIfNeeded(infoWith(true, false), req)
		if len(req.Messages) != 2 || req.Messages[0].Role != "system" {
			t.Fatalf("应在最前面插入 system 消息，实际 %+v", req.Messages)
		}
		got := req.Messages[0].StringContent()
		if !strings.Contains(got, `"city"`) || !strings.Contains(got, "JSON Schema") {
			t.Fatalf("system 内容应含 schema 原文，实际 %q", got)
		}
		if strings.Contains(got, `"strict"`) {
			t.Fatalf("应只取 schema 子对象，不该带 strict 包装：%q", got)
		}
	})

	t.Run("已有 system 时拼接而非覆盖", func(t *testing.T) {
		req := chatReqWithSchema(wrapped)
		req.Messages = append([]dto.Message{{Role: "system", Content: "你是助手"}}, req.Messages...)
		applyJsonSchemaPromptIfNeeded(infoWith(true, false), req)
		got := req.Messages[0].StringContent()
		if !strings.HasPrefix(got, "你是助手") || !strings.Contains(got, "JSON Schema") {
			t.Fatalf("应保留客户 system 并追加，实际 %q", got)
		}
	})

	t.Run("非 json_schema 不注入", func(t *testing.T) {
		req := chatReqWithSchema(wrapped)
		req.ResponseFormat.Type = "json_object"
		applyJsonSchemaPromptIfNeeded(infoWith(true, false), req)
		if len(req.Messages) != 1 {
			t.Fatalf("json_object 不该注入")
		}
	})
}

func responsesReqWithFormat(format string) *dto.OpenAIResponsesRequest {
	return &dto.OpenAIResponsesRequest{
		Model: "kimi-k3",
		Input: json.RawMessage(`"北京今天25度"`),
		Text:  json.RawMessage(`{"format":` + format + `}`),
	}
}

func TestResponsesJsonSchemaPromptInjection(t *testing.T) {
	format := `{"type":"json_schema","name":"w","strict":true,"schema":{"type":"object","properties":{"city":{"type":"string"}}}}`

	t.Run("关闭时不注入", func(t *testing.T) {
		req := responsesReqWithFormat(format)
		applyResponsesJsonSchemaPromptIfNeeded(infoWith(false, false), req)
		assert.Empty(t, req.Instructions)
	})

	t.Run("开启且无 instructions 时写入", func(t *testing.T) {
		req := responsesReqWithFormat(format)
		applyResponsesJsonSchemaPromptIfNeeded(infoWith(true, false), req)
		var got string
		require.NoError(t, json.Unmarshal(req.Instructions, &got))
		assert.Contains(t, got, `"city"`)
		assert.Contains(t, got, "JSON Schema")
		assert.NotContains(t, got, `"strict"`, "应只取 schema 子对象，不该带 strict 包装")
	})

	t.Run("已有 instructions 时拼接保留", func(t *testing.T) {
		req := responsesReqWithFormat(format)
		req.Instructions = json.RawMessage(`"你是助手"`)
		applyResponsesJsonSchemaPromptIfNeeded(infoWith(true, false), req)
		var got string
		require.NoError(t, json.Unmarshal(req.Instructions, &got))
		assert.True(t, strings.HasPrefix(got, "你是助手"), "应保留客户指令，实际 %q", got)
		assert.Contains(t, got, "JSON Schema")
	})

	t.Run("format 非 json_schema 不注入", func(t *testing.T) {
		req := responsesReqWithFormat(`{"type":"text"}`)
		applyResponsesJsonSchemaPromptIfNeeded(infoWith(true, false), req)
		assert.Empty(t, req.Instructions)
	})

	t.Run("instructions 非字符串形状不动", func(t *testing.T) {
		req := responsesReqWithFormat(format)
		req.Instructions = json.RawMessage(`[{"type":"text","text":"x"}]`)
		applyResponsesJsonSchemaPromptIfNeeded(infoWith(true, false), req)
		assert.Equal(t, `[{"type":"text","text":"x"}]`, string(req.Instructions), "非字符串 instructions 不该被改写")
	})
}

func toolReq(params any) *dto.GeneralOpenAIRequest {
	return &dto.GeneralOpenAIRequest{
		Model:    "kimi-k3",
		Messages: []dto.Message{{Role: "user", Content: "hi"}},
		Tools: []dto.ToolCallRequest{{
			Type:     "function",
			Function: dto.FunctionRequest{Name: "f", Parameters: params},
		}},
	}
}

func TestValidateToolSchemas(t *testing.T) {
	legal := map[string]any{
		"type":       "object",
		"properties": map[string]any{"answer": map[string]any{"type": "integer"}},
		"required":   []any{"answer"},
	}
	badType := map[string]any{"type": "nonexistent-type", "properties": map[string]any{}}
	badProps := map[string]any{"type": "object", "properties": "not-an-object"}
	badNested := map[string]any{"type": "object",
		"properties": map[string]any{"x": map[string]any{"type": "strin"}}}
	badRequired := map[string]any{"type": "object", "required": []any{1}}

	t.Run("关闭时一律放行", func(t *testing.T) {
		if err := validateToolSchemasIfNeeded(infoWith(false, false), toolReq(badType)); err != nil {
			t.Fatalf("关闭时不该拦，实际 %v", err)
		}
	})
	t.Run("合法 schema 放行", func(t *testing.T) {
		if err := validateToolSchemasIfNeeded(infoWith(false, true), toolReq(legal)); err != nil {
			t.Fatalf("合法 schema 被拦：%v", err)
		}
	})
	for name, params := range map[string]any{
		"非法 type":        badType,
		"properties 非对象": badProps,
		"嵌套非法 type":      badNested,
		"required 非字符串":  badRequired,
	} {
		t.Run(name+"应拒", func(t *testing.T) {
			err := validateToolSchemasIfNeeded(infoWith(false, true), toolReq(params))
			if err == nil {
				t.Fatalf("应拒绝但放行了")
			}
			if !strings.Contains(err.Error(), "Invalid schema for function 'f'") {
				t.Fatalf("错误文案应定位到函数名，实际 %v", err)
			}
			t.Logf("  %s → %v", name, err)
		})
	}
}
