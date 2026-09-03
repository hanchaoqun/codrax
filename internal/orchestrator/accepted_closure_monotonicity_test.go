package orchestrator

import (
	"os"
	"reflect"
	"strings"
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

func TestShouldAutoCompleteExploreWindowFromAcceptedClosure_AllowsValidationFeedbackDebt(t *testing.T) {
	mut := types.NewMutableState("accepted closure with post-completion validation debt")
	mut.SetInvestigationComplete("accepted trace investigation already covers the answer")
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut}}

	if !o.shouldAutoCompleteExploreWindowFromAcceptedClosure([]string{"trace_validate_upstream"}, "", "") {
		t.Fatal("post-completion validation feedback must become a boundary note instead of reopening exploration")
	}
}

func TestRuntimeAcceptedClosureDowngradesSoftSourceFallbackOnly(t *testing.T) {
	mut := types.NewMutableState("accepted runtime closure")
	mut.SetInvestigationComplete("trace_query already answered the runtime-only root cause")
	mut.AppendDispatchToolResult(tier1TraceQueryRuntimeToolResult())
	o := &Orchestrator{busCtx: &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentRootCause,
			Scenario: types.ScenarioPerformanceBottleneck,
			ExternalObservationPolicy: &types.ExternalObservationPolicy{
				ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly,
				CurrentSourceMode:    types.ExternalObservationCurrentSourceDefault,
				Confidence:           0.9,
			},
		}},
	}}

	if !o.shouldDowngradeRuntimeAcceptedClosureExploreFallback([]types.Violation{{Kind: types.ViolCitation}}) {
		t.Fatal("runtime accepted closure should downgrade source/citation soft debt instead of reopening exploration")
	}
	if !o.shouldDowngradeRuntimeAcceptedClosureExploreFallback([]types.Violation{{Kind: types.ViolGhostAnchor}}) {
		t.Fatal("runtime accepted closure should downgrade soft source anchor debt instead of reopening exploration")
	}
	if !o.shouldDowngradeRuntimeAcceptedClosureExploreFallback([]types.Violation{{
		Kind:       types.ViolFacetUncovered,
		ClusterKey: types.FacetClusterKey(string(types.FacetCurrentCodePath), "answer_facet_coverage"),
	}}) {
		t.Fatal("runtime accepted closure should downgrade soft current-source facet debt instead of reopening exploration")
	}
	if !o.shouldDowngradeRuntimeAcceptedClosureExploreFallback([]types.Violation{{
		Kind:       types.ViolFacetUncovered,
		ClusterKey: types.FacetClusterKey(string(types.FacetObservedArtifactFact), "answer_facet_coverage"),
	}}) {
		t.Fatal("runtime accepted closure should keep runtime artifact facet repair in finalizer lane instead of broad exploration")
	}
	if o.shouldDowngradeRuntimeAcceptedClosureExploreFallback([]types.Violation{{Kind: types.ViolCitation, RepairLocusOverride: types.LocusExplore}}) {
		t.Fatal("typed explore-locus violation must remain load-bearing")
	}
	if o.shouldDowngradeRuntimeAcceptedClosureExploreFallback([]types.Violation{{
		Kind:       types.ViolFacetUncovered,
		ClusterKey: types.FacetClusterKey(string(types.FacetResolvedLiteralOrSymbol), "answer_facet_coverage"),
	}}) {
		t.Fatal("answer-critical resolved-literal facet debt must remain load-bearing")
	}
	if o.shouldDowngradeRuntimeAcceptedClosureExploreFallback([]types.Violation{{Kind: types.ViolIntentTraceShallow}}) {
		t.Fatal("answer-critical trace depth violations must still reopen bounded repair")
	}
}

func TestRuntimeAcceptedClosureKeepsPreciseCurrentSourceFallback(t *testing.T) {
	mut := types.NewMutableState("accepted mixed closure")
	mut.SetInvestigationComplete("trace_query answered the runtime slice")
	mut.AppendDispatchToolResult(tier1TraceQueryRuntimeToolResult())
	o := &Orchestrator{busCtx: &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentRootCause,
			Scenario: types.ScenarioPerformanceBottleneck,
			CurrentSourceExplanationProfile: &types.CurrentSourceExplanationProfile{
				IsCurrentSourceExplanationRequested: true,
				SourceQuotes:                        []string{"internal/tracequery/query.go:42"},
				Modes:                               []types.CurrentSourceExplanationMode{types.CurrentSourceExplanationExplainCurrentMechanism},
			},
		}},
	}}

	if o.shouldDowngradeRuntimeAcceptedClosureExploreFallback([]types.Violation{{Kind: types.ViolCitation}}) {
		t.Fatal("precise current-source requirement must not be downgraded by runtime accepted closure")
	}
}

