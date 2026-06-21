package compiler

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/budget"
	"github.com/hanchaoqun/codrax/internal/types"
)

// compileT is the test helper that mirrors Compile with a default
// BudgetSignals. Keeps the test call sites terse after the Compile
// signature gained the signals parameter.
func compileT(rm types.RequestModel) Output {
	return Compile(rm, budget.BudgetSignals{
		Complexity:      rm.Complexity,
		TermCount:       len(rm.TermGraph.Canonical),
		HypothesisCount: 1,
		PrescanHitRatio: 1.0,
	})
}

func containsString(in []string, want string) bool {
	for _, s := range in {
		if s == want {
			return true
		}
	}
	return false
}

func sampleRM(scenario types.Scenario, intent types.Intent, complexity types.Complexity) types.RequestModel {
	return types.RequestModel{
		RawRequest: "sample",
		Language:   "zh",
		Intent:     intent,
		Scenario:   scenario,
		Complexity: complexity,
		TermGraph: types.TermGraph{
			Canonical: []types.CanonicalTerm{
				{ID: "code:explorer", Surface: "Explorer", Language: "code", Kind: types.TermSymbol},
				{ID: "en:stop", Surface: "stop", Language: "en", Kind: types.TermConcept},
				{ID: "cfg:config/orchestrator.yaml", Surface: "config/orchestrator.yaml", Language: "code", Kind: types.TermConfig},
			},
		},
	}
}

// countNodeType returns how many nodes of the given type exist in g.
func countNodeType(g types.TaskGraph, t types.TaskNodeType) int {
	n := 0
	for _, node := range g.Nodes {
		if node.Type == t {
			n++
		}
	}
	return n
}

func hasEdge(g types.TaskGraph, from, to string, et types.EdgeType) bool {
	for _, e := range g.Edges {
		if e.From == from && e.To == to && e.EdgeType == et {
			return true
		}
	}
	return false
}

func TestCompile_ArchitectureExplain_HasExplanationContract(t *testing.T) {
	out := compileT(sampleRM(types.ScenarioArchitectureExplain, types.IntentExplain, types.ComplexityModerate))
	if countNodeType(out.TaskGraph, types.NodeProbe) < 1 {
		t.Fatalf("want at least 1 probe node")
	}
	if countNodeType(out.TaskGraph, types.NodeExtract) != 1 {
		t.Fatalf("want 1 extract stage node")
	}
	if countNodeType(out.TaskGraph, types.NodeFinalize) != 1 {
		t.Fatalf("want 1 finalize node")
	}
	// All edges must reference nodes that exist.
	ids := make(map[string]bool)
	for _, n := range out.TaskGraph.Nodes {
		ids[n.ID] = true
	}
	for _, e := range out.TaskGraph.Edges {
		if !ids[e.From] || !ids[e.To] {
			t.Fatalf("dangling edge %+v", e)
		}
	}
}

func TestStageExpansionOptInPreciseBoolean(t *testing.T) {
	out := compileT(sampleRM(types.ScenarioArchitectureExplain, types.IntentExplain, types.ComplexityModerate))
	var reprobe, finalize types.TaskNode
	for _, n := range out.TaskGraph.Nodes {
		if n.Type == types.NodeProbe && n.Optional {
			for _, c := range n.EntryConditions {
				if c.Kind == types.CritSourceClassUniverseIncomplete {
					reprobe = n
				}
			}
		}
		if n.Type == types.NodeFinalize {
			finalize = n
		}
	}
	if reprobe.ID == "" {
		t.Fatalf("missing optional source-inventory reprobe node: %+v", out.TaskGraph.Nodes)
	}
	if reprobe.OneShot != true || reprobe.MaxRetries != 1 {
		t.Fatalf("reprobe should be bounded one-shot; got %+v", reprobe)
	}
	if !containsString(reprobe.Outputs, "source_inventory_observation") || !containsString(reprobe.Outputs, "source_classes") {
		t.Fatalf("reprobe outputs missing source inventory artifacts: %v", reprobe.Outputs)
	}
	if finalize.ID == "" || !hasEdge(out.TaskGraph, reprobe.ID, finalize.ID, types.EdgeSoftDependency) {
		t.Fatalf("reprobe should have a soft edge to finalize; edges=%+v", out.TaskGraph.Edges)
	}
	for _, id := range out.TaskGraph.ExecutionPolicy.CriticalPath {
		if id == reprobe.ID {
			t.Fatalf("optional reprobe must not enter the default critical path: %v", out.TaskGraph.ExecutionPolicy.CriticalPath)
		}
	}
}

