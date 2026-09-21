package helper

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const effortRatioTestGroup = "claudemax分组"

// setupEffortRatioPricing 配好:模型倍率 2、分组倍率全 1、claudemax分组 的 max 强度 ×2。
func setupEffortRatioPricing(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	savedConfig := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		savedConfig[key] = value
		return nil
	}))
	savedModelRatios := ratio_setting.ModelRatio2JSONString()
	savedEffortRatios := ratio_setting.GroupEffortRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(savedConfig))
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(savedModelRatios))
		require.NoError(t, ratio_setting.UpdateGroupEffortRatioByJSONString(savedEffortRatios))
	})

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"group_ratio_setting.group_ratio": `{"default":1,"` + effortRatioTestGroup + `":1}`,
	}))
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"effort-ratio-model":2}`))
	require.NoError(t, ratio_setting.UpdateGroupEffortRatioByJSONString(`{"`+effortRatioTestGroup+`":{"max":2}}`))
}

func newEffortRatioRelayInfo(group string, request dto.Request) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		OriginModelName: "effort-ratio-model",
		UserGroup:       group,
		UsingGroup:      group,
		Request:         request,
	}
}

func claudeRequestWithEffort(effort string) *dto.ClaudeRequest {
	req := &dto.ClaudeRequest{Model: "effort-ratio-model"}
	if effort != "" {
		req.OutputConfig = json.RawMessage(`{"effort":"` + effort + `"}`)
	}
	return req
}

func TestModelPriceHelperAppliesGroupEffortRatio(t *testing.T) {
	setupEffortRatioPricing(t)

	priceFor := func(t *testing.T, group string, request dto.Request) hosttypes.PriceData {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Set("group", group)
		priceData, err := ModelPriceHelper(ctx, newEffortRatioRelayInfo(group, request), 1000, &types.TokenCountMeta{})
		require.NoError(t, err)
		return priceData
	}

	baseline := priceFor(t, effortRatioTestGroup, claudeRequestWithEffort(""))
	require.Greater(t, baseline.QuotaToPreConsume, 0)
	_, has := baseline.GetOtherRatio(hosttypes.OtherRatioKeyEffort)
	require.False(t, has, "no effort in request → no surcharge")

	t.Run("max effort in configured group doubles pre-consume", func(t *testing.T) {
		priced := priceFor(t, effortRatioTestGroup, claudeRequestWithEffort("max"))
		ratio, ok := priced.GetOtherRatio(hosttypes.OtherRatioKeyEffort)
		require.True(t, ok)
		require.Equal(t, 2.0, ratio)
		require.Equal(t, baseline.QuotaToPreConsume*2, priced.QuotaToPreConsume)
		// 分组倍率本身不动:倍率是叠加的附加项,不是改写分组倍率
		require.Equal(t, baseline.GroupRatioInfo.GroupRatio, priced.GroupRatioInfo.GroupRatio)
	})

	t.Run("other effort levels are untouched", func(t *testing.T) {
		priced := priceFor(t, effortRatioTestGroup, claudeRequestWithEffort("high"))
		_, ok := priced.GetOtherRatio(hosttypes.OtherRatioKeyEffort)
		require.False(t, ok)
		require.Equal(t, baseline.QuotaToPreConsume, priced.QuotaToPreConsume)
	})

	t.Run("same effort in an unconfigured group is untouched", func(t *testing.T) {
		priced := priceFor(t, "default", claudeRequestWithEffort("max"))
		_, ok := priced.GetOtherRatio(hosttypes.OtherRatioKeyEffort)
		require.False(t, ok)
		require.Equal(t, baseline.QuotaToPreConsume, priced.QuotaToPreConsume)
	})

	t.Run("OpenAI-format reasoning_effort is honored too", func(t *testing.T) {
		priced := priceFor(t, effortRatioTestGroup, &dto.GeneralOpenAIRequest{Model: "effort-ratio-model", ReasoningEffort: "MAX"})
		ratio, ok := priced.GetOtherRatio(hosttypes.OtherRatioKeyEffort)
		require.True(t, ok)
		require.Equal(t, 2.0, ratio)
		require.Equal(t, baseline.QuotaToPreConsume*2, priced.QuotaToPreConsume)
	})
}

func TestRefreshGroupRatioForRetryRecomputesEffortRatio(t *testing.T) {
	setupEffortRatioPricing(t)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("group", "default")
	info := newEffortRatioRelayInfo("default", claudeRequestWithEffort("max"))
	_, err := ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{})
	require.NoError(t, err)
	_, ok := info.PriceData.GetOtherRatio(hosttypes.OtherRatioKeyEffort)
	require.False(t, ok, "default group has no rule")

	// 重试落到配置了倍率的分组:补上
	info.UsingGroup = effortRatioTestGroup
	RefreshGroupRatioForRetry(ctx, info)
	ratio, ok := info.PriceData.GetOtherRatio(hosttypes.OtherRatioKeyEffort)
	require.True(t, ok)
	require.Equal(t, 2.0, ratio)

	// 再调一次不叠乘
	RefreshGroupRatioForRetry(ctx, info)
	ratio, ok = info.PriceData.GetOtherRatio(hosttypes.OtherRatioKeyEffort)
	require.True(t, ok)
	require.Equal(t, 2.0, ratio)
	require.Equal(t, 2.0, info.PriceData.OtherRatioMultiplier())

	// 换回没有规则的分组:摘掉
	info.UsingGroup = "default"
	RefreshGroupRatioForRetry(ctx, info)
	_, ok = info.PriceData.GetOtherRatio(hosttypes.OtherRatioKeyEffort)
	require.False(t, ok)
	require.Equal(t, 1.0, info.PriceData.OtherRatioMultiplier())
}
