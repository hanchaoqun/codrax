package dataworkflow

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestRepeatedNodeFailureFromErrorsUsesTypedViolationNode(t *testing.T) {
	errText := `execute data task: data action failed action_id="node_1" action_kind="custom_transform": boom`
	key, count, repeated := RepeatedNodeFailureFromErrors([]string{errText}, errText, 2)
	if !repeated {
		t.Fatal("repeated=false, want true")
	}
	if key != "node_1|custom_transform" || count != 2 {
		t.Fatalf("key/count=%q/%d, want node_1|custom_transform/2", key, count)
	}
}

func TestBuildRepeatedFailureReplacementPlanUsesConcreteScaffold(t *testing.T) {
	errText := `execute data task: data action failed action_id="filter_1" action_kind="filter_records": zero rows`
	plan, reason, ok := BuildRepeatedFailureReplacementPlan(RepeatedFailureReplacementPlanInput{
		Current: dataquery.TaskPlan{
			Goal: "finish grouped calculation",
		},
		Coverage: dataquery.CoverageContract{
			ContributionLedgerRequired: true,
		},
		Output: dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine},
		Facts: StageFacts{
			MaterialCoverageSufficient: true,
			ContributionLedgerRequired: true,
		},
		PreviousErrors: []string{errText},
		CurrentError:   errText,
		FailureLimit:   2,
		Scaffolds: []ActionScaffold{{
			Kind:       string(dataquery.DataActionValueDistribution),
			Executable: true,
			InputPath:  "records.json",
			Fields:     []string{"status", "amount"},
		}},
	})
	if !ok {
		t.Fatal("BuildRepeatedFailureReplacementPlan ok=false, want concrete replacement plan")
	}
	if !strings.Contains(reason, "filter_1|filter_records failed 2 times") {
		t.Fatalf("reason=%q, want repeated node context", reason)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Kind != dataquery.DataActionValueDistribution {
		t.Fatalf("actions=%+v, want value_distribution replacement", plan.Actions)
	}
	if !plan.ContinueAfter || plan.CoverageContract.ContributionLedgerRequired == false {
		t.Fatalf("plan=%+v, want continuation preserving coverage", plan)
	}
}

func TestBuildRepeatedFailureReplacementPlanRejectsPromptOnlyScaffold(t *testing.T) {
	errText := `execute data task: data action failed action_id="filter_1" action_kind="filter_records": zero rows`
	_, _, ok := BuildRepeatedFailureReplacementPlan(RepeatedFailureReplacementPlanInput{
		PreviousErrors: []string{errText},
		CurrentError:   errText,
		FailureLimit:   2,
		Scaffolds: []ActionScaffold{{
			Kind:         string(dataquery.DataActionJoinRecords),
			InputPaths:   []string{"left.json", "right.json"},
			CommonFields: []string{"id"},
		}},
	})
	if ok {
		t.Fatal("BuildRepeatedFailureReplacementPlan ok=true, want prompt-only scaffold rejected")
	}
}

func TestBuildExecutionFailureTransitionUsesTypedMissingFieldFallback(t *testing.T) {
	violation := dataquery.DataTaskViolation{
		Code:          "field_contract_violation",
		ActionKind:    string(dataquery.DataActionJoinRecords),
		InputAlias:    "orders.json",
		MissingFields: []string{"canonical_code"},
	}
	transition := BuildExecutionFailureTransition(ExecutionFailureTransitionInput{
		Current: dataquery.TaskPlan{Status: "ready"},
		FailureRecord: WorkflowRecord{
			Err:        "execute data task: join failed",
			Violations: []dataquery.DataTaskViolation{violation},
		},
		Violation: violation,
		SchemaProjections: []ArtifactSchemaProjection{
			{ID: "orders", Kind: string(dataquery.DataActionFilterRecords), NodeClass: ArtifactNodeClassRecord, Aliases: []string{"orders.json"}, Fields: []string{"vendor_name", "amount"}},
			{ID: "mapping", Kind: string(dataquery.DataActionNormalizeEntities), Aliases: []string{"mapping.json"}, Fields: []string{"vendor_name", "canonical_code"}},
		},
	})
	if transition.Action != ExecutionFailureFallbackPlan || !transition.HasPlan() {
		t.Fatalf("transition=%+v, want fallback plan", transition)
	}
	if len(transition.Plan.Actions) != 1 || transition.Plan.Actions[0].Kind != dataquery.DataActionEnrichRecords {
		t.Fatalf("plan actions=%+v, want enrich_records fallback", transition.Plan.Actions)
	}
	if !strings.Contains(transition.Reason, "missing join field") {
		t.Fatalf("reason=%q, want missing-field context", transition.Reason)
	}
}

