package types

import "testing"

func TestExecutionPolicyAuditWarnings(t *testing.T) {
	graph := TaskGraph{
		Nodes: []TaskNode{
			{ID: "a"},
			{ID: "b"},
		},
		ExecutionPolicy: ExecutionPolicy{
			CriticalPath: []string{"b", "missing", "b", "  "},
		},
	}
	got := ExecutionPolicyAuditWarnings(graph)
	if len(got) != 3 {
		t.Fatalf("warnings len=%d, want 3: %+v", len(got), got)
	}
	if got[0].Code != ExecutionPolicyAuditMissingCriticalPathNode || got[0].NodeID != "missing" || got[0].PathIndex != 1 {
		t.Fatalf("missing-node warning mismatch: %+v", got[0])
	}
	if got[1].Code != ExecutionPolicyAuditDuplicateCriticalPathID || got[1].NodeID != "b" || got[1].PathIndex != 2 || got[1].FirstIndex != 0 {
		t.Fatalf("duplicate warning mismatch: %+v", got[1])
	}
	if got[2].Code != ExecutionPolicyAuditBlankCriticalPathID || got[2].PathIndex != 3 {
		t.Fatalf("blank warning mismatch: %+v", got[2])
	}
}

func TestExecutionPolicyAuditWarningsNoneForValidPolicy(t *testing.T) {
	graph := TaskGraph{
		Nodes: []TaskNode{
			{ID: "a"},
			{ID: "b"},
		},
		ExecutionPolicy: ExecutionPolicy{CriticalPath: []string{"b", "a"}},
	}
	if got := ExecutionPolicyAuditWarnings(graph); len(got) != 0 {
		t.Fatalf("valid policy should not warn: %+v", got)
	}
}
