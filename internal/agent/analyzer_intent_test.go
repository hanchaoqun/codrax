package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap"
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

func TestReconcileHistoryMultiTargetScalar_DemotesPerTargetHistorySet(t *testing.T) {
	rm := types.RequestModel{
		Intent: types.IntentReturnValue,
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"scalar", "definitely_absent_marker"},
		},
		AnswerSubject: types.AnswerSubject{Kind: types.SubjectStringLiteral},
		Predicates: types.SemanticPredicates{
			IsHistoryLookup: true,
			IsScalarAnswer:  true,
		},
	}
	got, reason := reconcileHistoryMultiTargetScalar(rm)
	if reason == "" {
		t.Fatal("expected multi-target history scalar to reconcile")
	}
	if got.Predicates.IsScalarAnswer {
		t.Fatalf("multi-target history query must not stay scalar: %+v", got.Predicates)
	}
	if got.Intent != types.IntentEnumerate {
		t.Fatalf("intent = %q, want enumerate", got.Intent)
	}
	if got.AnswerSubject.Kind != "" {
		t.Fatalf("scalar answer subject should be cleared, got %+v", got.AnswerSubject)
	}
}

func TestReconcileHistoryMultiTargetScalar_PreservesTrueScalarCount(t *testing.T) {
	rm := types.RequestModel{
		Intent: types.IntentReturnValue,
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"scalar", "other"},
		},
		Predicates: types.SemanticPredicates{
			IsHistoryLookup: true,
			IsScalarAnswer:  true,
			IsCountQuestion: true,
		},
	}
	got, reason := reconcileHistoryMultiTargetScalar(rm)
	if reason != "" {
		t.Fatalf("count-valued history query should stay scalar, got reason %q", reason)
	}
	if !got.Predicates.IsScalarAnswer || got.Intent != types.IntentReturnValue {
		t.Fatalf("true scalar count drifted: %+v", got)
	}
}

func TestReconcileHistoryMultiTargetScalar_PreservesSingleTargetLookup(t *testing.T) {
	rm := types.RequestModel{
		Intent: types.IntentReturnValue,
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"scalar"},
		},
		Predicates: types.SemanticPredicates{
			IsHistoryLookup: true,
			IsScalarAnswer:  true,
		},
	}
	got, reason := reconcileHistoryMultiTargetScalar(rm)
	if reason != "" {
		t.Fatalf("single-target scalar lookup should not reconcile, got reason %q", reason)
	}
	if !got.Predicates.IsScalarAnswer || got.Intent != types.IntentReturnValue {
		t.Fatalf("single scalar history lookup drifted: %+v", got)
	}
}

func TestReconcileChangeImpactProfile_ForcesAffectedFileSetRouting(t *testing.T) {
	rm := types.RequestModel{
		Intent: types.IntentTrace,
		AnalyzerHints: types.AnalyzerHints{
			Kind:         string(types.ReqCallChain),
			Entities:     []string{"CitationReq.Required"},
			ExactTargets: []string{"CitationReq.Required"},
		},
		Predicates: types.SemanticPredicates{
			IsScalarAnswer:        true,
			IsRoleLocateLookup:    true,
			IsCrossComponent:      true,
			IsCategoryEnumeration: false,
			IsRelationalLookup:    false,
		},
		ChangeImpactProfile: &types.ChangeImpactProfile{
			IsChangeImpact:  true,
			Target:          "CitationReq.Required",
			TargetKind:      types.SubjectStructField,
			RequestedOutput: types.ImpactOutputFiles,
			Confidence:      0.9,
		},
	}
	got, reason := reconcileChangeImpactProfile(rm)
	if reason == "" {
		t.Fatal("expected active change-impact profile to reconcile classification")
	}
	if got.Intent != types.IntentEnumerate {
		t.Fatalf("intent = %q, want enumerate", got.Intent)
	}
	if got.AnalyzerHints.Kind != string(types.ReqEnumeration) {
		t.Fatalf("question kind = %q, want enumeration", got.AnalyzerHints.Kind)
	}
	if !got.Predicates.IsCategoryEnumeration || !got.Predicates.IsRelationalLookup {
		t.Fatalf("set-valued impact output should be category+relational, got %+v", got.Predicates)
	}
	if got.Predicates.IsCrossComponent {
		t.Fatalf("set-valued impact output should not require multi-topic cross-component decomposition, got %+v", got.Predicates)
	}
	if got.Predicates.IsScalarAnswer || got.Predicates.IsCountQuestion || got.Predicates.IsRoleLocateLookup {
		t.Fatalf("set-valued impact output should clear scalar predicates, got %+v", got.Predicates)
	}
	if len(got.AnalyzerHints.ExactTargets) != 1 {
		t.Fatalf("reconcile should preserve search-side exact target hints; exact final-answer contract is disabled in types/exact_lookup, got %+v", got.AnalyzerHints.ExactTargets)
	}
}

func TestReconcileChangeImpactProfile_LowConfidenceDoesNotForceRouting(t *testing.T) {
	rm := types.RequestModel{
		Intent: types.IntentTrace,
		AnalyzerHints: types.AnalyzerHints{
			Kind:         string(types.ReqCallChain),
			Entities:     []string{"CitationReq.Required"},
			ExactTargets: []string{"CitationReq.Required"},
		},
		Predicates: types.SemanticPredicates{
			IsScalarAnswer:     true,
			IsRoleLocateLookup: true,
			IsCrossComponent:   true,
		},
		ChangeImpactProfile: &types.ChangeImpactProfile{
			IsChangeImpact:  true,
			Target:          "CitationReq.Required",
			TargetKind:      types.SubjectStructField,
			RequestedOutput: types.ImpactOutputFiles,
			Confidence:      0.6,
		},
	}
	got, reason := reconcileChangeImpactProfile(rm)
	if reason != "" {
		t.Fatalf("low-confidence change-impact profile should stay advisory, got reason %q", reason)
	}
	if got.Intent != rm.Intent || got.AnalyzerHints.Kind != rm.AnalyzerHints.Kind ||
		got.Predicates != rm.Predicates {
		t.Fatalf("low-confidence profile forced routing: %+v", got)
	}
}

func TestReconcileChangeImpactProfile_LeavesStepOutputAlone(t *testing.T) {
	rm := types.RequestModel{
		Intent:        types.IntentExplain,
		AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
		ChangeImpactProfile: &types.ChangeImpactProfile{
			IsChangeImpact:  true,
			Target:          "CitationReq.Required",
			RequestedOutput: types.ImpactOutputSteps,
			Confidence:      0.9,
		},
	}
	got, reason := reconcileChangeImpactProfile(rm)
	if reason != "" {
		t.Fatalf("step-output impact questions should keep mechanism/trace routing, got reason %q", reason)
	}
	if got.Intent != rm.Intent || got.AnalyzerHints.Kind != rm.AnalyzerHints.Kind {
		t.Fatalf("step-output reconcile changed classification: %+v", got)
	}
}

