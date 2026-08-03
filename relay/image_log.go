package relay

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relaykit/dto"

	"github.com/gin-gonic/gin"
)

const (
	imagePayloadLogLimit   = 32 * 1024
	imageStringProbeLimit  = 256
	imageBase64Placeholder = "base64 data"
	imageSecretPlaceholder = "sensitive data"
)

type imageJSONContext struct {
	kind               byte
	expectingKey       bool
	key                string
	pendingPlaceholder string
}

// imagePayloadCapture keeps diagnostic payload logs bounded while discarding
// image data as it streams through. It understands JSON embedded in SSE too.
type imagePayloadCapture struct {
	buf                bytes.Buffer
	limit              int
	truncated          bool
	stack              []imageJSONContext
	inString           bool
	stringIsKey        bool
	redactString       bool
	stringEscaped      bool
	probeString        bool
	stringProbe        []byte
	keyBuffer          []byte
	skipCompositeDepth int
	skipString         bool
	skipEscaped        bool
}

func newImagePayloadCapture() *imagePayloadCapture {
	return &imagePayloadCapture{limit: imagePayloadLogLimit}
}

func (capture *imagePayloadCapture) Write(data []byte) (int, error) {
	for _, ch := range data {
		capture.writeByte(ch)
	}
	return len(data), nil
}

func (capture *imagePayloadCapture) String() string {
	if capture.probeString && len(capture.stringProbe) > 0 {
		if isBase64Payload(capture.stringProbe) {
			capture.appendText(imageBase64Placeholder)
		} else {
			capture.appendBytes(capture.stringProbe)
		}
		capture.stringProbe = nil
		capture.probeString = false
	}

	result := capture.buf.String()
	if !capture.truncated {
		return result
	}
	const marker = "... [truncated]"
	if len(result)+len(marker) <= capture.limit {
		return result + marker
	}
	if capture.limit <= len(marker) {
		return marker[:capture.limit]
	}
	return result[:capture.limit-len(marker)] + marker
}

func (capture *imagePayloadCapture) writeByte(ch byte) {
	if capture.skipCompositeDepth > 0 {
		capture.skipCompositeByte(ch)
		return
	}

	if capture.inString {
		capture.writeStringByte(ch)
		return
	}

	if ch == '"' {
		capture.startString()
		return
	}

	if ch == '{' || ch == '[' {
		if placeholder := capture.takePendingPlaceholder(); placeholder != "" {
			capture.appendQuoted(placeholder)
			capture.skipCompositeDepth = 1
			return
		}
		capture.appendByte(ch)
		capture.stack = append(capture.stack, imageJSONContext{
			kind:         ch,
			expectingKey: ch == '{',
		})
		return
	}

	if ch == '}' || ch == ']' {
		capture.appendByte(ch)
		if len(capture.stack) > 0 {
			capture.stack = capture.stack[:len(capture.stack)-1]
		}
		return
	}

	if ch == ',' {
		capture.appendByte(ch)
		if current := capture.currentContext(); current != nil && current.kind == '{' {
			current.expectingKey = true
			current.key = ""
			current.pendingPlaceholder = ""
		}
		return
	}

	if ch == ':' {
		capture.appendByte(ch)
		if current := capture.currentContext(); current != nil && current.kind == '{' {
			current.expectingKey = false
			current.pendingPlaceholder = imageLogPlaceholderForKey(current.key)
		}
		return
	}

	if !isJSONWhitespace(ch) {
		capture.takePendingPlaceholder()
	}
	capture.appendByte(ch)
}

func (capture *imagePayloadCapture) startString() {
	current := capture.currentContext()
	capture.stringIsKey = current != nil && current.kind == '{' && current.expectingKey
	capture.inString = true
	capture.stringEscaped = false
	capture.redactString = false
	capture.probeString = false
	capture.keyBuffer = capture.keyBuffer[:0]
	capture.stringProbe = capture.stringProbe[:0]
	capture.appendByte('"')

	if capture.stringIsKey {
		return
	}
	if placeholder := capture.takePendingPlaceholder(); placeholder != "" {
		capture.redactString = true
		capture.appendText(placeholder)
		return
	}
	capture.probeString = true
}

func (capture *imagePayloadCapture) writeStringByte(ch byte) {
	if capture.redactString {
		if capture.stringEscaped {
			capture.stringEscaped = false
			return
		}
		if ch == '\\' {
			capture.stringEscaped = true
			return
		}
		if ch == '"' {
			capture.appendByte(ch)
			capture.finishString()
		}
		return
	}

	if capture.stringEscaped {
		capture.stringEscaped = false
		capture.appendStringContentByte(ch)
		return
	}
	if ch == '\\' {
		capture.stringEscaped = true
		capture.appendStringContentByte(ch)
		return
	}
	if ch == '"' {
		if capture.probeString {
			if isBase64Payload(capture.stringProbe) {
				capture.appendText(imageBase64Placeholder)
			} else {
				capture.appendBytes(capture.stringProbe)
			}
		}
		capture.appendByte(ch)
		capture.finishString()
		return
	}
	capture.appendStringContentByte(ch)
}