func TestBuildExecutionFailureTransitionUsesComputeGroupFieldFallback(t *testing.T) {
	violation := dataquery.DataTaskViolation{
		Code:          "field_contract_violation",
		ActionID:      "compute_target_contributions",
		ActionKind:    string(dataquery.DataActionComputeContribs),
		Role:          "contribution_group_key",
		InputAlias:    "coverage_records.json",
		MissingFields: []string{"canonical_label"},
	}
	transition := BuildExecutionFailureTransition(ExecutionFailureTransitionInput{
		Current: dataquery.TaskPlan{
			Status: "ready",
			Actions: []dataquery.DataAction{{
				ID:         "compute_target_contributions",
				Kind:       dataquery.DataActionComputeContribs,
				InputPaths: []string{"coverage_records.json"},
				Params: map[string]string{
					"group_key_field": "canonical_label",
					"value_field":     "value",
					"filters_json":    `[{"field":"active","op":"eq","value":true}]`,
				},
			}},
		},
		FailureRecord: WorkflowRecord{
			Err:        "execute data task: compute grouped contributions failed",
			Violations: []dataquery.DataTaskViolation{violation},
		},
		Violation: violation,
		SchemaProjections: []ArtifactSchemaProjection{
			{ID: "coverage", Kind: string(dataquery.DataActionExtractRecords), NodeClass: ArtifactNodeClassRecord, Aliases: []string{"coverage_records.json"}, Fields: []string{"active", "raw_label", "value", "canonical_label"}},
			{ID: "qualified", Kind: string(dataquery.DataActionFilterRecords), NodeClass: ArtifactNodeClassRecord, Aliases: []string{"qualified_observations.json"}, Fields: []string{"active", "raw_label", "value"}},
			{ID: "labels", Kind: string(dataquery.DataActionExtractRecords), NodeClass: ArtifactNodeClassRecord, Aliases: []string{"labels.csv#records"}, Fields: []string{"raw_label", "canonical_label"}},
		},
	})
	if transition.Action != ExecutionFailureFallbackPlan || !transition.HasPlan() {
		t.Fatalf("transition=%+v, want fallback plan", transition)
	}
	if len(transition.Plan.Actions) != 1 || transition.Plan.Actions[0].Kind != dataquery.DataActionEnrichRecords {
		t.Fatalf("plan actions=%+v, want enrich_records fallback", transition.Plan.Actions)
	}
	if transition.Plan.Actions[0].Params["base_path"] != "qualified_observations.json" {
		t.Fatalf("base_path=%q, want focused grouped contribution base", transition.Plan.Actions[0].Params["base_path"])
	}
	if !strings.Contains(transition.Reason, "contribution group field") {
		t.Fatalf("reason=%q, want contribution group field context", transition.Reason)
	}
}

func TestBuildExecutionFailureTransitionReplacesRepeatedNodeWithScaffold(t *testing.T) {
	errText := `execute data task: data action failed action_id="filter_1" action_kind="filter_records": zero rows`
	transition := BuildExecutionFailureTransition(ExecutionFailureTransitionInput{
		Current: dataquery.TaskPlan{Goal: "finish calculation"},
		Coverage: dataquery.CoverageContract{
			ContributionLedgerRequired: true,
		},
		Output: dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine},
		State: WorkflowStateView{
			MaterialCoverageSufficient: true,
			ContributionLedgerRequired: true,
			AllowedNextActions:         []string{string(dataquery.DataActionValueDistribution)},
			ActionScaffold: []ActionScaffold{{
				Kind:       string(dataquery.DataActionValueDistribution),
				Executable: true,
				InputPath:  "records.json",
				Fields:     []string{"status", "amount"},
			}},
		},
		PreviousErrors: []string{errText},
		ErrorText:      errText,
		FailureLimit:   2,
	})
	if transition.Action != ExecutionFailureFallbackPlan || len(transition.Plan.Actions) != 1 {
		t.Fatalf("transition=%+v, want one fallback action", transition)
	}
	if transition.Plan.Actions[0].Kind != dataquery.DataActionValueDistribution {
		t.Fatalf("action=%+v, want value_distribution", transition.Plan.Actions[0])
	}
	if !strings.Contains(transition.Reason, "filter_1|filter_records failed 2 times") {
		t.Fatalf("reason=%q, want repeated-node reason", transition.Reason)
	}
}

func TestBuildExecutionFailureTransitionRequestsContinuationWhenRepeatedWithoutFallback(t *testing.T) {
	errText := `execute data task: data action failed action_id="filter_1" action_kind="filter_records": zero rows`
	transition := BuildExecutionFailureTransition(ExecutionFailureTransitionInput{
		PreviousErrors: []string{errText},
		ErrorText:      errText,
		FailureLimit:   2,
	})
	if transition.Action != ExecutionFailureNeedsContinuation {
		t.Fatalf("transition=%+v, want continuation request", transition)
	}
	if transition.NodeKey != "filter_1|filter_records" || transition.NodeCount != 2 {
		t.Fatalf("transition=%+v, want repeated node metadata", transition)
	}
	if transition.HasPlan() {
		t.Fatalf("transition has plan=%+v, continuation transition should not invent plan", transition.Plan)
	}
}

