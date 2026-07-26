package dataworkflow

import (
	"context"
	"os"
	"path/filepath"
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
		t.Fatalf("the shared target continuation completes decisions+contributions in one action: %+v", transition.Plan.Actions)
	}
	if transition.Plan.Actions[0].Params["role"] != "target" ||
		transition.Plan.Actions[0].Params["operation"] != "include" ||
		transition.Plan.Actions[0].Params["value_field"] != "id" {
		t.Fatalf("completion action must carry the answer member set as target contributions: %+v", transition.Plan.Actions[0])
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "active_user_records.json"), []byte(`[{"id":"u1","active":true},{"id":"u3","active":true}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := (dataquery.ActionRunner{
		RepoRoot: root,
		Seed:     dataquery.Result{Answer: `{"ids":["u1","u3"]}`},
	}).Run(context.Background(), transition.Plan)
	if err != nil {
		t.Fatalf("generated deterministic completion plan must execute and pass its ledger contract: %v", err)
	}
	if len(result.Contributions) != 2 || len(result.Rows) != 2 {
		t.Fatalf("completion plan must materialize both target contributions and derived decisions: contributions=%+v rows=%+v", result.Contributions, result.Rows)
	}
	for _, contribution := range result.Contributions {
		if contribution.Role.String() != "target" || contribution.Operation.String() != "include" {
			t.Fatalf("audit-only completion is forbidden: %+v", contribution)
		}
	}
	if err := dataquery.ValidateResultAgainstContract(
		transition.Plan.CoverageContract,
		result,
		dataquery.LedgerSatisfactionFacts{},
	); err != nil {
		t.Fatalf("generated plan result must pass the workflow-level terminal ledger validator: %v", err)
	}

	if err := os.WriteFile(filepath.Join(root, "active_user_records.json"), []byte(`[{"id":"u1","active":true},{"id":"u2","active":true}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := (dataquery.ActionRunner{
		RepoRoot: root,
		Seed:     dataquery.Result{Answer: `{"ids":["u1","u3"]}`},
	}).Run(context.Background(), transition.Plan); err == nil || !strings.Contains(err.Error(), "answer closure failed") {
		t.Fatalf("same-size but wrong-member records must fail closed instead of grounding the existing answer: %v", err)
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