func (capture *imagePayloadCapture) appendStringContentByte(ch byte) {
	if capture.stringIsKey {
		capture.appendByte(ch)
		if len(capture.keyBuffer) < 256 {
			capture.keyBuffer = append(capture.keyBuffer, ch)
		}
		return
	}
	if !capture.probeString {
		capture.appendByte(ch)
		return
	}

	capture.stringProbe = append(capture.stringProbe, ch)
	if len(capture.stringProbe) < imageStringProbeLimit {
		return
	}
	if isBase64Payload(capture.stringProbe) {
		capture.appendText(imageBase64Placeholder)
		capture.stringProbe = nil
		capture.probeString = false
		capture.redactString = true
		return
	}
	capture.appendBytes(capture.stringProbe)
	capture.stringProbe = nil
	capture.probeString = false
}

func (capture *imagePayloadCapture) finishString() {
	if capture.stringIsKey {
		if current := capture.currentContext(); current != nil {
			current.key = string(capture.keyBuffer)
		}
	}
	capture.inString = false
	capture.stringIsKey = false
	capture.redactString = false
	capture.stringEscaped = false
	capture.probeString = false
	capture.stringProbe = nil
}

func (capture *imagePayloadCapture) skipCompositeByte(ch byte) {
	if capture.skipString {
		if capture.skipEscaped {
			capture.skipEscaped = false
			return
		}
		if ch == '\\' {
			capture.skipEscaped = true
			return
		}
		if ch == '"' {
			capture.skipString = false
		}
		return
	}
	if ch == '"' {
		capture.skipString = true
		return
	}
	if ch == '{' || ch == '[' {
		capture.skipCompositeDepth++
		return
	}
	if ch == '}' || ch == ']' {
		capture.skipCompositeDepth--
	}
}

func (capture *imagePayloadCapture) currentContext() *imageJSONContext {
	if len(capture.stack) == 0 {
		return nil
	}
	return &capture.stack[len(capture.stack)-1]
}

func (capture *imagePayloadCapture) takePendingPlaceholder() string {
	current := capture.currentContext()
	if current == nil || current.kind != '{' {
		return ""
	}
	placeholder := current.pendingPlaceholder
	current.pendingPlaceholder = ""
	return placeholder
}

func (capture *imagePayloadCapture) appendQuoted(value string) {
	capture.appendByte('"')
	capture.appendText(value)
	capture.appendByte('"')
}

func (capture *imagePayloadCapture) appendText(value string) {
	capture.appendBytes([]byte(value))
}

func (capture *imagePayloadCapture) appendBytes(value []byte) {
	for _, ch := range value {
		capture.appendByte(ch)
	}
}

func (capture *imagePayloadCapture) appendByte(ch byte) {
	if capture.buf.Len() >= capture.limit {
		capture.truncated = true
		return
	}
	capture.buf.WriteByte(ch)
}

func imageLogPlaceholderForKey(key string) string {
	normalized := strings.ToLower(strings.TrimSpace(key))
	compact := strings.NewReplacer("_", "", "-", "").Replace(normalized)
	if strings.Contains(compact, "base64") || compact == "b64json" || compact == "b64" {
		return imageBase64Placeholder
	}
	switch compact {
	case "authorization", "apikey", "accesstoken", "refreshtoken", "password", "secret":
		return imageSecretPlaceholder
	default:
		return ""
	}
}

func isBase64DataURI(value []byte) bool {
	lower := strings.ToLower(string(value))
	return strings.HasPrefix(lower, "data:") && strings.Contains(lower, ";base64,")
}

func isBase64Payload(value []byte) bool {
	if isBase64DataURI(value) {
		return true
	}
	if len(value) < 128 || len(value)%4 != 0 {
		return false
	}
	for _, ch := range value {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '+' || ch == '/' || ch == '=' {
			continue
		}
		return false
	}
	return true
}

func isJSONWhitespace(ch byte) bool {
	return ch == ' ' || ch == '\n' || ch == '\r' || ch == '\t'
}

type imageResponseLogWriter struct {
	gin.ResponseWriter
	capture *imagePayloadCapture
}

func (writer *imageResponseLogWriter) Write(data []byte) (int, error) {
	n, err := writer.ResponseWriter.Write(data)
	if n > 0 && writer.Status() >= http.StatusBadRequest {
		_, _ = writer.capture.Write(data[:n])
	}
	return n, err
}

func (writer *imageResponseLogWriter) WriteString(data string) (int, error) {
	n, err := writer.ResponseWriter.WriteString(data)
	if n > 0 && writer.Status() >= http.StatusBadRequest {
		_, _ = writer.capture.Write([]byte(data)[:n])
	}
	return n, err
}