func TestReconcileDiagnosticQuestionProfile_IgnoresProfileOnlyMirrorDrift(t *testing.T) {
	rm := types.RequestModel{
		Intent:   types.IntentExplain,
		Scenario: types.ScenarioArchitectureExplain,
		Predicates: types.SemanticPredicates{
			IsDiagnosticQuestion: false,
		},
		DiagnosticProfile: types.DiagnosticIntentProfile{
			IsDiagnostic:        true,
			CurrentVersionCheck: true,
			Confidence:          0.9,
		},
	}

	got, reason := reconcileDiagnosticQuestionProfile(rm)
	if reason != "" {
		t.Fatalf("profile-only diagnostic mirror drift should not reconcile downstream, got %q", reason)
	}
	if got.Intent != rm.Intent || got.Scenario != rm.Scenario || got.Predicates != rm.Predicates {
		t.Fatalf("profile-only drift changed routing: %+v", got)
	}
}

func TestReconcileDiagnosticQuestionProfile_PredicateStillRoutesDiagnostic(t *testing.T) {
	rm := types.RequestModel{
		Intent:   types.IntentExplain,
		Scenario: types.ScenarioArchitectureExplain,
		Predicates: types.SemanticPredicates{
			IsDiagnosticQuestion: true,
		},
		DiagnosticProfile: types.DiagnosticIntentProfile{
			IsDiagnostic: true,
			Confidence:   0.9,
		},
	}

	got, reason := reconcileDiagnosticQuestionProfile(rm)
	if reason == "" {
		t.Fatal("diagnostic predicate should still reconcile routing")
	}
	if got.Intent != types.IntentRootCause || got.Scenario != types.ScenarioRootCause {
		t.Fatalf("diagnostic predicate should route root_cause, got intent=%s scenario=%s", got.Intent, got.Scenario)
	}
}

func TestReconcileNonDiagnosticErrorGranularityRoute_NormalizesRootCauseDrift(t *testing.T) {
	rm := types.RequestModel{
		Intent:   types.IntentRootCause,
		Scenario: types.ScenarioRootCause,
		Predicates: types.SemanticPredicates{
			IsDiagnosticQuestion: false,
		},
		DiagnosticProfile: types.DiagnosticIntentProfile{
			IsDiagnostic: true, // profile-only mirror drift; not a route signal.
			Confidence:   0.8,
		},
		ErrorGranularityProfile: &types.ErrorGranularityProfile{
			IsGranularityQuestion: true,
			Confidence:            0.9,
		},
	}

	got, reason := reconcileNonDiagnosticErrorGranularityRoute(rm)
	if reason == "" {
		t.Fatal("expected non-diagnostic error-granularity route reconcile")
	}
	if got.Intent != types.IntentExplain {
		t.Fatalf("Intent = %q, want explain", got.Intent)
	}
	if got.Scenario != types.ScenarioGeneric {
		t.Fatalf("Scenario = %q, want generic", got.Scenario)
	}
	if got.DiagnosticProfile.IsDiagnostic {
		t.Fatal("profile-only diagnostic mirror drift should be cleared")
	}
}

func TestReconcileNonDiagnosticErrorGranularityRoute_PreservesDiagnosticSignals(t *testing.T) {
	tests := []struct {
		name string
		rm   types.RequestModel
	}{
		{
			name: "diagnostic predicate",
			rm: types.RequestModel{
				Predicates: types.SemanticPredicates{IsDiagnosticQuestion: true},
			},
		},
		{
			name: "current risk",
			rm: types.RequestModel{
				DiagnosticProfile: types.DiagnosticIntentProfile{CurrentRisk: true},
			},
		},
		{
			name: "current version diagnostic",
			rm: types.RequestModel{
				DiagnosticProfile: types.DiagnosticIntentProfile{IsDiagnostic: true, CurrentVersionCheck: true},
			},
		},
		{
			name: "runtime artifact",
			rm: types.RequestModel{
				LogTriage: &types.LogBundle{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.rm.Intent = types.IntentRootCause
			tt.rm.Scenario = types.ScenarioRootCause
			tt.rm.ErrorGranularityProfile = &types.ErrorGranularityProfile{IsGranularityQuestion: true}
			got, reason := reconcileNonDiagnosticErrorGranularityRoute(tt.rm)
			if reason != "" {
				t.Fatalf("diagnostic/runtime signal should not reconcile, got %q", reason)
			}
			if got.Intent != types.IntentRootCause || got.Scenario != types.ScenarioRootCause {
				t.Fatalf("route changed despite diagnostic/runtime signal: intent=%s scenario=%s", got.Intent, got.Scenario)
			}
		})
	}
}

func TestReconcileQualifiedCodeSymbolConfigDrift_RoutesQualifiedSymbolsToComparison(t *testing.T) {
	graph := &repomap.Graph{SymbolDefs: map[string][]*repomap.Symbol{
		"templateArchitectureExplain": {{
			Name: "templateArchitectureExplain",
			Kind: "function",
			File: "internal/analysis/compiler/templates.go",
		}},
		"templateRootCause": {{
			Name: "templateRootCause",
			Kind: "function",
			File: "internal/analysis/compiler/templates.go",
		}},
		"TaskGraph": {{
			Name: "TaskGraph",
			Kind: "struct",
			File: "internal/types/analysis_ir.go",
		}},
	}}
	rm := types.RequestModel{
		RawRequest: "compiler.templateArchitectureExplain 和 compiler.templateRootCause 在 TaskGraph 节点数、citation 下限、retry budget 上分别有什么差异？逐项对比并给出具体数值。",
		Intent:     types.IntentExplain,
		Scenario:   types.ScenarioConfigTrace,
		AnalyzerHints: types.AnalyzerHints{
			Kind:              string(types.ReqConfigMapping),
			Entities:          []string{"compiler.templateArchitectureExplain", "compiler.templateRootCause", "TaskGraph"},
			PrimaryEntities:   []string{"compiler.templateArchitectureExplain", "compiler.templateRootCause", "TaskGraph"},
			MentionedEntities: []string{"compiler.templateArchitectureExplain", "compiler.templateRootCause", "TaskGraph"},
		},
		SubTopics: []types.SubTopic{
			{Summary: "compiler.templateArchitectureExplain", Entities: []string{"compiler.templateArchitectureExplain"}},
			{Summary: "compiler.templateRootCause", Entities: []string{"compiler.templateRootCause"}},
		},
		AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey, Confidence: 0.8},
		PredicateAxis: types.AxisConfigure,
		Predicates: types.SemanticPredicates{
			IsScalarAnswer:     false,
			IsRoleLocateLookup: false,
			IsCountQuestion:    false,
			IsCrossComponent:   true,
		},
	}

	got, reason := reconcileQualifiedCodeSymbolConfigDrift(rm, graph)
	if reason == "" {
		t.Fatal("expected qualified code symbols to reconcile config drift")
	}
	if got.Scenario != types.ScenarioArchitectureExplain {
		t.Fatalf("scenario = %q, want architecture_explain", got.Scenario)
	}
	if got.AnalyzerHints.Kind != string(types.ReqMechanism) {
		t.Fatalf("question kind = %q, want mechanism", got.AnalyzerHints.Kind)
	}
	if got.AnswerSubject.Kind != types.SubjectGeneric {
		t.Fatalf("answer subject = %q, want generic", got.AnswerSubject.Kind)
	}
	if got.PredicateAxis != types.AxisUnknown {
		t.Fatalf("predicate axis = %q, want unknown", got.PredicateAxis)
	}
	if len(got.Buckets) != 2 {
		t.Fatalf("buckets len = %d, want 2: %+v", len(got.Buckets), got.Buckets)
	}
	if got.Buckets[0].Label != "compiler.templateArchitectureExplain" ||
		got.Buckets[1].Label != "compiler.templateRootCause" {
		t.Fatalf("unexpected bucket labels: %+v", got.Buckets)
	}
	if len(got.SubTopics) != 2 {
		t.Fatalf("subtopics len = %d, want 2: %+v", len(got.SubTopics), got.SubTopics)
	}
	if !containsStringSlice(got.AnalyzerHints.Entities, "templateArchitectureExplain") ||
		!containsStringSlice(got.AnalyzerHints.Entities, "templateRootCause") {
		t.Fatalf("canonical code aliases should be added as derived search entities: %+v", got.AnalyzerHints.Entities)
	}
	if gotFamily := types.ResolveQuestionFamily(got); gotFamily != types.QFComparison {
		t.Fatalf("question family = %q, want comparison", gotFamily)
	}
}