func TestAcceptedClosureMissingRequiredOrigins_SourceOptionalRuntimeStatusDoesNotReopenCurrentSource(t *testing.T) {
	mut := types.NewMutableState("accepted trace closure with source-optional current-status contract")
	mut.SetInvestigationComplete("trace_query answered the runtime slice")
	mut.AppendDispatchToolResult(tier1TraceQueryRuntimeToolResult())
	o := &Orchestrator{busCtx: &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:   types.IntentRootCause,
				Scenario: types.ScenarioPerformanceBottleneck,
				Predicates: types.SemanticPredicates{
					IsDiagnosticQuestion: true,
				},
				DiagnosticProfile: types.DiagnosticIntentProfile{IsDiagnostic: true},
				PerfTrace: &types.PerfBundle{Observations: []types.PerfObservation{{
					Kind:       "root_cause_rank",
					Subject:    "app-100",
					Summary:    "runtime trace root cause is runnable contention",
					LineStart:  5,
					LineEnd:    8,
					DurationMs: 86.111,
				}}},
				ExternalObservationPolicy: &types.ExternalObservationPolicy{
					ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly,
					CurrentSourceMode:    types.ExternalObservationCurrentSourceDefault,
					Confidence:           0.9,
				},
			},
			AnswerContract: types.AnswerContract{
				CurrentStatusDiagnostic: &types.CurrentStatusDiagnosticContract{Required: true},
			},
		},
	}}

	if missing := o.acceptedClosureMissingRequiredOriginsForAutoComplete(); len(missing) != 0 {
		t.Fatalf("source-optional runtime authority should not reopen exploration for missing current_source, got %v", missing)
	}
}

func TestAcceptedClosureMissingRequiredOrigins_PreciseCurrentSourceStillBlocks(t *testing.T) {
	mut := types.NewMutableState("accepted trace closure with precise current-source lane")
	mut.SetInvestigationComplete("trace_query answered the runtime slice")
	mut.AppendDispatchToolResult(tier1TraceQueryRuntimeToolResult())
	o := &Orchestrator{busCtx: &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentRootCause,
			Scenario: types.ScenarioPerformanceBottleneck,
			Predicates: types.SemanticPredicates{
				IsDiagnosticQuestion: true,
			},
			DiagnosticProfile: types.DiagnosticIntentProfile{IsDiagnostic: true},
			PerfTrace: &types.PerfBundle{Observations: []types.PerfObservation{{
				Kind:       "root_cause_rank",
				Subject:    "app-100",
				Summary:    "runtime trace root cause is runnable contention",
				LineStart:  5,
				LineEnd:    8,
				DurationMs: 86.111,
			}}},
			ExternalObservationPolicy: &types.ExternalObservationPolicy{
				ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly,
				CurrentSourceMode:    types.ExternalObservationCurrentSourceDefault,
				Confidence:           0.9,
			},
			RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
				IsDimensionedAnswer: true,
				Dimensions: []types.RequestedAnswerDimension{{
					Label:       "current implementation",
					Role:        types.RequestedAnswerDimensionCurrentKeyCode,
					SourceQuote: "internal/tracequery/query.go:42",
					Required:    true,
					Index:       1,
				}},
				Confidence: 0.9,
			},
		}},
	}}

	missing := o.acceptedClosureMissingRequiredOriginsForAutoComplete()
	if len(missing) != 1 || missing[0] != types.AnswerEvidenceOriginCurrentSource {
		t.Fatalf("precise current-source authority should still require current_source, got %v", missing)
	}
}

func TestShouldAutoCompleteExploreWindowFromAcceptedClosure_StillBlocksStageRetry(t *testing.T) {
	mut := types.NewMutableState("accepted closure with explicit stage retry")
	mut.SetInvestigationComplete("accepted closure")
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut}}

	if o.shouldAutoCompleteExploreWindowFromAcceptedClosure(nil, "", "explicit structured handoff retry") {
		t.Fatal("explicit stage retry carries its own repair route and should still block auto-complete")
	}
}

