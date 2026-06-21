package orchestrator

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestDispatchExploreWindowProjectsNodeArtifacts(t *testing.T) {
	ev := projectionEvidence("ev-serial", "serial.go", 7)
	facts := []types.AnswerAggregateFact{projectionAggregateFact("serial members", "Serial")}
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			ctx.Mutable.SetInvestigationAggregateFacts(facts)
			ctx.Mutable.SetInvestigationComplete("serial complete")
			ctx.Mutable.RetainInvestigationAggregateFacts()
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
				UserQuestion:           "q",
				EvidenceItems:          []types.EvidenceItem{ev},
				AcceptedAggregateFacts: facts,
			})
			return &agent.StageOutput{
				EvidenceItems: []types.EvidenceItem{ev},
				AnswerChains:  []types.AnswerChain{{Item: ev, Score: 1, StrictOK: true}},
				MissingPiece:  types.MissingNone,
				SignalUpdates: &types.ExecutionSignals{HasEnoughFacts: true},
			}, nil
		},
	})
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.busCtx = projectionBusContext("serial projection")

	if _, err := o.dispatchExploreWindow([]*types.TaskNode{{ID: "explore-serial", Type: types.NodeEvidence}}); err != nil {
		t.Fatalf("dispatchExploreWindow: %v", err)
	}
	assertNodeArtifactKindCount(t, o.busCtx.Mutable, "explore-serial", types.RuntimeArtifactEvidenceItem, 1)
	assertNodeArtifactKindCount(t, o.busCtx.Mutable, "explore-serial", types.RuntimeArtifactAnswerChain, 1)
	assertNodeArtifactKindCount(t, o.busCtx.Mutable, "explore-serial", types.RuntimeArtifactAggregateFact, 1)

	snapshot := types.ReadRunSnapshotFromBusContext(o.busCtx, "projection-run")
	if got := len(snapshot.NodeArtifacts); got != 3 {
		t.Fatalf("snapshot node_artifacts = %+v, want 3 projected refs", snapshot.NodeArtifacts)
	}

	if _, err := o.dispatchExploreWindow([]*types.TaskNode{{ID: "explore-serial", Type: types.NodeEvidence}}); err != nil {
		t.Fatalf("second dispatchExploreWindow: %v", err)
	}
	if got := len(o.busCtx.Mutable.EvidenceClosure().NodeArtifactLedger().RecordsByProducer("explore-serial")); got != 3 {
		t.Fatalf("self-loop re-emit should not duplicate node artifacts, got %d", got)
	}
}

func TestDispatchExploreWindowProjectsAnalysisRefinementHandoff(t *testing.T) {
	decision := types.ProgressDecision{
		ShouldReplan: true,
		ReasonCode:   "raw user words must not leak",
		Delta: types.ProgressDelta{
			Kind:          types.ProgressDeltaDowngradeBlocker,
			DowngradeLane: types.DowngradeLaneContractChain,
			BlockerKey:    42,
			Consecutive:   1,
			HardThreshold: 3,
		},
	}
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			ctx.Mutable.EvidenceClosure().SetLatestProgressDecision(decision)
			return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
		},
	})
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.busCtx = projectionBusContext("analysis refine projection")
	window := []*types.TaskNode{{
		ID:      "n_refine",
		Type:    types.NodeProbe,
		Outputs: []string{"analysis_refinement_handoff", "progress_decision"},
	}}

	if _, err := o.dispatchExploreWindow(window); err != nil {
		t.Fatalf("dispatchExploreWindow: %v", err)
	}
	records := o.busCtx.Mutable.EvidenceClosure().NodeArtifactLedger().RecordsByProducer("n_refine")
	if len(records) != 1 {
		t.Fatalf("handoff records = %+v, want exactly one", records)
	}
	record := records[0]
	wantID := types.RuntimeArtifactIDForAnalysisRefinementHandoff("n_refine", decision)
	if record.Artifact.Kind != types.RuntimeArtifactAnalysisRefinementHandoff ||
		record.Artifact.ID != wantID ||
		record.Artifact.ContentHash != types.RuntimeArtifactHashForProgressDecision(decision) ||
		record.Consumer != types.RuntimeArtifactConsumerAnalysisRefinement {
		t.Fatalf("unexpected handoff record: %+v want_id=%q", record, wantID)
	}
	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("marshal handoff record: %v", err)
	}
	if strings.Contains(string(raw), "raw user words") {
		t.Fatalf("handoff record leaked progress reason prose: %s", raw)
	}

	if _, err := o.dispatchExploreWindow(window); err != nil {
		t.Fatalf("second dispatchExploreWindow: %v", err)
	}
	if got := len(o.busCtx.Mutable.EvidenceClosure().NodeArtifactLedger().RecordsByProducer("n_refine")); got != 1 {
		t.Fatalf("second dispatch should dedupe handoff artifact, got %d", got)
	}
	snapshot := types.ReadRunSnapshotFromBusContext(o.busCtx, "analysis-refine-projection")
	if got := len(snapshot.NodeArtifacts); got != 1 || snapshot.NodeArtifacts[0].Artifact.Kind != types.RuntimeArtifactAnalysisRefinementHandoff {
		t.Fatalf("snapshot handoff artifacts = %+v", snapshot.NodeArtifacts)
	}
}

