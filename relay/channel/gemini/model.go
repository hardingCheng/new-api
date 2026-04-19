package gemini

import (
	"strings"

	"github.com/QuantumNous/new-api/setting/model_setting"
)

func shouldUseInlineDataResponseMode(modelName string) bool {
	return model_setting.GetGeminiSettings().ImageResponseInlineDataEnabled &&
		strings.Contains(strings.ToLower(modelName), "image")
}
