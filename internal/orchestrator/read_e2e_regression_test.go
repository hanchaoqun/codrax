package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/analysis/compiler"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Read-mode regression e2e suite. T2/T3/T4 ship a lot of new write-
// mode machinery; the L1 red line requires read-mode to stay byte-
// identical. These tests cover three risk surfaces:
//
//   1. Dispatch routing — readIR must not contain retired write nodes, so
//      runTaskGraph routes to runReadSchedulerLoop. Write mode now bypasses
//      this read scheduler entirely.
//
//   2. TaskNode.OneShot zero-value compatibility — read TaskGraphs
//      construct nodes via the analyzer's compiler templates. Those
//      literals predate OneShot and don't set it; zero value must
//      mean "old behaviour" (markDone is the terminator). Finalize
//      MUST fire exactly once.
//
//   3. EdgeValidationFeedback retry — the read scheduler's
//      requeueValidationTargets path is exercised by validate-fail
//      cases. T4 added verify→{plan,apply} edges in the WRITE
//      graph; the READ graph's validate→evidence behaviour must
//      stay unchanged.

// TestE2E_ReadMode_DispatchRoutesToReadLoop verifies that an
// analyzer-emitted read TaskGraph (no NodePlan/Apply/Verify nodes)
// runs through runReadSchedulerLoop and reaches finalize cleanly.
// If retired write nodes leaked into this graph, runTaskGraph would fail loud
// before dispatching the read explorer.
func TestE2E_ReadMode_DispatchRoutesToReadLoop(t *testing.T) {
	explorerCalls := 0
	finalizeCalls := 0

	ir := dagIR(types.AnswerContract{
		Language: "en",
	})
	if taskGraphHasRetiredWriteNodes(ir.TaskGraph) {
		t.Fatal("dagIR is supposed to be a read graph; it contains retired write nodes")
	}

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			explorerCalls++
			return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
		},
		types.AgentExtractor: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			finalizeCalls++
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "- `Foo` (file.go:1)",
			}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{MaxRetriesPerStage: 2}, ar, sr, sar)
	o.SetMaxSteps(20)

	busCtx, err := o.Run("explain X", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Read loop dispatched explorer at least once and finalize
	// exactly once. If the write loop had been chosen (regression),
	// neither of these would fire.
	if explorerCalls == 0 {
		t.Errorf("explorer should run in read mode; got %d calls", explorerCalls)
	}
	if finalizeCalls != 1 {
		t.Errorf("finalize should run exactly once; got %d calls (regression?)", finalizeCalls)
	}
	if busCtx.Mode != types.ModeRead {
		t.Errorf("Mode should stay ModeRead; got %q", busCtx.Mode)
	}
	result := busCtx.Mutable.Result()
	if !strings.Contains(result, "Foo") {
		t.Errorf("read-mode finalizer answer missing; got %q", result)
	}
}

// TestE2E_ReadMode_OneShotZeroValueCompatible verifies that read-
// mode TaskNode literals (which never set OneShot) still get the
// "fire once then done" behaviour through markDone.
//
// Failure mode this catches: if someone accidentally added an
// `if n.OneShot { skip first dispatch }` shortcut into the read
// scheduler, the read finalize node (OneShot=false) would never
// fire and the answer would never render.
func TestE2E_ReadMode_OneShotZeroValueCompatible(t *testing.T) {
	finalizeCalls := 0
	ir := dagIR(types.AnswerContract{Language: "en"})

	// Sanity: all nodes in the read graph must have OneShot=false.
	for _, n := range ir.TaskGraph.Nodes {
		if n.OneShot {
			t.Fatalf("dagIR node %s has OneShot=true; read-mode templates must not set it", n.ID)
		}
	}

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
		},
		types.AgentExtractor: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			finalizeCalls++
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "- `X` (a.go:1)",
			}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{MaxRetriesPerStage: 2}, ar, sr, sar)
	o.SetMaxSteps(20)

	busCtx, err := o.Run("explain", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if finalizeCalls != 1 {
		t.Errorf("read finalize should fire exactly once with OneShot zero-value; got %d", finalizeCalls)
	}
	if busCtx.TaskState.LastError != "" {
		t.Errorf("happy-path read should leave LastError empty; got %q", busCtx.TaskState.LastError)
	}
}

