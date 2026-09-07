package tokenmart

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertToRequestPayloadDefaults(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:  "seedance-2.0-mini-480p",
		Prompt: "a red fox",
	}
	payload, err := convertToRequestPayload(&req)
	require.NoError(t, err)

	assert.Equal(t, defaultDurationSeconds, payload.Duration)
	assert.Equal(t, "480p", payload.Resolution)
	assert.Equal(t, defaultRatio, payload.Ratio)
	require.Len(t, payload.Content, 1)
	assert.Equal(t, "text", payload.Content[0].Type)
	assert.Equal(t, "a red fox", payload.Content[0].Text)
}

// 画幅走对外的 aspect_ratio，并且与参考素材的比例无关（上游按 ratio 出片）。
func TestConvertToRequestPayloadAspectRatio(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:       "seedance-2.0-mini-480p",
		Prompt:      "a red fox",
		AspectRatio: "9:16",
	}
	payload, err := convertToRequestPayload(&req)
	require.NoError(t, err)
	assert.Equal(t, "9:16", payload.Ratio)

	// 直接传 ratio 的调用方也要兼容
	req = relaycommon.TaskSubmitReq{Model: "seedance-2.0-mini-480p", Prompt: "x", Ratio: "1:1"}
	payload, err = convertToRequestPayload(&req)
	require.NoError(t, err)
	assert.Equal(t, "1:1", payload.Ratio)
}

// 多参考图逐张进 content，顺序保持；文本项固定在最后一项。
func TestConvertToRequestPayloadMultipleReferenceImages(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:       "seedance-2.0-mini-480p",
		Prompt:      "keep the same shopkeeper",
		Images:      []string{"https://example.com/a.png", "https://example.com/b.png", "https://example.com/c.png"},
		AspectRatio: "9:16",
	}
	payload, err := convertToRequestPayload(&req)
	require.NoError(t, err)

	require.Len(t, payload.Content, 4)
	for i, want := range []string{"https://example.com/a.png", "https://example.com/b.png", "https://example.com/c.png"} {
		assert.Equal(t, "image_url", payload.Content[i].Type)
		require.NotNil(t, payload.Content[i].ImageURL)
		assert.Equal(t, want, payload.Content[i].ImageURL.URL)
	}
	assert.Equal(t, "text", payload.Content[3].Type)
	assert.Equal(t, "keep the same shopkeeper", payload.Content[3].Text)
	assert.Equal(t, "9:16", payload.Ratio)
}

// 分辨率决定上游成本，只能由对客模型名的档位决定：
// 客户在 480p 档的模型上塞更高分辨率必须无效，否则成本会被抬到售价之上。
func TestConvertToRequestPayloadResolutionLockedByModelName(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:    "seedance-2.0-mini-480p",
		Prompt:   "a red fox",
		Metadata: map[string]interface{}{"resolution": "1080p"},
	}
	payload, err := convertToRequestPayload(&req)
	require.NoError(t, err)
	assert.Equal(t, "480p", payload.Resolution)

	// 720p 档的模型名则应取 720p
	req = relaycommon.TaskSubmitReq{Model: "seedance-2.0-mini-720p", Prompt: "x"}
	payload, err = convertToRequestPayload(&req)
	require.NoError(t, err)
	assert.Equal(t, "720p", payload.Resolution)
}

func TestResolutionFromModelName(t *testing.T) {
	cases := map[string]string{
		"seedance-2.0-mini-480p": "480p",
		"seedance-2.0-mini-720p": "720p",
		"seedance-2.0-1080p":     "1080p",
		"seedance-2.5-pro-4K":    "4k",
		"seedance-2.0-mini":      "",
		"":                       "",
	}
	for name, want := range cases {
		assert.Equal(t, want, resolutionFromModelName(name), name)
	}
}

func TestConvertToRequestPayloadDurationFromBody(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:    "seedance-2.0-mini-480p",
		Prompt:   "a red fox",
		Duration: 10,
	}
	payload, err := convertToRequestPayload(&req)
	require.NoError(t, err)
	assert.Equal(t, 10, payload.Duration)
}

func TestParseTaskResultStatusMapping(t *testing.T) {
	adaptor := &TaskAdaptor{}

	queued, err := adaptor.ParseTaskResult([]byte(`{"task":{"id":"mvt-1","status":"pending","outputs":[],"error":null}}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusQueued, queued.Status)

	running, err := adaptor.ParseTaskResult([]byte(`{"task":{"id":"mvt-1","status":"processing","outputs":[],"error":null}}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusInProgress, running.Status)

	done, err := adaptor.ParseTaskResult([]byte(`{"task":{"id":"mvt-1","status":"completed","outputs":["https://cdn.example.com/out.mp4"],"error":null}}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusSuccess, done.Status)
	assert.Equal(t, "https://cdn.example.com/out.mp4", done.Url)

	// 失败原因两种形状都要能取出来
	failedObject, err := adaptor.ParseTaskResult([]byte(`{"task":{"id":"mvt-1","status":"failed","outputs":[],"error":{"code":"InvalidParameter","message":"ratio is not valid"}}}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusFailure, failedObject.Status)
	assert.Equal(t, "InvalidParameter: ratio is not valid", failedObject.Reason)

	failedString, err := adaptor.ParseTaskResult([]byte(`{"task":{"id":"mvt-1","status":"failed","outputs":[],"error":"moderation blocked"}}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusFailure, failedString.Status)
	assert.Equal(t, "moderation blocked", failedString.Reason)

	// 失败但上游没给原因时，至少回落到状态字面量，不能是空
	failedBare, err := adaptor.ParseTaskResult([]byte(`{"task":{"id":"mvt-1","status":"failed","outputs":[],"error":null}}`))
	require.NoError(t, err)
	assert.Equal(t, "failed", failedBare.Reason)
}
