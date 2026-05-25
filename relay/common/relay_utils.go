package common

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

type HasPrompt interface {
	GetPrompt() string
}

type HasImage interface {
	HasImage() bool
}

func GetFullRequestURL(baseURL string, requestURL string, channelType int) string {
	fullRequestURL := fmt.Sprintf("%s%s", baseURL, requestURL)

	if strings.HasPrefix(baseURL, "https://gateway.ai.cloudflare.com") {
		switch channelType {
		case constant.ChannelTypeOpenAI:
			fullRequestURL = fmt.Sprintf("%s%s", baseURL, strings.TrimPrefix(requestURL, "/v1"))
		case constant.ChannelTypeAzure:
			fullRequestURL = fmt.Sprintf("%s%s", baseURL, strings.TrimPrefix(requestURL, "/openai/deployments"))
		}
	}
	return fullRequestURL
}

func GetAPIVersion(c *gin.Context) string {
	query := c.Request.URL.Query()
	apiVersion := query.Get("api-version")
	if apiVersion == "" {
		apiVersion = c.GetString("api_version")
	}
	return apiVersion
}

func createTaskError(err error, code string, statusCode int, localError bool) *dto.TaskError {
	return &dto.TaskError{
		Code:       code,
		Message:    err.Error(),
		StatusCode: statusCode,
		LocalError: localError,
		Error:      err,
	}
}

func storeTaskRequest(c *gin.Context, info *RelayInfo, action string, requestObj TaskSubmitReq) {
	info.Action = action
	c.Set("task_request", requestObj)
}
func GetTaskRequest(c *gin.Context) (TaskSubmitReq, error) {
	v, exists := c.Get("task_request")
	if !exists {
		return TaskSubmitReq{}, fmt.Errorf("request not found in context")
	}
	req, ok := v.(TaskSubmitReq)
	if !ok {
		return TaskSubmitReq{}, fmt.Errorf("invalid task request type")
	}
	return req, nil
}

func validatePrompt(prompt string) *dto.TaskError {
	if strings.TrimSpace(prompt) == "" {
		return createTaskError(fmt.Errorf("prompt is required"), "invalid_request", http.StatusBadRequest, true)
	}
	return nil
}

func IsSeedanceVideoModel(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	return strings.HasPrefix(model, "seedance-") || strings.HasPrefix(model, "doubao-seedance-")
}

func clampSeedanceDuration(seconds int) int {
	if seconds < 4 {
		return 4
	}
	if seconds > 15 {
		return 15
	}
	return seconds
}

func EffectiveTaskDuration(req TaskSubmitReq) int {
	seconds, _ := strconv.Atoi(req.Seconds)
	if req.Duration > seconds {
		return req.Duration
	}
	return seconds
}

func normalizeTaskDuration(req *TaskSubmitReq) {
	if req == nil {
		return
	}
	seconds := EffectiveTaskDuration(*req)
	if seconds <= 0 {
		return
	}
	req.Duration = seconds
	req.Seconds = strconv.Itoa(seconds)
}

func applySeedanceDurationBounds(req *TaskSubmitReq) {
	if req == nil || !IsSeedanceVideoModel(req.Model) {
		return
	}
	seconds := EffectiveTaskDuration(*req)
	seconds = clampSeedanceDuration(seconds)
	req.Duration = seconds
	req.Seconds = strconv.Itoa(seconds)
}

func ExtractReferenceVideoURLs(req TaskSubmitReq) []string {
	urls := make([]string, 0)
	seen := make(map[string]bool)
	appendURL := func(raw string) {
		url := strings.TrimSpace(raw)
		if url == "" || seen[url] {
			return
		}
		seen[url] = true
		urls = append(urls, url)
	}
	for _, item := range req.Content {
		if item.VideoURL != nil && strings.TrimSpace(item.VideoURL.URL) != "" {
			appendURL(item.VideoURL.URL)
		}
		if item.ImageURL != nil && isReferenceVideoCandidate(item.Type, item.Role, item.ImageURL.URL) {
			appendURL(item.ImageURL.URL)
		}
		if item.AudioURL != nil && isReferenceVideoCandidate(item.Type, item.Role, item.AudioURL.URL) {
			appendURL(item.AudioURL.URL)
		}
	}
	if req.Metadata != nil {
		if contentRaw, ok := req.Metadata["content"]; ok {
			if contentItems, ok := contentRaw.([]interface{}); ok {
				for _, raw := range contentItems {
					item, ok := raw.(map[string]interface{})
					if !ok {
						continue
					}
					if url := extractVideoURLFromMap(item); url != "" {
						appendURL(url)
					}
				}
			}
		}
	}
	return urls
}

func extractVideoURLFromMap(item map[string]interface{}) string {
	videoRaw, ok := item["video_url"]
	if ok {
		if url := mediaURLFromRaw(videoRaw); url != "" {
			return url
		}
	}
	itemType, _ := item["type"].(string)
	role, _ := item["role"].(string)
	for _, key := range []string{"image_url", "audio_url"} {
		url := mediaURLFromRaw(item[key])
		if isReferenceVideoCandidate(itemType, role, url) {
			return url
		}
	}
	return ""
}

func mediaURLFromRaw(raw interface{}) string {
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v)
	case map[string]interface{}:
		if url, ok := v["url"].(string); ok {
			return strings.TrimSpace(url)
		}
	}
	return ""
}