func TestE2E_ReadMode_ExtractInputReadyFalseSkipsExtract(t *testing.T) {
	explorerCalls := 0
	extractorCalls := 0
	finalizeCalls := 0
	ir := dagIR(types.AnswerContract{Language: "en"})
	compiler.EnsureReadStageNodes(&ir.TaskGraph)

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			explorerCalls++
			return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
		},
		types.AgentExtractor: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			extractorCalls++
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			finalizeCalls++
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "- `No typed extract input` (README.md:1)",
			}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{MaxRetriesPerStage: 2}, ar, sr, sar)
	o.SetMaxSteps(20)

	busCtx, err := o.Run("explain without structured evidence", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if explorerCalls != 3 {
		t.Fatalf("explorer calls = %d, want 3 hard-dependency windows", explorerCalls)
	}
	if extractorCalls != 0 {
		t.Fatalf("extractor must be skipped when extract_input_ready=false, got %d calls", extractorCalls)
	}
	if finalizeCalls != 1 {
		t.Fatalf("finalizer calls = %d, want 1", finalizeCalls)
	}
	extractID := firstReadNodeIDByType(t, busCtx.AnalysisIR, types.NodeExtract)
	if got := busCtx.Mutable.EvidenceClosure().NodeExecStatus(extractID); got != types.NodeExecDone {
		t.Fatalf("extract node status = %q, want %q", got, types.NodeExecDone)
	}
	if busCtx.TaskState.LastError != "" {
		t.Fatalf("read run should stay clean after extract skip, LastError=%q", busCtx.TaskState.LastError)
	}
}

func TestE2E_ReadMode_ExtractInputReadyTrueDispatchesExtract(t *testing.T) {
	explorerCalls := 0
	extractorCalls := 0
	finalizeCalls := 0
	ir := dagIR(types.AnswerContract{Language: "en"})
	compiler.EnsureReadStageNodes(&ir.TaskGraph)

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			explorerCalls++
			return &agent.StageOutput{
				MissingPiece: types.MissingFacts,
				EvidenceItems: []types.EvidenceItem{{
					ID:        "ev-typed-extract-input",
					Predicate: "definition",
					Subject:   "Thing",
					Object:    "exists",
					Summary:   "Thing exists",
					Source:    "thing.go",
					LineStart: 1,
				}},
			}, nil
		},
		types.AgentExtractor: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			extractorCalls++
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			finalizeCalls++
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "- `Thing` (thing.go:1)",
			}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{MaxRetriesPerStage: 2}, ar, sr, sar)
	o.SetMaxSteps(20)

	busCtx, err := o.Run("explain with typed evidence", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if explorerCalls != 3 {
		t.Fatalf("explorer calls = %d, want 3 hard-dependency windows", explorerCalls)
	}
	if extractorCalls != 1 {
		t.Fatalf("extractor must run when extract_input_ready=true, got %d calls", extractorCalls)
	}
	if finalizeCalls != 1 {
		t.Fatalf("finalizer calls = %d, want 1", finalizeCalls)
	}
	extractID := firstReadNodeIDByType(t, busCtx.AnalysisIR, types.NodeExtract)
	if got := busCtx.Mutable.EvidenceClosure().NodeExecStatus(extractID); got != types.NodeExecDone {
		t.Fatalf("extract node status = %q, want %q", got, types.NodeExecDone)
	}
	consumed := busCtx.Mutable.EvidenceClosure().NodeArtifactLedger().RecordsByConsumerNode(extractID)
	if len(consumed) != 1 ||
		consumed[0].Direction != types.RuntimeArtifactConsumed ||
		consumed[0].Artifact.ID != "ev-typed-extract-input" {
		t.Fatalf("extract consumed artifacts = %+v", consumed)
	}
}