// StartImageFailurePayloadLog records sanitized request and response payloads
// only when the final client-facing HTTP status is 4xx or 5xx.
func StartImageFailurePayloadLog(c *gin.Context) func(*dto.ImageRequest) {
	if c == nil || c.Writer == nil {
		return func(*dto.ImageRequest) {}
	}
	capture := newImagePayloadCapture()
	writer := &imageResponseLogWriter{ResponseWriter: c.Writer, capture: capture}
	c.Writer = writer
	return func(request *dto.ImageRequest) {
		if writer.Status() < http.StatusBadRequest {
			return
		}
		LogImageRequestPayload(c, request)
		payload := capture.String()
		if payload == "" {
			payload = "<empty>"
		}
		logger.LogInfo(c, fmt.Sprintf(
			"image response payload: user_id=%d channel_id=%d path=%s status=%d content_type=%q payload=%s",
			c.GetInt("id"), c.GetInt("channel_id"), imageRequestPath(c), writer.Status(), writer.Header().Get("Content-Type"), payload,
		))
	}
}

// LogImageRequestPayload records the original JSON body or a multipart summary.
// Multipart file bytes are never logged; only field name, filename, size and MIME type.
func LogImageRequestPayload(c *gin.Context, request *dto.ImageRequest) {
	if c == nil || c.Request == nil {
		return
	}
	payload, err := imageRequestLogPayload(c, request)
	if err != nil {
		logger.LogWarn(c, fmt.Sprintf("image request payload unavailable: path=%s error=%q", imageRequestPath(c), err.Error()))
		return
	}
	logger.LogInfo(c, fmt.Sprintf(
		"image request payload: user_id=%d path=%s content_type=%q payload=%s",
		c.GetInt("id"), imageRequestPath(c), c.Request.Header.Get("Content-Type"), payload,
	))
}

func imageRequestLogPayload(c *gin.Context, request *dto.ImageRequest) (string, error) {
	contentType := strings.ToLower(c.Request.Header.Get("Content-Type"))
	if strings.Contains(contentType, "multipart/form-data") {
		return imageMultipartRequestLogPayload(c)
	}

	storage, err := common.GetBodyStorage(c)
	if err == nil {
		if _, err = storage.Seek(0, io.SeekStart); err != nil {
			return "", err
		}
		capture := newImagePayloadCapture()
		_, copyErr := io.Copy(capture, storage)
		_, seekErr := storage.Seek(0, io.SeekStart)
		c.Request.Body = io.NopCloser(storage)
		if copyErr != nil {
			return "", copyErr
		}
		if seekErr != nil {
			return "", seekErr
		}
		return capture.String(), nil
	}

	if request == nil {
		return "", err
	}
	data, marshalErr := common.Marshal(request)
	if marshalErr != nil {
		return "", marshalErr
	}
	capture := newImagePayloadCapture()
	_, _ = capture.Write(data)
	return capture.String(), nil
}

func imageMultipartRequestLogPayload(c *gin.Context) (string, error) {
	form := c.Request.MultipartForm
	if form == nil {
		parsed, err := common.ParseMultipartFormReusable(c)
		if err != nil {
			return "", err
		}
		form = parsed
		c.Request.MultipartForm = form
	}

	payload := make(map[string]any, len(form.Value)+1)
	for key, values := range form.Value {
		loggedValues := make([]string, len(values))
		for i, value := range values {
			loggedValues[i] = imageLogStringValue(key, value)
		}
		if len(values) == 1 {
			payload[key] = loggedValues[0]
		} else {
			payload[key] = loggedValues
		}
	}

	files := make(map[string]any, len(form.File))
	for field, headers := range form.File {
		items := make([]map[string]any, 0, len(headers))
		for _, header := range headers {
			items = append(items, map[string]any{
				"filename":     header.Filename,
				"size":         header.Size,
				"content_type": header.Header.Get("Content-Type"),
			})
		}
		files[field] = items
	}
	if len(files) > 0 {
		payload["_files"] = files
	}

	data, err := common.Marshal(payload)
	if err != nil {
		return "", err
	}
	capture := newImagePayloadCapture()
	_, _ = capture.Write(data)
	return capture.String(), nil
}

func imageRequestPath(c *gin.Context) string {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return ""
	}
	return c.Request.URL.Path
}

func imageLogStringValue(key, value string) string {
	if placeholder := imageLogPlaceholderForKey(key); placeholder != "" {
		return placeholder
	}
	if isBase64Payload([]byte(value)) {
		return imageBase64Placeholder
	}
	return value
}

var _ gin.ResponseWriter = (*imageResponseLogWriter)(nil)
var _ http.ResponseWriter = (*imageResponseLogWriter)(nil)
