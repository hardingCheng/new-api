package helper

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/relay/constant"

	"github.com/gin-gonic/gin"
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

	body := bytes.NewBufferString(`{"model":"gpt-image-2","prompt":"draw","response_format":"url"}`)
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
}

func TestGetAndValidOpenAIImageEditMultipartPreservesExplicitResponseFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("model", "gpt-image-2"); err != nil {
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
