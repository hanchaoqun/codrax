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

func TestBuildAnalysisIR_DiagnosticPredicateReconcilesNoAttachment(t *testing.T) {
	mut := types.NewMutableState("diagnose the observed failure and whether a similar risk still exists")
	mut.SetRequestModel(types.RequestModel{
		RawRequest: "diagnose the observed failure and whether a similar risk still exists",
		Intent:     types.IntentExplain,
		Scenario:   types.ScenarioArchitectureExplain,
		Complexity: types.ComplexityModerate,
		Predicates: types.SemanticPredicates{IsDiagnosticQuestion: true},
		AnalyzerHints: types.AnalyzerHints{
			Keywords: []string{"failure", "risk"},
			Entities: []string{"Analyzer"},
			Kind:     "mechanism",
		},
	})
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mut}

	ir, err := buildAnalysisIR(ctx)
	if err != nil {
		t.Fatalf("buildAnalysisIR: %v", err)
	}
	if ir.RequestModel.Intent != types.IntentRootCause {
		t.Fatalf("Intent = %q, want root_cause", ir.RequestModel.Intent)
	}
	if ir.RequestModel.Scenario != types.ScenarioRootCause {
		t.Fatalf("Scenario = %q, want root_cause", ir.RequestModel.Scenario)
	}
	if got := types.ResolveQuestionFamily(ir.RequestModel); got != types.QFRootCauseTrace {
		t.Fatalf("family = %q, want %q", got, types.QFRootCauseTrace)
	}
}

func TestBuildAnalysisIR_DiagnosticPredicatePerfTraceUsesPerformanceScenario(t *testing.T) {
	mut := types.NewMutableState("diagnose this performance trace")
	mut.SetPerfTrace(&types.PerfBundle{
		IntentHint: "performance",
		Janks:      []types.PerfJank{{Reason: "frame deadline missed"}},
	})
	mut.SetRequestModel(types.RequestModel{
		RawRequest: "diagnose this performance trace",
		Intent:     types.IntentExplain,
		Scenario:   types.ScenarioArchitectureExplain,
		Complexity: types.ComplexityModerate,
		Predicates: types.SemanticPredicates{IsDiagnosticQuestion: true},
		AnalyzerHints: types.AnalyzerHints{
			Keywords: []string{"performance", "trace"},
			Entities: []string{"RenderLoop"},
			Kind:     "mechanism",
		},
	})
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mut}

	ir, err := buildAnalysisIR(ctx)
	if err != nil {
		t.Fatalf("buildAnalysisIR: %v", err)
	}
	if ir.RequestModel.Intent != types.IntentRootCause {
		t.Fatalf("Intent = %q, want root_cause", ir.RequestModel.Intent)
	}
	if ir.RequestModel.Scenario != types.ScenarioPerformanceBottleneck {
		t.Fatalf("Scenario = %q, want performance_bottleneck", ir.RequestModel.Scenario)
	}
}

func TestBuildAnalysisIR_NoAttachmentNonDiagnosticHasNoObservationContract(t *testing.T) {
	mut := types.NewMutableState("explorer 阶段是怎么调用 subagent 的")
	mut.SetRequestModel(types.RequestModel{
		RawRequest: "explorer 阶段是怎么调用 subagent 的",
		Intent:     types.IntentExplain,
		Scenario:   types.ScenarioArchitectureExplain,
		Complexity: types.ComplexityModerate,
		Predicates: types.SemanticPredicates{IsDiagnosticQuestion: false},
		DiagnosticProfile: types.DiagnosticIntentProfile{
			IsDiagnostic:         false,
			CurrentRisk:          false,
			HistoricalRegression: false,
			CurrentVersionCheck:  false,
			Confidence:           0.8,
		},
		AnalyzerHints: types.AnalyzerHints{
			Keywords: []string{"explorer", "subagent"},
			Entities: []string{"explorer", "subagent"},
			Kind:     "mechanism",
		},
	})
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mut}

	ir, err := buildAnalysisIR(ctx)
	if err != nil {
		t.Fatalf("buildAnalysisIR: %v", err)
	}
	if ir.RequestModel.ArtifactObservationProfile != nil {
		t.Fatalf("non-diagnostic no-attachment question should not create observation profile: %+v", ir.RequestModel.ArtifactObservationProfile)
	}
	if ir.AnswerContract.CurrentStatusDiagnostic != nil {
		t.Fatalf("non-diagnostic no-attachment question should not create current-status contract: %+v", ir.AnswerContract.CurrentStatusDiagnostic)
	}
}