func TestReconcileQualifiedCodeSymbolConfigDrift_RequiresTypedMentionedEntities(t *testing.T) {
	graph := &repomap.Graph{SymbolDefs: map[string][]*repomap.Symbol{
		"templateArchitectureExplain": {{
			Name: "templateArchitectureExplain",
			Kind: "function",
			File: "internal/analysis/compiler/templates.go",
		}},
		"templateRootCause": {{
			Name: "templateRootCause",
			Kind: "function",
			File: "internal/analysis/compiler/templates.go",
		}},
	}}
	rm := types.RequestModel{
		RawRequest: "compiler.templateArchitectureExplain 和 compiler.templateRootCause 的配置项有什么区别？",
		Intent:     types.IntentConfigQuery,
		Scenario:   types.ScenarioConfigTrace,
		AnalyzerHints: types.AnalyzerHints{
			Kind:            string(types.ReqConfigMapping),
			Entities:        []string{"compiler.templateArchitectureExplain", "compiler.templateRootCause"},
			PrimaryEntities: []string{"compiler.templateArchitectureExplain", "compiler.templateRootCause"},
			// MentionedEntities intentionally empty: raw text and
			// generic entity lanes are not precise enough to justify a
			// hard config→architecture rewrite.
		},
		SubTopics: []types.SubTopic{
			{Summary: "compiler.templateArchitectureExplain", Entities: []string{"compiler.templateArchitectureExplain"}},
			{Summary: "compiler.templateRootCause", Entities: []string{"compiler.templateRootCause"}},
		},
		AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey, Confidence: 0.8},
		PredicateAxis: types.AxisConfigure,
		Predicates:    types.SemanticPredicates{IsCrossComponent: true},
	}

	got, reason := reconcileQualifiedCodeSymbolConfigDrift(rm, graph)
	if reason != "" {
		t.Fatalf("raw/entity fallback must not drive hard reconcile, got reason %q", reason)
	}
	if got.Intent != types.IntentConfigQuery || got.Scenario != types.ScenarioConfigTrace ||
		got.AnalyzerHints.Kind != string(types.ReqConfigMapping) ||
		got.AnswerSubject.Kind != types.SubjectConfigKey ||
		got.PredicateAxis != types.AxisConfigure {
		t.Fatalf("config classification changed without typed mentioned lane: %+v", got)
	}
}

func TestReconcileQualifiedCodeSymbolConfigDrift_RequiresTypedComparisonSignal(t *testing.T) {
	graph := &repomap.Graph{SymbolDefs: map[string][]*repomap.Symbol{
		"templateArchitectureExplain": {{
			Name: "templateArchitectureExplain",
			Kind: "function",
			File: "internal/analysis/compiler/templates.go",
		}},
		"templateRootCause": {{
			Name: "templateRootCause",
			Kind: "function",
			File: "internal/analysis/compiler/templates.go",
		}},
	}}
	rm := types.RequestModel{
		RawRequest: "compiler.templateArchitectureExplain 和 compiler.templateRootCause 的配置项有什么区别？",
		Intent:     types.IntentConfigQuery,
		Scenario:   types.ScenarioConfigTrace,
		AnalyzerHints: types.AnalyzerHints{
			Kind:              string(types.ReqConfigMapping),
			Entities:          []string{"compiler.templateArchitectureExplain", "compiler.templateRootCause"},
			PrimaryEntities:   []string{"compiler.templateArchitectureExplain", "compiler.templateRootCause"},
			MentionedEntities: []string{"compiler.templateArchitectureExplain", "compiler.templateRootCause"},
		},
		AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey, Confidence: 0.8},
		PredicateAxis: types.AxisConfigure,
	}

	got, reason := reconcileQualifiedCodeSymbolConfigDrift(rm, graph)
	if reason != "" {
		t.Fatalf("symbol surfaces alone must not drive hard reconcile, got reason %q", reason)
	}
	if got.Intent != types.IntentConfigQuery || got.Scenario != types.ScenarioConfigTrace ||
		got.AnalyzerHints.Kind != string(types.ReqConfigMapping) ||
		got.AnswerSubject.Kind != types.SubjectConfigKey ||
		got.PredicateAxis != types.AxisConfigure {
		t.Fatalf("config classification changed without typed comparison signal: %+v", got)
	}
}

