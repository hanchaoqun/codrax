package types

import (
	"strings"
	"testing"
)

// TestSoftDemotedKinds_T3_RouteToCaveat asserts the 2026-05-17 T3
// soft-demote: five oracle kinds whose comment intent is "SOFT-by-
// default" are registry-classified SoftByDefault=true,
// DefaultSeverity=SeveritySoft, default RetryEligible=false (caveat
// path, no retry), and strict-promote returns RetryEligible=true at
// SeverityHigh (operator-yaml override re-engages the retry loop).
//
// Locks the comment-vs-registry agreement so future drift is caught
// at test time, not at production finalize-loop time.
func TestSoftDemotedKinds_T3_RouteToCaveat(t *testing.T) {
	softDemoted := []ViolationKind{
		ViolRichnessGlaringGap,
		ViolPrincipalProseUnderfilled,
		ViolUncertaintyBlockMissing,
		ViolStructuralEnumerationDivergence,
		ViolSymbolAnchorMismatch,
	}
	for _, kind := range softDemoted {
		spec, ok := ViolKindSpecFor(kind)
		if !ok {
			t.Errorf("kind=%q: no registry spec — T3 demotion requires registry entry", kind)
			continue
		}
		if !spec.SoftByDefault {
			t.Errorf("kind=%q: SoftByDefault=false; T3 requires true (caveat, not retry)", kind)
		}
		if spec.DefaultSeverity != SeveritySoft {
			t.Errorf("kind=%q: DefaultSeverity=%q; T3 requires SeveritySoft", kind, spec.DefaultSeverity)
		}
		if !spec.Promotable {
			t.Errorf("kind=%q: Promotable=false; T3 keeps operator strict-promote path open", kind)
		}
		// Default profile: telemetry-only.
		def := ViolationProfileFor(kind, false)
		if def.Severity != SeveritySoft {
			t.Errorf("kind=%q non-strict: Severity=%q want SeveritySoft", kind, def.Severity)
		}
		if def.RetryEligible {
			t.Errorf("kind=%q non-strict: RetryEligible=true want false (caveat path)", kind)
		}
		// Strict-promote profile: retry re-enabled at High.
		strict := ViolationProfileFor(kind, true)
		if strict.Severity != SeverityHigh {
			t.Errorf("kind=%q strict-promote: Severity=%q want SeverityHigh", kind, strict.Severity)
		}
		if !strict.RetryEligible {
			t.Errorf("kind=%q strict-promote: RetryEligible=false want true", kind)
		}
	}
}

// TestCommercialGateKinds_DefaultToCaveat locks the 2026-05-18 policy:
// contract/reviewer/completeness validators whose signals may be noisy,
// incomplete, upstream-owned, or system-caveatable must not force a
// finalizer retry by default. They retain Medium/High/Critical severity
// where appropriate so telemetry stays honest and operators can opt into
// strict promotion explicitly.
func TestCommercialGateKinds_DefaultToCaveat(t *testing.T) {
	kinds := []ViolationKind{
		ViolFamilyMismatch,
		ViolCitation,
		ViolMustInclude,
		ViolMustExclude,
		ViolAcceptance,
		ViolSuccessCriterion,
		ViolGhostAnchor,
		ViolChainDemoted,
		ViolSelfRefLiteral,
		ViolPreCompleteDowngrade,
		ViolLiteralFormFailed,
		ViolViewSwap,
		ViolSelfContradiction,
		ViolAuthorityOverreach,
		ViolClaimFormUnsupported,
		ViolAbsenceScopeExceeded,
		ViolMissingRequestedRoleUndisclosed,
		ViolBlockCoverageMissing,
		ViolPrincipalClaimUseMissing,
		ViolDiagramEdgeUnsupported,
		ViolAnswerTopicMismatch,
		ViolCurrentStatusVerdictMissing,
		ViolExhaustiveMemberSetCoverageDrift,
		ViolEnumerationLabelUngrounded,
		ViolEnumerationItemLabelExtractorDrift,
		ViolEnumerationLabelHallucinated,
		ViolInlineIdentifierHallucinated,
		ViolLaneBlockKindMismatch,
		ViolPrincipalSupportMemberOmitted,
		ViolScalarCountUnsourced,
		ViolPathDepthInsufficient,
		ViolCardinalityShort,
		ViolEntityParityImbalanced,
	}
	for _, kind := range kinds {
		spec, ok := ViolKindSpecFor(kind)
		if !ok {
			t.Errorf("kind=%q: no registry spec", kind)
			continue
		}
		if !spec.SoftByDefault {
			t.Errorf("kind=%q: SoftByDefault=false; commercial default must caveat instead of retry", kind)
		}
		if !spec.Promotable {
			t.Errorf("kind=%q: Promotable=false; operator strict promotion must stay available", kind)
		}
		strict := ViolationProfileFor(kind, true)
		if !strict.RetryEligible {
			t.Errorf("kind=%q strict-promote: RetryEligible=false want true", kind)
		}
	}
}

