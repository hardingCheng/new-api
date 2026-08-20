package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupBackoffTest(t *testing.T) {
	t.Helper()
	common.SetChannelBreakerBackoffEnabled(true)
	common.SetChannelBreakerBackoffMultipliers("1,2,5,15,60")
	common.SetChannelBreakerBackoffMaxCooldownSeconds(3600)
	common.SetChannelBreakerBackoffDecaySeconds(600)
	t.Cleanup(func() {
		common.SetChannelBreakerBackoffEnabled(false)
		common.SetChannelBreakerBackoffMultipliers("1,2,5,15,60")
		common.SetChannelBreakerBackoffMaxCooldownSeconds(3600)
		common.SetChannelBreakerBackoffDecaySeconds(600)
	})
}

func backoffTestRule(cooldownSecs int) channelBreakerRuntimeRule {
	rule := defaultChannelBreakerRuntimeRule()
	rule.Cooldown = time.Duration(cooldownSecs) * time.Second
	return rule
}

// reopenAfterRelease 模拟真实重开时序：下一次打开只可能发生在
// 上一次冷却结束（解除）之后，这里取解除后 quietGap 时刻。
func reopenAfterRelease(state *channelBreakerState, lastOpen time.Time, quietGap time.Duration, rule channelBreakerRuntimeRule) time.Time {
	next := lastOpen.Add(time.Duration(state.LastCooldownSecs)*time.Second + quietGap)
	openBreakerAt(state, next, rule)
	return next
}

func TestChannelBreakerBackoffEscalatesAndCaps(t *testing.T) {
	setupBackoffTest(t)
	rule := backoffTestRule(60)
	state := &channelBreakerState{}
	t0 := time.Unix(1700000000, 0)

	openBreakerAt(state, t0, rule)
	require.Equal(t, 60, state.CooldownSecs)

	// 每次都在解除后 30 秒（衰减窗内）再次打开——高档位冷却（900s/3600s）
	// 本身超过衰减窗 600s，衰减锚必须是解除时刻而不是打开时刻，否则到不了这里
	lastOpen := t0
	expected := []int{120, 300, 900, 3600, 3600}
	for i, want := range expected {
		lastOpen = reopenAfterRelease(state, lastOpen, 30*time.Second, rule)
		assert.Equal(t, want, state.CooldownSecs, "第 %d 次连续打开", i+2)
		assert.Equal(t, i+2, state.ConsecutiveOpens)
	}
}

func TestChannelBreakerBackoffDecayResetsAfterQuietWindow(t *testing.T) {
	setupBackoffTest(t)
	rule := backoffTestRule(60)
	state := &channelBreakerState{}
	t0 := time.Unix(1700000000, 0)

	openBreakerAt(state, t0, rule)
	lastOpen := reopenAfterRelease(state, t0, 30*time.Second, rule)
	lastOpen = reopenAfterRelease(state, lastOpen, 30*time.Second, rule)
	require.Equal(t, 300, state.CooldownSecs)

	// 解除后安静超过衰减窗（600s），连击归零，重新从初值开始
	reopenAfterRelease(state, lastOpen, 601*time.Second, rule)
	assert.Equal(t, 60, state.CooldownSecs)
	assert.Equal(t, 1, state.ConsecutiveOpens)
}

func TestChannelBreakerBackoffQuietGapWithinWindowKeepsStreak(t *testing.T) {
	setupBackoffTest(t)
	rule := backoffTestRule(60)
	state := &channelBreakerState{}
	t0 := time.Unix(1700000000, 0)

	openBreakerAt(state, t0, rule)
	openBreakerAt(state, t0.Add(3600*time.Second), rule)
	// 上一档冷却 60s + 衰减窗 600s = 660s 内的重开都算连续；
	// 3600s 早已超窗 → 已在上一步归零重计，此处验证从头爬梯
	assert.Equal(t, 1, state.ConsecutiveOpens)

	// 解除后 599 秒（差 1 秒到窗）再开，连击保持
	reopenAfterRelease(state, t0.Add(3600*time.Second), 599*time.Second, rule)
	assert.Equal(t, 2, state.ConsecutiveOpens)
	assert.Equal(t, 120, state.CooldownSecs)
}

