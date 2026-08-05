package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupUserRoutingChannelCacheTest(t *testing.T, memoryCacheEnabled bool) {
	t.Helper()
	truncateTables(t)
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = memoryCacheEnabled
	t.Cleanup(func() {
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
		channelSyncLock.Lock()
		group2model2channels = nil
		channelsIDM = nil
		channel2advancedCustomConfig = nil
		channelSyncLock.Unlock()
	})
}

func TestCandidateFilterRunsBeforePrioritySelection(t *testing.T) {
	for _, memoryCacheEnabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "database", true: "memory_cache"}[memoryCacheEnabled], func(t *testing.T) {
			setupUserRoutingChannelCacheTest(t, memoryCacheEnabled)

			highPriority := int64(100)
			lowPriority := int64(10)
			high := &Channel{Id: 301, Type: 1, Name: "channel-3", Key: "sk-3", Status: common.ChannelStatusEnabled, Group: "sd2", Models: "video", Priority: &highPriority}
			allowed := &Channel{Id: 101, Type: 1, Name: "channel-1", Key: "sk-1", Status: common.ChannelStatusEnabled, Group: "sd2", Models: "video", Priority: &lowPriority}
			require.NoError(t, DB.Create(high).Error)
			require.NoError(t, DB.Create(allowed).Error)
			require.NoError(t, DB.Create(&Ability{Group: "sd2", Model: "video", ChannelId: high.Id, Enabled: true, Priority: &highPriority, Weight: 100}).Error)
			require.NoError(t, DB.Create(&Ability{Group: "sd2", Model: "video", ChannelId: allowed.Id, Enabled: true, Priority: &lowPriority, Weight: 100}).Error)
			if memoryCacheEnabled {
				InitChannelCache()
			}

			selected, exhausted, err := GetRandomSatisfiedChannelWithFilters("sd2", "video", 0, "", func(channel *Channel) bool {
				return channel.Id == allowed.Id
			}, nil)
			require.NoError(t, err)
			assert.False(t, exhausted)
			require.NotNil(t, selected)
			require.Equal(t, allowed.Id, selected.Id)
		})
	}
}

func TestCandidateFilterDoesNotMoveDisabledPriorityBelowEnabledPriority(t *testing.T) {
	for _, memoryCacheEnabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "database", true: "memory_cache"}[memoryCacheEnabled], func(t *testing.T) {
			setupUserRoutingChannelCacheTest(t, memoryCacheEnabled)

			highPriority := int64(100)
			lowPriority := int64(10)
			disabled := &Channel{Id: 101, Type: 1, Name: "disabled-high", Key: "sk-1", Status: common.ChannelStatusEnabled, Group: "sd2", Models: "video", Priority: &highPriority}
			enabled := &Channel{Id: 102, Type: 1, Name: "enabled-low", Key: "sk-2", Status: common.ChannelStatusEnabled, Group: "sd2", Models: "video", Priority: &lowPriority}
			require.NoError(t, DB.Create(disabled).Error)
			require.NoError(t, DB.Create(enabled).Error)
			require.NoError(t, DB.Create(&Ability{Group: "sd2", Model: "video", ChannelId: disabled.Id, Enabled: true, Priority: &highPriority, Weight: 100}).Error)
			require.NoError(t, DB.Create(&Ability{Group: "sd2", Model: "video", ChannelId: enabled.Id, Enabled: true, Priority: &lowPriority, Weight: 100}).Error)
			if memoryCacheEnabled {
				InitChannelCache()
				channelsIDM[disabled.Id].Status = common.ChannelStatusAutoDisabled
			} else {
				require.NoError(t, DB.Model(disabled).Update("status", common.ChannelStatusAutoDisabled).Error)
			}

			candidateFilter := func(channel *Channel) bool {
				return channel.Id == disabled.Id || channel.Id == enabled.Id
			}
			selected, exhausted, err := GetRandomSatisfiedChannelWithFilters("sd2", "video", 0, "", candidateFilter, nil)
			require.NoError(t, err)
			assert.False(t, exhausted)
			assert.Nil(t, selected)

			selected, exhausted, err = GetRandomSatisfiedChannelWithFilters("sd2", "video", 1, "", candidateFilter, nil)
			require.NoError(t, err)
			assert.False(t, exhausted)
			require.NotNil(t, selected)
			assert.Equal(t, enabled.Id, selected.Id)

			selected, exhausted, err = GetRandomSatisfiedChannelWithFilters("sd2", "video", 2, "", candidateFilter, nil)
			require.NoError(t, err)
			assert.True(t, exhausted)
			assert.Nil(t, selected)
		})
	}
}

func TestLegacyMemorySelectionKeepsEmptyPriorityError(t *testing.T) {
	setupUserRoutingChannelCacheTest(t, true)

	priority := int64(100)
	disabledChannels := []*Channel{
		{Id: 101, Type: 1, Name: "disabled-1", Key: "sk-1", Status: common.ChannelStatusEnabled, Group: "sd2", Models: "video", Priority: &priority},
		{Id: 102, Type: 1, Name: "disabled-2", Key: "sk-2", Status: common.ChannelStatusEnabled, Group: "sd2", Models: "video", Priority: &priority},
	}
	for _, channel := range disabledChannels {
		require.NoError(t, DB.Create(channel).Error)
		require.NoError(t, DB.Create(&Ability{Group: "sd2", Model: "video", ChannelId: channel.Id, Enabled: true, Priority: &priority, Weight: 100}).Error)
	}
	InitChannelCache()
	for _, channel := range disabledChannels {
		channelsIDM[channel.Id].Status = common.ChannelStatusAutoDisabled
	}

	selected, err := GetRandomSatisfiedChannel("sd2", "video", 0, "")
	assert.Nil(t, selected)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no channel found")

	selected, exhausted, err := GetRandomSatisfiedChannelWithFilters("sd2", "video", 0, "", func(channel *Channel) bool {
		return channel.Id == disabledChannels[0].Id || channel.Id == disabledChannels[1].Id
	}, nil)
	require.NoError(t, err)
	assert.False(t, exhausted)
	assert.Nil(t, selected)
}
