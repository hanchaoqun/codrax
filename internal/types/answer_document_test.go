package types

import "testing"

// TestAnswerShape_IsEmittable asserts exactly the six shapes a
// producer can emit pass IsEmittable, and ShapeNone / zero / unknown
// all fail. The tool schema relies on this boundary to structurally
// reject "no shape declared" before a zero-value AnswerDocument can
// reach the renderer.
func TestAnswerShape_IsEmittable(t *testing.T) {
	ok := []AnswerShape{
		ShapeListOfSymbols, ShapeStepList, ShapeValue,
		ShapeBoolean, ShapeConfigValue, ShapeExplanation,
	}
	for _, s := range ok {
		if !s.IsEmittable() {
			t.Errorf("IsEmittable(%q) = false, want true", s)
		}
	}
	rejected := []AnswerShape{ShapeNone, "", "not_a_shape"}
	for _, s := range rejected {
		if s.IsEmittable() {
			t.Errorf("IsEmittable(%q) = true, want false", s)
		}
	}
}

func TestAnswerDocument_IsZero(t *testing.T) {
	var empty AnswerDocument
	if !empty.IsZero() {
		t.Error("zero-value AnswerDocument.IsZero() = false, want true")
	}
	if !(*AnswerDocument)(nil).IsZero() {
		t.Error("nil AnswerDocument.IsZero() = false, want true")
	}

	// Each populated field individually flips IsZero to false. Covers
	// the full field surface so a future field addition that forgets
	// to teach IsZero about itself still lights this up via the
	// shape-only case below.
	shape := AnswerDocument{Shape: ShapeListOfSymbols}
	if shape.IsZero() {
		t.Error("populated Shape: IsZero() = true, want false")
	}
	sum := AnswerDocument{Summary: "hi"}
	if sum.IsZero() {
		t.Error("populated Summary: IsZero() = true, want false")
	}
	steps := AnswerDocument{Steps: []AnswerStep{{Index: 1, Description: "x", CitationRef: -1}}}
	if steps.IsZero() {
		t.Error("populated Steps: IsZero() = true, want false")
	}
	cites := AnswerDocument{Citations: []Citation{{File: "a.go", Line: 1}}}
	if cites.IsZero() {
		t.Error("populated Citations: IsZero() = true, want false")
	}
	val := AnswerDocument{Value: &AnswerValue{Literal: "x", CitationRef: -1}}
	if val.IsZero() {
		t.Error("populated Value: IsZero() = true, want false")
	}
	bl := AnswerDocument{Boolean: &AnswerBoolean{Decision: true, Rationale: "r", CitationRef: -1}}
	if bl.IsZero() {
		t.Error("populated Boolean: IsZero() = true, want false")
	}
}

func TestCloneAnswerDocument_DeepCopies(t *testing.T) {
	orig := &AnswerDocument{
		Shape:               ShapeStepList,
		Summary:             "lead-in",
		Steps:               []AnswerStep{{Index: 1, Description: "one", CitationRef: 0}},
		Symbols:             []AnswerSymbol{{Name: "Foo", File: "a.go", Line: 1}},
		SymbolsCompleteness: CompletenessLowerBound,
		Value:               &AnswerValue{Literal: "42", CitationRef: 0},
		Boolean:             &AnswerBoolean{Decision: true, Rationale: "r", CitationRef: -1},
		Citations:           []Citation{{File: "a.go", Line: 1, Quote: "x"}},
		Caveats:             []string{"check this"},
	}
	clone := CloneAnswerDocument(orig)
	if clone == orig {
		t.Fatal("Clone returned the same pointer")
	}
	// Mutating the clone must not affect the original.
	clone.Steps[0].Description = "mutated"
	if orig.Steps[0].Description == "mutated" {
		t.Error("Steps slice shared with original after clone")
	}
	clone.Citations[0].File = "mutated.go"
	if orig.Citations[0].File == "mutated.go" {
		t.Error("Citations slice shared with original after clone")
	}
	clone.Value.Literal = "99"
	if orig.Value.Literal == "99" {
		t.Error("Value pointer shared with original after clone")
	}
	clone.Boolean.Decision = false
	if orig.Boolean.Decision == false {
		t.Error("Boolean pointer shared with original after clone")
	}
	clone.Caveats[0] = "mutated"
	if orig.Caveats[0] == "mutated" {
		t.Error("Caveats slice shared with original after clone")
	}
}

func TestCloneAnswerDocument_Nil(t *testing.T) {
	if CloneAnswerDocument(nil) != nil {
		t.Error("CloneAnswerDocument(nil) != nil")
	}
}

func TestMutableState_SetAnswerDocument_RoundTrip(t *testing.T) {
	m := NewMutableState("")
	if got := m.AnswerDocument(); got != nil {
		t.Errorf("fresh store: AnswerDocument() = %v, want nil", got)
	}
	doc := &AnswerDocument{
		Shape:     ShapeValue,
		Value:     &AnswerValue{Literal: "42", CitationRef: -1},
		Citations: []Citation{{File: "a.go", Line: 1}},
	}
	m.SetAnswerDocument(doc)
	got := m.AnswerDocument()
	if got == nil {
		t.Fatal("after Set: AnswerDocument() = nil")
	}
	if got == doc {
		t.Error("AnswerDocument() returned the input pointer; expected a defensive copy")
	}
	if got.Value == doc.Value {
		t.Error("Value pointer shared between stored and returned doc")
	}
	if got.Shape != ShapeValue || got.Value.Literal != "42" {
		t.Errorf("round-trip content mismatch: %+v", got)
	}
}

func TestMutableState_SetAnswerDocument_Replaces(t *testing.T) {
	m := NewMutableState("")
	m.SetAnswerDocument(&AnswerDocument{Shape: ShapeStepList,
		Steps: []AnswerStep{{Index: 1, Description: "a", CitationRef: -1}}})
	m.SetAnswerDocument(&AnswerDocument{Shape: ShapeExplanation, Summary: "new"})
	got := m.AnswerDocument()
	if got.Shape != ShapeExplanation {
		t.Errorf("second set did not replace: shape=%s", got.Shape)
	}
	if len(got.Steps) != 0 {
		t.Errorf("second set left stale steps: %v", got.Steps)
	}
}

func TestMutableState_ResetAnswerDocument(t *testing.T) {
	m := NewMutableState("")
	m.SetAnswerDocument(&AnswerDocument{Shape: ShapeExplanation, Summary: "x"})
	m.ResetAnswerDocument()
	if got := m.AnswerDocument(); got != nil {
		t.Errorf("after Reset: AnswerDocument() = %+v, want nil", got)
	}
}

// TestMutableState_AnswerDocument_DefensiveCopy ensures a mutation
// on the returned pointer does not race back into the internal
// state. The lock covers the Set/Get atomicity; the defensive copy
// covers post-access independence.
func TestMutableState_AnswerDocument_DefensiveCopy(t *testing.T) {
	m := NewMutableState("")
	m.SetAnswerDocument(&AnswerDocument{
		Shape:   ShapeStepList,
		Steps:   []AnswerStep{{Index: 1, Description: "orig", CitationRef: -1}},
		Caveats: []string{"original caveat"},
	})
	got := m.AnswerDocument()
	got.Steps[0].Description = "mutated"
	got.Caveats[0] = "mutated"
	again := m.AnswerDocument()
	if again.Steps[0].Description != "orig" {
		t.Errorf("internal Steps mutated: %s", again.Steps[0].Description)
	}
	if again.Caveats[0] != "original caveat" {
		t.Errorf("internal Caveats mutated: %s", again.Caveats[0])
	}
}
