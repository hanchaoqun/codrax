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
