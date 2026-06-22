package orchestrator

import (
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestReadStatusDebounceSuppressesRepeatedNoticeWithoutProgress(t *testing.T) {
	o := New(types.PipelineSettings{}, nil, nil, nil)
	o.busCtx = &types.BusContext{
		Mode:          types.ModeRead,
		PipelineStage: types.StageExplore,
		Mutable:       types.NewMutableState("read status debounce"),
	}
	var events []render.Event
	o.SetEmitter(func(ev render.Event) {
		events = append(events, ev)
	})
	ev := render.Event{
		Kind:       render.EventOrchestratorNotice,
		Timestamp:  time.Now(),
		Stage:      types.StageExplore,
		Agent:      "orchestrator",
		NoticeKind: render.NoticeRetry,
		Reasoning:  "deterministic retry status",
	}
	o.emit(ev)
	o.emit(ev)
	if len(events) != 1 {
		t.Fatalf("same read notice at same progress cursor should emit once, got %d: %+v", len(events), events)
	}

	o.busCtx.EvidenceItems = append(o.busCtx.EvidenceItems, types.EvidenceItem{ID: "ev-progress"})
	o.emit(ev)
	if len(events) != 2 {
		t.Fatalf("read notice should re-emit after typed evidence progress, got %d: %+v", len(events), events)
	}
}

func TestReadStatusDebounceSuppressesEvidenceNodeLifecycleFlapWithoutProgress(t *testing.T) {
	o := New(types.PipelineSettings{}, nil, nil, nil)
	o.busCtx = &types.BusContext{
		Mode:          types.ModeRead,
		PipelineStage: types.StageExplore,
		Mutable:       types.NewMutableState("read status debounce"),
	}
	var events []render.Event
	o.SetEmitter(func(ev render.Event) {
		events = append(events, ev)
	})
	start := render.Event{
		Kind:      render.EventTaskNodeStart,
		Timestamp: time.Now(),
		NodeID:    "n_evidence_1",
		NodeKind:  string(types.NodeEvidence),
	}
	done := render.Event{
		Kind:      render.EventTaskNodeEnd,
		Timestamp: time.Now(),
		NodeID:    "n_evidence_1",
		NodeKind:  string(types.NodeEvidence),
	}
	o.emit(start)
	o.emit(done)
	o.emit(start)
	o.emit(done)
	if len(events) != 2 {
		t.Fatalf("same evidence lifecycle at same progress cursor should collapse to one start/end pair, got %d: %+v", len(events), events)
	}

	o.busCtx.ToolResults = append(o.busCtx.ToolResults, types.ToolResult{ToolName: "repo_map", Success: true})
	o.emit(start)
	o.emit(done)
	if len(events) != 4 {
		t.Fatalf("evidence lifecycle should re-emit after typed tool progress, got %d: %+v", len(events), events)
	}
}

func TestReadStatusDebounceDoesNotSuppressWriteModeNotices(t *testing.T) {
	o := New(types.PipelineSettings{}, nil, nil, nil)
	o.busCtx = &types.BusContext{
		Mode:          types.ModeApply,
		PipelineStage: types.StageExplore,
		Mutable:       types.NewMutableState("write status debounce"),
	}
	var events []render.Event
	o.SetEmitter(func(ev render.Event) {
		events = append(events, ev)
	})
	ev := render.Event{
		Kind:       render.EventOrchestratorNotice,
		Timestamp:  time.Now(),
		Stage:      types.StageExplore,
		Agent:      "orchestrator",
		NoticeKind: render.NoticeRetry,
		Reasoning:  "write retry status",
	}
	o.emit(ev)
	o.emit(ev)
	if len(events) != 2 {
		t.Fatalf("write mode notices must not be suppressed by read-status debounce, got %d: %+v", len(events), events)
	}
}
