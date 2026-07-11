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

	require.True(t, handlerMultiKeyUpdate(channel, "key-a", common.ChannelStatusAutoDisabled, "balance exhausted"))
	require.Equal(t, common.ChannelStatusEnabled, channel.Status)
	require.Equal(t, common.ChannelStatusAutoDisabled, channel.ChannelInfo.MultiKeyStatusList[0])

	require.False(t, handlerMultiKeyUpdate(channel, "key-a", common.ChannelStatusAutoDisabled, "duplicate"))

	require.True(t, handlerMultiKeyUpdate(channel, "key-b", common.ChannelStatusAutoDisabled, "balance exhausted"))
	require.Equal(t, common.ChannelStatusAutoDisabled, channel.Status)

	require.True(t, handlerMultiKeyUpdate(channel, "key-a", common.ChannelStatusEnabled, ""))
	require.Equal(t, common.ChannelStatusEnabled, channel.Status)
	_, disabled := channel.ChannelInfo.MultiKeyStatusList[0]
	require.False(t, disabled)
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
