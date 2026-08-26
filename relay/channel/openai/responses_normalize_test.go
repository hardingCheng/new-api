package openai

import (
	"strings"
	"testing"
)

func TestNormalizeResponsesCreatedChunk(t *testing.T) {
	// 上游实测形状：裸 Response 对象、无 type，导致事件名被写成空串
	bare := `{"id":"resp_1","object":"response","created_at":1787715854,"status":"in_progress","model":"kimi-k3","output":[],"output_text":""}`
	got, ok := normalizeResponsesCreatedChunk(bare)
	if !ok {
		t.Fatal("裸 Response 对象应被规范化")
	}
	if !strings.Contains(got, `"type":"response.created"`) {
		t.Fatalf("应补上 type=response.created，实际 %s", got)
	}
	if !strings.Contains(got, `"resp_1"`) || !strings.Contains(got, `"response":`) {
		t.Fatalf("原对象应被包进 response 字段，实际 %s", got)
	}

	t.Run("已有 type 的事件不动", func(t *testing.T) {
		if _, ok := normalizeResponsesCreatedChunk(`{"type":"response.output_text.delta","delta":"a"}`); ok {
			t.Fatal("带 type 的事件不该被改写")
		}
	})
	t.Run("非 response 对象不动", func(t *testing.T) {
		if _, ok := normalizeResponsesCreatedChunk(`{"object":"chat.completion","id":"x"}`); ok {
			t.Fatal("非 response 对象不该被改写")
		}
	})
	t.Run("非法 JSON 不动", func(t *testing.T) {
		if _, ok := normalizeResponsesCreatedChunk(`not json`); ok {
			t.Fatal("非法 JSON 不该被改写")
		}
	})
}
