package types

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildReadRunReplayAuditTypedDeltas(t *testing.T) {
	before := ReadRunSnapshot{
		SchemaVersion: ReadRunSnapshotSchemaVersion,
		RunID:         "run-before",
		Request:       "raw request text must not be projected",
		RequestHash:   "request-hash",
		TaskGraphHash: "task-graph",
		RepoFingerprint: ReadRunRepoFingerprint{
			Kind:       ReadRunRepoFingerprintKindGitHead,
			Available:  true,
			Head:       "abc",
			StatusHash: "clean",
		},
		NodeStatuses: map[string]NodeExecStatus{
			"n-running": NodeExecRunning,
			"n-done":    NodeExecDone,
			"n-failed":  NodeExecFailed,
		},
		AcceptedEvidence: []AcceptedEvidenceRef{{
			ID:        "ev-before",
			Kind:      EvidenceMechanism,
			Source:    "a.go",
			LineStart: 1,
		}},
		ReadSet:    []string{"a.go"},
		ReadRanges: map[string][]LineRange{"a.go": {{Start: 1, End: 4}}},
		FileTotals: map[string]int{"a.go": 40},
		NodeArtifacts: []NodeArtifactRecord{{
			ProducerNodeID: "n-done",
			ProducerStage:  StageExplore,
			Artifact: RuntimeArtifactRef{
				Kind:      RuntimeArtifactEvidenceItem,
				ID:        "ev-before",
				Path:      "a.go",
				LineStart: 1,
			},
		}},
		SourceInventory: SourceInventoryObservation{
			Active:   true,
			Complete: true,
			Scopes:   []string{"."},
			SourceClasses: []SourceInventorySourceClassCount{{
				Role:     SourcePathRoleProduction,
				Count:    2,
				Complete: true,
			}},
			Sets: []SourceInventoryObservationSet{{
				Role:    AnswerCandidateRoleFile,
				Members: []SourceInventoryObservationMember{{Name: "a.go", File: "a.go"}},
			}},
		},
		ActiveState: ReadRunActiveState{TransientRetryPending: true},
	}
	after := before
	after.RunID = "run-after"
	after.Request = "different prose should still not appear"
	after.RepoFingerprint.Head = "def"
	after.NodeStatuses = map[string]NodeExecStatus{
		"n-running": NodeExecRequeued,
		"n-done":    NodeExecDone,
		"n-failed":  NodeExecRequeued,
		"n-new":     NodeExecDone,
	}
	after.AcceptedEvidence = append(after.AcceptedEvidence, AcceptedEvidenceRef{
		ID:        "ev-after",
		Kind:      EvidenceMechanism,
		Source:    "b.go",
		LineStart: 7,
	})
	after.ReadSet = []string{"a.go", "b.go"}
	after.ReadRanges = map[string][]LineRange{
		"a.go": {{Start: 1, End: 4}},
		"b.go": {{Start: 7, End: 8}},
	}
	after.FileTotals = map[string]int{"a.go": 40, "b.go": 80}
	after.NodeArtifacts = append(after.NodeArtifacts, NodeArtifactRecord{
		Direction:      RuntimeArtifactConsumed,
		ProducerNodeID: "n-done",
		ConsumerNodeID: "extract",
		Consumer:       RuntimeArtifactConsumerExtract,
		ReasonCode:     RuntimeArtifactReasonReadinessConsumed,
		Artifact: RuntimeArtifactRef{
			Kind:      RuntimeArtifactEvidenceItem,
			ID:        "ev-before",
			Path:      "a.go",
			LineStart: 1,
		},
	})
	after.SourceInventory.Scopes = []string{".", "internal"}
	after.SourceInventory.SourceClasses = append(after.SourceInventory.SourceClasses, SourceInventorySourceClassCount{
		Role:     SourcePathRoleFixture,
		Count:    3,
		Complete: true,
	})
	after.SourceInventory.Sets[0].Members = append(after.SourceInventory.Sets[0].Members, SourceInventoryObservationMember{Name: "b.go", File: "b.go"})
	after.ActiveState = ReadRunActiveState{}

	transitions := []ReadRunReplayTransition{
		{NodeID: "n-running", FromStatus: NodeExecRunning, ToStatus: NodeExecRequeued, ReasonCode: ReadRunReplayReasonRunningInterrupted},
		{NodeID: "n-done", FromStatus: NodeExecDone, ToStatus: NodeExecDone, ReasonCode: ReadRunReplayReasonDoneArtifactsValid, ArtifactValid: true},
		{NodeID: "n-failed", FromStatus: NodeExecFailed, ToStatus: NodeExecRequeued, ReasonCode: ReadRunReplayReasonFailedRetryAvailable},
		{NodeID: "n-missing", FromStatus: NodeExecDone, ToStatus: NodeExecRequeued, ReasonCode: ReadRunReplayReasonDoneArtifactMissing},
	}

	audit := BuildReadRunReplayAudit(&before, &after, transitions)
	if audit.BaseRunID != "run-before" || audit.CurrentRunID != "run-after" {
		t.Fatalf("run ids not projected: %+v", audit)
	}
	if audit.TaskGraphHashMatch != ReadRunReplayAuditMatchEqual ||
		audit.RequestHashMatch != ReadRunReplayAuditMatchEqual ||
		audit.RepoFingerprintMatch != ReadRunReplayAuditMatchMismatch {
		t.Fatalf("match states wrong: %+v", audit)
	}
	if got := findAuditDelta(audit.NodeStatusDeltas, string(NodeExecRequeued)); got.After != 2 || got.Before != 0 || got.Delta != 2 {
		t.Fatalf("requeued status delta = %+v", got)
	}
	if got := findAuditReason(audit.ReplayReasonCounts, ReadRunReplayReasonRunningInterrupted); got.Count != 1 {
		t.Fatalf("running reason count = %+v", got)
	}
	if audit.RequeuedFromRunning != 1 || audit.DonePreserved != 1 || audit.DoneRequeuedMissingArtifact != 1 || audit.FailedRequeued != 1 {
		t.Fatalf("transition rollup wrong: %+v", audit)
	}
	if audit.AcceptedEvidenceDelta.BeforeCount != 1 || audit.AcceptedEvidenceDelta.AfterCount != 2 || audit.AcceptedEvidenceDelta.AddedCount != 1 {
		t.Fatalf("accepted evidence delta wrong: %+v", audit.AcceptedEvidenceDelta)
	}
	if audit.ReadSetDelta.AddedCount != 1 || !strings.Contains(strings.Join(audit.ReadSetDelta.AddedSamples, ","), "b.go") {
		t.Fatalf("read set delta wrong: %+v", audit.ReadSetDelta)
	}
	if audit.ReadRangeDelta.AddedCount != 1 || audit.FileTotalDelta.AddedCount != 1 {
		t.Fatalf("coverage deltas wrong: ranges=%+v totals=%+v", audit.ReadRangeDelta, audit.FileTotalDelta)
	}
	if audit.NodeArtifactDelta.BeforeCount != 1 || audit.NodeArtifactDelta.AfterCount != 2 {
		t.Fatalf("node artifact delta wrong: %+v", audit.NodeArtifactDelta)
	}
	if got := findAuditDelta(audit.NodeArtifactDirectionDeltas, string(RuntimeArtifactConsumed)); got.After != 1 {
		t.Fatalf("consumed direction delta = %+v", got)
	}
	if audit.SourceInventoryDelta.SourceClassCountBefore != 1 ||
		audit.SourceInventoryDelta.SourceClassCountAfter != 2 ||
		audit.SourceInventoryDelta.SourceClassTotalAfter != 5 ||
		audit.SourceInventoryDelta.MemberCountAfter != 2 ||
		audit.SourceInventoryDelta.ScopeDelta.AddedCount != 1 {
		t.Fatalf("source inventory delta wrong: %+v", audit.SourceInventoryDelta)
	}
	if !audit.ActiveStateBefore || audit.ActiveStateAfter {
		t.Fatalf("active-state presence wrong: %+v", audit)
	}
	raw, err := json.Marshal(audit)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"raw request text", "different prose"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("audit leaked prose %q: %s", forbidden, raw)
		}
	}
}

