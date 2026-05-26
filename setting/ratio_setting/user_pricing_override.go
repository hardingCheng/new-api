package ratio_setting

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
)

const (
	UserPricingRuleRatio      = "ratio"
	UserPricingRuleModelPrice = "model_price"
	UserPricingRuleModelRatio = "model_ratio"
)

type UserPricingRule struct {
	UserID       int     `json:"user_id"`
	Username     string  `json:"username,omitempty"`
	UserGroup    string  `json:"user_group,omitempty"`
	GroupPattern string  `json:"group_pattern,omitempty"`
	ModelPattern string  `json:"model_pattern,omitempty"`
	Type         string  `json:"type"`
	Value        float64 `json:"value"`
	Disabled     bool    `json:"disabled,omitempty"`
}

type UserPricingOverrideConfig struct {
	Rules []UserPricingRule `json:"rules"`
}

type UserPricingOverrideMatch struct {
	Rule        UserPricingRule `json:"rule"`
	Description string          `json:"description,omitempty"`
}

var userPricingOverrideConfig = UserPricingOverrideConfig{
	Rules: []UserPricingRule{},
}

func UserPricingOverride2JSONString() string {
	jsonBytes, err := common.Marshal(userPricingOverrideConfig)
	if err != nil {
		common.SysError("error marshalling user pricing override: " + err.Error())
		return "{}"
	}
	return string(jsonBytes)
}

func UpdateUserPricingOverrideByJSONString(jsonStr string) error {
	if strings.TrimSpace(jsonStr) == "" {
		jsonStr = "{}"
	}
	var cfg UserPricingOverrideConfig
	if err := common.Unmarshal([]byte(jsonStr), &cfg); err != nil {
		return err
	}
	userPricingOverrideConfig = normalizeUserPricingOverrideConfig(cfg)
	InvalidateExposedDataCache()
	return nil
}

func GetUserPricingOverrideCopy() UserPricingOverrideConfig {
	cfg := userPricingOverrideConfig
	cfg.Rules = append([]UserPricingRule(nil), cfg.Rules...)
	return cfg
}

func MatchUserPricingOverride(userID int, username, userGroup, usingGroup, modelName string) []UserPricingOverrideMatch {
	if userID <= 0 {
		return nil
	}
	var matches []UserPricingOverrideMatch
	for _, rule := range userPricingOverrideConfig.Rules {
		if rule.Disabled || rule.UserID != userID || rule.Value < 0 {
			continue
		}
		if !matchUserPricingGroup(rule.GroupPattern, userGroup, usingGroup) {
			continue
		}
		if !matchUserPricingModel(rule.ModelPattern, modelName) {
			continue
		}
		matches = append(matches, UserPricingOverrideMatch{
			Rule:        rule,
			Description: ruleDescription(rule),
		})
	}
	return matches
}

func ApplyUserPricingOverrides(userID int, username, userGroup, usingGroup, modelName string, usePrice bool, modelPrice, modelRatio, groupRatio float64) (bool, float64, float64, float64, []UserPricingOverrideMatch) {
	matches := MatchUserPricingOverride(userID, username, userGroup, usingGroup, modelName)
	if len(matches) == 0 {
		return usePrice, modelPrice, modelRatio, groupRatio, nil
	}

	bestRatio, hasRatio := bestUserPricingMatch(matches, UserPricingRuleRatio)
	if hasRatio {
		groupRatio = bestRatio.Rule.Value
	}

	bestPrice, hasPrice := bestUserPricingMatch(matches, UserPricingRuleModelPrice)
	bestModelRatio, hasModelRatio := bestUserPricingMatch(matches, UserPricingRuleModelRatio)

	if hasPrice && (!hasModelRatio || matchPriority(bestPrice.Rule) >= matchPriority(bestModelRatio.Rule)) {
		usePrice = true
		modelPrice = bestPrice.Rule.Value
		modelRatio = 0
	} else if hasModelRatio {
		usePrice = false
		modelRatio = bestModelRatio.Rule.Value
		modelPrice = -1
	}

	applied := make([]UserPricingOverrideMatch, 0, 2)
	if hasRatio {
		applied = append(applied, bestRatio)
	}
	if usePrice && hasPrice {
		applied = append(applied, bestPrice)
	} else if !usePrice && hasModelRatio {
		applied = append(applied, bestModelRatio)
	}

	return usePrice, modelPrice, modelRatio, groupRatio, applied
}

func normalizeUserPricingOverrideConfig(cfg UserPricingOverrideConfig) UserPricingOverrideConfig {
	rules := make([]UserPricingRule, 0, len(cfg.Rules))
	for _, rule := range cfg.Rules {
		rule.Username = strings.TrimSpace(rule.Username)
		rule.UserGroup = strings.TrimSpace(rule.UserGroup)
		rule.GroupPattern = normalizeUserPricingPattern(rule.GroupPattern)
		rule.ModelPattern = normalizeUserPricingPattern(rule.ModelPattern)
		rule.Type = normalizeUserPricingRuleType(rule.Type)
		if rule.UserID <= 0 || rule.Type == "" || rule.Value < 0 {
			continue
		}
		rules = append(rules, rule)
	}
	return UserPricingOverrideConfig{Rules: rules}
}

func normalizeUserPricingPattern(pattern string) string {
	pattern = strings.TrimSpace(pattern)
	if pattern == "全部" || strings.EqualFold(pattern, "all") {
		return ""
	}
	return pattern
}

func normalizeUserPricingRuleType(ruleType string) string {
	switch strings.ToLower(strings.TrimSpace(ruleType)) {
	case UserPricingRuleRatio, "group_ratio", "倍率":
		return UserPricingRuleRatio
	case UserPricingRuleModelPrice, "model-price", "fixed_price", "固定价格":
		return UserPricingRuleModelPrice
	case UserPricingRuleModelRatio, "model-ratio", "模型倍率":
		return UserPricingRuleModelRatio
	default:
		return ""
	}
}

func matchUserPricingGroup(pattern, userGroup, usingGroup string) bool {
	if pattern == "" {
		return true
	}
	return wildcardMatch(pattern, usingGroup) || wildcardMatch(pattern, userGroup)
}

func matchUserPricingModel(pattern, modelName string) bool {
	if pattern == "" {
		return true
	}
	return wildcardMatch(pattern, modelName)
}

func bestUserPricingMatch(matches []UserPricingOverrideMatch, ruleType string) (UserPricingOverrideMatch, bool) {
	bestScore := -1
	var best UserPricingOverrideMatch
	for _, match := range matches {
		if match.Rule.Type != ruleType {
			continue
		}
		score := matchPriority(match.Rule)
		if score > bestScore {
			bestScore = score
			best = match
		}
	}
	return best, bestScore >= 0
}

func matchPriority(rule UserPricingRule) int {
	score := 0
	if rule.GroupPattern != "" {
		score += 1000 + len(strings.ReplaceAll(rule.GroupPattern, "*", ""))
	}
	if rule.ModelPattern != "" {
		score += 2000 + len(strings.ReplaceAll(rule.ModelPattern, "*", ""))
	}
	return score
}

func ruleDescription(rule UserPricingRule) string {
	if rule.GroupPattern != "" && rule.ModelPattern != "" {
		return "user_group_model"
	}
	if rule.ModelPattern != "" {
		return "user_model"
	}
	if rule.GroupPattern != "" {
		return "user_group"
	}
	return "user_all"
}
