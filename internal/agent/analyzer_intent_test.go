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
//  1. AnswerContract.CitationReq.Required == false
//  2. No CritCitationCountGE entries in AnswerContract.AcceptanceTests
//  3. No CritCitationCountGE entries on any TaskNode.SuccessCriteria
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

func TestBuildAnalysisIR_HistoryLookupStripsAllThreeGates(t *testing.T) {
	mut := types.NewMutableState("EvidenceClosure 结构体是哪次 commit 第一次引入本项目的")
	mut.SetRequestModel(types.RequestModel{
		RawRequest: "EvidenceClosure 结构体是哪次 commit 第一次引入本项目的",
		Intent:     types.IntentReturnValue,
		Complexity: types.ComplexitySimple,
		AnalyzerHints: types.AnalyzerHints{
			Keywords: []string{"EvidenceClosure", "commit", "history", "introduced"},
			Entities: []string{"EvidenceClosure"},
			Kind:     "history",
			Shape:    string(types.ShapeValue),
		},
		AnswerSubject: types.AnswerSubject{Kind: types.SubjectStringLiteral},
		Predicates: types.SemanticPredicates{
			IsScalarAnswer: true,
		},
	})
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mut}

	ir, err := buildAnalysisIR(ctx)
	if err != nil {
		t.Fatalf("buildAnalysisIR: %v", err)
	}

	if ir.AnswerContract.CitationReq.Required {
		t.Error("AnswerContract.CitationReq.Required should be false for history lookup")
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

func TestBuildAnalysisIR_DiagramContractPropagates(t *testing.T) {
	mut := types.NewMutableState("trace how request dispatch reaches the handler")
	mut.SetRequestModel(types.RequestModel{
		RawRequest:    "trace how request dispatch reaches the handler",
		Intent:        types.IntentTrace,
		Scenario:      types.ScenarioArchitectureExplain,
		Complexity:    types.ComplexityModerate,
		PredicateAxis: types.AxisCall,
		AnalyzerHints: types.AnalyzerHints{
			Keywords: []string{"dispatch", "handler"},
			Entities: []string{"Dispatch", "Handler"},
			Kind:     "call_chain",
			Shape:    string(types.ShapeStepList),
		},
		DiagramHint: &types.DiagramHint{Kind: types.DiagramCallDAG},
	})
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mut}

	ir, err := buildAnalysisIR(ctx)
	if err != nil {
		t.Fatalf("buildAnalysisIR: %v", err)
	}
	if ir.AnswerContract.Diagram == nil {
		t.Fatal("AnswerContract.Diagram = nil, want contract")
	}
	if !ir.AnswerContract.Diagram.Required {
		t.Fatalf("Diagram.Required = false, want true")
	}
	if ir.AnswerContract.Diagram.Minimum != 1 {
		t.Fatalf("Diagram.Minimum = %d, want 1", ir.AnswerContract.Diagram.Minimum)
	}
	if len(ir.AnswerContract.Diagram.PreferredKinds) == 0 || ir.AnswerContract.Diagram.PreferredKinds[0] != types.DiagramCallDAG {
		t.Fatalf("Diagram.PreferredKinds = %v, want first call_dag", ir.AnswerContract.Diagram.PreferredKinds)
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
func TestBuildAnalysisIR_ExactResolutionContractPropagates(t *testing.T) {
	mut := types.NewMutableState("where is explore_mid_loop_hint_budget defined")
	mut.SetRequestModel(types.RequestModel{
		RawRequest: "where is explore_mid_loop_hint_budget defined",
		Intent:     types.IntentConfigQuery,
		Scenario:   types.ScenarioConfigTrace,
		Complexity: types.ComplexitySimple,
		AnalyzerHints: types.AnalyzerHints{
			Kind:            "config_mapping",
			PrimaryEntities: []string{"explore_mid_loop_hint_budget"},
			Entities:        []string{"explore_mid_loop_hint_budget"},
			Shape:           string(types.ShapeConfigValue),
		},
		AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
	})
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mut}

	ir, err := buildAnalysisIR(ctx)
	if err != nil {
		t.Fatalf("buildAnalysisIR: %v", err)
	}
	if ir.AnswerContract.ExactResolution == nil {
		t.Fatal("AnswerContract.ExactResolution = nil, want contract")
	}
	if got := ir.AnswerContract.ExactResolution.TargetLabel; got != "config key" {
		t.Fatalf("TargetLabel = %q, want config key", got)
	}
	if got := ir.AnswerContract.ExactResolution.Targets; len(got) != 1 || got[0] != "explore_mid_loop_hint_budget" {
		t.Fatalf("Targets = %v, want [explore_mid_loop_hint_budget]", got)
	}
	if got := ir.AnswerContract.ExactResolution.RelatedContextPolicy; got != types.ExactContextSameFamilyGrounded {
		t.Fatalf("RelatedContextPolicy = %q, want %q", got, types.ExactContextSameFamilyGrounded)
	}
}

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

func TestIsHistoryLookupRequest(t *testing.T) {
	t.Run("declared history kind", func(t *testing.T) {
		rm := types.RequestModel{
			Intent: types.IntentReturnValue,
			AnalyzerHints: types.AnalyzerHints{
				Kind:  "history",
				Shape: string(types.ShapeValue),
			},
			Predicates: types.SemanticPredicates{IsScalarAnswer: true},
		}
		if !isHistoryLookupRequest(rm) {
			t.Fatal("declared history kind should trigger")
		}
	})

	t.Run("predicate-driven history lookup", func(t *testing.T) {
		rm := types.RequestModel{
			RawRequest: "Who introduced EvidenceClosure in git history?",
			Intent:     types.IntentReturnValue,
			AnalyzerHints: types.AnalyzerHints{
				Keywords: []string{"EvidenceClosure"},
				Shape:    string(types.ShapeValue),
			},
			Predicates: types.SemanticPredicates{
				IsScalarAnswer:  true,
				IsHistoryLookup: true,
			},
		}
		if !isHistoryLookupRequest(rm) {
			t.Fatal("history predicate should trigger on scalar history lookup")
		}
	})

	t.Run("ordinary return value stays citable", func(t *testing.T) {
		rm := types.RequestModel{
			RawRequest: "what does Execute return",
			Intent:     types.IntentReturnValue,
			AnalyzerHints: types.AnalyzerHints{
				Keywords: []string{"Execute", "return value"},
				Shape:    string(types.ShapeValue),
			},
			Predicates: types.SemanticPredicates{IsScalarAnswer: true},
		}
		if isHistoryLookupRequest(rm) {
			t.Fatal("ordinary return value should not trigger history lookup")
		}
	})
}

// TestReconcileShape pins the rules in the schema-v4 reconcileShape:
// Rule 1a (count/category + relational lookup → list_of_symbols),
// Rule 1b (category enumeration alone → list_of_symbols), and Rule 2
// (config_value → value when subject is a source-code literal).
// Inputs are typed predicates + AnswerSubject — no raw prose.
func TestReconcileShape(t *testing.T) {
	cases := []struct {
		name     string
		rm       types.RequestModel
		declared types.AnswerShape
		subject  types.AnswerSubject
		preds    types.SemanticPredicates
		want     types.AnswerShape
		wantFire bool
	}{
		// Rule 1a: count + relational lookup lifts value → list_of_symbols.
		{"value + count + relational → list_of_symbols",
			types.RequestModel{}, types.ShapeValue, types.AnswerSubject{},
			types.SemanticPredicates{IsCountQuestion: true, IsRelationalLookup: true},
			types.ShapeListOfSymbols, true},
		{"config_value + count + relational → list_of_symbols",
			types.RequestModel{}, types.ShapeConfigValue, types.AnswerSubject{Kind: types.SubjectFunctionName},
			types.SemanticPredicates{IsCountQuestion: true, IsRelationalLookup: true},
			types.ShapeListOfSymbols, true},
		// Rule 1a: category + relational lookup also lifts.
		{"value + category + relational → list_of_symbols",
			types.RequestModel{}, types.ShapeValue, types.AnswerSubject{},
			types.SemanticPredicates{IsCategoryEnumeration: true, IsRelationalLookup: true},
			types.ShapeListOfSymbols, true},
		// Rule 1a negatives: count without relational stays value.
		{"value + count alone stays value",
			types.RequestModel{}, types.ShapeValue, types.AnswerSubject{},
			types.SemanticPredicates{IsCountQuestion: true},
			types.ShapeValue, false},
		{"value + relational alone stays value",
			types.RequestModel{}, types.ShapeValue, types.AnswerSubject{},
			types.SemanticPredicates{IsRelationalLookup: true},
			types.ShapeValue, false},
		// Rule 1b: category enumeration alone (no relational) still lifts.
		{"value + category alone lifts (Rule 1b)",
			types.RequestModel{}, types.ShapeValue, types.AnswerSubject{Kind: types.SubjectEnumValue},
			types.SemanticPredicates{IsCategoryEnumeration: true},
			types.ShapeListOfSymbols, true},
		{"config_value + category alone lifts",
			types.RequestModel{}, types.ShapeConfigValue, types.AnswerSubject{},
			types.SemanticPredicates{IsCategoryEnumeration: true},
			types.ShapeListOfSymbols, true},
		// Non-value/config_value shapes are untouched by Rules 1a/1b.
		{"explanation untouched by Rule 1",
			types.RequestModel{}, types.ShapeExplanation, types.AnswerSubject{},
			types.SemanticPredicates{IsCountQuestion: true, IsRelationalLookup: true},
			types.ShapeExplanation, false},
		{"list_of_symbols untouched by Rule 1",
			types.RequestModel{}, types.ShapeListOfSymbols, types.AnswerSubject{},
			types.SemanticPredicates{IsCategoryEnumeration: true},
			types.ShapeListOfSymbols, false},
		// Rule 2: config_value → value for source-code literal subjects.
		{"config_value + function subject → value",
			types.RequestModel{}, types.ShapeConfigValue, types.AnswerSubject{Kind: types.SubjectFunctionName},
			types.SemanticPredicates{}, types.ShapeValue, true},
		{"config_value + type subject → value",
			types.RequestModel{}, types.ShapeConfigValue, types.AnswerSubject{Kind: types.SubjectTypeName},
			types.SemanticPredicates{}, types.ShapeValue, true},
		{"config_value + return subject → value",
			types.RequestModel{}, types.ShapeConfigValue, types.AnswerSubject{Kind: types.SubjectReturnValue},
			types.SemanticPredicates{}, types.ShapeValue, true},
		// Rule 2 negative: config_value + config_key subject stays.
		{"config_value + config_key subject stays",
			types.RequestModel{}, types.ShapeConfigValue, types.AnswerSubject{Kind: types.SubjectConfigKey},
			types.SemanticPredicates{}, types.ShapeConfigValue, false},
		{"single exact config trace list lifts to explanation",
			types.RequestModel{
				RawRequest: "explore_mid_loop_hint_budget 的最终有效值是怎么计算出来的？",
				Intent:     types.IntentConfigQuery,
				Scenario:   types.ScenarioConfigTrace,
				AnalyzerHints: types.AnalyzerHints{
					PrimaryEntities: []string{"explore_mid_loop_hint_budget"},
					ExactTargets:    []string{"explore_mid_loop_hint_budget"},
				},
			},
			types.ShapeListOfSymbols, types.AnswerSubject{Kind: types.SubjectConfigKey},
			types.SemanticPredicates{IsScalarAnswer: false}, types.ShapeExplanation, true},
		{"single exact scalar config trace list lifts to config_value",
			types.RequestModel{
				RawRequest: "http_timeout_ms 的最终有效值是多少？",
				Intent:     types.IntentConfigQuery,
				Scenario:   types.ScenarioConfigTrace,
				AnalyzerHints: types.AnalyzerHints{
					PrimaryEntities: []string{"http_timeout_ms"},
					ExactTargets:    []string{"http_timeout_ms"},
				},
			},
			types.ShapeListOfSymbols, types.AnswerSubject{Kind: types.SubjectConfigKey},
			types.SemanticPredicates{IsScalarAnswer: true}, types.ShapeConfigValue, true},
		{"scalar source-literal lookup explanation collapses to value",
			types.RequestModel{
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectFunctionName},
			},
			types.ShapeExplanation, types.AnswerSubject{Kind: types.SubjectFunctionName},
			types.SemanticPredicates{IsScalarAnswer: true}, types.ShapeValue, true},
		{"multi-axis structural explanation lifts step_list to explanation",
			types.RequestModel{
				Intent:        types.IntentExplain,
				Scenario:      types.ScenarioArchitectureExplain,
				PredicateAxis: types.AxisDefine,
			},
			types.ShapeStepList,
			types.AnswerSubject{
				Kind:       types.SubjectStructField,
				EntityAxes: []string{"Criterion → fields", "Hypothesis → fields", "Criterion ↔ Hypothesis → relationship"},
			},
			types.SemanticPredicates{}, types.ShapeExplanation, true},
		{"single-axis define step_list stays step_list",
			types.RequestModel{
				Intent:        types.IntentExplain,
				Scenario:      types.ScenarioArchitectureExplain,
				PredicateAxis: types.AxisDefine,
			},
			types.ShapeStepList,
			types.AnswerSubject{
				Kind:       types.SubjectStructField,
				EntityAxes: []string{"Criterion → fields"},
			},
			types.SemanticPredicates{}, types.ShapeStepList, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.rm.AnswerSubject = c.subject
			c.rm.Predicates = c.preds
			got, reason := reconcileShape(c.rm, c.declared, c.subject, c.preds)
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

func TestReconcileScenario(t *testing.T) {
	t.Run("scalar source literal lookup downgrades to generic", func(t *testing.T) {
		rm := types.RequestModel{
			Scenario:      types.ScenarioArchitectureExplain,
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectFunctionName},
			Predicates:    types.SemanticPredicates{IsScalarAnswer: true},
		}
		got, reason := reconcileScenario(rm)
		if got != types.ScenarioGeneric {
			t.Fatalf("scenario = %q, want %q", got, types.ScenarioGeneric)
		}
		if reason == "" {
			t.Fatal("reason = empty, want non-empty reconcile reason")
		}
	})

	t.Run("single-topic structural trace downgrades to generic", func(t *testing.T) {
		rm := types.RequestModel{
			Scenario:      types.ScenarioArchitectureExplain,
			Intent:        types.IntentTrace,
			PredicateAxis: types.AxisCall,
			AnalyzerHints: types.AnalyzerHints{Kind: "call_chain"},
		}
		got, reason := reconcileScenario(rm)
		if got != types.ScenarioGeneric {
			t.Fatalf("scenario = %q, want %q", got, types.ScenarioGeneric)
		}
		if reason == "" {
			t.Fatal("reason = empty, want non-empty reconcile reason")
		}
	})

	t.Run("cross-component trace keeps architecture scenario", func(t *testing.T) {
		rm := types.RequestModel{
			Scenario:      types.ScenarioArchitectureExplain,
			Intent:        types.IntentTrace,
			PredicateAxis: types.AxisCall,
			AnalyzerHints: types.AnalyzerHints{Kind: "call_chain"},
			Predicates:    types.SemanticPredicates{IsCrossComponent: true},
		}
		got, reason := reconcileScenario(rm)
		if got != types.ScenarioArchitectureExplain {
			t.Fatalf("scenario = %q, want %q", got, types.ScenarioArchitectureExplain)
		}
		if reason != "" {
			t.Fatalf("reason = %q, want empty", reason)
		}
	})
}

func TestReconcileDiagramContract(t *testing.T) {
	logBundle := &types.LogBundle{
		Errors: []types.LogError{{
			Frames: []types.LogFrame{
				{File: "internal/a.go", Line: 10, Func: "inner"},
				{File: "internal/b.go", Line: 20, Func: "outer"},
			},
		}},
	}

	cases := []struct {
		name      string
		rm        types.RequestModel
		shape     types.AnswerShape
		bundle    *types.LogBundle
		wantNil   bool
		wantKind  types.DiagramKind
		wantScope types.DiagramScope
	}{
		{
			name:    "step_list without structural cues stays nil",
			rm:      types.RequestModel{},
			shape:   types.ShapeStepList,
			wantNil: true,
		},
		{
			name: "trace intent prefers call dag",
			rm: types.RequestModel{
				Intent: types.IntentTrace,
			},
			shape:     types.ShapeExplanation,
			wantKind:  types.DiagramCallDAG,
			wantScope: types.DiagramScopeOverall,
		},
		{
			name: "architecture scenario prefers architecture diagram",
			rm: types.RequestModel{
				Scenario: types.ScenarioArchitectureExplain,
			},
			shape:     types.ShapeExplanation,
			wantKind:  types.DiagramArchitecture,
			wantScope: types.DiagramScopeOverall,
		},
		{
			name: "scalar call question still requires diagram",
			rm: types.RequestModel{
				PredicateAxis: types.AxisCall,
			},
			shape:     types.ShapeValue,
			wantKind:  types.DiagramCallDAG,
			wantScope: types.DiagramScopeOverall,
		},
		{
			name: "generic configure question does not auto-require diagram",
			rm: types.RequestModel{
				PredicateAxis: types.AxisConfigure,
				Intent:        types.IntentConfigQuery,
			},
			shape:   types.ShapeConfigValue,
			wantNil: true,
		},
		{
			name: "config trace still requires diagram",
			rm: types.RequestModel{
				PredicateAxis: types.AxisConfigure,
				Scenario:      types.ScenarioConfigTrace,
				Intent:        types.IntentConfigQuery,
			},
			shape:     types.ShapeExplanation,
			wantKind:  types.DiagramArchitecture,
			wantScope: types.DiagramScopeOverall,
		},
		{
			name:      "log call chain requires diagram",
			rm:        types.RequestModel{},
			shape:     types.ShapeBoolean,
			bundle:    logBundle,
			wantKind:  types.DiagramCallDAG,
			wantScope: types.DiagramScopeOverall,
		},
		{
			name: "multi-topic scopes diagram per subtopic",
			rm: types.RequestModel{
				Intent: types.IntentTrace,
				SubTopics: []types.SubTopic{
					{Summary: "A"},
					{Summary: "B"},
				},
			},
			shape:     types.ShapeExplanation,
			wantKind:  types.DiagramCallDAG,
			wantScope: types.DiagramScopePerSubTopic,
		},
		{
			name: "plain scalar without structural cues stays nil",
			rm: types.RequestModel{
				Intent: types.IntentReturnValue,
			},
			shape:   types.ShapeValue,
			wantNil: true,
		},
		{
			name: "scalar source-literal lookup suppresses architecture-only diagram",
			rm: types.RequestModel{
				Scenario:      types.ScenarioArchitectureExplain,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectFunctionName},
				Predicates:    types.SemanticPredicates{IsScalarAnswer: true},
			},
			shape:   types.ShapeValue,
			wantNil: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := reconcileDiagramContract(tc.rm, tc.shape, tc.bundle)
			if tc.wantNil {
				if got != nil {
					t.Fatalf("expected nil contract, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil diagram contract")
			}
			if !got.Required {
				t.Fatalf("Required = false, want true (%+v)", got)
			}
			if got.Minimum != 1 {
				t.Fatalf("Minimum = %d, want 1", got.Minimum)
			}
			if len(got.PreferredKinds) == 0 || got.PreferredKinds[0] != tc.wantKind {
				t.Fatalf("PreferredKinds = %v, want first %q", got.PreferredKinds, tc.wantKind)
			}
			if got.ScopeHint != tc.wantScope {
				t.Fatalf("ScopeHint = %q, want %q", got.ScopeHint, tc.wantScope)
			}
		})
	}
}

// TestIsMeasurementScalarRequest_Coverage pins the primary predicate
// signal AND the structural-coherence fallback added 2026-04-22 to
// catch LLM inter-run inconsistency on is_count_question.
//
// Fallback triple: answer_shape=value AND intent=return_value AND
// answer_subject.kind=numeric. All three must co-occur.
//
// Over-trigger is intentional: a citable-numeric question ("what is
// MAX_STEPS") that trips the triple loses only citation enforcement,
// not correctness; the alternative (under-trigger) exhausts retries
// on misclassified measurement-scalar questions.
func TestIsMeasurementScalarRequest_Coverage(t *testing.T) {
	triple := func(base types.RequestModel) types.RequestModel {
		base.Intent = types.IntentReturnValue
		base.AnalyzerHints.Shape = string(types.ShapeValue)
		base.AnswerSubject = types.AnswerSubject{Kind: types.SubjectNumeric}
		return base
	}

	cases := []struct {
		name string
		rm   types.RequestModel
		want bool
	}{
		{
			name: "primary: IsCountQuestion=true alone fires",
			rm: types.RequestModel{
				Predicates: types.SemanticPredicates{IsCountQuestion: true},
			},
			want: true,
		},
		{
			name: "primary beats all: IsCountQuestion trumps mismatched signals",
			rm: types.RequestModel{
				Intent:        types.IntentExplain,
				AnalyzerHints: types.AnalyzerHints{Shape: string(types.ShapeStepList)},
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectGeneric},
				Predicates:    types.SemanticPredicates{IsCountQuestion: true},
			},
			want: true,
		},
		{
			name: "fallback triple fires when IsCountQuestion=false",
			rm: triple(types.RequestModel{
				Predicates: types.SemanticPredicates{IsCountQuestion: false},
			}),
			want: true,
		},
		{
			name: "fallback requires shape=value — step_list declines",
			rm: func() types.RequestModel {
				rm := triple(types.RequestModel{})
				rm.AnalyzerHints.Shape = string(types.ShapeStepList)
				return rm
			}(),
			want: false,
		},
		{
			name: "fallback requires intent=return_value — explain declines",
			rm: func() types.RequestModel {
				rm := triple(types.RequestModel{})
				rm.Intent = types.IntentExplain
				return rm
			}(),
			want: false,
		},
		{
			name: "fallback requires numeric subject — function_name declines",
			rm: func() types.RequestModel {
				rm := triple(types.RequestModel{})
				rm.AnswerSubject = types.AnswerSubject{Kind: types.SubjectFunctionName}
				return rm
			}(),
			want: false,
		},
		{
			name: "fallback: fully zero RequestModel declines",
			rm:   types.RequestModel{},
			want: false,
		},
		{
			name: "fallback: legacy shape casing + whitespace still normalizes",
			rm: func() types.RequestModel {
				rm := triple(types.RequestModel{})
				rm.AnalyzerHints.Shape = "  VALUE  "
				return rm
			}(),
			want: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isMeasurementScalarRequest(c.rm); got != c.want {
				t.Errorf("isMeasurementScalarRequest = %v, want %v", got, c.want)
			}
		})
	}
}
