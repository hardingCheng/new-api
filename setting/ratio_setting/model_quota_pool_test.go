package ratio_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMatchModelQuotaPoolRules(t *testing.T) {
	t.Cleanup(func() {
		_ = UpdateModelQuotaPoolByJSONString("{}")
	})
	err := UpdateModelQuotaPoolByJSONString(`{
		"rules": [
			{"id":"global-seedance","model":"seedance-*","scope":"global","period":"day","limit":500},
			{"id":"global-fast","model":"seedance-2.0-fast-480p","scope":"global","period":"hour","limit":20},
			{"id":"user-fast","model":"seedance-2.0-fast-*","scope":"user","user_id":42,"period":"day","limit":10},
			{"id":"other-user","model":"seedance-2.0-fast-*","scope":"user","user_id":7,"period":"day","limit":3},
			{"id":"global-prism","model":"prism-*","scope":"global","period":"day","limit":500},
			{"id":"prism-user-fast","model":"prism-3.0-fast-*","scope":"user","user_id":42,"period":"day","limit":10}
		]
	}`)
	require.NoError(t, err)

	matches := MatchModelQuotaPoolRules(42, "seedance-2.0-fast-480p")
	require.Len(t, matches, 2)
	assert.Equal(t, "user-fast", matches[0].ID)
	assert.Equal(t, "global-fast", matches[1].ID)

	prismMatches := MatchModelQuotaPoolRules(42, "prism-3.0-fast-480p")
	require.Len(t, prismMatches, 2)
	assert.Equal(t, "prism-user-fast", prismMatches[0].ID)
	assert.Equal(t, "global-prism", prismMatches[1].ID)
}

func TestMatchModelQuotaPoolRulesUsesPublicModelNameOnly(t *testing.T) {
	t.Cleanup(func() {
		_ = UpdateModelQuotaPoolByJSONString("{}")
	})
	require.NoError(t, UpdateModelQuotaPoolByJSONString(`{
		"rules": [
			{"id":"public-model","model":"public-gpt","scope":"global","period":"day","limit":10},
			{"id":"upstream-model","model":"vendor-gpt","scope":"global","period":"day","limit":10}
		]
	}`))

	matches := MatchModelQuotaPoolRules(42, "public-gpt")
	require.Len(t, matches, 1)
	assert.Equal(t, "public-model", matches[0].ID)
}

func TestModelQuotaPoolNormalizeSkipsInvalidRules(t *testing.T) {
	t.Cleanup(func() {
		_ = UpdateModelQuotaPoolByJSONString("{}")
	})
	err := UpdateModelQuotaPoolByJSONString(`{
		"rules": [
			{"model":"","scope":"global","period":"day","limit":500},
			{"model":"seedance-*","scope":"user","period":"day","limit":500},
			{"model":"seedance-*","scope":"global","period":"daily","limit":500},
			{"model":"prism-*","scope":"user","period":"day","limit":500}
		]
	}`)
	if err != nil {
		t.Fatal(err)
	}
	cfg := GetModelQuotaPoolCopy()
	if len(cfg.Rules) != 1 {
		t.Fatalf("expected 1 valid rule, got %d", len(cfg.Rules))
	}
	if cfg.Rules[0].Period != ModelQuotaPoolPeriodDay {
		t.Fatalf("expected normalized day period, got %s", cfg.Rules[0].Period)
	}
}
