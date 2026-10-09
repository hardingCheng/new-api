package openai

import "net/http"

// upstreamErrorStatusCode maps the HTTP status of an upstream response whose
// body is an error object to the status the hub should carry for it.
//
// Some upstream gateways answer a failed generation with HTTP 200 and an
// {"error":{...}} body. Keeping the 2xx on that error would tell the client
// the request succeeded, and the retry policy, which never retries a 2xx,
// would not try the next channel. The upstream failed to produce a response,
// so from the hub's point of view that is a bad gateway. Any non-2xx status
// is passed through unchanged.
func upstreamErrorStatusCode(statusCode int) int {
	if statusCode >= 200 && statusCode < 300 {
		return http.StatusBadGateway
	}
	return statusCode
}