func TestChannelBreakerBackoffCustomBaseAndCap(t *testing.T) {
	setupBackoffTest(t)
	common.SetChannelBreakerBackoffMaxCooldownSeconds(400)
	rule := backoffTestRule(120)
	state := &channelBreakerState{}
	t0 := time.Unix(1700000000, 0)

	openBreakerAt(state, t0, rule)
	require.Equal(t, 120, state.CooldownSecs)
	lastOpen := reopenAfterRelease(state, t0, 30*time.Second, rule)
	assert.Equal(t, 240, state.CooldownSecs)
	reopenAfterRelease(state, lastOpen, 30*time.Second, rule)
	assert.Equal(t, 400, state.CooldownSecs)
}

func TestChannelBreakerBackoffDisabledKeepsCurrentBehavior(t *testing.T) {
	common.SetChannelBreakerBackoffEnabled(false)
	rule := backoffTestRule(60)
	state := &channelBreakerState{CooldownSecs: 77}
	t0 := time.Unix(1700000000, 0)

	openBreakerAt(state, t0, rule)
	assert.Equal(t, 77, state.CooldownSecs)
	assert.Zero(t, state.ConsecutiveOpens)
	assert.True(t, state.LastOpenAt.IsZero())
}

func TestChannelBreakerBackoffFieldsSurviveOpenReset(t *testing.T) {
	setupBackoffTest(t)
	rule := backoffTestRule(60)
	state := &channelBreakerState{Failures: 5, ProbeTotal: 3, ProbeSuccess: 1}
	t0 := time.Unix(1700000000, 0)

	openBreakerAt(state, t0, rule)
	// openBreakerAt 清短期字段，但不得清退避的长期记忆
	assert.Zero(t, state.Failures)
	assert.Zero(t, state.ProbeTotal)
	assert.Equal(t, 1, state.ConsecutiveOpens)
	assert.Equal(t, t0, state.LastOpenAt)
}

func TestChannelBreakerBackoffExtendsProbeWindow(t *testing.T) {
	setupBackoffTest(t)
	rule := backoffTestRule(60)
	now := time.Unix(1700000000, 0)
	state := &channelBreakerState{
		State:          ChannelBreakerStateHalfOpen,
		ProbeStartedAt: now.Add(-500 * time.Second),
	}

	// 探测窗口 = max(cooldown, 30s)：冷却被退避拉长后窗口同步变长
	state.CooldownSecs = 900
	assert.False(t, isStaleHalfOpen(state, rule, now))
	state.CooldownSecs = 300
	assert.True(t, isStaleHalfOpen(state, rule, now))
}

func TestChannelBreakerBackoffThroughProbeFailCycle(t *testing.T) {
	common.SetChannelBreakerEnabled(true)
	setupBackoffTest(t)
	disableRedisForBreakerTest(t)
	t.Cleanup(func() { common.SetChannelBreakerEnabled(false) })

	c := testBreakerContext("/v1/chat/completions")
	channelError := types.ChannelError{ChannelId: 3001, UsingKey: "key-a", AutoBan: true}
	ClearChannelBreaker(channelError)

	for i := 0; i < GetChannelBreakerFailureThreshold(); i++ {
		RecordChannelBreakerFailure(c, channelError, true)
	}
	rule := resolveChannelBreakerRule(c, channelError)
	normalized, ok := normalizeChannelBreakerTarget(channelError, rule)
	require.True(t, ok)
	key := channelBreakerStateKey(c, normalized, rule)
	requireBackoffCooldownForTest(t, key, 60)

	// 冷却已过 → 半开，5 次探测全失败 → 第二次打开，冷却按阶梯翻倍
	setBreakerOpenedAtForTest(key, time.Now().Add(-61*time.Second))
	for i := 0; i < common.GetChannelBreakerProbeCount(); i++ {
		require.True(t, AllowChannelByBreaker(c, channelError), "第 %d 次探测应放行", i+1)
		RecordChannelBreakerFailure(c, channelError, true)
	}
	requireBackoffCooldownForTest(t, key, 120)
}

func requireBackoffCooldownForTest(t *testing.T, key string, want int) {
	t.Helper()
	channelBreakerMu.Lock()
	defer channelBreakerMu.Unlock()
	state := loadChannelBreakerStateLocked(key)
	require.NotNil(t, state)
	require.Equal(t, ChannelBreakerStateOpen, state.State)
	require.Equal(t, want, state.CooldownSecs)
}