func TestReconcileQualifiedCodeSymbolConfigDrift_LeavesRealConfigTraceWithoutSymbolProof(t *testing.T) {
	rm := types.RequestModel{
		RawRequest: "server.port 和 server.host 的默认配置有什么区别？",
		Intent:     types.IntentConfigQuery,
		Scenario:   types.ScenarioConfigTrace,
		AnalyzerHints: types.AnalyzerHints{
			Kind:              string(types.ReqConfigMapping),
			Entities:          []string{"server.port", "server.host"},
			PrimaryEntities:   []string{"server.port", "server.host"},
			MentionedEntities: []string{"server.port", "server.host"},
		},
		AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey, Confidence: 0.8},
		PredicateAxis: types.AxisConfigure,
	}

	got, reason := reconcileQualifiedCodeSymbolConfigDrift(rm, &repomap.Graph{SymbolDefs: map[string][]*repomap.Symbol{}})
	if reason != "" {
		t.Fatalf("real config trace without code-symbol proof should not reconcile: %s", reason)
	}
	if got.Scenario != types.ScenarioConfigTrace || got.Intent != types.IntentConfigQuery {
		t.Fatalf("config trace changed unexpectedly: %+v", got)
	}
}

func containsStringSlice(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func TestBuildAnalysisIR_ChangeImpactFileOutputRoutesEnumeration(t *testing.T) {
	mut := types.NewMutableState("Which files need changes if CitationReq.Required changes shape?")
	mut.SetRequestModel(types.RequestModel{
		RawRequest: "Which files need changes if CitationReq.Required changes shape?",
		Intent:     types.IntentTrace,
		Complexity: types.ComplexityModerate,
		AnalyzerHints: types.AnalyzerHints{
			Kind:         string(types.ReqCallChain),
			Entities:     []string{"CitationReq.Required"},
			ExactTargets: []string{"CitationReq.Required"},
		},
		Predicates: types.SemanticPredicates{
			IsScalarAnswer:     true,
			IsRoleLocateLookup: true,
			IsCrossComponent:   true,
		},
		ChangeImpactProfile: &types.ChangeImpactProfile{
			IsChangeImpact:  true,
			Target:          "CitationReq.Required",
			TargetKind:      types.SubjectStructField,
			RequestedOutput: types.ImpactOutputFiles,
			Confidence:      0.9,
		},
	})
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mut}

	ir, err := buildAnalysisIR(ctx)
	if err != nil {
		t.Fatalf("buildAnalysisIR: %v", err)
	}
	rm := ir.RequestModel
	if rm.Intent != types.IntentEnumerate || rm.AnalyzerHints.Kind != string(types.ReqEnumeration) {
		t.Fatalf("change-impact file output should route as enumeration, got intent=%q kind=%q", rm.Intent, rm.AnalyzerHints.Kind)
	}
	if !rm.Predicates.IsCategoryEnumeration || !rm.Predicates.IsRelationalLookup {
		t.Fatalf("change-impact file output should be set-valued relational enumeration, got %+v", rm.Predicates)
	}
	if rm.Predicates.IsCrossComponent {
		t.Fatalf("change-impact file output should not require cross-component sub-topic fan-out, got %+v", rm.Predicates)
	}
	if got := types.ResolveQuestionFamily(rm); got != types.QFEnumeration {
		t.Fatalf("question family = %q, want %q", got, types.QFEnumeration)
	}
	if ir.AnswerContract.ExactResolution != nil {
		t.Fatalf("broad change-impact output should not compile exact-resolution absence contract: %+v", ir.AnswerContract.ExactResolution)
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

func TestBuildAnalysisIR_ExternalOnlyRuntimeArtifactStripsAllThreeGates(t *testing.T) {
	mut := types.NewMutableState("这个混合栈崩溃涉及 ArkTS 和仓颉两种语言，分别在哪一帧出问题？")
	mut.SetLogTriage(&types.LogBundle{
		Errors: []types.LogError{{
			Type:    "panic",
			Message: "index out of bounds",
			Frames: []types.LogFrame{{
				Lang: "cangjie",
				Func: "demo.bridge.ohSum",
				File: "src/bridge/Bridge.cj",
				Line: 18,
			}},
		}},
	})
	mut.SetRequestModel(types.RequestModel{
		RawRequest: "这个混合栈崩溃涉及 ArkTS 和仓颉两种语言，分别在哪一帧出问题？",
		Intent:     types.IntentRootCause,
		Scenario:   types.ScenarioRootCause,
		Complexity: types.ComplexitySimple,
		AnalyzerHints: types.AnalyzerHints{
			Keywords: []string{"ArkTS", "仓颉", "panic"},
			Entities: []string{"ArkTS", "仓颉", "demo.bridge.ohSum", "NativeBridge.invokeOhSum"},
		},
		Predicates: types.SemanticPredicates{
			IsDiagnosticQuestion: true,
			IsCrossComponent:     true,
		},
		DiagnosticProfile: types.DiagnosticIntentProfile{
			IsDiagnostic: true,
			Confidence:   0.9,
		},
		ExternalObservationPolicy: &types.ExternalObservationPolicy{
			CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
			ExclusionKind:     types.ExternalObservationSourceExclusionExplicitUserBoundary,
			SourceQuotes:      []string{"只分析日志"},
			Confidence:        0.9,
		},
	})
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mut}

	ir, err := buildAnalysisIR(ctx)
	if err != nil {
		t.Fatalf("buildAnalysisIR: %v", err)
	}

	if ir.RequestModel.Predicates.IsCategoryEnumeration {
		t.Error("root-cause artifact questions must not be amplified into category enumeration")
	}
	if ir.AnswerContract.CitationReq.Required {
		t.Error("external-only runtime artifact should not require repo citations")
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

func TestBuildAnalysisIR_PreflightTraceSourceOptionalUsesRuntimeDAG(t *testing.T) {
	mut := types.NewMutableState("分析 capture.systrace 里 100..180ms 的卡顿根因")
	mut.SetRequestModel(types.RequestModel{
		RawRequest: "分析 capture.systrace 里 100..180ms 的卡顿根因",
		Intent:     types.IntentRootCause,
		Scenario:   types.ScenarioRootCause,
		Complexity: types.ComplexityComplex,
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"capture.systrace", "100..180ms", "app-100"},
		},
		Predicates: types.SemanticPredicates{
			IsDiagnosticQuestion: true,
			IsCrossComponent:     true,
		},
		DiagnosticProfile: types.DiagnosticIntentProfile{
			IsDiagnostic: true,
			Confidence:   0.9,
		},
	})
	ctx := &types.AgentContext{
		Stage:   types.StageAnalyze,
		Mutable: mut,
		RuntimeArtifactPreflight: types.NormalizeRuntimeArtifactPreflightProfile(types.RuntimeArtifactPreflightProfile{
			SourceNavigationOptional: true,
			Artifacts: []types.RuntimeArtifactPreflightArtifact{{
				Kind:    "trace",
				Source:  "capture.systrace",
				Carrier: "request_path",
			}},
		}),
	}

	ir, err := buildAnalysisIR(ctx)
	if err != nil {
		t.Fatalf("buildAnalysisIR: %v", err)
	}
	if ir.AnswerContract.CitationReq.Required {
		t.Fatal("source-optional trace artifact must not keep current-checkout citation floor")
	}
	for _, a := range ir.AnswerContract.AcceptanceTests {
		if a.Kind == types.CritCitationCountGE {
			t.Fatalf("AcceptanceTests still carries CritCitationCountGE: %+v", a)
		}
	}
	if got := ir.EvidencePlan.SourceMix["trace_query"]; got != 100 || len(ir.EvidencePlan.SourceMix) != 1 {
		t.Fatalf("source-optional trace should compile to trace_query-only source mix, got %+v", ir.EvidencePlan.SourceMix)
	}
	if got := ir.EvidencePlan.NodeBudgetHints.PerToolCap["trace_query"]; got <= 0 {
		t.Fatalf("trace_query budget cap missing: %+v", ir.EvidencePlan.NodeBudgetHints)
	}
	for _, n := range ir.TaskGraph.Nodes {
		if strings.Contains(n.Objective, "codebase") || strings.Contains(n.Objective, "call sites") {
			t.Fatalf("runtime-only trace DAG should not retain source-code objective on node %s: %q", n.ID, n.Objective)
		}
		if len(n.SearchHints.EntityIDs) > 0 || len(n.SearchHints.KeywordIDs) > 0 {
			t.Fatalf("runtime-only trace node %s should not keep source search hints: %+v", n.ID, n.SearchHints)
		}
		for _, c := range n.SuccessCriteria {
			if c.Kind == types.CritCitationCountGE {
				t.Fatalf("TaskNode %q SuccessCriteria still carries CritCitationCountGE: %+v", n.ID, c)
			}
		}
	}
}

func TestBuildAnalysisIR_PreflightTracePreciseCurrentSourceKeepsSourceDAG(t *testing.T) {
	mut := types.NewMutableState("结合当前源码解释 capture.systrace 里的 trace_query 解析机制")
	mut.SetRequestModel(types.RequestModel{
		RawRequest: "结合当前源码解释 capture.systrace 里的 trace_query 解析机制",
		Intent:     types.IntentRootCause,
		Scenario:   types.ScenarioRootCause,
		Complexity: types.ComplexityModerate,
		CurrentSourceExplanationProfile: &types.CurrentSourceExplanationProfile{
			IsCurrentSourceExplanationRequested: true,
			Modes: []types.CurrentSourceExplanationMode{
				types.CurrentSourceExplanationExplainCurrentMechanism,
			},
			SourceQuotes: []string{"internal/tracequery/query.go:42"},
			Confidence:   0.9,
		},
		Predicates: types.SemanticPredicates{
			IsDiagnosticQuestion: true,
			IsCrossComponent:     true,
		},
		DiagnosticProfile: types.DiagnosticIntentProfile{
			IsDiagnostic: true,
			Confidence:   0.9,
		},
	})
	ctx := &types.AgentContext{
		Stage:   types.StageAnalyze,
		Mutable: mut,
		RuntimeArtifactPreflight: types.NormalizeRuntimeArtifactPreflightProfile(types.RuntimeArtifactPreflightProfile{
			SourceNavigationOptional: true,
			Artifacts: []types.RuntimeArtifactPreflightArtifact{{
				Kind:    "trace",
				Source:  "capture.systrace",
				Carrier: "request_path",
			}},
		}),
	}

	ir, err := buildAnalysisIR(ctx)
	if err != nil {
		t.Fatalf("buildAnalysisIR: %v", err)
	}
	if !ir.AnswerContract.CitationReq.Required {
		t.Fatal("precise current-source trace question must keep citation floor")
	}
	if got := ir.RequestModel.CurrentSourceLaneDecision(); got != types.CurrentSourceLaneRequired {
		t.Fatalf("CurrentSourceLaneDecision=%s, want required", got)
	}
	if got := ir.EvidencePlan.SourceMix["trace_query"]; got == 100 && len(ir.EvidencePlan.SourceMix) == 1 {
		t.Fatalf("precise current-source trace question must not collapse to runtime-only source mix: %+v", ir.EvidencePlan.SourceMix)
	}
	foundSourceObjective := false
	for _, n := range ir.TaskGraph.Nodes {
		if strings.Contains(n.Objective, "codebase") || strings.Contains(n.Objective, "call sites") {
			foundSourceObjective = true
			break
		}
	}
	if !foundSourceObjective {
		t.Fatalf("precise current-source trace question should retain source-oriented DAG objectives: %+v", ir.TaskGraph.Nodes)
	}
}

func TestBuildAnalysisIR_ResolvedRuntimeArtifactKeepsCitationGates(t *testing.T) {
	mut := types.NewMutableState("这个 panic 在当前代码哪里触发？")
	mut.SetLogTriage(&types.LogBundle{
		ResolvedFiles: []string{"internal/agent/analyzer.go"},
		Errors: []types.LogError{{
			Type:    "panic",
			Message: "boom",
			Frames: []types.LogFrame{{
				Lang: "go",
				Func: "buildAnalysisIR",
				File: "internal/agent/analyzer.go",
				Line: 1200,
			}},
		}},
	})
	mut.SetRequestModel(types.RequestModel{
		RawRequest: "这个 panic 在当前代码哪里触发？",
		Intent:     types.IntentRootCause,
		Scenario:   types.ScenarioRootCause,
		Complexity: types.ComplexitySimple,
		AnalyzerHints: types.AnalyzerHints{
			Keywords: []string{"panic", "buildAnalysisIR"},
			Entities: []string{"buildAnalysisIR"},
		},
		Predicates: types.SemanticPredicates{
			IsDiagnosticQuestion: true,
		},
		DiagnosticProfile: types.DiagnosticIntentProfile{
			IsDiagnostic: true,
			Confidence:   0.9,
		},
	})
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mut}

	ir, err := buildAnalysisIR(ctx)
	if err != nil {
		t.Fatalf("buildAnalysisIR: %v", err)
	}
	if !ir.AnswerContract.CitationReq.Required {
		t.Error("resolved runtime artifact should keep citation gates for current-repo facts")
	}
}

