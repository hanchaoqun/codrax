package types

import (
	"strings"
	"testing"
)

func TestExactResolutionSurfaceEvidencePool_IncludesAnswerChainEvidence(t *testing.T) {
	emitted := []EvidenceItem{{
		Kind:            EvidenceDirect,
		Source:          "internal/config/runtime.go",
		LineStart:       32,
		AnchorKind:      AnchorDefinition,
		AnchorSymbol:    "RuntimeSettings",
		ContextRole:     EvidenceContextRoleAbsenceSupport,
		GroundingStatus: GroundingGrounded,
	}}
	chains := []AnswerChain{{
		Item: EvidenceItem{
			Kind:            EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       707,
			AnchorKind:      AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			ContextRole:     EvidenceContextRoleRelatedContext,
			DiagramRole:     EvidenceDiagramRoleDefault,
			GroundingStatus: GroundingGrounded,
		},
	}}

	pool := ExactResolutionSurfaceEvidencePool(emitted, nil, chains)
	if len(pool) != 2 {
		t.Fatalf("pool len = %d, want 2", len(pool))
	}
	var sawDefault bool
	for _, item := range pool {
		if item.Source == "internal/types/config.go" && item.LineStart == 707 && item.DiagramRole == EvidenceDiagramRoleDefault {
			sawDefault = true
		}
	}
	if !sawDefault {
		t.Fatalf("pool missing answer-chain default anchor: %+v", pool)
	}
}

func TestCollectForbiddenExactContextLabels_SkipsImportSymbols(t *testing.T) {
	contract := &ExactResolutionContract{
		TargetKind:           SubjectConfigKey,
		TargetLabel:          "config key",
		Targets:              []string{"explore_mid_loop_hint_budget"},
		AllowAbsence:         true,
		RelatedContextPolicy: ExactContextSameFamilyGrounded,
		RelatedContextTerms:  []string{"explore"},
	}
	items := []EvidenceItem{{
		Kind:            EvidenceDirect,
		Source:          "internal/config/runtime.go",
		LineStart:       9,
		AnchorKind:      AnchorImport,
		AnchorSymbol:    "yaml",
		Subject:         "explore_mid_loop_hint_budget",
		ContextRole:     EvidenceContextRoleIllustrativeOnly,
		GroundingStatus: GroundingGrounded,
	}}

	labels := collectForbiddenExactContextLabels(contract, ScenarioConfigTrace, true, items, nil, nil)
	if len(labels) == 0 {
		t.Fatalf("expected at least the path label, got none")
	}
	for _, label := range labels {
		if strings.EqualFold(label.Display, "yaml") {
			t.Fatalf("import symbol yaml should not become a hard forbidden surface label: %+v", labels)
		}
	}
	var sawPath bool
	for _, label := range labels {
		if label.Kind == "path" && label.Display == "internal/config/runtime.go" {
			sawPath = true
		}
	}
	if !sawPath {
		t.Fatalf("expected forbidden labels to keep the path anchor, got %+v", labels)
	}
}

func TestCollectForbiddenExactContextLabels_IncludeStructuralSymbols(t *testing.T) {
	contract := &ExactResolutionContract{
		TargetKind:           SubjectConfigKey,
		TargetLabel:          "config key",
		Targets:              []string{"explore_mid_loop_hint_budget"},
		AllowAbsence:         true,
		RelatedContextPolicy: ExactContextSameFamilyGrounded,
	}
	items := []EvidenceItem{{
		Kind:            EvidenceDirect,
		Source:          "internal/types/explore_budget.go",
		LineStart:       40,
		AnchorKind:      AnchorDefinition,
		AnchorSymbol:    "ExploreBudget",
		ContextRole:     EvidenceContextRoleIllustrativeOnly,
		GroundingStatus: GroundingGrounded,
	}}

	labels := collectForbiddenExactContextLabels(contract, ScenarioConfigTrace, true, items, []string{"internal/types/config.go"}, nil)
	var sawSymbol, sawPath bool
	for _, label := range labels {
		if label.Kind == "symbol" && label.Display == "ExploreBudget" {
			sawSymbol = true
		}
		if label.Kind == "path" && label.Display == "internal/types/explore_budget.go" {
			sawPath = true
		}
	}
	if !sawSymbol || !sawPath {
		t.Fatalf("forbidden labels should retain structural background-only symbol + path, got %+v", labels)
	}
}

