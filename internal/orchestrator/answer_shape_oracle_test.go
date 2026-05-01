package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestRunAnswerShapeOracle_NilSafeReturns covers the cheap
// short-circuit paths.
func TestRunAnswerShapeOracle_NilSafeReturns(t *testing.T) {
	if got := runAnswerShapeOracle(nil, nil, nil); got != nil {
		t.Errorf("nil inputs should return nil; got %v", got)
	}
	if got := runAnswerShapeOracle(&types.AnswerDocument{}, nil, nil); got != nil {
		t.Errorf("nil rm should return nil; got %v", got)
	}
	if got := runAnswerShapeOracle(nil, &types.RequestModel{}, nil); got != nil {
		t.Errorf("nil doc should return nil; got %v", got)
	}
}

// TestRunAnswerShapeOracle_ExplainShouldNotEmitValue pins the
// commit-53 P2 contract: an Explain / RootCause request producing
// a ShapeValue / ShapeBoolean / ShapeConfigValue answer fires the
// shape_intent_mismatch violation.
func TestRunAnswerShapeOracle_ExplainShouldNotEmitValue(t *testing.T) {
	cases := []struct {
		intent types.Intent
		shape  types.AnswerShape
		expect bool
	}{
		{types.IntentExplain, types.ShapeValue, true},
		{types.IntentExplain, types.ShapeBoolean, true},
		{types.IntentExplain, types.ShapeConfigValue, true},
		{types.IntentRootCause, types.ShapeValue, true},
		{types.IntentExplain, types.ShapeExplanation, false}, // happy path
	}
	for _, c := range cases {
		t.Run(string(c.intent)+"_"+string(c.shape), func(t *testing.T) {
			doc := &types.AnswerDocument{Shape: c.shape}
			rm := &types.RequestModel{Intent: c.intent}
			vs := runAnswerShapeOracle(doc, rm, nil)
			fired := false
			for _, v := range vs {
				if v.Kind == types.ViolShapeIntentMismatch {
					fired = true
				}
			}
			if fired != c.expect {
				t.Errorf("intent=%s shape=%s: violation fired=%v, want %v",
					c.intent, c.shape, fired, c.expect)
			}
		})
	}
}

// TestRunAnswerShapeOracle_ReturnValueIntentRejectsExplanation
// covers the inverse: a value-class intent (return_value /
// config_query) producing an explanation shape.
func TestRunAnswerShapeOracle_ReturnValueIntentRejectsExplanation(t *testing.T) {
	cases := []struct {
		intent types.Intent
		shape  types.AnswerShape
		expect bool
	}{
		{types.IntentReturnValue, types.ShapeExplanation, true},
		{types.IntentConfigQuery, types.ShapeExplanation, true},
		{types.IntentReturnValue, types.ShapeValue, false},
		{types.IntentConfigQuery, types.ShapeConfigValue, false},
	}
	for _, c := range cases {
		t.Run(string(c.intent)+"_"+string(c.shape), func(t *testing.T) {
			doc := &types.AnswerDocument{Shape: c.shape}
			rm := &types.RequestModel{Intent: c.intent}
			vs := runAnswerShapeOracle(doc, rm, nil)
			fired := false
			for _, v := range vs {
				if v.Kind == types.ViolShapeIntentMismatch {
					fired = true
				}
			}
			if fired != c.expect {
				t.Errorf("intent=%s shape=%s: violation fired=%v, want %v",
					c.intent, c.shape, fired, c.expect)
			}
		})
	}
}

// TestRunAnswerShapeOracle_SubTopicCountMismatch pins the
// commit-53 P2 sub-topic divergence check. Default tolerance is
// ±1; +/-2 or beyond fires.
func TestRunAnswerShapeOracle_SubTopicCountMismatch(t *testing.T) {
	cases := []struct {
		name        string
		subTopicN   int
		bucketCount int
		expect      bool
	}{
		{"3 topics 3 buckets — clean", 3, 3, false},
		{"3 topics 2 buckets — within tolerance", 3, 2, false},
		{"3 topics 4 buckets — within tolerance", 3, 4, false},
		{"3 topics 1 bucket — divergence fires", 3, 1, true},
		{"3 topics 6 buckets — divergence fires", 3, 6, true},
		{"single topic — check skipped", 1, 1, false},
		{"single topic 5 buckets — still skipped", 1, 5, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			subTopics := make([]types.SubTopic, c.subTopicN)
			for i := range subTopics {
				subTopics[i] = types.SubTopic{Summary: "topic"}
			}
			symbols := make([]types.AnswerSymbol, c.bucketCount)
			for i := range symbols {
				symbols[i].File = "buck" + string(rune('a'+i)) + ".go"
			}
			doc := &types.AnswerDocument{Symbols: symbols, Shape: types.ShapeListOfSymbols}
			rm := &types.RequestModel{SubTopics: subTopics}
			vs := runAnswerShapeOracle(doc, rm, nil)
			fired := false
			for _, v := range vs {
				if v.Kind == types.ViolSubTopicCountMismatch {
					fired = true
				}
			}
			if fired != c.expect {
				t.Errorf("got fired=%v, want %v (subTopics=%d, buckets=%d)",
					fired, c.expect, c.subTopicN, c.bucketCount)
			}
		})
	}
}