func TestBuildAnalysisIR_ExternalRuntimeCurrentRiskWithoutCurrentVersionKeepsSourceDefault(t *testing.T) {
	mut := types.NewMutableState("这是什么错误？")
	mut.SetLogTriage(&types.LogBundle{
		Errors: []types.LogError{{
			Type:    "panic",
			Message: "called `Option::unwrap()` on a `None` value",
			Frames: []types.LogFrame{{
				Lang: "rust",
				Func: "my_app::config::load",
				File: "./src/config.rs",
				Line: 42,
			}},
		}},
	})
	mut.SetRequestModel(types.RequestModel{
		RawRequest: "这是什么错误？",
		Intent:     types.IntentRootCause,
		Scenario:   types.ScenarioRootCause,
		Complexity: types.ComplexitySimple,
		AnalyzerHints: types.AnalyzerHints{
			Keywords: []string{"panic", "unwrap", "None"},
			Entities: []string{"my_app::config::load", "src/config.rs:42"},
		},
		Predicates: types.SemanticPredicates{
			IsDiagnosticQuestion: true,
		},
		DiagnosticProfile: types.DiagnosticIntentProfile{
			IsDiagnostic:         true,
			CurrentRisk:          true,
			CurrentVersionCheck:  false,
			HistoricalRegression: false,
			Confidence:           0.95,
		},
	})
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mut}

	ir, err := buildAnalysisIR(ctx)
	if err != nil {
		t.Fatalf("buildAnalysisIR: %v", err)
	}
	if ir.RequestModel.DiagnosticProfile.CurrentRisk {
		t.Fatal("external-only artifact without typed current-version check must not open current-repo verification")
	}
	if ir.RequestModel.HasObservationOnlyRuntimeArtifact() {
		t.Fatalf("external runtime artifact must not be observation-only without typed exclusion: %+v", ir.RequestModel.DiagnosticProfile)
	}
	if ir.AnswerContract.CurrentStatusDiagnostic != nil && ir.AnswerContract.CurrentStatusDiagnostic.Required {
		t.Fatalf("CurrentStatusDiagnostic should not be required: %+v", ir.AnswerContract.CurrentStatusDiagnostic)
	}
	if ir.AnswerContract.CitationReq.Required {
		t.Fatal("source-optional external runtime artifact should not keep a hard current-source citation floor")
	}
}

