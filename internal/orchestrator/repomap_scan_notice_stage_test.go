package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestEmitRepoMapScanNoticeCarriesCurrentPipelineStage(t *testing.T) {
	o := &Orchestrator{
		busCtx: &types.BusContext{
			Language:      "zh",
			PipelineStage: types.StageExplore,
			TaskState:     types.TaskState{Stage: types.StageAnalyze},
		},
		language: "zh",
	}
	var got render.Event
	o.SetEmitter(func(ev render.Event) { got = ev })

	o.EmitRepoMapScanNotice(types.RepoMapScanEvent{
		DisplayName:    "core",
		Mode:           types.RepoMapScanCacheHit,
		Started:        true,
		TotalFiles:     1874,
		ParseableFiles: 900,
	})

	if got.Kind != render.EventOrchestratorNotice {
		t.Fatalf("expected EventOrchestratorNotice, got %v", got.Kind)
	}
	if got.Stage != types.StageExplore {
		t.Fatalf("repo_map scan notice must carry current pipeline stage; got %q", got.Stage)
	}
}

func TestEmitMultiRepoScanNoticeFallsBackToTaskStateStage(t *testing.T) {
	o := &Orchestrator{
		busCtx: &types.BusContext{
			Language:  "zh",
			TaskState: types.TaskState{Stage: types.StageAnalyze},
		},
		language: "zh",
	}
	var got render.Event
	o.SetEmitter(func(ev render.Event) { got = ev })

	o.EmitMultiRepoScanNotice("core", "core", true, false, 0)

	if got.Kind != render.EventOrchestratorNotice {
		t.Fatalf("expected EventOrchestratorNotice, got %v", got.Kind)
	}
	if got.Stage != types.StageAnalyze {
		t.Fatalf("multi-repo scan notice must fall back to TaskState stage; got %q", got.Stage)
	}
}
