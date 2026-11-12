package dto

import (
	"testing"

	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 客户反馈第 3 条:contents 与 parts 都传单个对象(官方允许)
func TestGeminiRequestAcceptsSingleObjectContentsAndParts(t *testing.T) {
	raw := []byte(`{"contents":{"role":"user","parts":{"text":"What is the weather in Beijing?"}}}`)

	var req GeminiChatRequest
	require.NoError(t, kitutil.Unmarshal(raw, &req))

	require.Len(t, req.Contents, 1)
	assert.Equal(t, "user", req.Contents[0].Role)
	require.Len(t, req.Contents[0].Parts, 1)
	assert.Equal(t, "What is the weather in Beijing?", req.Contents[0].Parts[0].Text)
}

// 客户反馈第 4 条:contents 单对象 + safety_settings 单对象 + 蛇形 file_data / generation_config
func TestGeminiRequestAcceptsSnakeCaseImageEditRequest(t *testing.T) {
	raw := []byte(`{
		"contents": {"role": "USER", "parts": [
			{"file_data": {"mime_type": "image/png", "file_uri": "https://example.test/a.png"}},
			{"text": "Convert this photo to black and white"}
		]},
		"generation_config": {"thinking_config": {}, "response_modalities": ["TEXT", "IMAGE"], "image_config": {"image_output_options": {}}},
		"safety_settings": {"method": "PROBABILITY", "category": "HARM_CATEGORY_DANGEROUS_CONTENT", "threshold": "BLOCK_MEDIUM_AND_ABOVE"}
	}`)

	var req GeminiChatRequest
	require.NoError(t, kitutil.Unmarshal(raw, &req))

	require.Len(t, req.Contents, 1)
	require.Len(t, req.Contents[0].Parts, 2)
	fd := req.Contents[0].Parts[0].FileData
	require.NotNil(t, fd)
	assert.Equal(t, "image/png", fd.MimeType)
	assert.Equal(t, "https://example.test/a.png", fd.FileUri)

	require.Len(t, req.SafetySettings, 1)
	assert.Equal(t, "HARM_CATEGORY_DANGEROUS_CONTENT", req.SafetySettings[0].Category)
	assert.Equal(t, "BLOCK_MEDIUM_AND_ABOVE", req.SafetySettings[0].Threshold)
	assert.Equal(t, "PROBABILITY", req.SafetySettings[0].Method)

	assert.Equal(t, []string{"TEXT", "IMAGE"}, req.GenerationConfig.ResponseModalities)
}

// 客户反馈第 2 条:蛇形 file_data 不能被丢掉(之前转发成空 part,上游报 oneof data 为空)
func TestGeminiRequestSnakeFileDataSurvivesReMarshal(t *testing.T) {
	raw := []byte(`{
		"contents": [{"role": "user", "parts": [
			{"text": "describe"},
			{"file_data": {"mime_type": "image/jpeg", "file_uri": "https://example.test/b.jpg"}}
		]}],
		"safety_settings": [{"category": "HARM_CATEGORY_DANGEROUS_CONTENT", "threshold": "BLOCK_MEDIUM_AND_ABOVE", "method": "PROBABILITY"}],
		"generation_config": {"response_modalities": ["TEXT"], "thinking_config": {"thinking_level": "HIGH"}}
	}`)

	var req GeminiChatRequest
	require.NoError(t, kitutil.Unmarshal(raw, &req))

	encoded, err := kitutil.Marshal(req)
	require.NoError(t, err)

	var out struct {
		Contents []struct {
			Parts []map[string]any `json:"parts"`
		} `json:"contents"`
		SafetySettings   []map[string]any `json:"safetySettings"`
		GenerationConfig map[string]any   `json:"generationConfig"`
	}
	require.NoError(t, kitutil.Unmarshal(encoded, &out))

	require.Len(t, out.Contents[0].Parts, 2)
	fileData, ok := out.Contents[0].Parts[1]["fileData"].(map[string]any)
	require.True(t, ok, "fileData 必须被保留: %s", string(encoded))
	assert.Equal(t, "https://example.test/b.jpg", fileData["fileUri"])
	assert.Equal(t, "image/jpeg", fileData["mimeType"])

	require.Len(t, out.SafetySettings, 1)
	assert.Equal(t, "PROBABILITY", out.SafetySettings[0]["method"])

	thinking, ok := out.GenerationConfig["thinkingConfig"].(map[string]any)
	require.True(t, ok, "generation_config 蛇形写法必须被读到: %s", string(encoded))
	assert.Equal(t, "HIGH", thinking["thinkingLevel"])
}

// 原有写法不受影响
func TestGeminiRequestCamelCaseArrayUnchanged(t *testing.T) {
	raw := []byte(`{
		"contents": [{"role": "user", "parts": [{"text": "hi"}, {"fileData": {"mimeType": "image/png", "fileUri": "gs://x/y.png"}}]}],
		"safetySettings": [{"category": "A", "threshold": "B"}],
		"generationConfig": {"maxOutputTokens": 32}
	}`)

	var req GeminiChatRequest
	require.NoError(t, kitutil.Unmarshal(raw, &req))

	require.Len(t, req.Contents, 1)
	require.Len(t, req.Contents[0].Parts, 2)
	assert.Equal(t, "gs://x/y.png", req.Contents[0].Parts[1].FileData.FileUri)
	require.Len(t, req.SafetySettings, 1)
	require.NotNil(t, req.GenerationConfig.MaxOutputTokens)
	assert.Equal(t, uint(32), *req.GenerationConfig.MaxOutputTokens)
}

func TestGeminiRequestRejectsInvalidContents(t *testing.T) {
	var req GeminiChatRequest
	assert.Error(t, kitutil.Unmarshal([]byte(`{"contents":"oops"}`), &req))
}
