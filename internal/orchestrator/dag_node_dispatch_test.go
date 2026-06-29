package orchestrator

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/analysis/criterion"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestShouldDispatchExploreNodesIndividually_PositiveTwoEvidenceSiblings
// pins the canonical happy path: two NodeEvidence sibling nodes (the
// shape produced by expandEvidenceNodes when len(SubTopics) > 1) trip
// the predicate so the scheduler will give each its own focused
// dispatch.
func TestShouldDispatchExploreNodesIndividually_PositiveTwoEvidenceSiblings(t *testing.T) {
	window := []*types.TaskNode{
		{ID: "n1_evidence_t0", Type: types.NodeEvidence},
		{ID: "n1_evidence_t1", Type: types.NodeEvidence},
	}
	if !shouldDispatchExploreNodesIndividually(window) {
		t.Fatal("two evidence siblings must trip per-node dispatch")
	}
}

// TestShouldDispatchExploreNodesIndividually_PositiveThreeEvidenceSiblings
// confirms the N=3 case (the forensic 01:44/08:14 case from the
// commit-message anchor) also fires.
func TestShouldDispatchExploreNodesIndividually_PositiveThreeEvidenceSiblings(t *testing.T) {
	window := []*types.TaskNode{
		{ID: "n1_evidence_t0", Type: types.NodeEvidence},
		{ID: "n1_evidence_t1", Type: types.NodeEvidence},
		{ID: "n1_evidence_t2", Type: types.NodeEvidence},
	}
	if !shouldDispatchExploreNodesIndividually(window) {
		t.Fatal("three evidence siblings must trip per-node dispatch")
	}
}

// TestShouldDispatchExploreNodesIndividually_SingleNodeSkipped pins
// the byte-equivalent fast-path: single-sub_topic questions (len ==
// 1) go through the original merged-window code path completely
// unchanged.
func TestShouldDispatchExploreNodesIndividually_SingleNodeSkipped(t *testing.T) {
	window := []*types.TaskNode{
		{ID: "n1_evidence", Type: types.NodeEvidence},
	}
	if shouldDispatchExploreNodesIndividually(window) {
		t.Fatal("single evidence node must NOT trip per-node dispatch (byte-equivalent path)")
	}
}

// TestShouldDispatchExploreNodesIndividually_EmptyWindowSkipped pins
// nil-safety.
func TestShouldDispatchExploreNodesIndividually_EmptyWindowSkipped(t *testing.T) {
	if shouldDispatchExploreNodesIndividually(nil) {
		t.Fatal("nil window must NOT trip per-node dispatch")
	}
	if shouldDispatchExploreNodesIndividually([]*types.TaskNode{}) {
		t.Fatal("empty window must NOT trip per-node dispatch")
	}
}

// TestShouldDispatchExploreNodesIndividually_SingleEvidencePlusCompanionsSkipped
// pins that a window with only ONE evidence node — regardless of how
// many non-evidence companions are present — does NOT trip per-node
// dispatch. The companions stay merged with the single evidence
// node as before E'.
func TestShouldDispatchExploreNodesIndividually_SingleEvidencePlusCompanionsSkipped(t *testing.T) {
	cases := []struct {
		name string
		w    []*types.TaskNode
	}{
		{
			"evidence_plus_probe",
			[]*types.TaskNode{
				{ID: "n1_evidence_t0", Type: types.NodeEvidence},
				{ID: "n1_probe", Type: types.NodeProbe},
			},
		},
		{
			"evidence_plus_validate",
			[]*types.TaskNode{
				{ID: "n1_evidence_t0", Type: types.NodeEvidence},
				{ID: "n1_validate", Type: types.NodeValidate},
			},
		},
		{
			"evidence_plus_reconcile",
			[]*types.TaskNode{
				{ID: "n1_evidence_t0", Type: types.NodeEvidence},
				{ID: "n1_reconcile", Type: types.NodeReconcile},
			},
		},
		{
			"all_validate_no_evidence",
			[]*types.TaskNode{
				{ID: "v1", Type: types.NodeValidate},
				{ID: "v2", Type: types.NodeValidate},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if shouldDispatchExploreNodesIndividually(c.w) {
				t.Errorf("case %q must NOT trip per-node dispatch", c.name)
			}
		})
	}
}

// TestShouldDispatchExploreNodesIndividually_ProductionShapeWithCompanions
// pins the LOAD-BEARING case: production-shape windows
// (`probe + N evidence siblings + validate`) MUST trip per-node
// dispatch even though probe/validate are present. The trim helper
// then keeps probe + first evidence + validate while subsequent
// scheduler iterations pick up the remaining siblings.
//
// This is the byte-precise forensic-anchor test for the 08:14 run.
func TestShouldDispatchExploreNodesIndividually_ProductionShapeWithCompanions(t *testing.T) {
	window := []*types.TaskNode{
		{ID: "n0_probe", Type: types.NodeProbe},
		{ID: "n1_evidence_t0", Type: types.NodeEvidence},
		{ID: "n1_evidence_t1", Type: types.NodeEvidence},
		{ID: "n2_validate", Type: types.NodeValidate},
	}
	if !shouldDispatchExploreNodesIndividually(window) {
		t.Fatal("production shape (probe + 2 evidence siblings + validate) MUST trip per-node dispatch")
	}
}

