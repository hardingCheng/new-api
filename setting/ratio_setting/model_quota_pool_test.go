package ratio_setting

import "testing"

func TestMatchModelQuotaPoolRules(t *testing.T) {
	t.Cleanup(func() {
		_ = UpdateModelQuotaPoolByJSONString("{}")
	})
	err := UpdateModelQuotaPoolByJSONString(`{
		"rules": [
			{"id":"global-seedance","model":"seedance-*","scope":"global","period":"day","limit":500},
			{"id":"global-fast","model":"seedance-2.0-fast-480p","scope":"global","period":"hour","limit":20},
			{"id":"user-fast","model":"seedance-2.0-fast-*","scope":"user","user_id":42,"period":"day","limit":10},
			{"id":"other-user","model":"seedance-2.0-fast-*","scope":"user","user_id":7,"period":"day","limit":3}
		]
	}`)
	if err != nil {
		t.Fatal(err)
	}

	matches := MatchModelQuotaPoolRules(42, "seedance-2.0-fast-480p")
	if len(matches) != 2 {
		t.Fatalf("expected user + global matches, got %d", len(matches))
	}
	if matches[0].ID != "user-fast" {
		t.Fatalf("expected best user rule first, got %s", matches[0].ID)
	}
	if matches[1].ID != "global-fast" {
		t.Fatalf("expected exact global rule, got %s", matches[1].ID)
	}
}

func TestModelQuotaPoolNormalizeSkipsInvalidRules(t *testing.T) {
	t.Cleanup(func() {
		_ = UpdateModelQuotaPoolByJSONString("{}")
	})
	err := UpdateModelQuotaPoolByJSONString(`{
		"rules": [
			{"model":"","scope":"global","period":"day","limit":500},
			{"model":"seedance-*","scope":"user","period":"day","limit":500},
			{"model":"seedance-*","scope":"global","period":"daily","limit":500}
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
