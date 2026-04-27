package types

import "testing"

func TestEffectiveDiagramContract_DropsHardRequirementWithoutSupport(t *testing.T) {
	base := &DiagramContract{
		Required:       true,
		Minimum:        1,
		PreferredKinds: []DiagramKind{DiagramArchitecture, DiagramFlow},
	}
	got := EffectiveDiagramContract(base, nil)
	if got == nil {
		t.Fatal("got nil contract")
	}
	if got.Required {
		t.Fatalf("required = true, want false when no grounded structure supports a hard diagram")
	}
	if !base.Required {
		t.Fatal("base contract must stay immutable")
	}
}

func TestSupportedDiagramKindsForAnswer_ConfigTraceNeedsMultipleValidatedRoles(t *testing.T) {
	items := []EvidenceItem{
		{
			Source:          "internal/types/config.go",
			LineStart:       707,
			GroundingStatus: GroundingGrounded,
			ContextRole:     EvidenceContextRoleRelatedContext,
			DiagramRole:     EvidenceDiagramRoleDefault,
			AnchorKind:      AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			Subject:         "ExploreMidLoopMinIteration",
		},
	}
	got := SupportedDiagramKindsForAnswer(ScenarioConfigTrace, false, nil, nil, nil, nil, nil, items)
	for _, kind := range got {
		if kind == DiagramArchitecture || kind == DiagramFlow {
			t.Fatalf("single validated precedence role should not unlock config-trace diagram support, got %v", got)
		}
	}
	items = append(items, EvidenceItem{
		Source:          "codrax.yaml.example",
		LineStart:       198,
		GroundingStatus: GroundingGrounded,
		ContextRole:     EvidenceContextRoleRelatedContext,
		DiagramRole:     EvidenceDiagramRoleConfig,
		AnchorKind:      AnchorAssignment,
		AnchorSymbol:    "explore_midloop_min_iteration",
		Subject:         "explore_midloop_min_iteration",
	})
	got = SupportedDiagramKindsForAnswer(
		ScenarioConfigTrace,
		false,
		nil,
		[]string{"internal/types/config.go", "codrax.yaml.example"},
		nil,
		nil,
		nil,
		items,
	)
	wantArch, wantFlow := false, false
	for _, kind := range got {
		if kind == DiagramArchitecture {
			wantArch = true
		}
		if kind == DiagramFlow {
			wantFlow = true
		}
	}
	if !wantArch || !wantFlow {
		t.Fatalf("validated multi-role config-trace evidence should support architecture+flow diagrams, got %v", got)
	}
}