func TestBuildAnswerSurfacePlan_SplitsCitationGradeAndProseOnlyContext(t *testing.T) {
	mut := NewMutableState("")
	mut.SetInvestigationResultKind("absence")
	mut.SetAbsenceJustification("repo-wide search found no exact key")
	mut.SetExactContextRequiredFiles([]string{"internal/types/config.go"})
	mut.AppendEvidence([]EvidenceItem{
		{
			Kind:            EvidenceDirect,
			Source:          "internal/config/runtime.go",
			LineStart:       32,
			AnchorKind:      AnchorDefinition,
			AnchorSymbol:    "RuntimeSettings",
			ContextRole:     EvidenceContextRoleAbsenceSupport,
			GroundingStatus: GroundingGrounded,
		},
		{
			Kind:            EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       707,
			AnchorKind:      AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			ContextRole:     EvidenceContextRoleRelatedContext,
			DiagramRole:     EvidenceDiagramRoleDefault,
			GroundingStatus: GroundingGrounded,
		},
		{
			Kind:            EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       627,
			AnchorKind:      AnchorDefinition,
			AnchorSymbol:    "ExploreHeuristics",
			ContextRole:     EvidenceContextRoleRelatedContext,
			GroundingStatus: GroundingGrounded,
		},
	})
	ir := &AnalysisIR{
		RequestModel: RequestModel{
			Scenario: ScenarioConfigTrace,
			AnswerSubject: AnswerSubject{
				Kind: SubjectConfigKey,
			},
		},
		AnswerContract: AnswerContract{
			RequiredAnswerShape: ShapeExplanation,
			ExactResolution: &ExactResolutionContract{
				TargetKind:           SubjectConfigKey,
				TargetLabel:          "config key",
				Targets:              []string{"explore_mid_loop_hint_budget"},
				AllowAbsence:         true,
				RelatedContextPolicy: ExactContextSameFamilyGrounded,
				RelatedContextTerms:  []string{"explore"},
			},
		},
	}

	plan := BuildAnswerSurfacePlan(ir, mut, nil, nil, nil, nil)
	if plan == nil {
		t.Fatalf("BuildAnswerSurfacePlan returned nil")
	}
	if len(plan.CitationGradeExactContextItems) != 2 {
		t.Fatalf("citation-grade items = %d, want 2", len(plan.CitationGradeExactContextItems))
	}
	if len(plan.ProseOnlyExactContextItems) != 1 {
		t.Fatalf("prose-only items = %d, want 1", len(plan.ProseOnlyExactContextItems))
	}
	if plan.ProseOnlyExactContextItems[0].AnchorSymbol != "ExploreHeuristics" {
		t.Fatalf("prose-only anchor = %+v, want ExploreHeuristics", plan.ProseOnlyExactContextItems[0])
	}
	var sawRuntime, sawDefault bool
	for _, item := range plan.CitationGradeExactContextItems {
		switch item.AnchorSymbol {
		case "RuntimeSettings":
			sawRuntime = true
		case "DefaultExploreHeuristics":
			sawDefault = true
		}
	}
	if !sawRuntime || !sawDefault {
		t.Fatalf("citation-grade anchors missing runtime/default split: %+v", plan.CitationGradeExactContextItems)
	}
	if plan.PreferredExactResolution == nil {
		t.Fatalf("preferred exact resolution should be compiled for stable absence")
	}
	if plan.PreferredExactResolution.Status != AnswerExactResolutionAbsent {
		t.Fatalf("preferred status = %s, want %s", plan.PreferredExactResolution.Status, AnswerExactResolutionAbsent)
	}
	if plan.PreferredExactResolution.ContextMode != AnswerExactResolutionContextGroundedOnly {
		t.Fatalf("preferred context mode = %s, want %s", plan.PreferredExactResolution.ContextMode, AnswerExactResolutionContextGroundedOnly)
	}
	if plan.SummarySurfaceMode != AnswerSummarySurfaceFollowOnGroundedContext {
		t.Fatalf("summary surface mode = %s, want %s", plan.SummarySurfaceMode, AnswerSummarySurfaceFollowOnGroundedContext)
	}
}

func TestBuildAnswerSurfacePlan_CompilesLogDiagramFence(t *testing.T) {
	mut := NewMutableState("")
	logBundle := &LogBundle{
		Errors: []LogError{{
			Frames: []LogFrame{
				{File: "internal/agent/analyzer.go", Line: 250, Func: "buildAnalysisIR"},
				{File: "internal/agent/analyzer.go", Line: 412, Func: "(*analyzerEvaluator).ParseOutput"},
			},
		}},
	}
	ir := &AnalysisIR{
		RequestModel: RequestModel{
			Scenario: ScenarioRootCause,
		},
		AnswerContract: AnswerContract{
			RequiredAnswerShape: ShapeExplanation,
			Diagram: &DiagramContract{
				Required:       true,
				Minimum:        1,
				PreferredKinds: []DiagramKind{DiagramCallDAG, DiagramSequence},
			},
		},
	}

	plan := BuildAnswerSurfacePlan(ir, mut, logBundle, nil, nil, nil)
	if plan == nil {
		t.Fatalf("BuildAnswerSurfacePlan returned nil")
	}
	if plan.CompiledDiagramKind != DiagramCallDAG {
		t.Fatalf("compiled kind = %s, want %s", plan.CompiledDiagramKind, DiagramCallDAG)
	}
	if !strings.Contains(plan.CompiledDiagramFence, "internal/agent/analyzer.go:250") ||
		!strings.Contains(plan.CompiledDiagramFence, "internal/agent/analyzer.go:412") {
		t.Fatalf("compiled fence missing resolved log frames: %q", plan.CompiledDiagramFence)
	}
}

