package service

import (
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

type UserPricingContext struct {
	User        UserPricingContextUser                 `json:"user"`
	Groups      []UserPricingContextGroup              `json:"groups"`
	Models      []string                               `json:"models"`
	ModelPrices map[string]UserPricingContextModelInfo `json:"model_prices"`
}

type UserPricingContextUser struct {
	ID                int     `json:"id"`
	Username          string  `json:"username"`
	DisplayName       string  `json:"display_name,omitempty"`
	Group             string  `json:"group"`
	CurrentGroupRatio float64 `json:"current_group_ratio"`
}

type UserPricingContextGroup struct {
	Name   string   `json:"name"`
	Desc   string   `json:"desc"`
	Ratio  float64  `json:"ratio"`
	Models []string `json:"models"`
}

type UserPricingContextModelInfo struct {
	UsePrice bool    `json:"use_price"`
	Price    float64 `json:"price,omitempty"`
	Ratio    float64 `json:"ratio,omitempty"`
	Exists   bool    `json:"exists"`
}

func GetUserUsableGroups(userGroup string) map[string]string {
	groupsCopy := setting.GetUserUsableGroupsCopy()
	if userGroup != "" {
		specialSettings, b := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.Get(userGroup)
		if b {
			// 处理特殊可用分组
			for specialGroup, desc := range specialSettings {
				if strings.HasPrefix(specialGroup, "-:") {
					// 移除分组
					groupToRemove := strings.TrimPrefix(specialGroup, "-:")
					delete(groupsCopy, groupToRemove)
				} else if strings.HasPrefix(specialGroup, "+:") {
					// 添加分组
					groupToAdd := strings.TrimPrefix(specialGroup, "+:")
					groupsCopy[groupToAdd] = desc
				} else {
					// 直接添加分组
					groupsCopy[specialGroup] = desc
				}
			}
		}
		// 如果userGroup不在UserUsableGroups中，返回UserUsableGroups + userGroup
		if _, ok := groupsCopy[userGroup]; !ok {
			groupsCopy[userGroup] = "用户分组"
		}
	}
	return groupsCopy
}

func GroupInUserUsableGroups(userGroup, groupName string) bool {
	_, ok := GetUserUsableGroups(userGroup)[groupName]
	return ok
}

// GetUserAutoGroup 根据用户分组获取自动分组设置
func GetUserAutoGroup(userGroup string) []string {
	groups := GetUserUsableGroups(userGroup)
	autoGroups := make([]string, 0)
	for _, group := range setting.GetAutoGroups() {
		if _, ok := groups[group]; ok {
			autoGroups = append(autoGroups, group)
		}
	}
	return autoGroups
}

// GetUserGroupRatio 获取用户使用某个分组的倍率
// userGroup 用户分组
// group 需要获取倍率的分组
func GetUserGroupRatio(userGroup, group string) float64 {
	ratio, ok := ratio_setting.GetGroupGroupRatio(userGroup, group)
	if ok {
		return ratio
	}
	return ratio_setting.GetGroupRatio(group)
}

func GetUserPricingContext(user *model.User) UserPricingContext {
	usableGroups := GetUserUsableGroups(user.Group)
	groups := make([]UserPricingContextGroup, 0, len(usableGroups))
	allModels := make(map[string]bool)
	for groupName, desc := range usableGroups {
		models := model.GetGroupEnabledModels(groupName)
		sort.Strings(models)
		for _, modelName := range models {
			allModels[modelName] = true
		}
		groups = append(groups, UserPricingContextGroup{
			Name:   groupName,
			Desc:   desc,
			Ratio:  GetUserGroupRatio(user.Group, groupName),
			Models: models,
		})
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Name < groups[j].Name
	})
	models := make([]string, 0, len(allModels))
	for modelName := range allModels {
		models = append(models, modelName)
	}
	sort.Strings(models)
	modelPrices := make(map[string]UserPricingContextModelInfo, len(models))
	for _, modelName := range models {
		price, usePrice, exists := ratio_setting.GetModelRatioOrPrice(modelName)
		info := UserPricingContextModelInfo{
			UsePrice: usePrice,
			Exists:   exists,
		}
		if usePrice {
			info.Price = price
		} else {
			info.Ratio = price
		}
		modelPrices[modelName] = info
	}
	return UserPricingContext{
		User: UserPricingContextUser{
			ID:                user.Id,
			Username:          user.Username,
			DisplayName:       user.DisplayName,
			Group:             user.Group,
			CurrentGroupRatio: GetUserGroupRatio(user.Group, user.Group),
		},
		Groups:      groups,
		Models:      models,
		ModelPrices: modelPrices,
	}
}
