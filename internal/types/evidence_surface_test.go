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

func TestExactResolutionSurfaceEvidencePool_PrefersMoreRestrictiveContextRole(t *testing.T) {
	base := []EvidenceItem{{
		Kind:            EvidenceDirect,
		Source:          "internal/types/config.go",
		LineStart:       870,
		AnchorKind:      AnchorDefinition,
		AnchorSymbol:    "ResolvedExploreHeuristics",
		ContextRole:     EvidenceContextRoleDefining,
		GroundingStatus: GroundingGrounded,
	}}
	refined := []EvidenceItem{{
		Kind:            EvidenceDirect,
		Source:          "internal/types/config.go",
		LineStart:       870,
		AnchorKind:      AnchorDefinition,
		AnchorSymbol:    "ResolvedExploreHeuristics",
		ContextRole:     EvidenceContextRoleRelatedContext,
		GroundingStatus: GroundingGrounded,
	}}

	pool := ExactResolutionSurfaceEvidencePool(base, refined, nil)
	if len(pool) != 1 {
		t.Fatalf("pool len = %d, want 1", len(pool))
	}
	if got := pool[0].ContextRole; got != EvidenceContextRoleRelatedContext {
		t.Fatalf("merged ContextRole = %q, want related_context", got)
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

func TestBuildAnswerSurfacePlan_UsesMinimalSummaryModeForUnnamedRoleLocateFallback(t *testing.T) {
	ir := &AnalysisIR{
		RequestModel: RequestModel{
			Intent:        IntentExplain,
			Complexity:    ComplexitySimple,
			Scenario:      ScenarioGeneric,
			AnswerSubject: AnswerSubject{Kind: SubjectFunctionName},
			AnalyzerHints: AnalyzerHints{
				Kind: "mechanism",
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

func TestBuildAnswerSurfacePlan_ConfigTraceCompiledFenceUsesRoleLabels(t *testing.T) {
	mut := NewMutableState("")
	ir := &AnalysisIR{
		RequestModel: RequestModel{
			Scenario: ScenarioConfigTrace,
		},
		AnswerContract: AnswerContract{
			RequiredAnswerShape: ShapeExplanation,
			Diagram: &DiagramContract{
				Required:       true,
				PreferredKinds: []DiagramKind{DiagramFlow},
			},
		},
	}
	evidence := []EvidenceItem{
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
			Source:          "internal/config/runtime.go",
			LineStart:       231,
			AnchorKind:      AnchorAssignment,
			AnchorSymbol:    "ExploreMidLoopMinIteration",
			ContextRole:     EvidenceContextRoleRelatedContext,
			DiagramRole:     EvidenceDiagramRoleRuntime,
			GroundingStatus: GroundingGrounded,
		},
	}

	plan := BuildAnswerSurfacePlan(ir, mut, nil, nil, nil, evidence)
	if plan == nil {
		t.Fatal("BuildAnswerSurfacePlan returned nil")
	}
	if got := plan.CompiledDiagramFence; !strings.Contains(got, "runtime binding") || !strings.Contains(got, "code default") {
		t.Fatalf("compiled config-trace fence should use role labels, got: %q", got)
	}
	if strings.Contains(plan.CompiledDiagramFence, "internal/config/runtime.go:231") || strings.Contains(plan.CompiledDiagramFence, "internal/types/config.go:707") {
		t.Fatalf("compiled config-trace fence should keep source anchors outside node labels, got: %q", plan.CompiledDiagramFence)
	}
	if len(plan.ConfigTraceDiagramAnchors) != 2 {
		t.Fatalf("config-trace anchors = %d, want 2", len(plan.ConfigTraceDiagramAnchors))
	}
	if got := ConfigTraceDiagramAnchorSupportLabel(plan.ConfigTraceDiagramAnchors[0]); got == "" {
		t.Fatal("expected config-trace anchor to preserve support location")
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

func TestBuildAnswerSurfacePlan_LogObservedAnchorsPreferAuthoritativeBindings(t *testing.T) {
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
			LineStart:       860,
			AnchorKind:      AnchorDefinition,
			AnchorSymbol:    "buildAnalysisIR",
			GroundingStatus: GroundingGrounded,
		},
		{
			Kind:            EvidenceDirect,
			Source:          "internal/agent/analyzer.go",
			LineStart:       651,
			AnchorKind:      AnchorCall,
			AnchorSymbol:    "buildAnalysisIR",
			Subject:         "analyzerEvaluator.ParseOutput",
			Object:          "buildAnalysisIR",
			GroundingStatus: GroundingGrounded,
		},
		{
			Kind:            EvidenceDirect,
			Source:          "internal/agent/analyzer.go",
			LineStart:       233,
			AnchorKind:      AnchorDefinition,
			AnchorSymbol:    "buildAnalyzerRepoOverview",
			GroundingStatus: GroundingGrounded,
		},
		{
			Kind:            EvidenceConditional,
			Source:          "internal/agent/analyzer.go",
			LineStart:       243,
			AnchorKind:      AnchorCondition,
			AnchorSymbol:    "buildAnalyzerRepoOverview",
			Subject:         "buildAnalyzerRepoOverview",
			Object:          "repoRoot",
			GroundingStatus: GroundingGrounded,
		},
	}

	plan := BuildAnswerSurfacePlan(ir, mut, logBundle, nil, nil, evidence)
	if plan == nil {
		t.Fatal("BuildAnswerSurfacePlan returned nil")
	}
	if len(plan.LogObservedAnchors) != 2 {
		t.Fatalf("log observed anchors = %d, want 2", len(plan.LogObservedAnchors))
	}
	gotLines := make(map[int]string, len(plan.LogObservedAnchors))
	for _, anchor := range plan.LogObservedAnchors {
		gotLines[anchor.AnchoredLine] = anchor.Func
	}
	if gotLines[860] != "buildAnalysisIR" {
		t.Fatalf("authoritative callee anchor missing, got %+v", plan.LogObservedAnchors)
	}
	if gotLines[651] != "ParseOutput" {
		t.Fatalf("authoritative caller anchor should recover from relationship evidence, got %+v", plan.LogObservedAnchors)
	}
	if _, ok := gotLines[233]; ok {
		t.Fatalf("same-file helper definition must not become a log authoritative anchor, got %+v", plan.LogObservedAnchors)
	}
	if _, ok := gotLines[243]; ok {
		t.Fatalf("same-file helper condition must not become a log authoritative anchor, got %+v", plan.LogObservedAnchors)
	}
}

func TestBuildAnswerSurfacePlan_CompilesDiagramFenceFromObservedAnchorsWhenAvailable(t *testing.T) {
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
			Diagram: &DiagramContract{
				Required:       true,
				PreferredKinds: []DiagramKind{DiagramCallDAG},
			},
		},
	}
	evidence := []EvidenceItem{
		{
			Kind:            EvidenceDirect,
			Source:          "internal/agent/analyzer.go",
			LineStart:       860,
			AnchorKind:      AnchorDefinition,
			AnchorSymbol:    "buildAnalysisIR",
			GroundingStatus: GroundingGrounded,
		},
		{
			Kind:            EvidenceRelationship,
			Source:          "internal/agent/analyzer.go",
			LineStart:       651,
			AnchorKind:      AnchorCall,
			AnchorSymbol:    "buildAnalysisIR",
			Subject:         "ParseOutput",
			Object:          "buildAnalysisIR",
			GroundingStatus: GroundingGrounded,
		},
	}

	plan := BuildAnswerSurfacePlan(ir, mut, logBundle, nil, nil, evidence)
	if plan == nil {
		t.Fatal("BuildAnswerSurfacePlan returned nil")
	}
	if !strings.Contains(plan.CompiledDiagramFence, "internal/agent/analyzer.go:860") ||
		!strings.Contains(plan.CompiledDiagramFence, "internal/agent/analyzer.go:651") {
		t.Fatalf("compiled fence should prefer observed anchored lines over raw log frames, got: %q", plan.CompiledDiagramFence)
	}
	if strings.Contains(plan.CompiledDiagramFence, "internal/agent/analyzer.go:250") ||
		strings.Contains(plan.CompiledDiagramFence, "internal/agent/analyzer.go:320") {
		t.Fatalf("compiled fence should not fall back to raw drifted frame lines when observed anchors exist, got: %q", plan.CompiledDiagramFence)
	}
}

func TestBuildAnswerSurfacePlan_UsesDriftBoundedRootCauseModeForRootCauseLogs(t *testing.T) {
	mut := NewMutableState("")
	logBundle := &LogBundle{
		Errors: []LogError{{
			Frames: []LogFrame{
				{File: "internal/agent/analyzer.go", Line: 250, Func: "buildAnalysisIR"},
				{File: "internal/agent/analyzer.go", Line: 320, Func: "ParseOutput"},
			},
		}},
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

	for _, shape := range []AnswerShape{ShapeStepList, ShapeExplanation} {
		ir := &AnalysisIR{
			RequestModel: RequestModel{
				Scenario: ScenarioRootCause,
				Intent:   IntentRootCause,
			},
			AnswerContract: AnswerContract{
				RequiredAnswerShape: shape,
			},
		}
		plan := BuildAnswerSurfacePlan(ir, mut, logBundle, nil, nil, evidence)
		if plan == nil {
			t.Fatalf("BuildAnswerSurfacePlan returned nil for shape=%s", shape)
		}
		if plan.SummarySurfaceMode != AnswerSummarySurfaceDriftBoundedRootCause {
			t.Fatalf("shape=%s summary surface mode = %s, want %s", shape, plan.SummarySurfaceMode, AnswerSummarySurfaceDriftBoundedRootCause)
		}
	}
}

func TestBuildAnswerSurfacePlan_CollectsDriftBoundedSurfaceItems(t *testing.T) {
	mut := NewMutableState("")
	logBundle := &LogBundle{
		Errors: []LogError{{
			Frames: []LogFrame{
				{File: "internal/agent/analyzer.go", Line: 250, Func: "buildAnalysisIR"},
				{File: "internal/agent/analyzer.go", Line: 320, Func: "(*analyzerEvaluator).ParseOutput"},
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
			Kind:            EvidenceRelationship,
			Source:          "internal/agent/analyzer.go",
			LineStart:       651,
			AnchorKind:      AnchorCall,
			AnchorSymbol:    "buildAnalysisIR",
			Subject:         "ParseOutput",
			Object:          "buildAnalysisIR",
			GroundingStatus: GroundingGrounded,
		},
		{
			Kind:            EvidenceConditional,
			Source:          "internal/agent/analyzer.go",
			LineStart:       861,
			AnchorKind:      AnchorCondition,
			AnchorSymbol:    "buildAnalysisIR",
			Condition:       "ctx == nil || ctx.Mutable == nil",
			GroundingStatus: GroundingGrounded,
		},
		{
			Kind:            EvidenceDirect,
			Source:          "internal/agent/analyzer.go",
			LineStart:       860,
			AnchorKind:      AnchorDefinition,
			AnchorSymbol:    "buildAnalysisIR",
			GroundingStatus: GroundingGrounded,
		},
		{
			Kind:            EvidenceDirect,
			Source:          "internal/agent/analyzer.go",
			LineStart:       233,
			AnchorKind:      AnchorDefinition,
			AnchorSymbol:    "buildAnalyzerRepoOverview",
			GroundingStatus: GroundingGrounded,
		},
		{
			Kind:            EvidenceRelationship,
			Source:          "internal/agent/analyzer.go",
			LineStart:       892,
			AnchorKind:      AnchorCall,
			AnchorSymbol:    "analyzerGraphForNormalize",
			Subject:         "buildAnalysisIR",
			Object:          "analyzerGraphForNormalize",
			GroundingStatus: GroundingGrounded,
		},
	}

	plan := BuildAnswerSurfacePlan(ir, mut, logBundle, nil, nil, evidence)
	if plan == nil {
		t.Fatal("BuildAnswerSurfacePlan returned nil")
	}
	if len(plan.DriftBoundedSurfaceItems) < 2 {
		t.Fatalf("drift-bounded surface items = %d, want at least 2", len(plan.DriftBoundedSurfaceItems))
	}
	if got := plan.DriftBoundedSurfaceItems[0].LineStart; got != 651 {
		t.Fatalf("first drift-bounded surface item line = %d, want call edge at 651", got)
	}
	if got := plan.DriftBoundedSurfaceItems[1].LineStart; got != 861 {
		t.Fatalf("second drift-bounded surface item line = %d, want guard anchor at 861", got)
	}
	for _, item := range plan.DriftBoundedSurfaceItems {
		if item.LineStart == 233 || item.LineStart == 892 {
			t.Fatalf("drift-bounded surface must not elevate helper/speculative anchors, got %+v", plan.DriftBoundedSurfaceItems)
		}
	}
}

func TestBuildAnswerSurfacePlan_CompilesStepBackboneFromAnswerSymbols(t *testing.T) {
	mut := NewMutableState("")
	mut.SetEmittedAnswerSymbols([]AnswerSymbol{
		{
			Name:      "RequestModel",
			File:      "internal/agent/analyzer.go",
			Line:      616,
			Kind:      KindMethod,
			Rationale: "在 buildAnalysisIR 内部获取 LLM 输出的 RequestModel，是后续步骤的输入基础",
		},
		{
			Name:      "gate.Run",
			File:      "internal/agent/analyzer.go",
			Line:      1062,
			Kind:      KindFunction,
			Rationale: "执行质量门检查，生成最终 gate 结果",
		},
	}, CompletenessLowerBound)
	ir := &AnalysisIR{
		AnswerContract: AnswerContract{
			RequiredAnswerShape: ShapeStepList,
		},
	}
	plan := BuildAnswerSurfacePlan(ir, mut, nil, nil, nil, nil)
	if plan == nil {
		t.Fatal("BuildAnswerSurfacePlan returned nil")
	}
	if plan.StepBackboneCompleteness != CompletenessLowerBound {
		t.Fatalf("step backbone completeness = %q, want %q", plan.StepBackboneCompleteness, CompletenessLowerBound)
	}
	if len(plan.StepBackbone) != 2 {
		t.Fatalf("step backbone anchors = %d, want 2", len(plan.StepBackbone))
	}
	if got := plan.StepBackbone[0]; got.Name != "RequestModel" || got.File != "internal/agent/analyzer.go" || got.Line != 616 {
		t.Fatalf("first step backbone anchor = %+v", got)
	}
}

func TestBuildAnswerSurfacePlan_CompilesFallbackStepBackboneFromEvidence(t *testing.T) {
	ir := &AnalysisIR{
		AnswerContract: AnswerContract{
			RequiredAnswerShape: ShapeStepList,
		},
	}
	evidence := []EvidenceItem{
		{
			Kind:            EvidenceDirect,
			Source:          "internal/analysis/gate/gate.go",
			LineStart:       127,
			AnchorKind:      AnchorCall,
			AnchorSymbol:    "checkCoverage",
			Summary:         "checkCoverage is appended as the first gate check",
			GroundingStatus: GroundingGrounded,
		},
		{
			Kind:            EvidenceDirect,
			Source:          "internal/analysis/gate/gate.go",
			LineStart:       128,
			AnchorKind:      AnchorCall,
			AnchorSymbol:    "checkDAGClosure",
			Summary:         "checkDAGClosure is appended next",
			GroundingStatus: GroundingGrounded,
		},
		{
			Kind:            EvidenceDirect,
			Source:          "internal/analysis/gate/gate.go",
			LineStart:       129,
			AnchorKind:      AnchorCall,
			AnchorSymbol:    "checkBudgetSanity",
			Summary:         "checkBudgetSanity is appended after DAG closure",
			GroundingStatus: GroundingGrounded,
		},
	}
	plan := BuildAnswerSurfacePlan(ir, NewMutableState(""), nil, nil, nil, evidence)
	if plan == nil {
		t.Fatal("BuildAnswerSurfacePlan returned nil")
	}
	if len(plan.StepBackbone) != 3 {
		t.Fatalf("fallback step backbone anchors = %d, want 3", len(plan.StepBackbone))
	}
	if got := plan.StepBackbone[0]; got.Name != "checkCoverage" || got.Line != 127 {
		t.Fatalf("first fallback step backbone anchor = %+v", got)
	}
	if plan.StepBackboneCompleteness != CompletenessLowerBound {
		t.Fatalf("fallback step backbone completeness = %q, want %q", plan.StepBackboneCompleteness, CompletenessLowerBound)
	}
}

func TestBuildAnswerSurfacePlan_AugmentsLowerBoundStepBackboneWithEvidence(t *testing.T) {
	mut := NewMutableState("")
	mut.SetEmittedAnswerSymbols([]AnswerSymbol{
		{Name: "detectLanguage", File: "internal/agent/analyzer.go", Line: 869, Kind: KindFunction},
		{Name: "Compile", File: "internal/agent/analyzer.go", Line: 1175, Kind: KindFunction},
		{Name: "gate.Run", File: "internal/agent/analyzer.go", Line: 1334, Kind: KindFunction},
	}, CompletenessLowerBound)
	ir := &AnalysisIR{
		AnswerContract: AnswerContract{
			RequiredAnswerShape: ShapeStepList,
		},
	}
	evidence := []EvidenceItem{
		{
			Kind:            EvidenceDirect,
			Source:          "internal/agent/analyzer.go",
			LineStart:       869,
			AnchorKind:      AnchorCall,
			AnchorSymbol:    "detectLanguage",
			GroundingStatus: GroundingGrounded,
		},
		{
			Kind:            EvidenceDirect,
			Source:          "internal/agent/analyzer.go",
			LineStart:       1186,
			AnchorKind:      AnchorCall,
			AnchorSymbol:    "Evaluate",
			GroundingStatus: GroundingGrounded,
		},
		{
			Kind:            EvidenceDirect,
			Source:          "internal/agent/analyzer.go",
			LineStart:       1187,
			AnchorKind:      AnchorCall,
			AnchorSymbol:    "Plan",
			GroundingStatus: GroundingGrounded,
		},
		{
			Kind:            EvidenceDirect,
			Source:          "internal/agent/analyzer.go",
			LineStart:       1175,
			AnchorKind:      AnchorCall,
			AnchorSymbol:    "Compile",
			GroundingStatus: GroundingGrounded,
		},
		{
			Kind:            EvidenceDirect,
			Source:          "internal/agent/analyzer.go",
			LineStart:       1334,
			AnchorKind:      AnchorCall,
			AnchorSymbol:    "Run",
			GroundingStatus: GroundingGrounded,
		},
	}

	plan := BuildAnswerSurfacePlan(ir, mut, nil, nil, nil, evidence)
	if plan == nil {
		t.Fatal("BuildAnswerSurfacePlan returned nil")
	}
	if len(plan.StepBackbone) != 5 {
		t.Fatalf("step backbone anchors = %d, want 5 after evidence augmentation", len(plan.StepBackbone))
	}
	gotNames := []string{
		plan.StepBackbone[0].Name,
		plan.StepBackbone[1].Name,
		plan.StepBackbone[2].Name,
		plan.StepBackbone[3].Name,
		plan.StepBackbone[4].Name,
	}
	wantNames := []string{"detectLanguage", "Compile", "Evaluate", "Plan", "gate.Run"}
	if strings.Join(gotNames, ",") != strings.Join(wantNames, ",") {
		t.Fatalf("augmented step backbone names = %v, want %v", gotNames, wantNames)
	}
	if plan.StepBackbone[2].Line != 1186 || plan.StepBackbone[3].Line != 1187 {
		t.Fatalf("augmented evidence anchors not inserted in file order: %+v", plan.StepBackbone)
	}
}

func TestBuildAnswerSurfacePlan_DoesNotAugmentCompleteOrBoundedStepBackbone(t *testing.T) {
	mut := NewMutableState("")
	mut.SetEmittedAnswerSymbols([]AnswerSymbol{
		{Name: "checkCoverage", File: "internal/analysis/gate/gate.go", Line: 127, Kind: KindFunction},
		{Name: "checkDAGClosure", File: "internal/analysis/gate/gate.go", Line: 128, Kind: KindFunction},
		{Name: "checkBudgetSanity", File: "internal/analysis/gate/gate.go", Line: 129, Kind: KindFunction},
	}, CompletenessComplete)
	ir := &AnalysisIR{
		RequestModel: RequestModel{
			RawRequest: "What order do gate.Run's 3 checks execute in?",
			EnumerationBoundary: &RequestedEnumerationBoundary{
				DeclaredCount: 3,
				SourceQuote:   "3 checks",
			},
		},
		AnswerContract: AnswerContract{
			RequiredAnswerShape: ShapeStepList,
		},
	}
	evidence := []EvidenceItem{
		{
			Kind:            EvidenceDirect,
			Source:          "internal/analysis/gate/gate.go",
			LineStart:       130,
			AnchorKind:      AnchorCall,
			AnchorSymbol:    "checkCleanup",
			GroundingStatus: GroundingGrounded,
		},
	}

	plan := BuildAnswerSurfacePlan(ir, mut, nil, nil, nil, evidence)
	if plan == nil {
		t.Fatal("BuildAnswerSurfacePlan returned nil")
	}
	if len(plan.StepBackbone) != 3 {
		t.Fatalf("bounded principal step backbone should remain at 3 checks, got %d", len(plan.StepBackbone))
	}
	if got := plan.StepBackbone[len(plan.StepBackbone)-1].Name; got != "checkBudgetSanity" {
		t.Fatalf("bounded complete step backbone should not be augmented, last = %q", got)
	}
}

func TestBuildAnswerSurfacePlan_DropsOwnerFromRequestedEnumerationBoundaryStepBackbone(t *testing.T) {
	mut := NewMutableState("")
	mut.SetEmittedAnswerSymbols([]AnswerSymbol{
		{Name: "gate.Run", File: "internal/analysis/gate/gate.go", Line: 120, Kind: KindMethod},
		{Name: "checkCoverage", File: "internal/analysis/gate/gate.go", Line: 127, Kind: KindFunction},
		{Name: "checkDAGClosure", File: "internal/analysis/gate/gate.go", Line: 128, Kind: KindFunction},
		{Name: "checkBudgetSanity", File: "internal/analysis/gate/gate.go", Line: 129, Kind: KindFunction},
	}, CompletenessComplete)
	ir := &AnalysisIR{
		RequestModel: RequestModel{
			RawRequest: "What order do gate.Run's 3 checks execute in?",
			AnalyzerHints: AnalyzerHints{
				MentionedEntities: []string{"gate.Run"},
			},
			EnumerationBoundary: &RequestedEnumerationBoundary{
				DeclaredCount: 3,
				SourceQuote:   "3 checks",
			},
		},
		AnswerContract: AnswerContract{
			RequiredAnswerShape: ShapeStepList,
		},
	}

	plan := BuildAnswerSurfacePlan(ir, mut, nil, nil, nil, nil)
	if plan == nil {
		t.Fatal("BuildAnswerSurfacePlan returned nil")
	}
	if len(plan.StepBackbone) != 3 {
		t.Fatalf("step backbone anchors = %d, want 3 after owner drop", len(plan.StepBackbone))
	}
	if got := plan.StepBackbone[0].Name; got != "checkCoverage" {
		t.Fatalf("first step backbone anchor = %q, want checkCoverage", got)
	}
	if plan.StepBackboneCompleteness != CompletenessLowerBound {
		t.Fatalf("step backbone completeness = %q, want %q after owner drop", plan.StepBackboneCompleteness, CompletenessLowerBound)
	}
}

func TestBuildAnswerSurfacePlan_CompilesExplanationAnchorBackboneFromEvidence(t *testing.T) {
	ir := &AnalysisIR{
		RequestModel: RequestModel{
			SubTopics: []SubTopic{
				{Summary: "Criterion 的角色", Entities: []string{"Criterion"}},
				{Summary: "Hypothesis 的角色", Entities: []string{"Hypothesis"}},
				{Summary: "AnalysisIR 如何持有 HypothesisSet", Entities: []string{"AnalysisIR.HypothesisSet", "HypothesisSet"}},
			},
		},
		AnswerContract: AnswerContract{
			RequiredAnswerShape: ShapeExplanation,
		},
	}
	evidence := []EvidenceItem{
		{
			Kind:            EvidenceDirect,
			Source:          "internal/types/analysis_ir.go",
			LineStart:       574,
			AnchorKind:      AnchorDefinition,
			AnchorSymbol:    "Criterion",
			GroundingStatus: GroundingGrounded,
		},
		{
			Kind:            EvidenceDirect,
			Source:          "internal/types/analysis_ir.go",
			LineStart:       896,
			AnchorKind:      AnchorDefinition,
			AnchorSymbol:    "Hypothesis",
			GroundingStatus: GroundingGrounded,
		},
		{
			Kind:            EvidenceDirect,
			Source:          "internal/types/analysis_ir.go",
			LineStart:       33,
			AnchorKind:      AnchorDefinition,
			AnchorSymbol:    "AnalysisIR.HypothesisSet",
			GroundingStatus: GroundingGrounded,
		},
	}

	plan := BuildAnswerSurfacePlan(ir, NewMutableState(""), nil, nil, nil, evidence)
	if plan == nil {
		t.Fatal("BuildAnswerSurfacePlan returned nil")
	}
	if len(plan.ExplanationAnchorBackbone) != 3 {
		t.Fatalf("explanation anchor backbone = %d, want 3", len(plan.ExplanationAnchorBackbone))
	}
	if plan.ExplanationAnchorCompleteness != CompletenessComplete {
		t.Fatalf("explanation anchor completeness = %q, want %q", plan.ExplanationAnchorCompleteness, CompletenessComplete)
	}
	if len(plan.ExplanationAnchorMissingTopics) != 0 {
		t.Fatalf("missing topics = %v, want none", plan.ExplanationAnchorMissingTopics)
	}
	if got := plan.ExplanationAnchorBackbone[2]; got.Name != "AnalysisIR.HypothesisSet" || got.Line != 33 {
		t.Fatalf("third explanation anchor = %+v", got)
	}
}

func TestBuildAnswerSurfacePlan_ExplanationAnchorBackboneSkipsAuxiliaryEvidence(t *testing.T) {
	ir := &AnalysisIR{
		RequestModel: RequestModel{
			SubTopics: []SubTopic{
				{Summary: "Planner 的职责", Entities: []string{"Planner"}},
				{Summary: "Orchestrator 的职责", Entities: []string{"Orchestrator"}},
			},
		},
		AnswerContract: AnswerContract{
			RequiredAnswerShape: ShapeExplanation,
		},
	}
	evidence := []EvidenceItem{
		{
			Kind:            EvidenceDirect,
			Source:          "docs/architecture.md",
			LineStart:       12,
			AnchorKind:      AnchorDefinition,
			AnchorSymbol:    "Planner",
			GroundingStatus: GroundingGrounded,
		},
	}

	plan := BuildAnswerSurfacePlan(ir, NewMutableState(""), nil, nil, nil, evidence)
	if plan == nil {
		t.Fatal("BuildAnswerSurfacePlan returned nil")
	}
	if len(plan.ExplanationAnchorBackbone) != 0 {
		t.Fatalf("auxiliary docs evidence must not satisfy explanation anchors, got %+v", plan.ExplanationAnchorBackbone)
	}
	if len(plan.ExplanationAnchorMissingTopics) != 2 {
		t.Fatalf("missing topics = %d, want 2", len(plan.ExplanationAnchorMissingTopics))
	}
}

func TestBuildAnswerSurfacePlan_ExplanationAnchorBackboneUsesGroundedSummaryBridge(t *testing.T) {
	ir := &AnalysisIR{
		RequestModel: RequestModel{
			SubTopics: []SubTopic{
				{Summary: "missing_key 配置键的最终有效值的计算逻辑", Entities: []string{"missing_key"}},
				{Summary: "code default / codrax.yaml / CLI 三层配置的覆盖优先级顺序", Entities: []string{"codrax.yaml", "CLI"}},
			},
		},
		AnswerContract: AnswerContract{
			RequiredAnswerShape: ShapeExplanation,
		},
	}
	evidence := []EvidenceItem{
		{
			Kind:            EvidenceConditional,
			Source:          "internal/types/config.go",
			LineStart:       727,
			AnchorKind:      AnchorDefinition,
			AnchorSymbol:    "ResolvedExampleSettings",
			Summary:         "ResolvedExampleSettings 实现各字段最终有效值的计算逻辑，零值时回填默认值。",
			GroundingStatus: GroundingGrounded,
		},
		{
			Kind:            EvidenceConditional,
			Source:          "cmd/root.go",
			LineStart:       2144,
			AnchorKind:      AnchorCondition,
			AnchorSymbol:    "flagExample",
			Summary:         "注释明确声明 code default → codrax.yaml → CLI flag 的三层覆盖优先级。",
			GroundingStatus: GroundingGrounded,
		},
	}

	plan := BuildAnswerSurfacePlan(ir, NewMutableState(""), nil, nil, nil, evidence)
	if plan == nil {
		t.Fatal("BuildAnswerSurfacePlan returned nil")
	}
	if len(plan.ExplanationAnchorBackbone) != 2 {
		t.Fatalf("explanation anchor backbone = %d, want 2", len(plan.ExplanationAnchorBackbone))
	}
	if plan.ExplanationAnchorCompleteness != CompletenessComplete {
		t.Fatalf("explanation anchor completeness = %q, want %q", plan.ExplanationAnchorCompleteness, CompletenessComplete)
	}
	if len(plan.ExplanationAnchorMissingTopics) != 0 {
		t.Fatalf("missing topics = %v, want none", plan.ExplanationAnchorMissingTopics)
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
