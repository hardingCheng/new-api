package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

func filterPricingByUsableGroups(pricing []model.Pricing, usableGroup map[string]string) []model.Pricing {
	if len(pricing) == 0 {
		return pricing
	}
	if len(usableGroup) == 0 {
		return []model.Pricing{}
	}

	filtered := make([]model.Pricing, 0, len(pricing))
	for _, item := range pricing {
		if common.StringsContains(item.EnableGroup, "all") {
			filtered = append(filtered, item)
			continue
		}
		for _, group := range item.EnableGroup {
			if _, ok := usableGroup[group]; ok {
				filtered = append(filtered, item)
				break
			}
		}
	}
	return filtered
}

func pricingGroupsForOverrides(item model.Pricing, groupRatio map[string]float64) []string {
	if common.StringsContains(item.EnableGroup, "all") {
		groups := make([]string, 0, len(groupRatio))
		for group := range groupRatio {
			groups = append(groups, group)
		}
		return groups
	}
	return item.EnableGroup
}

func applyUserPricingToPricingList(pricing []model.Pricing, user *model.UserBase, groupRatio map[string]float64) []model.Pricing {
	if user == nil || len(pricing) == 0 || len(groupRatio) == 0 {
		return pricing
	}
	if len(ratio_setting.GetUserPricingOverrideCopy().Rules) == 0 {
		return pricing
	}

	result := make([]model.Pricing, len(pricing))
	copy(result, pricing)
	for i := range result {
		item := &result[i]
		if item.BillingMode == "tiered_expr" {
			continue
		}
		billingModelName := item.ModelName
		if alias, matched := model_setting.ResolveUserModelAlias(user.Id, item.ModelName); matched {
			billingModelName = alias.TargetModel
		}
		groupOverrides := make(map[string]model.PricingUserPricingGroup)
		for _, group := range pricingGroupsForOverrides(*item, groupRatio) {
			baseGroupRatio, ok := groupRatio[group]
			if !ok {
				continue
			}
			usePrice := item.QuotaType == 1
			override := ratio_setting.ApplyUserPricingOverrides(
				user.Id,
				user.Username,
				user.Group,
				group,
				billingModelName,
				usePrice,
				item.ModelPrice,
				item.ModelRatio,
				baseGroupRatio,
			)
			if len(override.Matches) == 0 {
				continue
			}
			groupOverrides[group] = model.PricingUserPricingGroup{
				UsePrice:   override.UsePrice,
				ModelPrice: override.ModelPrice,
				ModelRatio: override.ModelRatio,
				GroupRatio: override.GroupRatio,
			}
		}
		if len(groupOverrides) > 0 {
			item.UserPricing = &model.PricingUserPricing{Groups: groupOverrides}
		}
	}
	return result
}

func applyUserModelViewToPricing(pricing []model.Pricing, userID int) []model.Pricing {
	if userID <= 0 || len(pricing) == 0 {
		return pricing
	}
	modelNames := make([]string, 0, len(pricing))
	pricingByModel := make(map[string]model.Pricing, len(pricing))
	for _, item := range pricing {
		modelNames = append(modelNames, item.ModelName)
		pricingByModel[item.ModelName] = item
	}
	visibleModels := model_setting.BuildVisibleUserModels(userID, modelNames)
	if len(visibleModels) == len(pricing) {
		unchanged := true
		for index, visibleModel := range visibleModels {
			if visibleModel.Name != pricing[index].ModelName {
				unchanged = false
				break
			}
		}
		if unchanged {
			return pricing
		}
	}

	result := make([]model.Pricing, 0, len(visibleModels))
	for _, visibleModel := range visibleModels {
		item, exists := pricingByModel[visibleModel.TargetModel]
		if !exists {
			continue
		}
		item.ModelName = visibleModel.Name
		item.EnableGroup = append([]string(nil), item.EnableGroup...)
		item.SupportedEndpointTypes = append([]constant.EndpointType(nil), item.SupportedEndpointTypes...)
		item.UserPricing = nil
		result = append(result, item)
	}
	return result
}

func GetPricing(c *gin.Context) {
	pricing := model.GetPricing()
	userId, exists := c.Get("id")
	usableGroup := map[string]string{}
	groupRatio := map[string]float64{}
	for s, f := range ratio_setting.GetGroupRatioCopy() {
		groupRatio[s] = f
	}
	var group string
	var user *model.UserBase
	if exists {
		userCache, err := model.GetUserCache(userId.(int))
		if err == nil {
			user = userCache
			group = user.Group
			for g := range groupRatio {
				ratio, ok := ratio_setting.GetGroupGroupRatio(group, g)
				if ok {
					groupRatio[g] = ratio
				}
			}
		}
	}

	usableGroup = service.GetUserUsableGroups(group)
	pricing = filterPricingByUsableGroups(pricing, usableGroup)
	if user != nil {
		pricing = applyUserModelViewToPricing(pricing, user.Id)
	}
	// check groupRatio contains usableGroup
	for group := range ratio_setting.GetGroupRatioCopy() {
		if _, ok := usableGroup[group]; !ok {
			delete(groupRatio, group)
		}
	}
	pricing = applyUserPricingToPricingList(pricing, user, groupRatio)

	c.JSON(200, gin.H{
		"success":            true,
		"data":               pricing,
		"vendors":            model.GetVendors(),
		"group_ratio":        groupRatio,
		"usable_group":       usableGroup,
		"supported_endpoint": model.GetSupportedEndpointMap(),
		"auto_groups":        service.GetUserAutoGroup(group),
		"pricing_version":    "a42d372ccf0b5dd13ecf71203521f9d2",
	})
}

func ResetModelRatio(c *gin.Context) {
	defaultStr := ratio_setting.DefaultModelRatio2JSONString()
	err := model.UpdateOption("ModelRatio", defaultStr)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	err = ratio_setting.UpdateModelRatioByJSONString(defaultStr)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"success": true,
		"message": "重置模型倍率成功",
	})
}
