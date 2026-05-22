package taskcommon

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

// UnmarshalMetadata converts a map[string]any metadata to a typed struct via JSON round-trip.
// This replaces the repeated pattern: json.Marshal(metadata) → json.Unmarshal(bytes, &target).
func UnmarshalMetadata(metadata map[string]any, target any) error {
	if metadata == nil {
		return nil
	}
	// Prevent metadata from overriding model fields to avoid billing bypass.
	delete(metadata, "model")
	metaBytes, err := common.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata failed: %w", err)
	}
	if err := common.Unmarshal(metaBytes, target); err != nil {
		return fmt.Errorf("unmarshal metadata failed: %w", err)
	}
	return nil
}

// DefaultString returns val if non-empty, otherwise fallback.
func DefaultString(val, fallback string) string {
	if val == "" {
		return fallback
	}
	return val
}

// DefaultInt returns val if non-zero, otherwise fallback.
func DefaultInt(val, fallback int) int {
	if val == 0 {
		return fallback
	}
	return val
}

// EncodeLocalTaskID encodes an upstream operation name to a URL-safe base64 string.
// Used by Gemini/Vertex to store upstream names as task IDs.
func EncodeLocalTaskID(name string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(name))
}

// DecodeLocalTaskID decodes a base64-encoded upstream operation name.
func DecodeLocalTaskID(id string) (string, error) {
	b, err := base64.RawURLEncoding.DecodeString(id)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// BuildProxyURL constructs the video proxy URL using the public task ID.
// e.g., "https://your-server.com/v1/videos/task_xxxx/content"
func BuildProxyURL(taskID string) string {
	return fmt.Sprintf("%s/v1/videos/%s/content", system_setting.ServerAddress, taskID)
}

func IsTaskDurationBillingRequest(info *relaycommon.RelayInfo, requestModel string) bool {
	if info == nil {
		return model_setting.IsTaskDurationBillingModel(requestModel)
	}
	return model_setting.IsTaskDurationBillingModel(requestModel, info.OriginModelName, info.UpstreamModelName)
}

func IsTaskReferenceVideoBillingRequest(info *relaycommon.RelayInfo, requestModel string) bool {
	if info == nil {
		return model_setting.IsTaskReferenceVideoBillingModel(requestModel)
	}
	return model_setting.IsTaskReferenceVideoBillingModel(requestModel, info.OriginModelName, info.UpstreamModelName)
}

var videoExtensions = []string{
	".mp4", ".mov", ".avi", ".mkv", ".webm", ".flv", ".wmv",
	".m4v", ".3gp", ".3gpp", ".mpeg", ".mpg", ".ts", ".mts",
	".vob", ".ogv", ".asf", ".rm", ".rmvb", ".f4v",
}

func HasVideoURLContent(c *gin.Context) bool {
	return len(ExtractReferenceVideoURLs(c)) > 0
}

func ExtractReferenceVideoURLs(c *gin.Context) []string {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil
	}
	bodyBytes, err := storage.Bytes()
	if err != nil {
		return nil
	}
	return ExtractReferenceVideoURLsFromBody(bodyBytes)
}

func ExtractReferenceVideoURLsFromBody(bodyBytes []byte) []string {
	var body any
	if err := common.Unmarshal(bodyBytes, &body); err != nil {
		return nil
	}
	urlSet := make(map[string]struct{})
	collectReferenceVideoURLs(body, urlSet)
	urls := make([]string, 0, len(urlSet))
	for url := range urlSet {
		urls = append(urls, url)
	}
	return urls
}

func collectReferenceVideoURLs(value any, urls map[string]struct{}) {
	switch v := value.(type) {
	case map[string]interface{}:
		if isReferenceVideoItem(v) {
			collectReferenceVideoSources(v, urls, true)
		} else {
			collectReferenceVideoSources(v, urls, false)
		}
		for _, child := range v {
			collectReferenceVideoURLs(child, urls)
		}
	case []interface{}:
		for _, child := range v {
			collectReferenceVideoURLs(child, urls)
		}
	}
}

func collectReferenceVideoSources(itemMap map[string]interface{}, urls map[string]struct{}, allowMarkedSource bool) {
	for _, source := range extractVideoSourceCandidates(itemMap) {
		if normalized := normalizeReferenceVideoSource(source, allowMarkedSource); normalized != "" {
			urls[normalized] = struct{}{}
		}
	}
}

func isReferenceVideoItem(itemMap map[string]interface{}) bool {
	contentType, _ := itemMap["type"].(string)
	role, _ := itemMap["role"].(string)
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	role = strings.ToLower(strings.TrimSpace(role))
	if contentType == "video_url" || contentType == "video" || role == "reference_video" {
		return true
	}
	_, hasVideoURL := itemMap["video_url"]
	return hasVideoURL
}