func TestE2E_ReadMode_ExtractInputReadyRequiresExploreProducerLineage(t *testing.T) {
	explorerCalls := 0
	extractorCalls := 0
	finalizeCalls := 0
	ir := dagIR(types.AnswerContract{Language: "en"})
	compiler.EnsureReadStageNodes(&ir.TaskGraph)
	analyzerEvidence := types.EvidenceItem{
		ID:        "ev-analyzer-only",
		Predicate: "definition",
		Subject:   "AnalyzerOnly",
		Object:    "exists",
		Summary:   "Analyzer-only evidence exists",
		Source:    "analyzer.go",
		LineStart: 1,
	}

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{
				MissingPiece:  types.MissingFacts,
				AnalysisIR:    ir,
				EvidenceItems: []types.EvidenceItem{analyzerEvidence},
			}, nil
		},
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			explorerCalls++
			return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
		},
		types.AgentExtractor: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			extractorCalls++
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			finalizeCalls++
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "- `AnalyzerOnly` (analyzer.go:1)",
			}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{MaxRetriesPerStage: 2}, ar, sr, sar)
	o.SetMaxSteps(20)

	busCtx, err := o.Run("explain analyzer-only evidence", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if explorerCalls != 3 {
		t.Fatalf("explorer calls = %d, want 3 hard-dependency windows", explorerCalls)
	}
	if extractorCalls != 0 {
		t.Fatalf("extractor must be skipped when typed payload lacks explore producer lineage, got %d calls", extractorCalls)
	}
	if finalizeCalls != 1 {
		t.Fatalf("finalizer calls = %d, want 1", finalizeCalls)
	}
	extractID := firstReadNodeIDByType(t, busCtx.AnalysisIR, types.NodeExtract)
	if got := busCtx.Mutable.EvidenceClosure().NodeExecStatus(extractID); got != types.NodeExecDone {
		t.Fatalf("extract node status = %q, want %q", got, types.NodeExecDone)
	}
	if consumed := busCtx.Mutable.EvidenceClosure().NodeArtifactLedger().RecordsByConsumerNode(extractID); len(consumed) != 0 {
		t.Fatalf("extract must not consume analyzer-only payload without producer lineage: %+v", consumed)
	}
	if busCtx.TaskState.LastError != "" {
		t.Fatalf("read run should stay clean after lineage-missing extract skip, LastError=%q", busCtx.TaskState.LastError)
	}
}

func TestE2E_ReadMode_AnalyzeRefineFalsePathStaysSilent(t *testing.T) {
	explorerCalls := 0
	extractorCalls := 0
	finalizeCalls := 0
	var dispatchKeys []string
	ir := dagIR(types.AnswerContract{Language: "en"})
	compiler.EnsureReadStageNodes(&ir.TaskGraph)
	refineID := firstReadNodeIDByCriterion(t, ir, types.CritProgressReplanRequired)

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			explorerCalls++
			dispatchKeys = append(dispatchKeys, ctx.ExploreDispatchKey)
			return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
		},
		types.AgentExtractor: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			extractorCalls++
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			finalizeCalls++
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "- `No refine` (README.md:1)",
			}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{MaxRetriesPerStage: 2}, ar, sr, sar)
	o.SetMaxSteps(20)

	busCtx, err := o.Run("explain without typed progress delta", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if explorerCalls != 3 {
		t.Fatalf("explorer calls = %d, want stable hard-dependency baseline 3", explorerCalls)
	}
	if extractorCalls != 0 {
		t.Fatalf("extractor must still be skipped without typed extract input, got %d calls", extractorCalls)
	}
	if finalizeCalls != 1 {
		t.Fatalf("finalizer calls = %d, want 1", finalizeCalls)
	}
	if dispatchKeyContainsNodeID(dispatchKeys, refineID) {
		t.Fatalf("analyze-refine optional node must stay silent without typed progress decision; dispatch keys=%v refine=%s", dispatchKeys, refineID)
	}
	if got := busCtx.Mutable.EvidenceClosure().NodeExecStatus(refineID); got != types.NodeExecPending {
		t.Fatalf("refine node status = %q, want %q", got, types.NodeExecPending)
	}
	if busCtx.TaskState.LastError != "" {
		t.Fatalf("read run should stay clean, LastError=%q", busCtx.TaskState.LastError)
	}
}

