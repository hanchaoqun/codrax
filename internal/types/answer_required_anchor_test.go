package types

import "testing"

// TestCompileRequiredMechanismAnchors_FiltersProjectNames is the F1
// regression for the 2026-05-16 architectural-comparison finalize
// loop. The analyzer's ExactTargets include "codrax" /
// "opencode" — repository names that look identifier-shaped to
// InferContractTermKind but have no code definition site. Without
// the sub-repo-name filter, CompileRequiredMechanismAnchors promotes
// them into RequiredMechanismAnchor; the pre-emit normaliser then
// auto-injects them as item labels in `required_mechanism_anchors`,
// and the post-emit `enumeration_label_ungrounded` oracle rejects
// the auto-injected labels because they match no evidence anchor or
// QuestionBucket. The result is a same-error-class retry loop that
// burns the finalizer budget and falls back to raw content with
// cited=0.
func TestCompileRequiredMechanismAnchors_FiltersProjectNames(t *testing.T) {
	rm := RequestModel{
		RawRequest: "对比一下 codrax 和 opencode 在防止模型幻觉方面各有什么优缺点？",
		Intent:     IntentExplain,
		Scenario:   ScenarioArchitectureExplain,
		Predicates: SemanticPredicates{IsCrossComponent: true},
		AnalyzerHints: AnalyzerHints{
			ExactTargets: []string{"codrax", "opencode", "Orchestrator"},
		},
	}
	contract := AnswerContract{}

	// Without sub-repo filter (single-repo posture / no MultiGraph
	// wired): legacy behaviour — entities flow through.
	got := CompileRequiredMechanismAnchors(rm, contract, QFArchitecture, nil)
	hasCodrax := false
	hasOpencode := false
	hasOrch := false
	for _, a := range got {
		switch a.Text {
		case "codrax":
			hasCodrax = true
		case "opencode":
			hasOpencode = true
		case "Orchestrator":
			hasOrch = true
		}
	}
	if !hasCodrax || !hasOpencode {
		t.Fatalf("baseline (no sub-repo filter) should keep project names: %+v", got)
	}
	if !hasOrch {
		t.Fatalf("baseline should include real symbol Orchestrator: %+v", got)
	}

	// With sub-repo filter (multi-repo posture, codrax + opencode
	// active in topology): project names dropped, real symbol kept.
	got = CompileRequiredMechanismAnchors(rm, contract, QFArchitecture,
		[]string{"codrax", "opencode", "claude/codrax", "small/codrax-small"})
	for _, a := range got {
		if a.Text == "codrax" || a.Text == "opencode" {
			t.Errorf("project-name %q must be filtered out, got %+v", a.Text, got)
		}
	}
	foundOrch := false
	for _, a := range got {
		if a.Text == "Orchestrator" {
			foundOrch = true
		}
	}
	if !foundOrch {
		t.Errorf("real symbol Orchestrator must survive filter: %+v", got)
	}
}

func TestCompileRequiredMechanismAnchors_FilterIsCaseInsensitive(t *testing.T) {
	rm := RequestModel{
		RawRequest: "Codrax vs opencode",
		Intent:     IntentExplain,
		Scenario:   ScenarioArchitectureExplain,
		Predicates: SemanticPredicates{IsCrossComponent: true},
		AnalyzerHints: AnalyzerHints{
			// Mixed case to verify requiredAnchorKey's ToLower normalises.
			ExactTargets: []string{"Codrax", "OpenCode"},
		},
	}
	got := CompileRequiredMechanismAnchors(rm, AnswerContract{}, QFArchitecture,
		[]string{"codrax", "opencode"})
	if len(got) != 0 {
		t.Errorf("case variants of project names must be filtered, got %+v", got)
	}
}

func TestCompileRequiredMechanismAnchors_DisabledForRuntimeArtifactWithoutRequiredSource(t *testing.T) {
	rm := RequestModel{
		Intent:   IntentExplain,
		Scenario: ScenarioPerformanceBottleneck,
		PerfTrace: &PerfBundle{
			Observations: []PerfObservation{{
				Kind:    "state_churn",
				Subject: "app-20",
				Summary: "trace_query window_stats returned state_churn metrics",
			}},
		},
		AnalyzerHints: AnalyzerHints{
			MentionedEntities: []string{"app-20", "window_stats", "dominant_state", "state_churn"},
			ExactTargets:      []string{"app-20"},
		},
	}
	got := CompileRequiredMechanismAnchors(rm, AnswerContract{}, QFGeneric, nil)
	if len(got) != 0 {
		t.Fatalf("source-optional runtime artifact dimensions must not become current-source mechanism anchors: %+v", got)
	}
}

func TestCompileRequiredMechanismAnchors_SoftRuntimeSourceDoesNotCreateHardAnchor(t *testing.T) {
	rm := runtimeSourceExactContractFixture("current implementation")
	if precision := RuntimeSourceRequestCurrentSourceRequirementPrecision(&rm, TurnRouteHint{}); precision != RuntimeSourceRequirementSoft {
		t.Fatalf("fixture should be soft runtime/source, got %s", precision)
	}
	got := CompileRequiredMechanismAnchors(rm, AnswerContract{}, QFGeneric, nil)
	if len(got) != 0 {
		t.Fatalf("soft runtime/source current-source lane must not create required mechanism anchors: %+v", got)
	}
}

