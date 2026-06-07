package dataworkflow

import (
	"fmt"
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
			Kind:       string(dataquery.DataActionValueDistribution),
			Executable: true,
			InputPath:  "records.json",
			Fields:     []string{"_source", "status", "amount"},
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
		Kind:       string(dataquery.DataActionValueDistribution),
		Executable: true,
		InputPath:  "records.json",
		Fields:     []string{"status"},
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

func TestBuildCoverageExpansionPlanSplitsMaterialShapes(t *testing.T) {
	plan, ok := BuildCoverageExpansionPlan(CoverageExpansionPlanInput{
		Current:            dataquery.TaskPlan{Goal: "cover materials"},
		Coverage:           dataquery.CoverageContract{RuleCoverageRequired: true},
		MissingPaths:       []string{"rules.md", "records.csv", "blob.bin"},
		DeriveRulesForText: true,
		MaxActions:         4,
		ValidationRule:     "coverage guard requested atomic material coverage",
	})
	if !ok {
		t.Fatal("BuildCoverageExpansionPlan ok=false")
	}
	if len(plan.Actions) != 3 {
		t.Fatalf("actions=%+v, want rule/record/inspect actions", plan.Actions)
	}
	gotKinds := []dataquery.DataActionKind{plan.Actions[0].Kind, plan.Actions[1].Kind, plan.Actions[2].Kind}
	wantKinds := []dataquery.DataActionKind{dataquery.DataActionDeriveRules, dataquery.DataActionExtractRecords, dataquery.DataActionInspectMaterial}
	for i := range wantKinds {
		if gotKinds[i] != wantKinds[i] {
			t.Fatalf("action kinds=%v, want %v", gotKinds, wantKinds)
		}
	}
	if strings.Join(plan.InputPaths, ",") != "rules.md,records.csv,blob.bin" {
		t.Fatalf("InputPaths=%v, want missing paths preserved", plan.InputPaths)
	}
	if len(plan.CoverageContract.ValidationRules) != 1 {
		t.Fatalf("ValidationRules=%v, want supplied typed validation rule", plan.CoverageContract.ValidationRules)
	}
}

func TestBuildMaterialDiscoveryPlanClearsCoverageFloors(t *testing.T) {
	plan, ok := BuildMaterialDiscoveryPlan(MaterialDiscoveryPlanInput{
		Current: dataquery.TaskPlan{Goal: "discover inputs"},
		Coverage: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{{Path: "wide.csv", Required: true}},
			OptionalMaterials: []dataquery.CoverageMaterial{{Path: "notes.txt"}},
		},
		Paths:          []string{"wide.csv", "notes.txt"},
		ValidationRule: "previous broad plan was converted into material discovery",
	})
	if !ok {
		t.Fatal("BuildMaterialDiscoveryPlan ok=false")
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Kind != dataquery.DataActionMaterialInventory {
		t.Fatalf("actions=%+v, want material_inventory", plan.Actions)
	}
	if len(plan.CoverageContract.RequiredMaterials) != 0 || len(plan.CoverageContract.OptionalMaterials) != 0 {
		t.Fatalf("coverage=%+v, want cleared material floors during inventory", plan.CoverageContract)
	}
	if len(plan.CoverageContract.ValidationRules) != 1 {
		t.Fatalf("ValidationRules=%v, want audit rule", plan.CoverageContract.ValidationRules)
	}
}

func TestBuildMaterialDiscoveryTransitionUsesTypedTriggerFacts(t *testing.T) {
	paths := make([]string, DefaultBroadMaterialDiscoveryLimit)
	for i := range paths {
		paths[i] = fmt.Sprintf("input-%02d.csv", i)
	}
	plan, ok := BuildMaterialDiscoveryTransition(MaterialDiscoveryTransitionInput{
		Current:              dataquery.TaskPlan{Goal: "discover before compute"},
		Paths:                paths,
		HasExecutableSurface: true,
		BroadMaterialLimit:   DefaultBroadMaterialDiscoveryLimit,
	})
	if !ok {
		t.Fatal("BuildMaterialDiscoveryTransition ok=false")
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Kind != dataquery.DataActionMaterialInventory {
		t.Fatalf("actions=%+v, want material_inventory", plan.Actions)
	}
}

func TestBuildMaterialDiscoveryTransitionRejectsNonBroadOrAlreadyCovered(t *testing.T) {
	paths := make([]string, DefaultBroadMaterialDiscoveryLimit)
	for i := range paths {
		paths[i] = fmt.Sprintf("input-%02d.csv", i)
	}
	for name, input := range map[string]MaterialDiscoveryTransitionInput{
		"covered": {
			Paths:                      paths,
			HasExecutableSurface:       true,
			MaterialCoverageSufficient: true,
		},
		"prior_inventory": {
			Paths:                      paths,
			HasExecutableSurface:       true,
			PriorMaterialInventorySeen: true,
		},
		"non_custom_action": {
			Paths:                paths,
			HasExecutableSurface: true,
			NonCustomActionCount: 1,
		},
		"narrow_paths": {
			Paths:                paths[:DefaultBroadMaterialDiscoveryLimit-1],
			HasExecutableSurface: true,
		},
		"no_executable_surface": {
			Paths: paths,
		},
	} {
		if plan, ok := BuildMaterialDiscoveryTransition(input); ok {
			t.Fatalf("%s: plan=%+v, want no transition", name, plan)
		}
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

func TestBuildRequiredOutputProjectionPlanUsesOutputGraph(t *testing.T) {
	result := dataquery.Result{
		OutputContract: dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, ExplanationAllowed: false},
		Reconcile: &dataquery.ReconcileReport{Groups: []dataquery.ReconcileGroup{{
			GroupKey: dataquery.LooseText("A"),
			Metric:   dataquery.LooseText("value"),
			Actual:   dataquery.LooseText("10"),
		}}},
	}
	graph := BuildOutputProjectionGraph(OutputProjectionGraphInput{
		Output:           result.OutputContract,
		ReconcilePresent: true,
		ReconcileGroups:  1,
	})
	plan, ok := BuildRequiredOutputProjectionPlan(OutputProjectionPlanInput{
		Result:         result,
		OutputGraph:    graph,
		UseOutputGraph: true,
	})
	if !ok {
		t.Fatal("BuildRequiredOutputProjectionPlan ok=false, want graph-driven projection")
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Kind != dataquery.DataActionAssembleAnswer {
		t.Fatalf("actions=%+v, want assemble_answer", plan.Actions)
	}
}

func TestBuildRequiredOutputProjectionPlanSkipsSatisfiedOutputGraph(t *testing.T) {
	graph := BuildOutputProjectionGraph(OutputProjectionGraphInput{AnswerPresent: true})
	plan, ok := BuildRequiredOutputProjectionPlan(OutputProjectionPlanInput{
		OutputGraph:    graph,
		UseOutputGraph: true,
	})
	if ok || len(plan.Actions) > 0 {
		t.Fatalf("plan=%+v, ok=%t; satisfied output graph should not repair", plan, ok)
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

func TestBuildRequiredLedgerCompletionPlanUsesLedgerGraph(t *testing.T) {
	result := dataquery.Result{Contributions: []dataquery.ContributionRecord{{
		ItemID:    dataquery.LooseText("row-1"),
		GroupKey:  dataquery.LooseText("A"),
		Metric:    dataquery.LooseText("value"),
		Value:     dataquery.LooseText("10"),
		Operation: dataquery.LooseText("add"),
	}}}
	graph := BuildLedgerGraph(StageFacts{
		MaterialCoverageSufficient: true,
		ContributionLedgerRequired: true,
		ContributionRecords:        1,
		ReconcileRequired:          true,
	})
	plan, ok := BuildRequiredLedgerCompletionPlan(RequiredLedgerCompletionPlanInput{
		Current:        dataquery.TaskPlan{Goal: "finish validation"},
		Coverage:       dataquery.CoverageContract{ContributionLedgerRequired: true, ReconcileRequired: true},
		Result:         result,
		LedgerGraph:    graph,
		UseLedgerGraph: true,
		ErrorText:      "typed ledger guard without a dataquery json path",
	})
	if !ok {
		t.Fatal("BuildRequiredLedgerCompletionPlan ok=false, want graph-driven reconcile")
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Kind != dataquery.DataActionReconcile {
		t.Fatalf("actions=%+v, want reconcile_artifacts from ledger graph", plan.Actions)
	}
}

func TestBuildRequiredLedgerCompletionPlanDoesNotSkipEarlierLedgerGraphGap(t *testing.T) {
	graph := BuildLedgerGraph(StageFacts{
		MaterialCoverageSufficient: true,
		ContributionLedgerRequired: true,
		ReconcileRequired:          true,
	})
	plan, ok := BuildRequiredLedgerCompletionPlan(RequiredLedgerCompletionPlanInput{
		Coverage:       dataquery.CoverageContract{ContributionLedgerRequired: true, ReconcileRequired: true},
		LedgerGraph:    graph,
		UseLedgerGraph: true,
		ErrorText:      "typed ledger guard without a dataquery json path",
	})
	if ok || len(plan.Actions) > 0 {
		t.Fatalf("plan=%+v, ok=%t; missing contributions must not jump to reconcile", plan, ok)
	}
}

func TestBuildWorkflowNextStageFallbackPlanCompletesReconcileStage(t *testing.T) {
	result := dataquery.Result{Contributions: []dataquery.ContributionRecord{{
		ItemID:    dataquery.LooseText("row-1"),
		GroupKey:  dataquery.LooseText("A"),
		Metric:    dataquery.LooseText("value"),
		Value:     dataquery.LooseText("10"),
		Operation: dataquery.LooseText("add"),
	}}}
	plan, reason, ok := BuildWorkflowNextStageFallbackPlan(WorkflowNextStageFallbackPlanInput{
		Current: dataquery.TaskPlan{Status: "complete", Goal: "finish validation"},
		Coverage: dataquery.CoverageContract{
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Output: dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine},
		Facts: StageFacts{
			MaterialCoverageSufficient: true,
			ContributionRecords:        1,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Result:       &result,
		ReasonPrefix: "terminal plan ended",
	})
	if !ok {
		t.Fatal("BuildWorkflowNextStageFallbackPlan ok=false, want reconcile continuation")
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Kind != dataquery.DataActionReconcile {
		t.Fatalf("actions=%+v, want reconcile_artifacts", plan.Actions)
	}
	if !strings.Contains(reason, "required reconcile stage") {
		t.Fatalf("reason=%q, want reconcile stage reason", reason)
	}
}

func TestBuildWorkflowNextStageFallbackPlanProjectsFinalAnswer(t *testing.T) {
	result := dataquery.Result{
		Contributions: []dataquery.ContributionRecord{{
			ItemID:    dataquery.LooseText("row-1"),
			GroupKey:  dataquery.LooseText("A"),
			Metric:    dataquery.LooseText("value"),
			Value:     dataquery.LooseText("10"),
			Operation: dataquery.LooseText("add"),
		}},
		Reconcile: &dataquery.ReconcileReport{
			Status: dataquery.LooseText("pass"),
			Groups: []dataquery.ReconcileGroup{{
				GroupKey: dataquery.LooseText("A"),
				Metric:   dataquery.LooseText("value"),
				Actual:   dataquery.LooseText("10"),
			}},
		},
	}
	plan, reason, ok := BuildWorkflowNextStageFallbackPlan(WorkflowNextStageFallbackPlanInput{
		Current: dataquery.TaskPlan{Status: "complete", Goal: "format final values"},
		Coverage: dataquery.CoverageContract{
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Output: dataquery.OutputContract{Format: dataquery.OutputCSVLine, ExplanationAllowed: false},
		Facts: StageFacts{
			MaterialCoverageSufficient: true,
			ContributionRecords:        1,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
			HasReconcile:               true,
			HasAnswer:                  false,
		},
		Result:       &result,
		ReasonPrefix: "terminal plan ended",
	})
	if !ok {
		t.Fatal("BuildWorkflowNextStageFallbackPlan ok=false, want final projection continuation")
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Kind != dataquery.DataActionAssembleAnswer {
		t.Fatalf("actions=%+v, want assemble_answer", plan.Actions)
	}
	if plan.ContinueAfter {
		t.Fatalf("ContinueAfter=true, want terminal projection plan")
	}
	if !strings.Contains(reason, "answer projection") {
		t.Fatalf("reason=%q, want answer projection reason", reason)
	}
}

func TestBuildCompletionRepairTransitionUsesDeterministicLedgerPlan(t *testing.T) {
	transition := BuildCompletionRepairTransition(CompletionRepairTransitionInput{
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
	if !transition.Deterministic || transition.NeedsPlannerRepair || !transition.HasPlan() {
		t.Fatalf("transition=%+v, want deterministic plan", transition)
	}
	if len(transition.Plan.Actions) != 1 || transition.Plan.Actions[0].Kind != dataquery.DataActionReconcile {
		t.Fatalf("actions=%+v, want reconcile action", transition.Plan.Actions)
	}
}

func TestBuildCompletionRepairTransitionFallsBackToPlanner(t *testing.T) {
	transition := BuildCompletionRepairTransition(CompletionRepairTransitionInput{
		Current:   dataquery.TaskPlan{Goal: "finish validation"},
		ErrorText: "validate data workflow completion: semantic check failed",
	})
	if transition.Deterministic || !transition.NeedsPlannerRepair || transition.HasPlan() {
		t.Fatalf("transition=%+v, want planner repair fallback", transition)
	}
}
