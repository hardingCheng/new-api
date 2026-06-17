package dto

import "strings"

const ImageQualityInstruction = "保持图片质量不变。Keep the image quality unchanged."

func AppendImageQualityInstruction(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" || strings.Contains(prompt, ImageQualityInstruction) {
		return prompt
	}
	return prompt + "\n\n" + ImageQualityInstruction
}