// EVOLUTION RECORD (§40.14 V7-2, 2026-09-02): the fixture used to install
// an explore-owned RetryState directly and expected the veto. The arm now
// vetoes only a RetryState bound to the CURRENT explore-backtrack epoch
// with no completion decided since (generation binding), so the fixture
// replays the production FallbackBackToExplore ordering — populate, then
// ResetForFallback(Explore) binds, then the completion latch resets. The
// unbound shape this test previously exercised is pinned as "never
// vetoes" in accepted_closure_retry_authority_epoch_test.go.
func TestShouldAutoCompleteExploreWindowFromAcceptedClosure_PreservesTypedExploreContractBacktrack(t *testing.T) {
	mut := types.NewMutableState("accepted closure before required relation repair")
	mut.SetInvestigationComplete("the earlier evidence pass was accepted")
	mut.SetRetryState(&types.RetryState{
		Attempt:          1,
		LastPrimaryOwner: string(LocusExplore),
		ActiveViolations: []types.ScoredViolation{{
			Kind:     types.ViolRequiredDiagramEdgeAbsent,
			Severity: types.SeverityHigh,
			Layer:    "v2_oracle",
		}},
	})
	mut.ResetForFallback(types.FallbackResetTargetExplore)
	mut.ResetInvestigationComplete()
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut}}

	if o.shouldAutoCompleteExploreWindowFromAcceptedClosure(nil, "", "") {
		t.Fatal("a typed Explore-owned contract backtrack must not be erased by the earlier accepted closure")
	}
	if !acceptedClosureHasActiveExploreContractBacktrack(mut) {
		t.Fatal("the closed retry-state carrier must expose the active Explore-owned contract repair")
	}
}

func TestShouldAutoCompleteExploreWindowFromAcceptedClosure_DoesNotPromoteEmptyOrFinalizerRetryState(t *testing.T) {
	for _, tc := range []struct {
		name  string
		retry *types.RetryState
	}{
		{name: "no retry state"},
		{name: "empty explore retry", retry: &types.RetryState{Attempt: 1, LastPrimaryOwner: string(LocusExplore)}},
		{name: "finalizer retry", retry: &types.RetryState{
			Attempt: 1, LastPrimaryOwner: string(LocusFinalizer),
			ActiveViolations: []types.ScoredViolation{{Kind: types.ViolPrincipalClaimUseMissing, Severity: types.SeverityHigh}},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mut := types.NewMutableState(tc.name)
			mut.SetInvestigationComplete("accepted closure")
			mut.SetRetryState(tc.retry)
			o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut}}

			if !o.shouldAutoCompleteExploreWindowFromAcceptedClosure(nil, "", "") {
				t.Fatal("non-Explore or empty retry state must not turn advisory closure debt into a new exploration pass")
			}
		})
	}
}

