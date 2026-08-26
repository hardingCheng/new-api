package openai

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
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
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}

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
	usage := dto.Usage{}
	if responsesResponse.Usage != nil {
		usage.PromptTokens = responsesResponse.Usage.InputTokens
		usage.CompletionTokens = responsesResponse.Usage.OutputTokens
		usage.TotalTokens = responsesResponse.Usage.TotalTokens
		if responsesResponse.Usage.InputTokensDetails != nil {
			usage.PromptTokensDetails.CachedTokens = responsesResponse.Usage.InputTokensDetails.CachedTokens
			usage.PromptTokensDetails.CacheWriteTokens = responsesResponse.Usage.InputTokensDetails.CacheWriteTokens
		}
	}
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

	return &usage, nil
}

func OaiResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		logger.LogError(c, "invalid response or response body")
		return nil, types.NewError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse)
	}

	defer service.CloseResponseBodyGracefully(resp)

	var usage = &dto.Usage{}
	var responseTextBuilder strings.Builder
	imageCounter := &relaycommon.ImageGenerationCallCounter{}
	imageCommitted := false

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
		sendResponsesStreamData(c, streamResponse, data)
		switch streamResponse.Type {
		case "response.completed", "response.done":
			if streamResponse.Response != nil {
				if streamResponse.Response.Usage != nil {
					if streamResponse.Response.Usage.InputTokens != 0 {
						usage.PromptTokens = streamResponse.Response.Usage.InputTokens
					}
					if streamResponse.Response.Usage.OutputTokens != 0 {
						usage.CompletionTokens = streamResponse.Response.Usage.OutputTokens
					}
					if streamResponse.Response.Usage.TotalTokens != 0 {
						usage.TotalTokens = streamResponse.Response.Usage.TotalTokens
					}
					if streamResponse.Response.Usage.InputTokensDetails != nil {
						usage.PromptTokensDetails.CachedTokens = streamResponse.Response.Usage.InputTokensDetails.CachedTokens
						usage.PromptTokensDetails.CacheWriteTokens = streamResponse.Response.Usage.InputTokensDetails.CacheWriteTokens
					}
				}
				if !imageCommitted {
					if relaycommon.IsNonBillableResponsesStatus(streamResponse.Response.Status) {
						imageCounter.Reset()
						imageCounter.Commit(info)
						imageCommitted = true
					} else {
						for i := range streamResponse.Response.Output {
							idx := i
							imageCounter.Observe(&streamResponse.Response.Output[i], &idx)
						}
						imageCounter.Commit(info)
						imageCommitted = true
					}
				}
			} else if !imageCommitted {
				imageCounter.Commit(info)
				imageCommitted = true
			}
		case "response.failed", "response.incomplete", "response.cancelled", "response.canceled":
			if !imageCommitted {
				imageCounter.Reset()
				imageCounter.Commit(info)
				imageCommitted = true
			}
		case "response.output_text.delta":
			// 处理输出文本
			responseTextBuilder.WriteString(streamResponse.Delta)
		case dto.ResponsesOutputTypeItemDone:
			if streamResponse.Item != nil {
				switch streamResponse.Item.Type {
				case dto.BuildInCallWebSearchCall:
					info.CountBillableToolCall(dto.BuildInCallWebSearchCall, "")
				case dto.BuildInCallFileSearchCall:
					info.CountBillableToolCall(dto.BuildInCallFileSearchCall, "")
				case dto.BuildInCallFunctionCall:
					info.CountBillableToolCall(dto.BuildInCallFunctionCall, streamResponse.Item.Name)
				case dto.ResponsesOutputTypeImageGenerationCall:
					if !imageCommitted {
						imageCounter.Observe(streamResponse.Item, streamResponse.OutputIndex)
					}
				}
			}
		}
	})

	if usage.CompletionTokens == 0 {
		// 计算输出文本的 token 数量
		tempStr := responseTextBuilder.String()
		if len(tempStr) > 0 {
			// 非正常结束，使用输出文本的 token 数量
			completionTokens := service.CountTextToken(tempStr, info.UpstreamModelName)
			usage.CompletionTokens = completionTokens
		}
	}

	if usage.PromptTokens == 0 && usage.CompletionTokens != 0 {
		usage.PromptTokens = info.GetEstimatePromptTokens()
	}

	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens

	return usage, nil
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