func isReferenceVideoCandidate(itemType string, role string, rawURL string) bool {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return false
	}
	itemType = strings.ToLower(strings.TrimSpace(itemType))
	role = strings.ToLower(strings.TrimSpace(role))
	if itemType == "video_url" || role == "reference_video" {
		return true
	}
	if strings.HasPrefix(strings.ToLower(rawURL), "data:video/") {
		return true
	}
	lowerURL := strings.ToLower(rawURL)
	if u, err := url.Parse(lowerURL); err == nil {
		lowerURL = u.Path
	}
	for _, ext := range []string{".mp4", ".mov", ".m4v", ".webm", ".ogg", ".ogv", ".avi", ".mkv"} {
		if strings.HasSuffix(lowerURL, ext) {
			return true
		}
	}
	return false
}

func validateMultipartTaskRequest(c *gin.Context, info *RelayInfo, action string) (TaskSubmitReq, error) {
	var req TaskSubmitReq
	if _, err := c.MultipartForm(); err != nil {
		return req, err
	}

	formData := c.Request.PostForm
	req = TaskSubmitReq{
		Prompt:   formData.Get("prompt"),
		Model:    formData.Get("model"),
		Mode:     formData.Get("mode"),
		Image:    formData.Get("image"),
		Size:     formData.Get("size"),
		Metadata: make(map[string]interface{}),
	}

	if durationStr := formData.Get("seconds"); durationStr != "" {
		if duration, err := strconv.Atoi(durationStr); err == nil {
			req.Seconds = durationStr
			req.Duration = duration
		}
	}
	if durationStr := formData.Get("duration"); durationStr != "" {
		if duration, err := strconv.Atoi(durationStr); err == nil {
			req.Duration = duration
		}
	}

	if images := formData["images"]; len(images) > 0 {
		req.Images = images
	}

	for key, values := range formData {
		if len(values) > 0 && !isKnownTaskField(key) {
			if intVal, err := strconv.Atoi(values[0]); err == nil {
				req.Metadata[key] = intVal
			} else if floatVal, err := strconv.ParseFloat(values[0], 64); err == nil {
				req.Metadata[key] = floatVal
			} else {
				req.Metadata[key] = values[0]
			}
		}
	}
	return req, nil
}

func ValidateMultipartDirect(c *gin.Context, info *RelayInfo) *dto.TaskError {
	var prompt string
	var model string
	var seconds int
	var size string
	var hasInputReference bool

	var req TaskSubmitReq
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		return createTaskError(err, "invalid_json", http.StatusBadRequest, true)
	}

	prompt = req.Prompt
	model = req.Model
	size = req.Size
	seconds = EffectiveTaskDuration(req)
	if req.InputReference != "" {
		req.Images = []string{req.InputReference}
	}

	if strings.TrimSpace(req.Model) == "" {
		return createTaskError(fmt.Errorf("model field is required"), "missing_model", http.StatusBadRequest, true)
	}

	if req.HasImage() {
		hasInputReference = true
	}

	if taskErr := validatePrompt(prompt); taskErr != nil {
		return taskErr
	}

	action := constant.TaskActionTextGenerate
	if hasInputReference {
		action = constant.TaskActionGenerate
	}
	if strings.HasPrefix(model, "sora-2") {

		if size == "" {
			size = "720x1280"
		}

		if seconds <= 0 {
			seconds = 4
		}

		if model == "sora-2" && !lo.Contains([]string{"720x1280", "1280x720"}, size) {
			return createTaskError(fmt.Errorf("sora-2 size is invalid"), "invalid_size", http.StatusBadRequest, true)
		}
		if model == "sora-2-pro" && !lo.Contains([]string{"720x1280", "1280x720", "1792x1024", "1024x1792"}, size) {
			return createTaskError(fmt.Errorf("sora-2 size is invalid"), "invalid_size", http.StatusBadRequest, true)
		}
		// OtherRatios 已移到 Sora adaptor 的 EstimateBilling 中设置
	}

	normalizeTaskDuration(&req)
	applySeedanceDurationBounds(&req)
	storeTaskRequest(c, info, action, req)

	return nil
}

func isKnownTaskField(field string) bool {
	knownFields := map[string]bool{
		"prompt":          true,
		"model":           true,
		"mode":            true,
		"image":           true,
		"images":          true,
		"size":            true,
		"seconds":         true,
		"duration":        true,
		"input_reference": true, // Sora 特有字段
	}
	return knownFields[field]
}

func ValidateBasicTaskRequest(c *gin.Context, info *RelayInfo, action string) *dto.TaskError {
	var err error
	contentType := c.GetHeader("Content-Type")
	var req TaskSubmitReq
	if strings.HasPrefix(contentType, "multipart/form-data") {
		req, err = validateMultipartTaskRequest(c, info, action)
		if err != nil {
			return createTaskError(err, "invalid_multipart_form", http.StatusBadRequest, true)
		}
	}
	// 为了metadata字段的兼容性，统一UnmarshalBodyReusable
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		return createTaskError(err, "invalid_request", http.StatusBadRequest, true)
	}

	if taskErr := validatePrompt(req.Prompt); taskErr != nil {
		return taskErr
	}

	if len(req.Images) == 0 && strings.TrimSpace(req.Image) != "" {
		// 兼容单图上传
		req.Images = []string{req.Image}
	}

	normalizeTaskDuration(&req)
	applySeedanceDurationBounds(&req)
	storeTaskRequest(c, info, action, req)
	return nil
}