func TestCompile_ExtractStageNodeWiresBeforeFinalize(t *testing.T) {
	out := compileT(sampleRM(types.ScenarioArchitectureExplain, types.IntentExplain, types.ComplexityModerate))
	var extract, finalize types.TaskNode
	for _, n := range out.TaskGraph.Nodes {
		switch n.Type {
		case types.NodeExtract:
			extract = n
		case types.NodeFinalize:
			finalize = n
		}
	}
	if extract.ID == "" || finalize.ID == "" {
		t.Fatalf("missing extract/finalize nodes: %+v", out.TaskGraph.Nodes)
	}
	if len(extract.EntryConditions) != 1 || extract.EntryConditions[0].Kind != types.CritExtractInputReady {
		t.Fatalf("extract node must carry typed extract readiness; got %+v", extract.EntryConditions)
	}
	if !containsString(extract.Outputs, "answer_symbols") || !containsString(extract.Outputs, "hypothesis_verdicts") {
		t.Fatalf("extract outputs missing answer-ready artifacts: %v", extract.Outputs)
	}
	if !hasEdge(out.TaskGraph, extract.ID, finalize.ID, types.EdgeHardDependency) {
		t.Fatalf("extract must feed finalize; edges=%+v", out.TaskGraph.Edges)
	}
	posExtract, posFinalize := -1, -1
	for i, id := range out.TaskGraph.ExecutionPolicy.CriticalPath {
		if id == extract.ID {
			posExtract = i
		}
		if id == finalize.ID {
			posFinalize = i
		}
	}
	if posExtract < 0 || posFinalize < 0 || posExtract > posFinalize {
		t.Fatalf("critical path must place extract before finalize: %v", out.TaskGraph.ExecutionPolicy.CriticalPath)
	}
}

func TestCompile_RootCause_HasValidationFeedback(t *testing.T) {
	out := compileT(sampleRM(types.ScenarioRootCause, types.IntentRootCause, types.ComplexityComplex))
	// validate → evidence feedback edge must exist.
	var validateID, evidenceID string
	for _, n := range out.TaskGraph.Nodes {
		if n.Type == types.NodeValidate {
			validateID = n.ID
		}
		if n.Type == types.NodeEvidence {
			evidenceID = n.ID
		}
	}
	if validateID == "" || evidenceID == "" {
		t.Fatalf("missing validate or evidence node")
	}
	if !hasEdge(out.TaskGraph, validateID, evidenceID, types.EdgeValidationFeedback) {
		t.Fatalf("expected validation_feedback edge %s→%s; edges=%+v", validateID, evidenceID, out.TaskGraph.Edges)
	}
}

func TestCompile_RootCause_CurrentStatusDiagnosticHasThreeLaneContract(t *testing.T) {
	rm := sampleRM(types.ScenarioRootCause, types.IntentRootCause, types.ComplexityComplex)
	rm.Predicates.IsDiagnosticQuestion = true
	rm.DiagnosticProfile = types.DiagnosticIntentProfile{
		IsDiagnostic:        true,
		CurrentRisk:         true,
		CurrentVersionCheck: true,
		Confidence:          0.9,
	}
	out := compileT(rm)
	if out.AnswerContract.CurrentStatusDiagnostic == nil || !out.AnswerContract.CurrentStatusDiagnostic.Required {
		t.Fatalf("CurrentStatusDiagnostic contract missing: %+v", out.AnswerContract.CurrentStatusDiagnostic)
	}
	var finalObjective string
	var probeOutputs []string
	for _, n := range out.TaskGraph.Nodes {
		if n.Type == types.NodeProbe && !n.Optional {
			probeOutputs = n.Outputs
		}
		if n.Type == types.NodeFinalize {
			finalObjective = n.Objective
		}
	}
	if !containsString(probeOutputs, "historical_observation") || !containsString(probeOutputs, "current_code_targets") {
		t.Fatalf("probe outputs do not carry both lanes: %v", probeOutputs)
	}
	if !strings.Contains(finalObjective, "historical observation") ||
		!strings.Contains(finalObjective, "current code verification") ||
		!strings.Contains(finalObjective, "still_present") {
		t.Fatalf("final objective missing current-status surfaces: %q", finalObjective)
	}
}

