package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func setupChannelStatusTest(t *testing.T) {
	t.Helper()
	truncateTables(t)
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() { common.MemoryCacheEnabled = oldMemoryCacheEnabled })
}

func TestHandlerMultiKeyUpdateIsIsolatedAndIdempotent(t *testing.T) {
	channel := &Channel{
		Id:     1001,
		Key:    "key-a\nkey-b",
		Status: common.ChannelStatusEnabled,
		ChannelInfo: ChannelInfo{
			IsMultiKey: true,
		},
	}

	require.True(t, handlerMultiKeyUpdate(channel, "key-a", common.ChannelStatusAutoDisabled, "balance exhausted", false))
	require.Equal(t, common.ChannelStatusEnabled, channel.Status)
	require.Equal(t, common.ChannelStatusAutoDisabled, channel.ChannelInfo.MultiKeyStatusList[0])

	require.False(t, handlerMultiKeyUpdate(channel, "key-a", common.ChannelStatusAutoDisabled, "duplicate", false))

	require.True(t, handlerMultiKeyUpdate(channel, "key-b", common.ChannelStatusAutoDisabled, "balance exhausted", false))
	require.Equal(t, common.ChannelStatusAutoDisabled, channel.Status)

	require.True(t, handlerMultiKeyUpdate(channel, "key-a", common.ChannelStatusEnabled, "", false))
	require.Equal(t, common.ChannelStatusEnabled, channel.Status)
	_, disabled := channel.ChannelInfo.MultiKeyStatusList[0]
	require.False(t, disabled)
}

func TestHandlerMultiKeyUpdateDoesNotOverrideManualKeyDisable(t *testing.T) {
	channel := &Channel{
		Id:     1002,
		Key:    "key-a\nkey-b",
		Status: common.ChannelStatusEnabled,
		ChannelInfo: ChannelInfo{
			IsMultiKey:         true,
			MultiKeyStatusList: map[int]int{0: common.ChannelStatusManuallyDisabled},
		},
	}

	require.False(t, handlerMultiKeyUpdate(channel, "key-a", common.ChannelStatusAutoDisabled, "in-flight failure", false))
	require.Equal(t, common.ChannelStatusEnabled, channel.Status)
	require.Equal(t, common.ChannelStatusManuallyDisabled, channel.ChannelInfo.MultiKeyStatusList[0])
}

func TestUpdateChannelStatusWithoutAutoRecoveryPersistsUntilManualEnable(t *testing.T) {
	setupChannelStatusTest(t)
	channel := &Channel{
		Type:   1,
		Name:   "terminal-disabled-channel",
		Key:    "key-a",
		Status: common.ChannelStatusEnabled,
	}
	require.NoError(t, DB.Create(channel).Error)

	require.True(t, UpdateChannelStatusWithoutAutoRecovery(
		channel.Id, "", common.ChannelStatusAutoDisabled, "upstream quota exhausted"))

	var disabled Channel
	require.NoError(t, DB.First(&disabled, channel.Id).Error)
	require.True(t, disabled.IsAutoRecoveryDisabled())

	require.True(t, UpdateChannelStatus(channel.Id, "", common.ChannelStatusEnabled, "manual recovery"))

	var enabled Channel
	require.NoError(t, DB.First(&enabled, channel.Id).Error)
	require.Equal(t, common.ChannelStatusEnabled, enabled.Status)
	require.False(t, enabled.ChannelInfo.AutoRecoveryDisabled)
	require.False(t, enabled.IsAutoRecoveryDisabled())
}