func TestShouldKeepSourceInventoryExploreWindowUnified(t *testing.T) {
	window := []*types.TaskNode{
		{ID: "n1_evidence_t0", Type: types.NodeEvidence},
		{ID: "n1_evidence_t1", Type: types.NodeEvidence},
	}
	ctx := &types.BusContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		SourceInventoryProfile: &types.SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles: []types.AnswerCandidateRole{
				types.AnswerCandidateRolePackage,
				types.AnswerCandidateRoleFunction,
			},
			Confidence: 0.9,
		},
	}}}

	if !shouldDispatchExploreNodesIndividually(window) {
		t.Fatal("fixture should otherwise be splittable")
	}
	if !shouldKeepSourceInventoryExploreWindowUnified(ctx, window) {
		t.Fatal("source-inventory dimensions should share one explore dispatch")
	}
	ctx.AnalysisIR.RequestModel.SourceInventoryProfile = nil
	if shouldKeepSourceInventoryExploreWindowUnified(ctx, window) {
		t.Fatal("ordinary multi-evidence windows should keep existing split behavior")
	}
}

func TestShouldKeepSourceInventoryExploreWindowUnified_BoundedEnumerationFallback(t *testing.T) {
	window := []*types.TaskNode{
		{ID: "n1_evidence_t0", Type: types.NodeEvidence},
		{ID: "n1_evidence_t1", Type: types.NodeEvidence},
	}
	ctx := &types.BusContext{AnalysisIR: &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
		},
		EvidencePlan: types.EvidencePlan{RequiredFiles: []string{
			"internal/analysis/aggregator/aggregator.go",
			"internal/analysis/amplifier/amplifier.go",
			"internal/analysis/binder/binder.go",
			"internal/analysis/budget/budget.go",
			"internal/analysis/compiler/compile.go",
			"internal/analysis/gate/gate.go",
		}},
	}}

	if !shouldDispatchExploreNodesIndividually(window) {
		t.Fatal("fixture should otherwise be splittable")
	}
	if !shouldKeepSourceInventoryExploreWindowUnified(ctx, window) {
		t.Fatal("bounded source enumeration should stay in one explore dispatch even without source_inventory_profile")
	}
}

func TestShouldKeepSourceInventoryExploreWindowUnified_BoundedEnumerationFallbackConservative(t *testing.T) {
	window := []*types.TaskNode{
		{ID: "n1_evidence_t0", Type: types.NodeEvidence},
		{ID: "n1_evidence_t1", Type: types.NodeEvidence},
	}
	cases := []struct {
		name string
		ir   *types.AnalysisIR
	}{
		{
			name: "non_enumeration",
			ir: &types.AnalysisIR{
				RequestModel: types.RequestModel{
					Intent: types.IntentExplain,
					Predicates: types.SemanticPredicates{
						IsCategoryEnumeration: true,
					},
				},
				EvidencePlan: types.EvidencePlan{RequiredFiles: []string{
					"internal/analysis/a/a.go",
					"internal/analysis/b/b.go",
					"internal/analysis/c/c.go",
					"internal/analysis/d/d.go",
					"internal/analysis/e/e.go",
					"internal/analysis/f/f.go",
				}},
			},
		},
		{
			name: "unbounded_roots",
			ir: &types.AnalysisIR{
				RequestModel: types.RequestModel{
					Intent: types.IntentEnumerate,
					Predicates: types.SemanticPredicates{
						IsCategoryEnumeration: true,
					},
				},
				EvidencePlan: types.EvidencePlan{RequiredFiles: []string{
					"a.go",
					"b.go",
					"c.go",
					"d.go",
					"e.go",
					"f.go",
				}},
			},
		},
		{
			name: "auxiliary_out_of_scope",
			ir: &types.AnalysisIR{
				RequestModel: types.RequestModel{
					Intent: types.IntentEnumerate,
					Predicates: types.SemanticPredicates{
						IsCategoryEnumeration: true,
					},
				},
				EvidencePlan: types.EvidencePlan{RequiredFiles: []string{
					"docs/analysis/a.md",
					"docs/analysis/b.md",
					"docs/analysis/c.md",
					"docs/analysis/d.md",
					"docs/analysis/e.md",
					"docs/analysis/f.md",
				}},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &types.BusContext{AnalysisIR: tc.ir}
			if shouldKeepSourceInventoryExploreWindowUnified(ctx, window) {
				t.Fatalf("%s should not activate the bounded source-enumeration fallback", tc.name)
			}
		})
	}
}

