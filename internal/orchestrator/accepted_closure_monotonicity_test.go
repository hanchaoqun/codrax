package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/analysis/criterion"
	"github.com/hanchaoqun/codrax/internal/render"
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

func TestShouldAutoCompleteExploreWindowFromAcceptedClosure_AllowsCompletionFormDebt(t *testing.T) {
	mut := types.NewMutableState("accepted closure with completion-form debt")
	mut.SetInvestigationComplete("accepted closure already covers the answer")
	mut.EvidenceClosure().AddRepair(types.RepairDirective{
		Kind:      types.RepairStructuredHandoff,
		Origin:    types.RepairOriginCompletionFormPrefix + "aggregate_facts",
		Subject:   "repair aggregate_facts shape",
		Rationale: "completion form needs local schema repair, not new evidence",
	})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut}}

	if !o.shouldAutoCompleteExploreWindowFromAcceptedClosure(nil, "", "") {
		t.Fatal("completion-form debt must not reopen exploration after accepted closure")
	}
}

func TestShouldAutoCompleteExploreWindowFromAcceptedClosure_AllowsConvergedDowngradeLaneDebt(t *testing.T) {
	mut := types.NewMutableState("accepted closure with converged principal handoff debt")
	mut.SetInvestigationComplete("completed with a typed caveat after low-delta convergence")
	mut.EvidenceClosure().AppendCompletionCaveat(types.CompletionCaveat{
		Lane:       types.DowngradeLanePrincipalMemberSetHandoff,
		ReasonCode: types.ProgressReasonConverged,
		Reason:     "same blocker recurred",
	})
	mut.EvidenceClosure().AddRepair(types.RepairDirective{
		Kind:          types.RepairStructuredHandoff,
		Origin:        "emit_investigation_complete.principal_member_set_handoff",
		Subject:       "principal_member_set_handoff:has_per_member_table",
		DowngradeLane: types.DowngradeLanePrincipalMemberSetHandoff,
		Rationale:     "principal handoff remained missing at convergence",
	})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut}}

	if !o.shouldAutoCompleteExploreWindowFromAcceptedClosure(nil, "", "") {
		t.Fatal("repair tied to an already converged downgrade lane must not reopen exploration")
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

func TestAcceptedClosure_ReconcileIgnoresChainPromotionPendingReadOnly(t *testing.T) {
	mut := types.NewMutableState("accepted closure with advisory chain debt")
	mut.SetInvestigationComplete("accepted closure already covers the answer")
	mut.EvidenceClosure().AddPendingRead(types.PendingRead{
		File:      "extra.go",
		Origin:    "chain_promotion.bridge_literal",
		Rationale: "advisory relation-chain enrichment after accepted closure",
	})
	o := &Orchestrator{busCtx: &types.BusContext{
		Mutable:       mut,
		EvidenceItems: []types.EvidenceItem{{ID: "ev", Source: "src.go", LineStart: 1}},
	}}
	reconcile := &types.TaskNode{
		ID:              "n_reconcile",
		Type:            types.NodeReconcile,
		EntryConditions: []types.Criterion{{Kind: types.CritHasEnoughFacts}},
	}
	if !o.shouldAutoCompleteReadyReconcileNode(reconcile, criterion.Env{}) {
		t.Fatal("reconcile-only node should treat chain-promotion debt as advisory after accepted closure")
	}
	if !o.shouldAutoCompleteExploreWindowFromAcceptedClosure(nil, "", "") {
		t.Fatal("normal explore auto-complete should also treat chain-promotion debt as advisory after accepted closure")
	}

	blocking := types.NewMutableState("accepted closure with load-bearing anchor debt")
	blocking.SetInvestigationComplete("accepted closure already covers the answer")
	blocking.EvidenceClosure().AddPendingRead(types.PendingRead{
		File:      "anchor.go",
		Origin:    "pre_complete.primary_anchor",
		Rationale: "primary anchor remains unread",
	})
	o.busCtx.Mutable = blocking
	if o.shouldAutoCompleteReadyReconcileNode(reconcile, criterion.Env{}) {
		t.Fatal("load-bearing primary-anchor debt must still block reconcile auto-complete")
	}
}

func TestAcceptedClosureSuppressesAdvisoryFactRetry(t *testing.T) {
	mut := types.NewMutableState("accepted closure with advisory retry")
	mut.SetInvestigationComplete("accepted closure already covers the answer")
	mut.ResetInvestigationComplete()
	mut.EvidenceClosure().AddPendingRead(types.PendingRead{
		File:       "support.go",
		Origin:     "chain_promotion.bridge_literal",
		Rationale:  "support-chain line after accepted closure",
		LineRanges: []types.LineRange{{Start: 10, End: 12}},
	})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut}}
	state := newGraphState(types.TaskGraph{ExecutionPolicy: types.ExecutionPolicy{RetryBudget: 1}})
	node := &types.TaskNode{ID: "n_evidence", Type: types.NodeEvidence}
	out := &agent.StageOutput{
		MissingPiece: types.MissingFacts,
		RetryHint:    "support-only retry should not reopen accepted closure",
	}

	if o.requeueExploreWindowForFactRetry(state, []*types.TaskNode{node}, out) {
		t.Fatal("accepted closure should suppress fact retry when only advisory support debt remains")
	}
	if state.retryUsed != 0 {
		t.Fatalf("suppressed fact retry must not consume retry budget, got %d", state.retryUsed)
	}
}

func TestEmitAcceptedClosureAdvisoryDebtSkippedNotice_OnceAndNotRetry(t *testing.T) {
	var events []render.Event
	o := &Orchestrator{
		busCtx: &types.BusContext{Language: "zh"},
		emit: func(ev render.Event) {
			events = append(events, ev)
		},
	}

	o.emitAcceptedClosureAdvisoryDebtSkippedNotice()
	o.emitAcceptedClosureAdvisoryDebtSkippedNotice()

	if len(events) != 1 {
		t.Fatalf("advisory debt skip notice should emit once, got %d event(s)", len(events))
	}
	got := events[0]
	if got.Kind != render.EventOrchestratorNotice {
		t.Fatalf("event kind = %v, want EventOrchestratorNotice", got.Kind)
	}
	if got.NoticeKind != render.NoticeInvestigationReady {
		t.Fatalf("notice kind = %v, want NoticeInvestigationReady", got.NoticeKind)
	}
	if got.NoticeKind == render.NoticeRetry {
		t.Fatal("advisory skip must not be rendered as a retry notice")
	}
	if got.Reasoning != softAdvisoryDebtSkippedMessage("zh") {
		t.Fatalf("reasoning = %q, want %q", got.Reasoning, softAdvisoryDebtSkippedMessage("zh"))
	}
}
