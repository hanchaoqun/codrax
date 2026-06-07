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

func TestBuildRecordMaterializationFallbackPlanUsesRequiredMaterials(t *testing.T) {
	plan, reason, ok := BuildRecordMaterializationFallbackPlan(RecordMaterializationFallbackInput{
		Current: dataquery.TaskPlan{Goal: "prepare typed records"},
		Coverage: dataquery.CoverageContract{RequiredMaterials: []dataquery.CoverageMaterial{{
			Path:      "records.csv",
			Required:  true,
			UsageMode: dataquery.MaterialUseScriptConsumed,
		}}},
		Output:             dataquery.OutputContract{Format: dataquery.OutputMarkdown, ExplanationAllowed: true},
		AllowedNextActions: []string{string(dataquery.DataActionExtractRecords)},
		ExtractLimit:       77,
		ReasonPrefix:       "batch result completed",
	})
	if !ok {
		t.Fatal("BuildRecordMaterializationFallbackPlan ok=false")
	}
	if !strings.Contains(reason, "record materialization") {
		t.Fatalf("reason=%q, want record materialization reason", reason)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Kind != dataquery.DataActionExtractRecords {
		t.Fatalf("actions=%+v, want extract_records", plan.Actions)
	}
	if strings.Join(plan.Actions[0].InputPaths, ",") != "records.csv" || plan.Actions[0].Params["limit"] != "77" {
		t.Fatalf("action=%+v, want required input and configured limit", plan.Actions[0])
	}
	if !plan.ContinueAfter || plan.OutputContract.Format != dataquery.OutputMarkdown {
		t.Fatalf("plan=%+v, want continuation preserving output contract", plan)
	}
}

func TestBuildRequiredOutputProjectionPlanUsesReconcileGroups(t *testing.T) {
	plan, ok := BuildRequiredOutputProjectionPlan(OutputProjectionPlanInput{
		Current:  dataquery.TaskPlan{Goal: "format final values"},
		Coverage: dataquery.CoverageContract{ReconcileRequired: true},
		Output:   dataquery.OutputContract{Format: dataquery.OutputCSVLine, ExplanationAllowed: false},
		Result: dataquery.Result{
			Reconcile: &dataquery.ReconcileReport{
				Status: dataquery.LooseText("pass"),
				Groups: []dataquery.ReconcileGroup{{
					GroupKey: dataquery.LooseText("A"),
					Metric:   dataquery.LooseText("value"),
					Actual:   dataquery.LooseText("10"),
				}},
			},
		},
	})
	if !ok {
		t.Fatal("BuildRequiredOutputProjectionPlan ok=false")
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Kind != dataquery.DataActionAssembleAnswer {
		t.Fatalf("actions=%+v, want assemble_answer", plan.Actions)
	}
	if plan.ContinueAfter {
		t.Fatalf("ContinueAfter=true, want terminal projection batch")
	}
	if !plan.CoverageContract.ReconcileRequired {
		t.Fatalf("CoverageContract=%+v, want reconcile required preserved", plan.CoverageContract)
	}
}

func TestBuildRequiredLedgerCompletionPlanCompletesReconcileFromContributions(t *testing.T) {
	plan, ok := BuildRequiredLedgerCompletionPlan(RequiredLedgerCompletionPlanInput{
		Current:  dataquery.TaskPlan{Goal: "finish validation"},
		Coverage: dataquery.CoverageContract{ReconcileRequired: true},
		Result: dataquery.Result{Contributions: []dataquery.ContributionRecord{{
			ItemID:    dataquery.LooseText("row-1"),
			GroupKey:  dataquery.LooseText("A"),
			Metric:    dataquery.LooseText("value"),
			Value:     dataquery.LooseText("10"),
			Operation: dataquery.LooseText("add"),
		}}},
		ErrorText: `validate data workflow completion: data validation incomplete: coverage_contract.reconcile_required=true but result.reconcile is empty`,
	})
	if !ok {
		t.Fatal("BuildRequiredLedgerCompletionPlan ok=false")
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Kind != dataquery.DataActionReconcile {
		t.Fatalf("actions=%+v, want reconcile_artifacts", plan.Actions)
	}
	if plan.ContinueAfter {
		t.Fatalf("ContinueAfter=true, want terminal ledger repair batch")
	}
}
