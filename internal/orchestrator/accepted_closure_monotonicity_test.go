package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestShouldAutoCompleteExploreWindowFromAcceptedClosure_AllowsSupportOnlyDebt(t *testing.T) {
	mut := types.NewMutableState("accepted closure support-only debt")
	mut.SetInvestigationComplete("accepted closure already covers the answer")
	mut.EvidenceClosure().AddPendingRead(types.PendingRead{
		File:      "support.go",
		Origin:    "phase1_unread",
		Rationale: "top-K breadth support after closure",
	})
	mut.EvidenceClosure().AddRepair(types.RepairDirective{
		Kind:      types.RepairEmitEvidence,
		Origin:    "support.telemetry",
		Rationale: "advisory only",
		Advisory:  true,
	})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut}}

	if !o.shouldAutoCompleteExploreWindowFromAcceptedClosure(nil, "", "") {
		t.Fatal("accepted closure should auto-complete when only support/advisory debt remains")
	}
}

func TestShouldAutoCompleteExploreWindowFromAcceptedClosure_BlocksLoadBearingDebt(t *testing.T) {
	tests := []struct {
		name    string
		pending types.PendingRead
		repair  types.RepairDirective
	}{
		{
			name: "primary anchor",
			pending: types.PendingRead{
				File:      "anchor.go",
				Origin:    "pre_complete.primary_anchor",
				Rationale: "exact anchor remains unread",
			},
		},
		{
			name: "required file hint",
			pending: types.PendingRead{
				File:      "required.go",
				Origin:    "required_file_hint_unread",
				Rationale: "required current-source file remains unread",
			},
		},
		{
			name: "multi path anchor",
			pending: types.PendingRead{
				File:      "range.go",
				Origin:    "pre_complete.multi_path_anchor",
				Rationale: "surgical exact anchor remains unread",
			},
		},
		{
			name: "non advisory repair",
			repair: types.RepairDirective{
				Kind:      types.RepairEmitEvidence,
				Origin:    "pre_complete.principal_support_materialization",
				Rationale: "principal typed support missing",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mut := types.NewMutableState("accepted closure with load-bearing debt")
			mut.SetInvestigationComplete("accepted closure already covers the answer")
			if tt.pending.File != "" {
				mut.EvidenceClosure().AddPendingRead(tt.pending)
			}
			if tt.repair.Kind != "" {
				mut.EvidenceClosure().AddRepair(tt.repair)
			}
			o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut}}
			if o.shouldAutoCompleteExploreWindowFromAcceptedClosure(nil, "", "") {
				t.Fatalf("accepted closure must not auto-complete with load-bearing debt: pending=%+v repair=%+v", tt.pending, tt.repair)
			}
		})
	}
}

func TestAcceptedClosure_SourceInventorySubjectRebindDoesNotReopenReconcile(t *testing.T) {
	mut := types.NewMutableState("source inventory closure")
	mut.SetSourceInventoryObservation(types.SourceInventoryObservation{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     true,
		Scopes:       []string{"src"},
		Provenance:   []string{"tool:list_files:direct"},
		Lens:         []string{"direct_children", "count"},
		Sets: []types.SourceInventoryObservationSet{{
			Role:     types.AnswerCandidateRolePackage,
			Complete: true,
			Count:    2,
			Members: []types.SourceInventoryObservationMember{
				{
					Name:       "alpha",
					Key:        "src/alpha",
					File:       "src/alpha",
					Role:       types.AnswerCandidateRolePackage,
					Provenance: []string{"tool:list_files:direct"},
				},
				{
					Name:       "beta",
					Key:        "src/beta",
					File:       "src/beta",
					Role:       types.AnswerCandidateRolePackage,
					Provenance: []string{"tool:list_files:direct"},
				},
			},
		}},
	})
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "package -> entrypoint",
		Value:   "2",
		Members: []string{"alpha -> Start", "beta -> Build"},
	}})
	mut.SetInvestigationComplete("model-authored source inventory closure")
	mut.EvidenceClosure().AddRepair(types.RepairDirective{
		Kind:      types.RepairRebindSubject,
		Subject:   string(types.SubjectFunctionName),
		Origin:    "chain_ranker",
		Rationale: "attribute-role subject mismatch after complete package inventory",
	})
	ir := &types.AnalysisIR{
		RequestModel: types.RequestModel{
			SourceInventoryProfile: &types.SourceInventoryProfile{
				IsSourceInventory: true,
				TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
				Confidence:        0.9,
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectFunctionName, Confidence: 0.62},
		},
	}
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut, AnalysisIR: ir}}

	if o.hasBlockingReconcileRepair() {
		t.Fatal("complete exact source-inventory closure should demote attribute-role rebind to advisory for reconcile")
	}
	if !o.shouldAutoCompleteExploreWindowFromAcceptedClosure(nil, "", "") {
		t.Fatal("accepted closure should auto-complete despite source-inventory attribute subject rebind")
	}
	if !o.acceptedSourceInventoryClosureSuppressesReconcileFactRetry([]*types.TaskNode{{ID: "n_reconcile", Type: types.NodeReconcile}}) {
		t.Fatal("reconcile fact retry should be suppressed after complete exact source-inventory closure")
	}
	if o.acceptedSourceInventoryClosureSuppressesReconcileFactRetry([]*types.TaskNode{{ID: "n_evidence", Type: types.NodeEvidence}}) {
		t.Fatal("non-reconcile windows must not inherit the reconcile retry suppression")
	}
}