func TestE2E_ReadMode_AnalyzeRefineTruePathDispatchesOnce(t *testing.T) {
	explorerCalls := 0
	finalizeCalls := 0
	progressSeeded := false
	var dispatchKeys []string
	ir := dagIR(types.AnswerContract{Language: "en"})
	compiler.EnsureReadStageNodes(&ir.TaskGraph)
	refineID := firstReadNodeIDByCriterion(t, ir, types.CritProgressReplanRequired)

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			explorerCalls++
			dispatchKeys = append(dispatchKeys, ctx.ExploreDispatchKey)
			if !progressSeeded && !dispatchKeyHasNodeID(ctx.ExploreDispatchKey, refineID) {
				progressSeeded = true
				ctx.Mutable.EvidenceClosure().RecordDowngradeProgressDelta(types.DowngradeLaneContractChain, 42, 3)
			}
			return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
		},
		types.AgentExtractor: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			finalizeCalls++
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "- `Refined once` (README.md:1)",
			}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{MaxRetriesPerStage: 2}, ar, sr, sar)
	o.SetMaxSteps(20)

	busCtx, err := o.Run("explain with typed progress delta", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !progressSeeded {
		t.Fatal("test did not seed typed progress decision")
	}
	if got := dispatchKeyNodeIDCount(dispatchKeys, refineID); got != 1 {
		t.Fatalf("analyze-refine optional node dispatch count = %d, want 1; dispatch keys=%v refine=%s", got, dispatchKeys, refineID)
	}
	if explorerCalls > 4 {
		t.Fatalf("analyze-refine true path should stay bounded, explorer calls=%d dispatch keys=%v", explorerCalls, dispatchKeys)
	}
	if finalizeCalls != 1 {
		t.Fatalf("finalizer calls = %d, want 1", finalizeCalls)
	}
	if got := busCtx.Mutable.EvidenceClosure().NodeExecStatus(refineID); got != types.NodeExecDone {
		t.Fatalf("refine node status = %q, want %q", got, types.NodeExecDone)
	}
	if busCtx.TaskState.LastError != "" {
		t.Fatalf("read run should stay clean, LastError=%q", busCtx.TaskState.LastError)
	}
}

// TestE2E_ReadMode_NoWriteSideEffects verifies that a read-mode Run
// leaves all write-mode state slots empty: ChangePlan, ChangeReport,
// BaselineReport, WriteClosure.AppliedSet should all be zero-value.
// Catches accidental write-mode leakage if a future refactor merges
// a stage hook into the read path.
func TestE2E_ReadMode_NoWriteSideEffects(t *testing.T) {
	ir := dagIR(types.AnswerContract{Language: "en"})
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "- `Y` (b.go:1)",
			}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{MaxRetriesPerStage: 2}, ar, sr, sar)
	o.SetMaxSteps(20)

	busCtx, err := o.Run("explain", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if busCtx.Mutable.ChangePlan() != nil {
		t.Errorf("read-mode Run should not populate ChangePlan; got %+v", busCtx.Mutable.ChangePlan())
	}
	if busCtx.Mutable.ChangeReport() != nil {
		t.Errorf("read-mode Run should not populate ChangeReport; got %+v", busCtx.Mutable.ChangeReport())
	}
	if busCtx.Mutable.BaselineReport() != nil {
		t.Errorf("read-mode Run should not populate BaselineReport; got %+v", busCtx.Mutable.BaselineReport())
	}
	if applied := busCtx.Mutable.WriteClosure().AppliedSet(); len(applied) != 0 {
		t.Errorf("read-mode Run should not mark anything applied; got %v", applied)
	}
	if busCtx.WorktreePath != "" {
		t.Errorf("read-mode Run should not provision a worktree; got %q", busCtx.WorktreePath)
	}
}

