package orchestrator

import (
	"strings"
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

func TestReadStatusDebounceSuppressesSkippedReadNodeLifecycle(t *testing.T) {
	o := New(types.PipelineSettings{}, nil, nil, nil)
	o.busCtx = &types.BusContext{
		Mode:          types.ModeRead,
		PipelineStage: types.StageExplore,
		Mutable:       types.NewMutableState("read status skipped lifecycle"),
	}
	var events []render.Event
	o.SetEmitter(func(ev render.Event) {
		events = append(events, ev)
	})
	now := time.Now()
	for _, kind := range []types.TaskNodeType{types.NodeEvidence, types.NodeProbe} {
		start := render.Event{
			Kind:        render.EventTaskNodeStart,
			Timestamp:   now,
			NodeID:      "skip-" + string(kind),
			NodeKind:    string(kind),
			NodeSkipped: true,
		}
		end := render.Event{
			Kind:        render.EventTaskNodeEnd,
			Timestamp:   now.Add(time.Millisecond),
			NodeID:      "skip-" + string(kind),
			NodeKind:    string(kind),
			NodeSkipped: true,
		}
		o.emit(start)
		o.emit(end)
	}
	if len(events) != 0 {
		t.Fatalf("skipped read node lifecycle is internal DAG bookkeeping and should not render, got %d: %+v", len(events), events)
	}
	o.emit(render.Event{
		Kind:        render.EventTaskNodeEnd,
		Timestamp:   now,
		NodeID:      "skip-reconcile",
		NodeKind:    string(types.NodeReconcile),
		NodeSkipped: true,
	})
	if len(events) != 1 {
		t.Fatalf("skipped reconcile terminal event should remain structurally observable, got %d: %+v", len(events), events)
	}
}

func TestReadStatusDebounceCoalescesReadFamilyRestartAfterSuccessfulEnd(t *testing.T) {
	o := New(types.PipelineSettings{}, nil, nil, nil)
	o.busCtx = &types.BusContext{
		Mode:          types.ModeRead,
		PipelineStage: types.StageExplore,
		Mutable:       types.NewMutableState("read status lifecycle coalescer"),
	}
	var events []render.Event
	o.SetEmitter(func(ev render.Event) {
		events = append(events, ev)
	})
	now := time.Now()
	start := render.Event{
		Kind:         render.EventTaskNodeStart,
		Timestamp:    now,
		NodeID:       "n_evidence_1",
		NodeKind:     string(types.NodeEvidence),
		DispatchKind: string(types.NodeEvidence),
	}
	doneAfterProgress := render.Event{
		Kind:         render.EventTaskNodeEnd,
		Timestamp:    now.Add(time.Millisecond),
		NodeID:       "n_evidence_1",
		NodeKind:     string(types.NodeEvidence),
		DispatchKind: string(types.NodeEvidence),
	}

	o.emit(start)
	o.busCtx.ToolResults = append(o.busCtx.ToolResults, types.ToolResult{ToolName: "repo_map", Success: true})
	o.emit(doneAfterProgress)
	restartWithoutProgress := start
	restartWithoutProgress.NodeID = "n_evidence_2"
	restartWithoutProgress.Timestamp = now.Add(2 * time.Millisecond)
	o.emit(restartWithoutProgress)
	if len(events) != 2 {
		t.Fatalf("successful evidence end followed by same-family restart at same cursor should suppress restart, got %d: %+v", len(events), events)
	}

	o.busCtx.ToolResults = append(o.busCtx.ToolResults, types.ToolResult{ToolName: "read_file", Success: true})
	restartAfterProgress := restartWithoutProgress
	restartAfterProgress.NodeID = "n_evidence_3"
	restartAfterProgress.Timestamp = now.Add(3 * time.Millisecond)
	o.emit(restartAfterProgress)
	if len(events) != 3 {
		t.Fatalf("same-family restart should re-emit after typed progress, got %d: %+v", len(events), events)
	}
}

func TestReadStatusDebounceDoesNotSuppressReadFamilyRestartAfterError(t *testing.T) {
	o := New(types.PipelineSettings{}, nil, nil, nil)
	o.busCtx = &types.BusContext{
		Mode:          types.ModeRead,
		PipelineStage: types.StageExplore,
		Mutable:       types.NewMutableState("read status lifecycle coalescer"),
	}
	var events []render.Event
	o.SetEmitter(func(ev render.Event) {
		events = append(events, ev)
	})
	now := time.Now()
	start := render.Event{
		Kind:         render.EventTaskNodeStart,
		Timestamp:    now,
		NodeID:       "n_probe_1",
		DispatchKind: string(types.NodeProbe),
	}
	failed := render.Event{
		Kind:         render.EventTaskNodeEnd,
		Timestamp:    now.Add(time.Millisecond),
		NodeID:       "n_probe_1",
		DispatchKind: string(types.NodeProbe),
		Error:        "transient provider error",
	}
	restart := start
	restart.NodeID = "n_probe_2"
	restart.Timestamp = now.Add(2 * time.Millisecond)

	o.emit(start)
	o.emit(failed)
	o.emit(restart)
	if len(events) != 3 {
		t.Fatalf("error and retry start must stay visible, got %d: %+v", len(events), events)
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

func TestReadStatusDebounceDoesNotSuppressWriteModeTaskNodeLifecycle(t *testing.T) {
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
	start := render.Event{
		Kind:         render.EventTaskNodeStart,
		Timestamp:    time.Now(),
		NodeID:       "write-evidence",
		NodeKind:     string(types.NodeEvidence),
		DispatchKind: string(types.NodeEvidence),
	}
	done := render.Event{
		Kind:         render.EventTaskNodeEnd,
		Timestamp:    time.Now(),
		NodeID:       "write-evidence",
		NodeKind:     string(types.NodeEvidence),
		DispatchKind: string(types.NodeEvidence),
	}
	o.emit(start)
	o.emit(done)
	o.emit(start)
	o.emit(done)
	if len(events) != 4 {
		t.Fatalf("write mode task-node lifecycle must not be suppressed by read-status debounce, got %d: %+v", len(events), events)
	}
}

func TestReadStatusSourceInventoryShapeUsesAuthoritySnapshot(t *testing.T) {
	mut := types.NewMutableState("list source inventory functions")
	mut.SetSourceInventoryObservation(types.SourceInventoryObservation{
		Active:   true,
		Complete: true,
		Scopes:   []string{"src"},
		Lens:     []string{"members"},
		Sets: []types.SourceInventoryObservationSet{{
			Role:     types.AnswerCandidateRoleFunction,
			Complete: true,
			Count:    1,
			Members: []types.SourceInventoryObservationMember{{
				Name:     "Run",
				Role:     types.AnswerCandidateRoleFunction,
				File:     "src/run.go",
				Line:     7,
				Language: "go",
			}},
		}},
	})
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			EvidencePlan: types.EvidencePlan{RequiredFiles: []string{"src/run.go", "src/missing.go"}},
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
				},
				CompletenessObligation: &types.CompletenessObligation{Required: true, SourceQuote: "all functions"},
				SourceInventoryProfile: &types.SourceInventoryProfile{
					IsSourceInventory: true,
					TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
					RequestedFields: []types.SourceInventoryRequestedField{
						types.SourceInventoryFieldName,
						types.SourceInventoryFieldLocation,
					},
					Confidence: 0.9,
				},
				SourceScopeProfile: &types.SourceScopeProfile{RequestedScope: types.SourceScopeProduction},
			},
		},
	}

	got := readStatusSourceInventoryShapeForContext(ctx, mut)
	for _, want := range []string{
		"authority=true",
		"need=",
		"landing=false",
		"required_files_uncovered",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("source-inventory status shape missing %q:\n%s", want, got)
		}
	}
}
