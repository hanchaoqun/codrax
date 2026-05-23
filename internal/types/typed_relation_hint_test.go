package types

import "testing"

// TestTypedRelationAnchorKindCoverage is the structural gate: every
// value in AllTypedRelations() MUST have a non-empty AnchorKind
// mapping in TypedRelationAnchorKind. Adding a new relation forces
// updating both surfaces.
func TestTypedRelationAnchorKindCoverage(t *testing.T) {
	for _, rel := range AllTypedRelations() {
		ak := TypedRelationAnchorKind(rel)
		if ak == "" {
			t.Errorf("relation %q has empty AnchorKind mapping; add it to TypedRelationAnchorKind switch", rel)
			continue
		}
		// Validate against the closed AnchorKind taxonomy.
		valid := false
		for _, declared := range AllAnchorKinds() {
			if ak == declared {
				valid = true
				break
			}
		}
		if !valid {
			t.Errorf("relation %q maps to AnchorKind %q which is not in AllAnchorKinds; pick a declared kind", rel, ak)
		}
	}
}

// TestTypedRelationAnchorKindUnknownReturnsEmpty pins the fallback:
// unknown relation strings must return "" so dedup never accidentally
// matches against a typo'd value.
func TestTypedRelationAnchorKindUnknownReturnsEmpty(t *testing.T) {
	if got := TypedRelationAnchorKind("not_a_relation"); got != "" {
		t.Errorf("expected empty AnchorKind for unknown relation; got %q", got)
	}
	if got := TypedRelationAnchorKind(""); got != "" {
		t.Errorf("expected empty AnchorKind for empty relation; got %q", got)
	}
}

func TestTypedRelationPrecisionCoverageGateEligibility(t *testing.T) {
	tests := []struct {
		name      string
		precision TypedRelationPrecision
		want      bool
	}{
		{"exact symbol id", TypedRelationPrecisionExactSymbolID, true},
		{"exact file", TypedRelationPrecisionExactFile, true},
		{"exact evidence", TypedRelationPrecisionExactEvidence, true},
		{"name only", TypedRelationPrecisionNameOnly, false},
		{"heuristic", TypedRelationPrecisionHeuristic, false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.precision.CoverageGateEligible(); got != tt.want {
				t.Fatalf("CoverageGateEligible()=%v, want %v", got, tt.want)
			}
		})
	}
}

func TestTypedRelationQueryAllowsKind(t *testing.T) {
	if !((TypedRelationQuery{}).AllowsKind(TypedRelationImplements)) {
		t.Fatalf("empty kind filter should allow known relations")
	}
	q := TypedRelationQuery{Kinds: []TypedRelationKind{TypedRelationImports}}
	if !q.AllowsKind(TypedRelationImports) {
		t.Fatalf("explicit kind filter should allow listed relation")
	}
	if q.AllowsKind(TypedRelationImplements) {
		t.Fatalf("explicit kind filter should reject unlisted relation")
	}
	if q.AllowsKind("") {
		t.Fatalf("empty relation kind must never be allowed")
	}
}

func TestTypedRelationCandidateCoverageGateEligibility(t *testing.T) {
	c := TypedRelationCandidate{
		Relation:  TypedRelationImplements,
		Precision: TypedRelationPrecisionExactSymbolID,
		Member:    TypedRelationMember{Name: "looperImpl", File: "impl.go", Line: 12},
	}
	if !c.CoverageGateEligible() {
		t.Fatalf("exact typed relation candidate should be gate-eligible before evidence/scope checks")
	}
	c.Precision = TypedRelationPrecisionNameOnly
	if c.CoverageGateEligible() {
		t.Fatalf("name-only candidate must not be hard-gate eligible")
	}
	c.Precision = TypedRelationPrecisionExactSymbolID
	c.Relation = ""
	if c.CoverageGateEligible() {
		t.Fatalf("candidate without relation kind must not be hard-gate eligible")
	}
}