func TestCompile_SearchHintsIncludeArtifactObservationSubjects(t *testing.T) {
	rm := sampleRM(types.ScenarioRootCause, types.IntentRootCause, types.ComplexityComplex)
	rm.Predicates.IsDiagnosticQuestion = true
	rm.DiagnosticProfile = types.DiagnosticIntentProfile{
		IsDiagnostic:        true,
		CurrentVersionCheck: true,
		Confidence:          0.9,
	}
	rm.ArtifactObservationProfile = &types.ArtifactObservationProfile{
		Source:            "user_request",
		SubjectCandidates: []string{"Finalizer", "emit_evidence"},
	}

	out := compileT(rm)
	var foundEntity, foundKeyword bool
	for _, n := range out.TaskGraph.Nodes {
		if containsString(n.SearchHints.EntityIDs, "Finalizer") &&
			containsString(n.SearchHints.EntityIDs, "emit_evidence") {
			foundEntity = true
		}
		if containsString(n.SearchHints.KeywordIDs, "Finalizer") &&
			containsString(n.SearchHints.KeywordIDs, "emit_evidence") {
			foundKeyword = true
		}
	}
	if !foundEntity || !foundKeyword {
		t.Fatalf("artifact observation subjects not projected into soft search hints: graph=%+v", out.TaskGraph.Nodes)
	}
}

func TestCompile_SearchHintsIncludeConversationReferenceSubjects(t *testing.T) {
	rm := sampleRM(types.ScenarioArchitectureExplain, types.IntentExplain, types.ComplexityModerate)
	rm.ConversationReferenceProfile = &types.ConversationReferenceProfile{
		RequiresPriorContext:  true,
		NeedsRepoVerification: true,
		Ambiguity:             types.ConversationReferenceAmbiguityNone,
		ResolvedSubjects: []types.ResolvedConversationSubject{{
			Surface: "buildAnalysisIR",
			Kind:    types.SubjectFunctionName,
			Source:  types.ConversationReferenceSourcePriorContext,
			Role:    "primary_subject",
		}},
	}

	out := compileT(rm)
	var foundEntity, foundKeyword bool
	for _, n := range out.TaskGraph.Nodes {
		if containsString(n.SearchHints.EntityIDs, "buildAnalysisIR") {
			foundEntity = true
		}
		if containsString(n.SearchHints.KeywordIDs, "buildAnalysisIR") {
			foundKeyword = true
		}
	}
	if !foundEntity || !foundKeyword {
		t.Fatalf("conversation reference subjects not projected into soft search hints: graph=%+v", out.TaskGraph.Nodes)
	}
}

func TestCompile_SearchHintsIncludeChangeImpactTarget(t *testing.T) {
	rm := sampleRM(types.ScenarioGeneric, types.IntentEnumerate, types.ComplexityComplex)
	rm.ChangeImpactProfile = &types.ChangeImpactProfile{
		IsChangeImpact:  true,
		Target:          "CitationReq.Required",
		TargetKind:      types.SubjectStructField,
		Scope:           types.ImpactScopeProduction,
		RequestedOutput: types.ImpactOutputFiles,
		Confidence:      0.92,
	}

	out := compileT(rm)
	var foundEntity, foundKeyword bool
	for _, n := range out.TaskGraph.Nodes {
		if containsString(n.SearchHints.EntityIDs, "CitationReq.Required") {
			foundEntity = true
		}
		if containsString(n.SearchHints.KeywordIDs, "CitationReq.Required") {
			foundKeyword = true
		}
	}
	if !foundEntity || !foundKeyword {
		t.Fatalf("change impact target not projected into soft search hints: graph=%+v", out.TaskGraph.Nodes)
	}
}

