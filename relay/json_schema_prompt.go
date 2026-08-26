package relay

import (
	"encoding/json"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// 结构化输出兜底：部分上游收下 response_format.json_schema 却不把 schema 交给模型，
// 结果模型只知道"要输出 JSON"、不知道要哪些字段——实测该上游原样请求 0/6 合规
// （返回 markdown 正文），把 schema 写进 system 后 6/6 合规。
//
// 这里把 schema 原文补一条 system 消息交给模型，response_format 本身照常透传：
// 上游若本就支持 strict，多这条消息不改变结果；上游若不支持，这条就是唯一的约束来源。
// 默认关闭，按渠道开启（ChannelSettings.JsonSchemaPrompt）。
const jsonSchemaPromptPrefix = "You must respond with a single JSON object that validates against this JSON Schema:\n"
const jsonSchemaPromptSuffix = "\nOutput only the JSON object: no markdown fences, no explanation, no fields outside the schema."

func applyJsonSchemaPromptIfNeeded(info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) {
	if info == nil || info.ChannelMeta == nil || request == nil || !info.ChannelSetting.JsonSchemaPrompt {
		return
	}
	if request.ResponseFormat == nil || request.ResponseFormat.Type != "json_schema" {
		return
	}
	schema := extractJsonSchemaText(request.ResponseFormat.JsonSchema)
	if schema == "" {
		return
	}
	instruction := jsonSchemaPromptPrefix + schema + jsonSchemaPromptSuffix
	systemRole := request.GetSystemRoleName()
	for i, message := range request.Messages {
		if message.Role != systemRole {
			continue
		}
		// 已有 system 消息时拼在后面，保留客户自己的指令
		if message.IsStringContent() {
			request.Messages[i].SetStringContent(message.StringContent() + "\n\n" + instruction)
		} else {
			contents := message.ParseContent()
			contents = append(contents, dto.MediaContent{Type: dto.ContentTypeText, Text: instruction})
			request.Messages[i].Content = contents
		}
		return
	}
	request.Messages = append([]dto.Message{{Role: systemRole, Content: instruction}}, request.Messages...)
}

// applyResponsesJsonSchemaPromptIfNeeded 是 responses 协议侧的同一兜底：
// text.format.type=json_schema 时把 schema 原文并入 instructions（responses 协议的
// system 级指令位），text.format 本身照常透传。与 chat 侧共用同一渠道开关。
func applyResponsesJsonSchemaPromptIfNeeded(info *relaycommon.RelayInfo, request *dto.OpenAIResponsesRequest) {
	if info == nil || info.ChannelMeta == nil || request == nil || !info.ChannelSetting.JsonSchemaPrompt {
		return
	}
	schema := extractResponsesJsonSchemaText(request.Text)
	if schema == "" {
		return
	}
	instruction := jsonSchemaPromptPrefix + schema + jsonSchemaPromptSuffix
	existingRaw := strings.TrimSpace(string(request.Instructions))
	if existingRaw == "" || existingRaw == "null" {
		encoded, err := common.Marshal(instruction)
		if err != nil {
			return
		}
		request.Instructions = encoded
		return
	}
	// instructions 官方为字符串；已有时拼在后面，保留客户自己的指令。
	// 非字符串形状（个别客户端的扩展写法）不动，避免写坏请求。
	var existing string
	if err := common.Unmarshal(request.Instructions, &existing); err != nil {
		return
	}
	encoded, err := common.Marshal(existing + "\n\n" + instruction)
	if err != nil {
		return
	}
	request.Instructions = encoded
}

// extractResponsesJsonSchemaText 从 responses 请求的 text.format 中取出 schema 原文，
// 仅 format.type=json_schema 时生效。
func extractResponsesJsonSchemaText(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var wrapper struct {
		Format json.RawMessage `json:"format"`
	}
	if err := common.Unmarshal(raw, &wrapper); err != nil || len(wrapper.Format) == 0 {
		return ""
	}
	var format struct {
		Type string `json:"type"`
	}
	if err := common.Unmarshal(wrapper.Format, &format); err != nil || format.Type != "json_schema" {
		return ""
	}
	return extractJsonSchemaText(wrapper.Format)
}

// extractJsonSchemaText 取出 json_schema.schema 的原文。json_schema 是
// RawMessage，客户可能直接给 schema、也可能按规范包一层 {name, strict, schema}。
func extractJsonSchemaText(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var wrapper struct {
		Schema json.RawMessage `json:"schema"`
	}
	if err := common.Unmarshal(raw, &wrapper); err == nil && len(wrapper.Schema) > 0 {
		if text := strings.TrimSpace(string(wrapper.Schema)); text != "" && text != "null" {
			return text
		}
	}
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" {
		return ""
	}
	return text
}
