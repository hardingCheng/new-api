package ratio_setting

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

const (
	ModelQuotaPoolScopeGlobal = "global"
	ModelQuotaPoolScopeUser   = "user"

	ModelQuotaPoolMetricRequests    = "requests"
	ModelQuotaPoolMetricTotalTokens = "total_tokens"
	ModelQuotaPoolMetricQuota       = "quota"

	ModelQuotaPoolPeriodMinute = "minute"
	ModelQuotaPoolPeriodHour   = "hour"
	ModelQuotaPoolPeriodDay    = "day"
	ModelQuotaPoolPeriodWeek   = "week"
	ModelQuotaPoolPeriodMonth  = "month"
)

type ModelQuotaPoolRule struct {
	ID        string `json:"id,omitempty"`
	Model     string `json:"model"`
	Scope     string `json:"scope"`
	Metric    string `json:"metric,omitempty"`
	UserID    int    `json:"user_id,omitempty"`
	Username  string `json:"username,omitempty"`
	UserGroup string `json:"user_group,omitempty"`
	Period    string `json:"period"`
	Limit     int64  `json:"limit"`
	Message   string `json:"message,omitempty"`
	Disabled  bool   `json:"disabled,omitempty"`
}

type ModelQuotaPoolConfig struct {
	Rules []ModelQuotaPoolRule `json:"rules"`
}

type ModelQuotaPoolMatch struct {
	Rule        ModelQuotaPoolRule `json:"rule"`
	Scope       string             `json:"scope"`
	Metric      string             `json:"metric,omitempty"`
	PeriodKey   string             `json:"period_key"`
	RedisKey    string             `json:"redis_key,omitempty"`
	Limit       int64              `json:"limit"`
	Amount      int64              `json:"amount,omitempty"`
	UsedBefore  int64              `json:"used_before"`
	UsedAfter   int64              `json:"used_after"`
	Remaining   int64              `json:"remaining"`
	Description string             `json:"description,omitempty"`
}

var modelQuotaPoolConfig = ModelQuotaPoolConfig{
	Rules: []ModelQuotaPoolRule{},
}

func ModelQuotaPool2JSONString() string {
	jsonBytes, err := common.Marshal(modelQuotaPoolConfig)
	if err != nil {
		common.SysError("error marshalling model quota pool: " + err.Error())
		return "{}"
	}
	return string(jsonBytes)
}

func UpdateModelQuotaPoolByJSONString(jsonStr string) error {
	if strings.TrimSpace(jsonStr) == "" {
		jsonStr = "{}"
	}
	var cfg ModelQuotaPoolConfig
	if err := common.Unmarshal([]byte(jsonStr), &cfg); err != nil {
		return err
	}
	modelQuotaPoolConfig = normalizeModelQuotaPoolConfig(cfg)
	InvalidateExposedDataCache()
	return nil
}

func GetModelQuotaPoolCopy() ModelQuotaPoolConfig {
	cfg := modelQuotaPoolConfig
	cfg.Rules = append([]ModelQuotaPoolRule(nil), cfg.Rules...)
	return cfg
}

func MatchModelQuotaPoolRules(userID int, modelName string) []ModelQuotaPoolRule {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return nil
	}

	var globalRules []ModelQuotaPoolRule
	var userRules []ModelQuotaPoolRule
	for _, rule := range modelQuotaPoolConfig.Rules {
		if rule.Disabled || rule.Limit <= 0 || !wildcardMatch(rule.Model, modelName) {
			continue
		}
		switch rule.Scope {
		case ModelQuotaPoolScopeGlobal:
			globalRules = append(globalRules, rule)
		case ModelQuotaPoolScopeUser:
			if rule.UserID > 0 && rule.UserID == userID {
				userRules = append(userRules, rule)
			}
		}
	}

	matches := make([]ModelQuotaPoolRule, 0, 2)
	if userRule, ok := bestModelQuotaPoolRule(userRules); ok {
		matches = append(matches, userRule)
	}
	if globalRule, ok := bestModelQuotaPoolRule(globalRules); ok {
		matches = append(matches, globalRule)
	}
	return matches
}