func TestCompile_SearchHintsIncludeAnalyzerEntities(t *testing.T) {
	rm := sampleRM(types.ScenarioArchitectureExplain, types.IntentExplain, types.ComplexityModerate)
	rm.AnalyzerHints.Entities = []string{"Namespace::Planner::Build", "templateArchitectureExplain"}
	rm.AnalyzerHints.DerivedEntities = []string{"PlannerBuild"}

	out := compileT(rm)
	var foundQualified, foundDerived bool
	for _, n := range out.TaskGraph.Nodes {
		if containsString(n.SearchHints.EntityIDs, "Namespace::Planner::Build") &&
			containsString(n.SearchHints.KeywordIDs, "templateArchitectureExplain") {
			foundQualified = true
		}
		if containsString(n.SearchHints.EntityIDs, "PlannerBuild") &&
			containsString(n.SearchHints.KeywordIDs, "PlannerBuild") {
			foundDerived = true
		}
	}
	if !foundQualified || !foundDerived {
		t.Fatalf("analyzer entities should project into soft search hints: graph=%+v", out.TaskGraph.Nodes)
	}
}

func TestCompile_ConfigTrace_BudgetReactsToComplexity(t *testing.T) {
	out := compileT(sampleRM(types.ScenarioConfigTrace, types.IntentConfigQuery, types.ComplexitySimple))
	// Simple complexity must give a smaller budget than moderate.
	moderate := compileT(sampleRM(types.ScenarioConfigTrace, types.IntentConfigQuery, types.ComplexityModerate))
	if out.EvidencePlan.Budget.MaxFiles >= moderate.EvidencePlan.Budget.MaxFiles {
		t.Fatalf("simple budget %d should be < moderate %d",
			out.EvidencePlan.Budget.MaxFiles, moderate.EvidencePlan.Budget.MaxFiles)
	}
}

func TestCompile_PerformanceBottleneck_ProducesContract(t *testing.T) {
	out := compileT(sampleRM(types.ScenarioPerformanceBottleneck, types.IntentTrace, types.ComplexityModerate))
	if !out.AnswerContract.CitationReq.Required {
		t.Fatalf("perf bottleneck contract must require citations: %+v", out.AnswerContract.CitationReq)
	}
}

func TestCompile_Generic_ProducesContract(t *testing.T) {
	for _, intent := range []types.Intent{
		types.IntentExplain,
		types.IntentReturnValue,
		types.IntentEnumerate,
		types.IntentConfigQuery,
	} {
		out := compileT(sampleRM(types.ScenarioGeneric, intent, types.ComplexityModerate))
		if !out.AnswerContract.CitationReq.Required {
			t.Fatalf("intent=%s: generic contract must require citations: %+v", intent, out.AnswerContract.CitationReq)
		}
	}
}

func TestCompile_StructuralTraceUsesTraceWalkthroughTemplate(t *testing.T) {
	rm := sampleRM(types.ScenarioArchitectureExplain, types.IntentTrace, types.ComplexityModerate)
	rm.PredicateAxis = types.AxisCall
	rm.AnalyzerHints.Kind = "call_chain"

	out := compileT(rm)
	if countNodeType(out.TaskGraph, types.NodeReconcile) != 0 {
		t.Fatalf("single-topic structural trace should not allocate a reconcile node; graph=%+v", out.TaskGraph.Nodes)
	}
	if countNodeType(out.TaskGraph, types.NodeValidate) != 1 {
		t.Fatalf("single-topic structural trace should keep one validate node")
	}
	if out.AnswerContract.CitationReq.MinCitations != TmplCitationCountMedium {
		t.Fatalf("citation floor = %d, want %d", out.AnswerContract.CitationReq.MinCitations, TmplCitationCountMedium)
	}
}

