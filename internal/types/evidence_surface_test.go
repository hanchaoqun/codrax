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

func TestBuildAnswerSurfacePlan_CompilesCapabilitySurfaceAuthority(t *testing.T) {
	ir := &AnalysisIR{
		RequestModel: RequestModel{
			AnalyzerHints: AnalyzerHints{
				CapabilitySurface: &CapabilitySurfaceHint{
					Binding: StageBinding{
						Stage: StageAnalyze,
						Agent: AgentAnalyzer,
						Skill: "analysis-skill",
					},
					Tool: "read_file",
					AuthorityFiles: []string{
						"./internal/orchestrator/topology.go",
						"internal/skill/analysis_contract.go",
						"internal/orchestrator/topology.go",
					},
				},
			},
		},
	}
	plan := BuildAnswerSurfacePlan(ir, NewMutableState(""), nil, nil, nil, nil)
	if plan == nil {
		t.Fatal("BuildAnswerSurfacePlan returned nil")
	}
	if plan.CapabilitySurface == nil {
		t.Fatal("expected capability surface hint on answer surface plan")
	}
	if plan.CapabilitySurface.Tool != "read_file" {
		t.Fatalf("compiled tool = %q, want read_file", plan.CapabilitySurface.Tool)
	}
	if plan.ExactResolution != nil {
		t.Fatalf("capability-surface plan should not carry exact-resolution contract, got %+v", plan.ExactResolution)
	}
	want := []string{
		"internal/orchestrator/topology.go",
		"internal/skill/analysis_contract.go",
	}
	if strings.Join(plan.CapabilityAuthorityFiles, ",") != strings.Join(want, ",") {
		t.Fatalf("authority files = %v, want %v", plan.CapabilityAuthorityFiles, want)
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

func TestBuildAnswerSurfacePlan_ConfigTraceCompilesRequestedRolesFromSurfaceFallbacks(t *testing.T) {
	mut := NewMutableState("")
	ir := &AnalysisIR{
		RequestModel: RequestModel{
			Scenario: ScenarioConfigTrace,
		},
		AnswerContract: AnswerContract{
			ExactResolution: &ExactResolutionContract{
				TargetKind:            SubjectConfigKey,
				TargetLabel:           "config key",
				Targets:               []string{"explore_mid_loop_hint_budget"},
				AllowAbsence:          true,
				RelatedContextPolicy:  ExactContextSameFamilyGrounded,
				RelatedContextTerms:   []string{"explore"},
				RequestedContextRoles: []EvidenceDiagramRole{EvidenceDiagramRoleDefault, EvidenceDiagramRoleConfig, EvidenceDiagramRoleOverride},
			},
			Diagram: &DiagramContract{
				Required:       true,
				PreferredKinds: []DiagramKind{DiagramFlow},
			},
		},
	}
	mut.SetInvestigationComplete("three-layer precedence already grounded")
	mut.SetInvestigationResultKind("absence")
	mut.SetAbsenceJustification("all requested precedence roles were read")
	evidence := []EvidenceItem{
		{
			Kind:            EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       876,
			AnchorKind:      AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			Subject:         "DefaultExploreHeuristics",
			ContextRole:     EvidenceContextRoleRelatedContext,
			GroundingStatus: GroundingGrounded,
		},
		{
			Kind:            EvidenceDirect,
			Source:          "codrax.yaml.example",
			LineStart:       22,
			AnchorKind:      AnchorDefinition,
			AnchorSymbol:    "Precedence",
			ContextRole:     EvidenceContextRoleRelatedContext,
			DiagramRole:     EvidenceDiagramRoleConfig,
			GroundingStatus: GroundingGrounded,
		},
		{
			Kind:                 EvidenceMechanism,
			Source:               "cmd/root.go",
			LineStart:            1635,
			AnchorKind:           AnchorCondition,
			AnchorSymbol:         "ExploreMidLoopMinIteration",
			Subject:              "CLI override binding",
			Snippet:              "if rs.ExploreMidLoopMinIteration != nil { h.MidLoopMinIteration = *rs.ExploreMidLoopMinIteration }",
			ContextRole:          EvidenceContextRoleRelatedContext,
			RequestedDiagramRole: EvidenceDiagramRoleOverride,
			GroundingStatus:      GroundingGrounded,
		},
	}

	plan := BuildAnswerSurfacePlan(ir, mut, nil, nil, nil, evidence)
	if plan == nil {
		t.Fatal("BuildAnswerSurfacePlan returned nil")
	}
	if len(plan.ConfigTraceDiagramAnchors) != 3 {
		t.Fatalf("config-trace anchors = %d, want 3: %+v", len(plan.ConfigTraceDiagramAnchors), plan.ConfigTraceDiagramAnchors)
	}
	for _, want := range []string{"code default", "config file", "operator override"} {
		if !strings.Contains(plan.CompiledDiagramFence, want) {
			t.Fatalf("compiled config-trace fence missing %q: %q", want, plan.CompiledDiagramFence)
		}
	}
}

func TestBuildAnswerSurfacePlan_ConfigTraceCompilesFenceFromImplicitConfigAndDefaultRoles(t *testing.T) {
	mut := NewMutableState("")
	ir := &AnalysisIR{
		RequestModel: RequestModel{
			Scenario: ScenarioConfigTrace,
			AnalyzerHints: AnalyzerHints{
				Kind:         "config_mapping",
				ExactTargets: []string{"explore_mid_loop_hint_budget"},
			},
			AnswerSubject: AnswerSubject{Kind: SubjectConfigKey},
		},
		AnswerContract: AnswerContract{
			ExactResolution: &ExactResolutionContract{
				TargetKind:            SubjectConfigKey,
				TargetLabel:           "config key",
				Targets:               []string{"explore_mid_loop_hint_budget"},
				AllowAbsence:          true,
				RelatedContextPolicy:  ExactContextSameFamilyGrounded,
				RelatedContextTerms:   []string{"explore"},
				RequestedContextRoles: []EvidenceDiagramRole{EvidenceDiagramRoleDefault, EvidenceDiagramRoleConfig, EvidenceDiagramRoleOverride},
			},
			Diagram: &DiagramContract{
				Required:       false,
				PreferredKinds: []DiagramKind{DiagramFlow},
			},
		},
	}
	mut.SetInvestigationComplete("grounded exact absence with nearby precedence context")
	mut.SetInvestigationResultKind("absence")
	mut.SetAbsenceJustification("the exact config key is absent from both the config surface and the runtime struct")
	mut.SetExactContextRequiredFiles([]string{"codrax.yaml.example", "internal/config/runtime.go"})
	evidence := []EvidenceItem{
		{
			Kind:            EvidenceDirect,
			Source:          "codrax.yaml.example",
			LineStart:       403,
			AnchorKind:      AnchorCondition,
			AnchorSymbol:    "codrax.yaml.example",
			ContextRole:     EvidenceContextRoleAbsenceSupport,
			GroundingStatus: GroundingGrounded,
			Summary:         "Explorer heuristics config block lists explore_midloop_* keys but does not include explore_mid_loop_hint_budget",
		},
		{
			Kind:            EvidenceDirect,
			Source:          "internal/config/runtime.go",
			LineStart:       329,
			AnchorKind:      AnchorDefinition,
			AnchorSymbol:    "RuntimeSettings",
			ContextRole:     EvidenceContextRoleDefining,
			GroundingStatus: GroundingGrounded,
			Summary:         "RuntimeSettings exposes explore_* fields such as ExploreMidLoopMinIteration and related midloop heuristics, but no hint_budget variant",
		},
	}

	plan := BuildAnswerSurfacePlan(ir, mut, nil, nil, nil, evidence)
	if plan == nil {
		t.Fatal("BuildAnswerSurfacePlan returned nil")
	}
	if len(plan.ConfigTraceDiagramAnchors) != 2 {
		t.Fatalf("config-trace anchors = %d, want 2: %+v", len(plan.ConfigTraceDiagramAnchors), plan.ConfigTraceDiagramAnchors)
	}
	for _, want := range []string{"config file", "code default"} {
		if !strings.Contains(plan.CompiledDiagramFence, want) {
			t.Fatalf("compiled config-trace fence missing %q: %q", want, plan.CompiledDiagramFence)
		}
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

func TestBuildAnswerSurfacePlan_CollectsExternalObservationSeeds(t *testing.T) {
	mut := NewMutableState("")
	logBundle := &LogBundle{
		Errors: []LogError{{
			Type: "runtime error: invalid memory address or nil pointer dereference",
			Frames: []LogFrame{
				{
					File: "internal/agent/analyzer.go",
					Line: 320,
					Func: "github.com/hanchaoqun/codrax/internal/agent.(*analyzerEvaluator).ParseOutput",
					Raw:  "github.com/hanchaoqun/codrax/internal/agent.(*analyzerEvaluator).ParseOutput(0x0, 0x0, 0x0)",
				},
				{
					File: "internal/agent/analyzer.go",
					Line: 250,
					Func: "github.com/hanchaoqun/codrax/internal/agent.buildAnalysisIR",
					Raw:  "github.com/hanchaoqun/codrax/internal/agent.buildAnalysisIR(0x0)",
				},
			},
		}},
	}
	ir := &AnalysisIR{
		RequestModel: RequestModel{
			Scenario: ScenarioRootCause,
			Intent:   IntentRootCause,
		},
		AnswerContract: AnswerContract{
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
			Kind:            EvidenceDirect,
			Source:          "internal/agent/analyzer.go",
			LineStart:       860,
			AnchorKind:      AnchorDefinition,
			AnchorSymbol:    "buildAnalysisIR",
			GroundingStatus: GroundingGrounded,
		},
	}

	plan := BuildAnswerSurfacePlan(ir, mut, logBundle, nil, nil, evidence)
	if plan == nil {
		t.Fatal("BuildAnswerSurfacePlan returned nil")
	}
	if len(plan.ExternalObservationSeeds) < 2 {
		t.Fatalf("external observation seeds = %d, want at least 2", len(plan.ExternalObservationSeeds))
	}
	var sawErrorType bool
	var sawFrame bool
	for _, seed := range plan.ExternalObservationSeeds {
		if seed.Kind == "error_type" && strings.Contains(seed.Raw, "nil pointer dereference") {
			sawErrorType = true
		}
		if seed.Kind == "log_frame" && strings.Contains(seed.Raw, "ParseOutput(0x0, 0x0, 0x0)") {
			sawFrame = true
			if seed.AnchoredFile != "internal/agent/analyzer.go" || seed.AnchoredLine != 651 {
				t.Fatalf("frame seed anchor = %s:%d, want analyzer.go:651", seed.AnchoredFile, seed.AnchoredLine)
			}
		}
	}
	if !sawErrorType {
		t.Fatalf("expected structured error type seed, got %+v", plan.ExternalObservationSeeds)
	}
	if !sawFrame {
		t.Fatalf("expected runtime frame seed, got %+v", plan.ExternalObservationSeeds)
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

	// Pre-PR5 the loop iterated [ShapeStepList, ShapeExplanation];
	// AnswerShape is retired so the surface plan keys off
	// Intent/Scenario directly (see ScenarioRootCause + IntentRootCause).
	ir := &AnalysisIR{
		RequestModel: RequestModel{
			Scenario: ScenarioRootCause,
			Intent:   IntentRootCause,
		},
		AnswerContract: AnswerContract{},
	}
	plan := BuildAnswerSurfacePlan(ir, mut, logBundle, nil, nil, evidence)
	if plan == nil {
		t.Fatal("BuildAnswerSurfacePlan returned nil")
	}
	if plan.SummarySurfaceMode != AnswerSummarySurfaceDriftBoundedRootCause {
		t.Fatalf("summary surface mode = %s, want %s", plan.SummarySurfaceMode, AnswerSummarySurfaceDriftBoundedRootCause)
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

func TestBuildAnswerSurfacePlan_DriftBoundedSurfaceSkipsOuterFunctionNoiseWhenCallEdgeExists(t *testing.T) {
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
		},
	}
	evidence := []EvidenceItem{
		{
			Kind:            EvidenceRelationship,
			Source:          "internal/agent/analyzer.go",
			LineStart:       674,
			AnchorKind:      AnchorCall,
			AnchorSymbol:    "buildAnalysisIR",
			Subject:         "ParseOutput",
			Object:          "buildAnalysisIR",
			GroundingStatus: GroundingGrounded,
		},
		{
			Kind:            EvidenceConditional,
			Source:          "internal/agent/analyzer.go",
			LineStart:       884,
			AnchorKind:      AnchorCondition,
			AnchorSymbol:    "buildAnalysisIR",
			OwnerSymbol:     "buildAnalysisIR",
			Condition:       "ctx == nil || ctx.Mutable == nil",
			GroundingStatus: GroundingGrounded,
		},
		{
			Kind:            EvidenceConditional,
			Source:          "internal/agent/analyzer.go",
			LineStart:       653,
			AnchorKind:      AnchorCondition,
			AnchorSymbol:    "emitCalls",
			OwnerSymbol:     "ParseOutput",
			Condition:       "emitCalls == 0",
			GroundingStatus: GroundingGrounded,
		},
	}

	plan := BuildAnswerSurfacePlan(ir, mut, logBundle, nil, nil, evidence)
	if plan == nil {
		t.Fatal("BuildAnswerSurfacePlan returned nil")
	}
	for _, item := range plan.DriftBoundedSurfaceItems {
		if item.LineStart == 653 {
			t.Fatalf("outer-function noise should stay out once the call edge is grounded, got %+v", plan.DriftBoundedSurfaceItems)
		}
	}
}

func TestDriftBoundedRenderableSurfaceItems_FocusesOnCalleeAnchorsWhenCallEdgeExists(t *testing.T) {
	items := []EvidenceItem{
		{
			Kind:         EvidenceRelationship,
			Source:       "internal/agent/analyzer.go",
			LineStart:    674,
			AnchorKind:   AnchorCall,
			AnchorSymbol: "buildAnalysisIR",
			Subject:      "ParseOutput",
			Object:       "buildAnalysisIR",
			Predicate:    "calls",
		},
		{
			Kind:         EvidenceConditional,
			Source:       "internal/agent/analyzer.go",
			LineStart:    884,
			AnchorKind:   AnchorCondition,
			AnchorSymbol: "buildAnalysisIR",
			OwnerSymbol:  "buildAnalysisIR",
			Condition:    "ctx == nil || ctx.Mutable == nil",
		},
		{
			Kind:         EvidenceConditional,
			Source:       "internal/agent/analyzer.go",
			LineStart:    653,
			AnchorKind:   AnchorCondition,
			AnchorSymbol: "ParseOutput",
			OwnerSymbol:  "ParseOutput",
			Condition:    "emitCalls == 0",
		},
	}

	filtered := DriftBoundedRenderableSurfaceItems(items)
	if len(filtered) != 2 {
		t.Fatalf("filtered len = %d, want 2 items focused on call edge + callee anchors", len(filtered))
	}
	for _, item := range filtered {
		if item.LineStart == 653 {
			t.Fatalf("outer caller guard should be dropped from renderable drift surface: %+v", filtered)
		}
	}
}

func TestBuildAnswerSurfacePlan_DriftBoundedSurfaceUsesOwnerSymbolForLineLocalCompanion(t *testing.T) {
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
			LineStart:       884,
			AnchorKind:      AnchorCondition,
			AnchorSymbol:    "ctx",
			OwnerSymbol:     "buildAnalysisIR",
			Condition:       "ctx == nil || ctx.Mutable == nil",
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
	if got := plan.DriftBoundedSurfaceItems[1].LineStart; got != 884 {
		t.Fatalf("second drift-bounded surface item line = %d, want owner-backed guard at 884", got)
	}
}

func TestRenderDriftBoundedSurfaceDiagramFence_UsesStatementLevelAnchors(t *testing.T) {
	items := []EvidenceItem{
		{
			Kind:         EvidenceRelationship,
			Source:       "internal/agent/analyzer.go",
			LineStart:    674,
			AnchorKind:   AnchorCall,
			AnchorSymbol: "buildAnalysisIR",
			Subject:      "analyzerEvaluator.ParseOutput",
			Object:       "buildAnalysisIR",
			Predicate:    "calls",
		},
		{
			Kind:       EvidenceConditional,
			Source:     "internal/agent/analyzer.go",
			LineStart:  884,
			AnchorKind: AnchorCondition,
			Condition:  "ctx == nil || ctx.Mutable == nil",
		},
		{
			Kind:       EvidenceMechanism,
			Source:     "internal/agent/analyzer.go",
			LineStart:  891,
			AnchorKind: AnchorAssignment,
			Object:     "ctx.Mutable.RequestModel()",
		},
	}

	fence := RenderDriftBoundedSurfaceDiagramFence(items)
	if fence == "" {
		t.Fatal("expected deterministic drift-bounded fence")
	}
	for _, want := range []string{
		"analyzerEvaluator.ParseOutput calls buildAnalysisIR @ internal/agent/analyzer.go:674",
		"if ctx == nil || ctx.Mutable == nil @ internal/agent/analyzer.go:884",
		"ctx.Mutable.RequestModel() @ internal/agent/analyzer.go:891",
	} {
		if !strings.Contains(fence, want) {
			t.Fatalf("fence missing %q:\n%s", want, fence)
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
		RequestModel: RequestModel{Intent: IntentTrace},
		AnswerContract: AnswerContract{
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
		RequestModel: RequestModel{Intent: IntentTrace},
		AnswerContract: AnswerContract{
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
		RequestModel: RequestModel{Intent: IntentTrace},
		AnswerContract: AnswerContract{
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

func TestEvidenceDeterministicSurfaceText_PrefersStructuredCoreOverFreeformSummary(t *testing.T) {
	item := EvidenceItem{
		Kind:         EvidenceConditional,
		AnchorSymbol: "logBundle",
		Predicate:    "guards",
		Condition:    "logBundle != nil",
		Summary:      "the nil check only validates logBundle itself and proves Meta may be nil later",
	}

	got := EvidenceDeterministicSurfaceText(item, false)
	if strings.Contains(got, "Meta may be nil") || strings.Contains(got, "proves") {
		t.Fatalf("deterministic surface leaked stronger summary prose: %q", got)
	}
	if got != "logBundle guards IF logBundle != nil" {
		t.Fatalf("deterministic surface = %q, want typed surface", got)
	}
}

func TestEvidenceStructuredSemanticLine_DoesNotFallbackToFreeformSummary(t *testing.T) {
	item := EvidenceItem{
		Kind:    EvidenceMechanism,
		Summary: "the caller definitely passed a nil ctx and the guard already succeeded",
	}

	if got := EvidenceStructuredSemanticLine(item, false); got != "" {
		t.Fatalf("structured semantic line leaked freeform summary: %q", got)
	}
}

func TestEvidenceDeterministicSurfaceText_AssignmentPrefersGroundedSnippetOverFreeformSemantics(t *testing.T) {
	item := EvidenceItem{
		Kind:         EvidenceDirect,
		AnchorKind:   AnchorAssignment,
		AnchorSymbol: "RequestModel",
		OwnerSymbol:  "buildAnalysisIR",
		Subject:      "buildAnalysisIR",
		Predicate:    "dereferences",
		Object:       "nil",
		Snippet:      "raw := ctx.Mutable.RequestModel()",
		Summary:      "buildAnalysisIR definitely dereferences nil here and proves caller-side nil provenance",
	}

	got := EvidenceDeterministicSurfaceText(item, false)
	if strings.Contains(got, "dereferences") || strings.Contains(got, "caller-side nil provenance") {
		t.Fatalf("assignment deterministic surface leaked unsupported semantic escalation: %q", got)
	}
	if got != "raw := ctx.Mutable.RequestModel()" {
		t.Fatalf("assignment deterministic surface = %q, want grounded snippet", got)
	}
}

func TestEvidenceDeterministicSurfaceText_DefinitionPrefersGroundedSnippetOverFreeformSemantics(t *testing.T) {
	item := EvidenceItem{
		Kind:         EvidenceMechanism,
		AnchorKind:   AnchorDefinition,
		AnchorSymbol: "buildAnalysisIR",
		Subject:      "buildAnalysisIR",
		Predicate:    "defines",
		Snippet:      "func buildAnalysisIR(ctx *types.AgentContext) (*types.AnalysisIR, error) {",
		Summary:      "buildAnalysisIR proves the current mechanism and explains why the panic happens",
	}

	got := EvidenceDeterministicSurfaceText(item, false)
	if strings.Contains(got, "proves the current mechanism") || strings.Contains(got, "defines") {
		t.Fatalf("definition deterministic surface leaked unsupported semantic escalation: %q", got)
	}
	if got != "func buildAnalysisIR(ctx *types.AgentContext) (*types.AnalysisIR, error) {" {
		t.Fatalf("definition deterministic surface = %q, want grounded snippet", got)
	}
}

func TestEvidenceDeterministicSurfaceText_ConditionPrefersGroundedSnippetOverFreeformSemantics(t *testing.T) {
	item := EvidenceItem{
		Kind:         EvidenceConditional,
		AnchorKind:   AnchorCondition,
		AnchorSymbol: "buildAnalysisIR",
		OwnerSymbol:  "buildAnalysisIR",
		Subject:      "buildAnalysisIR",
		Predicate:    "returns",
		Condition:    "ctx == nil || ctx.Mutable == nil",
		Snippet:      "if ctx == nil || ctx.Mutable == nil { return nil, errors.New(\"analyzer: missing AgentContext.Mutable\") }",
		Summary:      "the nil guard proves caller-side nil provenance and explains the panic",
	}

	got := EvidenceDeterministicSurfaceText(item, false)
	if strings.Contains(got, "proves caller-side nil provenance") || strings.Contains(got, "returns") {
		t.Fatalf("condition deterministic surface leaked unsupported semantic escalation: %q", got)
	}
	if got != "if ctx == nil || ctx.Mutable == nil { return nil, errors.New(\"analyzer: missing AgentContext.Mutable\") }" {
		t.Fatalf("condition deterministic surface = %q, want grounded snippet", got)
	}
}

func TestEvidenceAuthoritativeSurfaceText_DefinitionWithoutSnippetStaysNeutral(t *testing.T) {
	item := EvidenceItem{
		Kind:         EvidenceMechanism,
		AnchorKind:   AnchorDefinition,
		AnchorSymbol: "buildAnalysisIR",
		Subject:      "buildAnalysisIR",
		Predicate:    "proves caller-side nil provenance",
		Summary:      "buildAnalysisIR proves caller-side nil provenance",
	}

	got := EvidenceAuthoritativeSurfaceText(item, false)
	if strings.Contains(got, "proves caller-side nil provenance") {
		t.Fatalf("authoritative surface leaked unsupported definition semantics: %q", got)
	}
	if got != "definition anchor for buildAnalysisIR" {
		t.Fatalf("authoritative surface = %q, want neutral definition anchor", got)
	}
}

func TestEvidenceAuthoritativeSurfaceText_NeverFallsBackToFreeformSummary(t *testing.T) {
	item := EvidenceItem{
		Kind:    EvidenceMechanism,
		Summary: "the caller definitely passed a nil ctx and the guard already succeeded",
	}

	if got := EvidenceAuthoritativeSurfaceText(item, false); got != "" {
		t.Fatalf("authoritative surface leaked freeform summary: %q", got)
	}
}

func TestEvidenceStructuredSemanticLine_AssignmentWithoutSnippetFallsBackToNeutralAnchor(t *testing.T) {
	item := EvidenceItem{
		Kind:         EvidenceDirect,
		AnchorKind:   AnchorAssignment,
		AnchorSymbol: "RequestModel",
		OwnerSymbol:  "buildAnalysisIR",
		Subject:      "buildAnalysisIR",
		Predicate:    "dereferences",
		Object:       "nil",
	}

	got := EvidenceStructuredSemanticLine(item, false)
	if strings.Contains(got, "dereferences") || strings.Contains(got, "nil") {
		t.Fatalf("assignment structured surface leaked unsupported semantics: %q", got)
	}
	if got != "buildAnalysisIR assignment anchor for RequestModel" {
		t.Fatalf("assignment structured surface = %q, want neutral anchor surface", got)
	}
}

func TestEvidenceStructuredSemanticLine_ConditionWithoutSnippetFallsBackToGuardSurface(t *testing.T) {
	item := EvidenceItem{
		Kind:         EvidenceConditional,
		AnchorKind:   AnchorCondition,
		AnchorSymbol: "buildAnalysisIR",
		OwnerSymbol:  "buildAnalysisIR",
		Subject:      "buildAnalysisIR",
		Predicate:    "returns",
		Condition:    "ctx == nil || ctx.Mutable == nil",
	}

	got := EvidenceStructuredSemanticLine(item, false)
	if strings.Contains(got, "returns") {
		t.Fatalf("condition structured surface leaked unsupported semantics: %q", got)
	}
	if got != "buildAnalysisIR guard condition IF ctx == nil || ctx.Mutable == nil" {
		t.Fatalf("condition structured surface = %q, want neutral guard surface", got)
	}
}
