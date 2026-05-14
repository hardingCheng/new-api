package taskcommon

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/cachex"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/samber/hot"
)

const (
	referenceVideoSummaryContextKey = "reference_video_duration_summary"
	referenceVideoBillingModeKey    = "reference_video_billing_mode"
	referenceVideoDurationCacheNS   = "new-api:reference_video_duration:v1"
	defaultReferenceVideoProbeMB    = 128
	defaultReferenceVideoProbeSecs  = 20
	defaultReferenceVideoCacheTTL   = 24 * 60 * 60
	defaultReferenceVideoCacheCap   = 10_000
)

type ReferenceVideoDurationSummary struct {
	DetectedCount int                            `json:"detected_count"`
	ProbedCount   int                            `json:"probed_count"`
	TotalSeconds  float64                        `json:"total_seconds"`
	FailedCount   int                            `json:"failed_count,omitempty"`
	Details       []ReferenceVideoDurationDetail `json:"details,omitempty"`
}

type ReferenceVideoDurationDetail struct {
	Index       int     `json:"index,omitempty"`
	SourceType  string  `json:"source_type,omitempty"`
	SourceHost  string  `json:"source_host,omitempty"`
	SourceHash  string  `json:"source_hash,omitempty"`
	Duration    float64 `json:"duration,omitempty"`
	ProbeMethod string  `json:"probe_method,omitempty"`
	Status      string  `json:"status,omitempty"`
	ErrorCode   string  `json:"error_code,omitempty"`
	Error       string  `json:"error,omitempty"`
}

type referenceVideoProbeResult struct {
	Duration    float64
	ProbeMethod string
}

type referenceVideoDurationCacheEntry struct {
	Duration float64 `json:"duration"`
}

type referenceVideoDurationCacheCodec struct{}

