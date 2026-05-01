package agent

import (
	"testing"

	repomap "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestCollectExactResolutionSymbolCandidatesFromGraph_IncludesStrongSameFamilyDefaultSymbol(t *testing.T) {
	contract := &types.ExactResolutionContract{
		TargetKind:           types.SubjectConfigKey,
		TargetLabel:          "config key",
		Targets:              []string{"explore_mid_loop_hint_budget"},
		AllowAbsence:         true,
		RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
		RelatedContextTerms:  []string{"explore"},
	}
	fileSymbols := map[string][]string{
		"internal/config/runtime.go": {
			"ExploreMidLoopMinIteration internal/config/runtime.go:231",
			"ExploreMidLoopEnumCoverage internal/config/runtime.go:235",
		},
		"internal/types/config.go": {
			"DefaultExploreHeuristics internal/types/config.go:707",
			"ExploreSettings internal/types/config.go:618",
		},
	}
	evidence := []types.EvidenceItem{{
		Kind:            types.EvidenceDirect,
		Source:          "internal/config/runtime.go",
		LineStart:       231,
		AnchorKind:      types.AnchorAssignment,
		AnchorSymbol:    "ExploreMidLoopMinIteration",
		ContextRole:     types.EvidenceContextRoleRelatedContext,
		GroundingStatus: types.GroundingGrounded,
	}}

	cands := collectExactResolutionSymbolCandidatesFromGraph(nil, contract, nil, fileSymbols, evidence)
	if len(cands) == 0 {
		t.Fatalf("expected same-family candidates, got none")
	}
	var sawDefault bool
	for _, cand := range cands {
		if cand.Symbol == "DefaultExploreHeuristics" {
			sawDefault = true
			break
		}
	}
	if !sawDefault {
		t.Fatalf("expected DefaultExploreHeuristics candidate, got %+v", cands)
	}
}

func TestCollectExactResolutionSymbolCandidatesFromGraph_ConfigRoleAnchorsSuppressParallelRuntimeFamily(t *testing.T) {
	contract := &types.ExactResolutionContract{
		TargetKind:           types.SubjectConfigKey,
		TargetLabel:          "config key",
		Targets:              []string{"explore_mid_loop_hint_budget"},
		AllowAbsence:         true,
		RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
		RelatedContextTerms:  []string{"explore"},
	}
	fileSymbols := map[string][]string{
		"internal/config/runtime.go": {
			"ExploreMidLoopMinIteration internal/config/runtime.go:231",
		},
		"internal/types/config.go": {
			"DefaultExploreHeuristics internal/types/config.go:707",
		},
		"internal/types/explore_budget.go": {
			"ExploreBudget internal/types/explore_budget.go:40",
		},
	}
	evidence := []types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/config/runtime.go",
			LineStart:       231,
			AnchorKind:      types.AnchorAssignment,
			AnchorSymbol:    "ExploreMidLoopMinIteration",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			DiagramRole:     types.EvidenceDiagramRoleRuntime,
			GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       707,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			DiagramRole:     types.EvidenceDiagramRoleDefault,
			GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/explore_budget.go",
			LineStart:       40,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "ExploreBudget",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			GroundingStatus: types.GroundingGrounded,
		},
	}

	cands := collectExactResolutionSymbolCandidatesFromGraph(nil, contract, nil, fileSymbols, evidence)
	if len(cands) == 0 {
		t.Fatalf("expected config-lane candidates, got none")
	}
	for _, cand := range cands {
		if cand.File == "internal/types/explore_budget.go" || cand.Symbol == "ExploreBudget" {
			t.Fatalf("parallel runtime family candidate leaked past strict role anchors: %+v", cands)
		}
	}
}

func TestPendingExactResolutionContextCandidates_IgnoresRecoveredSummaryOnlyMentions(t *testing.T) {
	contract := &types.ExactResolutionContract{
		TargetKind:           types.SubjectConfigKey,
		TargetLabel:          "config key",
		Targets:              []string{"explore_mid_loop_hint_budget"},
		AllowAbsence:         true,
		RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
		RelatedContextTerms:  []string{"explore"},
	}
	cands := []exactResolutionSymbolCandidate{{
		File:   "internal/types/config.go",
		Symbol: "DefaultExploreHeuristics",
		Line:   707,
	}}
	recoveredSummaryOnly := []types.EvidenceItem{{
		Kind:            types.EvidenceDirect,
		Source:          "internal/types/config.go",
		LineStart:       627,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "RuntimeSettings",
		ContextRole:     types.EvidenceContextRoleRelatedContext,
		GroundingStatus: types.GroundingRecovered,
		Summary:         "DefaultExploreHeuristics defines explore defaults for the same family.",
	}}
	if got := pendingExactResolutionContextCandidates(contract, recoveredSummaryOnly, cands); len(got) != 1 {
		t.Fatalf("recovered summary-only evidence should leave candidate pending, got %+v", got)
	}

	groundedStructured := []types.EvidenceItem{{
		Kind:            types.EvidenceDirect,
		Source:          "internal/types/config.go",
		LineStart:       707,
		Subject:         "DefaultExploreHeuristics",
		Predicate:       "defines",
		Object:          "ExploreHeuristics defaults",
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "DefaultExploreHeuristics",
		ContextRole:     types.EvidenceContextRoleRelatedContext,
		DiagramRole:     types.EvidenceDiagramRoleDefault,
		GroundingStatus: types.GroundingGrounded,
	}}
	if got := pendingExactResolutionContextCandidates(contract, groundedStructured, cands); len(got) != 0 {
		t.Fatalf("grounded structured evidence should satisfy candidate, got %+v", got)
	}
}

