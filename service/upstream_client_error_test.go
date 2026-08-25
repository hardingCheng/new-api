package service

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/types"
)

// 归正只能命中白名单里的那几类客户端错误；同为 invalid_request_error 的
// 「模型不存在」必须放行，让它继续跨渠道重试。
func TestNormalizeUpstreamClientError(t *testing.T) {
	common.SetUpstreamClientErrKeywords(common.DefaultUpstreamClientErrKeywords)

	const jsonMsg = "Response input messages must contain the word 'json' in some form to use 'text.format' of type 'json_object'."
	const modelMsg = "The model `gpt-5.6-sol` does not exist or you do not have access to it."

	cases := []struct {
		name       string
		enabled    bool
		oaiErr     types.OpenAIError
		inStatus   int
		wantStatus int
		wantSkip   bool
	}{
		{"开关关闭时完全不介入", false,
			types.OpenAIError{Type: "invalid_request_error", Message: jsonMsg}, 502, 502, false},
		{"上游包成502的json错误归正为400并跳过重试", true,
			types.OpenAIError{Type: "invalid_request_error", Message: jsonMsg}, 502, http.StatusBadRequest, true},
		{"上游本来就返回400的不改动", true,
			types.OpenAIError{Type: "invalid_request_error", Message: jsonMsg}, 400, 400, false},
		{"模型不存在必须放行继续重试", true,
			types.OpenAIError{Type: "invalid_request_error", Message: modelMsg}, 502, 502, false},
		{"非invalid_request_error不介入", true,
			types.OpenAIError{Type: "server_error", Message: jsonMsg}, 502, 502, false},
		{"真上游故障不介入", true,
			types.OpenAIError{Type: "api_error", Message: "Upstream request failed"}, 502, 502, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			common.SetUpstreamClientErrNormalize(tc.enabled)
			gotStatus, opts := normalizeUpstreamClientError(tc.oaiErr, tc.inStatus)
			if gotStatus != tc.wantStatus {
				t.Fatalf("状态码 = %d, 期望 %d", gotStatus, tc.wantStatus)
			}
			gotSkip := len(opts) > 0
			if gotSkip != tc.wantSkip {
				t.Fatalf("skipRetry = %v, 期望 %v", gotSkip, tc.wantSkip)
			}
			if gotSkip {
				// skipRetry 同时关掉重试与熔断，两处都认这个标记
				err := types.WithOpenAIError(tc.oaiErr, gotStatus, opts...)
				if !types.IsSkipRetryError(err) {
					t.Fatal("期望 IsSkipRetryError 为 true")
				}
			}
		})
	}

	// 白名单为空时退化为不介入，避免误伤
	t.Run("白名单为空时不介入", func(t *testing.T) {
		common.SetUpstreamClientErrNormalize(true)
		common.SetUpstreamClientErrKeywords("")
		defer common.SetUpstreamClientErrKeywords(common.DefaultUpstreamClientErrKeywords)
		gotStatus, opts := normalizeUpstreamClientError(
			types.OpenAIError{Type: "invalid_request_error", Message: jsonMsg}, 502)
		if gotStatus != 502 || len(opts) != 0 {
			t.Fatalf("期望不介入, 得到 status=%d opts=%d", gotStatus, len(opts))
		}
	})
}
