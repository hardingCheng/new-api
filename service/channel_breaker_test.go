package service

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestChannelBreakerOpensAfterFailureThreshold(t *testing.T) {
	common.SetChannelBreakerEnabled(true)
	disableRedisForBreakerTest(t)
	t.Cleanup(func() { common.SetChannelBreakerEnabled(false) })

	c := testBreakerContext("/v1/chat/completions")
	channelError := types.ChannelError{ChannelId: 1001, UsingKey: "key-a", AutoBan: true}
	ClearChannelBreaker(channelError)

	for i := 1; i < GetChannelBreakerFailureThreshold(); i++ {
		opened, _ := RecordChannelBreakerFailure(c, channelError, true)
		require.False(t, opened)
		require.True(t, AllowChannelByBreaker(c, channelError))
	}

	opened, _ := RecordChannelBreakerFailure(c, channelError, true)
	require.True(t, opened)
	require.False(t, AllowChannelByBreaker(c, channelError))
}

func TestChannelBreakerSuccessClearsFailures(t *testing.T) {
	common.SetChannelBreakerEnabled(true)
	disableRedisForBreakerTest(t)
	t.Cleanup(func() { common.SetChannelBreakerEnabled(false) })

	c := testBreakerContext("/v1/chat/completions")
	channelError := types.ChannelError{ChannelId: 1002, UsingKey: "key-a", AutoBan: true}
	ClearChannelBreaker(channelError)

	opened, _ := RecordChannelBreakerFailure(c, channelError, true)
	require.False(t, opened)
	RecordChannelBreakerSuccess(c, channelError)

	for i := 1; i < GetChannelBreakerFailureThreshold(); i++ {
		opened, _ = RecordChannelBreakerFailure(c, channelError, true)
		require.False(t, opened)
	}
}

func TestChannelBreakerSeparatesKeys(t *testing.T) {
	common.SetChannelBreakerEnabled(true)
	disableRedisForBreakerTest(t)
	t.Cleanup(func() { common.SetChannelBreakerEnabled(false) })

	c := testBreakerContext("/v1/chat/completions")
	keyA := types.ChannelError{ChannelId: 1003, UsingKey: "key-a", AutoBan: true}
	keyB := types.ChannelError{ChannelId: 1003, UsingKey: "key-b", AutoBan: true}
	ClearChannelBreaker(keyA)
	ClearChannelBreaker(keyB)

	for i := 0; i < GetChannelBreakerFailureThreshold(); i++ {
		RecordChannelBreakerFailure(c, keyA, true)
	}

	require.False(t, AllowChannelByBreaker(c, keyA))
	require.True(t, AllowChannelByBreaker(c, keyB))
}

func TestChannelBreakerExcludesVideos(t *testing.T) {
	common.SetChannelBreakerEnabled(true)
	disableRedisForBreakerTest(t)
	t.Cleanup(func() { common.SetChannelBreakerEnabled(false) })

	c := testBreakerContext("/v1/videos")
	channelError := types.ChannelError{ChannelId: 1004, UsingKey: "key-a", AutoBan: true}
	ClearChannelBreaker(channelError)

	for i := 0; i < GetChannelBreakerFailureThreshold(); i++ {
		opened, _ := RecordChannelBreakerFailure(c, channelError, true)
		require.False(t, opened)
	}
	require.True(t, AllowChannelByBreaker(c, channelError))
}

func TestChannelBreakerHalfOpenRestoresAfterMajoritySuccess(t *testing.T) {
	common.SetChannelBreakerEnabled(true)
	disableRedisForBreakerTest(t)
	oldCooldown := channelBreakerCooldown
	channelBreakerCooldown = time.Millisecond
	t.Cleanup(func() {
		common.SetChannelBreakerEnabled(false)
		channelBreakerCooldown = oldCooldown
	})

	c := testBreakerContext("/v1/chat/completions")
	channelError := types.ChannelError{ChannelId: 1005, UsingKey: "key-a", AutoBan: true}
	ClearChannelBreaker(channelError)

	for i := 0; i < GetChannelBreakerFailureThreshold(); i++ {
		RecordChannelBreakerFailure(c, channelError, true)
	}
	time.Sleep(2 * time.Millisecond)

	for i := 0; i < common.GetChannelBreakerProbeSuccessCount(); i++ {
		require.True(t, AllowChannelByBreaker(c, channelError))
		RecordChannelBreakerSuccess(c, channelError)
	}

	require.True(t, AllowChannelByBreaker(c, channelError))
}