func TestExploreWindowDispatchGroups_ProfileOmittedBoundedEnumerationStaysUnified(t *testing.T) {
	t0 := &types.TaskNode{ID: "n1_evidence_t0", Type: types.NodeEvidence}
	t1 := &types.TaskNode{ID: "n1_evidence_t1", Type: types.NodeEvidence}
	window := []*types.TaskNode{t0, t1}
	ctx := &types.BusContext{AnalysisIR: &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
		},
		EvidencePlan: types.EvidencePlan{RequiredFiles: []string{
			"internal/analysis/aggregator/aggregator.go",
			"internal/analysis/amplifier/amplifier.go",
			"internal/analysis/binder/binder.go",
			"internal/analysis/budget/budget.go",
			"internal/analysis/compiler/compile.go",
			"internal/analysis/gate/gate.go",
		}},
	}}

	groups := exploreWindowDispatchGroups(ctx, window)
	if len(groups) != 1 {
		t.Fatalf("bounded source enumeration without source_inventory_profile must stay unified; got %d groups", len(groups))
	}
	if len(groups[0]) != 2 || groups[0][0] != t0 || groups[0][1] != t1 {
		t.Fatalf("unified dispatch should preserve the original evidence order, got %+v", groups[0])
	}
}

func TestExploreWindowDispatchGroups_OrdinaryMultiTopicStillSplits(t *testing.T) {
	t0 := &types.TaskNode{ID: "n1_evidence_t0", Type: types.NodeEvidence}
	t1 := &types.TaskNode{ID: "n1_evidence_t1", Type: types.NodeEvidence}
	groups := exploreWindowDispatchGroups(&types.BusContext{AnalysisIR: &types.AnalysisIR{
		RequestModel: types.RequestModel{Intent: types.IntentExplain},
	}}, []*types.TaskNode{t0, t1})

	if len(groups) != 2 {
		t.Fatalf("ordinary independent evidence siblings should still split; got %d groups", len(groups))
	}
	if len(groups[0]) != 1 || groups[0][0] != t0 || len(groups[1]) != 1 || groups[1][0] != t1 {
		t.Fatalf("split groups should preserve declaration order, got %+v", groups)
	}
}

func TestExploreWindowDispatchGroups_RuntimeCurrentSharedContextStaysUnified(t *testing.T) {
	t0 := &types.TaskNode{ID: "n1_evidence_t0", Type: types.NodeEvidence}
	t1 := &types.TaskNode{ID: "n1_evidence_t1", Type: types.NodeEvidence}
	ctx := &types.BusContext{AnalysisIR: &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Intent: types.IntentExplain,
			LogTriage: &types.LogBundle{
				Observations: []types.LogObservation{{
					Kind:       types.LogObservationRetryCycle,
					Subject:    "finalizer timeout",
					Summary:    "finalizer timed out before first byte",
					Diagnostic: true,
					Confidence: 0.9,
				}},
			},
			CurrentSourceExplanationProfile: &types.CurrentSourceExplanationProfile{
				IsCurrentSourceExplanationRequested: true,
				SourceQuotes: []string{
					"finalizer timeout",
				},
				Modes: []types.CurrentSourceExplanationMode{
					types.CurrentSourceExplanationExplainCurrentMechanism,
				},
				Confidence: 0.9,
			},
			SubTopics: []types.SubTopic{
				{Summary: "运行时现象", Entities: []string{"timeout"}},
				{Summary: "当前源码机制", Entities: []string{"finalizer"}},
			},
		},
	}}

	groups := exploreWindowDispatchGroups(ctx, []*types.TaskNode{t0, t1})
	if len(groups) != 1 {
		t.Fatalf("runtime artifact + current-source shared context should stay unified; got %d groups", len(groups))
	}
	if len(groups[0]) != 2 || groups[0][0] != t0 || groups[0][1] != t1 {
		t.Fatalf("unified dispatch should preserve evidence order, got %+v", groups[0])
	}
}

func TestExploreWindowDispatchGroups_UserBucketsStillSplitDespiteSharedSignals(t *testing.T) {
	t0 := &types.TaskNode{ID: "n1_evidence_t0", Type: types.NodeEvidence}
	t1 := &types.TaskNode{ID: "n1_evidence_t1", Type: types.NodeEvidence}
	ctx := &types.BusContext{AnalysisIR: &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Intent: types.IntentExplain,
			Predicates: types.SemanticPredicates{
				IsCrossComponent: true,
			},
			Buckets: []types.QuestionBucket{
				{Label: "codrax", Index: 1},
				{Label: "opencode", Index: 2},
			},
		},
	}}

	groups := exploreWindowDispatchGroups(ctx, []*types.TaskNode{t0, t1})
	if len(groups) != 2 {
		t.Fatalf("explicit user buckets must remain separately dispatchable, got %d groups", len(groups))
	}
}