func TestAcceptedClosureSuppressesCompletionFormRetryAfterReset(t *testing.T) {
	mut := types.NewMutableState("accepted closure with converged completion-form debt")
	mut.SetInvestigationComplete("completed with typed completion-form caveat")
	mut.ResetInvestigationComplete()
	mut.EvidenceClosure().AppendCompletionCaveat(types.CompletionCaveat{
		Lane:       types.DowngradeLaneCompletionForm,
		ReasonCode: types.ProgressReasonConverged,
		Reason:     "completion form remained unresolved at convergence",
	})
	mut.EvidenceClosure().AddRepair(types.RepairDirective{
		Kind:          types.RepairStructuredHandoff,
		Origin:        types.RepairOriginCompletionFormPrefix + "member_set_support_refs",
		Subject:       "completion_form:member_set_support_refs",
		DowngradeLane: types.DowngradeLaneCompletionForm,
		Rationale:     "support_refs form debt after accepted closure",
	})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut}}
	state := newGraphState(types.TaskGraph{ExecutionPolicy: types.ExecutionPolicy{RetryBudget: 1}})
	node := &types.TaskNode{ID: "n_evidence", Type: types.NodeEvidence}
	out := &agent.StageOutput{
		MissingPiece: types.MissingFacts,
		RetryHint:    "completion form repair should not reopen code exploration",
	}

	if o.requeueExploreWindowForFactRetry(state, []*types.TaskNode{node}, out) {
		t.Fatal("converged completion-form debt must not requeue exploration after accepted closure reset")
	}
	if state.retryUsed != 0 {
		t.Fatalf("suppressed completion-form retry must not consume retry budget, got %d", state.retryUsed)
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

func TestAcceptedClosure_ChainRankerSubjectRebindDoesNotReopenSiblingNodes(t *testing.T) {
	mut := types.NewMutableState("accepted call-chain closure")
	mut.SetInvestigationComplete("typed endpoint boundary and grounded chain accepted")
	mut.EvidenceClosure().AddRepair(types.RepairDirective{
		Kind:      types.RepairRebindSubject,
		Subject:   string(types.SubjectFunctionName),
		Origin:    "chain_ranker",
		Rationale: "ranked 97 candidate chains below a heuristic score floor",
	})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut}}

	if o.repairBlocksAcceptedClosure(types.RepairDirective{
		Kind:    types.RepairRebindSubject,
		Subject: string(types.SubjectFunctionName),
		Origin:  "chain_ranker",
	}) {
		t.Fatal("chain-ranker subject guidance must be advisory after typed completion")
	}
	if !o.shouldAutoCompleteExploreWindowFromAcceptedClosure(nil, "", "") {
		t.Fatal("accepted closure should auto-complete later evidence/validate siblings despite noisy subject guidance")
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

// acceptedClosureTraceOnlyExclusionBusContext rebuilds the production request
// chain from witness eval run
// trace_query_donghu_real_frame_multicausal-20260719-030504 (§29.140 GAP-4):
// the user asked 「只分析这份 trace，不分析代码」, the analyzer emitted the
// anchored typed exclusion (current_source_mode=exclude +
// exclusion_kind=explicit_user_exclusion + verbatim quote), the 1.9MB trace
// exceeded perf_triage_llm_max_bytes so NO LogTriage/PerfTrace bundle exists,
// and the only evidence lane is trace_query runtime observations. The required
// dimension 「链上主要原因」 role=current_key_code is the witness's noisy
// word-face token that keeps CurrentSourceLaneDecision at required.
func acceptedClosureTraceOnlyExclusionBusContext() *types.BusContext {
	mut := types.NewMutableState("只分析这份 trace，不分析代码。请分析 com.baidu.tieba 59566 主线程在 34579.472865s 到 34579.587805s 这一帧窗口内的卡顿原因。")
	mut.SetInvestigationComplete("trace_query 窗口证据已覆盖帧卡顿根因")
	mut.AppendDispatchToolResult(tier1TraceQueryRuntimeToolResult())
	return &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentRootCause,
			Scenario: types.ScenarioPerformanceBottleneck,
			Predicates: types.SemanticPredicates{
				IsDiagnosticQuestion: true,
			},
			DiagnosticProfile: types.DiagnosticIntentProfile{IsDiagnostic: true},
			ExternalObservationPolicy: &types.ExternalObservationPolicy{
				CurrentSourceMode:    types.ExternalObservationCurrentSourceExclude,
				ExclusionKind:        types.ExternalObservationSourceExclusionExplicitUserBoundary,
				ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly,
				SourceQuotes:         []string{"只分析这份 trace，不分析代码"},
				Confidence:           0.95,
			},
			RuntimeArtifactValueProfile: &types.RuntimeArtifactValueProfile{
				IsArtifactValueLookup: true,
				Target:                "主线程帧窗口",
				Value:                 "34579.472865s-34579.587805s",
				Unit:                  "s",
				LiteralKind:           types.FieldValueLiteralNumber,
				Confidence:            0.85,
			},
			RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
				IsDimensionedAnswer: true,
				Dimensions: []types.RequestedAnswerDimension{
					{Index: 1, Label: "直接阻塞/唤醒关系", Role: types.RequestedAnswerDimensionDiffClue, SourceQuote: "分层说明直接阻塞/唤醒关系", Required: true},
					{Index: 2, Label: "链上主要原因", Role: types.RequestedAnswerDimensionCurrentKeyCode, SourceQuote: "链上主要原因", Required: true},
					{Index: 3, Label: "调度/资源背景", Role: types.RequestedAnswerDimensionEvidenceSource, SourceQuote: "调度/资源背景", Required: true},
					{Index: 4, Label: "代表性时间窗", Role: types.RequestedAnswerDimensionStageWorkflow, SourceQuote: "代表性时间窗", Required: true},
				},
				Confidence: 0.9,
			},
			AnalyzerHints: types.AnalyzerHints{
				Kind: string(types.ReqMechanism),
				RequiredFileHints: []types.RequiredFileHint{{
					Path:       ".codrax/blob/20260719-030506-000-34307/attached_trace.txt",
					Confidence: 0.9,
				}},
			},
		}},
	}
}

