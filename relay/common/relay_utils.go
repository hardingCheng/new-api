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
	"github.com/QuantumNous/new-api/setting/model_setting"

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

func SanitizeURLForLog(rawURL string) string {
	if rawURL == "" {
		return rawURL
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	query := parsedURL.Query()
	if len(query) == 0 {
		return rawURL
	}

	changed := false
	for key := range query {
		if isSensitiveURLQueryKey(key) {
			query.Set(key, "***masked***")
			changed = true
		}
	}
	if !changed {
		return rawURL
	}

	parsedURL.RawQuery = query.Encode()
	return parsedURL.String()
}

func isSensitiveURLQueryKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	switch normalized {
	case "key",
		"api_key",
		"api-key",
		"apikey",
		"x-api-key",
		"access_token",
		"refresh_token",
		"id_token",
		"token",
		"authorization",
		"auth",
		"client_secret",
		"secret",
		"password",
		"passwd",
		"signature",
		"sig",
		"awsaccesskeyid",
		"x-amz-credential",
		"x-amz-security-token",
		"x-amz-signature":
		return true
	}
	return strings.Contains(normalized, "token") ||
		strings.Contains(normalized, "secret") ||
		strings.Contains(normalized, "signature")
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
	return strings.HasPrefix(model, "seedance-") ||
		strings.HasPrefix(model, "doubao-seedance-") ||
		strings.HasPrefix(model, "prism-")
}

func IsSeedanceRelayModel(info *RelayInfo, modelName string) bool {
	if IsSeedanceVideoModel(modelName) {
		return true
	}
	if info == nil {
		return false
	}
	if IsSeedanceVideoModel(info.OriginModelName) || IsSeedanceVideoModel(info.EffectiveRoutingModelName()) {
		return true
	}
	return info.ChannelMeta != nil && IsSeedanceVideoModel(info.UpstreamModelName)
}

func IsGrokImagineVideoModel(model string) bool {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "grok-imagine-1.0-video", "grok-imagine-video", "grok-imagine-video-1.5-preview":
		return true
	default:
		return false
	}
}

func ShouldFillGrokImagineInputReference(info *RelayInfo, req TaskSubmitReq) bool {
	if IsGrokImagineVideoModel(req.Model) {
		return true
	}
	if info == nil || info.ChannelMeta == nil {
		return false
	}
	return IsGrokImagineVideoModel(info.OriginModelName) || IsGrokImagineVideoModel(info.UpstreamModelName)
}

func FillMissingGrokImagineInputReference(info *RelayInfo, req *TaskSubmitReq) {
	if req == nil || !ShouldFillGrokImagineInputReference(info, *req) || len(req.InputReferenceValues()) > 0 {
		return
	}
	if values := referenceValuesFromImageFields(req.Image, req.Images); len(values) > 0 {
		req.SetInputReferenceValues(values)
	}
}

func referenceValuesFromImageFields(image string, images []string) []string {
	values := make([]string, 0, len(images)+1)
	if strings.TrimSpace(image) != "" {
		values = append(values, image)
	}
	values = append(values, images...)
	return normalizeStringList(values)
}

func FillMissingGrokImagineInputReferenceMap(info *RelayInfo, req TaskSubmitReq, bodyMap map[string]interface{}) {
	if bodyMap == nil || !ShouldFillGrokImagineInputReference(info, req) || hasNonEmptyField(bodyMap["input_reference"]) {
		return
	}
	values := req.InputReferenceValues()
	if len(values) == 0 {
		values = referenceValuesFromBodyMap(bodyMap)
	}
	if value := upstreamStringListValue(values); value != nil {
		bodyMap["input_reference"] = value
	}
}

func hasNonEmptyField(value interface{}) bool {
	switch v := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(v) != ""
	case []string:
		return len(normalizeStringList(v)) > 0
	case []interface{}:
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				return true
			}
		}
		return false
	default:
		return true
	}
}

func referenceValuesFromBodyMap(bodyMap map[string]interface{}) []string {
	values := make([]string, 0)
	appendValue := func(raw interface{}) {
		switch v := raw.(type) {
		case string:
			values = append(values, v)
		case []string:
			values = append(values, v...)
		case []interface{}:
			for _, item := range v {
				if s, ok := item.(string); ok {
					values = append(values, s)
				}
			}
		}
	}
	appendValue(bodyMap["image"])
	appendValue(bodyMap["images"])
	return normalizeStringList(values)
}

// IsGrokImagineVideo15Preview 匹配 grok-video-1.5-preview 与 grok-imagine-video-1.5-preview。
func IsGrokImagineVideo15Preview(model string) bool {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "grok-video-1.5-preview", "grok-imagine-video-1.5-preview":
		return true
	default:
		return false
	}
}

// FillGrokImagineVideo15PreviewImages 仅对 grok-video-1.5-preview /
// grok-imagine-video-1.5-preview 生效：始终用请求里的 reference_images 原样覆盖
// images 字段后再发给上游（上游需要 images 字段，但客户端只传 reference_images）。
// images 与 reference_images 完全一致，不做去重/去空格等归一化。
func FillGrokImagineVideo15PreviewImages(info *RelayInfo, req TaskSubmitReq, bodyMap map[string]interface{}) {
	if bodyMap == nil {
		return
	}
	if !IsGrokImagineVideo15Preview(req.Model) &&
		(info == nil || !IsGrokImagineVideo15Preview(info.OriginModelName)) {
		return
	}
	refs, ok := bodyMap["reference_images"]
	if !ok || !hasNonEmptyField(refs) {
		return
	}
	bodyMap["images"] = refs
}