func TestMultiKeyAutoRecoveryStopsOnlyWhenEveryKeyIsTerminal(t *testing.T) {
	setupChannelStatusTest(t)

	terminalChannel := &Channel{
		Type:   1,
		Name:   "terminal-multi-key",
		Key:    "key-a\nkey-b",
		Status: common.ChannelStatusEnabled,
		ChannelInfo: ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 2,
		},
	}
	require.NoError(t, DB.Create(terminalChannel).Error)
	require.True(t, UpdateChannelStatusWithoutAutoRecovery(
		terminalChannel.Id, "key-a", common.ChannelStatusAutoDisabled, "upstream quota exhausted"))

	var firstKeyDisabled Channel
	require.NoError(t, DB.First(&firstKeyDisabled, terminalChannel.Id).Error)
	require.Equal(t, common.ChannelStatusEnabled, firstKeyDisabled.Status)
	require.False(t, firstKeyDisabled.IsAutoRecoveryDisabled())

	require.True(t, UpdateChannelStatusWithoutAutoRecovery(
		terminalChannel.Id, "key-b", common.ChannelStatusAutoDisabled, "upstream quota exhausted"))

	var allKeysTerminal Channel
	require.NoError(t, DB.First(&allKeysTerminal, terminalChannel.Id).Error)
	require.True(t, allKeysTerminal.IsAutoRecoveryDisabled())

	mixedChannel := &Channel{
		Type:   1,
		Name:   "mixed-multi-key",
		Key:    "key-a\nkey-b",
		Status: common.ChannelStatusEnabled,
		ChannelInfo: ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 2,
		},
	}
	require.NoError(t, DB.Create(mixedChannel).Error)
	require.True(t, UpdateChannelStatusWithoutAutoRecovery(
		mixedChannel.Id, "key-a", common.ChannelStatusAutoDisabled, "upstream quota exhausted"))
	require.True(t, UpdateChannelStatus(
		mixedChannel.Id, "key-b", common.ChannelStatusAutoDisabled, "temporary upstream failure"))

	var mixedDisabled Channel
	require.NoError(t, DB.First(&mixedDisabled, mixedChannel.Id).Error)
	require.Equal(t, common.ChannelStatusAutoDisabled, mixedDisabled.Status)
	require.False(t, mixedDisabled.IsAutoRecoveryDisabled())
}

func TestRecalculateMultiKeyStatusUsesRecoverableKeyState(t *testing.T) {
	tests := []struct {
		name                 string
		statusList           map[int]int
		autoRecoveryDisabled map[int]bool
		expectedStatus       int
		expectedSkipRecovery bool
	}{
		{
			name:                 "all terminal",
			statusList:           map[int]int{0: common.ChannelStatusAutoDisabled, 1: common.ChannelStatusAutoDisabled},
			autoRecoveryDisabled: map[int]bool{0: true, 1: true},
			expectedStatus:       common.ChannelStatusAutoDisabled,
			expectedSkipRecovery: true,
		},
		{
			name:                 "one recoverable",
			statusList:           map[int]int{0: common.ChannelStatusAutoDisabled, 1: common.ChannelStatusAutoDisabled},
			autoRecoveryDisabled: map[int]bool{0: true},
			expectedStatus:       common.ChannelStatusAutoDisabled,
			expectedSkipRecovery: false,
		},
		{
			name:                 "terminal and manual",
			statusList:           map[int]int{0: common.ChannelStatusAutoDisabled, 1: common.ChannelStatusManuallyDisabled},
			autoRecoveryDisabled: map[int]bool{0: true},
			expectedStatus:       common.ChannelStatusAutoDisabled,
			expectedSkipRecovery: true,
		},
		{
			name:                 "all manual",
			statusList:           map[int]int{0: common.ChannelStatusManuallyDisabled, 1: common.ChannelStatusManuallyDisabled},
			expectedStatus:       common.ChannelStatusManuallyDisabled,
			expectedSkipRecovery: true,
		},
		{
			name:                 "enabled key wins",
			statusList:           map[int]int{0: common.ChannelStatusAutoDisabled},
			autoRecoveryDisabled: map[int]bool{0: true, 1: true},
			expectedStatus:       common.ChannelStatusEnabled,
			expectedSkipRecovery: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			channel := &Channel{
				Key:    "key-a\nkey-b",
				Status: common.ChannelStatusEnabled,
				ChannelInfo: ChannelInfo{
					IsMultiKey:                   true,
					MultiKeySize:                 2,
					MultiKeyStatusList:           test.statusList,
					MultiKeyAutoRecoveryDisabled: test.autoRecoveryDisabled,
				},
			}

			channel.RecalculateMultiKeyStatus()

			require.Equal(t, test.expectedStatus, channel.Status)
			require.Equal(t, test.expectedSkipRecovery, channel.ChannelInfo.AutoRecoveryDisabled)
		})
	}
}

