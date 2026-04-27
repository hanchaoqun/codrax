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