func extractVideoURLCandidates(itemMap map[string]interface{}) []string {
	return extractVideoSourceCandidates(itemMap)
}

func extractVideoSourceCandidates(itemMap map[string]interface{}) []string {
	candidates := make([]string, 0, 3)
	for _, key := range []string{"video_url", "video", "image_url", "url", "base64", "b64_json", "data"} {
		candidates = append(candidates, extractStringCandidates(itemMap[key])...)
	}
	return candidates
}

func extractStringCandidates(value interface{}) []string {
	switch v := value.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		return []string{v}
	case map[string]interface{}:
		candidates := make([]string, 0, 3)
		for _, key := range []string{"url", "base64", "b64_json", "data"} {
			candidates = append(candidates, extractStringCandidates(v[key])...)
		}
		return candidates
	case []interface{}:
		var candidates []string
		for _, item := range v {
			candidates = append(candidates, extractStringCandidates(item)...)
		}
		return candidates
	default:
		return nil
	}
}

func normalizeReferenceVideoSource(source string, allowMarkedSource bool) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return ""
	}
	lower := strings.ToLower(source)
	if strings.HasPrefix(lower, "data:video/") {
		return source
	}
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		if allowMarkedSource || isVideoURL(source) {
			return source
		}
		return ""
	}
	if isVideoURL(source) {
		return source
	}
	if allowMarkedSource && looksLikeBase64Video(source) {
		return "data:video/mp4;base64," + source
	}
	return ""
}

func looksLikeBase64Video(source string) bool {
	source = strings.TrimSpace(source)
	if len(source) < 32 {
		return false
	}
	for _, r := range source {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') ||
			r == '+' || r == '/' || r == '=' || r == '-' || r == '_' || r == '\n' || r == '\r' {
			continue
		}
		return false
	}
	return true
}

func containsVideoURL(value any) bool {
	switch v := value.(type) {
	case string:
		return normalizeReferenceVideoSource(v, true) != ""
	case map[string]interface{}:
		for _, child := range v {
			if containsVideoURL(child) {
				return true
			}
		}
	case []interface{}:
		for _, child := range v {
			if containsVideoURL(child) {
				return true
			}
		}
	}
	return false
}

func ContainsReferenceVideo(value any) bool {
	switch v := value.(type) {
	case map[string]interface{}:
		if isReferenceVideoItem(v) && containsVideoURL(v) {
			return true
		}
		for _, child := range v {
			if ContainsReferenceVideo(child) {
				return true
			}
		}
	case []interface{}:
		for _, child := range v {
			if ContainsReferenceVideo(child) {
				return true
			}
		}
	}
	return false
}

func extractURL(itemMap map[string]interface{}, key string) string {
	val, ok := itemMap[key]
	if !ok {
		return ""
	}
	if s, ok := val.(string); ok {
		return s
	}
	if obj, ok := val.(map[string]interface{}); ok {
		if url, ok := obj["url"].(string); ok {
			return url
		}
	}
	return ""
}

func isVideoURL(rawURL string) bool {
	lower := strings.ToLower(strings.TrimSpace(rawURL))
	if lower == "" {
		return false
	}
	if strings.HasPrefix(lower, "data:video/") {
		return true
	}
	path := lower
	if idx := strings.IndexAny(path, "?#"); idx != -1 {
		path = path[:idx]
	}
	for _, ext := range videoExtensions {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}

func IsSeedance2Model(modelName string) bool {
	modelName = strings.ToLower(strings.TrimSpace(modelName))
	return strings.HasPrefix(modelName, "seedance-2.0") ||
		strings.HasPrefix(modelName, "doubao-seedance-2.0")
}

// Status-to-progress mapping constants for polling updates.
const (
	ProgressSubmitted  = "10%"
	ProgressQueued     = "20%"
	ProgressInProgress = "30%"
	ProgressComplete   = "100%"
)

// ---------------------------------------------------------------------------
// BaseBilling — embeddable no-op implementations for TaskAdaptor billing methods.
// Adaptors that do not need custom billing can embed this struct directly.
// ---------------------------------------------------------------------------

type BaseBilling struct{}

// EstimateBilling returns nil (no extra ratios; use base model price).
func (BaseBilling) EstimateBilling(_ *gin.Context, _ *relaycommon.RelayInfo) map[string]float64 {
	return nil
}

// AdjustBillingOnSubmit returns nil (no submit-time adjustment).
func (BaseBilling) AdjustBillingOnSubmit(_ *relaycommon.RelayInfo, _ []byte) map[string]float64 {
	return nil
}

// AdjustBillingOnComplete returns 0 (keep pre-charged amount).
func (BaseBilling) AdjustBillingOnComplete(_ *model.Task, _ *relaycommon.TaskInfo) int {
	return 0
}
