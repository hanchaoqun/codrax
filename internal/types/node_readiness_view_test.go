package types

import "testing"

func TestNormalizeNodeReadinessViewCanonicalizesClosedFields(t *testing.T) {
	view := NormalizeNodeReadinessView(NodeReadinessView{
		NodeID:             " n1 ",
		Status:             NodeExecStatus(" running "),
		Ready:              true,
		ReasonCode:         NodeReadinessReasonCode("waiting_on_dependency"),
		WaitingOn:          []string{" b ", "a", "b", ""},
		FailedPredecessors: []string{"z", " z ", "y"},
		FailedCriteria: []NodeReadinessCriterionFailure{
			{Kind: " has_enough_facts ", Expr: " >=1 ", Detail: " missing "},
			{Kind: "has_enough_facts", Expr: ">=1", Detail: "missing"},
			{Kind: "", Expr: "", Detail: ""},
		},
		AttemptsUsed: -1,
		MaxAttempts:  -2,
	})
	if view.NodeID != "n1" || view.Status != NodeExecRunning ||
		view.ReasonCode != NodeReadinessReasonWaitingDependency {
		t.Fatalf("normalized identity/status/reason = %+v", view)
	}
	if got := joinReadinessTestStrings(view.WaitingOn); got != "a,b" {
		t.Fatalf("waiting_on = %q", got)
	}
	if got := joinReadinessTestStrings(view.FailedPredecessors); got != "y,z" {
		t.Fatalf("failed_predecessors = %q", got)
	}
	if len(view.FailedCriteria) != 1 ||
		view.FailedCriteria[0].Kind != "has_enough_facts" ||
		view.FailedCriteria[0].Expr != ">=1" ||
		view.FailedCriteria[0].Detail != "missing" {
		t.Fatalf("failed criteria not canonicalized: %+v", view.FailedCriteria)
	}
	if view.AttemptsUsed != 0 || view.MaxAttempts != 0 {
		t.Fatalf("negative attempts should clamp to zero: %+v", view)
	}
	if got := NormalizeNodeReadinessReasonCode("bogus"); got != NodeReadinessReasonUnknown {
		t.Fatalf("unknown reason = %q", got)
	}
}

func TestCloneNodeReadinessViewIsolated(t *testing.T) {
	original := NormalizeNodeReadinessView(NodeReadinessView{
		NodeID:             "n1",
		ReasonCode:         NodeReadinessReasonFailedDependency,
		WaitingOn:          []string{"n0"},
		FailedPredecessors: []string{"n2"},
		FailedCriteria: []NodeReadinessCriterionFailure{
			{Kind: "k", Expr: "e", Detail: "d"},
		},
	})
	clone := CloneNodeReadinessView(original)
	clone.WaitingOn[0] = "mutated"
	clone.FailedPredecessors[0] = "mutated"
	clone.FailedCriteria[0].Kind = "mutated"
	if original.WaitingOn[0] != "n0" ||
		original.FailedPredecessors[0] != "n2" ||
		original.FailedCriteria[0].Kind != "k" {
		t.Fatalf("clone mutated original: original=%+v clone=%+v", original, clone)
	}
}

func joinReadinessTestStrings(values []string) string {
	out := ""
	for i, value := range values {
		if i > 0 {
			out += ","
		}
		out += value
	}
	return out
}