// TestSoftViolationKinds_DefaultSoftKindsAreNotStrict pins the
// default classification. Pre-commit-53 ViolShape / ViolCitation
// etc remain strict (preserved byte-identical); only the 3 new
// commit-53 kinds default to soft.
func TestSoftViolationKinds_Defaults(t *testing.T) {
	SetSoftViolationKinds(nil, nil) // restore defaults

	wantSoft := []types.ViolationKind{
		types.ViolShapeIntentMismatch,
		types.ViolSubTopicCountMismatch,
		types.ViolDiagramIdentifier,
	}
	for _, k := range wantSoft {
		if !softViolationKinds[k] {
			t.Errorf("kind %s should default to soft", k)
		}
	}

	wantStrict := []types.ViolationKind{
		types.ViolShape,
		types.ViolCitation,
		types.ViolMustInclude,
		types.ViolMustExclude,
		types.ViolAcceptance,
	}
	for _, k := range wantStrict {
		if softViolationKinds[k] {
			t.Errorf("legacy kind %s should default to strict (not soft)", k)
		}
	}
}

// TestSoftViolationKinds_YamlOverride pins the operator-override
// path: yaml can promote a soft kind to strict, or demote a
// legacy kind to soft.
func TestSoftViolationKinds_YamlOverride(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })

	// Promote diagram_identifier from soft to strict; demote
	// must_include from strict to soft.
	SetSoftViolationKinds(
		[]string{"must_include"},                // extra soft
		[]string{"diagram_identifier_unverified"}, // extra strict
	)

	if softViolationKinds[types.ViolDiagramIdentifier] {
		t.Error("diagram_identifier should be strict after override")
	}
	if !softViolationKinds[types.ViolMustInclude] {
		t.Error("must_include should be soft after override")
	}
	// Other defaults unchanged.
	if !softViolationKinds[types.ViolShapeIntentMismatch] {
		t.Error("shape_intent_mismatch should remain soft after override")
	}
}

// TestRunAnswerShapeOracle_DeclaredCountDrift pins the commit-55
// Batch A.3 cross-stage check: extractor's emit_answer_symbol
// declared count is persisted on Mutable; if the finalizer
// renders a different number of symbols, ViolDeclaredCountDrift
// fires.
func TestRunAnswerShapeOracle_DeclaredCountDrift(t *testing.T) {
	cases := []struct {
		name        string
		declared    int
		rendered    int
		expectFired bool
	}{
		{"no claim → no fire", 0, 5, false},
		{"matched → no fire", 5, 5, false},
		{"declared 10 rendered 7 → fire", 10, 7, true},
		{"declared 3 rendered 5 → fire", 3, 5, true},
		{"declared 1 rendered 1 → no fire", 1, 1, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mu := types.NewMutableState("test")
			mu.SetEmittedAnswerSymbolDeclaredCount(c.declared)
			symbols := make([]types.AnswerSymbol, c.rendered)
			for i := range symbols {
				symbols[i].File = "f" + string(rune('a'+i)) + ".go"
			}
			doc := &types.AnswerDocument{Symbols: symbols, Shape: types.ShapeListOfSymbols}
			rm := &types.RequestModel{}
			vs := runAnswerShapeOracle(doc, rm, mu)
			fired := false
			for _, v := range vs {
				if v.Kind == types.ViolDeclaredCountDrift {
					fired = true
				}
			}
			if fired != c.expectFired {
				t.Errorf("declared=%d rendered=%d: fired=%v, want %v",
					c.declared, c.rendered, fired, c.expectFired)
			}
		})
	}
}

// TestHasAnyStrictViolation pins the gate predicate. A mix with
// any strict kind returns true; an all-soft mix returns false;
// empty returns false.
func TestHasAnyStrictViolation(t *testing.T) {
	SetSoftViolationKinds(nil, nil)
	cases := []struct {
		name string
		vs   []types.Violation
		want bool
	}{
		{"empty", nil, false},
		{"all soft", []types.Violation{
			{Kind: types.ViolShapeIntentMismatch},
			{Kind: types.ViolDiagramIdentifier},
		}, false},
		{"one strict", []types.Violation{
			{Kind: types.ViolShapeIntentMismatch},
			{Kind: types.ViolCitation},
		}, true},
		{"all strict", []types.Violation{
			{Kind: types.ViolShape},
			{Kind: types.ViolMustInclude},
		}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := hasAnyStrictViolation(c.vs); got != c.want {
				t.Errorf("got %v want %v for %v", got, c.want, c.vs)
			}
		})
	}
}
