package types

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadRunSnapshotFromBusContextCarriesTypedState(t *testing.T) {
	mut := NewMutableState("which agent calls subagents")
	mut.SetRepoRoot("/repo")
	closure := mut.EvidenceClosure()
	closure.SetNodeExecStatus("explore", NodeExecDone)
	closure.IncrementNodeExecAttempt("explore")
	closure.IncrementNodeExecAttempt("explore")
	closure.AddReadSet(map[string]bool{"internal/agent/agent.go": true})
	closure.AddReadRanges(map[string][]LineRange{
		"internal/agent/agent.go": {{Start: 10, End: 20}},
	})
	closure.RecordFileTotalLines("internal/agent/agent.go", 200)
	closure.AppendAcceptedEvidenceRefs([]AcceptedEvidenceRef{{
		ID:        "ev-agent",
		Source:    "internal/agent/agent.go",
		LineStart: 10,
	}})
	closure.RecordSourceInventoryObservation(SourceInventoryObservation{
		Active:   true,
		Complete: true,
		SourceClasses: []SourceInventorySourceClassCount{{
			Role:     SourcePathRoleProduction,
			Count:    1,
			Complete: true,
		}},
	})
	closure.RecordDowngradeProgressDelta(DowngradeLaneContractChain, 42, 3)

	ctx := &BusContext{
		RepoRoot: "/repo",
		Mutable:  mut,
		AnalysisIR: &AnalysisIR{
			RequestModel: RequestModel{RawRequest: "which agent calls subagents"},
			TaskGraph: TaskGraph{Nodes: []TaskNode{
				{ID: "explore", Type: NodeEvidence},
				{ID: "final", Type: NodeFinalize},
			}},
		},
	}

	snapshot := ReadRunSnapshotFromBusContext(ctx, "read-1")
	if snapshot.SchemaVersion != ReadRunSnapshotSchemaVersion || snapshot.RunID != "read-1" {
		t.Fatalf("snapshot identity = version %d run %q", snapshot.SchemaVersion, snapshot.RunID)
	}
	if snapshot.TaskGraphHash == "" || snapshot.TaskNodeCount != 2 {
		t.Fatalf("task graph identity missing: hash=%q nodes=%d", snapshot.TaskGraphHash, snapshot.TaskNodeCount)
	}
	if snapshot.NodeStatuses["explore"] != NodeExecDone {
		t.Fatalf("node statuses = %+v, want explore done", snapshot.NodeStatuses)
	}
	if snapshot.NodeAttempts["explore"] != 2 {
		t.Fatalf("node attempts = %+v, want explore=2", snapshot.NodeAttempts)
	}
	if len(snapshot.ReadSet) != 1 || snapshot.ReadSet[0] != "internal/agent/agent.go" {
		t.Fatalf("read set = %+v", snapshot.ReadSet)
	}
	if ranges := snapshot.ReadRanges["internal/agent/agent.go"]; len(ranges) != 1 || ranges[0].Start != 10 || ranges[0].End != 20 {
		t.Fatalf("read ranges = %+v", snapshot.ReadRanges)
	}
	if snapshot.FileTotals["internal/agent/agent.go"] != 200 {
		t.Fatalf("file totals = %+v", snapshot.FileTotals)
	}
	if len(snapshot.AcceptedEvidence) != 1 || snapshot.AcceptedEvidence[0].ID != "ev-agent" {
		t.Fatalf("accepted evidence = %+v", snapshot.AcceptedEvidence)
	}
	if !snapshot.SourceInventory.IsActive() || len(snapshot.SourceInventory.SourceClasses) != 1 {
		t.Fatalf("source inventory = %+v", snapshot.SourceInventory)
	}
	if snapshot.ProgressDecision.ReasonCode == "" {
		t.Fatalf("progress decision missing: %+v", snapshot.ProgressDecision)
	}
}

func TestReadRunSnapshotFileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "read-1.json")
	original := &ReadRunSnapshot{
		RunID:        "read-1",
		Request:      "where is agent runtime",
		NodeStatuses: map[string]NodeExecStatus{"explore": NodeExecDone},
		NodeAttempts: map[string]int{"explore": 2, "zero": 0},
		ReadSet:      []string{"b.go", "a.go", "a.go"},
	}
	if err := WriteReadRunSnapshotToFile(original, path); err != nil {
		t.Fatalf("WriteReadRunSnapshotToFile: %v", err)
	}
	loaded, err := LoadReadRunSnapshotFromFile(path)
	if err != nil {
		t.Fatalf("LoadReadRunSnapshotFromFile: %v", err)
	}
	if loaded == nil || loaded.RunID != "read-1" || len(loaded.ReadSet) != 2 || loaded.ReadSet[0] != "a.go" {
		t.Fatalf("loaded snapshot = %+v", loaded)
	}
	if loaded.NodeAttempts["explore"] != 2 || loaded.NodeAttempts["zero"] != 0 {
		t.Fatalf("loaded node attempts = %+v", loaded.NodeAttempts)
	}
	if tmp := path + ".tmp"; fileExists(tmp) {
		t.Fatalf("tmp file should not remain: %s", tmp)
	}
}

func TestNormalizeReadRunSnapshotPreservesExplicitSchemaVersion(t *testing.T) {
	zero := NormalizeReadRunSnapshot(ReadRunSnapshot{RunID: "zero-schema"})
	if zero.SchemaVersion != ReadRunSnapshotSchemaVersion {
		t.Fatalf("zero schema should default to current, got %d", zero.SchemaVersion)
	}
	explicit := NormalizeReadRunSnapshot(ReadRunSnapshot{
		SchemaVersion: ReadRunSnapshotSchemaVersion + 10,
		RunID:         "future-schema",
	})
	if explicit.SchemaVersion != ReadRunSnapshotSchemaVersion+10 {
		t.Fatalf("explicit schema mismatch must be preserved for resume validation, got %d", explicit.SchemaVersion)
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