// TestShouldDispatchExploreNodesIndividually_NilEntrySkipped pins the
// defensive nil-entry check (the window slice is built by the
// scheduler and SHOULD have no nil entries, but a future bug in
// readyExplorerWindowContext should not silently fan out).
func TestShouldDispatchExploreNodesIndividually_NilEntrySkipped(t *testing.T) {
	window := []*types.TaskNode{
		{ID: "n1_evidence_t0", Type: types.NodeEvidence},
		nil,
	}
	if shouldDispatchExploreNodesIndividually(window) {
		t.Fatal("nil entry in window must NOT trip per-node dispatch")
	}
}

func TestSourceInventoryLensFirstWindowPrioritizesTypedLens(t *testing.T) {
	lens := &types.TaskNode{
		ID:       "n_source_inventory_lens",
		Type:     types.NodeProbe,
		Optional: true,
		EntryConditions: []types.Criterion{{
			Kind: types.CritSourceInventoryLensMissing,
		}},
	}
	evidence := &types.TaskNode{ID: "n_evidence", Type: types.NodeEvidence}
	validate := &types.TaskNode{ID: "n_validate", Type: types.NodeValidate}

	got := sourceInventoryLensFirstWindow([]*types.TaskNode{evidence, lens, validate}, true, false)
	if len(got) != 1 || got[0] != lens {
		t.Fatalf("active missing lens must dispatch lens-only first, got %+v", idsOf(got))
	}
	if !sourceInventoryLensProbeOnlyWindow(got) {
		t.Fatalf("lens-only helper must identify prioritized source-inventory lens window: %+v", idsOf(got))
	}
	if surface := exploreToolSurfaceForWindow(got); surface != types.ExploreToolSurfaceSourceInventoryLens {
		t.Fatalf("lens-only window must select source-inventory tool surface, got %q", surface)
	}
}

func TestSourceInventoryLensFirstWindowNoopsWhenInactiveOrExecuted(t *testing.T) {
	lens := &types.TaskNode{
		ID:       "n_source_inventory_lens",
		Type:     types.NodeProbe,
		Optional: true,
		EntryConditions: []types.Criterion{{
			Kind: types.CritSourceInventoryLensMissing,
		}},
	}
	evidence := &types.TaskNode{ID: "n_evidence", Type: types.NodeEvidence}
	window := []*types.TaskNode{evidence, lens}
	if got := sourceInventoryLensFirstWindow(window, false, false); len(got) != 0 {
		t.Fatalf("inactive source inventory must not reorder window: %+v", idsOf(got))
	}
	if got := sourceInventoryLensFirstWindow(window, true, true); len(got) != 0 {
		t.Fatalf("already executed source inventory lens must not reorder window: %+v", idsOf(got))
	}
}

func TestSourceInventoryLensFirstWindowDoesNotTreatAnalyzePrescanAsExecuted(t *testing.T) {
	lens := &types.TaskNode{
		ID:       "n_source_inventory_lens",
		Type:     types.NodeProbe,
		Optional: true,
		EntryConditions: []types.Criterion{{
			Kind: types.CritSourceInventoryLensMissing,
		}},
	}
	evidence := &types.TaskNode{ID: "n_evidence", Type: types.NodeEvidence}
	analyzePrescan := types.SourceInventoryObservation{
		Active: true,
		Provenance: []string{
			types.SourceInventoryProvenanceRepoLensToolQuery,
			types.SourceInventoryProvenanceStageAnalyze,
		},
		Lens: []string{"members", "count"},
		Sets: []types.SourceInventoryObservationSet{{
			Role:    types.AnswerCandidateRoleType,
			Members: []types.SourceInventoryObservationMember{{Name: "Agent"}},
		}},
	}

	got := sourceInventoryLensFirstWindow([]*types.TaskNode{evidence, lens}, true, types.SourceInventoryLensExecuted(analyzePrescan))
	if len(got) != 1 || got[0] != lens {
		t.Fatalf("analyze-stage prescan must not suppress explore source-inventory lens probe: %+v", idsOf(got))
	}
}

func TestSourceInventoryLensFirstWindowDoesNotTreatListFilesSupportAsExecuted(t *testing.T) {
	lens := &types.TaskNode{
		ID:       "n_source_inventory_lens",
		Type:     types.NodeProbe,
		Optional: true,
		EntryConditions: []types.Criterion{{
			Kind: types.CritSourceInventoryLensMissing,
		}},
	}
	evidence := &types.TaskNode{ID: "n_evidence", Type: types.NodeEvidence}
	listFilesSupport := types.SourceInventoryObservation{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     false,
		Provenance:   []string{"tool:list_files:direct", "repo_lens:query_roles"},
		Lens:         []string{"members", "count"},
		Sets: []types.SourceInventoryObservationSet{{
			Role:     types.AnswerCandidateRoleType,
			Complete: false,
			Count:    1,
			Members: []types.SourceInventoryObservationMember{{
				Name:       "Greeter",
				Role:       types.AnswerCandidateRoleType,
				File:       "corpus/Greeter.cj",
				Line:       6,
				Provenance: []string{"tool:list_files:direct"},
			}},
		}},
	}

	got := sourceInventoryLensFirstWindow([]*types.TaskNode{evidence, lens}, true, types.SourceInventoryLensExecuted(listFilesSupport))
	if len(got) != 1 || got[0] != lens {
		t.Fatalf("list_files support inventory must not suppress executable source_inventory lens probe: %+v", idsOf(got))
	}
}

