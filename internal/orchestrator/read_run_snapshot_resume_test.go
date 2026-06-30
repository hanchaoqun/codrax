package orchestrator

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/analysis/compiler"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestReadRunSnapshotSeedAppliesTypedFields(t *testing.T) {
	repoRoot := "/tmp/codrax-read-seed"
	ir := dagIR(types.AnswerContract{Language: "en"})
	compiler.EnsureReadStageNodes(&ir.TaskGraph)
	exploreID := firstReadNodeIDByType(t, ir, types.NodeEvidence)
	snapshot := readRunSnapshotSeedFixture(t, ir, repoRoot)
	snapshot.NodeStatuses = map[string]types.NodeExecStatus{exploreID: types.NodeExecDone}
	snapshot.NodeAttempts = map[string]int{exploreID: 2}
	snapshot.NodeArtifacts = []types.NodeArtifactRecord{{
		ProducerNodeID: exploreID,
		ProducerStage:  types.StageExplore,
		Artifact:       types.RuntimeArtifactRef{Kind: types.RuntimeArtifactEvidenceItem, ID: "ev-seed"},
	}, {
		Direction:      types.RuntimeArtifactConsumed,
		ProducerNodeID: exploreID,
		ConsumerNodeID: firstReadNodeIDByType(t, ir, types.NodeExtract),
		ProducerStage:  types.StageExplore,
		Artifact:       types.RuntimeArtifactRef{Kind: types.RuntimeArtifactEvidenceItem, ID: "ev-seed"},
	}}

	o := &Orchestrator{
		readRunSnapshotSeed: &snapshot,
		busCtx: &types.BusContext{
			Mode:       types.ModeRead,
			RepoRoot:   repoRoot,
			AnalysisIR: ir,
			Mutable:    types.NewMutableState("resume typed snapshot"),
		},
	}
	o.busCtx.Mutable.SetRepoRoot(repoRoot)

	if err := o.applyReadRunSnapshotSeed(); err != nil {
		t.Fatalf("applyReadRunSnapshotSeed: %v", err)
	}
	if o.readRunSnapshotSeed != nil {
		t.Fatal("read snapshot seed must be consume-once")
	}
	closure := o.busCtx.Mutable.EvidenceClosure()
	if got := closure.NodeExecStatus(exploreID); got != types.NodeExecDone {
		t.Fatalf("node status = %q, want %q", got, types.NodeExecDone)
	}
	if got := closure.NodeExecAttempt(exploreID); got != 2 {
		t.Fatalf("node attempts = %d, want 2", got)
	}
	if !closure.HasRead("seeded.go") {
		t.Fatalf("read set did not hydrate from snapshot: %+v", closure.ReadSet())
	}
	if ranges := closure.ReadRanges("seeded.go"); len(ranges) != 1 || ranges[0].Start != 7 || ranges[0].End != 9 {
		t.Fatalf("read ranges = %+v", ranges)
	}
	if got := closure.FileTotalLines("seeded.go"); got != 42 {
		t.Fatalf("file total = %d, want 42", got)
	}
	if refs := closure.AcceptedEvidenceRefs(); len(refs) != 1 || refs[0].ID != "ev-seed" {
		t.Fatalf("accepted evidence refs = %+v", refs)
	}
	if artifacts := closure.NodeArtifactRecords(); len(artifacts) != 2 || artifacts[0].ProducerNodeID != exploreID {
		t.Fatalf("node artifacts = %+v", artifacts)
	}
	if consumed := closure.NodeArtifactLedger().RecordsByConsumerNode(firstReadNodeIDByType(t, ir, types.NodeExtract)); len(consumed) != 1 {
		t.Fatalf("consumed node artifacts = %+v", consumed)
	}
	if obs := closure.SourceInventoryObservation(); !obs.IsActive() || len(obs.SourceClasses) != 1 {
		t.Fatalf("source inventory observation = %+v", obs)
	}
	if decision := closure.LatestProgressDecision(); decision.ReasonCode != types.ProgressReasonContinue || !decision.ShouldReplan {
		t.Fatalf("progress decision = %+v", decision)
	}
}