func (c referenceVideoDurationCacheCodec) Encode(v referenceVideoDurationCacheEntry) (string, error) {
	b, err := common.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (c referenceVideoDurationCacheCodec) Decode(s string) (referenceVideoDurationCacheEntry, error) {
	var entry referenceVideoDurationCacheEntry
	if err := common.UnmarshalJsonStr(s, &entry); err != nil {
		return entry, err
	}
	return entry, nil
}

var referenceVideoDurationProbe = probeReferenceVideoDuration

var (
	referenceVideoDurationCacheOnce sync.Once
	referenceVideoDurationCache     *cachex.HybridCache[referenceVideoDurationCacheEntry]

	referenceVideoFFProbeLookupOnce sync.Once
	referenceVideoFFProbePath       string
	referenceVideoFFProbeErr        error
)

func roundReferenceVideoSeconds(seconds float64) float64 {
	if seconds <= 0 {
		return seconds
	}
	return math.Round(seconds*100) / 100
}

func referenceVideoProbeTimeout() time.Duration {
	seconds := common.GetEnvOrDefault("REFERENCE_VIDEO_PROBE_TIMEOUT_SECONDS", defaultReferenceVideoProbeSecs)
	if seconds <= 0 {
		seconds = defaultReferenceVideoProbeSecs
	}
	return time.Duration(seconds) * time.Second
}

func referenceVideoProbeMaxBytes() int64 {
	maxMB := common.GetEnvOrDefault("REFERENCE_VIDEO_PROBE_MAX_MB", defaultReferenceVideoProbeMB)
	if maxMB <= 0 {
		maxMB = defaultReferenceVideoProbeMB
	}
	return int64(maxMB) << 20
}

func referenceVideoDurationCacheTTL() time.Duration {
	seconds := common.GetEnvOrDefault("REFERENCE_VIDEO_DURATION_CACHE_TTL", defaultReferenceVideoCacheTTL)
	if seconds <= 0 {
		seconds = defaultReferenceVideoCacheTTL
	}
	return time.Duration(seconds) * time.Second
}

func referenceVideoDurationCacheCapacity() int {
	capacity := common.GetEnvOrDefault("REFERENCE_VIDEO_DURATION_CACHE_CAP", defaultReferenceVideoCacheCap)
	if capacity <= 0 {
		capacity = defaultReferenceVideoCacheCap
	}
	return capacity
}

func getReferenceVideoDurationCache() *cachex.HybridCache[referenceVideoDurationCacheEntry] {
	referenceVideoDurationCacheOnce.Do(func() {
		ttl := referenceVideoDurationCacheTTL()
		referenceVideoDurationCache = cachex.NewHybridCache[referenceVideoDurationCacheEntry](cachex.HybridCacheConfig[referenceVideoDurationCacheEntry]{
			Namespace: cachex.Namespace(referenceVideoDurationCacheNS),
			Redis:     common.RDB,
			RedisEnabled: func() bool {
				return common.RedisEnabled && common.RDB != nil
			},
			RedisCodec: referenceVideoDurationCacheCodec{},
			Memory: func() *hot.HotCache[string, referenceVideoDurationCacheEntry] {
				return hot.NewHotCache[string, referenceVideoDurationCacheEntry](hot.LRU, referenceVideoDurationCacheCapacity()).
					WithTTL(ttl).
					WithJanitor().
					Build()
			},
		})
	})
	return referenceVideoDurationCache
}

func lookupReferenceVideoFFProbe() (string, error) {
	referenceVideoFFProbeLookupOnce.Do(func() {
		bin := strings.TrimSpace(common.GetEnvOrDefaultString("REFERENCE_VIDEO_FFPROBE_BIN", "ffprobe"))
		if bin == "" {
			referenceVideoFFProbeErr = fmt.Errorf("ffprobe binary not configured")
			common.SysLog("reference video probe: ffprobe disabled because binary is not configured")
			return
		}
		referenceVideoFFProbePath, referenceVideoFFProbeErr = exec.LookPath(bin)
		if referenceVideoFFProbeErr != nil {
			common.SysLog(fmt.Sprintf("reference video probe: ffprobe unavailable (%v), falling back to native parser", referenceVideoFFProbeErr))
			return
		}
		common.SysLog(fmt.Sprintf("reference video probe: ffprobe enabled at %s", referenceVideoFFProbePath))
	})
	return referenceVideoFFProbePath, referenceVideoFFProbeErr
}

func cacheReferenceVideoBillingMode(c *gin.Context, mode string) {
	if c == nil {
		return
	}
	c.Set(referenceVideoBillingModeKey, operation_setting.NormalizeSeedanceReferenceVideoBillingMode(mode))
}

func GetCachedSeedanceReferenceVideoBillingMode(c *gin.Context) string {
	if c == nil {
		return ""
	}
	v, ok := c.Get(referenceVideoBillingModeKey)
	if !ok || v == nil {
		return ""
	}
	mode, ok := v.(string)
	if !ok {
		return ""
	}
	return operation_setting.NormalizeSeedanceReferenceVideoBillingMode(mode)
}

func GetCachedReferenceVideoDurationSummary(c *gin.Context) *ReferenceVideoDurationSummary {
	if c == nil {
		return nil
	}
	v, ok := c.Get(referenceVideoSummaryContextKey)
	if !ok || v == nil {
		return nil
	}
	summary, ok := v.(*ReferenceVideoDurationSummary)
	if !ok {
		return nil
	}
	return summary
}

func SummarizeReferenceVideoDurations(c *gin.Context) (*ReferenceVideoDurationSummary, error) {
	if summary := GetCachedReferenceVideoDurationSummary(c); summary != nil {
		return summary, nil
	}

	sources, err := extractReferenceVideoSources(c)
	if err != nil {
		return nil, err
	}

	summary := &ReferenceVideoDurationSummary{
		DetectedCount: len(sources),
	}
	if c != nil {
		c.Set(referenceVideoSummaryContextKey, summary)
	}
	if len(sources) == 0 {
		return summary, nil
	}

	ctx := context.Background()
	if c != nil && c.Request != nil {
		ctx = c.Request.Context()
	}

	for _, source := range sources {
		detail := buildReferenceVideoDurationDetail(len(summary.Details)+1, source)
		result, probeErr := referenceVideoDurationProbe(ctx, source)
		if probeErr != nil {
			summary.FailedCount++
			detail.Status = "failed"
			detail.ErrorCode = classifyReferenceVideoProbeError(probeErr)
			detail.Error = sanitizeReferenceVideoProbeError(source, probeErr)
			summary.Details = append(summary.Details, detail)
			common.SysLog(fmt.Sprintf("reference video duration probe skipped for %s: %v", safeReferenceVideoSourceRef(source), detail.Error))
			continue
		}
		if result != nil {
			result.Duration = roundReferenceVideoSeconds(result.Duration)
			detail.Duration = result.Duration
			detail.ProbeMethod = result.ProbeMethod
		}
		if result == nil || result.Duration <= 0 {
			summary.FailedCount++
			detail.Status = "failed"
			detail.ErrorCode = "invalid_duration"
			detail.Error = "invalid reference video duration"
			summary.Details = append(summary.Details, detail)
			continue
		}
		detail.Status = "success"
		summary.ProbedCount++
		summary.TotalSeconds = roundReferenceVideoSeconds(summary.TotalSeconds + result.Duration)
		summary.Details = append(summary.Details, detail)
	}

	return summary, nil
}

func BuildSeedanceReferenceVideoBillingRatios(c *gin.Context, outputSeconds int) map[string]float64 {
	if c == nil || outputSeconds <= 0 {
		return nil
	}

	if !HasVideoURLContent(c) {
		return nil
	}

	ratios := map[string]float64{
		"seconds": float64(outputSeconds),
	}
	configuredMode := operation_setting.GetSeedanceReferenceVideoBillingMode()

	summary, err := SummarizeReferenceVideoDurations(c)
	if err != nil {
		common.SysLog(fmt.Sprintf("reference video duration summary failed: %v", err))
		cacheReferenceVideoBillingMode(c, operation_setting.SeedanceReferenceVideoBillingModeLegacy)
		ratios["reference_video"] = 2.0
		return ratios
	}

	if summary == nil || summary.DetectedCount == 0 {
		cacheReferenceVideoBillingMode(c, operation_setting.SeedanceReferenceVideoBillingModeLegacy)
		ratios["reference_video"] = 2.0
		return ratios
	}

	switch configuredMode {
	case operation_setting.SeedanceReferenceVideoBillingModeDurationOnly:
		if summary.ProbedCount != summary.DetectedCount || summary.TotalSeconds <= 0 {
			cacheReferenceVideoBillingMode(c, operation_setting.SeedanceReferenceVideoBillingModeLegacy)
			ratios["reference_video"] = 2.0
			return ratios
		}
		cacheReferenceVideoBillingMode(c, operation_setting.SeedanceReferenceVideoBillingModeDurationOnly)
		ratios["reference_video"] = 1.0
		ratios["seconds"] = float64(outputSeconds) + summary.TotalSeconds
	default:
		cacheReferenceVideoBillingMode(c, operation_setting.SeedanceReferenceVideoBillingModeLegacy)
		ratios["reference_video"] = 2.0
	}

	return ratios
}

func extractReferenceVideoSources(c *gin.Context) ([]string, error) {
	if c == nil {
		return nil, nil
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, err
	}
	bodyBytes, err := storage.Bytes()
	if err != nil {
		return nil, err
	}

	var body map[string]interface{}
	if err := common.Unmarshal(bodyBytes, &body); err != nil {
		return nil, err
	}

	contentArr, ok := extractContentArray(body)
	if !ok {
		return nil, nil
	}

	sources := make([]string, 0)
	for _, item := range contentArr {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if source := extractReferenceVideoSource(itemMap); source != "" {
			sources = append(sources, source)
		}
	}
	return sources, nil
}

func extractContentArray(body map[string]interface{}) ([]interface{}, bool) {
	if contentRaw, ok := body["content"]; ok {
		contentArr, ok := contentRaw.([]interface{})
		if ok {
			return contentArr, true
		}
	}
	if metadataRaw, ok := body["metadata"]; ok {
		metadataMap, ok := metadataRaw.(map[string]interface{})
		if !ok {
			return nil, false
		}
		contentRaw, ok := metadataMap["content"]
		if !ok {
			return nil, false
		}
		contentArr, ok := contentRaw.([]interface{})
		return contentArr, ok
	}
	return nil, false
}

func extractReferenceVideoSource(itemMap map[string]interface{}) string {
	contentType := normalizeContentValue(itemMap["type"])
	role := normalizeContentValue(itemMap["role"])

	if contentType == "video_url" || contentType == "video" || role == "reference_video" {
		if source := firstVideoSource(itemMap); source != "" {
			return source
		}
	}

	if source := extractURL(itemMap, "video_url"); source != "" && isVideoURL(source) {
		return source
	}
	if source := extractURL(itemMap, "video"); source != "" && isVideoURL(source) {
		return source
	}
	if source, _ := itemMap["url"].(string); source != "" && isVideoURL(source) {
		return source
	}
	return ""
}

func firstVideoSource(itemMap map[string]interface{}) string {
	for _, key := range []string{"video_url", "video"} {
		if source := extractURL(itemMap, key); source != "" {
			return source
		}
	}
	if source, _ := itemMap["url"].(string); strings.TrimSpace(source) != "" {
		return source
	}
	return ""
}

func normalizeContentValue(v interface{}) string {
	s, _ := v.(string)
	return strings.ToLower(strings.TrimSpace(s))
}

func probeReferenceVideoDuration(ctx context.Context, source string) (*referenceVideoProbeResult, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return nil, fmt.Errorf("empty reference video source")
	}

	if shouldCacheReferenceVideoSource(source) {
		if duration, ok := getCachedReferenceVideoDuration(source); ok {
			return &referenceVideoProbeResult{
				Duration:    duration,
				ProbeMethod: "cache",
			}, nil
		}
	}

	lowerSource := strings.ToLower(source)
	var result *referenceVideoProbeResult
	var err error
	switch {
	case strings.HasPrefix(lowerSource, "data:video/"):
		result, err = probeReferenceVideoDataURI(ctx, source)
	case strings.HasPrefix(lowerSource, "http://") || strings.HasPrefix(lowerSource, "https://"):
		result, err = probeReferenceVideoURL(ctx, source)
	default:
		return nil, fmt.Errorf("unsupported reference video source")
	}

	if err == nil && result != nil && result.Duration > 0 && shouldCacheReferenceVideoSource(source) {
		cacheReferenceVideoDuration(source, result.Duration)
	}
	return result, err
}

