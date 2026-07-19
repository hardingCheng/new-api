package model_setting

import (
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
)

const (
	ReferenceVideoAllowed   = "allowed"
	ReferenceVideoForbidden = "forbidden"
)

type UserModelAlias struct {
	PublicModel    string `json:"public_model"`
	TargetModel    string `json:"target_model"`
	ReferenceVideo string `json:"reference_video"`
}

type UserModelViewRule struct {
	UserID    int              `json:"user_id"`
	Username  string           `json:"username,omitempty"`
	UserGroup string           `json:"user_group,omitempty"`
	Disabled  bool             `json:"disabled,omitempty"`
	Aliases   []UserModelAlias `json:"aliases"`
}

type UserModelViewConfig struct {
	Rules []UserModelViewRule `json:"rules"`
}

type VisibleUserModel struct {
	Name        string
	TargetModel string
}

var userModelViewConfig atomic.Value

func init() {
	userModelViewConfig.Store(UserModelViewConfig{Rules: []UserModelViewRule{}})
}

func UserModelView2JSONString() string {
	jsonBytes, err := common.Marshal(GetUserModelViewCopy())
	if err != nil {
		common.SysError("error marshalling user model view: " + err.Error())
		return `{"rules":[]}`
	}
	return string(jsonBytes)
}

func UpdateUserModelViewByJSONString(jsonStr string) error {
	config, err := ParseUserModelViewJSONString(jsonStr)
	if err != nil {
		return err
	}
	userModelViewConfig.Store(config)
	return nil
}

func ParseUserModelViewJSONString(jsonStr string) (UserModelViewConfig, error) {
	if strings.TrimSpace(jsonStr) == "" {
		jsonStr = `{"rules":[]}`
	}
	var config UserModelViewConfig
	if err := common.UnmarshalJsonStr(jsonStr, &config); err != nil {
		return UserModelViewConfig{}, err
	}

	userIDs := make(map[int]struct{}, len(config.Rules))
	for ruleIndex := range config.Rules {
		rule := &config.Rules[ruleIndex]
		if rule.UserID <= 0 {
			return UserModelViewConfig{}, fmt.Errorf("rule %d has an invalid user_id", ruleIndex+1)
		}
		if _, exists := userIDs[rule.UserID]; exists {
			return UserModelViewConfig{}, fmt.Errorf("user_id %d has more than one model view rule", rule.UserID)
		}
		userIDs[rule.UserID] = struct{}{}
		rule.Username = strings.TrimSpace(rule.Username)
		rule.UserGroup = strings.TrimSpace(rule.UserGroup)

		publicModels := make(map[string]struct{}, len(rule.Aliases))
		targetModels := make(map[string]struct{}, len(rule.Aliases))
		for aliasIndex := range rule.Aliases {
			alias := &rule.Aliases[aliasIndex]
			alias.PublicModel = strings.TrimSpace(alias.PublicModel)
			alias.TargetModel = strings.TrimSpace(alias.TargetModel)
			alias.ReferenceVideo = strings.ToLower(strings.TrimSpace(alias.ReferenceVideo))
			if alias.PublicModel == "" || alias.TargetModel == "" {
				return UserModelViewConfig{}, fmt.Errorf("user_id %d alias %d requires public_model and target_model", rule.UserID, aliasIndex+1)
			}
			if alias.PublicModel == alias.TargetModel {
				return UserModelViewConfig{}, fmt.Errorf("user_id %d alias %q must target a different model", rule.UserID, alias.PublicModel)
			}
			if alias.ReferenceVideo != ReferenceVideoAllowed && alias.ReferenceVideo != ReferenceVideoForbidden {
				return UserModelViewConfig{}, fmt.Errorf("user_id %d alias %q has invalid reference_video policy", rule.UserID, alias.PublicModel)
			}
			if _, exists := publicModels[alias.PublicModel]; exists {
				return UserModelViewConfig{}, fmt.Errorf("user_id %d has duplicate public model %q", rule.UserID, alias.PublicModel)
			}
			publicModels[alias.PublicModel] = struct{}{}
			targetModels[alias.TargetModel] = struct{}{}
		}
		for publicModel := range publicModels {
			if _, conflicts := targetModels[publicModel]; conflicts {
				return UserModelViewConfig{}, fmt.Errorf("user_id %d model %q cannot be both public and target", rule.UserID, publicModel)
			}
		}
	}
	if config.Rules == nil {
		config.Rules = []UserModelViewRule{}
	}
	return config, nil
}

func GetUserModelViewCopy() UserModelViewConfig {
	config, ok := userModelViewConfig.Load().(UserModelViewConfig)
	if !ok {
		return UserModelViewConfig{Rules: []UserModelViewRule{}}
	}
	copyConfig := UserModelViewConfig{Rules: make([]UserModelViewRule, len(config.Rules))}
	for index, rule := range config.Rules {
		copyConfig.Rules[index] = rule
		copyConfig.Rules[index].Aliases = append([]UserModelAlias(nil), rule.Aliases...)
	}
	return copyConfig
}

func GetUserModelViewRule(userID int) (UserModelViewRule, bool) {
	config, ok := userModelViewConfig.Load().(UserModelViewConfig)
	if !ok || userID <= 0 {
		return UserModelViewRule{}, false
	}
	for _, rule := range config.Rules {
		if rule.UserID == userID && !rule.Disabled {
			rule.Aliases = append([]UserModelAlias(nil), rule.Aliases...)
			return rule, true
		}
	}
	return UserModelViewRule{}, false
}

func ResolveUserModelAlias(userID int, modelName string) (UserModelAlias, bool) {
	rule, ok := GetUserModelViewRule(userID)
	if !ok {
		return UserModelAlias{}, false
	}
	modelName = strings.TrimSpace(modelName)
	for _, alias := range rule.Aliases {
		if alias.PublicModel == modelName {
			return alias, true
		}
	}
	return UserModelAlias{}, false
}

func BuildVisibleUserModels(userID int, availableModels []string) []VisibleUserModel {
	rule, ok := GetUserModelViewRule(userID)
	if !ok {
		models := make([]VisibleUserModel, 0, len(availableModels))
		for _, modelName := range availableModels {
			models = append(models, VisibleUserModel{Name: modelName, TargetModel: modelName})
		}
		return models
	}

	aliasesByTarget := make(map[string][]UserModelAlias)
	for _, alias := range rule.Aliases {
		aliasesByTarget[alias.TargetModel] = append(aliasesByTarget[alias.TargetModel], alias)
	}
	visible := make([]VisibleUserModel, 0, len(availableModels)+len(rule.Aliases))
	seen := make(map[string]struct{}, len(availableModels)+len(rule.Aliases))
	for _, modelName := range availableModels {
		aliases := aliasesByTarget[modelName]
		if len(aliases) == 0 {
			if _, exists := seen[modelName]; !exists {
				visible = append(visible, VisibleUserModel{Name: modelName, TargetModel: modelName})
				seen[modelName] = struct{}{}
			}
			continue
		}
		for _, alias := range aliases {
			if _, exists := seen[alias.PublicModel]; exists {
				continue
			}
			visible = append(visible, VisibleUserModel{Name: alias.PublicModel, TargetModel: alias.TargetModel})
			seen[alias.PublicModel] = struct{}{}
		}
	}
	return visible
}
