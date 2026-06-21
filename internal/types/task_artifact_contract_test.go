package types

import "testing"

func TestTaskArtifactContracts_ProjectInputsOutputsOnly(t *testing.T) {
	g := TaskGraph{Nodes: []TaskNode{
		{ID: "probe", Type: NodeProbe, Outputs: []string{"file_candidates"}},
		{ID: "evidence", Type: NodeEvidence, Inputs: []string{"file_candidates"}, Outputs: []string{"evidence_items"}},
		{ID: "empty", Type: NodeValidate},
	}}
	got := TaskArtifactContracts(g)
	if len(got) != 2 {
		t.Fatalf("contract count = %d, want 2: %+v", len(got), got)
	}
	if got[0].NodeID != "probe" || len(got[0].Inputs) != 0 || got[0].Outputs[0] != "file_candidates" {
		t.Fatalf("probe contract mismatch: %+v", got[0])
	}
	if got[1].NodeID != "evidence" || got[1].Inputs[0] != "file_candidates" || got[1].Outputs[0] != "evidence_items" {
		t.Fatalf("evidence contract mismatch: %+v", got[1])
	}
	g.Nodes[0].Outputs[0] = "mutated"
	if got[0].Outputs[0] != "file_candidates" {
		t.Fatalf("contracts must clone node slices; got %+v", got[0])
	}
}

func TestExecutionTreeBacktrackNotDAG(t *testing.T) {
	g := TaskGraph{Edges: []TaskEdge{
		{From: "validate", To: "evidence_a", EdgeType: EdgeValidationFeedback, Guard: "missing_anchor"},
		{From: "validate", To: "evidence_b", EdgeType: EdgeValidationFeedback, Guard: "missing_anchor"},
		{From: "validate", To: "evidence_a", EdgeType: EdgeValidationFeedback, Guard: "duplicate"},
		{From: "probe", To: "evidence_a", EdgeType: EdgeHardDependency},
	}}
	points := ValidationFeedbackBranchPoints(g)
	if len(points) != 1 {
		t.Fatalf("branch point count = %d, want 1: %+v", len(points), points)
	}
	if points[0].FromNodeID != "validate" || points[0].EdgeType != EdgeValidationFeedback {
		t.Fatalf("branch point header mismatch: %+v", points[0])
	}
	if len(points[0].Targets) != 2 {
		t.Fatalf("deduped targets = %+v, want 2", points[0].Targets)
	}
	if points[0].Targets[0].NodeID != "evidence_a" || points[0].Targets[0].Guard != "missing_anchor" {
		t.Fatalf("target[0] mismatch: %+v", points[0].Targets[0])
	}
	if points[0].Targets[1].NodeID != "evidence_b" {
		t.Fatalf("target[1] mismatch: %+v", points[0].Targets[1])
	}
	targets := ValidationFeedbackTargets(g, "validate")
	if len(targets) != 2 || targets[0] != "evidence_a" || targets[1] != "evidence_b" {
		t.Fatalf("targets = %+v, want evidence_a/evidence_b", targets)
	}
	if got := ValidationFeedbackTargets(g, "missing"); len(got) != 0 {
		t.Fatalf("missing source targets = %+v, want none", got)
	}
}
