package taskcommon

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

// UnmarshalMetadata converts a map[string]any metadata to a typed struct via JSON round-trip.
// This replaces the repeated pattern: json.Marshal(metadata) → json.Unmarshal(bytes, &target).
func UnmarshalMetadata(metadata map[string]any, target any) error {
	if metadata == nil {
		return nil
	}
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

// Status-to-progress mapping constants for polling updates.
const (
	ProgressSubmitted  = "10%"
	ProgressQueued     = "20%"
	ProgressInProgress = "30%"
	ProgressComplete   = "100%"
)

// videoExtensions contains common video file extensions used to detect video
// content even when the type field is mislabeled (e.g. marked as "image_url").
var videoExtensions = []string{
	".mp4", ".mov", ".avi", ".mkv", ".webm", ".flv", ".wmv",
	".m4v", ".3gp", ".3gpp", ".mpeg", ".mpg", ".ts", ".mts",
	".vob", ".ogv", ".asf", ".rm", ".rmvb", ".f4v",
}

// HasVideoURLContent reads the raw request body from context and checks whether
// the JSON "content" array contains video content. Detection includes:
//  1. type == "video_url" or "video" (explicit video type)
//  2. Any URL (in image_url, video_url, video, etc.) with a video file extension
//  3. Any data URI with a video/* MIME type
func HasVideoURLContent(c *gin.Context) bool {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return false
	}
	bodyBytes, err := storage.Bytes()
	if err != nil {
		return false
	}
	var body map[string]interface{}
	if err := common.Unmarshal(bodyBytes, &body); err != nil {
		return false
	}
	contentRaw, ok := body["content"]
	if !ok {
		return false
	}
	contentArr, ok := contentRaw.([]interface{})
	if !ok {
		return false
	}
	for _, item := range contentArr {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		// 1) Explicit video type
		t, _ := itemMap["type"].(string)
		if t == "video_url" || t == "video" {
			return true
		}
		// 2) Check all nested URL strings for video signatures
		for _, key := range []string{"image_url", "video_url", "video", "audio_url"} {
			urlStr := extractURL(itemMap, key)
			if urlStr != "" && isVideoURL(urlStr) {
				return true
			}
		}
		// 3) Check top-level "url" field in the item
		if urlStr, _ := itemMap["url"].(string); urlStr != "" && isVideoURL(urlStr) {
			return true
		}
	}
	return false
}

// extractURL extracts the URL string from a content item's nested field.
// Handles both object form {"url": "..."} and plain string form.
func extractURL(itemMap map[string]interface{}, key string) string {
	val, ok := itemMap[key]
	if !ok {
		return ""
	}
	// Plain string: "image_url": "https://..."
	if s, ok := val.(string); ok {
		return s
	}
	// Object form: "image_url": {"url": "https://..."}
	if obj, ok := val.(map[string]interface{}); ok {
		if u, ok := obj["url"].(string); ok {
			return u
		}
	}
	return ""
}

// isVideoURL checks if a URL points to video content by examining:
// - data URI MIME type (data:video/...)
// - file extension in the URL path
func isVideoURL(rawURL string) bool {
	lower := strings.ToLower(rawURL)
	// data URI with video MIME type
	if strings.HasPrefix(lower, "data:video/") {
		return true
	}
	// Strip query string and fragment for extension check
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

// IsSeedance2Model returns true if the model name starts with "seedance-2.0" or "doubao-seedance-2.0".
func IsSeedance2Model(modelName string) bool {
	m := strings.ToLower(strings.TrimSpace(modelName))
	return strings.HasPrefix(m, "seedance-2.0") || strings.HasPrefix(m, "doubao-seedance-2.0")
}

func IsGrokImagineVideoModel(modelName string) bool {
	m := strings.ToLower(strings.TrimSpace(modelName))
	return m == "grok-imagine-video" || m == "grok-imagine-1.0-video"
}

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