func TestChannelBreakerHalfOpenRestoresWithMixedMajoritySuccess(t *testing.T) {
	common.SetChannelBreakerEnabled(true)
	disableRedisForBreakerTest(t)
	oldCooldown := channelBreakerCooldown
	channelBreakerCooldown = time.Millisecond
	t.Cleanup(func() {
		common.SetChannelBreakerEnabled(false)
		channelBreakerCooldown = oldCooldown
	})

	c := testBreakerContext("/v1/chat/completions")
	channelError := types.ChannelError{ChannelId: 1006, UsingKey: "key-a", AutoBan: true}
	ClearChannelBreaker(channelError)

	for i := 0; i < GetChannelBreakerFailureThreshold(); i++ {
		RecordChannelBreakerFailure(c, channelError, true)
	}
	time.Sleep(2 * time.Millisecond)

	for i := 0; i < 2; i++ {
		require.True(t, AllowChannelByBreaker(c, channelError))
		RecordChannelBreakerFailure(c, channelError, true)
	}
	for i := 0; i < 3; i++ {
		require.True(t, AllowChannelByBreaker(c, channelError))
		RecordChannelBreakerSuccess(c, channelError)
	}

	require.True(t, AllowChannelByBreaker(c, channelError))
}

func TestChannelBreakerCanUseDoesNotConsumeProbeSlot(t *testing.T) {
	common.SetChannelBreakerEnabled(true)
	disableRedisForBreakerTest(t)
	oldCooldown := channelBreakerCooldown
	channelBreakerCooldown = time.Millisecond
	t.Cleanup(func() {
		common.SetChannelBreakerEnabled(false)
		channelBreakerCooldown = oldCooldown
	})

	c := testBreakerContext("/v1/chat/completions")
	channelError := types.ChannelError{ChannelId: 1007, UsingKey: "key-a", AutoBan: true}
	ClearChannelBreaker(channelError)

	for i := 0; i < GetChannelBreakerFailureThreshold(); i++ {
		RecordChannelBreakerFailure(c, channelError, true)
	}
	time.Sleep(2 * time.Millisecond)

	for i := 0; i < common.GetChannelBreakerProbeCount()*2; i++ {
		require.True(t, CanUseChannelByBreaker(c, channelError))
	}
	for i := 0; i < common.GetChannelBreakerProbeCount(); i++ {
		require.True(t, AcquireChannelBreakerProbe(c, channelError))
	}
	require.False(t, AcquireChannelBreakerProbe(c, channelError))
}

func TestChannelBreakerIndependentFromAutomaticDisableSwitch(t *testing.T) {
	common.SetAutomaticDisableChannelEnabled(false)
	common.SetChannelBreakerEnabled(true)
	disableRedisForBreakerTest(t)
	t.Cleanup(func() {
		common.SetAutomaticDisableChannelEnabled(false)
		common.SetChannelBreakerEnabled(false)
	})

	c := testBreakerContext("/v1/chat/completions")
	channelError := types.ChannelError{ChannelId: 1008, UsingKey: "key-a", AutoBan: true}
	ClearChannelBreaker(channelError)

	for i := 0; i < GetChannelBreakerFailureThreshold(); i++ {
		RecordChannelBreakerFailure(c, channelError, true)
	}

	require.False(t, AllowChannelByBreaker(c, channelError))
}

func TestChannelBreakerDisabledSwitchDoesNotBlockChannel(t *testing.T) {
	common.SetChannelBreakerEnabled(false)
	disableRedisForBreakerTest(t)

	c := testBreakerContext("/v1/chat/completions")
	channelError := types.ChannelError{ChannelId: 1009, UsingKey: "key-a", AutoBan: true}
	ClearChannelBreaker(channelError)

	for i := 0; i < GetChannelBreakerFailureThreshold(); i++ {
		opened, message := RecordChannelBreakerFailure(c, channelError, true)
		require.False(t, opened)
		require.Empty(t, message)
	}

	require.True(t, AllowChannelByBreaker(c, channelError))
}

