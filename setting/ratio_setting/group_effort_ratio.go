package ratio_setting

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/types"
)

// GroupEffortRatio:按「计费分组 → 推理强度 → 倍率」给整笔账单再乘一个倍率。
//
// 用途:有的分组走的上游按推理强度差异化收费(例如 max 强度成本翻倍),
// 这里把差价传导给客户,而不必为每档强度另起一个模型名。
// 形如 {"claudemax分组":{"max":2}};强度键不区分大小写,缺省为空 = 不加价。
// 倍率落在 PriceData 的附加倍率里(键 types.OtherRatioKeyEffort),预扣与结算同源。
var groupEffortRatioMap = types.NewRWMap[string, map[string]float64]()

func normalizeEffortLevel(effort string) string {
	return strings.ToLower(strings.TrimSpace(effort))
}

// GetGroupEffortRatio 返回分组 usingGroup 下强度 effort 命中的倍率;未配置返回 (0, false)。
func GetGroupEffortRatio(usingGroup, effort string) (float64, bool) {
	level := normalizeEffortLevel(effort)
	if level == "" {
		return 0, false
	}
	ratios, ok := groupEffortRatioMap.Get(usingGroup)
	if !ok {
		return 0, false
	}
	for configured, ratio := range ratios {
		if normalizeEffortLevel(configured) == level {
			return ratio, true
		}
	}
	return 0, false
}

func GroupEffortRatio2JSONString() string {
	return groupEffortRatioMap.MarshalJSONString()
}

func UpdateGroupEffortRatioByJSONString(jsonStr string) error {
	if strings.TrimSpace(jsonStr) == "" {
		jsonStr = "{}"
	}
	if err := CheckGroupEffortRatio(jsonStr); err != nil {
		return err
	}
	return types.LoadFromJsonString(groupEffortRatioMap, jsonStr)
}

// CheckGroupEffortRatio 校验配置必须是 分组 → {强度: 倍率} 两层 JSON,且倍率 > 0。
// 0 或负数会被附加倍率机制静默忽略,配置看着生效实际没生效,所以在入口直接拒绝。
func CheckGroupEffortRatio(jsonStr string) error {
	var cfg map[string]map[string]float64
	if err := json.Unmarshal([]byte(jsonStr), &cfg); err != nil {
		return err
	}
	for group, ratios := range cfg {
		seen := make(map[string]string, len(ratios))
		for level, ratio := range ratios {
			normalized := normalizeEffortLevel(level)
			if normalized == "" {
				return fmt.Errorf("group effort ratio: empty effort level under group %q", group)
			}
			if previous, dup := seen[normalized]; dup {
				return fmt.Errorf("group effort ratio: effort levels %q and %q under group %q collide (matching ignores case)", previous, level, group)
			}
			seen[normalized] = level
			if !(ratio > 0) || math.IsInf(ratio, 0) {
				return fmt.Errorf("group effort ratio must be greater than 0: %s/%s", group, level)
			}
		}
	}
	return nil
}
