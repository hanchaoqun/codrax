package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestDropCitationCountGE verifies the helper preserves non-matching
// criteria in order, removes every matching one, and never returns
// the caller's backing array.
func TestDropCitationCountGE(t *testing.T) {
	in := []types.Criterion{
		{Kind: types.CritContainsSymbol, Expr: "Foo"},
		{Kind: types.CritCitationCountGE, Expr: "1"},
		{Kind: types.CritRegexMatch, Expr: `\d+`},
		{Kind: types.CritCitationCountGE, Expr: "3"},
	}
	got := dropCitationCountGE(in)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Kind != types.CritContainsSymbol || got[1].Kind != types.CritRegexMatch {
		t.Errorf("unexpected surviving kinds: %+v", got)
	}

	// nil / empty passthrough.
	if out := dropCitationCountGE(nil); out != nil {
		t.Errorf("nil input should return nil, got %+v", out)
	}
	if out := dropCitationCountGE([]types.Criterion{}); len(out) != 0 {
		t.Errorf("empty input should stay empty, got %+v", out)
	}
}

// TestBuildAnalysisIR_CountQuestionStripsAllThreeGates verifies that
// a count question (predicates.is_count_question=true) produces zero
// citation gate anywhere in the IR:
//
//   1. AnswerContract.CitationReq.Required == false
//   2. No CritCitationCountGE entries in AnswerContract.AcceptanceTests
//   3. No CritCitationCountGE entries on any TaskNode.SuccessCriteria
//
// Schema-v4 rewrite: the trigger was the leading count-verb prose cue
// in v3; now the LLM emits is_count_question=true directly and the
// downstream carve-out fires off that.
func TestBuildAnalysisIR_CountQuestionStripsAllThreeGates(t *testing.T) {
	mut := types.NewMutableState("how many lines of go code in this project")
	mut.SetRequestModel(types.RequestModel{
		RawRequest: "how many lines of go code in this project",
		Intent:     types.IntentReturnValue, // self-consistency requires this when is_count=true
		Complexity: types.ComplexitySimple,
		AnalyzerHints: types.AnalyzerHints{
			Keywords: []string{"go", "lines", "count"},
			Entities: []string{},
			Shape:    string(types.ShapeValue),
		},
		Predicates: types.SemanticPredicates{
			IsScalarAnswer:  true,
			IsCountQuestion: true,
		},
	})
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mut}

	ir, err := buildAnalysisIR(ctx)
	if err != nil {
		t.Fatalf("buildAnalysisIR: %v", err)
	}

	if ir.AnswerContract.CitationReq.Required {
		t.Error("AnswerContract.CitationReq.Required should be false")
	}
	if got := ir.AnswerContract.CitationReq.MinCitations; got != 0 {
		t.Errorf("AnswerContract.CitationReq.MinCitations = %d, want 0", got)
	}
	for _, a := range ir.AnswerContract.AcceptanceTests {
		if a.Kind == types.CritCitationCountGE {
			t.Errorf("AcceptanceTests still carries CritCitationCountGE: %+v", a)
		}
	}
	for _, n := range ir.TaskGraph.Nodes {
		for _, c := range n.SuccessCriteria {
			if c.Kind == types.CritCitationCountGE {
				t.Errorf("TaskNode %q SuccessCriteria still carries CritCitationCountGE: %+v", n.ID, c)
			}
		}
	}
}

// TestBuildAnalysisIR_ReturnValueWithoutCountCue_KeepsGates is the
// negative control — a regular return_value question ("what does
// function F return") has a file:line to cite, so the carve-out must
// NOT fire when is_count_question is false.
func TestBuildAnalysisIR_ReturnValueWithoutCountCue_KeepsGates(t *testing.T) {
	mut := types.NewMutableState("what does function F return")
	mut.SetRequestModel(types.RequestModel{
		RawRequest: "what does function F return",
		Intent:     types.IntentReturnValue,
		Complexity: types.ComplexitySimple,
		AnalyzerHints: types.AnalyzerHints{
			Keywords: []string{"function", "F", "return"},
			Entities: []string{"F"},
			Shape:    string(types.ShapeValue),
		},
		Predicates: types.SemanticPredicates{
			IsScalarAnswer:  true,
			IsCountQuestion: false, // regular return_value, not a count
		},
	})
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mut}

	ir, err := buildAnalysisIR(ctx)
	if err != nil {
		t.Fatalf("buildAnalysisIR: %v", err)
	}
	if !ir.AnswerContract.CitationReq.Required {
		t.Error("CitationReq.Required must stay true for non-count return_value questions")
	}
}

