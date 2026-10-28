package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeArkVideoRequest(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		body    string
		want    map[string]any
		absent  []string
		wantErr string
	}{
		{
			name: "prompt comes from the content text item and ark-only fields are dropped",
			body: `{"model":"seedance-2.0-mini-480p","content":[{"type":"text","text":"一只猫在跑"},{"type":"image_url","image_url":{"url":"https://img.example/1.png"}}],
				"duration":5,"ratio":"16:9","callback_url":"https://hook.example/cb","service_tier":"flex","execution_expires_after":172800,"draft":true}`,
			want:   map[string]any{"prompt": "一只猫在跑", "model": "seedance-2.0-mini-480p", "duration": float64(5), "ratio": "16:9"},
			absent: []string{"callback_url", "service_tier", "execution_expires_after", "draft"},
		},
		{
			name: "bare model gets the resolution suffix and frames become duration",
			body: `{"model":"seedance-2.5","resolution":"720p","frames":121,"content":[{"type":"text","text":"海边"}]}`,
			want: map[string]any{"model": "seedance-2.5-720p", "duration": float64(5)},
		},
		{
			name: "model that already carries a resolution keeps its name",
			body: `{"model":"seedance-2.5-1080p","resolution":"720p","prompt":"海边"}`,
			want: map[string]any{"model": "seedance-2.5-1080p"},
		},
		{
			name: "legacy text commands are lifted into fields and stripped from the prompt",
			body: `{"model":"seedance-2.0-mini-480p","content":[{"type":"text","text":"城市夜景 --ratio 9:16 --duration 10 --camerafixed true"}]}`,
			want: map[string]any{"prompt": "城市夜景", "ratio": "9:16", "duration": float64(10), "camera_fixed": true},
		},
		{
			name: "explicit fields win over text commands",
			body: `{"model":"seedance-2.0-mini-480p","duration":4,"content":[{"type":"text","text":"城市夜景 --duration 10"}]}`,
			want: map[string]any{"prompt": "城市夜景", "duration": float64(4)},
		},
		{
			name:    "frames beyond the task duration bound are rejected",
			body:    `{"model":"seedance-2.5-720p","frames":9999999,"prompt":"海边"}`,
			wantErr: "frames must be an integer",
		},
		{
			name:    "missing prompt text is rejected",
			body:    `{"model":"seedance-2.5-720p","content":[{"type":"image_url","image_url":{"url":"https://img.example/1.png"}}]}`,
			wantErr: "content must contain a text item",
		},
		{
			name:    "missing model is rejected",
			body:    `{"prompt":"海边"}`,
			wantErr: "model is required",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			normalized, err := NormalizeArkVideoRequest([]byte(testCase.body))
			if testCase.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), testCase.wantErr)
				return
			}
			require.NoError(t, err)
			var got map[string]any
			require.NoError(t, common.Unmarshal(normalized, &got))
			for key, value := range testCase.want {
				assert.Equal(t, value, got[key], key)
			}
			for _, key := range testCase.absent {
				assert.NotContains(t, got, key)
			}
		})
	}
}

func TestArkVideoSubmitReplacesReusableBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	var parsed relaycommon.TaskSubmitReq
	engine.POST(ArkVideoTasksPath, ArkVideoSubmit(), func(c *gin.Context) {
		require.NoError(t, common.UnmarshalBodyReusable(c, &parsed))
		c.Status(http.StatusNoContent)
	})

	body := `{"model":"seedance-2.5","resolution":"480p","frames":241,"content":[{"type":"text","text":"海边日出 --ratio 16:9"},{"type":"image_url","image_url":{"url":"https://img.example/1.png"}}]}`
	request := httptest.NewRequest(http.MethodPost, ArkVideoTasksPath, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusNoContent, recorder.Code, recorder.Body.String())
	assert.Equal(t, "seedance-2.5-480p", parsed.Model)
	assert.Equal(t, "海边日出", parsed.Prompt)
	assert.Equal(t, 10, parsed.Duration)
	assert.Equal(t, "16:9", parsed.Ratio)
	require.Len(t, parsed.Content, 2)
	assert.Equal(t, "海边日出", parsed.Content[0].Text)
	require.NotNil(t, parsed.Content[1].ImageURL)
	assert.Equal(t, "https://img.example/1.png", parsed.Content[1].ImageURL.URL)
}

func TestArkVideoSubmitRejectsNonJSONWithArkError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST(ArkVideoTasksPath, ArkVideoSubmit(), func(c *gin.Context) { c.Status(http.StatusNoContent) })

	request := httptest.NewRequest(http.MethodPost, ArkVideoTasksPath, strings.NewReader("model=x"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	var response struct {
		Error struct {
			Code string `json:"code"`
			Type string `json:"type"`
		} `json:"error"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "InvalidParameter", response.Error.Code)
	assert.Equal(t, "BadRequest", response.Error.Type)
}
