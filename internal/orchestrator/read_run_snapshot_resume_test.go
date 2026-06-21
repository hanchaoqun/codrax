package orchestrator

import (
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
	if obs := closure.SourceInventoryObservation(); !obs.IsActive() || len(obs.SourceClasses) != 1 {
		t.Fatalf("source inventory observation = %+v", obs)
	}
	if decision := closure.LatestProgressDecision(); decision.ReasonCode != types.ProgressReasonContinue || !decision.ShouldReplan {
		t.Fatalf("progress decision = %+v", decision)
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
			name: "node",
			mutate: func(s *types.ReadRunSnapshot) {
				s.NodeStatuses = map[string]types.NodeExecStatus{"missing-node": types.NodeExecDone}
			},
			wantReason: readRunSnapshotSeedReasonNodeMismatch,
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