func TestReadRunSnapshotSeedInstallsActiveStateSeed(t *testing.T) {
	repoRoot := "/tmp/codrax-read-seed"
	ir := dagIR(types.AnswerContract{Language: "en"})
	compiler.EnsureReadStageNodes(&ir.TaskGraph)
	snapshot := readRunSnapshotSeedFixture(t, ir, repoRoot)
	snapshot.ActiveState = types.ReadRunActiveState{
		TransientRetryPending:    true,
		TransientRetryReasonCode: types.ReadRunTransientRetryReasonCheckpoint,
		TransientRetryHintHash:   strings.Repeat("b", 64),
		TransientRetryHintBytes:  64,
		ReadLoopNextAction: types.ReadRunNextActionState{
			Active:          true,
			Action:          types.ReadDispatchPolicyActionAddProof,
			ReasonCode:      "proof_weak",
			ProofState:      "weak",
			TruthAction:     types.TruthActionAddProof,
			RouteSurface:    types.ReadDispatchPolicySurfaceVerify,
			RouteReasonCode: "loop_tool_route_verification",
			ToolSuggestions: []string{"run_tests"},
		},
		ReadDispatchPolicy: types.ReadDispatchPolicy{
			Active:       true,
			Action:       types.ReadDispatchPolicyActionAddProof,
			AllowedTools: []string{"read_file", "emit_evidence"},
			ScopePaths:   []string{"seeded.go"},
			MaxToolCalls: 2,
			OneShot:      true,
		},
	}

	o := &Orchestrator{
		readRunSnapshotSeed: &snapshot,
		busCtx: &types.BusContext{
			Mode:       types.ModeRead,
			RepoRoot:   repoRoot,
			AnalysisIR: ir,
			Mutable:    types.NewMutableState("resume typed snapshot"),
		},
	}
	o.busCtx.Mutable.SetRepoRoot(repoRoot)
	if err := o.applyReadRunSnapshotSeed(); err != nil {
		t.Fatalf("applyReadRunSnapshotSeed: %v", err)
	}
	if !o.readRunActiveSeed.IsActive() {
		t.Fatalf("active seed missing: %+v", o.readRunActiveSeed)
	}
	state := newGraphState(ir.TaskGraph)
	state.applyReadRunActiveStateSeed(o.readRunActiveSeed)
	policy, ok := applyReadLoopNextActionHint(state, nil, nil)
	if !ok || !policy.IsActive() || policy.Action != types.ReadDispatchPolicyActionAddProof {
		t.Fatalf("next action policy = %+v ok=%t", policy, ok)
	}
	if _, ok := applyReadLoopNextActionHint(state, nil, nil); ok {
		t.Fatal("resumed read-loop next action must remain one-shot")
	}
	if hint, kind := state.consumeTransientRetryHint(); !strings.Contains(hint, "hint_hash="+strings.Repeat("b", 64)) || strings.Contains(hint, "raw retry") || kind != types.RetryHintKindDurableProgressContinuation {
		t.Fatalf("transient retry resume hint should be typed/hash-only, got %q", hint)
	}
}