func TestBuildAnalysisIR_LogObservationLookupKeepsSourceDefault(t *testing.T) {
	mut := types.NewMutableState("这段日志里 WARN 在附件日志的第几行？FATAL 是否存在？")
	mut.SetLogTriage(&types.LogBundle{
		Observations: []types.LogObservation{
			{
				Kind:       types.LogObservationRuntimeEvent,
				Subject:    "WARN 日志行号",
				Summary:    "WARN 出现在附件日志第 3 行",
				Evidence:   "2026-05-21T10:00:02Z WARN retrying slow dependency",
				Diagnostic: true,
				Confidence: 1,
			},
			{
				Kind:       types.LogObservationRuntimeEvent,
				Subject:    "FATAL 检查结果",
				Summary:    "FATAL 在附件日志中不存在",
				Evidence:   "附件日志中未搜索到 FATAL 字样",
				Confidence: 1,
			},
		},
	})
	mut.SetRequestModel(types.RequestModel{
		RawRequest: "这段日志里 WARN 在附件日志的第几行？FATAL 是否存在？",
		Intent:     types.IntentRootCause,
		Scenario:   types.ScenarioRootCause,
		Complexity: types.ComplexitySimple,
		AnalyzerHints: types.AnalyzerHints{
			Keywords: []string{"WARN", "FATAL", "附件日志", "行号"},
			Entities: []string{"WARN", "FATAL"},
		},
		Predicates: types.SemanticPredicates{
			IsDiagnosticQuestion: true,
		},
		DiagnosticProfile: types.DiagnosticIntentProfile{
			IsDiagnostic: true,
			Confidence:   0.95,
		},
	})
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mut}

	ir, err := buildAnalysisIR(ctx)
	if err != nil {
		t.Fatalf("buildAnalysisIR: %v", err)
	}
	if ir.RequestModel.HasObservationOnlyRuntimeArtifact() {
		t.Fatalf("log observation lookup must not be observation-only without typed exclusion: %+v", ir.RequestModel)
	}
}

func TestBuildAnalysisIR_ExternalOnlyCurrentVersionCheckKeepsCurrentStatus(t *testing.T) {
	mut := types.NewMutableState("确认这个日志问题当前版本 my_app::config::load 是否还存在")
	mut.SetLogTriage(&types.LogBundle{
		Errors: []types.LogError{{
			Type: "panic",
			Frames: []types.LogFrame{{
				Lang: "rust",
				Func: "my_app::config::load",
				File: "./src/config.rs",
				Line: 42,
			}},
		}},
	})
	mut.SetRequestModel(types.RequestModel{
		RawRequest: "确认这个日志问题当前版本 my_app::config::load 是否还存在",
		Intent:     types.IntentRootCause,
		Scenario:   types.ScenarioRootCause,
		Complexity: types.ComplexitySimple,
		AnalyzerHints: types.AnalyzerHints{
			ExactTargets: []string{"my_app::config::load"},
		},
		Predicates: types.SemanticPredicates{
			IsDiagnosticQuestion: true,
		},
		DiagnosticProfile: types.DiagnosticIntentProfile{
			IsDiagnostic:        true,
			CurrentRisk:         true,
			CurrentVersionCheck: true,
			Confidence:          0.95,
		},
	})
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mut}

	ir, err := buildAnalysisIR(ctx)
	if err != nil {
		t.Fatalf("buildAnalysisIR: %v", err)
	}
	if !ir.RequestModel.DiagnosticProfile.CurrentRisk {
		t.Fatal("explicit current-version check must preserve current-risk routing")
	}
	if ir.RequestModel.HasObservationOnlyRuntimeArtifact() {
		t.Fatalf("current-version check must not be observation-only: %+v", ir.RequestModel.DiagnosticProfile)
	}
}