func TestBuildAnalysisIR_DiagnosticProfileReconcilesWhenPredicateMissed(t *testing.T) {
	mut := types.NewMutableState("请确认日志里这个历史问题当前版本是否还存在")
	mut.SetRequestModel(types.RequestModel{
		RawRequest: "请确认日志里这个历史问题当前版本是否还存在",
		Intent:     types.IntentExplain,
		Scenario:   types.ScenarioArchitectureExplain,
		Complexity: types.ComplexityModerate,
		Predicates: types.SemanticPredicates{IsDiagnosticQuestion: false},
		DiagnosticProfile: types.DiagnosticIntentProfile{
			CurrentRisk:          true,
			HistoricalRegression: true,
			CurrentVersionCheck:  true,
			Confidence:           0.91,
		},
		AnalyzerHints: types.AnalyzerHints{
			Keywords: []string{"历史问题", "当前版本"},
			Entities: []string{"Analyzer"},
			Kind:     "mechanism",
		},
	})
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mut}

	ir, err := buildAnalysisIR(ctx)
	if err != nil {
		t.Fatalf("buildAnalysisIR: %v", err)
	}
	if ir.RequestModel.Intent != types.IntentRootCause {
		t.Fatalf("Intent = %q, want root_cause", ir.RequestModel.Intent)
	}
	if !ir.RequestModel.Predicates.IsDiagnosticQuestion {
		t.Fatal("diagnostic profile should restore IsDiagnosticQuestion")
	}
	if ir.AnswerContract.CurrentStatusDiagnostic == nil || !ir.AnswerContract.CurrentStatusDiagnostic.Required {
		t.Fatalf("CurrentStatusDiagnostic contract missing: %+v", ir.AnswerContract.CurrentStatusDiagnostic)
	}
	if profile := ir.RequestModel.ArtifactObservationProfile; profile == nil || profile.Source != "user_request" {
		t.Fatalf("no-attachment diagnostic should still expose user_request observation profile: %+v", profile)
	}
}

func TestBuildAnalysisIR_RebuildsArtifactProfileAfterDiagnosticReconcile(t *testing.T) {
	mut := types.NewMutableState("确认上次那个偏题问题当前版本是否还存在")
	mut.SetRequestModel(types.RequestModel{
		RawRequest: "确认上次那个偏题问题当前版本是否还存在",
		Intent:     types.IntentExplain,
		Scenario:   types.ScenarioArchitectureExplain,
		Complexity: types.ComplexityModerate,
		Predicates: types.SemanticPredicates{IsDiagnosticQuestion: true},
		DiagnosticProfile: types.DiagnosticIntentProfile{
			IsDiagnostic:        false,
			CurrentVersionCheck: true,
			ObservationSummary:  "previous final answer drifted off topic",
			Confidence:          0.88,
		},
		AnalyzerHints: types.AnalyzerHints{
			Keywords:     []string{"偏题", "当前版本"},
			Entities:     []string{"Finalizer"},
			ExactTargets: []string{"emit_evidence"},
			Kind:         "mechanism",
		},
	})
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mut}

	ir, err := buildAnalysisIR(ctx)
	if err != nil {
		t.Fatalf("buildAnalysisIR: %v", err)
	}
	profile := ir.RequestModel.ArtifactObservationProfile
	if profile == nil {
		t.Fatal("ArtifactObservationProfile = nil")
	}
	if !containsAnalyzerIntentString(profile.ObservationKinds, "diagnostic_question") ||
		!containsAnalyzerIntentString(profile.ObservationKinds, "current_version_check") {
		t.Fatalf("profile was not rebuilt from reconciled diagnostic fields: %+v", profile.ObservationKinds)
	}
	if profile.SymptomSummary != "previous final answer drifted off topic" {
		t.Fatalf("SymptomSummary = %q", profile.SymptomSummary)
	}
	if !containsAnalyzerIntentString(profile.SubjectCandidates, "Finalizer") ||
		!containsAnalyzerIntentString(profile.SubjectCandidates, "emit_evidence") {
		t.Fatalf("profile missing reconciled subject candidates: %+v", profile.SubjectCandidates)
	}
}