func TestRefreshedExactResolutionContextFiles_PrefersGroundedEvidenceOverPendingNeighbors(t *testing.T) {
	contract := &types.ExactResolutionContract{
		TargetKind:           types.SubjectConfigKey,
		TargetLabel:          "config key",
		Targets:              []string{"explore_mid_loop_hint_budget"},
		AllowAbsence:         true,
		RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
		RelatedContextTerms:  []string{"explore"},
	}
	cands := []exactResolutionSymbolCandidate{
		{File: "internal/types/explore_budget.go", Symbol: "ExploreBudget", Score: 12},
		{File: "internal/types/config.go", Symbol: "DefaultExploreHeuristics", Score: 11},
	}
	evidence := []types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Subject:         "explore_mid_loop_hint_budget",
			Predicate:       "absent_from",
			Object:          "RuntimeSettings",
			Source:          "internal/config/runtime.go",
			GroundingStatus: types.GroundingGrounded,
			ContextRole:     types.EvidenceContextRoleAbsenceSupport,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "RuntimeSettings",
		},
		{
			Kind:            types.EvidenceDirect,
			Subject:         "DefaultExploreHeuristics",
			Predicate:       "provides_defaults_for",
			Object:          "explore settings",
			Source:          "internal/types/config.go",
			GroundingStatus: types.GroundingGrounded,
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			DiagramRole:     types.EvidenceDiagramRoleDefault,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
		},
	}
	got := refreshedExactResolutionContextFiles(contract, types.ScenarioConfigTrace, nil, evidence, cands, []string{
		"internal/types/config.go",
		"internal/types/explore_budget.go",
	}, nil)
	if len(got) != 1 || got[0] != "internal/types/config.go" {
		t.Fatalf("grounded related-context files should override pending lexical neighbors, got %v", got)
	}
}

func TestRefreshedExactResolutionContextFiles_KeepsCoverageShapingFilesUntilConfigTraceClosureIsReady(t *testing.T) {
	contract := &types.ExactResolutionContract{
		TargetKind:           types.SubjectConfigKey,
		TargetLabel:          "config key",
		Targets:              []string{"explore_mid_loop_hint_budget"},
		AllowAbsence:         true,
		RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
		RelatedContextTerms:  []string{"explore"},
	}
	evidence := []types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Subject:         "explore_mid_loop_hint_budget",
			Predicate:       "absent_from",
			Object:          "ExploreHeuristics",
			Source:          "internal/types/config.go",
			GroundingStatus: types.GroundingGrounded,
			ContextRole:     types.EvidenceContextRoleAbsenceSupport,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "ExploreHeuristics",
			LineStart:       768,
		},
		{
			Kind:            types.EvidenceDirect,
			Subject:         "DefaultExploreHeuristics",
			Predicate:       "provides_defaults_for",
			Object:          "explore settings",
			Source:          "internal/types/config.go",
			GroundingStatus: types.GroundingGrounded,
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			DiagramRole:     types.EvidenceDiagramRoleDefault,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			LineStart:       848,
		},
		{
			Kind:                 types.EvidenceDirect,
			Subject:              "explore",
			Object:               "ExploreHeuristics",
			Source:               "codrax.yaml.example",
			GroundingStatus:      types.GroundingUngrounded,
			ContextRole:          types.EvidenceContextRoleRelatedContext,
			RequestedDiagramRole: types.EvidenceDiagramRoleConfig,
			AnchorKind:           types.AnchorDefinition,
			AnchorSymbol:         "explore",
			LineStart:            309,
		},
		{
			Kind:                 types.EvidenceDirect,
			Subject:              "pipeline-max-stage-visits",
			Source:               "cmd/root.go",
			GroundingStatus:      types.GroundingUngrounded,
			ContextRole:          types.EvidenceContextRoleRelatedContext,
			RequestedDiagramRole: types.EvidenceDiagramRoleOverride,
			AnchorKind:           types.AnchorAssignment,
			AnchorSymbol:         "h.MidLoopMinIteration",
			LineStart:            1556,
		},
	}
	got := refreshedExactResolutionContextFiles(contract, types.ScenarioConfigTrace, nil, evidence, nil, []string{
		"internal/types/config.go",
		"codrax.yaml.example",
	}, nil)
	want := []string{
		"internal/types/config.go",
		"codrax.yaml.example",
		"cmd/root.go",
	}
	if len(got) != len(want) {
		t.Fatalf("context files len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("context files = %v, want %v", got, want)
		}
	}
}

