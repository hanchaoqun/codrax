package types

import (
	"strings"
	"testing"
)

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

func TestEffectiveDiagramContract_DropsSoftPreferenceWithoutSupport(t *testing.T) {
	base := &DiagramContract{
		Required:       false,
		PreferredKinds: []DiagramKind{DiagramArchitecture},
	}
	if got := EffectiveDiagramContract(base, nil); got != nil {
		t.Fatalf("soft diagram preference without grounded support should be dropped, got %+v", got)
	}
}

func TestSupportedDiagramKindsForAnswer_AnswerChainsSupportSequencePreference(t *testing.T) {
	chains := []AnswerChain{
		{Item: EvidenceItem{Source: "internal/agent/analyzer.go", LineStart: 10, AnchorKind: AnchorDefinition, AnchorSymbol: "Analyze"}},
		{Item: EvidenceItem{Source: "internal/agent/explorer.go", LineStart: 20, AnchorKind: AnchorDefinition, AnchorSymbol: "Explore"}},
	}
	supported := SupportedDiagramKindsForAnswer(ScenarioArchitectureExplain, false, nil, nil, nil, nil, chains, nil)
	if !diagramKindSliceContains(supported, DiagramArchitecture) || !diagramKindSliceContains(supported, DiagramSequence) {
		t.Fatalf("answer chains supported kinds=%v, want architecture and sequence", supported)
	}
	dc := EffectiveDiagramContract(&DiagramContract{
		Required:       true,
		Minimum:        1,
		PreferredKinds: []DiagramKind{DiagramSequence, DiagramArchitecture},
	}, supported)
	if dc == nil || len(dc.PreferredKinds) == 0 || dc.PreferredKinds[0] != DiagramSequence {
		t.Fatalf("effective diagram contract=%+v, want sequence preference preserved", dc)
	}
	kind, fence := CompileDiagramSurfaceFence(dc, ScenarioArchitectureExplain, nil, nil, nil, nil, chains, nil, nil)
	if kind != DiagramSequence {
		t.Fatalf("compiled diagram kind=%s, want sequence", kind)
	}
	if !strings.Contains(fence, "sequenceDiagram") {
		t.Fatalf("compiled fence=%q, want sequenceDiagram", fence)
	}
}

func TestEffectiveDiagramContract_PreservesRequiredKindAndSequenceFence(t *testing.T) {
	items := []EvidenceItem{{
		ID:              "ev-call",
		Source:          "mm/page_alloc.c",
		LineStart:       3943,
		Scope:           ScopeLine,
		GroundingStatus: GroundingGrounded,
		AnchorKind:      AnchorCall,
		Subject:         "get_page_from_freelist",
		Object:          "rmqueue",
	}}
	supported := SupportedDiagramKindsForAnswer(ScenarioArchitectureExplain, false, nil, nil, nil, nil, nil, items)
	if !diagramKindSliceContains(supported, DiagramSequence) {
		t.Fatalf("call-edge evidence should support sequence diagrams, got %v", supported)
	}
	dc := EffectiveDiagramContract(&DiagramContract{
		Required:       true,
		Minimum:        1,
		RequiredKind:   DiagramSequence,
		PreferredKinds: []DiagramKind{DiagramSequence, DiagramArchitecture},
	}, supported)
	if dc == nil || !dc.Required || dc.RequiredKind != DiagramSequence {
		t.Fatalf("required sequence contract not preserved: %+v", dc)
	}
	kind, fence := CompileDiagramSurfaceFence(dc, ScenarioArchitectureExplain, nil, nil, nil, nil, nil, items, nil)
	if kind != DiagramSequence {
		t.Fatalf("compiled diagram kind=%s, want sequence", kind)
	}
	if !strings.Contains(fence, "sequenceDiagram") || strings.Contains(fence, "flowchart TD") {
		t.Fatalf("sequence contract must compile a sequenceDiagram seed, got %q", fence)
	}
}

func TestEffectiveDiagramContract_DoesNotSubstituteRequiredKind(t *testing.T) {
	dc := EffectiveDiagramContract(&DiagramContract{
		Required:       true,
		Minimum:        1,
		RequiredKind:   DiagramSequence,
		PreferredKinds: []DiagramKind{DiagramSequence, DiagramArchitecture},
	}, []DiagramKind{DiagramArchitecture})
	if dc == nil {
		t.Fatal("got nil contract")
	}
	if dc.Required {
		t.Fatalf("unsupported required kind should downgrade the hard requirement instead of substituting another kind: %+v", dc)
	}
	if dc.RequiredKind != DiagramNone {
		t.Fatalf("downgraded contract must clear RequiredKind, got %+v", dc)
	}
}

