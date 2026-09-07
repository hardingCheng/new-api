package helper

import (
	"strings"

	"github.com/tidwall/gjson"
)

// 上游在 SSE 流内塞错误事件时，HTTP 200 响应头早已发给客户端，既改不了状态码也无法换渠道重试。
// 若不在这里记一笔，整条请求会被记成成功，运维侧只能看到 completion_tokens 极小 + client_gone，
// 无从得知客户实际看到的是什么报错。已知的三种形状：
//
//	OpenAI Responses  data: {"type":"error","code":"server_is_overloaded","message":"Our servers are currently overloaded. Please try again later."}
//	OpenAI Chat       data: {"error":{"type":"service_unavailable_error","code":"server_is_overloaded","message":"..."}}
//	Claude Messages   data: {"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}
//
// detectUpstreamStreamError 只读不改，转发给客户端的字节不受影响；返回可直接落库的一行摘要。
func detectUpstreamStreamError(data string) (string, bool) {
	// 廉价前置过滤：绝大多数 delta 分片不含 error 字样，直接跳过 JSON 解析。
	if !strings.Contains(data, `"error"`) {
		return "", false
	}
	if !gjson.Valid(data) {
		return "", false
	}
	root := gjson.Parse(data)

	// 形状二、形状三：错误细节在 error 对象里。
	if e := root.Get("error"); e.IsObject() {
		return formatUpstreamStreamError(e.Get("type").String(), e.Get("code").String(), e.Get("message").String()), true
	}
	// 形状一：顶层 type=error，code / message 是兄弟字段。
	if root.Get("type").String() == "error" {
		return formatUpstreamStreamError("error", root.Get("code").String(), root.Get("message").String()), true
	}
	return "", false
}

// maxUpstreamStreamErrorMessage 限制单条 message 长度，避免上游把长文塞进日志 other 字段。
const maxUpstreamStreamErrorMessage = 200

func formatUpstreamStreamError(errType, code, message string) string {
	parts := make([]string, 0, 3)
	if errType != "" {
		parts = append(parts, "type="+errType)
	}
	if code != "" {
		parts = append(parts, "code="+code)
	}
	if message != "" {
		parts = append(parts, "message="+truncateRunes(message, maxUpstreamStreamErrorMessage))
	}
	if len(parts) == 0 {
		return "upstream stream error"
	}
	return "upstream stream error: " + strings.Join(parts, " ")
}

// truncateRunes 按字符而非字节截断，避免把多字节字符切成乱码。
func truncateRunes(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit]) + "…"
}
