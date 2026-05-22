package model_setting

import (
	"path"
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

type TaskBillingSettings struct {
	DurationBillingModelPatterns        []string `json:"duration_billing_model_patterns"`
	DurationBillingExcludeModelPatterns []string `json:"duration_billing_exclude_model_patterns"`
	ReferenceVideoBillingModelPatterns  []string `json:"reference_video_billing_model_patterns"`
}

var defaultTaskBillingSettings = TaskBillingSettings{
	DurationBillingModelPatterns: []string{
		"sora-2*",
		"seedance-*",
		"doubao-seedance-*",
	},
	DurationBillingExcludeModelPatterns: []string{
		"grok-imagine-video",
		"grok-imagine-1.0-video",
	},
	ReferenceVideoBillingModelPatterns: []string{
		"seedance-*",
		"doubao-seedance-*",
	},
}

var taskBillingSettings = defaultTaskBillingSettings

func init() {
	config.GlobalConfig.Register("task_billing_setting", &taskBillingSettings)
}

func GetTaskBillingSettings() *TaskBillingSettings {
	return &taskBillingSettings
}

func IsTaskDurationBillingModel(modelNames ...string) bool {
	settings := GetTaskBillingSettings()
	return matchTaskBillingModelNames(
		modelNames,
		settings.DurationBillingModelPatterns,
		settings.DurationBillingExcludeModelPatterns,
	)
}

func IsTaskReferenceVideoBillingModel(modelNames ...string) bool {
	settings := GetTaskBillingSettings()
	return matchTaskBillingModelNames(
		modelNames,
		settings.ReferenceVideoBillingModelPatterns,
		settings.DurationBillingExcludeModelPatterns,
	)
}

func matchTaskBillingModelNames(modelNames []string, includePatterns []string, excludePatterns []string) bool {
	matched := false
	for _, modelName := range modelNames {
		modelName = normalizeTaskBillingModelPattern(modelName)
		if modelName == "" {
			continue
		}
		if matchTaskBillingPattern(excludePatterns, modelName) {
			return false
		}
		if matchTaskBillingPattern(includePatterns, modelName) {
			matched = true
		}
	}
	return matched
}

func matchTaskBillingPattern(patterns []string, modelName string) bool {
	for _, pattern := range patterns {
		pattern = normalizeTaskBillingModelPattern(pattern)
		if pattern == "" {
			continue
		}
		if pattern == modelName {
			return true
		}
		if strings.Contains(pattern, "*") {
			if ok, err := path.Match(pattern, modelName); err == nil && ok {
				return true
			}
		}
	}
	return false
}

func normalizeTaskBillingModelPattern(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
