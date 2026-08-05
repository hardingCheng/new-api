package model_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func preserveUserChannelRouting(t *testing.T) {
	t.Helper()
	originalJSON, err := common.Marshal(GetUserChannelRoutingCopy())
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, UpdateUserChannelRoutingByJSONString(string(originalJSON)))
	})
}

func TestMatchUserChannelRoutingSelectsUserGroupAndModelScope(t *testing.T) {
	preserveUserChannelRouting(t)
	require.NoError(t, UpdateUserChannelRoutingByJSONString(`{
		"rules": [
			{"id":"global","name":"global","user_id":7,"group_pattern":"*","model_pattern":"*","channel_ids":[9],"fallback":"strict"},
			{"id":"sd2","name":"sd2","user_id":7,"group_pattern":"sd2","model_pattern":"*","channel_ids":[1,2],"fallback":"strict"},
			{"id":"sd2-video","name":"sd2 video","user_id":7,"group_pattern":"sd2","model_pattern":"video-*","channel_ids":[3],"fallback":"default"},
			{"id":"disabled","name":"disabled","user_id":8,"group_pattern":"sd2","model_pattern":"*","channel_ids":[4],"fallback":"strict","disabled":true}
		]
	}`))

	match, ok := MatchUserChannelRouting(7, "sd2", "video-fast")
	require.True(t, ok)
	assert.Equal(t, "sd2-video", match.Rule.ID)
	assert.Equal(t, UserChannelRoutingFallbackDefault, match.Rule.Fallback)
	_, allowsChannel3 := match.AllowedChannelIDs[3]
	assert.True(t, allowsChannel3)

	match, ok = MatchUserChannelRouting(7, "sd2", "image-fast")
	require.True(t, ok)
	assert.Equal(t, "sd2", match.Rule.ID)
	assert.Equal(t, []int{1, 2}, match.Rule.ChannelIDs)

	match, ok = MatchUserChannelRouting(7, "other", "image-fast")
	require.True(t, ok)
	assert.Equal(t, "global", match.Rule.ID)

	_, ok = MatchUserChannelRouting(8, "sd2", "video-fast")
	assert.False(t, ok)
	_, ok = MatchUserChannelRouting(99, "sd2", "video-fast")
	assert.False(t, ok)
}

func TestParseUserChannelRoutingRejectsInvalidRules(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "missing channels", value: `{"rules":[{"id":"a","name":"a","user_id":1,"group_pattern":"sd2","channel_ids":[]}]}`},
		{name: "duplicate channel", value: `{"rules":[{"id":"a","name":"a","user_id":1,"group_pattern":"sd2","channel_ids":[1,1]}]}`},
		{name: "duplicate id", value: `{"rules":[{"id":"a","name":"a","user_id":1,"group_pattern":"sd2","channel_ids":[1]},{"id":"a","name":"b","user_id":2,"group_pattern":"sd2","channel_ids":[2]}]}`},
		{name: "duplicate scope", value: `{"rules":[{"id":"a","name":"a","user_id":1,"group_pattern":"sd2","model_pattern":"*","channel_ids":[1]},{"id":"b","name":"b","user_id":1,"group_pattern":"sd2","model_pattern":"*","channel_ids":[2]}]}`},
		{name: "invalid fallback", value: `{"rules":[{"id":"a","name":"a","user_id":1,"group_pattern":"sd2","channel_ids":[1],"fallback":"anything"}]}`},
		{name: "group wildcard segment", value: `{"rules":[{"id":"a","name":"a","user_id":1,"group_pattern":"sd*","channel_ids":[1]}]}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseUserChannelRoutingJSONString(test.value)
			require.Error(t, err)
		})
	}
}

func TestGetUserChannelRoutingCopyDoesNotExposeStoredChannelIDs(t *testing.T) {
	preserveUserChannelRouting(t)
	require.NoError(t, UpdateUserChannelRoutingByJSONString(`{
		"rules": [{"id":"route","name":"Route","user_id":7,"group_pattern":"sd2","channel_ids":[1,2],"fallback":"strict"}]
	}`))

	config := GetUserChannelRoutingCopy()
	require.Len(t, config.Rules, 1)
	config.Rules[0].ChannelIDs[0] = 999

	match, ok := MatchUserChannelRouting(7, "sd2", "video")
	require.True(t, ok)
	assert.Equal(t, []int{1, 2}, match.Rule.ChannelIDs)
}