func TestBuildReadRunReplayAuditNilSnapshotsDeterministic(t *testing.T) {
	a := BuildReadRunReplayAudit(nil, nil, nil)
	b := BuildReadRunReplayAudit(&ReadRunSnapshot{}, nil, []ReadRunReplayTransition{{ReasonCode: " "}})
	if a.TaskGraphHashMatch != ReadRunReplayAuditMatchUnknown ||
		b.RequestHashMatch != ReadRunReplayAuditMatchUnknown ||
		len(b.ReplayReasonCounts) != 0 {
		t.Fatalf("nil/empty audit not normalized: a=%+v b=%+v", a, b)
	}
	rawA, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	rawB, err := json.Marshal(NormalizeReadRunReplayAudit(a))
	if err != nil {
		t.Fatal(err)
	}
	if string(rawA) != string(rawB) {
		t.Fatalf("normalization not deterministic:\n%s\n%s", rawA, rawB)
	}
}

func findAuditDelta(items []ReadRunReplayAuditCountDelta, name string) ReadRunReplayAuditCountDelta {
	for _, item := range items {
		if item.Name == name {
			return item
		}
	}
	return ReadRunReplayAuditCountDelta{}
}

func findAuditReason(items []ReadRunReplayAuditReasonCount, reason ReadRunReplayReasonCode) ReadRunReplayAuditReasonCount {
	for _, item := range items {
		if item.ReasonCode == reason {
			return item
		}
	}
	return ReadRunReplayAuditReasonCount{}
}