func TestCompile_SingleTopicCrossComponentTraceStillUsesTraceWalkthroughTemplate(t *testing.T) {
	rm := sampleRM(types.ScenarioArchitectureExplain, types.IntentTrace, types.ComplexityModerate)
	rm.PredicateAxis = types.AxisCall
	rm.AnalyzerHints.Kind = "call_chain"
	rm.Predicates.IsCrossComponent = true

	out := compileT(rm)
	if countNodeType(out.TaskGraph, types.NodeReconcile) != 0 {
		t.Fatalf("single-topic cross-component trace should still avoid reconcile node")
	}
}

func TestCompile_MultiTopicCrossComponentTraceKeepsArchitectureTemplate(t *testing.T) {
	rm := sampleRM(types.ScenarioArchitectureExplain, types.IntentTrace, types.ComplexityModerate)
	rm.PredicateAxis = types.AxisCall
	rm.AnalyzerHints.Kind = "call_chain"
	rm.Predicates.IsCrossComponent = true
	rm.SubTopics = []types.SubTopic{{Summary: "path A"}, {Summary: "path B"}}

	out := compileT(rm)
	if countNodeType(out.TaskGraph, types.NodeReconcile) != 1 {
		t.Fatalf("cross-component trace should keep reconcile node")
	}
}

func TestCompile_UnknownScenarioFallsBackToGeneric(t *testing.T) {
	out := compileT(sampleRM("no_such_scenario", types.IntentExplain, types.ComplexityModerate))
	// Generic template has probe, evidence, optional source-inventory reprobe,
	// extract, finalize.
	if len(out.TaskGraph.Nodes) != 5 {
		t.Fatalf("unknown scenario should fall back to generic (5 nodes); got %d", len(out.TaskGraph.Nodes))
	}
}

func TestCompile_HintsPropagateFromTermGraph(t *testing.T) {
	out := compileT(sampleRM(types.ScenarioArchitectureExplain, types.IntentExplain, types.ComplexityModerate))
	foundCodeHint := false
	for _, n := range out.TaskGraph.Nodes {
		for _, id := range n.SearchHints.EntityIDs {
			if id == "code:explorer" {
				foundCodeHint = true
			}
		}
	}
	if !foundCodeHint {
		t.Fatalf("code:explorer must reach node search hints")
	}
}

func TestCompile_ComplexityScalesBudget(t *testing.T) {
	simple := compileT(sampleRM(types.ScenarioArchitectureExplain, types.IntentExplain, types.ComplexitySimple))
	moderate := compileT(sampleRM(types.ScenarioArchitectureExplain, types.IntentExplain, types.ComplexityModerate))
	complex := compileT(sampleRM(types.ScenarioArchitectureExplain, types.IntentExplain, types.ComplexityComplex))
	if !(simple.EvidencePlan.Budget.MaxFiles < moderate.EvidencePlan.Budget.MaxFiles &&
		moderate.EvidencePlan.Budget.MaxFiles < complex.EvidencePlan.Budget.MaxFiles) {
		t.Fatalf("budget not monotonic: simple=%d moderate=%d complex=%d",
			simple.EvidencePlan.Budget.MaxFiles, moderate.EvidencePlan.Budget.MaxFiles,
			complex.EvidencePlan.Budget.MaxFiles)
	}
}

