package types

import (
	"testing"
)

// TestLegacyDeriveSeverityMatchesRegistry — every kind in
// AllViolationKinds must have legacyDeriveSeverity agree with the
// registry-declared DefaultSeverity. Migration window invariant.
func TestLegacyDeriveSeverityMatchesRegistry(t *testing.T) {
	for _, kind := range AllViolationKinds() {
		spec, ok := ViolKindSpecFor(kind)
		if !ok {
			t.Errorf("kind=%q has no registry spec", kind)
			continue
		}
		legacy := legacyDeriveSeverity(kind, false)
		if legacy != spec.DefaultSeverity {
			t.Errorf("kind=%q: severity drift — registry=%q legacyDeriveSeverity=%q",
				kind, spec.DefaultSeverity, legacy)
		}
	}
}

// TestNotPromotable_IgnoresStrictAtProfileLayer asserts the
// ViolationProfileFor rule: permanently-non-promotable kinds keep
// their default RetryEligible regardless of isStrict.
func TestNotPromotable_IgnoresStrictAtProfileLayer(t *testing.T) {
	for _, spec := range AllViolKindSpecs() {
		if spec.Promotable {
			continue
		}
		// permanently non-promotable kinds' RetryEligible must stay
		// fixed across both isStrict values.
		soft := ViolationProfileFor(spec.Kind, false).RetryEligible
		strict := ViolationProfileFor(spec.Kind, true).RetryEligible
		if soft != strict {
			t.Errorf("kind=%q is Promotable=false but RetryEligible flipped under isStrict (false=%v, true=%v)",
				spec.Kind, soft, strict)
		}
	}
}

// TestRegisterViolKind_RejectsInvalidSpec ensures bad specs panic
// at registration time so a typo cannot ship.
func TestRegisterViolKind_RejectsInvalidSpec(t *testing.T) {
	mustPanic := func(name string, fn func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Errorf("%s: expected panic, none thrown", name)
			}
		}()
		fn()
	}
	mustPanic("empty kind", func() {
		RegisterViolKind(ViolKindSpec{DefaultSeverity: SeverityMedium, FallbackLocus: LocusFinalizer})
	})
	mustPanic("invalid severity", func() {
		RegisterViolKind(ViolKindSpec{Kind: "test_x", DefaultSeverity: "bogus", FallbackLocus: LocusFinalizer})
	})
	mustPanic("invalid locus", func() {
		RegisterViolKind(ViolKindSpec{Kind: "test_x", DefaultSeverity: SeverityMedium, FallbackLocus: "bogus"})
	})
}

// TestAllViolKindSpecs_OrderStable verifies declaration order is
// preserved (mirrors AllViolationKinds for telemetry stability).
func TestAllViolKindSpecs_OrderStable(t *testing.T) {
	first := AllViolKindSpecs()
	second := AllViolKindSpecs()
	if len(first) != len(second) {
		t.Fatalf("AllViolKindSpecs returned different lengths: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i].Kind != second[i].Kind {
			t.Errorf("position %d: %q vs %q", i, first[i].Kind, second[i].Kind)
		}
	}
}