func TestDispatchExploreWindowSkipsAnalysisRefinementHandoffWithoutTypedProgress(t *testing.T) {
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
		},
	})
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.busCtx = projectionBusContext("analysis refine false projection")
	if _, err := o.dispatchExploreWindow([]*types.TaskNode{{
		ID:      "n_refine",
		Type:    types.NodeProbe,
		Outputs: []string{"analysis_refinement_handoff"},
	}}); err != nil {
		t.Fatalf("dispatchExploreWindow: %v", err)
	}
	if got := o.busCtx.Mutable.EvidenceClosure().NodeArtifactLedger().RecordsByProducer("n_refine"); len(got) != 0 {
		t.Fatalf("handoff without progress decision should stay absent: %+v", got)
	}
}

func TestDispatchExploreWindowsParallelProjectsAcceptedBranchesOnly(t *testing.T) {
	evA := projectionEvidence("ev-a", "a.go", 1)
	evB := projectionEvidence("ev-b", "b.go", 2)
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			switch ctx.ExploreDispatchKey {
			case "node-a":
				return &agent.StageOutput{EvidenceItems: []types.EvidenceItem{evA}, MissingPiece: types.MissingFacts}, nil
			case "node-b":
				return &agent.StageOutput{EvidenceItems: []types.EvidenceItem{evB}, MissingPiece: types.MissingFacts}, nil
			default:
				t.Fatalf("unexpected dispatch key %q", ctx.ExploreDispatchKey)
				return nil, nil
			}
		},
	})
	o := New(types.PipelineSettings{MaxParallelism: 2}, ar, sr, sar)
	o.busCtx = projectionBusContext("parallel projection")

	if _, err := o.dispatchExploreWindowsParallel([][]*types.TaskNode{
		{{ID: "node-a", Type: types.NodeEvidence}},
		{{ID: "node-b", Type: types.NodeEvidence}},
	}, nil, 2); err != nil {
		t.Fatalf("dispatchExploreWindowsParallel: %v", err)
	}
	assertNodeArtifactKindCount(t, o.busCtx.Mutable, "node-a", types.RuntimeArtifactEvidenceItem, 1)
	assertNodeArtifactKindCount(t, o.busCtx.Mutable, "node-b", types.RuntimeArtifactEvidenceItem, 1)
}

func TestDispatchExploreWindowsParallelSkipsNonWinningNodeArtifacts(t *testing.T) {
	partialDone := make(chan struct{})
	evPartial := projectionEvidence("ev-partial", "partial.go", 3)
	evWinner := projectionEvidence("ev-winner", "winner.go", 4)
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			switch ctx.ExploreDispatchKey {
			case "partial":
				close(partialDone)
				return &agent.StageOutput{
					EvidenceItems: []types.EvidenceItem{evPartial},
					MissingPiece:  types.MissingFacts,
					SignalUpdates: &types.ExecutionSignals{HasEnoughFacts: false},
				}, nil
			case "winner":
				<-partialDone
				ctx.Mutable.SetInvestigationComplete("winner complete")
				return &agent.StageOutput{
					EvidenceItems: []types.EvidenceItem{evWinner},
					MissingPiece:  types.MissingNone,
					SignalUpdates: &types.ExecutionSignals{HasEnoughFacts: true},
				}, nil
			default:
				t.Fatalf("unexpected dispatch key %q", ctx.ExploreDispatchKey)
				return nil, nil
			}
		},
	})
	o := New(types.PipelineSettings{MaxParallelism: 2}, ar, sr, sar)
	o.busCtx = projectionBusContext("parallel winner projection")

	if _, err := o.dispatchExploreWindowsParallel([][]*types.TaskNode{
		{{ID: "partial", Type: types.NodeEvidence}},
		{{ID: "winner", Type: types.NodeEvidence}},
	}, nil, 2); err != nil {
		t.Fatalf("dispatchExploreWindowsParallel: %v", err)
	}
	if got := o.busCtx.Mutable.EvidenceClosure().NodeArtifactLedger().RecordsByProducer("partial"); len(got) != 0 {
		t.Fatalf("non-winning sibling leaked node artifacts: %+v", got)
	}
	assertNodeArtifactKindCount(t, o.busCtx.Mutable, "winner", types.RuntimeArtifactEvidenceItem, 1)
}

func projectionBusContext(objective string) *types.BusContext {
	return &types.BusContext{
		Mutable:       types.NewMutableState(objective),
		Signals:       types.ExecutionSignals{},
		PipelineStage: types.StageAnalyze,
		ActiveAgent:   types.AgentAnalyzer,
		TaskState:     types.TaskState{Stage: types.StageAnalyze},
	}
}

func projectionEvidence(id, path string, line int) types.EvidenceItem {
	return types.EvidenceItem{
		ID:           id,
		Kind:         types.EvidenceDirect,
		Subject:      strings.TrimSuffix(path, ".go"),
		Predicate:    "defines",
		Object:       id,
		Source:       path,
		LineStart:    line,
		LineEnd:      line,
		Scope:        types.ScopeLine,
		AnchorKind:   types.AnchorDefinition,
		AnchorSymbol: id,
	}
}

func projectionAggregateFact(label, member string) types.AnswerAggregateFact {
	return types.AnswerAggregateFact{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   label,
		Value:   "1",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{member},
	}
}

func assertNodeArtifactKindCount(t *testing.T, mut *types.MutableState, producer string, kind types.RuntimeArtifactKind, want int) {
	t.Helper()
	if mut == nil {
		t.Fatal("nil mutable state")
	}
	got := 0
	for _, record := range mut.EvidenceClosure().NodeArtifactLedger().RecordsByProducer(producer) {
		if record.Artifact.Kind == kind {
			got++
		}
	}
	if got != want {
		t.Fatalf("producer=%s kind=%s count=%d want=%d records=%+v",
			producer, kind, got, want, mut.EvidenceClosure().NodeArtifactLedger().RecordsByProducer(producer))
	}
}