func TestInferScenario(t *testing.T) {
	cases := []struct {
		name   string
		rm     types.RequestModel
		expect types.Scenario
	}{
		{"config intent", types.RequestModel{Intent: types.IntentConfigQuery}, types.ScenarioConfigTrace},
		{"root cause intent", types.RequestModel{Intent: types.IntentRootCause}, types.ScenarioRootCause},
		{
			"single-topic structural trace uses generic scenario",
			types.RequestModel{
				Intent:        types.IntentTrace,
				PredicateAxis: types.AxisCall,
				AnalyzerHints: types.AnalyzerHints{Kind: "call_chain"},
			},
			types.ScenarioGeneric,
		},
		{"explain default", types.RequestModel{Intent: types.IntentExplain}, types.ScenarioArchitectureExplain},
		{
			"pure history narrative uses generic scenario",
			types.RequestModel{
				Intent:   types.IntentExplain,
				Scenario: types.ScenarioGeneric,
				Predicates: types.SemanticPredicates{
					IsHistoryLookup: true,
				},
				AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqHistory)},
			},
			types.ScenarioGeneric,
		},
		{
			"failure-scope decision uses generic scenario",
			types.RequestModel{
				Intent: types.IntentExplain,
				ErrorGranularityProfile: &types.ErrorGranularityProfile{
					IsGranularityQuestion: true,
					Confidence:            0.9,
				},
				AnalyzerHints: types.AnalyzerHints{Entities: []string{"emit_evidence", "anchor_kind"}},
			},
			types.ScenarioGeneric,
		},
		{
			"root-cause-labeled failure-scope decision uses generic scenario",
			types.RequestModel{
				Intent: types.IntentRootCause,
				ErrorGranularityProfile: &types.ErrorGranularityProfile{
					IsGranularityQuestion: true,
					Confidence:            0.9,
				},
				AnalyzerHints: types.AnalyzerHints{Entities: []string{"emit_evidence", "anchor_kind"}},
			},
			types.ScenarioGeneric,
		},
		{
			"perf terms trigger perf scenario",
			types.RequestModel{Intent: types.IntentExplain, TermGraph: types.TermGraph{
				Canonical: []types.CanonicalTerm{{ID: "en:bottleneck"}},
			}},
			types.ScenarioPerformanceBottleneck,
		},
		{
			"zh perf term triggers perf scenario",
			types.RequestModel{Intent: types.IntentExplain, TermGraph: types.TermGraph{
				Canonical: []types.CanonicalTerm{{ID: "zh:性能"}},
			}},
			types.ScenarioPerformanceBottleneck,
		},
	}
	for _, c := range cases {
		if got := InferScenario(c.rm); got != c.expect {
			t.Errorf("%s: got %q want %q", c.name, got, c.expect)
		}
	}
}

func TestCompile_LanguagePropagatesToContract(t *testing.T) {
	rm := sampleRM(types.ScenarioArchitectureExplain, types.IntentExplain, types.ComplexityModerate)
	rm.Language = "en"
	out := compileT(rm)
	if out.AnswerContract.Language != "en" {
		t.Fatalf("language not propagated: %q", out.AnswerContract.Language)
	}
}

// Over-fit audit: each template must always produce a non-empty graph
// with at least one probe → evidence → finalize path and a
// fully-valid DAG (no dangling edges, no duplicate node IDs).
func TestCompile_AllTemplatesStructurallyValid(t *testing.T) {
	scenarios := []types.Scenario{
		types.ScenarioArchitectureExplain,
		types.ScenarioRootCause,
		types.ScenarioConfigTrace,
		types.ScenarioPerformanceBottleneck,
		types.ScenarioGeneric,
	}
	for _, sc := range scenarios {
		out := compileT(sampleRM(sc, types.IntentExplain, types.ComplexityModerate))
		if len(out.TaskGraph.Nodes) == 0 {
			t.Errorf("%s: empty node set", sc)
			continue
		}
		ids := make(map[string]bool)
		for _, n := range out.TaskGraph.Nodes {
			if ids[n.ID] {
				t.Errorf("%s: duplicate node id %q", sc, n.ID)
			}
			ids[n.ID] = true
		}
		for _, e := range out.TaskGraph.Edges {
			if !ids[e.From] || !ids[e.To] {
				t.Errorf("%s: dangling edge %+v", sc, e)
			}
		}
		if countNodeType(out.TaskGraph, types.NodeProbe) == 0 {
			t.Errorf("%s: missing probe node", sc)
		}
		if countNodeType(out.TaskGraph, types.NodeFinalize) == 0 {
			t.Errorf("%s: missing finalize node", sc)
		}
		if countNodeType(out.TaskGraph, types.NodeExtract) != 1 {
			t.Errorf("%s: missing singleton extract node", sc)
		}
		if out.EvidencePlan.Budget.MaxFiles == 0 {
			t.Errorf("%s: empty evidence budget", sc)
		}
	}
}