func TestSupportedDiagramKindsForAnswer_EvidenceSupportsExplicitArchitectureDiagram(t *testing.T) {
	items := []EvidenceItem{
		{
			ID:              "ev1",
			Source:          "internal/agent/agent.go",
			LineStart:       188,
			Scope:           ScopeLine,
			GroundingStatus: GroundingGrounded,
			AnchorKind:      AnchorDefinition,
			AnchorSymbol:    "BaseAgent",
		},
		{
			ID:              "ev2",
			Source:          "internal/agent/explorer.go",
			LineStart:       8599,
			Scope:           ScopeLine,
			GroundingStatus: GroundingGrounded,
			AnchorKind:      AnchorCall,
			Subject:         "explorerEvaluator.Observe",
			Object:          "observeMidLoop",
		},
	}
	supported := SupportedDiagramKindsForAnswer(ScenarioArchitectureExplain, false, nil, nil, nil, nil, nil, items)
	if !diagramKindSliceContains(supported, DiagramArchitecture) {
		t.Fatalf("validated architecture evidence should support architecture diagrams, got %v", supported)
	}
	dc := EffectiveDiagramContract(&DiagramContract{
		Required:       true,
		Minimum:        1,
		PreferredKinds: []DiagramKind{DiagramArchitecture},
	}, supported)
	if dc == nil || !dc.Required {
		t.Fatalf("explicit diagram contract should remain required with evidence support, got %+v", dc)
	}
	kind, fence := CompileDiagramSurfaceFence(dc, ScenarioArchitectureExplain, nil, nil, nil, nil, nil, items, nil)
	if kind != DiagramArchitecture {
		t.Fatalf("compiled diagram kind=%s, want %s", kind, DiagramArchitecture)
	}
	if !strings.Contains(fence, "flowchart TD") ||
		!strings.Contains(fence, "explorerEvaluator.Observe") ||
		!strings.Contains(fence, "observeMidLoop") {
		t.Fatalf("compiled evidence fence missing grounded nodes: %q", fence)
	}
}

func diagramKindSliceContains(kinds []DiagramKind, want DiagramKind) bool {
	for _, got := range kinds {
		if got == want {
			return true
		}
	}
	return false
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

func TestSupportedDiagramKindsForAnswer_ConfigTraceAllowsConfigFileCommentRole(t *testing.T) {
	items := []EvidenceItem{
		{
			Source:          "internal/types/config.go",
			LineStart:       707,
			GroundingStatus: GroundingGrounded,
			ContextRole:     EvidenceContextRoleRelatedContext,
			DiagramRole:     EvidenceDiagramRoleDefault,
			AnchorKind:      AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
		},
		{
			Source:          "codrax.yaml.example",
			LineStart:       20,
			GroundingStatus: GroundingGrounded,
			ContextRole:     EvidenceContextRoleIllustrativeOnly,
			DiagramRole:     EvidenceDiagramRoleConfig,
			AnchorKind:      AnchorCondition,
			AnchorSymbol:    "Precedence",
		},
	}
	got := SupportedDiagramKindsForAnswer(
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
		t.Fatalf("config-file precedence comments should count toward config-trace diagram support, got %v", got)
	}
}

func TestSupportedDiagramKindsForAnswer_ConfigTraceAllowsConfigFileAbsenceRole(t *testing.T) {
	items := []EvidenceItem{
		{
			Source:          "internal/types/config.go",
			LineStart:       848,
			GroundingStatus: GroundingGrounded,
			ContextRole:     EvidenceContextRoleRelatedContext,
			DiagramRole:     EvidenceDiagramRoleDefault,
			AnchorKind:      AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
		},
		{
			Source:          "codrax.yaml.example",
			LineStart:       314,
			GroundingStatus: GroundingGrounded,
			ContextRole:     EvidenceContextRoleAbsenceSupport,
			DiagramRole:     EvidenceDiagramRoleConfig,
			AnchorKind:      AnchorCondition,
			AnchorSymbol:    "explore_mid_loop_hint_budget",
			Predicate:       "absent_from",
		},
	}
	got := SupportedDiagramKindsForAnswer(
		ScenarioConfigTrace,
		true,
		&ExactResolutionContract{TargetKind: SubjectConfigKey, Targets: []string{"explore_mid_loop_hint_budget"}, AllowAbsence: true},
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
		t.Fatalf("config-file absence support should still count toward config-trace diagram support, got %v", got)
	}
}

func TestConfigTraceCoverageRoleInFiles_FileRoleFallbacks(t *testing.T) {
	contract := &ExactResolutionContract{
		TargetKind:            SubjectConfigKey,
		Targets:               []string{"missing_key"},
		RequestedContextRoles: []EvidenceDiagramRole{EvidenceDiagramRoleDefault, EvidenceDiagramRoleConfig, EvidenceDiagramRoleOverride},
	}
	tests := []struct {
		name string
		item EvidenceItem
		want EvidenceDiagramRole
	}{
		{
			name: "config file role becomes config coverage",
			item: EvidenceItem{
				Source:          "codrax.yaml.example",
				Scope:           ScopeFile,
				GroundingStatus: GroundingGrounded,
				FileRoleLabel:   FileRoleConfigCanonical,
			},
			want: EvidenceDiagramRoleConfig,
		},
		{
			name: "default struct role becomes default coverage",
			item: EvidenceItem{
				Source:          "internal/types/config.go",
				Scope:           ScopeFile,
				GroundingStatus: GroundingGrounded,
				FileRoleLabel:   FileRoleDefaultStruct,
			},
			want: EvidenceDiagramRoleDefault,
		},
		{
			name: "cli registration role becomes override coverage",
			item: EvidenceItem{
				Source:          "cmd/root.go",
				Scope:           ScopeFile,
				GroundingStatus: GroundingGrounded,
				FileRoleLabel:   FileRoleCLIRegistration,
			},
			want: EvidenceDiagramRoleOverride,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ConfigTraceCoverageRoleInFiles(contract, nil, tc.item); got != tc.want {
				t.Fatalf("ConfigTraceCoverageRoleInFiles() = %q, want %q", got, tc.want)
			}
		})
	}
}