func TestAcceptedClosureMissingRequiredOrigins_ExplicitTraceOnlyExclusionWaivesCurrentSourceDebt(t *testing.T) {
	o := &Orchestrator{busCtx: acceptedClosureTraceOnlyExclusionBusContext()}
	rm := o.busCtx.AnalysisIR.RequestModel
	contract := &o.busCtx.AnalysisIR.AnswerContract

	// §29.146 UPSTREAM-3: the witness disease chain now dies UPSTREAM. 件3 —
	// the mislabeled prose current_key_code dimension (「链上主要原因」) no
	// longer mints a current-source verification anchor, so the user's typed
	// exclusion owns the lane decision (pre-fix this read
	// CurrentSourceLaneRequired via the word-face fallback)...
	if got := rm.CurrentSourceLaneDecision(); got != types.CurrentSourceLaneExcluded {
		t.Fatalf("typed exclusion should own the lane decision once the prose anchor is demoted, got %s", got)
	}
	// ...and therefore the intent contract carries no current_source origin,
	// so the mixed-origin auto-complete debt is never armed at all — the
	// §29.140 GAP-4 waiver lane has retired from 兜底 (only defense) to 保险
	// (backstop).
	if parallelExploreMixedOriginNeedsSiblingHandoffs(rm, contract, o.busCtx.RuntimeArtifactPreflight) {
		t.Fatal("witness shape must no longer arm the mixed-origin handoff debt upstream")
	}

	// Terminal behavior is identical to the §29.140 GAP-4 post-filter era:
	// the accepted closure lands on the FIRST emit instead of redispatching
	// the explore window (witness burned 3 emit_investigation_complete
	// rounds / 21 trace_query calls / ~180s).
	if missing := o.acceptedClosureMissingRequiredOriginsForAutoComplete(); len(missing) != 0 {
		t.Fatalf("explicit trace-only exclusion should leave no current_source debt, got %v", missing)
	}
	if !o.shouldAutoCompleteExploreWindowFromAcceptedClosure(nil, "", "") {
		t.Fatal("accepted trace-only closure should auto-complete on the first emit")
	}
}

