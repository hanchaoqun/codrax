package types

import "testing"

func TestDiagramRelationKind_IsValid(t *testing.T) {
	for _, rk := range AllDiagramRelationKinds() {
		if !rk.IsValid() {
			t.Errorf("AllDiagramRelationKinds member %q reported !IsValid()", rk)
		}
	}
	if DiagramRelUnknown.IsValid() {
		t.Errorf("DiagramRelUnknown MUST NOT be IsValid() — sentinel is not a member")
	}
	if DiagramRelationKind("not-a-real-kind").IsValid() {
		t.Errorf("arbitrary string MUST NOT be IsValid()")
	}
}

func TestClaimFormForRelation_AllKinds(t *testing.T) {
	cases := []struct {
		rk   DiagramRelationKind
		want ClaimForm
	}{
		{DiagramRelCall, ClaimCallEdge},
		{DiagramRelGuard, ClaimGuardCondition},
		{DiagramRelImport, ClaimImportEdge},
		{DiagramRelPrecedence, ClaimPrecedenceRole},
		{DiagramRelObserve, ClaimExternalObservation},
		{DiagramRelContain, ClaimUnknown}, // containment is block-level, not edge
		{DiagramRelUnknown, ClaimUnknown},
	}
	for _, c := range cases {
		if got := ClaimFormForRelation(c.rk); got != c.want {
			t.Errorf("ClaimFormForRelation(%q) = %q, want %q", c.rk, got, c.want)
		}
	}
}

func TestRelationForClaimForm_AllKinds(t *testing.T) {
	cases := []struct {
		cf   ClaimForm
		want DiagramRelationKind
	}{
		{ClaimCallEdge, DiagramRelCall},
		{ClaimGuardCondition, DiagramRelGuard},
		{ClaimImportEdge, DiagramRelImport},
		{ClaimPrecedenceRole, DiagramRelPrecedence},
		{ClaimExternalObservation, DiagramRelObserve},
		{ClaimDefinitionFact, DiagramRelUnknown},
		{ClaimUnknown, DiagramRelUnknown},
	}
	for _, c := range cases {
		if got := RelationForClaimForm(c.cf); got != c.want {
			t.Errorf("RelationForClaimForm(%q) = %q, want %q", c.cf, got, c.want)
		}
	}
}

// 6 relations × 5 label forms (verbatim/keyword-with-context/uppercase/empty/no-match).
// Verifies the typed dictionary is the only signal — no fuzzy or
// repo-specific matching leaks in.
func TestInferRelationFromLabel_RelationGrid(t *testing.T) {
	type form struct {
		name  string
		label string
		want  DiagramRelationKind
	}
	suites := []struct {
		rk    DiagramRelationKind
		forms []form
	}{
		{DiagramRelCall, []form{
			{"verbatim-keyword", "call", DiagramRelCall},
			{"keyword-with-context", "call ProcessRequest", DiagramRelCall},
			{"uppercase", "INVOKE", DiagramRelCall},
			{"empty", "", DiagramRelUnknown},
			{"no-match", "totally-unrelated-arrow-text", DiagramRelUnknown},
		}},
		{DiagramRelGuard, []form{
			{"verbatim-keyword", "guard", DiagramRelGuard},
			{"keyword-with-context", "if x>0", DiagramRelGuard},
			{"uppercase", "WHEN ready", DiagramRelGuard},
			{"empty", "   ", DiagramRelUnknown},
			{"no-match", "abcdef", DiagramRelUnknown},
		}},
		{DiagramRelImport, []form{
			{"verbatim-keyword", "import", DiagramRelImport},
			{"keyword-with-context", "depends on legacy api", DiagramRelImport},
			{"uppercase", "REQUIRES auth", DiagramRelImport},
			{"empty", "", DiagramRelUnknown},
			{"no-match", "xyzzy", DiagramRelUnknown},
		}},
		{DiagramRelPrecedence, []form{
			{"verbatim-keyword", "precedence", DiagramRelPrecedence},
			{"keyword-with-context", "override default", DiagramRelPrecedence},
			{"uppercase", "LAYER 2", DiagramRelPrecedence},
			{"empty", "", DiagramRelUnknown},
			{"no-match", "qqqq", DiagramRelUnknown},
		}},
		{DiagramRelContain, []form{
			{"verbatim-keyword", "contain", DiagramRelContain},
			{"keyword-with-context", "include subsystem", DiagramRelContain},
			{"uppercase", "PART OF cluster", DiagramRelContain},
			{"empty", "", DiagramRelUnknown},
			{"no-match", "zzz", DiagramRelUnknown},
		}},
		{DiagramRelObserve, []form{
			{"verbatim-keyword", "observe", DiagramRelObserve},
			{"keyword-with-context", "log entry on failure", DiagramRelObserve},
			{"uppercase", "METRIC pulse", DiagramRelObserve},
			{"empty", "", DiagramRelUnknown},
			{"no-match", "uvw", DiagramRelUnknown},
		}},
	}
	for _, suite := range suites {
		for _, f := range suite.forms {
			got := InferRelationFromLabel(f.label)
			if got != f.want {
				t.Errorf("[%s/%s] InferRelationFromLabel(%q) = %q, want %q",
					suite.rk, f.name, f.label, got, f.want)
			}
		}
	}
}

// Priority: guard markers MUST win over generic call verbs when
// both appear in the same label (architectural decision — the
// conditional intent is the dominant semantic).
func TestInferRelationFromLabel_GuardWinsOverCall(t *testing.T) {
	cases := []string{
		"if call ready",
		"when invoke handler",
		"branch and call worker",
	}
	for _, label := range cases {
		if got := InferRelationFromLabel(label); got != DiagramRelGuard {
			t.Errorf("InferRelationFromLabel(%q) = %q, want DiagramRelGuard (guard MUST win over call)",
				label, got)
		}
	}
}

// Priority: precedence (specific ordering) wins over import.
func TestInferRelationFromLabel_PrecedenceWinsOverImport(t *testing.T) {
	if got := InferRelationFromLabel("override import path"); got != DiagramRelPrecedence {
		t.Errorf("InferRelationFromLabel(%q) = %q, want DiagramRelPrecedence",
			"override import path", got)
	}
}

func TestDiagramRelationKeywords_Snapshot(t *testing.T) {
	dict := DiagramRelationKeywords()
	if len(dict) == 0 {
		t.Fatalf("DiagramRelationKeywords MUST be non-empty")
	}
	// Every dictionary keyword MUST resolve to a valid relation kind
	// when wrapped in a label context that preserves boundary
	// whitespace (some keywords like "if " require a trailing word
	// boundary to disambiguate from "diff" / "different"). Catches
	// accidental override by an earlier substring after reorders.
	for _, e := range dict {
		label := "edge " + e.Keyword + " marker"
		got := InferRelationFromLabel(label)
		if !got.IsValid() {
			t.Errorf("dictionary keyword %q (label %q) resolved to %q (want valid kind, %q expected)",
				e.Keyword, label, got, e.Kind)
		}
	}
}

// Returned slice MUST be a copy — mutations by callers must not
// leak back into the package-level dictionary.
func TestDiagramRelationKeywords_DefensiveCopy(t *testing.T) {
	dict := DiagramRelationKeywords()
	original := dict[0]
	dict[0].Keyword = "MUTATED"
	dict[0].Kind = DiagramRelUnknown
	again := DiagramRelationKeywords()
	if again[0] != original {
		t.Errorf("DiagramRelationKeywords returned shared slice — mutation leaked: got %+v, want %+v",
			again[0], original)
	}
}
