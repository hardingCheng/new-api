package relay

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func ImageHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {
	info.InitChannelMeta(c)

	imageReq, ok := info.Request.(*dto.ImageRequest)
	if !ok {
		return types.NewErrorWithStatusCode(fmt.Errorf("invalid request type, expected dto.ImageRequest, got %T", info.Request), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}

	request, err := common.DeepCopy(imageReq)
	if err != nil {
		return types.NewError(fmt.Errorf("failed to copy request to ImageRequest: %w", err), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}

	err = helper.ModelMappedHelper(c, info, request)
	if err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}
	removedUnsupportedBackground := removeUnsupportedTransparentBackgroundForImageRequest(info, request) ||
		removeUnsupportedTransparentBackgroundFromMultipartForm(c, info, request)

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)

	var requestBody io.Reader

	if !removedUnsupportedBackground && (model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled) {
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			return types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		requestBody = common.ReaderOnly(storage)
	} else {
		convertedRequest, err := adaptor.ConvertImageRequest(c, info, *request)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed)
		}
		relaycommon.AppendRequestConversionFromRequest(info, convertedRequest)

		switch convertedRequest.(type) {
		case *bytes.Buffer:
			requestBody = convertedRequest.(io.Reader)
		default:
			jsonData, err := common.Marshal(convertedRequest)
			if err != nil {
				return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
			}

			// apply param override
			if len(info.ParamOverride) > 0 {
				jsonData, err = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
				if err != nil {
					return newAPIErrorFromParamOverride(err)
				}
			}

			if common.DebugEnabled {
				logger.LogDebug(c, fmt.Sprintf("image request body: %s", string(jsonData)))
			}
			requestBody = bytes.NewBuffer(jsonData)
		}
	}

	statusCodeMappingStr := c.GetString("status_code_mapping")

	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}
	var httpResp *http.Response
	if resp != nil {
		httpResp = resp.(*http.Response)
		info.IsStream = info.IsStream || strings.HasPrefix(httpResp.Header.Get("Content-Type"), "text/event-stream")
		if httpResp.StatusCode != http.StatusOK {
			if httpResp.StatusCode == http.StatusCreated && info.ApiType == constant.APITypeReplicate {
				// replicate channel returns 201 Created when using Prefer: wait, treat it as success.
				httpResp.StatusCode = http.StatusOK
			} else {
				newAPIError = service.RelayErrorHandler(c.Request.Context(), httpResp, false)
				// reset status code 重置状态码
				service.ResetStatusCode(newAPIError, statusCodeMappingStr)
				return newAPIError
			}
		}
	}

	usage, newAPIError := adaptor.DoResponse(c, httpResp, info)
	if newAPIError != nil {
		// reset status code 重置状态码
		service.ResetStatusCode(newAPIError, statusCodeMappingStr)
		return newAPIError
	}

	imageN := uint(1)
	if request.N != nil {
		imageN = *request.N
	}
	if usage.(*dto.Usage).TotalTokens == 0 {
		usage.(*dto.Usage).TotalTokens = int(imageN)
	}
	if usage.(*dto.Usage).PromptTokens == 0 {
		usage.(*dto.Usage).PromptTokens = int(imageN)
	}

	quality := "standard"
	if request.Quality == "hd" {
		quality = "hd"
	}

	var logContent []string

	if len(request.Size) > 0 {
		logContent = append(logContent, fmt.Sprintf("大小 %s", request.Size))
	}
	if len(quality) > 0 {
		logContent = append(logContent, fmt.Sprintf("品质 %s", quality))
	}
	if imageN > 0 {
		logContent = append(logContent, fmt.Sprintf("生成数量 %d", imageN))
	}

	postConsumeQuota(c, info, usage.(*dto.Usage), logContent...)
	return nil
}

func removeUnsupportedTransparentBackgroundForImageRequest(info *relaycommon.RelayInfo, request *dto.ImageRequest) bool {
	if request == nil || len(request.Background) == 0 {
		return false
	}
	if !isGPTImage2Request(info, request) {
		return false
	}

	var background string
	if err := common.Unmarshal(request.Background, &background); err != nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(background), "transparent") {
		return false
	}
	request.Background = nil
	return true
}

func removeUnsupportedTransparentBackgroundFromMultipartForm(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ImageRequest) bool {
	if c == nil || !strings.Contains(c.GetHeader("Content-Type"), "multipart/form-data") || !isGPTImage2Request(info, request) {
		return false
	}
	if c.Request.MultipartForm == nil {
		if _, err := c.MultipartForm(); err != nil {
			return false
		}
	}
	if c.Request.MultipartForm == nil || len(c.Request.MultipartForm.Value["background"]) == 0 {
		return false
	}
	removed := false
	values := c.Request.MultipartForm.Value["background"]
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), "transparent") {
			removed = true
			break
		}
	}
	if !removed {
		return false
	}
	delete(c.Request.MultipartForm.Value, "background")
	c.Request.PostForm.Del("background")
	c.Request.Form.Del("background")
	return true
}

func isGPTImage2Request(info *relaycommon.RelayInfo, request *dto.ImageRequest) bool {
	if request != nil && isGPTImage2Model(request.Model) {
		return true
	}
	if info == nil {
		return false
	}
	if isGPTImage2Model(info.OriginModelName) {
		return true
	}
	if info.ChannelMeta != nil && isGPTImage2Model(info.UpstreamModelName) {
		return true
	}
	return false
}

func isGPTImage2Model(modelName string) bool {
	modelName = strings.ToLower(strings.TrimSpace(modelName))
	return modelName == "gpt-image-2" || modelName == "gpt-image-2-pro"
}
