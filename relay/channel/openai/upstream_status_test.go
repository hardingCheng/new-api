package openai

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpstreamErrorStatusCode(t *testing.T) {
	for _, tc := range []struct{ in, want int }{
		{200, http.StatusBadGateway},
		{201, http.StatusBadGateway},
		{299, http.StatusBadGateway},
		{400, 400},
		{451, 451},
		{504, 504},
	} {
		assert.Equal(t, tc.want, upstreamErrorStatusCode(tc.in), "status %d", tc.in)
	}
}

func TestOpenaiImageHandlerTurns2xxErrorBodyIntoBadGateway(t *testing.T) {
	body := `{"error":{"message":"gpt-image-2 upstream request failed","type":"openai_error","code":"upstream_unavailable"}}`
	c, recorder, resp, info := newImageTestContext(t, body, "application/json", false)

	_, err := OpenaiImageHandler(c, info, resp)

	require.NotNil(t, err)
	assert.Equal(t, http.StatusBadGateway, err.StatusCode)
	assert.Equal(t, "upstream_unavailable", string(err.GetErrorCode()))
	assert.Equal(t, "gpt-image-2 upstream request failed", err.Error())
	assert.Empty(t, recorder.Body.String(), "nothing must be written to the client before the retry decision")
}

func TestOpenaiImageStreamHandlerTurns2xxErrorBodyIntoBadGateway(t *testing.T) {
	body := `{"error":{"message":"gpt-image-2 upstream request failed","type":"openai_error","code":"upstream_unavailable"}}`
	c, recorder, resp, info := newImageTestContext(t, body, "application/json", true)

	_, err := OpenaiImageStreamHandler(c, info, resp)

	require.NotNil(t, err)
	assert.Equal(t, http.StatusBadGateway, err.StatusCode)
	assert.Empty(t, recorder.Body.String())
}

func TestOpenaiImageHandlerKeepsNon2xxErrorStatus(t *testing.T) {
	body := `{"error":{"message":"非常抱歉，该提示可能违反了我们的内容政策。","type":"invalid_request_error","code":"moderation_blocked"}}`
	c, _, resp, info := newImageTestContext(t, body, "application/json", false)
	resp.StatusCode = http.StatusBadRequest

	_, err := OpenaiImageHandler(c, info, resp)

	require.NotNil(t, err)
	assert.Equal(t, http.StatusBadRequest, err.StatusCode)
}
