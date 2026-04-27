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
}
