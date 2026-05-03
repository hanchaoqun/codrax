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
