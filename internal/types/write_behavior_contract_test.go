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