func TestSourceInventoryFollowupFirstWindowPrioritizesTypedDebt(t *testing.T) {
	followup := &types.TaskNode{
		ID:       "n_source_inventory_followup",
		Type:     types.NodeProbe,
		Optional: true,
		EntryConditions: []types.Criterion{{
			Kind: types.CritSourceInventoryFollowupDebt,
		}},
	}
	evidence := &types.TaskNode{ID: "n_evidence", Type: types.NodeEvidence}
	validate := &types.TaskNode{ID: "n_validate", Type: types.NodeValidate}

	got := sourceInventoryFollowupFirstWindow([]*types.TaskNode{evidence, followup, validate}, true)
	if len(got) != 1 || got[0] != followup {
		t.Fatalf("active typed debt must dispatch follow-up only first, got %+v", idsOf(got))
	}
	if !sourceInventoryFollowupProbeOnlyWindow(got) {
		t.Fatalf("follow-up-only helper must identify prioritized source-inventory follow-up window: %+v", idsOf(got))
	}
	if surface := exploreToolSurfaceForWindow(got); surface != types.ExploreToolSurfaceSourceInventoryFollowup {
		t.Fatalf("follow-up window must select source-inventory follow-up tool surface, got %q", surface)
	}
	if got := sourceInventoryFollowupFirstWindow([]*types.TaskNode{evidence, followup}, false); len(got) != 0 {
		t.Fatalf("inactive typed debt must not reorder window: %+v", idsOf(got))
	}
}

func TestPrioritizeSourceInventoryLensWindow_SuppressesFollowupAfterCompletionCaveat(t *testing.T) {
	followup := &types.TaskNode{
		ID:       "n_source_inventory_followup",
		Type:     types.NodeProbe,
		Optional: true,
		EntryConditions: []types.Criterion{{
			Kind: types.CritSourceInventoryFollowupDebt,
		}},
	}
	evidence := &types.TaskNode{ID: "n_evidence", Type: types.NodeEvidence}
	mut := types.NewMutableState("q")
	mut.EvidenceClosure().AppendCompletionCaveat(types.CompletionCaveat{
		Lane:       types.DowngradeLaneSourceInventoryCompletion,
		ReasonCode: types.ProgressReasonConverged,
	})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut}}

	got := o.prioritizeSourceInventoryLensWindow([]*types.TaskNode{evidence, followup}, criterion.Env{
		SourceInventoryFollowupDebt: types.NormalizeSourceInventoryFollowupDebt(types.SourceInventoryFollowupDebt{
			Active:     true,
			ReasonCode: types.SourceInventoryFollowupDebtPagination,
			Query: types.SourceInventoryLensQuery{
				Path:          ".",
				Scopes:        []string{"."},
				Roles:         []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
				IncludeCounts: true,
			},
			Roles: []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
		}),
	})
	if len(got) != 1 || got[0] != evidence {
		t.Fatalf("completion caveat should filter follow-up instead of prioritizing it: %+v", idsOf(got))
	}
	if o.busCtx.SourceInventoryFollowupDebt.IsActive() {
		t.Fatalf("completion-caveated source-inventory follow-up debt should be cleared: %+v", o.busCtx.SourceInventoryFollowupDebt)
	}
}

// TestTrimExploreWindowToFirstEvidence_DropsExtraSiblingsKeepsCompanions
// pins the trim helper's load-bearing contract on production-shape
// windows: companions (probe / validate) survive; only sibling
// evidence beyond the first is dropped.
func TestTrimExploreWindowToFirstEvidence_DropsExtraSiblingsKeepsCompanions(t *testing.T) {
	probe := &types.TaskNode{ID: "n0_probe", Type: types.NodeProbe}
	t0 := &types.TaskNode{ID: "n1_evidence_t0", Type: types.NodeEvidence}
	t1 := &types.TaskNode{ID: "n1_evidence_t1", Type: types.NodeEvidence}
	t2 := &types.TaskNode{ID: "n1_evidence_t2", Type: types.NodeEvidence}
	validate := &types.TaskNode{ID: "n2_validate", Type: types.NodeValidate}
	window := []*types.TaskNode{probe, t0, t1, t2, validate}

	trimmed := trimExploreWindowToFirstEvidence(window)
	if len(trimmed) != 3 {
		t.Fatalf("trimmed window length = %d, want 3 (probe + t0 + validate); got: %+v", len(trimmed), trimmed)
	}
	if trimmed[0] != probe {
		t.Errorf("trimmed[0] must be probe (preserved companion); got %s", trimmed[0].ID)
	}
	if trimmed[1] != t0 {
		t.Errorf("trimmed[1] must be the FIRST evidence (t0); got %s", trimmed[1].ID)
	}
	if trimmed[2] != validate {
		t.Errorf("trimmed[2] must be validate (preserved companion); got %s", trimmed[2].ID)
	}
	for _, n := range trimmed {
		if n != nil && n.ID == "n1_evidence_t1" {
			t.Errorf("trimmed window must NOT contain dropped sibling t1")
		}
		if n != nil && n.ID == "n1_evidence_t2" {
			t.Errorf("trimmed window must NOT contain dropped sibling t2")
		}
	}
}

