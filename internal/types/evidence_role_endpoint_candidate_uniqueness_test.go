package types

import (
	"reflect"
	"testing"
)

func b1642bEndpointCandidate() EvidenceItem {
	return EvidenceItem{
		ID: "call-a-b", Kind: EvidenceDirect, AnchorKind: AnchorCall,
		Subject: "NodeA", Object: "NodeB", AnchorSymbol: "NodeB",
		Source: "src/Edges.go", LineStart: 12, LineEnd: 12, Scope: ScopeLine,
		Origin: ClaimOriginCurrentRepo, EvidenceRef: "source-read-1", Producer: "emit_evidence",
		GroundingStatus: GroundingGrounded,
	}
}

// These public resolvers select a possible citation repair, not a value or
// causal authority. An endpoint alone cannot choose one of several relations
// merely because their declarations occupy the same displayed source line.
func TestB1642BExactEndpointCandidateAmbiguityIsOrderIndependent(t *testing.T) {
	a := b1642bEndpointCandidate()
	forms := []ClaimForm{ClaimCallEdge, ClaimImportEdge, ClaimCallbackHandoff}
	for _, tc := range []struct {
		name string
		edit func(*EvidenceItem)
	}{
		{"same_line_other_destination", func(b *EvidenceItem) { b.Object = "NodeC"; b.AnchorSymbol = "NodeC" }},
		{"same_line_reverse_direction", func(b *EvidenceItem) { b.Subject = "NodeB"; b.Object = "NodeA"; b.AnchorSymbol = "NodeA" }},
		{"same_line_import_kind", func(b *EvidenceItem) { b.AnchorKind = AnchorImport }},
		{"same_line_callback_kind", func(b *EvidenceItem) { b.AnchorKind = AnchorCallback }},
		{"different_line", func(b *EvidenceItem) { b.LineStart = 15; b.LineEnd = 15 }},
		{"different_line_end", func(b *EvidenceItem) { b.LineEnd = 14 }},
		{"unknown_line_end", func(b *EvidenceItem) { b.LineEnd = 0 }},
		{"different_file", func(b *EvidenceItem) { b.Source = "other/Edges.go" }},
		{"case_sensitive_file", func(b *EvidenceItem) { b.Source = "src/edges.go" }},
		{"literal_source_not_trimmed", func(b *EvidenceItem) { b.Source = " src/Edges.go" }},
		{"different_receipt", func(b *EvidenceItem) { b.EvidenceRef = "source-read-2" }},
		{"missing_receipt", func(b *EvidenceItem) { b.EvidenceRef = "" }},
		{"different_producer", func(b *EvidenceItem) { b.Producer = "parser_relations" }},
		{"unknown_origin", func(b *EvidenceItem) { b.Origin = "" }},
		{"different_scope", func(b *EvidenceItem) { b.Scope = ScopeLineRange }},
		{"different_condition", func(b *EvidenceItem) { b.Condition = "featureEnabled" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := a
			b.ID = "candidate-b"
			tc.edit(&b)
			for _, ev := range []EvidenceItem{a, b} {
				if got, ok := UniqueGroundedClaimRoleForExactEndpoint([]EvidenceItem{ev}, forms, "NodeA"); !ok || got.ID != ev.ID {
					t.Fatalf("fixture must independently qualify as a candidate: ok=%v got=%+v", ok, got)
				}
				if EvidenceClaimRoleAssertedByAnswerSurface(ev, forms, "NodeA", "") {
					t.Fatal("fixture must exercise endpoint fallback, not an explicit arrow")
				}
			}
			for _, pool := range [][]EvidenceItem{{a, b}, {b, a}} {
				if !HasAmbiguousGroundedClaimRoleForExactEndpoint(pool, forms, "NodeA") {
					t.Error("competing endpoint candidates were reported as absent or unique")
				}
				if got, ok := UniqueGroundedClaimRoleForExactEndpoint(pool, forms, "NodeA"); ok || !reflect.DeepEqual(got, EvidenceItem{}) {
					t.Errorf("different candidates became one endpoint repair: ok=%v got=%+v", ok, got)
				}
				if got, ok := SelectAnswerItemCitationRole(pool, nil, forms, "NodeA", ""); ok || !reflect.DeepEqual(got, EvidenceItem{}) {
					t.Errorf("shared advisory fallback selected a first-pool candidate: ok=%v got=%+v", ok, got)
				}
			}
		})
	}
}