func TestDecideExecutionFailureRecoveryPrioritizesFallback(t *testing.T) {
	plan := dataquery.TaskPlan{Status: "ready", Actions: []dataquery.DataAction{{ID: "inspect", Kind: dataquery.DataActionValueDistribution}}}
	decision := DecideExecutionFailureRecovery(ExecutionFailureTransition{
		Action: ExecutionFailureFallbackPlan,
		Plan:   plan,
		Reason: "typed fallback",
	}, true, true, true, "error")
	if decision.Action != FailureRecoveryFallbackPlan || !decision.HasPlan() || decision.Reason != "typed fallback" {
		t.Fatalf("decision=%+v, want typed fallback", decision)
	}
}

func TestDecidePlannerPlanResultAcceptsExecutablePlan(t *testing.T) {
	plan := dataquery.TaskPlan{Status: "ready", Actions: []dataquery.DataAction{{ID: "continue", Kind: dataquery.DataActionFilterRecords}}}
	decision := DecidePlannerPlanResult(PlannerPlanDecisionInput{Plan: plan})
	if decision.Action != PlannerPlanAccept || !decision.HasPlan() || decision.Plan.Actions[0].ID != "continue" {
		t.Fatalf("decision=%+v, want accepted planner plan", decision)
	}
	decision.Plan.Actions[0].ID = "mutated"
	if plan.Actions[0].ID != "continue" {
		t.Fatalf("planner plan leaked mutation: %+v", plan)
	}
}

func TestDecidePlannerPlanResultUsesFallbackOnPlannerError(t *testing.T) {
	fallback := dataquery.TaskPlan{Status: "ready", Actions: []dataquery.DataAction{{ID: "derive", Kind: dataquery.DataActionDeriveRules}}}
	decision := DecidePlannerPlanResult(PlannerPlanDecisionInput{
		ErrorText:         "planner returned no tool call",
		FallbackPlan:      fallback,
		FallbackReason:    "continue from typed state",
		FallbackAvailable: true,
		FailurePrefix:     "continue data task",
	})
	if decision.Action != PlannerPlanFallbackPlan || decision.Reason != "continue from typed state" || decision.Plan.Actions[0].ID != "derive" {
		t.Fatalf("decision=%+v, want deterministic fallback", decision)
	}
}

func TestDecidePlannerPlanResultFailsWithoutExecutablePlanOrFallback(t *testing.T) {
	decision := DecidePlannerPlanResult(PlannerPlanDecisionInput{FailurePrefix: "continue data task"})
	if decision.Action != PlannerPlanFail || decision.Reason != "continue data task" {
		t.Fatalf("decision=%+v, want no-plan failure", decision)
	}
	decision = DecidePlannerPlanResult(PlannerPlanDecisionInput{
		ErrorText:     "planner failed",
		FailurePrefix: "continue data task",
	})
	if decision.Action != PlannerPlanFail || decision.Reason != "continue data task: planner failed" {
		t.Fatalf("decision=%+v, want prefixed planner failure", decision)
	}
}

func TestDecideExecutionFailureRecoveryContinuationFallsBackToRepair(t *testing.T) {
	decision := DecideExecutionFailureRecovery(ExecutionFailureTransition{
		Action: ExecutionFailureNeedsContinuation,
		Reason: "expand graph",
	}, true, true, true, "error")
	if decision.Action != FailureRecoveryContinuation {
		t.Fatalf("decision=%+v, want continuation", decision)
	}
	decision = DecideExecutionFailureRecovery(ExecutionFailureTransition{
		Action: ExecutionFailureNeedsContinuation,
		Reason: "expand graph",
	}, false, true, true, "error")
	if decision.Action != FailureRecoveryRepair {
		t.Fatalf("decision=%+v, want repair when continuation unavailable", decision)
	}
}

func TestDecideExecutionFailureRecoveryFailsWithoutRepairBudget(t *testing.T) {
	decision := DecideExecutionFailureRecovery(ExecutionFailureTransition{
		Action: ExecutionFailureNeedsRepair,
		Reason: "node failed",
	}, false, true, false, "error")
	if decision.Action != FailureRecoveryFail || decision.Reason != "node failed" {
		t.Fatalf("decision=%+v, want fail without repair budget", decision)
	}
}