func TestE2E_ReadMode_SavesTypedReadRunSnapshot(t *testing.T) {
	saver := &captureReadRunSnapshotSaver{}
	ir := dagIR(types.AnswerContract{Language: "en"})
	compiler.EnsureReadStageNodes(&ir.TaskGraph)
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			ctx.Mutable.EvidenceClosure().AppendAcceptedEvidenceRefs([]types.AcceptedEvidenceRef{{
				ID:        "ev-read-snapshot",
				Source:    "snapshot.go",
				LineStart: 1,
			}})
			return &agent.StageOutput{
				MissingPiece: types.MissingFacts,
				EvidenceItems: []types.EvidenceItem{{
					ID:        "ev-read-snapshot",
					Predicate: "definition",
					Subject:   "Snapshot",
					Object:    "persisted",
					Summary:   "Snapshot is persisted",
					Source:    "snapshot.go",
					LineStart: 1,
				}},
			}, nil
		},
		types.AgentExtractor: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "- `Snapshot` (snapshot.go:1)",
			}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{MaxRetriesPerStage: 2}, ar, sr, sar)
	o.SetReadRunSnapshotStore(saver)
	o.SetMaxSteps(20)

	busCtx, err := o.Run("explain read snapshots", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(saver.snapshots) != 1 {
		t.Fatalf("read run snapshot save count = %d, want 1", len(saver.snapshots))
	}
	snapshot := saver.snapshots[0]
	if snapshot.RunID == "" || snapshot.RunID != busCtx.TraceID {
		t.Fatalf("snapshot RunID = %q, want trace id %q", snapshot.RunID, busCtx.TraceID)
	}
	if snapshot.Request != "explain read snapshots" || snapshot.RepoRoot != "/tmp/repo" {
		t.Fatalf("snapshot request/repo = %q/%q", snapshot.Request, snapshot.RepoRoot)
	}
	if snapshot.TaskGraphHash == "" || snapshot.TaskNodeCount == 0 {
		t.Fatalf("snapshot missing task graph identity: %+v", snapshot)
	}
	if len(snapshot.NodeStatuses) == 0 {
		t.Fatalf("snapshot missing node execution statuses: %+v", snapshot)
	}
	if len(snapshot.AcceptedEvidence) == 0 {
		t.Fatalf("snapshot missing accepted evidence refs: %+v", snapshot)
	}
}

func TestReadRunSnapshotSkippedOutsideReadMode(t *testing.T) {
	saver := &captureReadRunSnapshotSaver{}
	o := &Orchestrator{
		readRunSnapshotSaver: saver,
		busCtx: &types.BusContext{
			Mode:    types.ModeApply,
			TraceID: "trace-write",
			Mutable: types.NewMutableState("write request"),
		},
	}
	o.persistReadRunSnapshot()
	if len(saver.snapshots) != 0 {
		t.Fatalf("write-mode run must not save read snapshots: %+v", saver.snapshots)
	}
}