func TestB1642BExactEndpointDuplicateAndUniqueCompatibility(t *testing.T) {
	a := b1642bEndpointCandidate()
	copy := a
	copy.ID, copy.Summary, copy.Confidence = "copy-id", "independent description", 0.7
	copy.GroundingStatus = GroundingRecovered
	for _, pool := range [][]EvidenceItem{{a}, {a, copy}, {copy, a}} {
		for _, label := range []string{"NodeA", "NodeB", " `NodeA` "} {
			if HasAmbiguousGroundedClaimRoleForExactEndpoint(pool, []ClaimForm{ClaimCallEdge}, label) {
				t.Fatalf("unique or exact duplicate candidate became ambiguous: label=%q", label)
			}
			got, ok := UniqueGroundedClaimRoleForExactEndpoint(pool, []ClaimForm{ClaimCallEdge}, label)
			if !ok || !reflect.DeepEqual(got, pool[0]) {
				t.Fatalf("same exact candidate or unique endpoint was lost: label=%q ok=%v got=%+v", label, ok, got)
			}
			got, ok = SelectAnswerItemCitationRole(pool, nil, []ClaimForm{ClaimCallEdge}, label, "")
			if !ok || !reflect.DeepEqual(got, pool[0]) {
				t.Fatalf("shared unique endpoint fallback changed: label=%q ok=%v got=%+v", label, ok, got)
			}
		}
	}
	legacy := a
	legacy.Origin, legacy.EvidenceRef, legacy.Producer = "", "", ""
	legacy.LineEnd, legacy.GroundingStatus = 0, ""
	legacyCopy := legacy
	legacyCopy.ID = "legacy-copy"
	if got, ok := UniqueGroundedClaimRoleForExactEndpoint([]EvidenceItem{legacy, legacyCopy}, []ClaimForm{ClaimCallEdge}, "NodeA"); !ok || got.ID != legacy.ID {
		t.Fatal("legacy unique relation admission must not acquire a new receipt or status requirement")
	}
	if HasAmbiguousGroundedClaimRoleForExactEndpoint([]EvidenceItem{legacy, legacyCopy}, []ClaimForm{ClaimCallEdge}, "NodeA") {
		t.Fatal("legacy exact duplicates became ambiguous")
	}
	other := a
	other.ID, other.Object, other.AnchorSymbol = "call-a-c", "NodeC", "NodeC"
	for _, pool := range [][]EvidenceItem{{a, other}, {other, a}} {
		got, ok := SelectAnswerItemCitationRole(pool, []EvidenceItem{other}, []ClaimForm{ClaimCallEdge}, "NodeA -> NodeC", "")
		if !ok || !reflect.DeepEqual(got, other) {
			t.Fatal("a current citation for the explicit directed relation must remain selected")
		}
	}
	// Candidate identity is intentionally stricter than the existing claim-role
	// predicate. This repair must not turn that predicate into a source gate.
	other = a
	other.ID, other.Source, other.LineStart = "other-location", "other.go", 99
	if !SameEvidenceClaimRole(a, other) || ClaimFormOf(other) != ClaimCallEdge {
		t.Fatal("claim-role and claim-form semantics changed with candidate uniqueness")
	}
}

func TestB1642BExactEndpointIgnoresNonCandidates(t *testing.T) {
	a := b1642bEndpointCandidate()
	for _, tc := range []struct {
		name string
		edit func(*EvidenceItem)
	}{
		{"unrelated_endpoints", func(b *EvidenceItem) { b.Subject = "NodeX"; b.Object = "NodeY" }},
		{"definition", func(b *EvidenceItem) { b.AnchorKind = AnchorDefinition }},
		{"disallowed_relation", func(b *EvidenceItem) { b.AnchorKind = AnchorImport }},
		{"ungrounded", func(b *EvidenceItem) { b.GroundingStatus = GroundingUngrounded }},
		{"missing_source", func(b *EvidenceItem) { b.Source = "" }},
		{"unknown_line", func(b *EvidenceItem) { b.LineStart = 0 }},
		{"negative_line", func(b *EvidenceItem) { b.LineStart = -1 }},
		{"external_observation", func(b *EvidenceItem) { b.Origin = ClaimOriginPerf }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := a
			b.ID = "non-candidate"
			tc.edit(&b)
			for _, pool := range [][]EvidenceItem{{a, b}, {b, a}} {
				if HasAmbiguousGroundedClaimRoleForExactEndpoint(pool, []ClaimForm{ClaimCallEdge}, "NodeA") {
					t.Error("a non-candidate introduced endpoint ambiguity")
				}
				if got, ok := UniqueGroundedClaimRoleForExactEndpoint(pool, []ClaimForm{ClaimCallEdge}, "NodeA"); !ok || !reflect.DeepEqual(got, a) {
					t.Fatalf("non-candidate hid a unique legal relation: ok=%v got=%+v", ok, got)
				}
			}
			if _, ok := UniqueGroundedClaimRoleForExactEndpoint([]EvidenceItem{b}, []ClaimForm{ClaimCallEdge}, "NodeA"); ok {
				t.Fatal("non-candidate became an endpoint repair")
			}
			if HasAmbiguousGroundedClaimRoleForExactEndpoint([]EvidenceItem{b}, []ClaimForm{ClaimCallEdge}, "NodeA") {
				t.Fatal("absence of eligible endpoint candidates became ambiguity")
			}
		})
	}
	for _, tc := range []struct {
		pool  []EvidenceItem
		forms []ClaimForm
		label string
	}{
		{nil, []ClaimForm{ClaimCallEdge}, "NodeA"},
		{[]EvidenceItem{a}, nil, "NodeA"},
		{[]EvidenceItem{a}, []ClaimForm{ClaimCallEdge}, ""},
		{[]EvidenceItem{a}, []ClaimForm{ClaimCallEdge}, "Node"},
	} {
		if _, ok := UniqueGroundedClaimRoleForExactEndpoint(tc.pool, tc.forms, tc.label); ok {
			t.Fatal("empty or inexact endpoint query acquired a candidate")
		}
		if HasAmbiguousGroundedClaimRoleForExactEndpoint(tc.pool, tc.forms, tc.label) {
			t.Fatal("empty or inexact endpoint query became ambiguous")
		}
	}
}