// §29.146 UPSTREAM-3 件1 pin: when the waiver lane holds while the
// mixed-origin shape stays armed (here: the deterministic zero-current-source
// census, arm ②, on a default-posture request the authority suppressor does
// not cover), the waived current_source lane is decided BEFORE the debt is
// minted — it never enters the minted lane set — and the not-mint path is
// byte-equivalent to the §29.140 mint-then-drop path (witness-behavior
// equivalence). The post-filter stays wired as the invariant backstop.
func TestAcceptedClosureWithholdsWaivedCurrentSourceLaneBeforeDebtMint(t *testing.T) {
	bus := acceptedClosureTraceOnlyExclusionBusContext()
	bus.AnalysisIR.RequestModel.ExternalObservationPolicy = nil
	bus.RuntimeArtifactPreflight = types.RuntimeArtifactPreflightProfile{
		Active: true,
		Artifacts: []types.RuntimeArtifactPreflightArtifact{{
			Kind:   "trace",
			Source: "xxx_all.systrace",
		}},
		RepoSourceCensus: types.RuntimeArtifactRepoSourceCensus{
			Completed:     true,
			SourceFiles:   0,
			ArtifactFiles: 1,
		},
	}
	o := &Orchestrator{busCtx: bus}
	rm := o.busCtx.AnalysisIR.RequestModel
	contract := &o.busCtx.AnalysisIR.AnswerContract

	if o.runtimeSourceAuthoritySuppressesAcceptedClosureOriginDebt() {
		t.Fatal("fixture must reach the pre-mint withhold lane, not the authority suppressor")
	}
	if !parallelExploreMixedOriginNeedsSiblingHandoffs(rm, contract, o.busCtx.RuntimeArtifactPreflight) {
		t.Fatal("fixture should arm the mixed-origin handoff shape via the typed contract lane")
	}
	// Pre-fix shape: the unpruned lane set still carries current_source — the
	// debt would have been MINTED and only the post-filter would drop it.
	unpruned := requiredMixedOriginAutoCompleteLanes(
		types.CompileAnswerIntentContractWithPreflight(rm, contract, o.busCtx.RuntimeArtifactPreflight))
	hasCurrent := false
	for _, origin := range unpruned {
		if origin == types.AnswerEvidenceOriginCurrentSource {
			hasCurrent = true
		}
	}
	if !hasCurrent {
		t.Fatalf("probe setup: unpruned lanes should still carry current_source, got %v", unpruned)
	}

	// 件1 decision point: the shared waiver predicate withholds the
	// current_source lane BEFORE missingObservationOrigins runs.
	lanes := o.acceptedClosureRequiredOriginLanesBeforeDebtMint()
	keptRuntime := false
	for _, origin := range lanes {
		if origin == types.AnswerEvidenceOriginCurrentSource {
			t.Fatalf("current_source must be withheld before the debt is minted, got %v", lanes)
		}
		if origin == types.AnswerEvidenceOriginRuntimeArtifact {
			keptRuntime = true
		}
	}
	if !keptRuntime {
		t.Fatalf("withhold must only remove current_source; runtime_artifact lane lost: %v", lanes)
	}

	// Witness-behavior equivalence: not-mint == mint-then-drop, lane for lane
	// (compared on the same origin-sequence face the production log emits).
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(o.busCtx, types.ObservationExtractLedgerEvidenceLimit))
	mintThenDrop := o.dropWaivedCurrentSourceOriginDebt(missingObservationOrigins(unpruned, ledger))
	notMinted := o.acceptedClosureMissingRequiredOriginsForAutoComplete()
	if formatAnswerEvidenceOriginsForLog(notMinted) != formatAnswerEvidenceOriginsForLog(mintThenDrop) {
		t.Fatalf("pre-mint withhold must be behaviorally equivalent to the mint-then-drop post-filter\nnot-mint=%v\nmint-then-drop=%v", notMinted, mintThenDrop)
	}

	// Backstop stays functional as the invariant net: if a future minting
	// path bypasses the pre-mint decision, the post-filter still drops ONLY
	// the waived current_source arm.
	backstop := o.dropWaivedCurrentSourceOriginDebt([]types.AnswerEvidenceOrigin{
		types.AnswerEvidenceOriginCurrentSource,
		types.AnswerEvidenceOriginRuntimeArtifact,
	})
	if len(backstop) != 1 || backstop[0] != types.AnswerEvidenceOriginRuntimeArtifact {
		t.Fatalf("post-filter backstop must keep dropping only the waived current_source arm, got %v", backstop)
	}
}

// §29.146 UPSTREAM-3 件1 structural census: the single debt-minting call must
// consume ONLY the pre-mint lane set (the helper whose tail decision is the
// waiver withhold), and the post-filter must stay wired around the mint as the
// invariant backstop (double defense). A refactor that feeds the mint an
// unwithheld lane set — or drops either defense half — silently reverts the
// lane decision sequence.
func TestAcceptedClosurePreMintWithholdPrecedesDebtMint(t *testing.T) {
	data, err := os.ReadFile("accepted_closure_origin_debt.go")
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	src := string(data)
	if !strings.Contains(src, "required := o.acceptedClosureRequiredOriginLanesBeforeDebtMint()") {
		t.Fatal("the debt entry must source its lanes from the pre-mint helper (waiver decided before minting)")
	}
	if !strings.Contains(src, "return o.withholdWaivedCurrentSourceOriginLaneBeforeDebtMint(required)") {
		t.Fatal("the pre-mint helper must end on the waiver withhold — the exclusion lane no longer decides after minting")
	}
	if !strings.Contains(src, "dropWaivedCurrentSourceOriginDebt(missingObservationOrigins(required, ledger))") {
		t.Fatal("the post-filter backstop must stay wired around the single mint call")
	}
}