func TestListAndClearChannelBreakerStatuses(t *testing.T) {
	common.SetChannelBreakerEnabled(true)
	disableRedisForBreakerTest(t)
	t.Cleanup(func() { common.SetChannelBreakerEnabled(false) })

	c := testBreakerContext("/v1/chat/completions")
	channelError := types.ChannelError{ChannelId: 1010, UsingKey: "key-a", AutoBan: true}
	ClearChannelBreaker(channelError)

	for i := 0; i < GetChannelBreakerFailureThreshold(); i++ {
		RecordChannelBreakerFailure(c, channelError, true)
	}

	var matched *ChannelBreakerStatus
	for _, status := range ListChannelBreakerStatuses() {
		if status.ChannelId == 1010 && status.KeyHash == ChannelBreakerKeyHash("key-a") {
			statusCopy := status
			matched = &statusCopy
			break
		}
	}
	require.NotNil(t, matched)
	require.Equal(t, ChannelBreakerStateOpen, matched.State)
	require.True(t, ClearChannelBreakerByStateKey(matched.StateKey))
	for _, status := range ListChannelBreakerStatuses() {
		require.NotEqual(t, matched.StateKey, status.StateKey)
	}
}

func TestChannelBreakerGroupRuleOverridesDefault(t *testing.T) {
	common.SetChannelBreakerEnabled(true)
	common.SetChannelBreakerRules([]common.ChannelBreakerRule{
		{
			Id:                "vip-rule",
			Name:              "VIP规则",
			Enabled:           true,
			Scope:             common.ChannelBreakerScopeGroup,
			Targets:           []string{"vip"},
			FailureLimit:      2,
			CooldownSeconds:   60,
			ProbeCount:        5,
			ProbeSuccessCount: 3,
		},
	})
	disableRedisForBreakerTest(t)
	t.Cleanup(func() {
		common.SetChannelBreakerEnabled(false)
		common.SetChannelBreakerRules(nil)
	})

	c := testBreakerContext("/v1/chat/completions")
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "vip")
	channelError := types.ChannelError{ChannelId: 1011, UsingKey: "key-a", AutoBan: true}
	ClearChannelBreaker(channelError)

	opened, _ := RecordChannelBreakerFailure(c, channelError, true)
	require.False(t, opened)
	opened, _ = RecordChannelBreakerFailure(c, channelError, true)
	require.True(t, opened)

	var matched *ChannelBreakerStatus
	for _, status := range ListChannelBreakerStatuses() {
		if status.ChannelId == 1011 {
			statusCopy := status
			matched = &statusCopy
			break
		}
	}
	require.NotNil(t, matched)
	require.Equal(t, "vip-rule", matched.RuleId)
	require.Equal(t, "vip", matched.Group)
}

func TestChannelBreakerModelRuleCanDisableBreaker(t *testing.T) {
	common.SetChannelBreakerEnabled(true)
	common.SetChannelBreakerRules([]common.ChannelBreakerRule{
		{
			Id:             "video-disabled",
			Name:           "视频模型不熔断",
			Enabled:        true,
			Scope:          common.ChannelBreakerScopeModel,
			Targets:        []string{"grok-imagine-video"},
			DisableBreaker: true,
		},
	})
	disableRedisForBreakerTest(t)
	t.Cleanup(func() {
		common.SetChannelBreakerEnabled(false)
		common.SetChannelBreakerRules(nil)
	})

	c := testBreakerContext("/v1/chat/completions")
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "grok-imagine-video")
	channelError := types.ChannelError{ChannelId: 1012, UsingKey: "key-a", AutoBan: true}
	ClearChannelBreaker(channelError)

	for i := 0; i < GetChannelBreakerFailureThreshold()+2; i++ {
		opened, message := RecordChannelBreakerFailure(c, channelError, true)
		require.False(t, opened)
		require.Empty(t, message)
	}
	require.True(t, AllowChannelByBreaker(c, channelError))
}

