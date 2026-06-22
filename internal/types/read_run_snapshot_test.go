package types

import (
	"os"
	"path/filepath"
	"strings"
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
	closure.AppendNodeArtifactRecords([]NodeArtifactRecord{{
		ProducerNodeID: "explore",
		ProducerStage:  StageExplore,
		Artifact:       RuntimeArtifactRef{Kind: RuntimeArtifactEvidenceItem, ID: "ev-agent"},
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
		RepoRoot:              "/repo",
		Mutable:               mut,
		AttachedLog:           "panic: sample\n",
		AttachedHitrace:       "sched_switch prev=app next=ui\n",
		AttachedHitraceSource: "android_atrace",
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
	if snapshot.RequestHash == "" || snapshot.RequestHash != ReadRunRequestHash("which agent calls subagents") {
		t.Fatalf("request hash = %q", snapshot.RequestHash)
	}
	if logFP, ok := ReadRunAttachmentFingerprintByKind(snapshot.Attachments, ReadRunAttachmentKindLog); !ok || !logFP.Present || logFP.Bytes == 0 || logFP.Hash == "" {
		t.Fatalf("log attachment fingerprint missing: %+v", snapshot.Attachments)
	}
	if traceFP, ok := ReadRunAttachmentFingerprintByKind(snapshot.Attachments, ReadRunAttachmentKindTrace); !ok || !traceFP.Present || traceFP.Source != "android_atrace" || traceFP.Hash == "" {
		t.Fatalf("trace attachment fingerprint missing: %+v", snapshot.Attachments)
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
	if len(snapshot.NodeArtifacts) != 1 || snapshot.NodeArtifacts[0].ProducerNodeID != "explore" {
		t.Fatalf("node artifacts = %+v", snapshot.NodeArtifacts)
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
		RunID:   "read-1",
		Request: "where is agent runtime",
		RepoFingerprint: ReadRunRepoFingerprint{
			Kind:       ReadRunRepoFingerprintKindGitHead,
			Available:  true,
			Head:       "abcdef",
			StatusHash: "123456",
		},
		Environment: ReadRunEnvironmentFingerprint{
			Kind:            ReadRunEnvironmentKindCodrax,
			Available:       true,
			CodraxVersion:   "0.1.test",
			CodraxBuildTime: "2026-06-22T00:00:00Z",
			GoVersion:       "go1.22.5",
			GOOS:            "darwin",
			GOARCH:          "arm64",
			Tools: []ReadRunToolFingerprint{{
				Name:        "git",
				Available:   true,
				Executable:  "/usr/bin/git",
				VersionHash: strings.Repeat("b", 64),
			}},
			Configs: []ReadRunConfigFingerprint{
				ReadRunConfigFingerprintFromStringSlice(ReadRunConfigFingerprintSearchExcludeRoots, []string{"out", ".codrax"}),
			},
		},
		Attachments: []ReadRunAttachmentFingerprint{
			ReadRunAttachmentFingerprintFromPayload(ReadRunAttachmentKindLog, "panic: one\n", ""),
			ReadRunAttachmentFingerprintFromPayload(ReadRunAttachmentKindTrace, "sched_switch\n", "harmony_hitrace"),
		},
		NodeStatuses: map[string]NodeExecStatus{"explore": NodeExecDone},
		NodeAttempts: map[string]int{"explore": 2, "zero": 0},
		NodeArtifacts: []NodeArtifactRecord{
			{
				ProducerNodeID: "explore",
				ProducerStage:  StageExplore,
				Artifact:       RuntimeArtifactRef{Kind: RuntimeArtifactEvidenceItem, ID: "ev-1"},
			},
			{
				ProducerNodeID: "explore",
				ProducerStage:  StageExplore,
				Artifact:       RuntimeArtifactRef{Kind: RuntimeArtifactEvidenceItem, ID: "ev-1"},
			},
		},
		ReadSet: []string{"b.go", "a.go", "a.go"},
		ActiveState: ReadRunActiveState{
			TransientRetryPending:    true,
			TransientRetryReasonCode: ReadRunTransientRetryReasonCheckpoint,
			TransientRetryHintHash:   strings.Repeat("a", 64),
			TransientRetryHintBytes:  128,
			ReadLoopNextAction: ReadRunNextActionState{
				Active:      true,
				Action:      ReadDispatchPolicyActionAddProof,
				ReasonCode:  "proof_weak",
				ProofState:  "weak",
				TruthAction: TruthActionAddProof,
			},
			ReadDispatchPolicy: ReadDispatchPolicy{
				Active:       true,
				Action:       ReadDispatchPolicyActionAddProof,
				AllowedTools: []string{"read_file", "emit_evidence"},
				ScopePaths:   []string{"pkg/a.py"},
				MaxToolCalls: 2,
				OneShot:      true,
			},
		},
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
	if loaded.RequestHash != ReadRunRequestHash("where is agent runtime") {
		t.Fatalf("loaded request hash = %q", loaded.RequestHash)
	}
	if !loaded.RepoFingerprint.Available || loaded.RepoFingerprint.Head != "abcdef" {
		t.Fatalf("loaded repo fingerprint = %+v", loaded.RepoFingerprint)
	}
	if !loaded.Environment.Available || loaded.Environment.CodraxVersion != "0.1.test" ||
		len(loaded.Environment.Tools) != 1 || len(loaded.Environment.Configs) != 1 {
		t.Fatalf("loaded environment fingerprint = %+v", loaded.Environment)
	}
	if traceFP, ok := ReadRunAttachmentFingerprintByKind(loaded.Attachments, ReadRunAttachmentKindTrace); !ok || traceFP.Source != "harmony_hitrace" || traceFP.Hash == "" {
		t.Fatalf("loaded attachments = %+v", loaded.Attachments)
	}
	if loaded.NodeAttempts["explore"] != 2 || loaded.NodeAttempts["zero"] != 0 {
		t.Fatalf("loaded node attempts = %+v", loaded.NodeAttempts)
	}
	if len(loaded.NodeArtifacts) != 1 || loaded.NodeArtifacts[0].Artifact.ID != "ev-1" {
		t.Fatalf("loaded node artifacts = %+v", loaded.NodeArtifacts)
	}
	if !loaded.ActiveState.IsActive() || loaded.ActiveState.ReadLoopNextAction.Action != ReadDispatchPolicyActionAddProof || loaded.ActiveState.TransientRetryHintHash == "" {
		t.Fatalf("loaded active state = %+v", loaded.ActiveState)
	}
	if tmp := path + ".tmp"; fileExists(tmp) {
		t.Fatalf("tmp file should not remain: %s", tmp)
	}
}

func TestReadRunEnvironmentFingerprintEquality(t *testing.T) {
	base := NormalizeReadRunEnvironmentFingerprint(ReadRunEnvironmentFingerprint{
		Kind:            ReadRunEnvironmentKindCodrax,
		Available:       true,
		CodraxVersion:   "0.1.20260622",
		CodraxBuildTime: "2026-06-22T00:00:00Z",
		GoVersion:       "go1.22.5",
		GOOS:            "darwin",
		GOARCH:          "arm64",
		Tools: []ReadRunToolFingerprint{{
			Name:        "git",
			Available:   true,
			Executable:  "/usr/bin/git",
			VersionHash: strings.Repeat("c", 64),
			FileHash:    strings.Repeat("d", 64),
		}},
		Configs: []ReadRunConfigFingerprint{
			ReadRunConfigFingerprintFromStringSlice(ReadRunConfigFingerprintSearchExcludeRoots, []string{"out", ".codrax"}),
		},
	})
	same := base
	same.Tools = append([]ReadRunToolFingerprint(nil), base.Tools...)
	same.Tools = append(same.Tools, ReadRunToolFingerprint{Name: "missing", Available: false, ReasonCode: ReadRunFingerprintReasonNotFound})
	if !ReadRunEnvironmentFingerprintsEqual(base, same) {
		t.Fatalf("matching comparable environment should be equal: base=%+v same=%+v", base, same)
	}
	missingCurrentTool := base
	missingCurrentTool.Tools = []ReadRunToolFingerprint{{Name: "git", Available: false, ReasonCode: ReadRunFingerprintReasonNotFound}}
	if !ReadRunEnvironmentFingerprintsEqual(base, missingCurrentTool) {
		t.Fatalf("unavailable current tool should be audit-only, not mismatch")
	}
	changedTool := base
	changedTool.Tools = append([]ReadRunToolFingerprint(nil), base.Tools...)
	changedTool.Tools[0].VersionHash = strings.Repeat("e", 64)
	if ReadRunEnvironmentFingerprintsEqual(base, changedTool) {
		t.Fatalf("available tool version mismatch should be detected")
	}
	changedConfig := base
	changedConfig.Configs = []ReadRunConfigFingerprint{
		ReadRunConfigFingerprintFromStringSlice(ReadRunConfigFingerprintSearchExcludeRoots, []string{"different"}),
	}
	if ReadRunEnvironmentFingerprintsEqual(base, changedConfig) {
		t.Fatalf("available config hash mismatch should be detected")
	}
	changedRuntime := base
	changedRuntime.CodraxVersion = "0.2.0"
	if ReadRunEnvironmentFingerprintsEqual(base, changedRuntime) {
		t.Fatalf("available runtime version mismatch should be detected")
	}
}

func TestValidateReadRunActiveState(t *testing.T) {
	base := ReadRunActiveState{
		ReadLoopNextAction: ReadRunNextActionState{
			Active:      true,
			Action:      ReadDispatchPolicyActionAddProof,
			ReasonCode:  "proof_weak",
			ProofState:  "weak",
			TruthAction: TruthActionAddProof,
		},
		ReadDispatchPolicy: ReadDispatchPolicy{
			Active:       true,
			Action:       ReadDispatchPolicyActionAddProof,
			AllowedTools: []string{"read_file"},
			ScopePaths:   []string{"pkg"},
			MaxToolCalls: 1,
			OneShot:      true,
		},
	}
	if got := ValidateReadRunActiveState(base, "/repo"); !got.Valid || got.ReasonCode != ReadRunActiveStateValid {
		t.Fatalf("valid active state rejected: %+v", got)
	}
	cases := []struct {
		name       string
		mutate     func(*ReadRunActiveState)
		wantReason string
	}{
		{
			name: "unsupported_action",
			mutate: func(s *ReadRunActiveState) {
				s.ReadLoopNextAction.Action = "repair"
			},
			wantReason: ReadRunActiveStateUnsupportedAction,
		},
		{
			name: "invalid_proof_state",
			mutate: func(s *ReadRunActiveState) {
				s.ReadLoopNextAction.ProofState = "maybe"
			},
			wantReason: ReadRunActiveStateUnsupportedProofState,
		},
		{
			name: "policy_mismatch",
			mutate: func(s *ReadRunActiveState) {
				s.ReadDispatchPolicy.Action = "verify"
			},
			wantReason: ReadRunActiveStatePolicyMismatch,
		},
		{
			name: "invalid_truth_action",
			mutate: func(s *ReadRunActiveState) {
				s.ReadLoopNextAction.TruthAction = TruthRecommendedAction("maybe")
			},
			wantReason: ReadRunActiveStateUnsupportedTruthAction,
		},
		{
			name: "malformed_scope",
			mutate: func(s *ReadRunActiveState) {
				s.ReadDispatchPolicy.ScopePaths = []string{"../escape"}
			},
			wantReason: ReadRunActiveStateMalformedScope,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			state := base
			tc.mutate(&state)
			got := ValidateReadRunActiveState(state, "/repo")
			if got.Valid || got.ReasonCode != tc.wantReason {
				t.Fatalf("ValidateReadRunActiveState = %+v, want %q", got, tc.wantReason)
			}
		})
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
