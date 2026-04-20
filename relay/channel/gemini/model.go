package gemini

import (
	"strings"

	"github.com/QuantumNous/new-api/setting/model_setting"
)

func shouldUseInlineDataResponseMode(modelName string) bool {
	if !model_setting.GetGeminiSettings().ImageResponseInlineDataEnabled {
		return false
	}
	return model_setting.IsGeminiModelSupportImagine(modelName) ||
		strings.Contains(strings.ToLower(modelName), "image")
}
