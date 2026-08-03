package relay

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImagePayloadCaptureRedactsImageDataAndKeepsDiagnosticFields(t *testing.T) {
	largeBase64 := strings.Repeat("QUJD", 80)
	payload := `{"created":123,"data":[{"b64_json":"` + largeBase64 + `","url":"https://example.com/image.png","revised_prompt":"draw a cat"}],"nested":{"image_base64":["first","second"]},"api_key":"secret-key"}`

	capture := newImagePayloadCapture()
	for _, chunk := range []string{payload[:91], payload[91:247], payload[247:]} {
		_, err := capture.Write([]byte(chunk))
		require.NoError(t, err)
	}
	logged := capture.String()

	assert.Contains(t, logged, `"created":123`)
	assert.Contains(t, logged, `"b64_json":"base64 data"`)
	assert.Contains(t, logged, `"image_base64":"base64 data"`)
	assert.Contains(t, logged, `"api_key":"sensitive data"`)
	assert.Contains(t, logged, `"revised_prompt":"draw a cat"`)
	assert.NotContains(t, logged, largeBase64)
	assert.NotContains(t, logged, `"first"`)
}

func TestImagePayloadCaptureRedactsDataURIAndUnlabelledBase64(t *testing.T) {
	largeBase64 := strings.Repeat("YWJj", 80)
	shortBase64 := strings.Repeat("QUJD", 40)
	payload := `{"image":"data:image/png;base64,` + largeBase64 + `","output":"` + largeBase64 + `","short_output":"` + shortBase64 + `","prompt":"keep this prompt"}`

	capture := newImagePayloadCapture()
	_, err := capture.Write([]byte(payload))
	require.NoError(t, err)
	logged := capture.String()

	assert.Equal(t, 3, strings.Count(logged, imageBase64Placeholder))
	assert.Contains(t, logged, `"prompt":"keep this prompt"`)
	assert.NotContains(t, logged, largeBase64)
	assert.NotContains(t, logged, shortBase64)
}

func TestImagePayloadCaptureSanitizesSSEAcrossWrites(t *testing.T) {
	base64Data := strings.Repeat("QUJD", 80)
	chunks := []string{
		"event: image_generation.partial_image\n",
		`data: {"type":"image_generation.partial_image","b64_json":"` + base64Data[:137],
		base64Data[137:] + `","partial_image_index":0}` + "\n\n",
		"data: [DONE]\n\n",
	}

	capture := newImagePayloadCapture()
	for _, chunk := range chunks {
		_, err := capture.Write([]byte(chunk))
		require.NoError(t, err)
	}
	logged := capture.String()

	assert.Contains(t, logged, `"b64_json":"base64 data"`)
	assert.Contains(t, logged, `"partial_image_index":0`)
	assert.Contains(t, logged, "data: [DONE]")
	assert.NotContains(t, logged, base64Data)
}

func TestImagePayloadCaptureKeepsErrorResponse(t *testing.T) {
	payload := `{"error":{"message":"size is required","type":"invalid_request_error","code":"missing_size"}}`
	capture := newImagePayloadCapture()
	_, err := capture.Write([]byte(payload))
	require.NoError(t, err)

	assert.Equal(t, payload, capture.String())
}

func TestImageMultipartRequestLogPayloadIncludesFieldsAndFileMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "gpt-image-2-pro"))
	require.NoError(t, writer.WriteField("prompt", "edit this image"))
	require.NoError(t, writer.WriteField("size", "1024x1024"))
	part, err := writer.CreateFormFile("image", "input.png")
	require.NoError(t, err)
	_, err = part.Write([]byte("private image bytes"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	payload, err := imageRequestLogPayload(c, &dto.ImageRequest{})
	require.NoError(t, err)
	defer common.CleanupBodyStorage(c)

	assert.Contains(t, payload, `"model":"gpt-image-2-pro"`)
	assert.Contains(t, payload, `"prompt":"edit this image"`)
	assert.Contains(t, payload, `"size":"1024x1024"`)
	assert.Contains(t, payload, `"filename":"input.png"`)
	assert.Contains(t, payload, `"size":19`)
	assert.NotContains(t, payload, "private image bytes")
}

func TestImageResponseLogWriterPreservesFailedResponseAndCapturesSanitizedPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	capture := newImagePayloadCapture()
	c.Writer = &imageResponseLogWriter{ResponseWriter: c.Writer, capture: capture}
	response := `{"data":[{"b64_json":"` + strings.Repeat("QUJD", 80) + `"}],"usage":{"total_tokens":7}}`

	c.Data(http.StatusBadRequest, "application/json", []byte(response))

	assert.Equal(t, response, recorder.Body.String())
	assert.Contains(t, capture.String(), `"b64_json":"base64 data"`)
	assert.Contains(t, capture.String(), `"total_tokens":7`)
	assert.NotContains(t, capture.String(), strings.Repeat("QUJD", 20))
}

func TestImageResponseLogWriterDoesNotCaptureSuccessfulResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	capture := newImagePayloadCapture()
	c.Writer = &imageResponseLogWriter{ResponseWriter: c.Writer, capture: capture}
	response := `{"data":[{"url":"https://example.com/image.png"}]}`

	c.Data(http.StatusOK, "application/json", []byte(response))

	assert.Equal(t, response, recorder.Body.String())
	assert.Empty(t, capture.String())
}

func TestImageFailurePayloadLogWritesOnlyForFailedRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var logs bytes.Buffer
	common.LogWriterMu.Lock()
	originalWriter := gin.DefaultWriter
	gin.DefaultWriter = &logs
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultWriter = originalWriter
		common.LogWriterMu.Unlock()
	})

	successContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	successContext.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"gpt-image-1","prompt":"successful request"}`))
	finishSuccessLog := StartImageFailurePayloadLog(successContext)
	successContext.JSON(http.StatusOK, gin.H{"data": []any{}})
	finishSuccessLog(&dto.ImageRequest{Model: "gpt-image-1", Prompt: "successful request"})
	assert.Empty(t, logs.String())

	failureContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	failureContext.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"gpt-image-1","prompt":"failed request"}`))
	finishFailureLog := StartImageFailurePayloadLog(failureContext)
	failureContext.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "size is required"}})
	finishFailureLog(&dto.ImageRequest{Model: "gpt-image-1", Prompt: "failed request"})

	assert.Contains(t, logs.String(), "image request payload:")
	assert.Contains(t, logs.String(), `"prompt":"failed request"`)
	assert.Contains(t, logs.String(), "image response payload:")
	assert.Contains(t, logs.String(), `"message":"size is required"`)
}