// TestDiagramEdgeEndpointHallucinated_CaveatOnly locks the commercial
// boundary for visual diagrams. Mermaid node identifiers are a carrier
// for the rendered diagram, not always a direct code-symbol assertion,
// so unresolved endpoints must never trigger a finalizer retry even
// when an operator lists the kind under strict config.
func TestDiagramEdgeEndpointHallucinated_CaveatOnly(t *testing.T) {
	spec, ok := ViolKindSpecFor(ViolDiagramEdgeEndpointHallucinated)
	if !ok {
		t.Fatal("ViolDiagramEdgeEndpointHallucinated must be registered")
	}
	if spec.DefaultSeverity != SeveritySoft {
		t.Fatalf("DefaultSeverity = %q, want %q", spec.DefaultSeverity, SeveritySoft)
	}
	if !spec.SoftByDefault {
		t.Fatal("SoftByDefault = false, want true")
	}
	if spec.Promotable {
		t.Fatal("Promotable = true, want false")
	}
	if spec.FallbackLocus != LocusTerminal {
		t.Fatalf("FallbackLocus = %q, want %q", spec.FallbackLocus, LocusTerminal)
	}
	if spec.CaveatFamilyID != CaveatFamilyDiagramFidelity {
		t.Fatalf("CaveatFamilyID = %q, want %q", spec.CaveatFamilyID, CaveatFamilyDiagramFidelity)
	}
	for _, strict := range []bool{false, true} {
		profile := ViolationProfileFor(ViolDiagramEdgeEndpointHallucinated, strict)
		if profile.Severity != SeveritySoft || profile.RetryEligible {
			t.Fatalf("strict=%v profile = %+v, want permanent SOFT with no retry", strict, profile)
		}
	}
}

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