func TestAcceptedClosureMissingRequiredOrigins_MixedTraceCodeRequestStillBlocks(t *testing.T) {
	// Negative arm (§29.140 GAP-4): the same production chain WITHOUT the
	// typed exclusion, carrying the genuine trace+code shape (an active
	// CurrentSourceExplanationProfile), must keep its current_source
	// redispatch pressure.
	bus := acceptedClosureTraceOnlyExclusionBusContext()
	bus.AnalysisIR.RequestModel.ExternalObservationPolicy = nil
	bus.AnalysisIR.RequestModel.CurrentSourceExplanationProfile = &types.CurrentSourceExplanationProfile{
		IsCurrentSourceExplanationRequested: true,
		Modes:                               []types.CurrentSourceExplanationMode{types.CurrentSourceExplanationExplainCurrentMechanism},
		SourceQuotes:                        []string{"结合当前代码说明卡顿原因"},
		Confidence:                          0.9,
	}
	o := &Orchestrator{busCtx: bus}

	missing := o.acceptedClosureMissingRequiredOriginsForAutoComplete()
	if len(missing) != 1 || missing[0] != types.AnswerEvidenceOriginCurrentSource {
		t.Fatalf("genuine trace+code request must keep the current_source redispatch debt, got %v", missing)
	}
	if o.shouldAutoCompleteExploreWindowFromAcceptedClosure(nil, "", "") {
		t.Fatal("genuine trace+code request must not auto-complete before current-source evidence lands")
	}
}

func TestAcceptedClosureTraceOnlyExclusionWaiverKeepsRuntimeOnlyCaveatShape(t *testing.T) {
	// Downgradable terminal equivalence (§29.140 GAP-4 pin ③): the waiver is
	// read-only — the runtime-only authority face is byte-identical after the
	// gate runs, so the terminal caveat renders the same shape. §29.146
	// UPSTREAM-3 update: with the prose current_key_code anchor demoted (件3)
	// the lane reads excluded instead of the witness's noisy required, so the
	// runtime-only lane is now the NATIVE lane (CanUseRuntimeOnlyWithCaveat)
	// rather than a downgrade target — CanDowngradeToCaveat is vacuously off
	// because there is no source requirement left to downgrade.
	o := &Orchestrator{busCtx: acceptedClosureTraceOnlyExclusionBusContext()}
	before := types.BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(o.busCtx, types.ObservationLedger{})
	if !before.CanUseRuntimeOnlyWithCaveat {
		t.Fatalf("fixture should keep the runtime-only caveat lane active, got %+v", before)
	}
	if before.CurrentSourceLane != types.CurrentSourceLaneExcluded {
		t.Fatalf("typed exclusion should own the lane once the prose anchor is demoted, got %+v", before)
	}
	if before.CanHardBlockCompletion || before.CurrentSourceRequirement == types.RuntimeSourceRequirementPrecise {
		t.Fatalf("typed exclusion must keep the source requirement soft/non-blocking, got %+v", before)
	}
	if missing := o.acceptedClosureMissingRequiredOriginsForAutoComplete(); len(missing) != 0 {
		t.Fatalf("waiver should clear the debt, got %v", missing)
	}
	after := types.BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(o.busCtx, types.ObservationLedger{})
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("waiver must be read-only: authority snapshot changed\nbefore=%+v\nafter=%+v", before, after)
	}
}

func TestAcceptedClosureMissingRequiredOrigins_ZeroCurrentSourceRepoWaivesCurrentSourceDebt(t *testing.T) {
	// §29.140 GAP-4 arm ②: the deterministic run-entry census proved the
	// checkout holds zero current-source files, so the current_source debt is
	// structurally unpayable — no redispatch can ever mint the observation.
	bus := acceptedClosureTraceOnlyExclusionBusContext()
	bus.AnalysisIR.RequestModel.ExternalObservationPolicy = nil
	bus.RuntimeArtifactPreflight = types.RuntimeArtifactPreflightProfile{
		Active: true,
		Artifacts: []types.RuntimeArtifactPreflightArtifact{{
			Kind:   "trace",
			Source: "xxx_all.systrace",
		}},
		RepoSourceCensus: types.RuntimeArtifactRepoSourceCensus{
			Completed:     true,
			SourceFiles:   0,
			ArtifactFiles: 1,
		},
	}
	o := &Orchestrator{busCtx: bus}

	if missing := o.acceptedClosureMissingRequiredOriginsForAutoComplete(); len(missing) != 0 {
		t.Fatalf("zero-current-source repo should waive the structurally unpayable current_source debt, got %v", missing)
	}
	if !o.shouldAutoCompleteExploreWindowFromAcceptedClosure(nil, "", "") {
		t.Fatal("zero-current-source repo closure should auto-complete on the first emit")
	}
}