func TestUpdateChannelStatusPersistsMultiKeyIsolation(t *testing.T) {
	setupChannelStatusTest(t)
	channel := &Channel{
		Type:   1,
		Name:   "cluster-multi-key",
		Key:    "key-a\nkey-b",
		Status: common.ChannelStatusEnabled,
		ChannelInfo: ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 2,
		},
	}
	require.NoError(t, DB.Create(channel).Error)

	require.True(t, UpdateChannelStatus(channel.Id, "key-a", common.ChannelStatusAutoDisabled, "balance exhausted"))
	require.False(t, UpdateChannelStatus(channel.Id, "key-a", common.ChannelStatusAutoDisabled, "duplicate"))

	var afterFirst Channel
	require.NoError(t, DB.First(&afterFirst, channel.Id).Error)
	require.Equal(t, common.ChannelStatusEnabled, afterFirst.Status)
	require.Equal(t, common.ChannelStatusAutoDisabled, afterFirst.ChannelInfo.MultiKeyStatusList[0])
	require.Equal(t, "key-a\nkey-b", afterFirst.Key)

	require.True(t, UpdateChannelStatus(channel.Id, "key-b", common.ChannelStatusAutoDisabled, "balance exhausted"))
	var afterSecond Channel
	require.NoError(t, DB.First(&afterSecond, channel.Id).Error)
	require.Equal(t, common.ChannelStatusAutoDisabled, afterSecond.Status)
	require.Equal(t, common.ChannelStatusAutoDisabled, afterSecond.ChannelInfo.MultiKeyStatusList[0])
	require.Equal(t, common.ChannelStatusAutoDisabled, afterSecond.ChannelInfo.MultiKeyStatusList[1])

	require.True(t, UpdateChannelStatus(channel.Id, "key-a", common.ChannelStatusEnabled, ""))
	var restored Channel
	require.NoError(t, DB.First(&restored, channel.Id).Error)
	require.Equal(t, common.ChannelStatusEnabled, restored.Status)
	_, keyADisabled := restored.ChannelInfo.MultiKeyStatusList[0]
	require.False(t, keyADisabled)
	require.Equal(t, common.ChannelStatusAutoDisabled, restored.ChannelInfo.MultiKeyStatusList[1])
}

func TestMultiKeyUpdatePreservesChannelLevelManualDisable(t *testing.T) {
	setupChannelStatusTest(t)
	channel := &Channel{
		Type:   1,
		Name:   "manually-disabled-multi-key",
		Key:    "key-a\nkey-b",
		Status: common.ChannelStatusEnabled,
		ChannelInfo: ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 2,
		},
	}
	require.NoError(t, DB.Create(channel).Error)
	require.True(t, UpdateChannelStatus(channel.Id, "", common.ChannelStatusManuallyDisabled, "manual operation"))
	require.True(t, UpdateChannelStatus(channel.Id, "key-a", common.ChannelStatusAutoDisabled, "in-flight failure"))

	var disabled Channel
	require.NoError(t, DB.First(&disabled, channel.Id).Error)
	require.Equal(t, common.ChannelStatusManuallyDisabled, disabled.Status)
	require.True(t, disabled.ChannelInfo.ChannelManuallyDisabled)
	require.Equal(t, common.ChannelStatusAutoDisabled, disabled.ChannelInfo.MultiKeyStatusList[0])

	require.True(t, UpdateChannelStatus(channel.Id, "", common.ChannelStatusEnabled, "manual recovery"))
	require.NoError(t, DB.First(&disabled, channel.Id).Error)
	require.Equal(t, common.ChannelStatusEnabled, disabled.Status)
	require.False(t, disabled.ChannelInfo.ChannelManuallyDisabled)
}

func TestUpdateChannelStatusRestoresEnabledChannelToMemoryRouting(t *testing.T) {
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	t.Cleanup(func() {
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
		channelSyncLock.Lock()
		group2model2channels = nil
		channelsIDM = nil
		channel2advancedCustomConfig = nil
		channelSyncLock.Unlock()
	})
	truncateTables(t)

	priority := int64(10)
	channel := &Channel{
		Type:     1,
		Name:     "recoverable-channel",
		Key:      "sk-test",
		Status:   common.ChannelStatusEnabled,
		Models:   "test-model",
		Group:    "default",
		Priority: &priority,
	}
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, DB.Create(&Ability{
		Group:     "default",
		Model:     "test-model",
		ChannelId: channel.Id,
		Enabled:   true,
		Priority:  &priority,
	}).Error)
	InitChannelCache()

	selected, err := GetRandomSatisfiedChannel("default", "test-model", 0, "")
	require.NoError(t, err)
	require.NotNil(t, selected)
	require.Equal(t, channel.Id, selected.Id)

	require.True(t, UpdateChannelStatus(channel.Id, "", common.ChannelStatusAutoDisabled, "temporary failure"))
	selected, err = GetRandomSatisfiedChannel("default", "test-model", 0, "")
	require.NoError(t, err)
	require.Nil(t, selected)

	require.True(t, UpdateChannelStatus(channel.Id, "", common.ChannelStatusEnabled, ""))
	selected, err = GetRandomSatisfiedChannel("default", "test-model", 0, "")
	require.NoError(t, err)
	require.NotNil(t, selected)
	require.Equal(t, channel.Id, selected.Id)
}
