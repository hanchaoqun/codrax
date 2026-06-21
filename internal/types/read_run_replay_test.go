package types

import "testing"

func TestDeriveReadRunReplayTransitions(t *testing.T) {
	graph := TaskGraph{Nodes: []TaskNode{
		{ID: "pending", Type: NodeProbe},
		{ID: "requeued", Type: NodeProbe},
		{ID: "running", Type: NodeEvidence},
		{ID: "done_no_required", Type: NodeProbe, Outputs: []string{"source_inventory_observation"}},
		{ID: "done_valid", Type: NodeEvidence, Outputs: []string{"evidence_items", "answer_chains"}},
		{ID: "done_missing", Type: NodeEvidence, Outputs: []string{"evidence_items"}},
		{ID: "done_handoff_valid", Type: NodeProbe, Outputs: []string{"analysis_refinement_handoff"}},
		{ID: "done_handoff_missing", Type: NodeProbe, Outputs: []string{"analysis_refinement_handoff"}},
		{ID: "failed_retry", Type: NodeEvidence, MaxRetries: 1},
		{ID: "failed_exhausted", Type: NodeEvidence, MaxRetries: 1},
		{ID: "failed_unlimited", Type: NodeEvidence},
	}}
	handoffID := RuntimeArtifactIDForAnalysisRefinementHandoff("done_handoff_valid", ProgressDecision{
		ShouldReplan: true,
		ReasonCode:   ProgressReasonContinue,
		Delta: ProgressDelta{
			Kind:          ProgressDeltaDowngradeBlocker,
			DowngradeLane: DowngradeLaneContractChain,
			BlockerKey:    7,
			Consecutive:   1,
			HardThreshold: 3,
		},
	})
	transitions := DeriveReadRunReplayTransitions(ReadRunReplayInput{
		TaskGraph: graph,
		NodeStatuses: map[string]NodeExecStatus{
			"pending":              NodeExecPending,
			"requeued":             NodeExecRequeued,
			"running":              NodeExecRunning,
			"done_no_required":     NodeExecDone,
			"done_valid":           NodeExecDone,
			"done_missing":         NodeExecDone,
			"done_handoff_valid":   NodeExecDone,
			"done_handoff_missing": NodeExecDone,
			"failed_retry":         NodeExecFailed,
			"failed_exhausted":     NodeExecFailed,
			"failed_unlimited":     NodeExecFailed,
		},
		NodeAttempts: map[string]int{
			"failed_retry":     1,
			"failed_exhausted": 2,
		},
		NodeArtifacts: []NodeArtifactRecord{
			{
				ProducerNodeID: "done_valid",
				ProducerStage:  StageExplore,
				Artifact: RuntimeArtifactRef{
					Kind: RuntimeArtifactEvidenceItem,
					ID:   "ev-valid",
				},
			},
			{
				ProducerNodeID: "done_handoff_valid",
				ProducerStage:  StageExplore,
				Consumer:       RuntimeArtifactConsumerAnalysisRefinement,
				Artifact: RuntimeArtifactRef{
					Kind: RuntimeArtifactAnalysisRefinementHandoff,
					ID:   handoffID,
				},
			},
		},
	})
	byNode := map[string]ReadRunReplayTransition{}
	for _, transition := range transitions {
		byNode[transition.NodeID] = transition
	}
	cases := []struct {
		node       string
		wantStatus NodeExecStatus
		wantReason ReadRunReplayReasonCode
		wantValid  bool
	}{
		{"pending", NodeExecPending, ReadRunReplayReasonPendingPreserved, false},
		{"requeued", NodeExecRequeued, ReadRunReplayReasonRequeuedPreserved, false},
		{"running", NodeExecRequeued, ReadRunReplayReasonRunningInterrupted, false},
		{"done_no_required", NodeExecDone, ReadRunReplayReasonDoneNoRequiredArtifact, true},
		{"done_valid", NodeExecDone, ReadRunReplayReasonDoneArtifactsValid, true},
		{"done_missing", NodeExecRequeued, ReadRunReplayReasonDoneArtifactMissing, false},
		{"done_handoff_valid", NodeExecDone, ReadRunReplayReasonDoneArtifactsValid, true},
		{"done_handoff_missing", NodeExecRequeued, ReadRunReplayReasonDoneArtifactMissing, false},
		{"failed_retry", NodeExecRequeued, ReadRunReplayReasonFailedRetryAvailable, false},
		{"failed_exhausted", NodeExecFailed, ReadRunReplayReasonFailedRetryExhausted, false},
		{"failed_unlimited", NodeExecRequeued, ReadRunReplayReasonFailedRetryAvailable, false},
	}
	for _, tc := range cases {
		got, ok := byNode[tc.node]
		if !ok {
			t.Fatalf("missing transition for %s: %+v", tc.node, transitions)
		}
		if got.ToStatus != tc.wantStatus || got.ReasonCode != tc.wantReason || got.ArtifactValid != tc.wantValid {
			t.Fatalf("%s transition = %+v, want status=%q reason=%q artifact_valid=%t", tc.node, got, tc.wantStatus, tc.wantReason, tc.wantValid)
		}
	}
	if refs := byNode["done_valid"].ArtifactRefIDs; len(refs) != 1 || refs[0] != "ev-valid" {
		t.Fatalf("done_valid artifact refs = %+v", refs)
	}
	if refs := byNode["done_handoff_valid"].ArtifactRefIDs; len(refs) != 1 || refs[0] != handoffID {
		t.Fatalf("done_handoff_valid artifact refs = %+v", refs)
	}
	statuses := ReadRunReplayNodeStatuses(transitions)
	if statuses["running"] != NodeExecRequeued || statuses["failed_exhausted"] != NodeExecFailed || statuses["done_missing"] != NodeExecRequeued || statuses["done_handoff_missing"] != NodeExecRequeued {
		t.Fatalf("replay statuses = %+v", statuses)
	}
}
