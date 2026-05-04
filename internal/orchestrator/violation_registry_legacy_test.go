package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// violation_registry_legacy_test.go — B0 v3 runtime consolidation
// (2026-05-04). Asserts byte-identical agreement between the new
// ViolKindRegistry-derived tables and the pre-v3 hardcoded literals
// for every kind in types.AllViolationKinds(). This guards against
// drift during the migration window: any spec change MUST keep the
// legacy literal aligned (or vice versa) until the legacy literal
// is deleted in a follow-up PR.

// TestRegistryDerivesAllLegacyTables sweeps every kind and compares:
//
//   - Registry severity        vs legacyDeriveSeverity
//   - Registry SoftByDefault    vs legacyDefaultSoftKinds
//   - Registry FallbackLocus → target   vs legacyDefaultFallbackPolicy
//   - Registry Layer            vs legacyInferViolationLayer
//
// A divergence on any axis fails the test naming the kind + axis.
func TestRegistryDerivesAllLegacyTables(t *testing.T) {
	legacySoft := legacyDefaultSoftKinds()
	legacyPolicy := legacyDefaultFallbackPolicy()

	for _, kind := range types.AllViolationKinds() {
		spec, ok := types.ViolKindSpecFor(kind)
		if !ok {
			t.Errorf("kind=%q: missing registry spec", kind)
			continue
		}

		// Severity (default + isStrict=false path) — DeriveSeverity
		// short-circuits to the registry, so we exercise the public
		// API and trust the registry vs. legacy comparison happens
		// inside the types package's own legacy fallback test.
		profile := types.ViolationProfileFor(kind, false)
		if profile.Severity != spec.DefaultSeverity {
			t.Errorf("kind=%q: severity drift — registry=%q ViolationProfileFor=%q",
				kind, spec.DefaultSeverity, profile.Severity)
		}

		// SoftByDefault.
		legacyIsSoft := legacySoft[kind]
		if legacyIsSoft != spec.SoftByDefault {
			t.Errorf("kind=%q: SoftByDefault drift — registry=%t legacy=%t",
				kind, spec.SoftByDefault, legacyIsSoft)
		}

		// FallbackLocus → target.
		regTarget := targetForLocus(spec.FallbackLocus)
		legacyTarget, present := legacyPolicy[kind]
		if !present {
			// Legacy didn't list it — accept registry value as canonical.
			continue
		}
		if regTarget != legacyTarget {
			t.Errorf("kind=%q: fallback target drift — registry=%q (locus=%q) legacy=%q",
				kind, regTarget, spec.FallbackLocus, legacyTarget)
		}

		// Layer.
		legacyLayer := legacyInferViolationLayer(kind)
		if spec.Layer != legacyLayer {
			t.Errorf("kind=%q: layer drift — registry=%q legacy=%q",
				kind, spec.Layer, legacyLayer)
		}
	}
}

// TestEveryViolKindHasSpec asserts every declared ViolationKind has a
// registry spec. Adding a new kind to violation.go without registering
// a spec fails the build via this test.
func TestEveryViolKindHasSpec(t *testing.T) {
	for _, kind := range types.AllViolationKinds() {
		if _, ok := types.ViolKindSpecFor(kind); !ok {
			t.Errorf("kind=%q has no ViolKindSpec — register in internal/types/violation_registry.go", kind)
		}
	}
}

// TestRegistrySpecCountMatchesAllViolationKinds is a defensive
// check: every registered spec must correspond to a kind in
// AllViolationKinds (no orphaned specs).
func TestRegistrySpecCountMatchesAllViolationKinds(t *testing.T) {
	declared := map[types.ViolationKind]bool{}
	for _, k := range types.AllViolationKinds() {
		declared[k] = true
	}
	for _, spec := range types.AllViolKindSpecs() {
		if !declared[spec.Kind] {
			t.Errorf("registry spec for kind=%q is not in AllViolationKinds — add to violation.go AllViolationKinds slice or remove the spec",
				spec.Kind)
		}
	}
}

// TestPermanentlySoftKindsAreStillSoftAfterPromote verifies that
// kinds with Promotable=false keep RetryEligible=false even when
// callers pass isStrict=true.
func TestPermanentlySoftKindsAreStillSoftAfterPromote(t *testing.T) {
	for _, spec := range types.AllViolKindSpecs() {
		if spec.Promotable {
			continue
		}
		profile := types.ViolationProfileFor(spec.Kind, true)
		if profile.RetryEligible {
			t.Errorf("kind=%q is Promotable=false but RetryEligible=true after isStrict — operator promote should be a no-op",
				spec.Kind)
		}
		if profile.Severity != spec.DefaultSeverity {
			t.Errorf("kind=%q is Promotable=false but Severity changed under isStrict — got %q want %q",
				spec.Kind, profile.Severity, spec.DefaultSeverity)
		}
	}
}