func TestBuildAnalysisIR_AttachesArtifactObservationProfileFromLog(t *testing.T) {
	mut := types.NewMutableState("分析日志里的重试和行号问题")
	mut.SetLogTriage(&types.LogBundle{
		Meta: types.LogMeta{Summary: "finalizer repeatedly rewrote an off-topic answer"},
		Observations: []types.LogObservation{
			{
				Kind:       types.LogObservationRetryCycle,
				Subject:    "final answer",
				Summary:    "the answer was rewritten repeatedly",
				Diagnostic: true,
				Confidence: 0.9,
			},
			{
				Kind:       types.LogObservationLineMapping,
				Summary:    "line offsets did not match the cited source",
				Diagnostic: true,
				Confidence: 0.88,
			},
		},
	})
	mut.SetRequestModel(types.RequestModel{
		RawRequest: "分析日志里的重试和行号问题",
		Intent:     types.IntentExplain,
		Scenario:   types.ScenarioArchitectureExplain,
		Complexity: types.ComplexityModerate,
		Predicates: types.SemanticPredicates{},
		AnalyzerHints: types.AnalyzerHints{
			Keywords: []string{"重试", "行号"},
			Entities: []string{"Finalizer"},
			Kind:     "mechanism",
		},
	})
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mut}

	ir, err := buildAnalysisIR(ctx)
	if err != nil {
		t.Fatalf("buildAnalysisIR: %v", err)
	}
	profile := ir.RequestModel.ArtifactObservationProfile
	if profile == nil {
		t.Fatal("ArtifactObservationProfile = nil")
	}
	if !profile.HasRetryLoop || !profile.HasLineMismatch {
		t.Fatalf("artifact profile did not preserve typed observations: %+v", profile)
	}
	if profile.DiagnosticConfidence < 0.9 {
		t.Fatalf("DiagnosticConfidence = %.2f, want >= 0.9", profile.DiagnosticConfidence)
	}
}