func TestReadRunSnapshotSeedAppliesReplayTransitions(t *testing.T) {
	repoRoot := "/tmp/codrax-read-replay"
	ir := readRunReplayIR()
	snapshot := types.ReadRunSnapshot{
		SchemaVersion: types.ReadRunSnapshotSchemaVersion,
		RunID:         "read-replay-1",
		Request:       "resume replay snapshot",
		RepoRoot:      repoRoot,
		TaskGraphHash: types.ReadTaskGraphHash(ir.TaskGraph),
		TaskNodeCount: len(ir.TaskGraph.Nodes),
		NodeStatuses: map[string]types.NodeExecStatus{
			"running":          types.NodeExecRunning,
			"done_valid":       types.NodeExecDone,
			"done_missing":     types.NodeExecDone,
			"failed_retry":     types.NodeExecFailed,
			"failed_exhausted": types.NodeExecFailed,
		},
		NodeAttempts: map[string]int{
			"failed_retry":     1,
			"failed_exhausted": 2,
		},
		NodeArtifacts: []types.NodeArtifactRecord{{
			ProducerNodeID: "done_valid",
			ProducerStage:  types.StageExplore,
			Artifact:       types.RuntimeArtifactRef{Kind: types.RuntimeArtifactEvidenceItem, ID: "ev-valid"},
		}},
	}
	o := &Orchestrator{
		readRunSnapshotSeed: &snapshot,
		busCtx: &types.BusContext{
			Mode:       types.ModeRead,
			RepoRoot:   repoRoot,
			AnalysisIR: ir,
			Mutable:    types.NewMutableState("resume replay snapshot"),
		},
	}
	o.busCtx.Mutable.SetRepoRoot(repoRoot)
	if err := o.applyReadRunSnapshotSeed(); err != nil {
		t.Fatalf("applyReadRunSnapshotSeed: %v", err)
	}
	closure := o.busCtx.Mutable.EvidenceClosure()
	want := map[string]types.NodeExecStatus{
		"running":          types.NodeExecRequeued,
		"done_valid":       types.NodeExecDone,
		"done_missing":     types.NodeExecRequeued,
		"failed_retry":     types.NodeExecRequeued,
		"failed_exhausted": types.NodeExecFailed,
	}
	for nodeID, wantStatus := range want {
		if got := closure.NodeExecStatus(nodeID); got != wantStatus {
			t.Fatalf("node %s status = %q, want %q", nodeID, got, wantStatus)
		}
	}
	if got := closure.NodeExecAttempt("failed_retry"); got != 1 {
		t.Fatalf("failed_retry attempts = %d, want 1", got)
	}
	if got := closure.NodeExecAttempt("failed_exhausted"); got != 2 {
		t.Fatalf("failed_exhausted attempts = %d, want 2", got)
	}
}

func TestReadRunSnapshotSeedRejectsMismatches(t *testing.T) {
	repoRoot := "/tmp/codrax-read-seed"
	ir := dagIR(types.AnswerContract{Language: "en"})
	compiler.EnsureReadStageNodes(&ir.TaskGraph)
	base := readRunSnapshotSeedFixture(t, ir, repoRoot)
	cases := []struct {
		name       string
		mutate     func(*types.ReadRunSnapshot)
		wantReason string
	}{
		{
			name: "schema",
			mutate: func(s *types.ReadRunSnapshot) {
				s.SchemaVersion = types.ReadRunSnapshotSchemaVersion + 100
			},
			wantReason: readRunSnapshotSeedReasonSchemaMismatch,
		},
		{
			name: "repo",
			mutate: func(s *types.ReadRunSnapshot) {
				s.RepoRoot = "/tmp/codrax-other-repo"
			},
			wantReason: readRunSnapshotSeedReasonRepoMismatch,
		},
		{
			name: "task_graph",
			mutate: func(s *types.ReadRunSnapshot) {
				s.TaskGraphHash = "different-graph"
			},
			wantReason: readRunSnapshotSeedReasonTaskGraphMismatch,
		},
		{
			name: "request_fingerprint",
			mutate: func(s *types.ReadRunSnapshot) {
				s.RequestHash = types.ReadRunRequestHash("different request")
			},
			wantReason: readRunSnapshotSeedReasonRequestFingerprint,
		},
		{
			name: "attachment_fingerprint",
			mutate: func(s *types.ReadRunSnapshot) {
				s.Attachments = []types.ReadRunAttachmentFingerprint{
					types.ReadRunAttachmentFingerprintFromPayload(types.ReadRunAttachmentKindLog, "panic: saved\n", ""),
				}
			},
			wantReason: readRunSnapshotSeedReasonAttachmentFingerprint,
		},
		{
			name: "node",
			mutate: func(s *types.ReadRunSnapshot) {
				s.NodeStatuses = map[string]types.NodeExecStatus{"missing-node": types.NodeExecDone}
			},
			wantReason: readRunSnapshotSeedReasonNodeMismatch,
		},
		{
			name: "node_attempt",
			mutate: func(s *types.ReadRunSnapshot) {
				s.NodeAttempts = map[string]int{"missing-node": 1}
			},
			wantReason: readRunSnapshotSeedReasonNodeAttemptMismatch,
		},
		{
			name: "node_artifact",
			mutate: func(s *types.ReadRunSnapshot) {
				s.NodeArtifacts = []types.NodeArtifactRecord{{
					ProducerNodeID: "missing-node",
					Artifact:       types.RuntimeArtifactRef{Kind: types.RuntimeArtifactEvidenceItem, ID: "ev-missing"},
				}}
			},
			wantReason: readRunSnapshotSeedReasonNodeArtifactMismatch,
		},
		{
			name: "node_artifact_consumer",
			mutate: func(s *types.ReadRunSnapshot) {
				exploreID := firstReadNodeIDByType(t, ir, types.NodeEvidence)
				s.NodeArtifacts = []types.NodeArtifactRecord{{
					Direction:      types.RuntimeArtifactConsumed,
					ProducerNodeID: exploreID,
					ConsumerNodeID: "missing-consumer",
					Artifact:       types.RuntimeArtifactRef{Kind: types.RuntimeArtifactEvidenceItem, ID: "ev-missing"},
				}}
			},
			wantReason: readRunSnapshotSeedReasonNodeArtifactMismatch,
		},
		{
			name: "active_state",
			mutate: func(s *types.ReadRunSnapshot) {
				s.ActiveState = types.ReadRunActiveState{
					ReadLoopNextAction: types.ReadRunNextActionState{
						Active: true,
						Action: "repair",
					},
					ReadDispatchPolicy: types.ReadDispatchPolicy{
						Active: true,
						Action: "repair",
					},
				}
			},
			wantReason: readRunSnapshotSeedReasonActiveStateMismatch,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			snapshot := base
			tc.mutate(&snapshot)
			o := &Orchestrator{
				busCtx: &types.BusContext{
					Mode:       types.ModeRead,
					RepoRoot:   repoRoot,
					AnalysisIR: ir,
					Mutable:    types.NewMutableState("resume typed snapshot"),
				},
			}
			o.busCtx.Mutable.SetRepoRoot(repoRoot)
			o.SetReadRunSnapshotSeed(&snapshot)
			err := o.applyReadRunSnapshotSeed()
			if err == nil || !strings.Contains(err.Error(), tc.wantReason) {
				t.Fatalf("applyReadRunSnapshotSeed error = %v, want reason %q", err, tc.wantReason)
			}
			if o.busCtx.Mutable.EvidenceClosure().HasRead("seeded.go") {
				t.Fatal("rejected snapshot must not hydrate read coverage")
			}
		})
	}
}

