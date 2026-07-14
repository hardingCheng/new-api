package ratio_setting

import (
	"strings"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
)

const (
	UserPricingRuleRatio      = "ratio"
	UserPricingRuleModelPrice = "model_price"
	UserPricingRuleModelRatio = "model_ratio"

	// 参考视频秒数定价：仅作用于「参考视频」那部分秒数（reference_video_seconds），
	// 不影响生成秒数。四种模式互斥，同一用户+模型只取最高优先级的一条。
	// A：参考秒 × value（0=免费，0.5=半价，1=原价）
	UserPricingRuleVideoRefFactor = "video_ref_factor"
	// B：参考秒固定单价（美元/秒），与生成秒单价脱钩
	UserPricingRuleVideoRefPrice = "video_ref_price"
	// C：参考整段固定总价（美元），不论参考多少秒
	UserPricingRuleVideoRefFlat = "video_ref_flat"
	// D：参考秒数封顶（秒），参考最多按 value 秒计
	UserPricingRuleVideoRefCap = "video_ref_cap"
)

// 参考视频计价模式（PriceData.VideoRefMode 取值）。
const (
	VideoRefModeNone   = ""
	VideoRefModeFactor = "factor"
	VideoRefModePrice  = "price"
	VideoRefModeFlat   = "flat"
	VideoRefModeCap    = "cap"
)

func referenceRuleMode(ruleType string) string {
	switch ruleType {
	case UserPricingRuleVideoRefFactor:
		return VideoRefModeFactor
	case UserPricingRuleVideoRefPrice:
		return VideoRefModePrice
	case UserPricingRuleVideoRefFlat:
		return VideoRefModeFlat
	case UserPricingRuleVideoRefCap:
		return VideoRefModeCap
	}
	return VideoRefModeNone
}

type UserPricingRule struct {
	UserID       int     `json:"user_id"`
	Username     string  `json:"username,omitempty"`
	UserGroup    string  `json:"user_group,omitempty"`
	GroupPattern string  `json:"group_pattern,omitempty"`
	ModelPattern string  `json:"model_pattern,omitempty"`
	Type         string  `json:"type"`
	Value        float64 `json:"value"`
	// ApplyGroupRatio 仅对参考固定单价/固定总价（video_ref_price / video_ref_flat）生效：
	// false（默认）=绝对值，不受分组折扣影响；true=参考价也乘以分组倍率。
	ApplyGroupRatio bool `json:"apply_group_ratio,omitempty"`
	Disabled        bool `json:"disabled,omitempty"`
}

type UserPricingOverrideConfig struct {
	Rules []UserPricingRule `json:"rules"`
}

type UserPricingOverrideMatch struct {
	Rule        UserPricingRule `json:"rule"`
	Description string          `json:"description,omitempty"`
}

var userPricingOverrideConfig atomic.Value

func init() {
	userPricingOverrideConfig.Store(UserPricingOverrideConfig{Rules: []UserPricingRule{}})
}

func currentUserPricingOverrideConfig() UserPricingOverrideConfig {
	cfg, ok := userPricingOverrideConfig.Load().(UserPricingOverrideConfig)
	if !ok {
		return UserPricingOverrideConfig{Rules: []UserPricingRule{}}
	}
	return cfg
}

func UserPricingOverride2JSONString() string {
	jsonBytes, err := common.Marshal(currentUserPricingOverrideConfig())
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
	userPricingOverrideConfig.Store(normalizeUserPricingOverrideConfig(cfg))
	InvalidateExposedDataCache()
	return nil
}

func GetUserPricingOverrideCopy() UserPricingOverrideConfig {
	cfg := currentUserPricingOverrideConfig()
	cfg.Rules = append([]UserPricingRule(nil), cfg.Rules...)
	return cfg
}

