package taskcommon

import (
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSummarizeReferenceVideoDurations(t *testing.T) {
	restore := referenceVideoDurationProbe
	referenceVideoDurationProbe = func(_ context.Context, source string) (*referenceVideoProbeResult, error) {
		switch source {
		case "mock://video-a":
			return &referenceVideoProbeResult{Duration: 3, ProbeMethod: "mock"}, nil
		case "mock://video-b":
			return &referenceVideoProbeResult{Duration: 5, ProbeMethod: "mock"}, nil
		default:
			return nil, errors.New("unexpected source")
		}
	}
	t.Cleanup(func() {
		referenceVideoDurationProbe = restore
	})

	c := newReferenceVideoTestContext(`{
		"prompt":"test",
		"model":"seedance-2.0",
		"duration":10,
		"content":[
			{"type":"text","text":"ignore"},
			{"type":"image_url","image_url":{"url":"https://example.com/image.png"}},
			{"type":"video_url","role":"reference_video","video_url":{"url":"mock://video-a"}},
			{"type":"video_url","video_url":{"url":"mock://video-b"}}
		]
	}`)

	summary, err := SummarizeReferenceVideoDurations(c)
	require.NoError(t, err)
	require.NotNil(t, summary)
	require.Equal(t, 2, summary.DetectedCount)
	require.Equal(t, 2, summary.ProbedCount)
	require.InDelta(t, 8.0, summary.TotalSeconds, 0.0001)
	require.Len(t, summary.Details, 2)
	require.Equal(t, "success", summary.Details[0].Status)
	require.Equal(t, "mock", summary.Details[0].ProbeMethod)
}

func TestBuildSeedanceReferenceVideoBillingRatiosLegacyMode(t *testing.T) {
	restore := referenceVideoDurationProbe
	referenceVideoDurationProbe = func(_ context.Context, source string) (*referenceVideoProbeResult, error) {
		switch source {
		case "mock://video-a":
			return &referenceVideoProbeResult{Duration: 4, ProbeMethod: "mock"}, nil
		case "mock://video-b":
			return &referenceVideoProbeResult{Duration: 6, ProbeMethod: "mock"}, nil
		default:
			return nil, errors.New("unexpected source")
		}
	}
	t.Cleanup(func() {
		referenceVideoDurationProbe = restore
	})
	originalMode := operation_setting.GetTaskBillingSetting().SeedanceReferenceVideoMode
	operation_setting.GetTaskBillingSetting().SeedanceReferenceVideoMode = operation_setting.SeedanceReferenceVideoBillingModeLegacy
	t.Cleanup(func() {
		operation_setting.GetTaskBillingSetting().SeedanceReferenceVideoMode = originalMode
	})

	c := newReferenceVideoTestContext(`{
		"prompt":"test",
		"model":"seedance-2.0",
		"duration":10,
		"content":[
			{"type":"video_url","video_url":{"url":"mock://video-a"}},
			{"type":"video_url","video_url":{"url":"mock://video-b"}}
		]
	}`)

	ratios := BuildSeedanceReferenceVideoBillingRatios(c, 10)
	require.Equal(t, map[string]float64{
		"seconds":         10,
		"reference_video": 2,
	}, ratios)
	require.Equal(t, operation_setting.SeedanceReferenceVideoBillingModeLegacy, GetCachedSeedanceReferenceVideoBillingMode(c))
}

func TestBuildSeedanceReferenceVideoBillingRatiosDurationMode(t *testing.T) {
	restore := referenceVideoDurationProbe
	referenceVideoDurationProbe = func(_ context.Context, source string) (*referenceVideoProbeResult, error) {
		switch source {
		case "mock://video-a":
			return &referenceVideoProbeResult{Duration: 4, ProbeMethod: "mock"}, nil
		case "mock://video-b":
			return &referenceVideoProbeResult{Duration: 6, ProbeMethod: "mock"}, nil
		default:
			return nil, errors.New("unexpected source")
		}
	}
	t.Cleanup(func() {
		referenceVideoDurationProbe = restore
	})
	originalMode := operation_setting.GetTaskBillingSetting().SeedanceReferenceVideoMode
	operation_setting.GetTaskBillingSetting().SeedanceReferenceVideoMode = operation_setting.SeedanceReferenceVideoBillingModeDurationOnly
	t.Cleanup(func() {
		operation_setting.GetTaskBillingSetting().SeedanceReferenceVideoMode = originalMode
	})

	c := newReferenceVideoTestContext(`{
		"prompt":"test",
		"model":"seedance-2.0",
		"duration":10,
		"content":[
			{"type":"video_url","video_url":{"url":"mock://video-a"}},
			{"type":"video_url","video_url":{"url":"mock://video-b"}}
		]
	}`)

	ratios := BuildSeedanceReferenceVideoBillingRatios(c, 10)
	require.Equal(t, map[string]float64{
		"seconds":         20,
		"reference_video": 1,
	}, ratios)
	require.Equal(t, operation_setting.SeedanceReferenceVideoBillingModeDurationOnly, GetCachedSeedanceReferenceVideoBillingMode(c))
}

func TestBuildSeedanceReferenceVideoBillingRatiosDurationModeProbeFailure(t *testing.T) {
	restore := referenceVideoDurationProbe
	referenceVideoDurationProbe = func(_ context.Context, source string) (*referenceVideoProbeResult, error) {
		if source == "mock://video-a" {
			return &referenceVideoProbeResult{Duration: 15, ProbeMethod: "mock"}, nil
		}
		return nil, errors.New("probe failed")
	}
	t.Cleanup(func() {
		referenceVideoDurationProbe = restore
	})
	originalMode := operation_setting.GetTaskBillingSetting().SeedanceReferenceVideoMode
	operation_setting.GetTaskBillingSetting().SeedanceReferenceVideoMode = operation_setting.SeedanceReferenceVideoBillingModeDurationOnly
	t.Cleanup(func() {
		operation_setting.GetTaskBillingSetting().SeedanceReferenceVideoMode = originalMode
	})

	c := newReferenceVideoTestContext(`{
		"prompt":"test",
		"model":"seedance-2.0",
		"duration":10,
		"content":[
			{"type":"video_url","video_url":{"url":"mock://video-a"}},
			{"type":"video_url","video_url":{"url":"mock://video-b"}}
		]
	}`)

	ratios := BuildSeedanceReferenceVideoBillingRatios(c, 10)
	require.Equal(t, map[string]float64{
		"seconds":         10,
		"reference_video": 2,
	}, ratios)
	require.Equal(t, operation_setting.SeedanceReferenceVideoBillingModeLegacy, GetCachedSeedanceReferenceVideoBillingMode(c))
	summary := GetCachedReferenceVideoDurationSummary(c)
	require.NotNil(t, summary)
	require.Equal(t, 1, summary.FailedCount)
	require.Len(t, summary.Details, 2)
	require.Equal(t, "failed", summary.Details[1].Status)
	require.Equal(t, "probe_failed", summary.Details[1].ErrorCode)
}

func TestReferenceVideoDurationDetailMaps(t *testing.T) {
	summary := &ReferenceVideoDurationSummary{
		Details: []ReferenceVideoDurationDetail{
			{
				Index:       1,
				SourceType:  "url",
				SourceHost:  "example.com",
				SourceHash:  "abc123",
				Duration:    4,
				ProbeMethod: "ffprobe",
				Status:      "success",
			},
		},
	}

	maps := summary.DetailMaps()
	require.Len(t, maps, 1)
	require.Equal(t, "example.com", maps[0]["source_host"])
	require.Equal(t, "ffprobe", maps[0]["probe_method"])
}

func TestReferenceVideoSourceCacheKeySkipsUnsupportedScheme(t *testing.T) {
	require.Empty(t, referenceVideoSourceCacheKey("mock://video-a"))
}

func TestDetectReferenceVideoExtFromHeader(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "reference-video-header-*.bin")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	_, err = tmpFile.Write([]byte{
		0x00, 0x00, 0x00, 0x18,
		'f', 't', 'y', 'p',
		'i', 's', 'o', 'm',
		0x00, 0x00, 0x00, 0x00,
	})
	require.NoError(t, err)

	ext, err := detectReferenceVideoExt(tmpFile, "")
	require.NoError(t, err)
	require.Equal(t, ".mp4", ext)
}

func newReferenceVideoTestContext(body string) *gin.Context {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest("POST", "/v1/videos", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	return c
}