// TestBuildAnalysisIR_NonCountQuestionKeepsGates is the negative
// control on the enumerate path — a regular enumerate question
// (predicates.is_count_question=false) must NOT have the citation
// gates stripped.
func TestBuildAnalysisIR_NonCountQuestionKeepsGates(t *testing.T) {
	mut := types.NewMutableState("list all X that match Y")
	mut.SetRequestModel(types.RequestModel{
		RawRequest: "list all X that match Y",
		Intent:     types.IntentEnumerate,
		Complexity: types.ComplexitySimple,
		AnalyzerHints: types.AnalyzerHints{
			Keywords: []string{"list", "X", "Y"},
			Entities: []string{"X", "Y"},
			Shape:    string(types.ShapeListOfSymbols),
		},
		Predicates: types.SemanticPredicates{
			IsScalarAnswer:  false,
			IsCountQuestion: false,
		},
	})
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mut}

	ir, err := buildAnalysisIR(ctx)
	if err != nil {
		t.Fatalf("buildAnalysisIR: %v", err)
	}

	if ir.RequestModel.Intent != types.IntentEnumerate {
		t.Errorf("Intent = %q, want enumerate (rule over-fired)", ir.RequestModel.Intent)
	}
	if !ir.AnswerContract.CitationReq.Required {
		t.Error("AnswerContract.CitationReq.Required must remain true for legit enumerate")
	}
	foundAT := false
	for _, a := range ir.AnswerContract.AcceptanceTests {
		if a.Kind == types.CritCitationCountGE {
			foundAT = true
		}
	}
	if !foundAT {
		t.Error("AcceptanceTests should still carry CritCitationCountGE on legit enumerate")
	}
	foundSC := false
	for _, n := range ir.TaskGraph.Nodes {
		for _, c := range n.SuccessCriteria {
			if c.Kind == types.CritCitationCountGE {
				foundSC = true
			}
		}
	}
	if !foundSC {
		t.Error("At least one TaskNode.SuccessCriteria should still carry CritCitationCountGE")
	}
}

// TestReconcileIntent — defense-in-depth against (intent=enumerate +
// is_count_question=true) slipping past self-consistency. In normal
// operation validateSelfConsistency in emit_analysis rejects this
// combination upstream and reconcileIntent never sees it.
//
// The bundle parameter (session 20) replaced the older hasLogStack
// bool: reconcileIntent reads bundle.IntentHint directly. A nil
// bundle is the no-log case; a bundle with IntentHint=RootCause is
// the "log_triage stage emitted a bundle with a real stack" case.
func TestReconcileIntent(t *testing.T) {
	rootCauseBundle := &types.LogBundle{IntentHint: types.IntentRootCause}
	cases := []struct {
		name       string
		declared   types.Intent
		preds      types.SemanticPredicates
		bundle     *types.LogBundle
		want       types.Intent
		wantReason bool
	}{
		{"enumerate + is_count_question downgrades to return_value",
			types.IntentEnumerate, types.SemanticPredicates{IsCountQuestion: true}, nil,
			types.IntentReturnValue, true},
		{"enumerate without is_count_question untouched",
			types.IntentEnumerate, types.SemanticPredicates{}, nil,
			types.IntentEnumerate, false},
		{"explain pass-through with is_count_question",
			types.IntentExplain, types.SemanticPredicates{IsCountQuestion: true}, nil,
			types.IntentExplain, false},
		{"return_value pass-through",
			types.IntentReturnValue, types.SemanticPredicates{IsCountQuestion: true}, nil,
			types.IntentReturnValue, false},
		{"log_triage bundle with root_cause hint forces root_cause",
			types.IntentExplain, types.SemanticPredicates{}, rootCauseBundle,
			types.IntentRootCause, true},
		{"log_triage bundle no-op when already root_cause",
			types.IntentRootCause, types.SemanticPredicates{}, rootCauseBundle,
			types.IntentRootCause, false},
		{"count-question wins over log_triage bundle (exotic but ordered)",
			types.IntentEnumerate, types.SemanticPredicates{IsCountQuestion: true}, rootCauseBundle,
			types.IntentReturnValue, true},
		{"nil bundle skips the log-triage override",
			types.IntentExplain, types.SemanticPredicates{}, nil,
			types.IntentExplain, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, reason := reconcileIntent(c.declared, c.preds, c.bundle)
			if got != c.want {
				t.Errorf("intent = %q, want %q", got, c.want)
			}
			if c.wantReason && reason == "" {
				t.Error("expected non-empty reason")
			}
			if !c.wantReason && reason != "" {
				t.Errorf("expected empty reason, got %q", reason)
			}
		})
	}
}