func readRunReplayIR() *types.AnalysisIR {
	graph := types.TaskGraph{Nodes: []types.TaskNode{
		{ID: "pending", Type: types.NodeProbe, Objective: "pending"},
		{ID: "running", Type: types.NodeProbe, Objective: "interrupted"},
		{ID: "done_valid", Type: types.NodeEvidence, Objective: "valid evidence", Outputs: []string{"evidence_items"}},
		{ID: "done_missing", Type: types.NodeEvidence, Objective: "missing evidence", Outputs: []string{"evidence_items"}},
		{ID: "failed_retry", Type: types.NodeEvidence, Objective: "retryable", MaxRetries: 1},
		{ID: "failed_exhausted", Type: types.NodeEvidence, Objective: "terminal", MaxRetries: 1},
	}}
	return &types.AnalysisIR{
		Version:      types.AnalysisIRVersion,
		RequestModel: types.RequestModel{Language: "en", Intent: types.IntentExplain, RawRequest: "resume replay snapshot"},
		TaskGraph:    graph,
		AnswerContract: types.AnswerContract{
			Language: "en",
		},
	}
}

func TestReadRunSnapshotSeedRejectsRepoFingerprintMismatch(t *testing.T) {
	repoRoot := initReadRunSnapshotGitRepo(t)
	ir := dagIR(types.AnswerContract{Language: "en"})
	compiler.EnsureReadStageNodes(&ir.TaskGraph)
	snapshot := readRunSnapshotSeedFixture(t, ir, repoRoot)
	snapshot.RepoFingerprint = types.ReadRunRepoFingerprint{
		Kind:      types.ReadRunRepoFingerprintKindGitHead,
		Available: true,
		Head:      "deadbeef",
	}
	o := &Orchestrator{
		busCtx: &types.BusContext{
			Mode:       types.ModeRead,
			RepoRoot:   repoRoot,
			AnalysisIR: ir,
			Mutable:    types.NewMutableState("resume typed snapshot"),
		},
	}
	o.busCtx.Mutable.SetRepoRoot(repoRoot)
	o.SetReadRunSnapshotSeed(&snapshot)
	err := o.applyReadRunSnapshotSeed()
	if err == nil || !strings.Contains(err.Error(), readRunSnapshotSeedReasonRepoFingerprint) {
		t.Fatalf("applyReadRunSnapshotSeed error = %v, want repo fingerprint mismatch", err)
	}
	if o.busCtx.Mutable.EvidenceClosure().HasRead("seeded.go") {
		t.Fatal("repo fingerprint mismatch must not hydrate read coverage")
	}
}