func TestRefreshedExactResolutionContextFiles_AddsGraphCoverageHopsForMissingRequestedRoles(t *testing.T) {
	contract := &types.ExactResolutionContract{
		TargetKind:            types.SubjectConfigKey,
		TargetLabel:           "config key",
		Targets:               []string{"explore_mid_loop_hint_budget"},
		AllowAbsence:          true,
		RelatedContextPolicy:  types.ExactContextSameFamilyGrounded,
		RelatedContextTerms:   []string{"explore"},
		RequestedContextRoles: []types.EvidenceDiagramRole{types.EvidenceDiagramRoleDefault, types.EvidenceDiagramRoleConfig, types.EvidenceDiagramRoleOverride},
	}
	graph := &repomap.Graph{
		ReverseImports: map[string][]string{
			"internal/types/config.go": {"cmd/root.go", "internal/agent/explorer.go"},
			"codrax.yaml.example":      {"cmd/root.go"},
		},
		QueryScores: map[string]float64{
			"cmd/root.go":                0.9,
			"internal/agent/explorer.go": 0.4,
		},
	}
	evidence := []types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Subject:         "DefaultExploreHeuristics",
			Predicate:       "provides_defaults_for",
			Object:          "explore settings",
			Source:          "internal/types/config.go",
			GroundingStatus: types.GroundingGrounded,
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			DiagramRole:     types.EvidenceDiagramRoleDefault,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			LineStart:       848,
		},
		{
			Kind:                 types.EvidenceDirect,
			Subject:              "explore",
			Object:               "config precedence comment",
			Source:               "codrax.yaml.example",
			GroundingStatus:      types.GroundingGrounded,
			ContextRole:          types.EvidenceContextRoleRelatedContext,
			DiagramRole:          types.EvidenceDiagramRoleConfig,
			RequestedDiagramRole: types.EvidenceDiagramRoleConfig,
			AnchorKind:           types.AnchorDefinition,
			AnchorSymbol:         "explore",
			LineStart:            20,
		},
	}
	got := refreshedExactResolutionContextFiles(
		contract,
		types.ScenarioConfigTrace,
		graph,
		evidence,
		nil,
		[]string{"internal/types/config.go", "codrax.yaml.example"},
		nil,
	)
	want := []string{
		"internal/types/config.go",
		"codrax.yaml.example",
		"cmd/root.go",
		"internal/agent/explorer.go",
	}
	if len(got) != len(want) {
		t.Fatalf("context files len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("context files = %v, want %v", got, want)
		}
	}
}

func TestExactResolutionContextFilesFromScenarioCoverageEvidence_IgnoresUnrelatedRequestedDefaultFile(t *testing.T) {
	contract := &types.ExactResolutionContract{
		TargetKind:            types.SubjectConfigKey,
		TargetLabel:           "config key",
		Targets:               []string{"explore_mid_loop_hint_budget"},
		AllowAbsence:          true,
		RelatedContextPolicy:  types.ExactContextSameFamilyGrounded,
		RelatedContextTerms:   []string{"config"},
		RequestedContextRoles: []types.EvidenceDiagramRole{types.EvidenceDiagramRoleDefault, types.EvidenceDiagramRoleConfig, types.EvidenceDiagramRoleOverride},
	}
	evidence := []types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Subject:         "DefaultExploreHeuristics",
			Source:          "internal/types/config.go",
			GroundingStatus: types.GroundingGrounded,
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			DiagramRole:     types.EvidenceDiagramRoleDefault,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			LineStart:       876,
		},
		{
			Kind:                 types.EvidenceDirect,
			Subject:              "Composer",
			Source:               "internal/analysis/hint/composer.go",
			GroundingStatus:      types.GroundingGrounded,
			ContextRole:          types.EvidenceContextRoleRelatedContext,
			RequestedDiagramRole: types.EvidenceDiagramRoleDefault,
			AnchorKind:           types.AnchorDefinition,
			AnchorSymbol:         "Composer",
			Snippet:              "type Composer struct { cfg Config }",
			LineStart:            129,
		},
	}
	got := exactResolutionContextFilesFromScenarioCoverageEvidence(
		contract,
		types.ScenarioConfigTrace,
		evidence,
		[]string{"internal/types/config.go", "codrax.yaml.example"},
	)
	for _, file := range got {
		if file == "internal/analysis/hint/composer.go" {
			t.Fatalf("unrelated requested-role file leaked into coverage scope: %v", got)
		}
	}
}
