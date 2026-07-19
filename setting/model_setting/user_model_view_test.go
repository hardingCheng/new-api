package model_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func preserveUserModelView(t *testing.T) {
	t.Helper()
	originalJSON, err := common.Marshal(GetUserModelViewCopy())
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, UpdateUserModelViewByJSONString(string(originalJSON)))
	})
}

func TestUserModelViewResolvesAndReplacesTargetModels(t *testing.T) {
	preserveUserModelView(t)
	require.NoError(t, UpdateUserModelViewByJSONString(`{
		"rules": [{
			"user_id": 42,
			"username": "special-user",
			"aliases": [
				{"public_model":"521ai-2.0-sp-720p","target_model":"seedance-2.0-720p","reference_video":"allowed"},
				{"public_model":"521ai-2.0-720p","target_model":"seedance-2.0-720p","reference_video":"forbidden"}
			]
		}]
	}`))

	alias, matched := ResolveUserModelAlias(42, "521ai-2.0-sp-720p")
	require.True(t, matched)
	assert.Equal(t, "seedance-2.0-720p", alias.TargetModel)
	assert.Equal(t, ReferenceVideoAllowed, alias.ReferenceVideo)

	visible := BuildVisibleUserModels(42, []string{
		"unrelated-model",
		"seedance-2.0-720p",
	})
	assert.Equal(t, []VisibleUserModel{
		{Name: "unrelated-model", TargetModel: "unrelated-model"},
		{Name: "521ai-2.0-sp-720p", TargetModel: "seedance-2.0-720p"},
		{Name: "521ai-2.0-720p", TargetModel: "seedance-2.0-720p"},
	}, visible)
}

func TestDisabledUserModelViewDoesNotChangeModels(t *testing.T) {
	preserveUserModelView(t)
	require.NoError(t, UpdateUserModelViewByJSONString(`{
		"rules": [{
			"user_id": 42,
			"disabled": true,
			"aliases": [
				{"public_model":"521ai-2.0-720p","target_model":"seedance-2.0-720p","reference_video":"forbidden"}
			]
		}]
	}`))

	_, matched := ResolveUserModelAlias(42, "521ai-2.0-720p")
	assert.False(t, matched)
	assert.Equal(t, []VisibleUserModel{
		{Name: "seedance-2.0-720p", TargetModel: "seedance-2.0-720p"},
	}, BuildVisibleUserModels(42, []string{"seedance-2.0-720p"}))
}

func TestParseUserModelViewRejectsAmbiguousRules(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{
			name: "duplicate user",
			value: `{"rules":[
				{"user_id":42,"aliases":[]},
				{"user_id":42,"aliases":[]}
			]}`,
		},
		{
			name: "duplicate public model",
			value: `{"rules":[{"user_id":42,"aliases":[
				{"public_model":"alias","target_model":"target-a","reference_video":"allowed"},
				{"public_model":"alias","target_model":"target-b","reference_video":"forbidden"}
			]}]}`,
		},
		{
			name: "public target conflict",
			value: `{"rules":[{"user_id":42,"aliases":[
				{"public_model":"alias-a","target_model":"target-a","reference_video":"allowed"},
				{"public_model":"target-a","target_model":"target-b","reference_video":"forbidden"}
			]}]}`,
		},
		{
			name: "invalid reference policy",
			value: `{"rules":[{"user_id":42,"aliases":[
				{"public_model":"alias","target_model":"target","reference_video":"sometimes"}
			]}]}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseUserModelViewJSONString(test.value)
			require.Error(t, err)
		})
	}
}
