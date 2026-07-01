package openai

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"

	"github.com/gin-gonic/gin"
)

func TestConvertImageEditMultipartWritesDefaultResponseFormat(t *testing.T) {
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
	if _, err := c.MultipartForm(); err != nil {
		t.Fatalf("failed to parse source multipart form: %v", err)
	}

	adaptor := &Adaptor{}
	converted, err := adaptor.ConvertImageRequest(c, &relaycommon.RelayInfo{RelayMode: constant.RelayModeImagesEdits}, dto.ImageRequest{
		Model:          "gpt-image-2",
		Prompt:         "edit",
		ResponseFormat: "b64_json",
	})
	if err != nil {
		t.Fatalf("ConvertImageRequest returned error: %v", err)
	}

	reader, ok := converted.(io.Reader)
	if !ok {
		t.Fatalf("converted request is %T, want io.Reader", converted)
	}
	outboundBody, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}

	_, params, err := mime.ParseMediaType(c.Request.Header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("failed to parse outbound content type: %v", err)
	}
	mr := multipart.NewReader(bytes.NewReader(outboundBody), params["boundary"])
	form, err := mr.ReadForm(1024)
	if err != nil {
		t.Fatalf("failed to parse outbound multipart body: %v", err)
	}
	if got := form.Value["response_format"]; len(got) != 1 || got[0] != "b64_json" {
		t.Fatalf("response_format = %#v, want [b64_json]", got)
	}
}

func TestConvertImageEditMultipartOmitsResponseFormatForGPTImage2Token(t *testing.T) {
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
	if _, err := c.MultipartForm(); err != nil {
		t.Fatalf("failed to parse source multipart form: %v", err)
	}

	adaptor := &Adaptor{}
	converted, err := adaptor.ConvertImageRequest(c, &relaycommon.RelayInfo{RelayMode: constant.RelayModeImagesEdits}, dto.ImageRequest{
		Model:          "gpt-image-2-token",
		Prompt:         "edit",
		ResponseFormat: "url",
	})
	if err != nil {
		t.Fatalf("ConvertImageRequest returned error: %v", err)
	}

	reader, ok := converted.(io.Reader)
	if !ok {
		t.Fatalf("converted request is %T, want io.Reader", converted)
	}
	outboundBody, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}

	_, params, err := mime.ParseMediaType(c.Request.Header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("failed to parse outbound content type: %v", err)
	}
	mr := multipart.NewReader(bytes.NewReader(outboundBody), params["boundary"])
	form, err := mr.ReadForm(1024)
	if err != nil {
		t.Fatalf("failed to parse outbound multipart body: %v", err)
	}
	if got := form.Value["response_format"]; len(got) != 0 {
		t.Fatalf("response_format = %#v, want omitted", got)
	}
}