func TestBuildAnalysisIR_ExternalRuntimeSpuriousCurrentVersionCheckKeepsSourceDefault(t *testing.T) {
	mut := types.NewMutableState("哪些 goroutine 同时出错？它们的共同问题是什么？")
	mut.SetLogTriage(&types.LogBundle{
		Errors: []types.LogError{{
			Type:    "fatal error",
			Message: "concurrent map writes",
			Frames: []types.LogFrame{{
				Lang: "go",
				Func: "main.writeSession",
				File: "internal/agent/analyzer.go",
				Line: 100,
			}},
		}},
	})
	mut.SetRequestModel(types.RequestModel{
		RawRequest: "哪些 goroutine 同时出错？它们的共同问题是什么？",
		Intent:     types.IntentRootCause,
		Scenario:   types.ScenarioRootCause,
		Complexity: types.ComplexitySimple,
		AnalyzerHints: types.AnalyzerHints{
			Keywords: []string{"goroutine", "concurrent map writes"},
			Entities: []string{"writeSession", "goroutine 15", "goroutine 87", "goroutine 120"},
		},
		Predicates: types.SemanticPredicates{
			IsDiagnosticQuestion: true,
		},
		DiagnosticProfile: types.DiagnosticIntentProfile{
			IsDiagnostic:        true,
			CurrentRisk:         true,
			CurrentVersionCheck: true,
			Confidence:          0.95,
		},
	})
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mut}

	ir, err := buildAnalysisIR(ctx)
	if err != nil {
		t.Fatalf("buildAnalysisIR: %v", err)
	}
	if ir.RequestModel.DiagnosticProfile.CurrentRisk ||
		ir.RequestModel.DiagnosticProfile.CurrentVersionCheck ||
		ir.RequestModel.DiagnosticProfile.HistoricalRegression {
		t.Fatalf("external-only artifact without current-source anchor should clear current-status flags: %+v", ir.RequestModel.DiagnosticProfile)
	}
	if ir.RequestModel.HasObservationOnlyRuntimeArtifact() {
		t.Fatalf("runtime artifact must not be observation-only without typed exclusion: %+v", ir.RequestModel)
	}
	if ir.AnswerContract.CurrentStatusDiagnostic != nil && ir.AnswerContract.CurrentStatusDiagnostic.Required {
		t.Fatalf("CurrentStatusDiagnostic should not be required: %+v", ir.AnswerContract.CurrentStatusDiagnostic)
	}
}

func TestBuildAnalysisIR_ExternalRuntimeDiagnosticMechanismBridgeKeepsSourceRequired(t *testing.T) {
	mut := types.NewMutableState("这段日志显示 finalizer 因 LLM timeout 触发重试；请结合当前源码解释系统如何区分模型响应超时和成文校验失败，并说明这个日志结论的边界。")
	mut.SetLogTriage(&types.LogBundle{
		Observations: []types.LogObservation{{
			Kind:      types.LogObservationRetryCycle,
			Summary:   "first_byte_timeout exceeded after 40s",
			LineStart: 2,
		}},
	})
	mut.SetRequestModel(types.RequestModel{
		RawRequest: "这段日志显示 finalizer 因 LLM timeout 触发重试；请结合当前源码解释系统如何区分模型响应超时和成文校验失败，并说明这个日志结论的边界。",
		Intent:     types.IntentRootCause,
		Scenario:   types.ScenarioRootCause,
		Complexity: types.ComplexityModerate,
		Predicates: types.SemanticPredicates{
			IsDiagnosticQuestion: true,
			IsCrossComponent:     true,
		},
		DiagnosticProfile: types.DiagnosticIntentProfile{
			IsDiagnostic:        true,
			CurrentVersionCheck: true,
			Confidence:          0.9,
		},
	})
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mut}

	ir, err := buildAnalysisIR(ctx)
	if err != nil {
		t.Fatalf("buildAnalysisIR: %v", err)
	}
	if !ir.RequestModel.DiagnosticProfile.CurrentVersionCheck {
		t.Fatalf("typed diagnostic mechanism bridge should preserve current-version source lane: %+v", ir.RequestModel.DiagnosticProfile)
	}
	if got := ir.RequestModel.CurrentSourceLaneDecision(); got != types.CurrentSourceLaneRequired {
		t.Fatalf("CurrentSourceLaneDecision=%s, want required", got)
	}
	if ir.RequestModel.HasObservationOnlyRuntimeArtifact() {
		t.Fatalf("diagnostic mechanism bridge must not be observation-only: %+v", ir.RequestModel)
	}
}

