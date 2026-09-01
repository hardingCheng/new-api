package dmc

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertToRequestPayloadDefaults(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:  "MiniMax-H3",
		Prompt: "a red fox",
	}
	payload, err := convertToRequestPayload(&req)
	require.NoError(t, err)

	assert.Equal(t, defaultDurationSeconds, payload.Duration)
	assert.Equal(t, defaultResolution, payload.Resolution)
	assert.Equal(t, defaultRatio, payload.Ratio)
	require.Len(t, payload.Content, 1)
	assert.Equal(t, "text", payload.Content[0].Type)
	assert.Equal(t, "a red fox", payload.Content[0].Text)
}

func TestConvertToRequestPayloadDurationFromMetadata(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:  "MiniMax-H3",
		Prompt: "a red fox",
		Metadata: map[string]interface{}{
			"duration": 12,
			"ratio":    "9:16",
		},
	}
	payload, err := convertToRequestPayload(&req)
	require.NoError(t, err)

	assert.Equal(t, 12, payload.Duration)
	assert.Equal(t, "9:16", payload.Ratio)
}

func TestConvertToRequestPayloadBodyDurationWinsOverMetadata(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:    "MiniMax-H3",
		Prompt:   "a red fox",
		Duration: 8,
		Metadata: map[string]interface{}{"duration": 3},
	}
	payload, err := convertToRequestPayload(&req)
	require.NoError(t, err)

	assert.Equal(t, 8, payload.Duration)
}

// duration 是计费乘数，metadata 路径绕过标准 DTO 校验，越界值必须在这里暴露出来被上层拒绝。
func TestConvertToRequestPayloadOutOfRangeDurationSurfaces(t *testing.T) {
	for _, duration := range []int{-1, 16, 600} {
		req := relaycommon.TaskSubmitReq{
			Model:    "MiniMax-H3",
			Prompt:   "a red fox",
			Metadata: map[string]interface{}{"duration": duration},
		}
		payload, err := convertToRequestPayload(&req)
		require.NoError(t, err)
		outOfRange := payload.Duration < MinDurationSeconds || payload.Duration > MaxDurationSeconds
		assert.True(t, outOfRange, "duration %d should be flagged out of range, got payload duration %d", duration, payload.Duration)
	}
}

func TestConvertToRequestPayloadTextIsPromptOnly(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:  "MiniMax-H3",
		Prompt: "official prompt",
		Content: []relaycommon.TaskContentItem{
			{Type: "text", Text: "smuggled text"},
			{Type: "image_url", ImageURL: &relaycommon.TaskMediaURL{URL: "https://example.com/a.png"}, Role: "first_frame"},
		},
	}
	payload, err := convertToRequestPayload(&req)
	require.NoError(t, err)

	require.Len(t, payload.Content, 2)
	assert.Equal(t, "image_url", payload.Content[0].Type)
	assert.Equal(t, "first_frame", payload.Content[0].Role)
	assert.Equal(t, "text", payload.Content[1].Type)
	assert.Equal(t, "official prompt", payload.Content[1].Text)
}

func TestParseTaskResultStatusMapping(t *testing.T) {
	a := &TaskAdaptor{}

	cases := []struct {
		name         string
		body         string
		wantStatus   model.TaskStatus
		wantURL      string
		wantReason   string
		wantProgress string
	}{
		{
			name:       "queued",
			body:       `{"task":{"id":"task_1","status":"queued"}}`,
			wantStatus: model.TaskStatusQueued,
		},
		{
			name:         "running",
			body:         `{"task":{"id":"task_1","status":"running","progress":0.5}}`,
			wantStatus:   model.TaskStatusInProgress,
			wantProgress: "50%",
		},
		{
			// 上游 running 阶段 progress 可能已经是 1；写成 100% 会让任务被轮询器跳过
			name:         "running with progress 1 must not report 100%",
			body:         `{"task":{"id":"task_1","status":"running","progress":1}}`,
			wantStatus:   model.TaskStatusInProgress,
			wantProgress: "99%",
		},
		{
			name:       "succeeded",
			body:       `{"task":{"id":"task_1","status":"succeeded","progress":1,"content":{"url":"https://media.example.com/x.mp4"}}}`,
			wantStatus: model.TaskStatusSuccess,
			wantURL:    "https://media.example.com/x.mp4",
		},
		{
			name:       "failed with error",
			body:       `{"task":{"id":"task_1","status":"failed","error":{"code":"input_download_failed","message":"download media failed"}}}`,
			wantStatus: model.TaskStatusFailure,
			wantReason: "input_download_failed: download media failed",
		},
		{
			name:       "cancelled",
			body:       `{"task":{"id":"task_1","status":"cancelled"}}`,
			wantStatus: model.TaskStatusFailure,
			wantReason: "cancelled",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := a.ParseTaskResult([]byte(tc.body))
			require.NoError(t, err)
			assert.Equal(t, string(tc.wantStatus), string(got.Status))
			if tc.wantURL != "" {
				assert.Equal(t, tc.wantURL, got.Url)
			}
			if tc.wantReason != "" {
				assert.Equal(t, tc.wantReason, got.Reason)
			}
			if tc.wantProgress != "" {
				assert.Equal(t, tc.wantProgress, got.Progress)
			}
		})
	}
}
