package model_setting

import (
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
)

const (
	UserChannelRoutingFallbackStrict  = "strict"
	UserChannelRoutingFallbackDefault = "default"
)

type UserChannelRoutingRule struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	UserID       int    `json:"user_id"`
	Username     string `json:"username,omitempty"`
	UserGroup    string `json:"user_group,omitempty"`
	GroupPattern string `json:"group_pattern"`
	ModelPattern string `json:"model_pattern,omitempty"`
	ChannelIDs   []int  `json:"channel_ids"`
	Fallback     string `json:"fallback"`
	Disabled     bool   `json:"disabled,omitempty"`
}

type UserChannelRoutingConfig struct {
	Rules []UserChannelRoutingRule `json:"rules"`
}

type UserChannelRoutingMatch struct {
	Rule              UserChannelRoutingRule
	AllowedChannelIDs map[int]struct{}
}

type userChannelRoutingSnapshot struct {
	config UserChannelRoutingConfig
	byUser map[int][]UserChannelRoutingRule
}

var userChannelRoutingConfig atomic.Value

func init() {
	userChannelRoutingConfig.Store(userChannelRoutingSnapshot{
		config: UserChannelRoutingConfig{Rules: []UserChannelRoutingRule{}},
		byUser: map[int][]UserChannelRoutingRule{},
	})
}

func UserChannelRouting2JSONString() string {
	jsonBytes, err := common.Marshal(GetUserChannelRoutingCopy())
	if err != nil {
		common.SysError("error marshalling user channel routing: " + err.Error())
		return `{"rules":[]}`
	}
	return string(jsonBytes)
}

func UpdateUserChannelRoutingByJSONString(jsonStr string) error {
	config, err := ParseUserChannelRoutingJSONString(jsonStr)
	if err != nil {
		return err
	}
	byUser := make(map[int][]UserChannelRoutingRule)
	for _, rule := range config.Rules {
		byUser[rule.UserID] = append(byUser[rule.UserID], cloneUserChannelRoutingRule(rule))
	}
	userChannelRoutingConfig.Store(userChannelRoutingSnapshot{config: config, byUser: byUser})
	return nil
}

func ParseUserChannelRoutingJSONString(jsonStr string) (UserChannelRoutingConfig, error) {
	if strings.TrimSpace(jsonStr) == "" {
		jsonStr = `{"rules":[]}`
	}
	var config UserChannelRoutingConfig
	if err := common.UnmarshalJsonStr(jsonStr, &config); err != nil {
		return UserChannelRoutingConfig{}, err
	}

	ruleIDs := make(map[string]struct{}, len(config.Rules))
	scopes := make(map[string]struct{}, len(config.Rules))
	for index := range config.Rules {
		rule := &config.Rules[index]
		rule.ID = strings.TrimSpace(rule.ID)
		rule.Name = strings.TrimSpace(rule.Name)
		rule.Username = strings.TrimSpace(rule.Username)
		rule.UserGroup = strings.TrimSpace(rule.UserGroup)
		rule.GroupPattern = strings.TrimSpace(rule.GroupPattern)
		rule.ModelPattern = strings.TrimSpace(rule.ModelPattern)
		rule.Fallback = strings.ToLower(strings.TrimSpace(rule.Fallback))

		if rule.ID == "" {
			return UserChannelRoutingConfig{}, fmt.Errorf("rule %d requires id", index+1)
		}
		if _, exists := ruleIDs[rule.ID]; exists {
			return UserChannelRoutingConfig{}, fmt.Errorf("duplicate rule id %q", rule.ID)
		}
		ruleIDs[rule.ID] = struct{}{}
		if rule.Name == "" {
			return UserChannelRoutingConfig{}, fmt.Errorf("rule %q requires name", rule.ID)
		}
		if rule.UserID <= 0 {
			return UserChannelRoutingConfig{}, fmt.Errorf("rule %q has an invalid user_id", rule.ID)
		}
		if rule.GroupPattern == "" {
			return UserChannelRoutingConfig{}, fmt.Errorf("rule %q requires group_pattern", rule.ID)
		}
		if rule.GroupPattern != "*" && strings.Contains(rule.GroupPattern, "*") {
			return UserChannelRoutingConfig{}, fmt.Errorf("rule %q group_pattern must be an exact group or *", rule.ID)
		}
		if rule.ModelPattern == "" {
			rule.ModelPattern = "*"
		}
		if rule.Fallback == "" {
			rule.Fallback = UserChannelRoutingFallbackStrict
		}
		if rule.Fallback != UserChannelRoutingFallbackStrict && rule.Fallback != UserChannelRoutingFallbackDefault {
			return UserChannelRoutingConfig{}, fmt.Errorf("rule %q has an invalid fallback", rule.ID)
		}
		if len(rule.ChannelIDs) == 0 {
			return UserChannelRoutingConfig{}, fmt.Errorf("rule %q requires at least one channel_id", rule.ID)
		}
		seenChannelIDs := make(map[int]struct{}, len(rule.ChannelIDs))
		for _, channelID := range rule.ChannelIDs {
			if channelID <= 0 {
				return UserChannelRoutingConfig{}, fmt.Errorf("rule %q has an invalid channel_id", rule.ID)
			}
			if _, exists := seenChannelIDs[channelID]; exists {
				return UserChannelRoutingConfig{}, fmt.Errorf("rule %q has duplicate channel_id %d", rule.ID, channelID)
			}
			seenChannelIDs[channelID] = struct{}{}
		}

		scope := fmt.Sprintf("%d\x00%s\x00%s", rule.UserID, rule.GroupPattern, rule.ModelPattern)
		if _, exists := scopes[scope]; exists {
			return UserChannelRoutingConfig{}, fmt.Errorf("user_id %d has duplicate channel routing scope %s / %s", rule.UserID, rule.GroupPattern, rule.ModelPattern)
		}
		scopes[scope] = struct{}{}
	}
	if config.Rules == nil {
		config.Rules = []UserChannelRoutingRule{}
	}
	return config, nil
}

