package common

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

const MaxUsageTokenMultiplier = 100

var usageTokenContainerKeys = map[string]struct{}{
	"usage":          {},
	"usage_metadata": {},
	"usageMetadata":  {},
	"billing_usage":  {},
}

var usageTokenFieldKeys = map[string]struct{}{
	"prompt_tokens":                    {},
	"completion_tokens":                {},
	"total_tokens":                     {},
	"prompt_cache_hit_tokens":          {},
	"input_tokens":                     {},
	"output_tokens":                    {},
	"cached_tokens":                    {},
	"cached_creation_tokens":           {},
	"cache_write_tokens":               {},
	"text_tokens":                      {},
	"audio_tokens":                     {},
	"image_tokens":                     {},
	"reasoning_tokens":                 {},
	"cache_creation_input_tokens":      {},
	"cache_read_input_tokens":          {},
	"ephemeral_5m_input_tokens":        {},
	"ephemeral_1h_input_tokens":        {},
	"claude_cache_creation_5_m_tokens": {},
	"claude_cache_creation_1_h_tokens": {},
	"promptTokenCount":                 {},
	"toolUsePromptTokenCount":          {},
	"candidatesTokenCount":             {},
	"totalTokenCount":                  {},
	"thoughtsTokenCount":               {},
	"cachedContentTokenCount":          {},
	"tokenCount":                       {},
}

// ScaleTokenCount applies the contractual usage multiplier while keeping
// persisted token counters within the database's signed 32-bit range.
func ScaleTokenCount(value, multiplier int) int {
	if value <= 0 || multiplier <= 1 {
		return value
	}
	if value > math.MaxInt32/multiplier {
		return math.MaxInt32
	}
	return value * multiplier
}

// ScaleTokenUsageJSON rewrites token counters inside OpenAI, Anthropic and
// Gemini usage objects. Unrelated numeric fields and invalid/non-JSON payloads
// are returned unchanged, which makes the function safe for SSE [DONE] frames
// and binary response paths.
func ScaleTokenUsageJSON(data []byte, multiplier int) []byte {
	if multiplier <= 1 || len(data) == 0 {
		return data
	}

	scaled, changed := scaleTokenUsageJSONValue(json.RawMessage(data), false, multiplier)
	if !changed {
		return data
	}
	return scaled
}

func scaleTokenUsageJSONValue(data json.RawMessage, inUsage bool, multiplier int) (json.RawMessage, bool) {
	switch GetJsonType(data) {
	case "object":
		var object map[string]json.RawMessage
		if err := Unmarshal(data, &object); err != nil {
			return data, false
		}

		changed := false
		for key, value := range object {
			_, isUsageContainer := usageTokenContainerKeys[key]
			childInUsage := inUsage || isUsageContainer

			if childInUsage {
				if _, isTokenField := usageTokenFieldKeys[key]; isTokenField && GetJsonType(value) == "number" {
					if scaled, ok := scaleTokenJSONNumber(value, multiplier); ok {
						object[key] = scaled
						changed = true
						continue
					}
				}
			}

			scaled, childChanged := scaleTokenUsageJSONValue(value, childInUsage, multiplier)
			if childChanged {
				object[key] = scaled
				changed = true
			}
		}
		if !changed {
			return data, false
		}
		encoded, err := Marshal(object)
		if err != nil {
			return data, false
		}
		return json.RawMessage(encoded), true

	case "array":
		var array []json.RawMessage
		if err := Unmarshal(data, &array); err != nil {
			return data, false
		}

		changed := false
		for i, value := range array {
			scaled, childChanged := scaleTokenUsageJSONValue(value, inUsage, multiplier)
			if childChanged {
				array[i] = scaled
				changed = true
			}
		}
		if !changed {
			return data, false
		}
		encoded, err := Marshal(array)
		if err != nil {
			return data, false
		}
		return json.RawMessage(encoded), true
	}

	return data, false
}

func scaleTokenJSONNumber(data json.RawMessage, multiplier int) (json.RawMessage, bool) {
	value, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil || value <= 0 {
		return data, false
	}
	if value > int64(math.MaxInt32)/int64(multiplier) {
		value = math.MaxInt32
	} else {
		value *= int64(multiplier)
	}
	return json.RawMessage(strconv.FormatInt(value, 10)), true
}
