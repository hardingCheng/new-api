package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyUserPricingToPricingList(t *testing.T) {
	original := ratio_setting.GetUserPricingOverrideCopy()
	originalJSON, err := common.Marshal(original)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateUserPricingOverrideByJSONString(string(originalJSON)))
	})

	overrideJSON := `{
		"rules": [
			{"user_id": 7, "group_pattern": "vip", "model_pattern": "per-call-model", "type": "model_price", "value": 0},
			{"user_id": 7, "group_pattern": "default", "model_pattern": "ratio-model", "type": "model_ratio", "value": 0},
			{"user_id": 7, "group_pattern": "discount", "model_pattern": "ratio-model", "type": "ratio", "value": 0.25},
			{"user_id": 7, "group_pattern": "vip", "model_pattern": "per-call-to-ratio", "type": "model_ratio", "value": 3}
		]
	}`
	require.NoError(t, ratio_setting.UpdateUserPricingOverrideByJSONString(overrideJSON))

	pricing := []model.Pricing{
		{
			ModelName:   "per-call-model",
			QuotaType:   1,
			ModelPrice:  2,
			EnableGroup: []string{"vip", "default"},
		},
		{
			ModelName:   "ratio-model",
			QuotaType:   0,
			ModelRatio:  1.5,
			EnableGroup: []string{"default", "discount"},
		},
		{
			ModelName:   "per-call-to-ratio",
			QuotaType:   1,
			ModelPrice:  4,
			EnableGroup: []string{"vip"},
		},
		{
			ModelName:   "tiered-model",
			QuotaType:   0,
			ModelRatio:  1,
			BillingMode: "tiered_expr",
			EnableGroup: []string{"vip"},
		},
	}
	groupRatio := map[string]float64{
		"default":  1,
		"discount": 0.8,
		"vip":      1.2,
	}
	user := &model.UserBase{Id: 7, Username: "alice", Group: "member"}

	result := applyUserPricingToPricingList(pricing, user, groupRatio)

	require.Nil(t, pricing[0].UserPricing, "base pricing cache slice must not be mutated")

	perCall := result[0].UserPricing.Groups["vip"]
	require.True(t, perCall.UsePrice)
	require.Equal(t, 0.0, perCall.ModelPrice)
	require.Equal(t, 1.2, perCall.GroupRatio)

	ratioDefault := result[1].UserPricing.Groups["default"]
	require.False(t, ratioDefault.UsePrice)
	require.Equal(t, 0.0, ratioDefault.ModelRatio)
	require.Equal(t, 1.0, ratioDefault.GroupRatio)

	ratioDiscount := result[1].UserPricing.Groups["discount"]
	require.False(t, ratioDiscount.UsePrice)
	require.Equal(t, 1.5, ratioDiscount.ModelRatio)
	require.Equal(t, 0.25, ratioDiscount.GroupRatio)

	switched := result[2].UserPricing.Groups["vip"]
	require.False(t, switched.UsePrice)
	require.Equal(t, 3.0, switched.ModelRatio)
	require.Equal(t, -1.0, switched.ModelPrice)

	require.Nil(t, result[3].UserPricing, "tiered expression pricing is not overridden in the runtime billing path")
}

func TestUserModelViewPricingInheritsTargetModelOverride(t *testing.T) {
	originalViewJSON, err := common.Marshal(model_setting.GetUserModelViewCopy())
	require.NoError(t, err)
	originalPricingJSON, err := common.Marshal(ratio_setting.GetUserPricingOverrideCopy())
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, model_setting.UpdateUserModelViewByJSONString(string(originalViewJSON)))
		require.NoError(t, ratio_setting.UpdateUserPricingOverrideByJSONString(string(originalPricingJSON)))
	})

	require.NoError(t, model_setting.UpdateUserModelViewByJSONString(`{
		"rules": [{
			"user_id": 7,
			"aliases": [
				{"public_model":"521ai-2.0-720p","target_model":"seedance-2.0-720p","reference_video":"forbidden"}
			]
		}]
	}`))
	require.NoError(t, ratio_setting.UpdateUserPricingOverrideByJSONString(`{
		"rules": [{
			"user_id": 7,
			"group_pattern": "sd2",
			"model_pattern": "seedance-2.0-720p",
			"type": "model_price",
			"value": 0.75
		}]
	}`))

	basePricing := []model.Pricing{
		{
			ModelName:              "seedance-2.0-720p",
			QuotaType:              1,
			ModelPrice:             2,
			EnableGroup:            []string{"sd2"},
			SupportedEndpointTypes: []constant.EndpointType{constant.EndpointTypeOpenAIVideo},
		},
	}
	pricing := applyUserModelViewToPricing(basePricing, 7)
	require.Len(t, pricing, 1)
	assert.Equal(t, "521ai-2.0-720p", pricing[0].ModelName)
	assert.Equal(t, []constant.EndpointType{constant.EndpointTypeOpenAIVideo}, pricing[0].SupportedEndpointTypes)
	regularPricing := applyUserModelViewToPricing(basePricing, 8)
	require.Len(t, regularPricing, 1)
	assert.Equal(t, "seedance-2.0-720p", regularPricing[0].ModelName)

	result := applyUserPricingToPricingList(
		pricing,
		&model.UserBase{Id: 7, Username: "special-user", Group: "sd2"},
		map[string]float64{"sd2": 1},
	)
	require.NotNil(t, result[0].UserPricing)
	override := result[0].UserPricing.Groups["sd2"]
	assert.True(t, override.UsePrice)
	assert.Equal(t, 0.75, override.ModelPrice)
}
