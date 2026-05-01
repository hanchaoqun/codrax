package types

import "testing"

// TestRequirementToEvidenceKinds_Closure asserts the canonical
// invariants that bind the three Kind namespaces together:
//
//  1. Every RequirementKind in AllRequirementKinds() has an entry in
//     RequirementToEvidenceKinds with len >= 1. No question-side
//     classification may exist without at least one EvidenceKind that
//     can contribute to satisfying it.
//
//  2. Every EvidenceKind mapped in RequirementToEvidenceKinds is one
//     of the canonical kinds declared in AllEvidenceKinds(). The map
//     cannot reference a stray or misspelled kind that no producer
//     will ever emit.
//
//  3. No entry in RequirementToEvidenceKinds references ReqUnknown
//     (the zero value) — it is never a real requirement.
//
// Before this test existed the mapping was implicit in a seven-case
// switch inside explorer_erm.checkRequirementSatisfaction. Adding a
// new RequirementKind required remembering to update a switch that
// had no compile-time guard at all; a missed case silently produced
// zero-satisfaction for the new kind and the ERM layer would loop
// forever. The closure test is the guard that stops that class of
// drift from returning.
func TestRequirementToEvidenceKinds_Closure(t *testing.T) {
	// Build a set of canonical EvidenceKinds for membership checks.
	canonicalEv := make(map[EvidenceKind]bool, len(allEvidenceKinds))
	for _, k := range allEvidenceKinds {
		canonicalEv[k] = true
	}

	// Invariant 1: every non-zero RequirementKind has a non-empty
	// mapping entry.
	for _, rk := range AllRequirementKinds() {
		ev, ok := RequirementToEvidenceKinds[rk]
		if !ok {
			t.Errorf("RequirementToEvidenceKinds missing entry for %q — every RequirementKind must map to at least one EvidenceKind", rk)
			continue
		}
		if len(ev) == 0 {
			t.Errorf("RequirementToEvidenceKinds[%q] is empty — must have at least one EvidenceKind", rk)
		}
		// Invariant 2: every mapped EvidenceKind is canonical.
		for _, k := range ev {
			if !canonicalEv[k] {
				t.Errorf("RequirementToEvidenceKinds[%q] references unknown EvidenceKind %q", rk, k)
			}
		}
	}

	// Invariant 3: ReqUnknown must not appear as a key.
	if _, ok := RequirementToEvidenceKinds[ReqUnknown]; ok {
		t.Errorf("RequirementToEvidenceKinds must not contain ReqUnknown (the zero value)")
	}

	// Also: every RequirementKind in the map must itself be in
	// AllRequirementKinds() — catches a map entry for a deleted kind.
	canonicalReq := make(map[RequirementKind]bool, len(allRequirementKinds))
	for _, k := range allRequirementKinds {
		canonicalReq[k] = true
	}
	for k := range RequirementToEvidenceKinds {
		if !canonicalReq[k] {
			t.Errorf("RequirementToEvidenceKinds has stray entry %q that is not in AllRequirementKinds()", k)
		}
	}
}

// TestEvidenceKind_IsLLMEmittable_Partition asserts that exactly the
// six "investigation-shape" kinds (including EvidenceAbsent re-enabled
// 2026-05+ for ScopeNegative) are LLM-emittable, and the five
// deterministic-only kinds are not. This is the source of truth the
// emit_evidence tool schema is derived from; a regression here would
// silently let the LLM emit concrete_value / dataflow_path / etc.,
// which would launder unverified claims through a channel whose
// semantics say "I did the deterministic check".
func TestEvidenceKind_IsLLMEmittable_Partition(t *testing.T) {
	want := map[EvidenceKind]bool{
		EvidenceDirect:       true,
		EvidenceConditional:  true,
		EvidenceRegistration: true,
		EvidenceMechanism:    true,
		EvidenceRelationship: true,
		// EvidenceAbsent re-enabled 2026-05+: ScopeNegative gives the
		// kind a typed home (NegativeQuery + NegativeScope replace
		// line_start as the verification target). Pairing rule
		// enforced by EvidenceItem.ValidateScope: kind=absent REQUIRES
		// scope=negative.
		EvidenceAbsent:       true,
		EvidenceConcrete:     false,
		EvidenceDataflowPath: false,
		EvidenceConflict:     false,
		EvidenceUnresolved:   false,
		EvidenceTruncated:    false,
	}
	for _, k := range AllEvidenceKinds() {
		expect, known := want[k]
		if !known {
			t.Errorf("AllEvidenceKinds() contains %q but the IsLLMEmittable partition test has no expectation for it — update this test when adding a new EvidenceKind", k)
			continue
		}
		if got := k.IsLLMEmittable(); got != expect {
			t.Errorf("IsLLMEmittable(%q) = %v, want %v", k, got, expect)
		}
	}
	// Symmetric: the test table must cover exactly AllEvidenceKinds()
	// with no stray rows for deleted kinds.
	if len(want) != len(allEvidenceKinds) {
		t.Errorf("partition test has %d entries, AllEvidenceKinds() has %d — table out of sync", len(want), len(allEvidenceKinds))
	}
}