// TestTrimExploreWindowToFirstEvidence_NoEvidenceLeavesWindowUnchanged
// pins that windows with no evidence siblings pass through verbatim.
func TestTrimExploreWindowToFirstEvidence_NoEvidenceLeavesWindowUnchanged(t *testing.T) {
	probe := &types.TaskNode{ID: "n0_probe", Type: types.NodeProbe}
	validate := &types.TaskNode{ID: "n2_validate", Type: types.NodeValidate}
	window := []*types.TaskNode{probe, validate}

	trimmed := trimExploreWindowToFirstEvidence(window)
	if len(trimmed) != 2 {
		t.Errorf("no-evidence window must pass through; got len=%d", len(trimmed))
	}
}

// TestTrimExploreWindowToFirstEvidence_SingleEvidenceLeavesWindowUnchanged
// pins that single-evidence windows are unchanged (byte-equivalent
// to the pre-E' merged-dispatch path).
func TestTrimExploreWindowToFirstEvidence_SingleEvidenceLeavesWindowUnchanged(t *testing.T) {
	probe := &types.TaskNode{ID: "n0_probe", Type: types.NodeProbe}
	t0 := &types.TaskNode{ID: "n1_evidence_t0", Type: types.NodeEvidence}
	validate := &types.TaskNode{ID: "n2_validate", Type: types.NodeValidate}
	window := []*types.TaskNode{probe, t0, validate}

	trimmed := trimExploreWindowToFirstEvidence(window)
	if len(trimmed) != 3 {
		t.Errorf("single-evidence window must pass through; got len=%d", len(trimmed))
	}
}

// TestTrimExploreWindowToFirstEvidence_EmptyReturnsEmpty pins nil-safety.
func TestTrimExploreWindowToFirstEvidence_EmptyReturnsEmpty(t *testing.T) {
	if got := trimExploreWindowToFirstEvidence(nil); len(got) != 0 {
		t.Errorf("nil window must trim to empty; got len=%d", len(got))
	}
	if got := trimExploreWindowToFirstEvidence([]*types.TaskNode{}); len(got) != 0 {
		t.Errorf("empty window must trim to empty; got len=%d", len(got))
	}
}

// ============================================================
// E2E: TaskGraph with N evidence siblings → root probe + N evidence dispatches
// ============================================================

// dagIRMultiTopic builds an AnalysisIR with N independent evidence
// sibling nodes (`{prefix}_t{i}` shape) that mirror the production
// expandEvidenceNodes output for multi-sub_topic questions.
func dagIRMultiTopic(siblingCount int) *types.AnalysisIR {
	if siblingCount < 2 {
		siblingCount = 2
	}
	nodes := []types.TaskNode{
		{ID: "n0", Type: types.NodeProbe, Objective: "scan repo",
			SearchHints: types.SearchHints{KeywordIDs: []string{"k1"}}},
	}
	edges := []types.TaskEdge{}
	// N evidence siblings, each `n1_evidence_tI`, no sibling-sibling
	// edge (so shouldDispatchExploreNodesIndividually fires).
	for i := 0; i < siblingCount; i++ {
		nodeID := "n1_evidence_t" + string(rune('0'+i))
		nodes = append(nodes, types.TaskNode{
			ID:          nodeID,
			Type:        types.NodeEvidence,
			Objective:   "evidence for sub-topic " + string(rune('0'+i)),
			SearchHints: types.SearchHints{EntityIDs: []string{"e" + string(rune('0'+i))}},
		})
		edges = append(edges,
			types.TaskEdge{From: "n0", To: nodeID, EdgeType: types.EdgeHardDependency},
			types.TaskEdge{From: nodeID, To: "n2_validate", EdgeType: types.EdgeHardDependency},
		)
	}
	nodes = append(nodes,
		types.TaskNode{ID: "n2_validate", Type: types.NodeValidate, Objective: "check chains"},
		types.TaskNode{ID: "n3", Type: types.NodeFinalize, Objective: "render answer"},
	)
	edges = append(edges,
		types.TaskEdge{From: "n2_validate", To: "n3", EdgeType: types.EdgeHardDependency},
	)
	critPath := []string{"n0", "n1_evidence_t0", "n2_validate", "n3"}
	subTopics := make([]types.SubTopic, 0, siblingCount)
	for i := 0; i < siblingCount; i++ {
		subTopics = append(subTopics, types.SubTopic{
			Summary:  "sub-topic " + string(rune('0'+i)),
			Entities: []string{"e" + string(rune('0'+i))},
		})
	}
	return &types.AnalysisIR{
		Version: types.AnalysisIRVersion,
		RequestModel: types.RequestModel{
			Language:  "en",
			Intent:    types.IntentExplain,
			SubTopics: subTopics,
		},
		TaskGraph: types.TaskGraph{
			Nodes:           nodes,
			Edges:           edges,
			ExecutionPolicy: types.ExecutionPolicy{MaxParallelism: 1, RetryBudget: 1, CriticalPath: critPath},
		},
		EvidencePlan: types.EvidencePlan{
			Budget: types.EvidenceBudget{MaxFiles: 30, MaxBytes: 200000, MaxReactIters: 10, MaxToolCalls: 40},
		},
		AnswerContract: types.AnswerContract{Language: "en"},
	}
}

