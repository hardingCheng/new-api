package ratio_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 重试路径会重算分组倍率后**重新**套一遍用户价格覆盖
// (relay/helper.RefreshGroupRatioForRetry),所以覆盖必须是幂等的:
// 每个分支都要用规则值绝对赋值,而不是在传入值上叠乘。
// 一旦有人把某个分支改成累乘,重试就会把客户的折扣打两次 —— 比忘记打折更糟,
// 因为它是少收钱且不报错。这个测试就是钉住这条性质。
func TestApplyUserPricingOverridesIsIdempotent(t *testing.T) {
	require.NoError(t, UpdateUserPricingOverrideByJSONString(`{"rules":[
		{"user_id":42,"type":"ratio","value":0.66},
		{"user_id":43,"type":"model_ratio","value":3,"model_pattern":"gpt-*"},
		{"user_id":44,"type":"model_price","value":0.02,"model_pattern":"gpt-*"}
	]}`))
	t.Cleanup(func() {
		_ = UpdateUserPricingOverrideByJSONString(`{"rules":[]}`)
	})

	cases := []struct {
		name    string
		userID  int
		model   string
		usePric bool
		price   float64
		ratio   float64
		group   float64
	}{
		{name: "overall ratio", userID: 42, model: "gpt-5.4-mini", price: -1, ratio: 0.5, group: 1.0},
		{name: "model ratio", userID: 43, model: "gpt-5.4-mini", price: -1, ratio: 0.5, group: 1.0},
		{name: "model price", userID: 44, model: "gpt-5.4-mini", usePric: true, price: 0.01, ratio: 0, group: 1.0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			first := ApplyUserPricingOverrides(
				tc.userID, "", "tf", "tf", tc.model, tc.usePric, tc.price, tc.ratio, tc.group)
			require.NotEmpty(t, first.Matches, "规则应该命中,否则这个用例没在测幂等")

			// 把第一次的输出当输入再跑一次 —— 这正是重试路径发生的事。
			second := ApplyUserPricingOverrides(
				tc.userID, "", "tf", "tf", tc.model,
				first.UsePrice, first.ModelPrice, first.ModelRatio, first.GroupRatio)

			assert.Equal(t, first.GroupRatio, second.GroupRatio, "分组倍率被叠乘了")
			assert.Equal(t, first.ModelPrice, second.ModelPrice, "模型单价被叠乘了")
			assert.Equal(t, first.ModelRatio, second.ModelRatio, "模型倍率被叠乘了")
			assert.Equal(t, first.UsePrice, second.UsePrice)
		})
	}
}

// 带 group_pattern 的规则**跟着客户的注册分组走** —— pattern 命中 userGroup 或
// usingGroup 任一即生效(matchUserPricingGroup)。这正是跨分组重试时需要的语义:
// 重试可能把请求路由到别的分组,折扣要跟着客户而不是跟着路由。
// 只有客户本身不属于该分组、且请求也没走该分组时,规则才不该命中。
func TestApplyUserPricingOverridesGroupScopeFollowsCustomer(t *testing.T) {
	require.NoError(t, UpdateUserPricingOverrideByJSONString(`{"rules":[
		{"user_id":55,"type":"ratio","value":0.66,"group_pattern":"tf"}
	]}`))
	t.Cleanup(func() {
		_ = UpdateUserPricingOverrideByJSONString(`{"rules":[]}`)
	})

	// 客户在 tf、请求也走 tf:命中
	inTf := ApplyUserPricingOverrides(55, "", "tf", "tf", "gpt-5.4-mini", false, -1, 0.5, 1.0)
	assert.NotEmpty(t, inTf.Matches)
	assert.Equal(t, 0.66, inTf.GroupRatio)

	// 客户在 tf、但重试把请求路由到别的分组:仍命中(折扣跟人)
	retried := ApplyUserPricingOverrides(55, "", "tf", "other", "gpt-5.4-mini", false, -1, 0.5, 2.0)
	assert.NotEmpty(t, retried.Matches, "跨分组重试后折扣不该丢")
	assert.Equal(t, 0.66, retried.GroupRatio)

	// 客户不在 tf、请求也不走 tf:不命中,分组倍率保持传入值
	unrelated := ApplyUserPricingOverrides(55, "", "hz", "hz", "gpt-5.4-mini", false, -1, 0.5, 2.0)
	assert.Empty(t, unrelated.Matches)
	assert.Equal(t, 2.0, unrelated.GroupRatio)
}