func GetUserChannelRoutingCopy() UserChannelRoutingConfig {
	snapshot, ok := userChannelRoutingConfig.Load().(userChannelRoutingSnapshot)
	if !ok {
		return UserChannelRoutingConfig{Rules: []UserChannelRoutingRule{}}
	}
	config := UserChannelRoutingConfig{Rules: make([]UserChannelRoutingRule, len(snapshot.config.Rules))}
	for index, rule := range snapshot.config.Rules {
		config.Rules[index] = cloneUserChannelRoutingRule(rule)
	}
	return config
}

func MatchUserChannelRouting(userID int, usingGroup, modelName string) (UserChannelRoutingMatch, bool) {
	if userID <= 0 {
		return UserChannelRoutingMatch{}, false
	}
	snapshot, ok := userChannelRoutingConfig.Load().(userChannelRoutingSnapshot)
	if !ok {
		return UserChannelRoutingMatch{}, false
	}

	bestIndex := -1
	bestScore := -1
	rules := snapshot.byUser[userID]
	for index, rule := range rules {
		if rule.Disabled || (rule.GroupPattern != "*" && rule.GroupPattern != usingGroup) || !matchUserChannelRoutingPattern(rule.ModelPattern, modelName) {
			continue
		}
		score := 0
		if rule.GroupPattern != "*" {
			score += 1_000_000
		}
		if rule.ModelPattern != "*" {
			if !strings.Contains(rule.ModelPattern, "*") {
				score += 100_000
			}
			score += len(strings.ReplaceAll(rule.ModelPattern, "*", ""))
		}
		if score > bestScore {
			bestIndex = index
			bestScore = score
		}
	}
	if bestIndex < 0 {
		return UserChannelRoutingMatch{}, false
	}

	rule := cloneUserChannelRoutingRule(rules[bestIndex])
	allowed := make(map[int]struct{}, len(rule.ChannelIDs))
	for _, channelID := range rule.ChannelIDs {
		allowed[channelID] = struct{}{}
	}
	return UserChannelRoutingMatch{Rule: rule, AllowedChannelIDs: allowed}, true
}

func cloneUserChannelRoutingRule(rule UserChannelRoutingRule) UserChannelRoutingRule {
	rule.ChannelIDs = append([]int(nil), rule.ChannelIDs...)
	return rule
}

func matchUserChannelRoutingPattern(pattern, value string) bool {
	if pattern == "*" {
		return true
	}
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == value
	}
	if !strings.HasPrefix(pattern, "*") {
		if !strings.HasPrefix(value, parts[0]) {
			return false
		}
		value = value[len(parts[0]):]
		parts = parts[1:]
	}
	lastIndex := len(parts) - 1
	for index, part := range parts {
		if part == "" {
			continue
		}
		if index == lastIndex && !strings.HasSuffix(pattern, "*") {
			return strings.HasSuffix(value, part)
		}
		position := strings.Index(value, part)
		if position < 0 {
			return false
		}
		value = value[position+len(part):]
	}
	return true
}