func TestBuildAnswerSurfacePlan_UsesMinimalSummaryModeForRoleLocateScalar(t *testing.T) {
	ir := &AnalysisIR{
		RequestModel: RequestModel{
			Scenario:      ScenarioGeneric,
			AnswerSubject: AnswerSubject{Kind: SubjectFunctionName},
			AnalyzerHints: AnalyzerHints{
				Kind: "return_value",
			},
			PredicateAxis: AxisReturn,
			Predicates: SemanticPredicates{
				IsScalarAnswer: true,
			},
		},
		AnswerContract: AnswerContract{
			RequiredAnswerShape: ShapeValue,
		},
	}

	plan := BuildAnswerSurfacePlan(ir, NewMutableState(""), nil, nil, nil, nil)
	if plan == nil {
		t.Fatalf("BuildAnswerSurfacePlan returned nil")
	}
	if plan.SummarySurfaceMode != AnswerSummarySurfaceMinimalScalarRoleLocate {
		t.Fatalf("summary surface mode = %s, want %s", plan.SummarySurfaceMode, AnswerSummarySurfaceMinimalScalarRoleLocate)
	}
}

func TestBuildAnswerSurfacePlan_CollectsLogSourceDriftAnchors(t *testing.T) {
	mut := NewMutableState("")
	logBundle := &LogBundle{
		Errors: []LogError{{
			Frames: []LogFrame{
				{File: "internal/agent/analyzer.go", Line: 250, Func: "buildAnalysisIR"},
				{File: "internal/agent/analyzer.go", Line: 320, Func: "ParseOutput"},
			},
		}},
	}
	ir := &AnalysisIR{
		RequestModel: RequestModel{
			Scenario: ScenarioRootCause,
			Intent:   IntentRootCause,
		},
		AnswerContract: AnswerContract{
			RequiredAnswerShape: ShapeExplanation,
		},
	}
	evidence := []EvidenceItem{
		{
			Kind:            EvidenceDirect,
			Source:          "internal/agent/analyzer.go",
			LineStart:       612,
			AnchorKind:      AnchorDefinition,
			AnchorSymbol:    "buildAnalysisIR",
			GroundingStatus: GroundingGrounded,
		},
		{
			Kind:            EvidenceDirect,
			Source:          "internal/agent/analyzer.go",
			LineStart:       367,
			AnchorKind:      AnchorDefinition,
			AnchorSymbol:    "ParseOutput",
			GroundingStatus: GroundingGrounded,
		},
	}

	plan := BuildAnswerSurfacePlan(ir, mut, logBundle, nil, nil, evidence)
	if plan == nil {
		t.Fatal("BuildAnswerSurfacePlan returned nil")
	}
	if len(plan.LogSourceDriftAnchors) != 1 {
		t.Fatalf("log source drift anchors = %d, want 1", len(plan.LogSourceDriftAnchors))
	}
	if got := plan.LogSourceDriftAnchors[0]; got.File != "internal/agent/analyzer.go" || got.ObservedLine != 250 || got.AnchoredLine != 612 {
		t.Fatalf("first drift anchor = %+v, want analyzer.go 250 -> 612", got)
	}
}

func TestEvidencePreferredSurfaceText_NeutralizesExactResolutionRelevantSummary(t *testing.T) {
	contract := &ExactResolutionContract{
		TargetKind:           SubjectConfigKey,
		TargetLabel:          "config key",
		Targets:              []string{"explore_mid_loop_hint_budget"},
		AllowAbsence:         true,
		RelatedContextPolicy: ExactContextSameFamilyGrounded,
		RelatedContextTerms:  []string{"explore"},
	}
	item := EvidenceItem{
		Kind:            EvidenceMechanism,
		Subject:         "DefaultExploreHeuristics",
		Predicate:       "explains",
		Object:          "nearby precedence baseline",
		Summary:         "This item names explore_mid_loop_hint_budget only in explanatory context; do NOT repair this item.",
		Source:          "internal/types/config.go",
		LineStart:       707,
		AnchorKind:      AnchorDefinition,
		AnchorSymbol:    "DefaultExploreHeuristics",
		ContextRole:     EvidenceContextRoleRelatedContext,
		GroundingStatus: GroundingGrounded,
	}

	got := EvidencePreferredSurfaceText(item, contract, true)
	if strings.Contains(got, "explore_mid_loop_hint_budget") {
		t.Fatalf("preferred surface leaked exact target prose: %q", got)
	}
	if strings.Contains(strings.ToLower(got), "do not repair") {
		t.Fatalf("preferred surface leaked operational note: %q", got)
	}
	if !strings.Contains(got, "DefaultExploreHeuristics explains nearby precedence baseline") {
		t.Fatalf("preferred surface lost structural claim: %q", got)
	}
}