// grokImagineImageURLChannelIds 指定渠道：这些渠道的 grok-video-1.5-preview /
// grok-imagine-video-1.5-preview 上游只认单个 image_url 字段，需要把客户端的
// reference_images 数组改写为 image_url（取第一个链接）。
var grokImagineImageURLChannelIds = map[int]bool{
	345: true,
	349: true,
}

// ShouldRewriteGrokImagineReferenceToImageURL 仅当命中指定渠道且模型为
// grok-video-1.5-preview / grok-imagine-video-1.5-preview 时为真。
func ShouldRewriteGrokImagineReferenceToImageURL(info *RelayInfo, req TaskSubmitReq) bool {
	if info == nil || info.ChannelMeta == nil || !grokImagineImageURLChannelIds[info.ChannelId] {
		return false
	}
	return IsGrokImagineVideo15Preview(req.Model) || IsGrokImagineVideo15Preview(info.OriginModelName)
}

// RewriteGrokImagineReferenceToImageURL 删除 reference_images，并把第一个链接写入 image_url。
func RewriteGrokImagineReferenceToImageURL(bodyMap map[string]interface{}) {
	if bodyMap == nil {
		return
	}
	refs, ok := bodyMap["reference_images"]
	if !ok {
		return
	}
	first := firstReferenceImageURL(refs)
	delete(bodyMap, "reference_images")
	if first != "" {
		bodyMap["image_url"] = first
	}
}

func firstReferenceImageURL(raw interface{}) string {
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v)
	case []string:
		for _, s := range v {
			if s = strings.TrimSpace(s); s != "" {
				return s
			}
		}
	case []interface{}:
		for _, item := range v {
			if s, ok := item.(string); ok {
				if s = strings.TrimSpace(s); s != "" {
					return s
				}
			}
		}
	}
	return ""
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

func applySeedanceDurationBounds(req *TaskSubmitReq, info *RelayInfo) {
	if req == nil || !IsSeedanceRelayModel(info, req.Model) {
		return
	}
	seconds := EffectiveTaskDuration(*req)
	seconds = clampSeedanceDuration(seconds)
	req.Duration = seconds
	req.Seconds = strconv.Itoa(seconds)
}

func validateReferenceVideoPolicy(req TaskSubmitReq, info *RelayInfo) *dto.TaskError {
	if info == nil || info.ReferenceVideoPolicy != model_setting.ReferenceVideoForbidden {
		return nil
	}
	if len(ExtractReferenceVideoURLs(req)) == 0 {
		return nil
	}
	return createTaskError(fmt.Errorf("reference video is not supported for model %s", req.Model), "reference_video_not_supported", http.StatusBadRequest, true)
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

// MaxTaskDurationSeconds caps user-supplied video duration. Duration is used
// as a billing multiplier (OtherRatio "seconds"); an unbounded value could
// overflow quota calculation into a negative charge.
const MaxTaskDurationSeconds = 3600

func ValidateTaskDurationBounds(req TaskSubmitReq) *dto.TaskError {
	if req.Duration < 0 || req.Duration > MaxTaskDurationSeconds {
		return createTaskError(fmt.Errorf("seconds must be between 1 and %d", MaxTaskDurationSeconds), "invalid_seconds", http.StatusBadRequest, true)
	}
	if req.Seconds != "" {
		seconds, err := strconv.Atoi(req.Seconds)
		if err != nil || seconds < 0 || seconds > MaxTaskDurationSeconds {
			return createTaskError(fmt.Errorf("seconds must be between 1 and %d", MaxTaskDurationSeconds), "invalid_seconds", http.StatusBadRequest, true)
		}
	}
	if EffectiveTaskDuration(req) > MaxTaskDurationSeconds {
		return createTaskError(fmt.Errorf("seconds must be between 1 and %d", MaxTaskDurationSeconds), "invalid_seconds", http.StatusBadRequest, true)
	}
	return nil
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
	if inputReferences := formData["input_reference"]; len(inputReferences) > 0 {
		req.SetInputReferenceValues(inputReferences)
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
	FillMissingGrokImagineInputReference(info, &req)
	if req.InputReference != "" {
		req.Images = []string{req.InputReference}
	} else if len(req.Images) == 0 && strings.TrimSpace(req.Image) != "" {
		// 兼容单图上传
		req.Images = []string{strings.TrimSpace(req.Image)}
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

	if taskErr := ValidateTaskDurationBounds(req); taskErr != nil {
		return taskErr
	}
	if taskErr := validateReferenceVideoPolicy(req, info); taskErr != nil {
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
	applySeedanceDurationBounds(&req, info)
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

	if taskErr := ValidateTaskDurationBounds(req); taskErr != nil {
		return taskErr
	}
	if taskErr := validateReferenceVideoPolicy(req, info); taskErr != nil {
		return taskErr
	}

	if len(req.Images) == 0 && strings.TrimSpace(req.Image) != "" {
		// 兼容单图上传
		req.Images = []string{req.Image}
	}
	FillMissingGrokImagineInputReference(info, &req)

	normalizeTaskDuration(&req)
	applySeedanceDurationBounds(&req, info)
	storeTaskRequest(c, info, action, req)
	return nil
}
