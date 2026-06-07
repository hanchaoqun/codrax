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

func TestBuildNextStageConcreteFallbackPlanUsesArtifactSchemaProjection(t *testing.T) {
	plan, reason, ok := BuildNextStageConcreteFallbackPlan(NextStageFallbackPlanInput{
		Current: dataquery.TaskPlan{
			Goal: "finish grouped calculation",
		},
		Coverage: dataquery.CoverageContract{
			ContributionLedgerRequired: true,
		},
		Output: dataquery.OutputContract{Format: dataquery.OutputCSVLine},
		Facts: StageFacts{
			MaterialCoverageSufficient: true,
			ContributionLedgerRequired: true,
			EntityStageMaterialized:    true,
		},
		AllowedNextActions: []string{string(dataquery.DataActionJoinRecords)},
		Artifacts: []ArtifactSchemaProjection{
			{ID: "left.json", Aliases: []string{"left.json"}, JSONShape: "records", Fields: []string{"id", "amount"}, RowCount: 2},
			{ID: "right.json", Aliases: []string{"right.json"}, JSONShape: "records", Fields: []string{"id", "label"}, RowCount: 2},
		},
		ReasonPrefix: "batch result completed",
	})
	if !ok {
		t.Fatal("BuildNextStageConcreteFallbackPlan ok=false")
	}
	if !strings.Contains(reason, "converted to concrete typed action scaffold") {
		t.Fatalf("reason=%q", reason)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Kind != dataquery.DataActionJoinRecords {
		t.Fatalf("actions=%+v, want concrete join_records", plan.Actions)
	}
	if plan.Actions[0].Params["left_fields"] != `["id"]` || plan.Actions[0].Params["right_fields"] != `["id"]` {
		t.Fatalf("join params=%+v, want concrete common key fields", plan.Actions[0].Params)
	}
	if plan.OutputContract.Format != dataquery.OutputCSVLine || !plan.CoverageContract.ContributionLedgerRequired {
		t.Fatalf("contracts=%+v/%+v, want preserved output and coverage contracts", plan.OutputContract, plan.CoverageContract)
	}
}

func TestBuildNextStageConcreteFallbackPlanBlocksRepeatedRelationNoProgress(t *testing.T) {
	_, _, ok := BuildNextStageConcreteFallbackPlan(NextStageFallbackPlanInput{
		Current: dataquery.TaskPlan{
			Goal: "finish grouped calculation",
		},
		Coverage: dataquery.CoverageContract{
			ContributionLedgerRequired: true,
		},
		Facts: StageFacts{
			MaterialCoverageSufficient: true,
			ContributionLedgerRequired: true,
			EntityStageMaterialized:    true,
		},
		AllowedNextActions: []string{string(dataquery.DataActionJoinRecords)},
		Artifacts: []ArtifactSchemaProjection{
			{ID: "left.json", Aliases: []string{"left.json"}, JSONShape: "records", Fields: []string{"id", "amount"}, RowCount: 2},
			{ID: "right.json", Aliases: []string{"right.json"}, JSONShape: "records", Fields: []string{"id", "label"}, RowCount: 2},
		},
		ProgressEvents: []ProgressEvent{{
			ResultPresent: true,
			Actions: []dataquery.DataAction{{
				Kind: dataquery.DataActionJoinRecords,
			}},
		}},
		NoProgressStop: 1,
	})
	if ok {
		t.Fatal("BuildNextStageConcreteFallbackPlan ok=true, want repeated relation no-progress blocked")
	}
}