func TestReadRunSnapshotSeedRejectsEnvironmentFingerprintMismatch(t *testing.T) {
	repoRoot := "/tmp/codrax-read-seed"
	ir := dagIR(types.AnswerContract{Language: "en"})
	compiler.EnsureReadStageNodes(&ir.TaskGraph)
	snapshot := readRunSnapshotSeedFixture(t, ir, repoRoot)
	snapshot.Environment = types.ReadRunEnvironmentFingerprint{
		Kind:      types.ReadRunEnvironmentKindCodrax,
		Available: true,
		Configs: []types.ReadRunConfigFingerprint{{
			Name:      types.ReadRunConfigFingerprintSearchExcludeRoots,
			Available: true,
			Hash:      strings.Repeat("f", 64),
		}},
	}
	o := &Orchestrator{
		busCtx: &types.BusContext{
			Mode:       types.ModeRead,
			RepoRoot:   repoRoot,
			AnalysisIR: ir,
			Mutable:    types.NewMutableState("resume typed snapshot"),
		},
	}
	o.busCtx.Mutable.SetRepoRoot(repoRoot)
	o.SetReadRunSnapshotSeed(&snapshot)
	err := o.applyReadRunSnapshotSeed()
	if err == nil || !strings.Contains(err.Error(), readRunSnapshotSeedReasonEnvironment) {
		t.Fatalf("applyReadRunSnapshotSeed error = %v, want environment fingerprint mismatch", err)
	}
	if o.busCtx.Mutable.EvidenceClosure().HasRead("seeded.go") {
		t.Fatal("environment fingerprint mismatch must not hydrate read coverage")
	}
}

func TestReadRunSnapshotSeedAllowsUnavailableEnvironmentFingerprint(t *testing.T) {
	repoRoot := "/tmp/codrax-read-seed"
	ir := dagIR(types.AnswerContract{Language: "en"})
	compiler.EnsureReadStageNodes(&ir.TaskGraph)
	snapshot := readRunSnapshotSeedFixture(t, ir, repoRoot)
	snapshot.Environment = types.ReadRunEnvironmentFingerprint{
		Kind:      types.ReadRunEnvironmentKindCodrax,
		Available: true,
		Tools: []types.ReadRunToolFingerprint{{
			Name:       "git",
			Available:  false,
			ReasonCode: types.ReadRunFingerprintReasonNotFound,
		}},
		Configs: []types.ReadRunConfigFingerprint{{
			Name:       types.ReadRunConfigFingerprintSearchExcludeRoots,
			Available:  false,
			ReasonCode: types.ReadRunFingerprintReasonUnavailable,
		}},
	}
	o := &Orchestrator{
		busCtx: &types.BusContext{
			Mode:       types.ModeRead,
			RepoRoot:   repoRoot,
			AnalysisIR: ir,
			Mutable:    types.NewMutableState("resume typed snapshot"),
		},
	}
	o.busCtx.Mutable.SetRepoRoot(repoRoot)
	o.SetReadRunSnapshotSeed(&snapshot)
	if err := o.applyReadRunSnapshotSeed(); err != nil {
		t.Fatalf("unavailable environment fingerprint should be audit-only, got %v", err)
	}
	if !o.busCtx.Mutable.EvidenceClosure().HasRead("seeded.go") {
		t.Fatal("unavailable environment fingerprint should allow hydration")
	}
}

