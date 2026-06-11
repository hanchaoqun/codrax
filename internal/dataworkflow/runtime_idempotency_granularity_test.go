package dataworkflow

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

// Batch-6 E1 residual: a blocked plan must poison only the action(s)
// the typed violation attributes the failure to — a correct sibling
// action stays resubmittable in a repair plan.
func TestBlockedIdempotencyKeys_AttributedActionOnly(t *testing.T) {
	bad := dataquery.DataAction{ID: "bad_filter", Kind: dataquery.DataActionFilterRecords}
	good := dataquery.DataAction{ID: "good_transform", Kind: dataquery.DataActionCustomTransform, Script: "print(2*10)"}
	records := []WorkflowRecord{{
		Plan: dataquery.TaskPlan{Actions: []dataquery.DataAction{bad, good}},
		Err:  "data action failed",
		Violations: []dataquery.DataTaskViolation{{
			Code:     "action_param_violation",
			ActionID: "bad_filter",
		}},
	}}
	keys := map[string]bool{}
	for _, k := range blockedIdempotencyKeysFromRecords(records) {
		keys[k] = true
	}
	if !keys[ActionIdempotencyKey(bad)] {
		t.Fatal("the attributed failing action must be poisoned")
	}
	if keys[ActionIdempotencyKey(good)] {
		t.Fatal("a correct sibling action must stay resubmittable")
	}
}

// No action attribution (plan-shape rejection) keeps conservative
// whole-plan poisoning; so does a stale attribution naming no action.
func TestBlockedIdempotencyKeys_NoAttributionPoisonsPlan(t *testing.T) {
	a := dataquery.DataAction{ID: "a1", Kind: dataquery.DataActionFilterRecords}
	b := dataquery.DataAction{ID: "b1", Kind: dataquery.DataActionComputeContribs}

	records := []WorkflowRecord{{
		Plan: dataquery.TaskPlan{Actions: []dataquery.DataAction{a, b}},
		Err:  "data planning incomplete: plan is too large",
	}}
	keys := map[string]bool{}
	for _, k := range blockedIdempotencyKeysFromRecords(records) {
		keys[k] = true
	}
	if !keys[ActionIdempotencyKey(a)] || !keys[ActionIdempotencyKey(b)] {
		t.Fatal("plan-shape rejection must poison the whole plan")
	}

	records[0].Violations = []dataquery.DataTaskViolation{{Code: "x", ActionID: "not_in_plan"}}
	keys = map[string]bool{}
	for _, k := range blockedIdempotencyKeysFromRecords(records) {
		keys[k] = true
	}
	if !keys[ActionIdempotencyKey(a)] || !keys[ActionIdempotencyKey(b)] {
		t.Fatal("stale attribution must fall back to whole-plan poisoning")
	}
}
