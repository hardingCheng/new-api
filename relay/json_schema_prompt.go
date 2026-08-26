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