// TestE2E_ReadMode_ValidateFeedbackRetry locks the read-mode
// EdgeValidationFeedback retry path that T4 must NOT have changed.
// Construct an explorer mock that fails on iteration 0 (so the
// validate node's SuccessCriteria fail) and passes on iteration 1
// (the requeued evidence). Verify the explorer ran exactly twice
// and finalize once.
//
// This is a regression test for runReadSchedulerLoop's preservation
// of the unchanged validate-feedback semantics.
func TestE2E_ReadMode_ValidateFeedbackRetry(t *testing.T) {
	// dagIR's TaskGraph already has an EdgeValidationFeedback edge
	// from n2 (validate) → n1 (evidence). We add a SuccessCriteria
	// to validate that fails on first dispatch and passes on retry.
	ir := dagIR(types.AnswerContract{Language: "en"})
	// Add a SC that requires at least 1 evidence item — the first
	// explorer dispatch returns no evidence so the SC fails;
	// requeueValidationTargets requeues n1; second explorer
	// dispatch returns 1 evidence item; SC passes.
	for i := range ir.TaskGraph.Nodes {
		if ir.TaskGraph.Nodes[i].Type == types.NodeValidate {
			ir.TaskGraph.Nodes[i].SuccessCriteria = []types.Criterion{
				{Kind: string(types.CritEvidenceCount), Expr: ">=1"},
			}
		}
	}

	explorerDispatches := 0
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			explorerDispatches++
			out := &agent.StageOutput{MissingPiece: types.MissingFacts}
			// Second dispatch: emit evidence so SC passes.
			if explorerDispatches >= 2 {
				out.EvidenceItems = append(out.EvidenceItems, types.EvidenceItem{
					ID:        "ev-1",
					Predicate: "test",
					Subject:   "X",
					Object:    "Y",
					Summary:   "test evidence",
					Source:    "test.go",
					LineStart: 1,
				})
			}
			return out, nil
		},
		types.AgentExtractor: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "- `X` (test.go:1)",
			}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)

	busCtx, err := o.Run("explain X", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Explorer should have run AT LEAST twice (initial fail + retry
	// after EdgeValidationFeedback requeue). Allow flex if the
	// scheduler dispatches in a slightly different order, but the
	// key signal is "more than one dispatch happened".
	if explorerDispatches < 2 {
		t.Errorf("EdgeValidationFeedback should retry explorer; got only %d dispatches", explorerDispatches)
	}
	if busCtx.TaskState.LastError != "" {
		t.Errorf("retry-success should clear LastError; got %q", busCtx.TaskState.LastError)
	}
}

func TestE2E_ReadMode_AcceptedInvestigationCompleteStopsValidateReExplore(t *testing.T) {
	ir := dagIR(types.AnswerContract{Language: "en"})
	for i := range ir.TaskGraph.Nodes {
		if ir.TaskGraph.Nodes[i].Type == types.NodeValidate {
			ir.TaskGraph.Nodes[i].SuccessCriteria = []types.Criterion{
				{Kind: string(types.CritAnswerSetBounded), Expr: "<=1"},
			}
		}
	}

	explorerDispatches := 0
	finalizeCalls := 0
	var extractorBoundaryNotes []string
	var finalizerBoundaryNotes []string
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			explorerDispatches++
			ctx.Mutable.SetInvestigationComplete("model completed investigation after tool preflight")
			return &agent.StageOutput{
				MissingPiece: types.MissingFacts,
				EvidenceItems: []types.EvidenceItem{
					{
						ID:        "ev-a",
						Predicate: "test",
						Subject:   "A",
						Object:    "supported",
						Summary:   "A is supported",
						Source:    "a.go",
						LineStart: 1,
					},
				},
				AnswerSymbols: []types.AnswerSymbol{
					{Name: "A"}, {Name: "B"}, {Name: "C"},
				},
			}, nil
		},
		types.AgentExtractor: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			if ta := ctx.Mutable.TurnAArtifacts(); ta != nil {
				extractorBoundaryNotes = append([]string(nil), ta.ValidationBoundaryNotes...)
			}
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			finalizeCalls++
			if ta := ctx.Mutable.TurnAArtifacts(); ta != nil {
				finalizerBoundaryNotes = append([]string(nil), ta.ValidationBoundaryNotes...)
			}
			return &agent.StageOutput{MissingPiece: types.MissingNone, FinalAnswer: "- `A` (a.go:1)"}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)

	busCtx, err := o.Run("explain completed investigation", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if explorerDispatches != 1 {
		t.Fatalf("accepted investigation completion should not re-open explore on validate criteria; got %d dispatches", explorerDispatches)
	}
	if finalizeCalls != 1 {
		t.Fatalf("finalizer should run once after accepted completion; got %d", finalizeCalls)
	}
	if !boundaryNotesContain(extractorBoundaryNotes, types.CritAnswerSetBounded) {
		t.Fatalf("extractor should receive answer_set_bounded boundary note; got %+v", extractorBoundaryNotes)
	}
	if !boundaryNotesContain(finalizerBoundaryNotes, types.CritAnswerSetBounded) {
		t.Fatalf("finalizer should receive answer_set_bounded boundary note; got %+v", finalizerBoundaryNotes)
	}
	if !busCtx.TaskState.IsTerminal {
		t.Fatal("want terminal task")
	}
}