func probeReferenceVideoDataURI(ctx context.Context, source string) (*referenceVideoProbeResult, error) {
	mimeType, dataPart, isBase64, err := parseDataURI(source)
	if err != nil {
		return nil, err
	}
	ext := referenceVideoExtFromMime(mimeType)

	tmpFile, err := os.CreateTemp("", "reference-video-*"+tempFileSuffix(ext))
	if err != nil {
		return nil, err
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)
	defer tmpFile.Close()

	var reader io.Reader
	if isBase64 {
		reader = base64.NewDecoder(base64.StdEncoding, strings.NewReader(dataPart))
	} else {
		decoded, decodeErr := url.PathUnescape(dataPart)
		if decodeErr != nil {
			return nil, decodeErr
		}
		reader = strings.NewReader(decoded)
	}

	maxBytes := referenceVideoProbeMaxBytes()
	written, err := io.Copy(tmpFile, io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if written > maxBytes {
		return nil, fmt.Errorf("reference video data uri exceeds %d bytes", maxBytes)
	}
	if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	return probeReferenceVideoTempFile(ctx, tmpPath, tmpFile, ext)
}

func probeReferenceVideoURL(ctx context.Context, source string) (*referenceVideoProbeResult, error) {
	fetchSetting := system_setting.GetFetchSetting()
	if err := common.ValidateURLWithFetchSetting(
		source,
		fetchSetting.EnableSSRFProtection,
		fetchSetting.AllowPrivateIp,
		fetchSetting.DomainFilterMode,
		fetchSetting.IpFilterMode,
		fetchSetting.DomainList,
		fetchSetting.IpList,
		fetchSetting.AllowedPorts,
		fetchSetting.ApplyIPFilterForDomain,
	); err != nil {
		return nil, err
	}

	probeTimeout := referenceVideoProbeTimeout()
	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	client := &http.Client{
		Timeout: probeTimeout,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			return common.ValidateURLWithFetchSetting(
				req.URL.String(),
				fetchSetting.EnableSSRFProtection,
				fetchSetting.AllowPrivateIp,
				fetchSetting.DomainFilterMode,
				fetchSetting.IpFilterMode,
				fetchSetting.DomainList,
				fetchSetting.IpList,
				fetchSetting.AllowedPorts,
				fetchSetting.ApplyIPFilterForDomain,
			)
		},
	}

	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, source, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "new-api/reference-video-probe")
	req.Header.Set("Accept", "*/*")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status code %d", resp.StatusCode)
	}
	maxBytes := referenceVideoProbeMaxBytes()
	if resp.ContentLength > maxBytes {
		return nil, fmt.Errorf("reference video exceeds %d bytes", maxBytes)
	}

	ext := referenceVideoExtFromURL(source)
	if ext == "" {
		ext = referenceVideoExtFromMime(resp.Header.Get("Content-Type"))
	}

	tmpFile, err := os.CreateTemp("", "reference-video-*"+tempFileSuffix(ext))
	if err != nil {
		return nil, err
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)
	defer tmpFile.Close()

	written, err := io.Copy(tmpFile, io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if written > maxBytes {
		return nil, fmt.Errorf("reference video exceeds %d bytes", maxBytes)
	}
	if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	return probeReferenceVideoTempFile(probeCtx, tmpPath, tmpFile, ext)
}