func ModelQuotaPoolRuleKey(rule ModelQuotaPoolRule) string {
	if strings.TrimSpace(rule.ID) != "" {
		return strings.TrimSpace(rule.ID)
	}
	if rule.Scope == ModelQuotaPoolScopeUser {
		return fmt.Sprintf("%s:%d:%s:%s:%s", rule.Scope, rule.UserID, rule.Metric, rule.Period, rule.Model)
	}
	return fmt.Sprintf("%s:%s:%s:%s", rule.Scope, rule.Metric, rule.Period, rule.Model)
}

func normalizeModelQuotaPoolConfig(cfg ModelQuotaPoolConfig) ModelQuotaPoolConfig {
	rules := make([]ModelQuotaPoolRule, 0, len(cfg.Rules))
	seen := make(map[string]struct{})
	for _, rule := range cfg.Rules {
		rule.Model = strings.TrimSpace(rule.Model)
		rule.Scope = normalizeModelQuotaPoolScope(rule.Scope)
		rule.Metric = normalizeModelQuotaPoolMetric(rule.Metric)
		rule.Period = normalizeModelQuotaPoolPeriod(rule.Period)
		rule.Username = strings.TrimSpace(rule.Username)
		rule.UserGroup = strings.TrimSpace(rule.UserGroup)
		rule.Message = strings.TrimSpace(rule.Message)
		rule.ID = strings.TrimSpace(rule.ID)
		if rule.Model == "" || rule.Scope == "" || rule.Metric == "" || rule.Period == "" || rule.Limit <= 0 {
			continue
		}
		if rule.Scope == ModelQuotaPoolScopeUser && rule.UserID <= 0 {
			continue
		}
		key := ModelQuotaPoolRuleKey(rule)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		rules = append(rules, rule)
	}
	return ModelQuotaPoolConfig{Rules: rules}
}

func normalizeModelQuotaPoolScope(scope string) string {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case ModelQuotaPoolScopeGlobal, "all", "shared", "全局", "共享池", "全局共享池":
		return ModelQuotaPoolScopeGlobal
	case ModelQuotaPoolScopeUser, "user_pool", "指定用户", "用户池", "指定用户池":
		return ModelQuotaPoolScopeUser
	default:
		return ""
	}
}

func normalizeModelQuotaPoolMetric(metric string) string {
	switch strings.ToLower(strings.TrimSpace(metric)) {
	case "", ModelQuotaPoolMetricRequests, "request", "count", "次数", "请求次数":
		return ModelQuotaPoolMetricRequests
	case ModelQuotaPoolMetricTotalTokens, "prompt_tokens", "prompt", "input_tokens", "tokens", "输入token", "输入 tokens", "token消耗", "总token", "总 tokens", "总token消耗":
		return ModelQuotaPoolMetricTotalTokens
	case ModelQuotaPoolMetricQuota, "cost", "spend", "quota_cost", "额度", "花费", "花费消耗", "额度消耗":
		return ModelQuotaPoolMetricQuota
	default:
		return ""
	}
}

func normalizeModelQuotaPoolPeriod(period string) string {
	switch strings.ToLower(strings.TrimSpace(period)) {
	case ModelQuotaPoolPeriodMinute, "min", "1m", "分钟", "每分钟":
		return ModelQuotaPoolPeriodMinute
	case ModelQuotaPoolPeriodHour, "1h", "小时", "每小时":
		return ModelQuotaPoolPeriodHour
	case ModelQuotaPoolPeriodDay, "daily", "1d", "日", "天", "每日":
		return ModelQuotaPoolPeriodDay
	case ModelQuotaPoolPeriodWeek, "weekly", "1w", "周", "每周":
		return ModelQuotaPoolPeriodWeek
	case ModelQuotaPoolPeriodMonth, "monthly", "1mo", "月", "每月":
		return ModelQuotaPoolPeriodMonth
	default:
		return ""
	}
}

func bestModelQuotaPoolRule(rules []ModelQuotaPoolRule) (ModelQuotaPoolRule, bool) {
	bestScore := -1
	var best ModelQuotaPoolRule
	for _, rule := range rules {
		score := len(strings.ReplaceAll(rule.Model, "*", ""))
		if rule.Model == strings.ReplaceAll(rule.Model, "*", "") {
			score += 10000
		}
		if score > bestScore {
			bestScore = score
			best = rule
		}
	}
	return best, bestScore >= 0
}