func TestReadRunSnapshotSeedAcceptsMatchingAttachmentFingerprint(t *testing.T) {
	repoRoot := "/tmp/codrax-read-seed"
	ir := dagIR(types.AnswerContract{Language: "en"})
	compiler.EnsureReadStageNodes(&ir.TaskGraph)
	snapshot := readRunSnapshotSeedFixture(t, ir, repoRoot)
	snapshot.Attachments = []types.ReadRunAttachmentFingerprint{
		types.ReadRunAttachmentFingerprintFromPayload(types.ReadRunAttachmentKindLog, "panic: saved\n", ""),
	}
	o := &Orchestrator{
		busCtx: &types.BusContext{
			Mode:        types.ModeRead,
			RepoRoot:    repoRoot,
			AnalysisIR:  ir,
			Mutable:     types.NewMutableState("resume typed snapshot"),
			AttachedLog: "panic: saved\n",
		},
	}
	o.busCtx.Mutable.SetRepoRoot(repoRoot)
	o.SetReadRunSnapshotSeed(&snapshot)
	if err := o.applyReadRunSnapshotSeed(); err != nil {
		t.Fatalf("applyReadRunSnapshotSeed: %v", err)
	}
	if !o.busCtx.Mutable.EvidenceClosure().HasRead("seeded.go") {
		t.Fatal("matching attachment fingerprint should allow hydration")
	}
}

func TestReadRunSnapshotSeedSkippedOutsideReadMode(t *testing.T) {
	repoRoot := "/tmp/codrax-read-seed"
	ir := dagIR(types.AnswerContract{Language: "en"})
	compiler.EnsureReadStageNodes(&ir.TaskGraph)
	snapshot := readRunSnapshotSeedFixture(t, ir, repoRoot)
	o := &Orchestrator{
		readRunSnapshotSeed: &snapshot,
		busCtx: &types.BusContext{
			Mode:       types.ModeApply,
			RepoRoot:   repoRoot,
			AnalysisIR: ir,
			Mutable:    types.NewMutableState("write mode should ignore read seed"),
		},
	}
	if err := o.applyReadRunSnapshotSeed(); err != nil {
		t.Fatalf("write-mode seed should be a no-op, got %v", err)
	}
	if o.busCtx.Mutable.EvidenceClosure().HasRead("seeded.go") {
		t.Fatal("write mode must not hydrate read snapshot seed")
	}
}

func TestE2E_ReadMode_AppliesReadRunSnapshotSeedBeforeExplore(t *testing.T) {
	repoRoot := "/tmp/codrax-read-seed"
	ir := dagIR(types.AnswerContract{Language: "en"})
	compiler.EnsureReadStageNodes(&ir.TaskGraph)
	snapshot := readRunSnapshotSeedFixture(t, ir, repoRoot)
	explorerSawSeed := false
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			explorerSawSeed = ctx.Mutable.EvidenceClosure().HasRead("seeded.go")
			return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
		},
		types.AgentExtractor: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "- `Seed` (seeded.go:7)",
			}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{MaxRetriesPerStage: 2}, ar, sr, sar)
	o.SetReadRunSnapshotSeed(&snapshot)
	o.SetMaxSteps(20)

	busCtx, err := o.Run("resume typed snapshot", repoRoot, "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if busCtx.TaskState.LastError != "" {
		t.Fatalf("LastError = %q", busCtx.TaskState.LastError)
	}
	if !explorerSawSeed {
		t.Fatal("explorer did not observe snapshot seed before read scheduling")
	}
}

func TestE2E_ReadMode_RejectsBadReadRunSnapshotSeedFailLoud(t *testing.T) {
	repoRoot := "/tmp/codrax-read-seed"
	ir := dagIR(types.AnswerContract{Language: "en"})
	compiler.EnsureReadStageNodes(&ir.TaskGraph)
	snapshot := readRunSnapshotSeedFixture(t, ir, repoRoot)
	snapshot.SchemaVersion = types.ReadRunSnapshotSchemaVersion + 1
	explorerCalls := 0
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			explorerCalls++
			return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{MaxRetriesPerStage: 2}, ar, sr, sar)
	o.SetReadRunSnapshotSeed(&snapshot)
	o.SetMaxSteps(20)

	busCtx, err := o.Run("resume typed snapshot", repoRoot, "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if explorerCalls != 0 {
		t.Fatalf("rejected read snapshot seed must stop read scheduling; explorer calls=%d", explorerCalls)
	}
	if !strings.Contains(busCtx.TaskState.LastError, readRunSnapshotSeedReasonSchemaMismatch) {
		t.Fatalf("LastError = %q, want schema mismatch", busCtx.TaskState.LastError)
	}
}