func parseDataURI(source string) (mimeType, dataPart string, isBase64 bool, err error) {
	if !strings.HasPrefix(strings.ToLower(source), "data:") {
		return "", "", false, fmt.Errorf("invalid data uri")
	}

	parts := strings.SplitN(source, ",", 2)
	if len(parts) != 2 {
		return "", "", false, fmt.Errorf("invalid data uri payload")
	}

	header := strings.TrimPrefix(parts[0], "data:")
	dataPart = parts[1]
	if header == "" {
		return "", "", false, fmt.Errorf("missing data uri mime type")
	}

	headerParts := strings.Split(header, ";")
	mimeType = strings.ToLower(strings.TrimSpace(headerParts[0]))
	for _, part := range headerParts[1:] {
		if strings.EqualFold(strings.TrimSpace(part), "base64") {
			isBase64 = true
			break
		}
	}
	return mimeType, dataPart, isBase64, nil
}

func shouldCacheReferenceVideoSource(source string) bool {
	lowerSource := strings.ToLower(strings.TrimSpace(source))
	return strings.HasPrefix(lowerSource, "http://") ||
		strings.HasPrefix(lowerSource, "https://") ||
		strings.HasPrefix(lowerSource, "data:video/")
}

func referenceVideoSourceCacheKey(source string) string {
	source = strings.TrimSpace(source)
	if source == "" || !shouldCacheReferenceVideoSource(source) {
		return ""
	}
	return common.Sha1([]byte(source))
}