func TestCompileRequiredMechanismAnchors_PreciseRuntimeSourceKeepsHardAnchor(t *testing.T) {
	rm := runtimeSourceExactContractFixture("internal/tracequery/query.go:42")
	if precision := RuntimeSourceRequestCurrentSourceRequirementPrecision(&rm, TurnRouteHint{}); precision != RuntimeSourceRequirementPrecise {
		t.Fatalf("fixture should be precise runtime/source, got %s", precision)
	}
	got := CompileRequiredMechanismAnchors(rm, AnswerContract{}, QFGeneric, nil)
	if len(got) != 1 || got[0].Text != "TraceQueryPlanner" {
		t.Fatalf("precise runtime/source current-source lane should keep required mechanism anchor, got %+v", got)
	}
}

func TestCompileRequiredMechanismAnchors_RuntimeCurrentSourceRequiredKeepsAnchors(t *testing.T) {
	rm := RequestModel{
		Intent:   IntentExplain,
		Scenario: ScenarioPerformanceBottleneck,
		PerfTrace: &PerfBundle{
			Observations: []PerfObservation{{
				Kind:    "state_churn",
				Subject: "app-20",
				Summary: "trace_query window_stats returned state_churn metrics",
			}},
		},
		AnalyzerHints: AnalyzerHints{
			MentionedEntities: []string{"TraceQueryPlanner"},
			ExactTargets:      []string{"TraceQueryPlanner"},
			RequiredFileHints: []RequiredFileHint{{
				Path:       "internal/tracequery/query.go",
				Confidence: 0.9,
			}},
		},
	}
	got := CompileRequiredMechanismAnchors(rm, AnswerContract{}, QFGeneric, nil)
	if len(got) != 1 || got[0].Text != "TraceQueryPlanner" {
		t.Fatalf("runtime questions with a current-source lane should keep source anchors, got %+v", got)
	}
}

func TestCompileRequiredMechanismAnchors_CallChainKeepsEndpointsAcrossRelationFlags(t *testing.T) {
	rm := RequestModel{
		Intent:        IntentTrace,
		Scenario:      ScenarioArchitectureExplain,
		PredicateAxis: AxisCall,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
			IsRelationalLookup:    true,
		},
		AnalyzerHints: AnalyzerHints{
			Kind:              string(ReqCallChain),
			MentionedEntities: []string{"buildAnalysisIR", "gate.Run", "analyzer.go", "kind"},
		},
		CallChainEndpointProfile: &CallChainEndpointProfile{Source: "buildAnalysisIR", Sink: "gate.Run"},
	}
	got := CompileRequiredMechanismAnchors(rm, AnswerContract{}, QFCallChain, nil)
	if len(got) != 2 {
		t.Fatalf("call-chain endpoints must survive relation/category flags, got %+v", got)
	}
	for i, want := range []string{"buildAnalysisIR", "gate.Run"} {
		if got[i].Text != want {
			t.Fatalf("anchor[%d]=%+v, want %q", i, got[i], want)
		}
	}
}

func TestCompileRequiredMechanismAnchors_CallChainFiltersPathContextNotQualifiedSymbols(t *testing.T) {
	rm := RequestModel{
		Intent:        IntentTrace,
		PredicateAxis: AxisCall,
		AnalyzerHints: AnalyzerHints{
			Kind: string(ReqCallChain),
			MentionedEntities: []string{
				"internal/agent/analyzer.go",
				"config.yaml",
				"gate.Run",
				"StageOutput.AnalysisIR",
			},
		},
		CallChainEndpointProfile: &CallChainEndpointProfile{Source: "gate.Run", Sink: "StageOutput.AnalysisIR"},
	}
	got := CompileRequiredMechanismAnchors(rm, AnswerContract{}, QFCallChain, nil)
	if len(got) != 2 || got[0].Text != "gate.Run" || got[1].Text != "StageOutput.AnalysisIR" {
		t.Fatalf("call-chain path context must not become endpoints; qualified symbols must survive: %+v", got)
	}
}

func TestCompileRequiredMechanismAnchors_CallChainDiscoverUsesOnlyPreciseSource(t *testing.T) {
	rm := RequestModel{
		Intent:        IntentTrace,
		PredicateAxis: AxisCall,
		AnalyzerHints: AnalyzerHints{
			Kind:              string(ReqCallChain),
			MentionedEntities: []string{"run_pipeline", "register", "kind", "json"},
		},
		CallChainEndpointProfile: &CallChainEndpointProfile{
			Source:   "run_pipeline",
			SinkMode: CallChainSinkResolutionDiscover,
		},
	}
	got := CompileRequiredMechanismAnchors(rm, AnswerContract{}, QFCallChain, nil)
	if len(got) != 1 || got[0].Text != "run_pipeline" {
		t.Fatalf("discover-mode hard anchors must contain only the precise source; noisy mentions cannot be promoted: %+v", got)
	}
}

func TestCompileRequiredMechanismAnchors_GenericMentionedEntitiesStaySoft(t *testing.T) {
	rm := RequestModel{
		Intent:   IntentExplain,
		Scenario: ScenarioArchitectureExplain,
		AnalyzerHints: AnalyzerHints{
			MentionedEntities: []string{"kind", "json", "helperCandidate"},
			ExactTargets:      []string{"runTaskGraph"},
		},
	}
	got := CompileRequiredMechanismAnchors(rm, AnswerContract{}, QFArchitecture, nil)
	if len(got) != 1 || got[0].Text != "runTaskGraph" {
		t.Fatalf("only exact typed targets may become generic hard anchors: %+v", got)
	}
}