func TestE2E_ReadMode_AutoReadRunSnapshotSeedMismatchFallsBackFresh(t *testing.T) {
	repoRoot := "/tmp/codrax-read-seed"
	ir := dagIR(types.AnswerContract{Language: "en"})
	compiler.EnsureReadStageNodes(&ir.TaskGraph)
	snapshot := readRunSnapshotSeedFixture(t, ir, repoRoot)
	snapshot.TaskGraphHash = "different-graph"
	explorerCalls := 0
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			explorerCalls++
			if ctx.Mutable.EvidenceClosure().HasRead("seeded.go") {
				t.Fatal("auto seed mismatch must not hydrate read coverage")
			}
			return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
		},
		types.AgentExtractor: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "- fresh answer",
			}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{MaxRetriesPerStage: 2}, ar, sr, sar)
	o.SetReadRunSnapshotAutoSeed(&snapshot)
	o.SetMaxSteps(20)

	busCtx, err := o.Run("resume typed snapshot", repoRoot, "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if explorerCalls == 0 {
		t.Fatal("auto seed mismatch should fall back to a fresh read run")
	}
	if busCtx.TaskState.LastError != "" {
		t.Fatalf("LastError = %q, want empty for soft auto seed mismatch", busCtx.TaskState.LastError)
	}
	if busCtx.Mutable.EvidenceClosure().HasRead("seeded.go") {
		t.Fatal("auto seed mismatch must not leave partial hydrated state")
	}
}

func readRunSnapshotSeedFixture(t *testing.T, ir *types.AnalysisIR, repoRoot string) types.ReadRunSnapshot {
	t.Helper()
	if ir == nil {
		t.Fatal("nil AnalysisIR")
	}
	return types.ReadRunSnapshot{
		SchemaVersion: types.ReadRunSnapshotSchemaVersion,
		RunID:         "read-seed-1",
		Request:       "resume typed snapshot",
		RepoRoot:      repoRoot,
		TaskGraphHash: types.ReadTaskGraphHash(ir.TaskGraph),
		TaskNodeCount: len(ir.TaskGraph.Nodes),
		NodeAttempts:  map[string]int{firstReadNodeIDByType(t, ir, types.NodeEvidence): 1},
		ReadSet:       []string{"seeded.go"},
		ReadRanges: map[string][]types.LineRange{
			"seeded.go": {{Start: 7, End: 9}},
		},
		FileTotals: map[string]int{"seeded.go": 42},
		AcceptedEvidence: []types.AcceptedEvidenceRef{{
			ID:        "ev-seed",
			Source:    "seeded.go",
			LineStart: 7,
		}},
		SourceInventory: types.SourceInventoryObservation{
			Active:   true,
			Complete: true,
			SourceClasses: []types.SourceInventorySourceClassCount{{
				Role:     types.SourcePathRoleProduction,
				Count:    1,
				Complete: true,
			}},
		},
		ProgressDecision: types.ProgressDecision{
			ShouldReplan: true,
			ReasonCode:   types.ProgressReasonContinue,
			Delta: types.ProgressDelta{
				Kind:          types.ProgressDeltaDowngradeBlocker,
				DowngradeLane: types.DowngradeLaneContractChain,
				BlockerKey:    7,
				Consecutive:   1,
				HardThreshold: 3,
			},
		},
	}
}

func initReadRunSnapshotGitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runReadRunSnapshotGit(t, repo, "init", "--initial-branch=main")
	runReadRunSnapshotGit(t, repo, "config", "user.email", "test@example.com")
	runReadRunSnapshotGit(t, repo, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("seed\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runReadRunSnapshotGit(t, repo, "add", "README.md")
	runReadRunSnapshotGit(t, repo, "commit", "-m", "seed")
	return repo
}

func runReadRunSnapshotGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