// TestRunTaskGraph_PerNodeDispatch_TwoEvidenceSiblings is the E'
// load-bearing e2e test: when the analyzer produces 2 independent
// evidence sibling nodes, the scheduler dispatches the root probe, then
// each evidence sibling independently, then validation.
//
// Forensic anchor: this is the byte-precise regression test for the
// 08:14 "compare codrax and opencode" run where one explorer instance
// chewed through both sub-topics serially for 7+ minutes. After E',
// the scheduler dispatches each sibling independently so each
// dispatch's hint is focused on a single sub-topic.
func TestRunTaskGraph_PerNodeDispatch_TwoEvidenceSiblings(t *testing.T) {
	var explorerCalls int
	var observedExplorerHints []string
	var mu sync.Mutex

	ir := dagIRMultiTopic(2)

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			mu.Lock()
			explorerCalls++
			observedExplorerHints = append(observedExplorerHints, ctx.RetryHint)
			mu.Unlock()
			return &agent.StageOutput{
				MissingPiece:  types.MissingNone,
				SignalUpdates: &types.ExecutionSignals{HasEnoughFacts: true},
				EvidenceItems: []types.EvidenceItem{{ID: "ev", Source: "src.go", LineStart: 1}},
			}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "- `Foo` (file.go:1)",
			}, nil
		},
	}

	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)

	_, err := o.Run("compare A and B", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if explorerCalls != 3 {
		t.Errorf("expected 3 explorer dispatches after graph fan-out policy (probe/evidence windows; validate may auto-complete), got %d", explorerCalls)
	}
	if len(observedExplorerHints) != 3 {
		t.Fatalf("expected 3 hints, got %d", len(observedExplorerHints))
	}

	// E': evidence hints should focus on ONE sub-topic. Parallel dispatch may
	// invoke the stub in either goroutine order, so assert the set shape
	// instead of relying on append order.
	var saw0, saw1 bool
	for _, hint := range observedExplorerHints {
		has0 := strings.Contains(hint, "sub-topic 0")
		has1 := strings.Contains(hint, "sub-topic 1")
		if !has0 && !has1 {
			continue
		}
		if has0 && has1 {
			t.Errorf("focused hint must not contain both sub-topics; got %q", hint)
		}
		if has0 {
			saw0 = true
		}
		if has1 {
			saw1 = true
		}
	}
	if !saw0 || !saw1 {
		t.Errorf("expected one focused hint per sub-topic, got %q", observedExplorerHints)
	}
}

// TestRunTaskGraph_SingleEvidenceNode_ByteEquivalent pins the single-subtopic
// hard-dependency path: probe, evidence, and validate each dispatch once.
func TestRunTaskGraph_SingleEvidenceNode_ByteEquivalent(t *testing.T) {
	var explorerCalls int

	ir := dagIR(types.AnswerContract{Language: "en"})

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			explorerCalls++
			return &agent.StageOutput{
				MissingPiece:  types.MissingNone,
				SignalUpdates: &types.ExecutionSignals{HasEnoughFacts: true},
				EvidenceItems: []types.EvidenceItem{{ID: "ev", Source: "src.go", LineStart: 1}},
			}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "- `Foo` (file.go:1)",
			}, nil
		},
	}

	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)

	_, err := o.Run("explain X", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if explorerCalls != 3 {
		t.Errorf("single-sub_topic: expected 3 hard-dependency explorer dispatches, got %d", explorerCalls)
	}
}

