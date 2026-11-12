package gemini

import (
	"fmt"
	"io"
	"net/http"
	"strings"

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
)

func GeminiTextGenerationHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	// 读取响应体
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	logger.LogDebug(c, "Gemini native response body: %s", responseBody)

	// 解析为 Gemini 原生响应格式
	var geminiResponse dto.GeminiChatResponse
	err = common.Unmarshal(responseBody, &geminiResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	info.ObserveResponseModel(gjson.GetBytes(responseBody, "modelVersion").Str)
	countGeminiBillableFunctionCalls(info, &geminiResponse)

	if len(geminiResponse.Candidates) == 0 && geminiResponse.PromptFeedback != nil && geminiResponse.PromptFeedback.BlockReason != nil {
		info.PerformanceBusinessRejection = true
		common.SetContextKey(c, constant.ContextKeyAdminRejectReason, fmt.Sprintf("gemini_block_reason=%s", *geminiResponse.PromptFeedback.BlockReason))
	}

	// 计算使用量（优先上游 UsageMetadata，缺失时本地估算并保留 Gemini 计费语义）
	usage := buildUsageFromGeminiResponse(c, info, &geminiResponse)

	service.IOCopyBytesGracefully(c, resp, responseBody)

	return &usage, nil
}

func NativeGeminiEmbeddingHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	logger.LogDebug(c, "Gemini native embedding response body: %s", responseBody)

	usage := service.ResponseText2Usage(c, "", info.UpstreamModelName, info.GetEstimatePromptTokens())

	if info.IsGeminiBatchEmbedding {
		var geminiResponse dto.GeminiBatchEmbeddingResponse
		err = common.Unmarshal(responseBody, &geminiResponse)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
	} else {
		var geminiResponse dto.GeminiEmbeddingResponse
		err = common.Unmarshal(responseBody, &geminiResponse)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
	}

	service.IOCopyBytesGracefully(c, resp, responseBody)

	return usage, nil
}

func GeminiTextGenerationStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	// 官方 :streamGenerateContent 不带 alt=sse 时返回一个 JSON 数组(逐个元素流出),带 alt=sse 才是 SSE。
	// hub 对上游一律用 SSE 拉流,这里按客户端请求的格式回写。
	if strings.Contains(c.Request.URL.Path, ":streamGenerateContent") && c.Query("alt") != "sse" {
		return geminiJSONArrayStreamHandler(c, info, resp)
	}

	helper.SetEventStreamHeaders(c)

	return geminiStreamHandler(c, info, resp, func(data string, geminiResponse *dto.GeminiChatResponse) bool {
		err := helper.StringData(c, data)
		if err != nil {
			logger.LogError(c, "failed to write stream data: "+err.Error())
			return false
		}
		info.SendResponseCount++
		return true
	})
}

func geminiJSONArrayStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	// StreamScannerHandler 开头会调 SetEventStreamHeaders 把 Content-Type 改成 text/event-stream;
	// gin 要到第一次写 body 才真正发头,所以先打上已设置标记让它跳过。
	c.Set("event_stream_headers_set", true)
	c.Writer.WriteHeader(http.StatusOK)

	opened := false
	usage, apiErr := geminiStreamHandler(c, info, resp, func(data string, _ *dto.GeminiChatResponse) bool {
		if c.Request.Context().Err() != nil {
			return false
		}
		sep := ",\n"
		if !opened {
			sep = "["
			opened = true
		}
		if _, err := c.Writer.WriteString(sep + data); err != nil {
			logger.LogError(c, "failed to write stream data: "+err.Error())
			return false
		}
		c.Writer.Flush()
		info.SendResponseCount++
		return true
	})

	closing := "]"
	if !opened {
		closing = "[]"
	}
	if c.Request.Context().Err() == nil {
		_, _ = c.Writer.WriteString(closing)
		c.Writer.Flush()
	}
	return usage, apiErr
}
