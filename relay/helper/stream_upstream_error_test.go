package helper

import (
	"strings"
	"testing"
)

func TestDetectUpstreamStreamError(t *testing.T) {
	cases := []struct {
		name     string
		data     string
		wantOK   bool
		contains []string
	}{
		{
			name:     "OpenAI Responses 顶层 type=error（Dings 客诉的那句）",
			data:     `{"type":"error","code":"server_is_overloaded","message":"Our servers are currently overloaded. Please try again later.","sequence_number":7}`,
			wantOK:   true,
			contains: []string{"code=server_is_overloaded", "Our servers are currently overloaded"},
		},
		{
			name:     "OpenAI Chat 嵌套 error 对象",
			data:     `{"error":{"type":"service_unavailable_error","code":"server_is_overloaded","message":"Our servers are currently overloaded. Please try again later."}}`,
			wantOK:   true,
			contains: []string{"type=service_unavailable_error", "code=server_is_overloaded"},
		},
		{
			name:     "Claude Messages overloaded_error",
			data:     `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`,
			wantOK:   true,
			contains: []string{"type=overloaded_error", "message=Overloaded"},
		},
		{
			name:     "error 对象只有 message",
			data:     `{"error":{"message":"upstream boom"}}`,
			wantOK:   true,
			contains: []string{"message=upstream boom"},
		},
		{
			name:   "正常 delta 分片不误报",
			data:   `{"type":"response.output_text.delta","delta":"hello"}`,
			wantOK: false,
		},
		{
			name:   "正文里出现 error 字样但不是错误事件",
			data:   `{"type":"response.output_text.delta","delta":"the \"error\" was handled"}`,
			wantOK: false,
		},
		{
			name:   "error 是 null 不误报",
			data:   `{"id":"resp_1","error":null,"status":"completed"}`,
			wantOK: false,
		},
		{
			name:   "非 JSON 不 panic 不误报",
			data:   `{"error": broken`,
			wantOK: false,
		},
		{
			name:   "空串",
			data:   ``,
			wantOK: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := detectUpstreamStreamError(tc.data)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (got msg %q)", ok, tc.wantOK, got)
			}
			if !tc.wantOK {
				return
			}
			if !strings.HasPrefix(got, "upstream stream error") {
				t.Errorf("摘要缺少前缀: %q", got)
			}
			for _, want := range tc.contains {
				if !strings.Contains(got, want) {
					t.Errorf("摘要 %q 缺少 %q", got, want)
				}
			}
		})
	}
}

func TestDetectUpstreamStreamErrorTruncatesLongMessage(t *testing.T) {
	long := strings.Repeat("中", 500)
	got, ok := detectUpstreamStreamError(`{"error":{"message":"` + long + `"}}`)
	if !ok {
		t.Fatal("应识别为上游流内错误")
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("超长 message 应被截断并加省略号: %q", got[len(got)-20:])
	}
	// 截断按字符计：200 个「中」+ 省略号，不应出现乱码字节。
	if strings.Contains(got, "�") {
		t.Error("截断产生了无效 UTF-8")
	}
}
