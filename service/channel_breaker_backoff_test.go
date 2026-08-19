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

func TestChannelBreakerBackoffEscalatesAndCaps(t *testing.T) {
	setupBackoffTest(t)
	rule := backoffTestRule(60)
	state := &channelBreakerState{}
	t0 := time.Unix(1700000000, 0)

	expected := []int{60, 120, 300, 900, 3600, 3600}
	for i, want := range expected {
		// 每次打开间隔 60 秒，处于衰减窗内，连击持续累积
		openBreakerAt(state, t0.Add(time.Duration(i)*time.Minute), rule)
		assert.Equal(t, want, state.CooldownSecs, "第 %d 次连续打开", i+1)
		assert.Equal(t, i+1, state.ConsecutiveOpens)
	}
}

func TestChannelBreakerBackoffDecayResetsAfterQuietWindow(t *testing.T) {
	setupBackoffTest(t)
	rule := backoffTestRule(60)
	state := &channelBreakerState{}
	t0 := time.Unix(1700000000, 0)

	openBreakerAt(state, t0, rule)
	openBreakerAt(state, t0.Add(time.Minute), rule)
	openBreakerAt(state, t0.Add(2*time.Minute), rule)
	require.Equal(t, 300, state.CooldownSecs)

	// 安静超过衰减窗（600s），连击归零，重新从初值开始
	openBreakerAt(state, t0.Add(2*time.Minute+601*time.Second), rule)
	assert.Equal(t, 60, state.CooldownSecs)
	assert.Equal(t, 1, state.ConsecutiveOpens)
}

func TestChannelBreakerBackoffCustomBaseAndCap(t *testing.T) {
	setupBackoffTest(t)
	common.SetChannelBreakerBackoffMaxCooldownSeconds(400)
	rule := backoffTestRule(120)
	state := &channelBreakerState{}
	t0 := time.Unix(1700000000, 0)

	expected := []int{120, 240, 400}
	for i, want := range expected {
		openBreakerAt(state, t0.Add(time.Duration(i)*time.Minute), rule)
		assert.Equal(t, want, state.CooldownSecs, "第 %d 次连续打开", i+1)
	}
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