func buildReferenceVideoDurationDetail(index int, source string) ReferenceVideoDurationDetail {
	detail := ReferenceVideoDurationDetail{
		Index:      index,
		SourceHash: referenceVideoSourceCacheKey(source),
	}
	lowerSource := strings.ToLower(strings.TrimSpace(source))
	switch {
	case strings.HasPrefix(lowerSource, "http://") || strings.HasPrefix(lowerSource, "https://"):
		detail.SourceType = "url"
		if parsed, err := url.Parse(source); err == nil {
			detail.SourceHost = parsed.Hostname()
		}
	case strings.HasPrefix(lowerSource, "data:video/"):
		detail.SourceType = "data_uri"
	default:
		detail.SourceType = "unknown"
	}
	return detail
}

func (d ReferenceVideoDurationDetail) ToMap() map[string]interface{} {
	result := make(map[string]interface{})
	if d.Index > 0 {
		result["index"] = d.Index
	}
	if d.SourceType != "" {
		result["source_type"] = d.SourceType
	}
	if d.SourceHost != "" {
		result["source_host"] = d.SourceHost
	}
	if d.SourceHash != "" {
		result["source_hash"] = d.SourceHash
	}
	if d.Duration > 0 {
		result["duration"] = d.Duration
	}
	if d.ProbeMethod != "" {
		result["probe_method"] = d.ProbeMethod
	}
	if d.Status != "" {
		result["status"] = d.Status
	}
	if d.ErrorCode != "" {
		result["error_code"] = d.ErrorCode
	}
	if d.Error != "" {
		result["error"] = d.Error
	}
	return result
}