func TestBuildChannelBreakerBarkBodyIncludesRequestDetails(t *testing.T) {
	body := buildChannelBreakerBarkBody(ChannelBreakerNotificationContext{
		ChannelError: types.ChannelError{
			ChannelId:   1020,
			ChannelName: "primary-openai",
			ChannelType: constant.ChannelTypeOpenAI,
			UsingKey:    "sk-test-key",
		},
		Reason:      "upstream returned 429",
		ModelName:   "gpt-4o-mini",
		Group:       "vip",
		UserId:      88,
		Username:    "alice",
		TokenId:     99,
		RequestPath: "/v1/chat/completions",
		StatusCode:  429,
	})

	require.Contains(t, body, "渠道：primary-openai (#1020)")
	require.Contains(t, body, "类型：OpenAI")
	require.Contains(t, body, "模型：gpt-4o-mini")
	require.Contains(t, body, "分组：vip")
	require.Contains(t, body, "用户：alice (#88)")
	require.Contains(t, body, "令牌：#99")
	require.Contains(t, body, "路径：/v1/chat/completions")
	require.Contains(t, body, "状态码：429")
	require.Contains(t, body, "密钥哈希："+ChannelBreakerKeyHash("sk-test-key"))
	require.Contains(t, body, "原因：upstream returned 429")
}

func TestLowBalanceThresholdQuotaUsesCNYThreshold(t *testing.T) {
	oldQuotaPerUnit := common.QuotaPerUnit
	oldExchangeRate := operation_setting.USDExchangeRate
	common.QuotaPerUnit = 500000
	operation_setting.USDExchangeRate = 7.3
	t.Cleanup(func() {
		common.QuotaPerUnit = oldQuotaPerUnit
		operation_setting.USDExchangeRate = oldExchangeRate
	})

	require.Equal(t, 684932, lowBalanceThresholdQuota(10))
	require.Zero(t, lowBalanceThresholdQuota(0))
}

func TestShouldInstantDisableChannel(t *testing.T) {
	common.SetChannelBreakerEnabled(true)
	common.SetChannelBreakerRules([]common.ChannelBreakerRule{
		{
			Id:                        "instant-balance",
			Name:                      "上游余额不足立即禁用",
			Enabled:                   true,
			Scope:                     common.ChannelBreakerScopeGlobal,
			InstantDisableEnabled:     true,
			InstantDisableStatusCodes: "403",
			InstantDisableKeywords:    "insufficient account balance\n预扣费额度失败",
		},
	})
	disableRedisForBreakerTest(t)
	t.Cleanup(func() {
		common.SetChannelBreakerEnabled(false)
		common.SetChannelBreakerRules(nil)
	})

	c := testBreakerContext("/v1/chat/completions")
	channelError := types.ChannelError{ChannelId: 2001, UsingKey: "key-a", AutoBan: true}

	// 上游返回的 403 余额不足：状态码 AND 关键词命中 -> 立即禁用
	upstreamErr := types.NewErrorWithStatusCode(
		errors.New("Insufficient account balance"), types.ErrorCodeBadResponse, http.StatusForbidden)
	should, _, rule := shouldInstantDisableChannel(c, channelError, upstreamErr)
	require.True(t, should)
	require.Equal(t, "instant-balance", rule.Id)

	// 关键安全约束：我方预扣费/用户额度不足带 skipRetry，绝不能触发禁用
	ownQuotaErr := types.NewErrorWithStatusCode(
		errors.New("预扣费额度失败, 用户剩余额度: ¥0.06"), types.ErrorCodeInsufficientUserQuota,
		http.StatusForbidden, types.ErrOptionWithSkipRetry())
	should, _, _ = shouldInstantDisableChannel(c, channelError, ownQuotaErr)
	require.False(t, should)

	// AND 语义：状态码命中但关键词未命中 -> 不触发
	otherErr := types.NewErrorWithStatusCode(
		errors.New("Permission denied"), types.ErrorCodeBadResponse, http.StatusForbidden)
	should, _, _ = shouldInstantDisableChannel(c, channelError, otherErr)
	require.False(t, should)

	// 关键词命中但状态码不在范围 -> 不触发
	wrongCodeErr := types.NewErrorWithStatusCode(
		errors.New("Insufficient account balance"), types.ErrorCodeBadResponse, http.StatusTooManyRequests)
	should, _, _ = shouldInstantDisableChannel(c, channelError, wrongCodeErr)
	require.False(t, should)

	// 余额耗尽是终态：即使渠道未开启 AutoBan 也强制立即禁用
	noBan := types.ChannelError{ChannelId: 2001, UsingKey: "key-a", AutoBan: false}
	should, _, _ = shouldInstantDisableChannel(c, noBan, upstreamErr)
	require.True(t, should)
}

