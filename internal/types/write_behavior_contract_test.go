package types

import "testing"

func TestNormalizeWriteBehaviorContracts_PreservesNotRaisesAndPolarity(t *testing.T) {
	got := NormalizeWriteBehaviorContracts([]WriteBehaviorContract{{
		ID:       "no-crash",
		Kind:     WriteBehaviorException,
		Polarity: WriteBehaviorPolarityExpected,
		Operator: WriteBehaviorOpNotRaises,
		Expected: "ValueError",
		Required: true,
		Source:   "write_analyzer",
	}}, nil)
	if len(got) != 1 {
		t.Fatalf("contracts len = %d, want 1: %+v", len(got), got)
	}
	if got[0].Operator != WriteBehaviorOpNotRaises {
		t.Fatalf("Operator = %q, want not_raises", got[0].Operator)
	}
	if got[0].Polarity != WriteBehaviorPolarityExpected {
		t.Fatalf("Polarity = %q, want expected", got[0].Polarity)
	}
	if !got[0].Required {
		t.Fatal("expected target contract to remain required")
	}
}

func TestNormalizeWriteBehaviorContracts_DefaultsExpectedAndForbiddenToRequired(t *testing.T) {
	got := NormalizeWriteBehaviorContracts([]WriteBehaviorContract{
		{
			ID:       "no-crash",
			Kind:     WriteBehaviorException,
			Polarity: WriteBehaviorPolarityExpected,
			Operator: WriteBehaviorOpNotRaises,
			Expected: "request no longer crashes",
			Source:   "write_analyzer",
		},
		{
			ID:       "no-regression",
			Kind:     WriteBehaviorInvariant,
			Polarity: WriteBehaviorPolarityForbidden,
			Operator: WriteBehaviorOpRaises,
			Expected: "old regression does not return",
			Source:   "write_analyzer",
		},
	}, nil)
	if len(got) != 2 {
		t.Fatalf("contracts len = %d, want 2: %+v", len(got), got)
	}
	ids := RequiredWriteBehaviorContractIDs(got, false)
	for _, want := range []string{"no-crash", "no-regression"} {
		if _, ok := ids[want]; !ok {
			t.Fatalf("expected %q to be required after normalization: contracts=%+v ids=%+v", want, got, ids)
		}
	}
}

func TestNormalizeWriteBehaviorContracts_ObservedIsNotRequiredTarget(t *testing.T) {
	got := NormalizeWriteBehaviorContracts([]WriteBehaviorContract{{
		ID:       "current-crash",
		Kind:     WriteBehaviorException,
		Polarity: WriteBehaviorPolarityObserved,
		Operator: WriteBehaviorOpRaises,
		Expected: "ZeroDivisionError",
		Required: true,
		Source:   "write_analyzer",
	}}, nil)
	if len(got) != 1 {
		t.Fatalf("contracts len = %d, want 1: %+v", len(got), got)
	}
	if got[0].Polarity != WriteBehaviorPolarityObserved {
		t.Fatalf("Polarity = %q, want observed", got[0].Polarity)
	}
	if got[0].Required {
		t.Fatalf("observed pre-fix facts must not remain required completion targets: %+v", got[0])
	}
	if ids := RequiredWriteBehaviorContractIDs(got, false); len(ids) != 0 {
		t.Fatalf("observed contract should not be required coverage: %+v", ids)
	}
}

func TestNormalizeWriteBehaviorContracts_FallbacksAreExpectedTargets(t *testing.T) {
	got := NormalizeWriteBehaviorContracts(nil, []string{"bug no longer reproduces"})
	if len(got) != 1 {
		t.Fatalf("contracts len = %d, want 1: %+v", len(got), got)
	}
	if got[0].Polarity != WriteBehaviorPolarityExpected || !got[0].Required {
		t.Fatalf("fallback expected outcome should be required expected target: %+v", got[0])
	}
	if ids := RequiredWriteBehaviorContractIDs(got, false); len(ids) != 0 {
		t.Fatalf("explicit-required view should exclude fallback ids: %+v", ids)
	}
	if ids := RequiredWriteBehaviorContractIDs(got, true); len(ids) != 1 {
		t.Fatalf("fallback-inclusive required ids missing: %+v", ids)
	}
}

func TestNormalizeWriteBehaviorContracts_AppendsDistinctExpectedOutcomes(t *testing.T) {
	got := NormalizeWriteBehaviorContracts([]WriteBehaviorContract{{
		ID:       "nan-height",
		Kind:     WriteBehaviorException,
		Polarity: WriteBehaviorPolarityExpected,
		Operator: WriteBehaviorOpNotRaises,
		Expected: "ax.bar([np.nan], [0]) does not raise",
		Source:   "write_analyzer",
	}}, []string{
		"ax.bar([np.nan], [0]) does not raise",
		"ax.bar([np.nan], [np.nan]) does not raise",
	})
	if len(got) != 2 {
		t.Fatalf("contracts len = %d, want explicit plus one distinct fallback: %+v", len(got), got)
	}
	if got[0].ID != "nan-height" || !got[0].Required || got[0].Source != "write_analyzer" {
		t.Fatalf("explicit required contract drifted: %+v", got[0])
	}
	if got[1].Source != "expected_outcome_fallback" || got[1].ID != "outcome-2" || !got[1].Required {
		t.Fatalf("distinct expected outcome should append as required fallback: %+v", got[1])
	}
	if ids := RequiredWriteBehaviorContractIDs(got, false); len(ids) != 1 {
		t.Fatalf("explicit-required view should exclude fallback ids: %+v", ids)
	}
	if ids := RequiredWriteBehaviorContractIDs(got, true); len(ids) != 2 {
		t.Fatalf("fallback-inclusive required ids missing: %+v", ids)
	}
}
