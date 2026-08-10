package helper

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/relay/constant"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGetAndValidOpenAIImageRequestDefaultsGPTImageResponseFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := bytes.NewBufferString(`{"model":"gpt-image-2","prompt":"draw"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", body)
	req.Header.Set("Content-Type", "application/json")

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	imageReq, err := GetAndValidOpenAIImageRequest(c, constant.RelayModeImagesGenerations)
	if err != nil {
		t.Fatalf("GetAndValidOpenAIImageRequest returned error: %v", err)
	}
	if imageReq.ResponseFormat != "b64_json" {
		t.Fatalf("ResponseFormat = %q, want %q", imageReq.ResponseFormat, "b64_json")
	}
}

func TestGetAndValidOpenAIImageRequestPreservesExplicitResponseFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := bytes.NewBufferString(`{"model":"gpt-image-2-pro","prompt":"draw","response_format":"url"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", body)
	req.Header.Set("Content-Type", "application/json")

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	imageReq, err := GetAndValidOpenAIImageRequest(c, constant.RelayModeImagesGenerations)
	if err != nil {
		t.Fatalf("GetAndValidOpenAIImageRequest returned error: %v", err)
	}
	if imageReq.ResponseFormat != "url" {
		t.Fatalf("ResponseFormat = %q, want %q", imageReq.ResponseFormat, "url")
	}
}

func TestGetAndValidOpenAIImageRequestOmitsResponseFormatForGPTImage2Token(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := bytes.NewBufferString(`{"model":"gpt-image-2-token","prompt":"draw","response_format":"url"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", body)
	req.Header.Set("Content-Type", "application/json")

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	imageReq, err := GetAndValidOpenAIImageRequest(c, constant.RelayModeImagesGenerations)
	if err != nil {
		t.Fatalf("GetAndValidOpenAIImageRequest returned error: %v", err)
	}
	if imageReq.ResponseFormat != "" {
		t.Fatalf("ResponseFormat = %q, want empty", imageReq.ResponseFormat)
	}
}

func TestGetAndValidOpenAIImageEditMultipartDefaultsGPTImageResponseFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("model", "gpt-image-2"); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("prompt", "edit"); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("image", "input.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("fake image")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	imageReq, err := GetAndValidOpenAIImageRequest(c, constant.RelayModeImagesEdits)
	if err != nil {
		t.Fatalf("GetAndValidOpenAIImageRequest returned error: %v", err)
	}
	if imageReq.ResponseFormat != "b64_json" {
		t.Fatalf("ResponseFormat = %q, want %q", imageReq.ResponseFormat, "b64_json")
	}
	if imageReq.Size != "auto" {
		t.Fatalf("Size = %q, want %q", imageReq.Size, "auto")
	}
}

func TestGetAndValidOpenAIImageEditJSONDefaultsAndPreservesSize(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "missing size defaults to auto",
			body: `{"model":"gpt-image-1","prompt":"edit","image":"https://example.com/input.png"}`,
			want: "auto",
		},
		{
			name: "explicit size is preserved",
			body: `{"model":"gpt-image-1","prompt":"edit","size":"1024x1024","image":"https://example.com/input.png"}`,
			want: "1024x1024",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewBufferString(tt.body))
			request.Header.Set("Content-Type", "application/json")
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			context.Request = request

			imageReq, err := GetAndValidOpenAIImageRequest(context, constant.RelayModeImagesEdits)
			require.NoError(t, err)
			require.Equal(t, tt.want, imageReq.Size)
		})
	}
}

func TestGetAndValidOpenAIImageEditMultipartOmitsResponseFormatForGPTImage2Token(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("model", "gpt-image-2-token"); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("prompt", "edit"); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("response_format", "url"); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("image", "input.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("fake image")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	imageReq, err := GetAndValidOpenAIImageRequest(c, constant.RelayModeImagesEdits)
	if err != nil {
		t.Fatalf("GetAndValidOpenAIImageRequest returned error: %v", err)
	}
	if imageReq.ResponseFormat != "" {
		t.Fatalf("ResponseFormat = %q, want empty", imageReq.ResponseFormat)
	}
}

func TestGetAndValidOpenAIImageEditMultipartPreservesExplicitResponseFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("model", "gpt-image-2-pro"); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("prompt", "edit"); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("response_format", "url"); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("image", "input.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("fake image")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	imageReq, err := GetAndValidOpenAIImageRequest(c, constant.RelayModeImagesEdits)
	if err != nil {
		t.Fatalf("GetAndValidOpenAIImageRequest returned error: %v", err)
	}
	if imageReq.ResponseFormat != "url" {
		t.Fatalf("ResponseFormat = %q, want %q", imageReq.ResponseFormat, "url")
	}
}