func containsAnalyzerIntentString(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
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
// The bundle parameter is retained so the no-longer-strict log hint
// path remains covered: a nil bundle is the no-log case; a bundle with
// IntentHint=RootCause is advisory only and must not steal user intent.
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
		// Commit 61 Batch F.3 (red line "no system hard-cap"):
		// log_triage's IntentHint NO LONGER overrides LLM's chosen
		// Intent. The user's explicit intent (e.g. "explain") stays
		// even when the attached log carries a panic. Trust the LLM
		// — emit_analysis already saw the raw log via formatAttachedLog
		// and would have classified root_cause itself if user asked.
		{"log_triage bundle with root_cause hint NO LONGER forces root_cause (commit 61)",
			types.IntentExplain, types.SemanticPredicates{}, rootCauseBundle,
			types.IntentExplain, false},
		{"log_triage bundle still no-op when already root_cause",
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
				Kind: "history",
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
			},
			Predicates: types.SemanticPredicates{IsScalarAnswer: true},
		}
		if isHistoryLookupRequest(rm) {
			t.Fatal("ordinary return value should not trigger history lookup")
		}
	})
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

	t.Run("single-topic cross-component trace still downgrades to generic", func(t *testing.T) {
		rm := types.RequestModel{
			Scenario:      types.ScenarioArchitectureExplain,
			Intent:        types.IntentTrace,
			PredicateAxis: types.AxisCall,
			AnalyzerHints: types.AnalyzerHints{Kind: "call_chain"},
			Predicates:    types.SemanticPredicates{IsCrossComponent: true},
		}
		got, reason := reconcileScenario(rm)
		if got != types.ScenarioGeneric {
			t.Fatalf("scenario = %q, want %q", got, types.ScenarioGeneric)
		}
		if reason == "" {
			t.Fatal("reason = empty, want non-empty reconcile reason")
		}
	})

	t.Run("multi-topic cross-component trace keeps architecture scenario", func(t *testing.T) {
		rm := types.RequestModel{
			Scenario:      types.ScenarioArchitectureExplain,
			Intent:        types.IntentTrace,
			PredicateAxis: types.AxisCall,
			AnalyzerHints: types.AnalyzerHints{Kind: "call_chain"},
			SubTopics: []types.SubTopic{
				{Summary: "path A"},
				{Summary: "path B"},
			},
			Predicates: types.SemanticPredicates{IsCrossComponent: true},
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
		bundle    *types.LogBundle
		wantNil   bool
		wantKind  types.DiagramKind
		wantScope types.DiagramScope
	}{
		{
			name:    "step_list without structural cues stays nil",
			rm:      types.RequestModel{},
			wantNil: true,
		},
		{
			name: "trace intent prefers call dag",
			rm: types.RequestModel{
				Intent: types.IntentTrace,
			},
			wantKind:  types.DiagramCallDAG,
			wantScope: types.DiagramScopeOverall,
		},
		{
			name: "architecture scenario prefers architecture diagram",
			rm: types.RequestModel{
				Scenario: types.ScenarioArchitectureExplain,
			},
			wantKind:  types.DiagramArchitecture,
			wantScope: types.DiagramScopeOverall,
		},
		{
			name: "scalar call question still requires diagram",
			rm: types.RequestModel{
				PredicateAxis: types.AxisCall,
			},
			wantKind:  types.DiagramCallDAG,
			wantScope: types.DiagramScopeOverall,
		},
		{
			name: "generic configure question does not auto-require diagram",
			rm: types.RequestModel{
				PredicateAxis: types.AxisConfigure,
				Intent:        types.IntentConfigQuery,
			},
			wantNil: true,
		},
		{
			name: "config trace still requires diagram",
			rm: types.RequestModel{
				PredicateAxis: types.AxisConfigure,
				Scenario:      types.ScenarioConfigTrace,
				Intent:        types.IntentConfigQuery,
			},
			wantKind:  types.DiagramArchitecture,
			wantScope: types.DiagramScopeOverall,
		},
		{
			name:      "log call chain requires diagram",
			rm:        types.RequestModel{},
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
			wantKind:  types.DiagramCallDAG,
			wantScope: types.DiagramScopePerSubTopic,
		},
		{
			name: "plain scalar without structural cues stays nil",
			rm: types.RequestModel{
				Intent: types.IntentReturnValue,
			},
			wantNil: true,
		},
		{
			name: "scalar source-literal lookup suppresses architecture-only diagram",
			rm: types.RequestModel{
				Scenario:      types.ScenarioArchitectureExplain,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectFunctionName},
				Predicates:    types.SemanticPredicates{IsScalarAnswer: true},
			},
			wantNil: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := reconcileDiagramContract(tc.rm, tc.bundle)
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
// Fallback pair (post-PR2 simplification — the "shape=value" leg
// was retired with AnswerShape): intent=return_value AND
// answer_subject.kind=numeric AND is_scalar_answer=true. All three
// must co-occur.
//
// Over-trigger is intentional: a citable-numeric question ("what is
// MAX_STEPS") that trips the triple loses only citation enforcement,
// not correctness; the alternative (under-trigger) exhausts retries
// on misclassified measurement-scalar questions.
func TestIsMeasurementScalarRequest_Coverage(t *testing.T) {
	triple := func(base types.RequestModel) types.RequestModel {
		base.Intent = types.IntentReturnValue
		base.Predicates.IsScalarAnswer = true
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
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isMeasurementScalarRequest(c.rm); got != c.want {
				t.Errorf("isMeasurementScalarRequest = %v, want %v", got, c.want)
			}
		})
	}
}