func TestBuildAnalysisIR_DiagnosticExplanationClearsScalarAnswerSubjectBeforeGate(t *testing.T) {
	mut := types.NewMutableState("这是什么错误？")
	mut.SetLogTriage(&types.LogBundle{
		Errors: []types.LogError{{
			Type:    "panic",
			Message: "called Option::unwrap() on a None value",
			Frames: []types.LogFrame{{
				Lang: "rust",
				Func: "my_app::config::load",
				File: "./src/config.rs",
				Line: 42,
			}},
		}},
	})
	mut.SetRequestModel(types.RequestModel{
		RawRequest: "这是什么错误？",
		Intent:     types.IntentRootCause,
		Scenario:   types.ScenarioRootCause,
		Complexity: types.ComplexitySimple,
		AnalyzerHints: types.AnalyzerHints{
			Kind:     "mechanism",
			Keywords: []string{"panic", "unwrap", "None"},
			Entities: []string{"my_app::config::load"},
		},
		AnswerSubject: types.AnswerSubject{
			Kind:       types.SubjectStringLiteral,
			Confidence: 0.9,
		},
		Predicates: types.SemanticPredicates{
			IsDiagnosticQuestion: true,
			IsScalarAnswer:       false,
		},
		DiagnosticProfile: types.DiagnosticIntentProfile{
			IsDiagnostic: true,
			Confidence:   0.95,
		},
	})
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mut}

	ir, err := buildAnalysisIR(ctx)
	if err != nil {
		t.Fatalf("buildAnalysisIR: %v", err)
	}
	if ir.QualityGate.Rejected {
		t.Fatalf("quality gate should pass after subject reconcile: %+v", ir.QualityGate)
	}
	if ir.RequestModel.AnswerSubject.Kind != types.SubjectGeneric {
		t.Fatalf("answer subject = %q, want generic fallback", ir.RequestModel.AnswerSubject.Kind)
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

func TestBuildAnalysisIR_IsolatedCurrentVersionCheckDoesNotHijackConfigLookup(t *testing.T) {
	mut := types.NewMutableState("feature_x_timeout 在默认值 / yaml / CLI 三层的有效值是什么")
	mut.SetRequestModel(types.RequestModel{
		RawRequest: "feature_x_timeout 在默认值 / yaml / CLI 三层的有效值是什么",
		Intent:     types.IntentConfigQuery,
		Scenario:   types.ScenarioGeneric,
		Complexity: types.ComplexitySimple,
		Predicates: types.SemanticPredicates{IsDiagnosticQuestion: false},
		DiagnosticProfile: types.DiagnosticIntentProfile{
			IsDiagnostic:         false,
			CurrentRisk:          false,
			HistoricalRegression: false,
			CurrentVersionCheck:  true,
			Confidence:           0.85,
		},
		AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey, Confidence: 0.9},
		AnalyzerHints: types.AnalyzerHints{
			Keywords: []string{"feature_x_timeout", "yaml", "CLI", "default"},
			Entities: []string{"feature_x_timeout"},
			Kind:     "config_mapping",
		},
	})
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mut}

	ir, err := buildAnalysisIR(ctx)
	if err != nil {
		t.Fatalf("buildAnalysisIR: %v", err)
	}
	if ir.RequestModel.Intent == types.IntentRootCause || ir.RequestModel.Predicates.IsDiagnosticQuestion {
		t.Fatalf("isolated current_version_check hijacked config lookup into diagnostic lane: intent=%s preds=%+v", ir.RequestModel.Intent, ir.RequestModel.Predicates)
	}
	if ir.RequestModel.ArtifactObservationProfile != nil {
		t.Fatalf("isolated current_version_check should not create observation profile: %+v", ir.RequestModel.ArtifactObservationProfile)
	}
	if ir.AnswerContract.CurrentStatusDiagnostic != nil {
		t.Fatalf("isolated current_version_check should not create current-status contract: %+v", ir.AnswerContract.CurrentStatusDiagnostic)
	}
	if ir.AnswerContract.ExactResolution == nil {
		t.Fatal("config lookup should keep exact-resolution contract")
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
	if !types.ObservationKindsContain(profile.ObservationKinds, types.ObservationKindDiagnosticQuestion) ||
		!types.ObservationKindsContain(profile.ObservationKinds, types.ObservationKindCurrentVersionCheck) {
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
	if ir.AnswerContract.Diagram.RequiredKind != types.DiagramCallDAG {
		t.Fatalf("Diagram.RequiredKind = %q, want call_dag", ir.AnswerContract.Diagram.RequiredKind)
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
		{"enumerate + is_count_question keeps enumeration intent",
			types.IntentEnumerate, types.SemanticPredicates{IsCountQuestion: true}, nil,
			types.IntentEnumerate, true},
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
		{"count predicate does not steal enumerate intent even with log bundle",
			types.IntentEnumerate, types.SemanticPredicates{IsCountQuestion: true}, rootCauseBundle,
			types.IntentEnumerate, true},
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

	t.Run("non-scalar history summary still uses VCS metadata", func(t *testing.T) {
		rm := types.RequestModel{
			RawRequest: "最近一次合入的是什么特性？请说明特性内容。",
			Intent:     types.IntentExplain,
			AnalyzerHints: types.AnalyzerHints{
				Kind:     "history",
				Keywords: []string{"merge", "feature"},
			},
			Predicates: types.SemanticPredicates{
				IsScalarAnswer:  false,
				IsHistoryLookup: true,
			},
		}
		if !isHistoryLookupRequest(rm) {
			t.Fatal("history predicate should trigger even when the requested answer is a summary, not a scalar")
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

	t.Run("failure scope decision downgrades to generic", func(t *testing.T) {
		rm := types.RequestModel{
			Scenario: types.ScenarioArchitectureExplain,
			Intent:   types.IntentExplain,
			ErrorGranularityProfile: &types.ErrorGranularityProfile{
				IsGranularityQuestion: true,
				Confidence:            0.9,
			},
			AnalyzerHints: types.AnalyzerHints{
				Entities: []string{"emit_evidence", "anchor_kind"},
			},
		}
		got, reason := reconcileScenario(rm)
		if got != types.ScenarioGeneric {
			t.Fatalf("scenario = %q, want %q", got, types.ScenarioGeneric)
		}
		if reason == "" {
			t.Fatal("reason = empty, want non-empty reconcile reason")
		}
		rm.Intent = types.IntentRootCause
		rm.Scenario = types.ScenarioRootCause
		got, reason = reconcileScenario(rm)
		if got != types.ScenarioGeneric {
			t.Fatalf("root-cause-labeled failure-scope scenario = %q, want %q", got, types.ScenarioGeneric)
		}
		if reason == "" {
			t.Fatal("root-cause-labeled reconcile reason = empty, want non-empty")
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
		name         string
		rm           types.RequestModel
		bundle       *types.LogBundle
		wantNil      bool
		wantRequired bool
		wantKind     types.DiagramKind
		wantScope    types.DiagramScope
	}{
		{
			name:    "step_list without structural cues stays nil",
			rm:      types.RequestModel{},
			wantNil: true,
		},
		{
			name: "trace intent prefers call dag without requiring diagram",
			rm: types.RequestModel{
				Intent: types.IntentTrace,
			},
			wantKind:     types.DiagramCallDAG,
			wantScope:    types.DiagramScopeOverall,
			wantRequired: false,
		},
		{
			name: "architecture scenario prefers architecture diagram without requiring it",
			rm: types.RequestModel{
				Scenario: types.ScenarioArchitectureExplain,
			},
			wantKind:     types.DiagramArchitecture,
			wantScope:    types.DiagramScopeOverall,
			wantRequired: false,
		},
		{
			name: "scalar call question only prefers diagram",
			rm: types.RequestModel{
				PredicateAxis: types.AxisCall,
			},
			wantKind:     types.DiagramCallDAG,
			wantScope:    types.DiagramScopeOverall,
			wantRequired: false,
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
			name: "config trace prefers diagram without replacing requested answer shape",
			rm: types.RequestModel{
				PredicateAxis: types.AxisConfigure,
				Scenario:      types.ScenarioConfigTrace,
				Intent:        types.IntentConfigQuery,
			},
			wantKind:     types.DiagramArchitecture,
			wantScope:    types.DiagramScopeOverall,
			wantRequired: false,
		},
		{
			name:         "log call chain prefers diagram without requiring it",
			rm:           types.RequestModel{},
			bundle:       logBundle,
			wantKind:     types.DiagramCallDAG,
			wantScope:    types.DiagramScopeOverall,
			wantRequired: false,
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
			wantKind:     types.DiagramCallDAG,
			wantScope:    types.DiagramScopePerSubTopic,
			wantRequired: false,
		},
		{
			name: "explicit diagram hint is the only hard diagram request",
			rm: types.RequestModel{
				DiagramHint: &types.DiagramHint{Kind: types.DiagramSequence},
			},
			wantKind:     types.DiagramSequence,
			wantScope:    types.DiagramScopeOverall,
			wantRequired: true,
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
			if got.Required != tc.wantRequired {
				t.Fatalf("Required = %v, want %v (%+v)", got.Required, tc.wantRequired, got)
			}
			wantMinimum := 0
			if tc.wantRequired {
				wantMinimum = 1
			}
			if got.Minimum != wantMinimum {
				t.Fatalf("Minimum = %d, want %d", got.Minimum, wantMinimum)
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
