package openai

import (
	"github.com/QuantumNous/new-api/common"
)

// 对客 usage 规范化（ChannelSettings.NormalizeUsage，默认关，按渠道开启）：
// 上游各家 usage 形状不一——有的缺官方必有的缓存计量字段，有的把内部成本字段
// （estimated_cost 等）原样透传给客户。这里把客户可见的 usage 重建成官方形状：
// 顶层白名单保留官方字段，补齐 prompt_tokens_details.cached_tokens（billing 侧
// 已从各方言位置提取过真实值，上游确实没报时为 0，与实际计费口径一致——未计入
// 缓存的部分就是按未缓存收的费）。只改响应体，计费用的 usage 在此之前已定型。

var clientUsageAllowedKeys = map[string]bool{
	"prompt_tokens":             true,
	"completion_tokens":         true,
	"total_tokens":              true,
	"prompt_tokens_details":     true,
	"completion_tokens_details": true,
	// moonshot 方言的缓存命中计量，存量客户端有依赖，保留
	"prompt_cache_hit_tokens":  true,
	"prompt_cache_miss_tokens": true,
}

func normalizeClientUsage(rawUsage any, cachedTokens int) (map[string]any, bool) {
	var source map[string]any
	switch value := rawUsage.(type) {
	case map[string]any:
		source = value
	case nil:
		return nil, false
	default:
		// usageModified 时这里是 dto.Usage 结构体，统一经 JSON 转成 map 处理
		encoded, err := common.Marshal(value)
		if err != nil {
			return nil, false
		}
		if err := common.Unmarshal(encoded, &source); err != nil {
			return nil, false
		}
	}
	result := make(map[string]any, len(source))
	for key, value := range source {
		if clientUsageAllowedKeys[key] {
			result[key] = value
		}
	}
	details, _ := result["prompt_tokens_details"].(map[string]any)
	if details == nil {
		details = map[string]any{}
	}
	if _, exists := details["cached_tokens"]; !exists {
		details["cached_tokens"] = cachedTokens
	}
	result["prompt_tokens_details"] = details
	return result, true
}