// TestInstantDisableBuiltinDefault 覆盖生产默认态：
// 全局熔断开关 OFF、且没有任何自定义规则时，纯内置规则仍应对 403 余额不足生效。
func TestInstantDisableBuiltinDefault(t *testing.T) {
	common.SetChannelBreakerEnabled(false)
	common.SetChannelBreakerRules(nil)
	disableRedisForBreakerTest(t)

	c := testBreakerContext("/v1/chat/completions")
	channelError := types.ChannelError{ChannelId: 3001, UsingKey: "key-a", AutoBan: false}

	upstreamErr := types.NewErrorWithStatusCode(
		errors.New("Insufficient account balance"), types.ErrorCodeBadResponse, http.StatusForbidden)
	should, _, rule := shouldInstantDisableChannel(c, channelError, upstreamErr)
	require.True(t, should)
	require.Equal(t, "global-default", rule.Id)

	// 我方扣费失败带 skipRetry，内置规则也不能误触发
	ownQuotaErr := types.NewErrorWithStatusCode(
		errors.New("预扣费额度失败, 用户剩余额度: ¥0.06"), types.ErrorCodeInsufficientUserQuota,
		http.StatusForbidden, types.ErrOptionWithSkipRetry())
	should, _, _ = shouldInstantDisableChannel(c, channelError, ownQuotaErr)
	require.False(t, should)

	// 结构化兜底：我方额度错误即使漏标 skipRetry（NewAPIError + 内部额度码），仍须排除
	ownQuotaNoSkip := types.NewErrorWithStatusCode(
		errors.New("预扣费额度失败, 用户剩余额度: ¥0.06"), types.ErrorCodeInsufficientUserQuota,
		http.StatusForbidden)
	should, _, _ = shouldInstantDisableChannel(c, channelError, ownQuotaNoSkip)
	require.False(t, should)

	// 上游本身是 new-api、账号没钱：错误码同为 insufficient_user_quota，但 type 是上游错误，
	// 不应被结构化兜底误伤，仍须正常立即禁用
	upstreamNewApiErr := types.WithOpenAIError(types.OpenAIError{
		Message: "预扣费额度失败, 用户剩余额度: ¥0",
		Type:    string(types.ErrorCodeInsufficientUserQuota),
		Code:    string(types.ErrorCodeInsufficientUserQuota),
	}, http.StatusForbidden)
	should, _, _ = shouldInstantDisableChannel(c, channelError, upstreamNewApiErr)
	require.True(t, should)
}

func TestInstantDisableTriggersOnFirstHit(t *testing.T) {
	common.SetChannelBreakerEnabled(true)
	common.SetChannelBreakerRules([]common.ChannelBreakerRule{
		{
			Id:                        "instant-balance",
			Name:                      "上游余额不足立即禁用",
			Enabled:                   true,
			Scope:                     common.ChannelBreakerScopeGlobal,
			FailureLimit:              5,
			InstantDisableEnabled:     true,
			InstantDisableStatusCodes: "403",
			InstantDisableKeywords:    "insufficient account balance",
		},
	})
	disableRedisForBreakerTest(t)
	t.Cleanup(func() {
		common.SetChannelBreakerEnabled(false)
		common.SetChannelBreakerRules(nil)
	})

	c := testBreakerContext("/v1/chat/completions")
	channelError := types.ChannelError{ChannelId: 2002, UsingKey: "key-a", AutoBan: true}
	ClearChannelBreaker(channelError)

	balanceErr := types.NewErrorWithStatusCode(
		errors.New("Insufficient account balance"), types.ErrorCodeBadResponse, http.StatusForbidden)

	// 第一次命中即判定立即禁用，无需累加到 FailureLimit(5)
	should, _, _ := shouldInstantDisableChannel(c, channelError, balanceErr)
	require.True(t, should, "余额不足应在第一次命中即触发立即禁用")

	// 对照：同样的错误若走普通熔断计数，第一次只会 pending、不会 opened
	opened, _ := RecordChannelBreakerFailure(c, channelError, true)
	require.False(t, opened, "普通熔断第一次不会打开，需累加到阈值")
}

func testBreakerContext(path string) *gin.Context {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, path, nil)
	return c
}

func disableRedisForBreakerTest(t *testing.T) {
	t.Helper()
	oldRedisEnabled := common.RedisEnabled
	oldRDB := common.RDB
	common.RedisEnabled = false
	common.RDB = nil
	t.Cleanup(func() {
		common.RedisEnabled = oldRedisEnabled
		common.RDB = oldRDB
	})
}