func TestRunTaskGraph_ParallelDispatch_UsesConfiguredCap(t *testing.T) {
	ir := dagIRMultiTopic(3)
	ir.TaskGraph.ExecutionPolicy.MaxParallelism = 3
	var inFlight int32
	var maxInFlight int32
	var calls int32
	var missingParallelGroup int32
	var evMu sync.Mutex
	var parallelEvents []render.EventKind

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			if strings.Contains(ctx.ExploreDispatchKey, "n1_evidence") && (ctx.ParallelGroupID == "" || ctx.ExploreDispatchKey == "") {
				atomic.AddInt32(&missingParallelGroup, 1)
			}
			cur := atomic.AddInt32(&inFlight, 1)
			for {
				prev := atomic.LoadInt32(&maxInFlight)
				if cur <= prev || atomic.CompareAndSwapInt32(&maxInFlight, prev, cur) {
					break
				}
			}
			atomic.AddInt32(&calls, 1)
			time.Sleep(40 * time.Millisecond)
			atomic.AddInt32(&inFlight, -1)
			return &agent.StageOutput{
				MissingPiece:  types.MissingNone,
				SignalUpdates: &types.ExecutionSignals{HasEnoughFacts: true},
				EvidenceItems: []types.EvidenceItem{{ID: "ev", Source: "src.go", LineStart: 1}},
			}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingNone, FinalAnswer: "- `Foo` (file.go:1)"}, nil
		},
	}

	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{MaxParallelism: 2}, ar, sr, sar)
	o.SetMaxSteps(30)
	o.SetEmitter(func(ev render.Event) {
		switch ev.Kind {
		case render.EventParallelDispatchStart,
			render.EventParallelDispatchUnitStart,
			render.EventParallelDispatchUnitEnd,
			render.EventParallelDispatchEnd:
			evMu.Lock()
			parallelEvents = append(parallelEvents, ev.Kind)
			evMu.Unlock()
		}
	})
	if _, err := o.Run("compare A, B, and C", "/tmp/repo", "main"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 5 {
		t.Fatalf("explorer calls = %d, want 5 (probe + 3 evidence + validate)", got)
	}
	if got := atomic.LoadInt32(&maxInFlight); got != 2 {
		t.Fatalf("max concurrent explorer dispatches = %d, want configured cap 2", got)
	}
	if got := atomic.LoadInt32(&missingParallelGroup); got != 0 {
		t.Fatalf("parallel explorer contexts missing group/unit telemetry: %d", got)
	}
	evMu.Lock()
	defer evMu.Unlock()
	counts := map[render.EventKind]int{}
	for _, k := range parallelEvents {
		counts[k]++
	}
	if counts[render.EventParallelDispatchStart] != 1 || counts[render.EventParallelDispatchEnd] != 1 {
		t.Fatalf("parallel dispatch must emit one start/end event; got counts=%v", counts)
	}
	if counts[render.EventParallelDispatchUnitStart] != 3 || counts[render.EventParallelDispatchUnitEnd] != 3 {
		t.Fatalf("parallel dispatch must emit per-unit start/end events; got counts=%v", counts)
	}
}

func TestRunTaskGraph_ParallelDispatch_CanBeForcedSerial(t *testing.T) {
	ir := dagIRMultiTopic(3)
	var inFlight int32
	var maxInFlight int32
	var mu sync.Mutex
	var dispatchKeys []string

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			mu.Lock()
			dispatchKeys = append(dispatchKeys, ctx.ExploreDispatchKey)
			mu.Unlock()
			cur := atomic.AddInt32(&inFlight, 1)
			if cur > atomic.LoadInt32(&maxInFlight) {
				atomic.StoreInt32(&maxInFlight, cur)
			}
			time.Sleep(10 * time.Millisecond)
			atomic.AddInt32(&inFlight, -1)
			evidenceID := "ev-" + strings.ReplaceAll(ctx.ExploreDispatchKey, " ", "_")
			return &agent.StageOutput{
				MissingPiece:  types.MissingNone,
				EvidenceItems: []types.EvidenceItem{{ID: evidenceID, Source: "src.go", LineStart: 1}},
			}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingNone, FinalAnswer: "- `Foo` (file.go:1)"}, nil
		},
	}

	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{MaxParallelism: 1}, ar, sr, sar)
	o.SetMaxSteps(30)
	if _, err := o.Run("compare A, B, and C", "/tmp/repo", "main"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := atomic.LoadInt32(&maxInFlight); got != 1 {
		t.Fatalf("max concurrent explorer dispatches = %d, want serial cap 1", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(dispatchKeys) != 5 {
		t.Fatalf("dispatch keys = %v, want 5 hard-dependency keys", dispatchKeys)
	}
	for _, want := range []string{"n1_evidence_t0", "n1_evidence_t1", "n1_evidence_t2"} {
		var found bool
		for _, key := range dispatchKeys {
			if strings.Contains(key, want) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("dispatch keys %v missing %s", dispatchKeys, want)
		}
	}
}