func (s *ReferenceVideoDurationSummary) DetailMaps() []map[string]interface{} {
	if s == nil || len(s.Details) == 0 {
		return nil
	}
	result := make([]map[string]interface{}, 0, len(s.Details))
	for _, detail := range s.Details {
		result = append(result, detail.ToMap())
	}
	return result
}

func getCachedReferenceVideoDuration(source string) (float64, bool) {
	key := referenceVideoSourceCacheKey(source)
	if key == "" {
		return 0, false
	}
	entry, found, err := getReferenceVideoDurationCache().Get(key)
	if err != nil || !found || entry.Duration <= 0 {
		return 0, false
	}
	return roundReferenceVideoSeconds(entry.Duration), true
}

func cacheReferenceVideoDuration(source string, duration float64) {
	if duration <= 0 {
		return
	}
	duration = roundReferenceVideoSeconds(duration)
	key := referenceVideoSourceCacheKey(source)
	if key == "" {
		return
	}
	if err := getReferenceVideoDurationCache().SetWithTTL(key, referenceVideoDurationCacheEntry{
		Duration: duration,
	}, referenceVideoDurationCacheTTL()); err != nil {
		common.SysLog(fmt.Sprintf("cache reference video duration failed: %v", err))
	}
}

func tempFileSuffix(ext string) string {
	ext = strings.ToLower(strings.TrimSpace(ext))
	switch ext {
	case ".mp4", ".mov", ".m4v", ".webm":
		return ext
	default:
		return ".bin"
	}
}

func probeReferenceVideoTempFile(ctx context.Context, tmpPath string, tmpFile *os.File, ext string) (*referenceVideoProbeResult, error) {
	if duration, err := probeReferenceVideoDurationByFFProbe(ctx, tmpPath); err == nil && duration > 0 {
		return &referenceVideoProbeResult{
			Duration:    duration,
			ProbeMethod: "ffprobe",
		}, nil
	}

	sniffedExt, err := detectReferenceVideoExt(tmpFile, ext)
	if err != nil {
		return nil, err
	}
	if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	duration, err := common.GetVideoDuration(ctx, tmpFile, sniffedExt)
	if err != nil {
		return nil, err
	}
	if duration <= 0 {
		return nil, fmt.Errorf("invalid reference video duration: %f", duration)
	}
	return &referenceVideoProbeResult{
		Duration:    duration,
		ProbeMethod: "native",
	}, nil
}