// TestReconcileShape pins the rules in the schema-v4 reconcileShape:
// Rule 1a (count/category + relational lookup → list_of_symbols),
// Rule 1b (category enumeration alone → list_of_symbols), and Rule 2
// (config_value → value when subject is a source-code literal).
// Inputs are typed predicates + AnswerSubject — no raw prose.
func TestReconcileShape(t *testing.T) {
	cases := []struct {
		name     string
		declared types.AnswerShape
		subject  types.AnswerSubject
		preds    types.SemanticPredicates
		want     types.AnswerShape
		wantFire bool
	}{
		// Rule 1a: count + relational lookup lifts value → list_of_symbols.
		{"value + count + relational → list_of_symbols",
			types.ShapeValue, types.AnswerSubject{},
			types.SemanticPredicates{IsCountQuestion: true, IsRelationalLookup: true},
			types.ShapeListOfSymbols, true},
		{"config_value + count + relational → list_of_symbols",
			types.ShapeConfigValue, types.AnswerSubject{Kind: types.SubjectFunctionName},
			types.SemanticPredicates{IsCountQuestion: true, IsRelationalLookup: true},
			types.ShapeListOfSymbols, true},
		// Rule 1a: category + relational lookup also lifts.
		{"value + category + relational → list_of_symbols",
			types.ShapeValue, types.AnswerSubject{},
			types.SemanticPredicates{IsCategoryEnumeration: true, IsRelationalLookup: true},
			types.ShapeListOfSymbols, true},
		// Rule 1a negatives: count without relational stays value.
		{"value + count alone stays value",
			types.ShapeValue, types.AnswerSubject{},
			types.SemanticPredicates{IsCountQuestion: true},
			types.ShapeValue, false},
		{"value + relational alone stays value",
			types.ShapeValue, types.AnswerSubject{},
			types.SemanticPredicates{IsRelationalLookup: true},
			types.ShapeValue, false},
		// Rule 1b: category enumeration alone (no relational) still lifts.
		{"value + category alone lifts (Rule 1b)",
			types.ShapeValue, types.AnswerSubject{Kind: types.SubjectEnumValue},
			types.SemanticPredicates{IsCategoryEnumeration: true},
			types.ShapeListOfSymbols, true},
		{"config_value + category alone lifts",
			types.ShapeConfigValue, types.AnswerSubject{},
			types.SemanticPredicates{IsCategoryEnumeration: true},
			types.ShapeListOfSymbols, true},
		// Non-value/config_value shapes are untouched by Rules 1a/1b.
		{"explanation untouched by Rule 1",
			types.ShapeExplanation, types.AnswerSubject{},
			types.SemanticPredicates{IsCountQuestion: true, IsRelationalLookup: true},
			types.ShapeExplanation, false},
		{"list_of_symbols untouched by Rule 1",
			types.ShapeListOfSymbols, types.AnswerSubject{},
			types.SemanticPredicates{IsCategoryEnumeration: true},
			types.ShapeListOfSymbols, false},
		// Rule 2: config_value → value for source-code literal subjects.
		{"config_value + function subject → value",
			types.ShapeConfigValue, types.AnswerSubject{Kind: types.SubjectFunctionName},
			types.SemanticPredicates{}, types.ShapeValue, true},
		{"config_value + type subject → value",
			types.ShapeConfigValue, types.AnswerSubject{Kind: types.SubjectTypeName},
			types.SemanticPredicates{}, types.ShapeValue, true},
		{"config_value + return subject → value",
			types.ShapeConfigValue, types.AnswerSubject{Kind: types.SubjectReturnValue},
			types.SemanticPredicates{}, types.ShapeValue, true},
		// Rule 2 negative: config_value + config_key subject stays.
		{"config_value + config_key subject stays",
			types.ShapeConfigValue, types.AnswerSubject{Kind: types.SubjectConfigKey},
			types.SemanticPredicates{}, types.ShapeConfigValue, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, reason := reconcileShape(c.declared, c.subject, c.preds)
			if got != c.want {
				t.Errorf("shape = %q, want %q (reason=%q)", got, c.want, reason)
			}
			if c.wantFire && reason == "" {
				t.Errorf("expected rule to fire with a non-empty reason")
			}
			if !c.wantFire && reason != "" {
				t.Errorf("unexpected fire: reason=%q", reason)
			}
		})
	}
}