// TestDiagramRelationLabelOnly_TelemetryOnly locks the intent-
// preservation boundary for diagram relation metadata: a visible
// Mermaid label that infers the relation is enough for the answer, so
// missing edge_anchors metadata is observable but never user-facing and
// never retry-eligible.
func TestDiagramRelationLabelOnly_TelemetryOnly(t *testing.T) {
	spec, ok := ViolKindSpecFor(ViolDiagramRelationLabelOnly)
	if !ok {
		t.Fatal("ViolDiagramRelationLabelOnly must be registered")
	}
	if spec.DefaultSeverity != SeveritySoft {
		t.Fatalf("DefaultSeverity = %q, want %q", spec.DefaultSeverity, SeveritySoft)
	}
	if !spec.SoftByDefault {
		t.Fatal("SoftByDefault = false, want true")
	}
	if spec.Promotable {
		t.Fatal("Promotable = true, want false")
	}
	if spec.FallbackLocus != LocusTerminal {
		t.Fatalf("FallbackLocus = %q, want %q", spec.FallbackLocus, LocusTerminal)
	}
	if spec.CaveatFamilyID != "" {
		t.Fatalf("CaveatFamilyID = %q, want empty telemetry-only kind", spec.CaveatFamilyID)
	}
	for _, strict := range []bool{false, true} {
		profile := ViolationProfileFor(ViolDiagramRelationLabelOnly, strict)
		if profile.Severity != SeveritySoft || profile.RetryEligible {
			t.Fatalf("strict=%v profile = %+v, want permanent SOFT with no retry", strict, profile)
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

// TestCaveatFamily_AllRegisteredHaveTemplates pins W1.1: every
// CaveatFamilyID a ViolKindSpec references MUST resolve in the
// CaveatFamily registry, and every registered template MUST have
// non-empty ZH and EN strings.
func TestCaveatFamily_AllRegisteredHaveTemplates(t *testing.T) {
	for _, spec := range AllViolKindSpecs() {
		if spec.CaveatFamilyID == "" {
			continue // operator-only ViolKinds intentionally have no caveat
		}
		tmpl, ok := CaveatFamilyTemplateFor(spec.CaveatFamilyID)
		if !ok {
			t.Errorf("ViolKind %q references unknown CaveatFamilyID %q",
				spec.Kind, spec.CaveatFamilyID)
			continue
		}
		if tmpl.ZH == "" || tmpl.EN == "" {
			t.Errorf("CaveatFamily %q has empty ZH=%q or EN=%q",
				spec.CaveatFamilyID, tmpl.ZH, tmpl.EN)
		}
	}
}

// TestCaveatFamily_NoInternalJargon enforces the W1 red-line:
// templates must NOT contain ViolKind names, IR-field tokens, or
// orchestration jargon. Forbidden tokens scanned per template.
func TestCaveatFamily_NoInternalJargon(t *testing.T) {
	forbidden := []string{
		"yield kill", "Pipeline terminated", "retryUsed",
		"ViolKind", "IRField", "conf=", "event(s)",
		"block_items_label", "diagram_edges", "facet_uncovered",
		"answer_authority", "answer_richness_facet_coverage",
	}
	for _, tmpl := range AllCaveatFamilies() {
		for _, body := range []string{tmpl.ZH, tmpl.EN} {
			for _, token := range forbidden {
				if strings.Contains(body, token) {
					t.Errorf("CaveatFamily %q template contains forbidden token %q: %q",
						tmpl.ID, token, body)
				}
			}
		}
	}
}

// TestCaveatFamily_OperatorOnlyKindsHaveNoCaveat pins the
// expectation that pure-telemetry ViolKinds (reviewer / frequency-
// bridge) intentionally carry empty CaveatFamilyID — guarding
// against accidentally adding user-visible caveats for operator
// signals.
func TestCaveatFamily_OperatorOnlyKindsHaveNoCaveat(t *testing.T) {
	operatorOnly := map[ViolationKind]bool{
		ViolPlanCritic:              true,
		ViolReflectorObservation:    true,
		ViolAnswerReviewerDistilled: true,
		ViolDemotionStorm:           true,
		ViolForcedReadStorm:         true,
	}
	for _, spec := range AllViolKindSpecs() {
		if !operatorOnly[spec.Kind] {
			continue
		}
		if spec.CaveatFamilyID != "" {
			t.Errorf("operator-only ViolKind %q must have empty CaveatFamilyID; got %q",
				spec.Kind, spec.CaveatFamilyID)
		}
	}
}

// TestRegisterCaveatFamily_RejectsInvalid covers the panic path
// for empty ID / empty ZH / empty EN — fail-loud at init.
func TestRegisterCaveatFamily_RejectsInvalid(t *testing.T) {
	cases := []struct {
		name string
		t    CaveatFamilyTemplate
	}{
		{"empty_id", CaveatFamilyTemplate{ID: "", ZH: "x", EN: "y"}},
		{"empty_zh", CaveatFamilyTemplate{ID: "x", ZH: "", EN: "y"}},
		{"empty_en", CaveatFamilyTemplate{ID: "x", ZH: "y", EN: ""}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Errorf("expected panic for %s", c.name)
				}
			}()
			RegisterCaveatFamily(c.t)
		})
	}
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
