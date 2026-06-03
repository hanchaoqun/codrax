package operation

import (
	"strings"
	"testing"
)

func TestWorkflowInstanceTracksDAGEdgesAndQueue(t *testing.T) {
	wf := NewWorkflowInstance("wf-1", "make a deck", 4, 8)
	root, ok, diag := wf.AddAction("", "", WorkflowAction{
		Plan: Plan{
			Provider:      "skill:manual_reader",
			Kind:          "presentation_generation",
			TargetSurface: "slides",
		},
		Request: Request{Text: "read manual"},
		Depth:   0,
		Status:  WorkflowActionPending,
	})
	if !ok || diag != "" {
		t.Fatalf("add root ok=%t diag=%q", ok, diag)
	}
	child, ok, diag := wf.AddAction(root.ID, WorkflowEdgeNext, WorkflowAction{
		NextAction: WorkflowNextAction{
			Provider:      "skill:ppt_builder",
			OperationKind: "presentation_generation",
			TargetSurface: "slides",
			Request:       "build slides",
		},
		Depth: 1,
	})
	if !ok || diag != "" {
		t.Fatalf("add child ok=%t diag=%q", ok, diag)
	}
	ret, ok, diag := wf.AddAction(child.ID, WorkflowEdgeReturn, WorkflowAction{
		ReturnAction: WorkflowNextAction{
			Provider:      "skill:manual_reader",
			OperationKind: "artifact_generation",
			Request:       "compose final report",
		},
		Depth: 2,
	})
	if !ok || diag != "" {
		t.Fatalf("add return ok=%t diag=%q", ok, diag)
	}
	if len(wf.Actions) != 3 || len(wf.Edges) != 2 || len(wf.Queue) != 3 {
		t.Fatalf("unexpected graph sizes actions=%d edges=%d queue=%d wf=%+v", len(wf.Actions), len(wf.Edges), len(wf.Queue), wf)
	}
	if wf.Edges[0].Kind != WorkflowEdgeNext || wf.Edges[1].Kind != WorkflowEdgeReturn {
		t.Fatalf("edge kinds not preserved: %+v", wf.Edges)
	}
	if wf.CurrentID != root.ID {
		t.Fatalf("CurrentID=%q want %q", wf.CurrentID, root.ID)
	}
	if got, ok := wf.ActionByID(ret.ID); !ok || got.ID != ret.ID {
		t.Fatalf("return action lookup failed got=%+v ok=%t", got, ok)
	}
	compact := wf.Compact()
	for _, want := range []string{"workflow_id=wf-1", "queued=3", "actions=3", "edges=2"} {
		if !strings.Contains(compact, want) {
			t.Fatalf("compact missing %q: %s", want, compact)
		}
	}
}

func TestWorkflowInstanceBudgetRejectsDepthAndActionOverflow(t *testing.T) {
	wf := NewWorkflowInstance("wf-budget", "task", 1, 1)
	if _, ok, diag := wf.AddAction("", "", WorkflowAction{Depth: 0}); !ok || diag != "" {
		t.Fatalf("root rejected ok=%t diag=%q", ok, diag)
	}
	if _, ok, diag := wf.AddAction("wf-action-1", WorkflowEdgeNext, WorkflowAction{Depth: 1}); ok || !strings.Contains(diag, "action limit") {
		t.Fatalf("action limit not enforced ok=%t diag=%q", ok, diag)
	}
	wf = NewWorkflowInstance("wf-depth", "task", 1, 4)
	if _, ok, diag := wf.AddAction("", "", WorkflowAction{Depth: 2}); ok || !strings.Contains(diag, "depth limit") {
		t.Fatalf("depth limit not enforced ok=%t diag=%q", ok, diag)
	}
}