func MatchUserPricingOverride(userID int, username, userGroup, usingGroup, modelName string) []UserPricingOverrideMatch {
	if userID <= 0 {
		return nil
	}
	var matches []UserPricingOverrideMatch
	cfg := currentUserPricingOverrideConfig()
	for _, rule := range cfg.Rules {
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

// UserPricingOverrideResult 携带命中的价格覆盖结果。
// VideoRefMode 为空表示未命中参考视频规则。
type UserPricingOverrideResult struct {
	UsePrice      bool
	ModelPrice    float64
	ModelRatio    float64
	GroupRatio    float64
	VideoRefMode  string  // "", factor, price, flat, cap
	VideoRefValue float64 // 对应模式的数值（倍率/单价/总价/封顶秒数）
	// VideoRefApplyGroupRatio 仅对 price/flat 生效：参考固定价是否也乘分组倍率。
	VideoRefApplyGroupRatio bool
	Matches                 []UserPricingOverrideMatch
}

func ApplyUserPricingOverrides(userID int, username, userGroup, usingGroup, modelName string, usePrice bool, modelPrice, modelRatio, groupRatio float64) UserPricingOverrideResult {
	result := UserPricingOverrideResult{
		UsePrice:   usePrice,
		ModelPrice: modelPrice,
		ModelRatio: modelRatio,
		GroupRatio: groupRatio,
	}
	matches := MatchUserPricingOverride(userID, username, userGroup, usingGroup, modelName)
	if len(matches) == 0 {
		return result
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

	// 参考视频规则：4 种模式互斥，取最高优先级的一条。
	bestRef, hasRef := bestReferenceMatch(matches)

	applied := make([]UserPricingOverrideMatch, 0, 3)
	if hasRatio {
		applied = append(applied, bestRatio)
	}
	if usePrice && hasPrice {
		applied = append(applied, bestPrice)
	} else if !usePrice && hasModelRatio {
		applied = append(applied, bestModelRatio)
	}
	if hasRef {
		result.VideoRefMode = referenceRuleMode(bestRef.Rule.Type)
		result.VideoRefValue = bestRef.Rule.Value
		result.VideoRefApplyGroupRatio = bestRef.Rule.ApplyGroupRatio
		applied = append(applied, bestRef)
	}

	result.UsePrice = usePrice
	result.ModelPrice = modelPrice
	result.ModelRatio = modelRatio
	result.GroupRatio = groupRatio
	result.Matches = applied
	return result
}

// bestReferenceMatch 在所有命中规则里挑出优先级最高的参考视频规则（4 种模式任一）。
func bestReferenceMatch(matches []UserPricingOverrideMatch) (UserPricingOverrideMatch, bool) {
	bestScore := -1
	var best UserPricingOverrideMatch
	for _, match := range matches {
		switch match.Rule.Type {
		case UserPricingRuleVideoRefFactor, UserPricingRuleVideoRefPrice, UserPricingRuleVideoRefFlat, UserPricingRuleVideoRefCap:
			score := matchPriority(match.Rule)
			if score > bestScore {
				bestScore = score
				best = match
			}
		}
	}
	return best, bestScore >= 0
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
	case UserPricingRuleVideoRefFactor, "video-ref-factor", "ref_factor", "参考秒倍率", "参考秒打折":
		return UserPricingRuleVideoRefFactor
	case UserPricingRuleVideoRefPrice, "video-ref-price", "ref_price", "参考秒单价", "参考固定单价":
		return UserPricingRuleVideoRefPrice
	case UserPricingRuleVideoRefFlat, "video-ref-flat", "ref_flat", "参考整段固定价", "参考固定总价":
		return UserPricingRuleVideoRefFlat
	case UserPricingRuleVideoRefCap, "video-ref-cap", "ref_cap", "参考秒封顶", "参考秒数封顶":
		return UserPricingRuleVideoRefCap
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

// GetUserGroupRatioOverride 返回用户在某分组上的个性倍率覆盖(仅匹配全模型的 ratio 规则;
// 带模型条件的规则无法折算成单一分组倍率,不参与)。供分组选择器等展示场景使用,
// 计费路径走 ApplyUserPricingOverrides,两边语义保持一致。
func GetUserGroupRatioOverride(userID int, username, userGroup, groupName string) (float64, bool) {
	matches := MatchUserPricingOverride(userID, username, userGroup, groupName, "")
	best, ok := bestUserPricingMatch(matches, UserPricingRuleRatio)
	if !ok {
		return 0, false
	}
	return best.Rule.Value, true
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