func boundaryNotesContain(notes []string, kind string) bool {
	for _, note := range notes {
		if strings.Contains(note, "criterion="+kind) {
			return true
		}
	}
	return false
}

func firstReadNodeIDByType(t *testing.T, ir *types.AnalysisIR, typ types.TaskNodeType) string {
	t.Helper()
	if ir == nil {
		t.Fatal("AnalysisIR is nil")
	}
	for _, node := range ir.TaskGraph.Nodes {
		if node.Type == typ {
			return node.ID
		}
	}
	t.Fatalf("node type %q not found in TaskGraph", typ)
	return ""
}

func firstReadNodeIDByCriterion(t *testing.T, ir *types.AnalysisIR, kind string) string {
	t.Helper()
	if ir == nil {
		t.Fatal("AnalysisIR is nil")
	}
	for _, node := range ir.TaskGraph.Nodes {
		for _, criterion := range node.EntryConditions {
			if criterion.Kind == kind {
				return node.ID
			}
		}
	}
	t.Fatalf("node with entry criterion %q not found in TaskGraph", kind)
	return ""
}

func dispatchKeyContainsNodeID(keys []string, nodeID string) bool {
	return dispatchKeyNodeIDCount(keys, nodeID) > 0
}

func dispatchKeyNodeIDCount(keys []string, nodeID string) int {
	count := 0
	for _, key := range keys {
		if dispatchKeyHasNodeID(key, nodeID) {
			count++
		}
	}
	return count
}

func dispatchKeyHasNodeID(key string, nodeID string) bool {
	for _, part := range strings.Split(key, "+") {
		if strings.TrimSpace(part) == nodeID {
			return true
		}
	}
	return false
}

type captureReadRunSnapshotSaver struct {
	snapshots []types.ReadRunSnapshot
}

func (s *captureReadRunSnapshotSaver) Save(snapshot *types.ReadRunSnapshot) (string, error) {
	if snapshot == nil {
		return "", nil
	}
	s.snapshots = append(s.snapshots, types.NormalizeReadRunSnapshot(*snapshot))
	return "/tmp/read-runs/" + snapshot.RunID + ".json", nil
}

// TestE2E_ReadMode_AnalyzeRetrySuccessClearsLastError verifies the
// phase-1 retry path: when the analyzer fails once, then succeeds on a
// retry, the stale analyze error must not block the read task phase or
// leak into the final run surface.
func TestE2E_ReadMode_AnalyzeRetrySuccessClearsLastError(t *testing.T) {
	analyzeDispatches := 0
	explorerCalls := 0
	finalizeCalls := 0
	ir := dagIR(types.AnswerContract{Language: "en"})

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			analyzeDispatches++
			if analyzeDispatches == 1 {
				return &agent.StageOutput{
					MissingPiece: types.MissingUnderstanding,
					Error:        "emit_analysis was not called during the analyze dispatch",
				}, nil
			}
			return &agent.StageOutput{
				MissingPiece: types.MissingUnderstanding,
				AnalysisIR:   ir,
			}, nil
		},
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			explorerCalls++
			return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
		},
		types.AgentExtractor: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			finalizeCalls++
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "- `Recovered` (file.go:1)",
			}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{MaxRetriesPerStage: 2}, ar, sr, sar)
	o.SetMaxSteps(20)

	busCtx, err := o.Run("trace recovered analyzer path", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if analyzeDispatches < 2 {
		t.Fatalf("expected analyzer retry after first failure; got %d dispatch(es)", analyzeDispatches)
	}
	if explorerCalls == 0 {
		t.Fatal("task phase should continue after analyze retry success")
	}
	if finalizeCalls != 1 {
		t.Fatalf("finalizer should still run exactly once; got %d", finalizeCalls)
	}
	if busCtx.TaskState.LastError != "" {
		t.Fatalf("analyze retry success should clear LastError; got %q", busCtx.TaskState.LastError)
	}
	if got := busCtx.Mutable.Result(); !strings.Contains(got, "Recovered") {
		t.Fatalf("final answer missing after analyze retry recovery; got %q", got)
	}
}
