package dataworkflow

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestBuildConcreteFallbackPlanUsesTypedScaffoldAndContracts(t *testing.T) {
	plan, reason, ok := BuildConcreteFallbackPlan(ConcreteFallbackPlanInput{
		Current: dataquery.TaskPlan{
			Goal:            "compute final value",
			SuccessCriteria: []string{"answer is reconciled"},
		},
		Coverage: dataquery.CoverageContract{
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Output: dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine},
		Facts: StageFacts{
			MaterialCoverageSufficient: true,
			EntityStageMaterialized:    true,
			ContributionLedgerRequired: true,
		},
		ReasonPrefix: "batch result completed",
		Scaffolds: []ActionScaffold{{
			Kind:      string(dataquery.DataActionValueDistribution),
			InputPath: "records.json",
			Fields:    []string{"_source", "status", "amount"},
			ParamsTemplate: map[string]string{
				"fields": `["<existing field from fields>"]`,
			},
		}},
	})
	if !ok {
		t.Fatal("BuildConcreteFallbackPlan ok=false")
	}
	if !strings.Contains(reason, "converted to concrete typed action scaffold") {
		t.Fatalf("reason=%q", reason)
	}
	if plan.Status != "ready" || !plan.ContinueAfter || plan.Goal != "compute final value" {
		t.Fatalf("plan=%+v, want ready continuation preserving goal", plan)
	}
	if plan.OutputContract.Format != dataquery.OutputPlainSingleLine || !plan.CoverageContract.ContributionLedgerRequired {
		t.Fatalf("plan contracts=%+v/%+v", plan.OutputContract, plan.CoverageContract)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Kind != dataquery.DataActionValueDistribution {
		t.Fatalf("actions=%+v, want value_distribution", plan.Actions)
	}
	if strings.Contains(plan.Actions[0].Params["fields"], "<") || strings.Contains(plan.Actions[0].Params["fields"], "_source") {
		t.Fatalf("fields=%q, want concrete non-internal fields", plan.Actions[0].Params["fields"])
	}
}

func TestBuildConcreteFallbackPlanSkipsSeenActionKeys(t *testing.T) {
	scaffold := ActionScaffold{
		Kind:      string(dataquery.DataActionValueDistribution),
		InputPath: "records.json",
		Fields:    []string{"status"},
	}
	action, ok := ConcreteActionFromScaffold(scaffold)
	if !ok {
		t.Fatal("ConcreteActionFromScaffold ok=false")
	}
	_, _, got := BuildConcreteFallbackPlan(ConcreteFallbackPlanInput{
		Scaffolds:      []ActionScaffold{scaffold},
		SeenActionKeys: ActionIdempotencyKeys([]dataquery.DataAction{action}),
	})
	if got {
		t.Fatal("BuildConcreteFallbackPlan ok=true, want seen action skipped")
	}
}
