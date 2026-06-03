package operation_setting

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

type ModelLimitSetting struct {
	SeedanceResourcePoolGuardEnabled bool   `json:"seedance_resource_pool_guard_enabled"`
	SeedanceResourcePoolGuardModels  string `json:"seedance_resource_pool_guard_models"`
	SeedanceResourcePoolGuardUserIds string `json:"seedance_resource_pool_guard_user_ids"`
	SeedanceResourcePoolGuardMessage string `json:"seedance_resource_pool_guard_message"`
}

var modelLimitSetting = ModelLimitSetting{
	SeedanceResourcePoolGuardEnabled: true,
	SeedanceResourcePoolGuardModels:  "seedance-2.0-fast-480p",
	SeedanceResourcePoolGuardUserIds: "42\n2113417732",
	SeedanceResourcePoolGuardMessage: "此模型资源池已耗尽，请使用其他的模型。",
}

func init() {
	config.GlobalConfig.Register("model_limit_setting", &modelLimitSetting)
}

func GetModelLimitSetting() *ModelLimitSetting {
	return &modelLimitSetting
}

func splitSeedanceResourcePoolGuardList(value string) []string {
	items := strings.FieldsFunc(value, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ',' || r == '，' || r == ';' || r == '；' || r == ' ' || r == '\t'
	})
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}

func IsSeedanceResourcePoolGuardBlocked(userId int, modelName string) (string, bool) {
	setting := GetModelLimitSetting()
	if !setting.SeedanceResourcePoolGuardEnabled {
		return "", false
	}

	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return "", false
	}

	modelMatched := false
	for _, configuredModel := range splitSeedanceResourcePoolGuardList(setting.SeedanceResourcePoolGuardModels) {
		if strings.EqualFold(configuredModel, modelName) {
			modelMatched = true
			break
		}
	}
	if !modelMatched {
		return "", false
	}

	userMatched := false
	for _, configuredUserId := range splitSeedanceResourcePoolGuardList(setting.SeedanceResourcePoolGuardUserIds) {
		id, err := strconv.Atoi(configuredUserId)
		if err == nil && id == userId {
			userMatched = true
			break
		}
	}
	if !userMatched {
		return "", false
	}

	message := strings.TrimSpace(setting.SeedanceResourcePoolGuardMessage)
	if message == "" {
		message = "此模型资源池已耗尽，请使用其他的模型。"
	}
	return message, true
}