// TestNormalizeRequirementKind_Synonyms asserts the synonym coercion
// table lands every accepted form on a valid RequirementKind. This
// pins the behaviour that was previously duplicated across
// tool/emit_analysis.normalizeQuestionKind and
// agent/explorer_erm.extractEvidenceRequirementsWithHint — both now
// delegate to this function, so a regression here would break both.
func TestNormalizeRequirementKind_Synonyms(t *testing.T) {
	cases := []struct {
		in   string
		want RequirementKind
	}{
		{"registration", ReqRegistration},
		{"register", ReqRegistration},
		{"mechanism", ReqMechanism},
		{"process", ReqMechanism},
		{"flow", ReqMechanism},
		{"return_value", ReqReturnValue},
		{"return-value", ReqReturnValue},
		{"return", ReqReturnValue},
		{"value_of", ReqReturnValue},
		{"history", ReqHistory},
		{"git_history", ReqHistory},
		{"history-lookup", ReqHistory},
		{"authorship", ReqHistory},
		{"commit_lookup", ReqHistory},
		{"conditional", ReqConditional},
		{"condition", ReqConditional},
		{"config_mapping", ReqConfigMapping},
		{"config-mapping", ReqConfigMapping},
		{"enumeration", ReqEnumeration},
		{"enumerate", ReqEnumeration},
		{"list", ReqEnumeration},
		{"count", ReqEnumeration},
		{"call_chain", ReqCallChain},
		{"call-chain", ReqCallChain},
		{"callchain", ReqCallChain},
		{"calls", ReqCallChain},
		{"", ReqUnknown},
		{"unknown", ReqUnknown},
		{"   REGISTRATION  ", ReqRegistration}, // case + whitespace insensitive
		{"garbage_nonsense", ReqUnknown},
	}
	for _, c := range cases {
		if got := NormalizeRequirementKind(c.in); got != c.want {
			t.Errorf("NormalizeRequirementKind(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestEvidenceStructurallyMatchesRequirement(t *testing.T) {
	callEv := EvidenceItem{
		Kind:       EvidenceDirect,
		Predicate:  "calls",
		AnchorKind: AnchorCall,
	}
	if !EvidenceStructurallyMatchesRequirement(callEv, ReqCallChain) {
		t.Fatal("call-shaped direct evidence should satisfy call_chain structurally")
	}
	if !EvidenceStructurallyMatchesRequirement(callEv, ReqMechanism) {
		t.Fatal("call-shaped direct evidence should satisfy mechanism structurally")
	}
	if EvidenceStructurallyMatchesRequirement(callEv, ReqConditional) {
		t.Fatal("call-shaped direct evidence must not satisfy conditional")
	}

	condEv := EvidenceItem{
		Kind:       EvidenceDirect,
		Predicate:  "guards",
		AnchorKind: AnchorCondition,
	}
	if !EvidenceStructurallyMatchesRequirement(condEv, ReqConditional) {
		t.Fatal("condition anchors should satisfy conditional structurally")
	}
	if !EvidenceStructurallyMatchesRequirement(condEv, ReqMechanism) {
		t.Fatal("condition anchors should contribute to mechanism structurally")
	}

	defEv := EvidenceItem{
		Kind:       EvidenceDirect,
		Predicate:  "defines",
		AnchorKind: AnchorDefinition,
	}
	if EvidenceStructurallyMatchesRequirement(defEv, ReqMechanism) {
		t.Fatal("plain definitions must not satisfy mechanism structurally")
	}
}