func probeReferenceVideoDurationByFFProbe(ctx context.Context, filePath string) (float64, error) {
	ffprobePath, err := lookupReferenceVideoFFProbe()
	if err != nil {
		return 0, err
	}

	probeCtx := ctx
	if probeCtx == nil {
		probeCtx = context.Background()
	}
	cmd := exec.CommandContext(
		probeCtx,
		ffprobePath,
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		filePath,
	)

	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	duration, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil {
		return 0, err
	}
	if duration <= 0 {
		return 0, fmt.Errorf("ffprobe returned invalid duration: %f", duration)
	}
	return duration, nil
}

func safeReferenceVideoSourceRef(source string) string {
	detail := buildReferenceVideoDurationDetail(0, source)
	ref := fmt.Sprintf("type=%s", detail.SourceType)
	if detail.SourceHost != "" {
		ref += ",host=" + detail.SourceHost
	}
	if detail.SourceHash != "" {
		hash := detail.SourceHash
		if len(hash) > 12 {
			hash = hash[:12]
		}
		ref += ",hash=" + hash
	}
	return ref
}

func sanitizeReferenceVideoProbeError(source string, err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	source = strings.TrimSpace(source)
	if source != "" {
		msg = strings.ReplaceAll(msg, source, safeReferenceVideoSourceRef(source))
	}
	if len(msg) > 240 {
		msg = msg[:240]
	}
	return msg
}

func classifyReferenceVideoProbeError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "deadline exceeded"), strings.Contains(msg, "timeout"):
		return "timeout"
	case strings.Contains(msg, "status code"):
		return "http_status"
	case strings.Contains(msg, "unsupported"), strings.Contains(msg, "invalid data uri"):
		return "unsupported_format"
	case strings.Contains(msg, "exceeds"):
		return "file_too_large"
	case strings.Contains(msg, "private"), strings.Contains(msg, "ssrf"), strings.Contains(msg, "not allowed"):
		return "blocked"
	default:
		return "probe_failed"
	}
}

func detectReferenceVideoExt(tmpFile *os.File, fallbackExt string) (string, error) {
	fallbackExt = strings.ToLower(strings.TrimSpace(fallbackExt))
	if isSupportedReferenceVideoExt(fallbackExt) {
		return fallbackExt, nil
	}

	if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
		return "", err
	}

	header := make([]byte, 512)
	n, err := tmpFile.Read(header)
	if err != nil && err != io.EOF {
		return "", err
	}
	header = header[:n]

	if len(header) >= 12 && bytes.Equal(header[4:8], []byte("ftyp")) {
		brand := string(header[8:12])
		if brand == "qt  " {
			return ".mov", nil
		}
		return ".mp4", nil
	}
	if len(header) >= 4 && bytes.Equal(header[:4], []byte{0x1A, 0x45, 0xDF, 0xA3}) {
		return ".webm", nil
	}

	if sniffedExt := referenceVideoExtFromMime(http.DetectContentType(header)); sniffedExt != "" {
		return sniffedExt, nil
	}
	return "", fmt.Errorf("unsupported reference video format")
}

func isSupportedReferenceVideoExt(ext string) bool {
	switch strings.ToLower(strings.TrimSpace(ext)) {
	case ".mp4", ".mov", ".m4v", ".webm":
		return true
	default:
		return false
	}
}

func referenceVideoExtFromURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	ext := strings.ToLower(path.Ext(parsed.Path))
	switch ext {
	case ".mp4", ".mov", ".m4v", ".webm":
		return ext
	default:
		return ""
	}
}

func referenceVideoExtFromMime(contentType string) string {
	mediaType := strings.ToLower(strings.TrimSpace(contentType))
	if parsedType, _, err := mime.ParseMediaType(mediaType); err == nil {
		mediaType = parsedType
	}

	switch mediaType {
	case "video/mp4", "application/mp4":
		return ".mp4"
	case "video/quicktime":
		return ".mov"
	case "video/x-m4v":
		return ".m4v"
	case "video/webm":
		return ".webm"
	default:
		return ""
	}
}
