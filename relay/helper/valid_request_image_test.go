package helper

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/dto"
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
	wantPrompt := "draw\n\n" + dto.ImageQualityInstruction
	if imageReq.Prompt != wantPrompt {
		t.Fatalf("Prompt = %q, want %q", imageReq.Prompt, wantPrompt)
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
	wantPrompt := "draw\n\n" + dto.ImageQualityInstruction
	if imageReq.Prompt != wantPrompt {
		t.Fatalf("Prompt = %q, want %q", imageReq.Prompt, wantPrompt)
	}
}

func TestGetAndValidOpenAIImageRequestAddsQualityInstructionForNonGPTImage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := bytes.NewBufferString(`{"model":"dall-e-3","prompt":"draw"}`)
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
	wantPrompt := "draw\n\n" + dto.ImageQualityInstruction
	if imageReq.Prompt != wantPrompt {
		t.Fatalf("Prompt = %q, want %q", imageReq.Prompt, wantPrompt)
	}
}

func TestGetAndValidOpenAIImageRequestLeavesEmptyPromptEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := bytes.NewBufferString(`{"model":"gpt-image-2"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", body)
	req.Header.Set("Content-Type", "application/json")

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	imageReq, err := GetAndValidOpenAIImageRequest(c, constant.RelayModeImagesGenerations)
	if err != nil {
		t.Fatalf("GetAndValidOpenAIImageRequest returned error: %v", err)
	}
	if imageReq.Prompt != "" {
		t.Fatalf("Prompt = %q, want empty", imageReq.Prompt)
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
	wantPrompt := "edit\n\n" + dto.ImageQualityInstruction
	if imageReq.Prompt != wantPrompt {
		t.Fatalf("Prompt = %q, want %q", imageReq.Prompt, wantPrompt)
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
	wantPrompt := "edit\n\n" + dto.ImageQualityInstruction
	if imageReq.Prompt != wantPrompt {
		t.Fatalf("Prompt = %q, want %q", imageReq.Prompt, wantPrompt)
	}
}

func TestGetAndValidOpenAIImageRequestDoesNotDuplicateQualityInstruction(t *testing.T) {
	gin.SetMode(gin.TestMode)

	prompt := "draw\n\n" + dto.ImageQualityInstruction
	body := bytes.NewBufferString(`{"model":"gpt-image-2","prompt":` + strconv.Quote(prompt) + `}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", body)
	req.Header.Set("Content-Type", "application/json")

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	imageReq, err := GetAndValidOpenAIImageRequest(c, constant.RelayModeImagesGenerations)
	if err != nil {
		t.Fatalf("GetAndValidOpenAIImageRequest returned error: %v", err)
	}
	if imageReq.Prompt != prompt {
		t.Fatalf("Prompt = %q, want %q", imageReq.Prompt, prompt)
	}
}