func TestDecideValidationFailureRecoveryUsesFallbackBeforeRepair(t *testing.T) {
	guard := NewGuardResult("missing_answer", "error", RepairNeedsTypedAction, "final answer missing", WorkflowViolation{Code: "missing_answer"})
	plan := dataquery.TaskPlan{Status: "ready", Actions: []dataquery.DataAction{{ID: "assemble", Kind: dataquery.DataActionAssembleAnswer}}}
	decision := DecideValidationFailureRecovery(ValidationFailureTransition{
		Action:    ValidationFailureFallbackPlan,
		Plan:      plan,
		Reason:    "assemble answer",
		ErrorText: "validation failed",
		Guard:     guard,
	}, true, true, "fallback")
	if decision.Action != FailureRecoveryFallbackPlan || !decision.HasPlan() || decision.Guard.Code != "missing_answer" {
		t.Fatalf("decision=%+v, want validation fallback", decision)
	}
}

func TestDecideValidationFailureRecoveryRepairsOrFails(t *testing.T) {
	decision := DecideValidationFailureRecovery(ValidationFailureTransition{
		Action:    ValidationFailureNeedsRepair,
		ErrorText: "validation failed",
	}, true, true, "fallback")
	if decision.Action != FailureRecoveryRepair || decision.Reason != "validation failed" {
		t.Fatalf("decision=%+v, want repair", decision)
	}
	decision = DecideValidationFailureRecovery(ValidationFailureTransition{
		Action:    ValidationFailureNeedsRepair,
		ErrorText: "validation failed",
	}, true, false, "fallback")
	if decision.Action != FailureRecoveryFail || decision.Reason != "validation failed" {
		t.Fatalf("decision=%+v, want fail without repair budget", decision)
	}
}

func TestBuildRepairFailureTransitionRequestsContinuationFromTypedState(t *testing.T) {
	transition := BuildRepairFailureTransition(RepairFailureTransitionInput{
		Records: []WorkflowRecord{{
			Plan: dataquery.TaskPlan{Actions: []dataquery.DataAction{{ID: "extract", Kind: dataquery.DataActionExtractRecords}}},
		}},
		NoStructuredRepairPlan:   true,
		ContinuationPlannerReady: true,
		RepairFailureReasonCode:  "repair_planner_no_tool",
	})
	if transition.Action != RepairFailureNeedsContinuation {
		t.Fatalf("transition=%+v, want continuation", transition)
	}
	if transition.ReasonCode != "repair_planner_no_tool" || transition.Reason == "" {
		t.Fatalf("transition=%+v, want typed reason", transition)
	}
}

func TestBuildRepairFailureTransitionFallsThroughWhenContinuationUnavailable(t *testing.T) {
	transition := BuildRepairFailureTransition(RepairFailureTransitionInput{
		Records:                  []WorkflowRecord{{Plan: dataquery.TaskPlan{Status: "ready"}}},
		NoStructuredRepairPlan:   true,
		ContinuationPlannerReady: false,
	})
	if transition.Action != RepairFailureNeedsFailure {
		t.Fatalf("transition=%+v, want terminal failure", transition)
	}
}

func TestRepeatedCustomTransformGuardResultReturnsTypedViolation(t *testing.T) {
	errText := `execute data task: data action failed action_id="clean" action_kind="custom_transform": custom_transform field contract failed: line 3 references missing field "x"`
	result := RepeatedCustomTransformGuardResult(
		dataquery.DataAction{ID: "clean", Kind: dataquery.DataActionCustomTransform},
		[]string{errText, errText},
		2,
	)
	if result.Empty() {
		t.Fatal("guard result empty, want repeated custom_transform guard")
	}
	if result.Code != "repeated_custom_transform_node" || len(result.Violations) != 1 {
		t.Fatalf("result=%+v, want typed repeated node violation", result)
	}
	if result.Violations[0].IdempotencyKey != "clean|custom_transform" {
		t.Fatalf("violation=%+v, want idempotency key", result.Violations[0])
	}
}

func TestRepeatedCustomTransformClassGuardResultUsesFailureClassStats(t *testing.T) {
	errs := []string{
		`execute data task: data action failed action_id="a" action_kind="custom_transform": custom_transform field contract failed: missing x`,
		`execute data task: data action failed action_id="b" action_kind="custom_transform": custom_transform field contract failed: missing y`,
	}
	result := RepeatedCustomTransformClassGuardResult(
		dataquery.DataAction{ID: "c", Kind: dataquery.DataActionCustomTransform, Script: "emit({})"},
		true,
		errs,
		3,
		2,
	)
	if result.Empty() {
		t.Fatal("guard result empty, want repeated custom_transform class guard")
	}
	if !strings.Contains(result.ErrorText(), "most_common_failure=custom_transform_field_contract") {
		t.Fatalf("message=%q, want top failure code", result.ErrorText())
	}
}