// 复核 F-1 收编 (§29.146): under the
// explicit trace-only exclusion, a missing RUNTIME lane must survive the
// current_source waiver — the filter is only allowed to drop current_source.
func TestAcceptedClosureWaiverKeepsRuntimeArtifactDebt(t *testing.T) {
	bus := acceptedClosureTraceOnlyExclusionBusContext()
	// §29.146 UPSTREAM-3: the bare witness shape no longer arms the
	// mixed-origin debt at all (件3 kills the prose anchor upstream), so the
	// negative arm keeps the mixed shape armed through the deterministic
	// typed contract lane instead.
	bus.AnalysisIR.AnswerContract.CurrentStatusDiagnostic = &types.CurrentStatusDiagnosticContract{Required: true}
	// Strip the runtime observation: the ledger now has NO runtime evidence,
	// so the runtime_artifact lane is genuinely in debt alongside
	// current_source.
	mut := types.NewMutableState("只分析这份 trace，不分析代码。")
	mut.SetInvestigationComplete("claimed complete with zero evidence")
	bus.Mutable = mut
	o := &Orchestrator{busCtx: bus}

	rm := o.busCtx.AnalysisIR.RequestModel
	contract := &o.busCtx.AnalysisIR.AnswerContract
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(o.busCtx, types.ObservationExtractLedgerEvidenceLimit))
	unfiltered := missingObservationOrigins(requiredMixedOriginAutoCompleteLanes(types.CompileAnswerIntentContract(rm, contract)), ledger)
	hasRuntime := false
	for _, origin := range unfiltered {
		if origin == types.AnswerEvidenceOriginRuntimeArtifact {
			hasRuntime = true
		}
	}
	if !hasRuntime {
		t.Fatalf("probe setup: runtime_artifact should be in unfiltered debt, got %v", unfiltered)
	}

	missing := o.acceptedClosureMissingRequiredOriginsForAutoComplete()
	keptRuntime := false
	for _, origin := range missing {
		if origin == types.AnswerEvidenceOriginRuntimeArtifact {
			keptRuntime = true
		}
		if origin == types.AnswerEvidenceOriginCurrentSource {
			t.Fatalf("current_source should be waived, got %v", missing)
		}
	}
	if !keptRuntime {
		t.Fatalf("waiver must NOT clear the runtime_artifact debt: an accepted closure with ZERO runtime evidence would auto-complete; got %v", missing)
	}
	if o.shouldAutoCompleteExploreWindowFromAcceptedClosure(nil, "", "") {
		t.Fatal("zero-evidence closure must not auto-complete even under the exclusion waiver")
	}
}

// 复核 F-2 收编 (§29.146): the unfiltered missingObservationOrigins helper
// may be consumed in production ONLY through the shared filtered entry
// (acceptedClosureMissingRequiredOriginsForAutoComplete) — a fork/inline
// refactor that hands a second gate the unfiltered set would silently
// resurrect the GAP-4 burn on that gate. Source census: exactly one
// production call site, and it lives inside the shared entry.
func TestAcceptedClosureUnfilteredHelperSingleEntry(t *testing.T) {
	data, err := os.ReadFile("accepted_closure_origin_debt.go")
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	calls := strings.Count(string(data), "missingObservationOrigins(required, ledger)")
	if calls != 1 {
		t.Fatalf("unfiltered helper must have exactly one production call site (the shared filtered entry); got %d", calls)
	}
	// The orchestrator body must not grow a second direct consumer.
	main, err := os.ReadFile("orchestrator.go")
	if err != nil {
		t.Fatalf("read orchestrator.go: %v", err)
	}
	if strings.Contains(string(main), "missingObservationOrigins(") {
		t.Fatalf("orchestrator.go must route origin-debt through the shared filtered entry, not the raw helper")
	}
}
