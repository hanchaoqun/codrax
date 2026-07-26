package dataworkflow

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

// fallback_plan_gapd1_test.go — GAP-EVAL-D1 (eval audit 2026-07-26) pins:
// the eval specimen burned 18 batches and terminal-failed with the correct
// answer in hand because (a) the DECISIONS ledger had no deterministic
// completion arm even though the conservative compute_contribs continuation
// is a declared decisions producer, and (b) every disclosure face named ONE
// missing ledger per round (the EMITBURN shape).

func TestBuildCompletionRepairTransitionCompletesMissingDecisionsLedger(t *testing.T) {
	// The specimen's terminal shape: rule coverage landed, decisions still
	// missing — FirstIncompleteRequiredLedger returns decisions, which used
	// to fall through the dispatch default with NO deterministic plan.
	graph := BuildLedgerGraph(StageFacts{
		MaterialCoverageSufficient: true,
		RuleCoverageRequired:       true,
		RuleCoverageRecords:        1,
		DecisionRecordsRequired:    true,
		ContributionLedgerRequired: true,
	})
	dep, ok := FirstIncompleteRequiredLedger(graph)
	if !ok || dep.Ledger != string(LedgerDecisions) {
		t.Fatalf("fixture precondition: decisions must be the first incomplete ledger: %+v ok=%v", dep, ok)
	}
	guard := LedgerGraphCompletionGuardResult(graph)
	if guard.Empty() {
		t.Fatal("fixture should produce a ledger guard")
	}
	transition := BuildCompletionRepairTransition(CompletionRepairTransitionInput{
		Current:        dataquery.TaskPlan{Goal: "finish strict answer validation"},
		Coverage:       dataquery.CoverageContract{DecisionRecordsRequired: true, ContributionLedgerRequired: true},
		Output:         dataquery.OutputContract{Format: dataquery.OutputJSONOnly},
		Result:         dataquery.Result{Answer: `{"ids":["u1","u3"]}`},
		LedgerGraph:    graph,
		UseLedgerGraph: true,
		Guard:          guard,
		Artifacts: []ArtifactSchemaProjection{{
			ID:        "active_user_records",
			Kind:      string(dataquery.DataActionFilterRecords),
			NodeClass: ArtifactNodeClassRecord,
			Aliases:   []string{"active_user_records.json"},
			JSONShape: "array(len=2,item=object(keys=id,active))",
			Fields:    []string{"id", "active"},
			RowCount:  2,
		}},
	})
	if !transition.Deterministic || !transition.HasPlan() {
		t.Fatalf("the decisions ledger must have a deterministic completion arm: %+v", transition)
	}
	if len(transition.Plan.Actions) != 1 || transition.Plan.Actions[0].Kind != dataquery.DataActionComputeContribs {
		t.Fatalf("the shared audit continuation completes decisions+contributions in one action: %+v", transition.Plan.Actions)
	}
}

func TestLedgerCompletionMessageAllNamesFullRoster(t *testing.T) {
	graph := BuildLedgerGraph(StageFacts{
		MaterialCoverageSufficient: true,
		RuleCoverageRequired:       true,
		DecisionRecordsRequired:    true,
		ContributionLedgerRequired: true,
		ReconcileRequired:          true,
	})
	msg := LedgerCompletionMessageAll(graph)
	for _, want := range []string{"rule_coverage", "decisions", "contributions", "reconcile", "required ledgers unfinished"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("full-roster message missing %q: %q", want, msg)
		}
	}
	// The blocked entries keep their prerequisite detail (the material/rule
	// blockers stay visible — the single-ledger form's information survives).
	if !strings.Contains(msg, "missing_prerequisites=") {
		t.Fatalf("blocked entries must keep prerequisite detail: %q", msg)
	}
}

func TestLedgerPrerequisiteSiblingRelaxIsNarrow(t *testing.T) {
	// The continuation-satisfiable set is EXACTLY the sibling books; any
	// other prerequisite (materials, rule coverage) keeps the refusal.
	if !ledgerPrerequisitesSatisfiedByContributionContinuation(LedgerDependency{
		MissingPrerequisites: []string{string(LedgerDecisions)},
	}) {
		t.Fatal("a decisions-only blocker is satisfied by the same continuation")
	}
	if ledgerPrerequisitesSatisfiedByContributionContinuation(LedgerDependency{
		MissingPrerequisites: []string{LedgerPrerequisiteMaterials},
	}) {
		t.Fatal("a materials blocker must keep the refusal")
	}
	if ledgerPrerequisitesSatisfiedByContributionContinuation(LedgerDependency{
		MissingPrerequisites: []string{string(LedgerDecisions), string(LedgerRuleCoverage)},
	}) {
		t.Fatal("a mixed blocker set must keep the refusal")
	}
}
