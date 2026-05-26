package ratio_setting

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/types"
)

const (
	VideoBillingModePerSecond = "per_second"
	VideoBillingModePerCall   = "per_call"
)

var videoBillingModeMap = types.NewRWMap[string, string]()

func VideoBillingMode2JSONString() string {
	return videoBillingModeMap.MarshalJSONString()
}

func UpdateVideoBillingModeByJSONString(jsonStr string) error {
	if strings.TrimSpace(jsonStr) == "" {
		jsonStr = "{}"
	}
	var parsed map[string]string
	if err := common.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return err
	}
	normalized := make(map[string]string, len(parsed))
	for model, mode := range parsed {
		model = strings.TrimSpace(model)
		mode = NormalizeVideoBillingMode(mode)
		if model == "" || mode == "" {
			continue
		}
		normalized[model] = mode
	}
	jsonBytes, err := common.Marshal(normalized)
	if err != nil {
		return err
	}
	return types.LoadFromJsonStringWithCallback(videoBillingModeMap, string(jsonBytes), InvalidateExposedDataCache)
}

func GetVideoBillingModeCopy() map[string]string {
	return videoBillingModeMap.ReadAll()
}

func NormalizeVideoBillingMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case VideoBillingModePerSecond, "second", "seconds", "per-second", "per second", "按秒", "按秒计费":
		return VideoBillingModePerSecond
	case VideoBillingModePerCall, "call", "request", "per-request", "per_request", "per-call", "per call", "按次", "按次计费":
		return VideoBillingModePerCall
	default:
		return ""
	}
}

func GetVideoBillingMode(modelName string) string {
	if mode, ok := matchConfiguredVideoBillingMode(modelName); ok {
		return mode
	}
	if common.StringsContains(constant.TaskPricePatches, modelName) {
		return VideoBillingModePerCall
	}
	return VideoBillingModePerSecond
}

func HasVideoBillingMode(modelName string) bool {
	_, ok := matchConfiguredVideoBillingMode(modelName)
	return ok
}

func IsVideoBillingPerCall(modelName string) bool {
	return GetVideoBillingMode(modelName) == VideoBillingModePerCall
}

func IsVideoBillingPerSecond(modelName string) bool {
	return GetVideoBillingMode(modelName) == VideoBillingModePerSecond
}

func matchConfiguredVideoBillingMode(modelName string) (string, bool) {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return "", false
	}
	if mode, ok := videoBillingModeMap.Get(modelName); ok {
		if normalized := NormalizeVideoBillingMode(mode); normalized != "" {
			return normalized, true
		}
	}
	formattedName := FormatMatchingModelName(modelName)
	if formattedName != modelName {
		if mode, ok := videoBillingModeMap.Get(formattedName); ok {
			if normalized := NormalizeVideoBillingMode(mode); normalized != "" {
				return normalized, true
			}
		}
	}

	bestPatternLen := -1
	bestMode := ""
	for pattern, mode := range videoBillingModeMap.ReadAll() {
		normalized := NormalizeVideoBillingMode(mode)
		if normalized == "" || !wildcardMatch(pattern, modelName) {
			continue
		}
		patternLen := len(strings.ReplaceAll(pattern, "*", ""))
		if patternLen > bestPatternLen {
			bestPatternLen = patternLen
			bestMode = normalized
		}
	}
	if bestMode == "" {
		return "", false
	}
	return bestMode, true
}

func wildcardMatch(pattern, value string) bool {
	pattern = strings.TrimSpace(pattern)
	value = strings.TrimSpace(value)
	if pattern == "" || value == "" {
		return false
	}
	if pattern == value {
		return true
	}
	if !strings.Contains(pattern, "*") {
		return false
	}
	parts := strings.Split(pattern, "*")
	position := 0
	for index, part := range parts {
		if part == "" {
			continue
		}
		found := strings.Index(value[position:], part)
		if found < 0 {
			return false
		}
		if index == 0 && !strings.HasPrefix(pattern, "*") && found != 0 {
			return false
		}
		position += found + len(part)
	}
	lastPart := parts[len(parts)-1]
	if lastPart != "" && !strings.HasSuffix(pattern, "*") && !strings.HasSuffix(value, lastPart) {
		return false
	}
	return true
}
