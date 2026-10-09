package openai

import (
	"fmt"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func OaiResponsesHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	// read response body
	var responsesResponse dto.OpenAIResponsesResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	err = common.Unmarshal(responseBody, &responsesResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if oaiError := responsesResponse.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, upstreamErrorStatusCode(resp.StatusCode))
	}

	info.ObserveResponseModel(responsesResponse.Model)
	responseBody = rewriteSGLangResponsesCreatedAt(info, responseBody, "created_at", responsesResponse.CreatedAt)
	// model 回填客户请求名:多上游池的自报名会随路由跳变。仅在不一致时用 map 重写,
	// 保留上游 body 的未知字段,也不给一致的直通增加开销
	if info != nil && info.OriginModelName != "" && responsesResponse.Model != "" && responsesResponse.Model != info.OriginModelName {
		var bodyMap map[string]interface{}
		if mapErr := common.Unmarshal(responseBody, &bodyMap); mapErr == nil {
			bodyMap["model"] = info.OriginModelName
			if newBody, marshalErr := common.Marshal(bodyMap); marshalErr == nil {
				responseBody = newBody
			}
		}
	}

	// 写入新的 response body
	service.IOCopyBytesGracefully(c, resp, responseBody)

	// compute usage
	usage := &dto.Usage{}
	service.ApplyResponsesUsage(usage, responsesResponse.Usage)
	// Count actual tool invocations from Output (not tool declarations).
	for _, output := range responsesResponse.Output {
		switch output.Type {
		case dto.BuildInCallWebSearchCall:
			info.CountBillableToolCall(dto.BuildInCallWebSearchCall, "")
		case dto.BuildInCallFileSearchCall:
			info.CountBillableToolCall(dto.BuildInCallFileSearchCall, "")
		case dto.BuildInCallFunctionCall:
			info.CountBillableToolCall(dto.BuildInCallFunctionCall, output.Name)
		}
	}

	imageCounter := &relaycommon.ImageGenerationCallCounter{}
	if !relaycommon.IsNonBillableResponsesStatus(responsesResponse.Status) {
		for i := range responsesResponse.Output {
			idx := i
			imageCounter.Observe(&responsesResponse.Output[i], &idx)
		}
	}
	imageCounter.Commit(info)

	return usage, nil
}

func OaiResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		logger.LogError(c, "invalid response or response body")
		return nil, types.NewError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse)
	}

	defer service.CloseResponseBodyGracefully(resp)

	accumulator := service.NewResponsesUsageAccumulator(info)

	seenCreated := false
	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {

		// 检查当前数据是否包含 completed 状态和 usage 信息
		var streamResponse dto.ResponsesStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResponse); err != nil {
			logger.LogError(c, "failed to unmarshal stream response: "+err.Error())
			sr.Error(err)
			return
		}
		// 首事件规范化：部分上游把 response.created 发成裸的 Response 对象且不带 type，
		// 事件名因此写成空串（helper.ResponseChunkData 用 type 当事件名），官方 SDK
		// 按 type 分发时直接抛错。这里补成规范形状，其余事件原样透传。
		if streamResponse.Type == "" && !seenCreated {
			if normalized, ok := normalizeResponsesCreatedChunk(data); ok {
				data = normalized
				streamResponse = dto.ResponsesStreamResponse{}
				if err := common.UnmarshalJsonStr(data, &streamResponse); err != nil {
					logger.LogError(c, "failed to unmarshal normalized stream response: "+err.Error())
					sr.Error(err)
					return
				}
			}
		}
		if streamResponse.Type == "response.created" {
			seenCreated = true
		}
		// 官方 SDK 把缺失字段反序列化成 None、随后拼接即崩，这里按官方 schema
		// 回填应为空值的字段（只在缺失时改写，其余事件零开销透传）
		if fixed, ok := backfillResponsesStreamEvent(streamResponse.Type, data); ok {
			data = fixed
		}
		if streamResponse.Response != nil {
			data = string(rewriteSGLangResponsesCreatedAt(info, []byte(data), "response.created_at", streamResponse.Response.CreatedAt))
		}
		sendResponsesStreamData(c, streamResponse, data)
		accumulator.Observe(&streamResponse)
	})

	common.SetContextKey(c, constant.ContextKeyResponseStreamStatus, info.StreamStatus)
	info.StreamStatus.RequireTerminal()
	return accumulator.Finish(), nil
}

func rewriteSGLangResponsesCreatedAt(info *relaycommon.RelayInfo, payload []byte, path string, createdAt dto.IntValue) []byte {
	if info.GetChannelType() != constant.ChannelTypeSGLang {
		return payload
	}
	if !gjson.GetBytes(payload, path).Exists() {
		return payload
	}
	patched, err := sjson.SetBytes(payload, path, int(createdAt))
	if err != nil {
		return payload
	}
	return patched
}

// backfillResponsesStreamEvent 给官方 SDK 反序列化必需、但部分上游省略的空值字段补位：
// response.created / response.in_progress 的 response 缺 output 补 []，
// response.content_part.added 的 output_text part 缺 text 补 ""。
// 字段齐全时不改写不重编码；其他事件类型直接跳过。
func backfillResponsesStreamEvent(eventType string, data string) (string, bool) {
	switch eventType {
	case "response.created", "response.in_progress":
		var payload map[string]any
		if err := common.UnmarshalJsonStr(data, &payload); err != nil {
			return "", false
		}
		response, ok := payload["response"].(map[string]any)
		if !ok {
			return "", false
		}
		if _, exists := response["output"]; exists {
			return "", false
		}
		response["output"] = []any{}
		fixed, err := common.Marshal(payload)
		if err != nil {
			return "", false
		}
		return string(fixed), true
	case "response.content_part.added":
		var payload map[string]any
		if err := common.UnmarshalJsonStr(data, &payload); err != nil {
			return "", false
		}
		part, ok := payload["part"].(map[string]any)
		if !ok {
			return "", false
		}
		partType, _ := part["type"].(string)
		if partType != "output_text" {
			return "", false
		}
		if _, exists := part["text"]; exists {
			return "", false
		}
		part["text"] = ""
		fixed, err := common.Marshal(payload)
		if err != nil {
			return "", false
		}
		return string(fixed), true
	default:
		return "", false
	}
}

// normalizeResponsesCreatedChunk 把"裸 Response 对象、无 type"的首帧改写成官方形状
// {"type":"response.created","response":{...}}。只在对象自称 object=response 时改写，
// 其它无法识别的分片保持原样，避免误伤未知事件。
func normalizeResponsesCreatedChunk(data string) (string, bool) {
	var payload map[string]any
	if err := common.UnmarshalJsonStr(data, &payload); err != nil {
		return "", false
	}
	if _, exists := payload["type"]; exists {
		return "", false
	}
	if obj, _ := payload["object"].(string); obj != "response" {
		return "", false
	}
	wrapped, err := common.Marshal(map[string]any{
		"type":     "response.created",
		"response": payload,
	})
	if err != nil {
		return "", false
	}
	return string(wrapped), true
}
