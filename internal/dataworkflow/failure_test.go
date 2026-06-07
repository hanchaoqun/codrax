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
