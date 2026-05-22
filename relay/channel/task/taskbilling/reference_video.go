package taskbilling

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

const ReferenceVideoSecondsContextKey = "reference_video_seconds"

func EstimateReferenceVideoSeconds(c *gin.Context) int {
	totalSeconds, err := ResolveReferenceVideoSeconds(c)
	if err == nil {
		return totalSeconds
	}
	logger.LogWarn(c, fmt.Sprintf("failed to resolve reference video duration: %s", err.Error()))
	return 0
}

func ResolveReferenceVideoSeconds(c *gin.Context) (int, error) {
	if cached, exists := c.Get(ReferenceVideoSecondsContextKey); exists {
		if seconds, ok := cached.(int); ok {
			return seconds, nil
		}
	}
	urls := taskcommon.ExtractReferenceVideoURLs(c)
	if len(urls) == 0 {
		c.Set(ReferenceVideoSecondsContextKey, 0)
		return 0, nil
	}
	totalSeconds := 0
	for _, videoURL := range urls {
		seconds, err := ProbeReferenceVideoDurationSeconds(c, videoURL)
		if err != nil {
			return 0, fmt.Errorf("probe reference video %q duration failed: %w", common.MaskSensitiveInfo(videoURL), err)
		}
		totalSeconds += seconds
	}
	c.Set(ReferenceVideoSecondsContextKey, totalSeconds)
	return totalSeconds, nil
}

func ProbeReferenceVideoDurationSeconds(c *gin.Context, rawURL string) (int, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return 0, fmt.Errorf("empty reference video url")
	}
	if strings.HasPrefix(strings.ToLower(rawURL), "data:video/") {
		return probeDataURLVideoDurationSeconds(c, rawURL)
	}

	resp, err := service.DoDownloadRequest(rawURL, "seedance_reference_video_duration")
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return 0, fmt.Errorf("download reference video failed, status code: %d", resp.StatusCode)
	}
	if seconds := DurationSecondsFromHeaders(resp.Header); seconds > 0 {
		return seconds, nil
	}

	maxFileSize := constant.MaxFileDownloadMB * 1024 * 1024
	videoBytes, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxFileSize+1)))
	if err != nil {
		return 0, err
	}
	if len(videoBytes) > maxFileSize {
		return 0, fmt.Errorf("reference video exceeds maximum allowed size: %dMB", constant.MaxFileDownloadMB)
	}

	ext := referenceVideoDurationExt(rawURL, resp.Header.Get("Content-Type"))
	return detectVideoDurationSeconds(c, videoBytes, ext)
}

func probeDataURLVideoDurationSeconds(c *gin.Context, rawURL string) (int, error) {
	comma := strings.Index(rawURL, ",")
	if comma < 0 {
		return 0, fmt.Errorf("invalid data video url")
	}
	meta := strings.ToLower(rawURL[:comma])
	if !strings.Contains(meta, ";base64") {
		return 0, fmt.Errorf("data video url must be base64 encoded")
	}
	encoded := strings.NewReplacer("\n", "", "\r", "", " ", "").Replace(rawURL[comma+1:])
	videoBytes, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		if decoded, urlErr := base64.URLEncoding.DecodeString(encoded); urlErr == nil {
			videoBytes = decoded
			err = nil
		}
	}
	if err != nil {
		if decoded, rawErr := base64.RawStdEncoding.DecodeString(encoded); rawErr == nil {
			videoBytes = decoded
			err = nil
		}
	}
	if err != nil {
		if decoded, rawURLErr := base64.RawURLEncoding.DecodeString(encoded); rawURLErr == nil {
			videoBytes = decoded
			err = nil
		}
	}
	if err != nil {
		return 0, err
	}
	ext := ".mp4"
	if strings.Contains(meta, "video/webm") {
		ext = ".webm"
	} else if strings.Contains(meta, "quicktime") || strings.Contains(meta, "video/quicktime") {
		ext = ".mp4"
	}
	return detectVideoDurationSeconds(c, videoBytes, ext)
}

func DurationSecondsFromHeaders(header http.Header) int {
	for _, key := range []string{"X-Content-Duration", "Content-Duration", "X-Video-Duration", "Duration"} {
		value := strings.TrimSpace(header.Get(key))
		if value == "" {
			continue
		}
		value = strings.TrimSuffix(strings.ToLower(value), "s")
		if duration, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil && duration > 0 {
			return int(math.Ceil(duration))
		}
	}
	return 0
}

func referenceVideoDurationExt(rawURL string, contentType string) string {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	switch {
	case strings.Contains(contentType, "mp4"),
		strings.Contains(contentType, "quicktime"),
		strings.Contains(contentType, "x-m4v"),
		strings.Contains(contentType, "3gpp"):
		return ".mp4"
	case strings.Contains(contentType, "webm"):
		return ".webm"
	}

	parsedURL, err := url.Parse(rawURL)
	if err == nil {
		switch strings.ToLower(path.Ext(parsedURL.Path)) {
		case ".mp4", ".m4v", ".mov", ".3gp", ".3gpp":
			return ".mp4"
		case ".webm":
			return ".webm"
		}
	}
	return ".mp4"
}

func detectVideoDurationSeconds(c *gin.Context, videoBytes []byte, ext string) (int, error) {
	if len(videoBytes) == 0 {
		return 0, fmt.Errorf("empty reference video data")
	}
	duration, err := common.GetAudioDuration(c.Request.Context(), bytes.NewReader(videoBytes), ext)
	if err != nil {
		return 0, err
	}
	if duration <= 0 {
		return 0, fmt.Errorf("reference video duration is zero")
	}
	return int(math.Ceil(duration)), nil
}
