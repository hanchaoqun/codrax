package tracequery

import (
	"strings"
	"testing"
)

func TestSpanWindowProcessScopeRequiresProvenMembership(t *testing.T) {
	idx := &Index{
		Events: []Event{
			{Line: 1, Ts: 1.000, Type: EventTraceMark, Comm: "ui-a", PID: 11, TGID: 100, SpanPID: 100, SpanAction: "B", SpanName: "Choreographer#doFrame 1"},
			{Line: 2, Ts: 1.010, Type: EventTraceMark, Comm: "ui-a", PID: 11, TGID: 100, SpanPID: 100, SpanAction: "E"},
			{Line: 3, Ts: 1.020, Type: EventTraceMark, Comm: "render-b", PID: 12, TGID: 100, SpanAction: "B", SpanName: "RenderFrame 1"},
			{Line: 4, Ts: 1.030, Type: EventTraceMark, Comm: "render-b", PID: 12, TGID: 100, SpanAction: "E"},
			// Same frame-like label without TGID/SpanPID proof must not enter
			// process scope merely because it is nearby or similarly named.
			{Line: 5, Ts: 1.040, Type: EventTraceMark, Comm: "ui-unknown", PID: 13, SpanAction: "B", SpanName: "Choreographer#doFrame 2"},
			{Line: 6, Ts: 1.050, Type: EventTraceMark, Comm: "ui-unknown", PID: 13, SpanAction: "E"},
		},
		FirstTs: 1.000, LastTs: 1.050,
	}
	threadScoped, _ := FindSpanWindows(idx, Query{
		PID: 100, TargetScope: TargetScopeThread,
		TimeStart: 0.990, TimeEnd: 1.060,
	}, 8)
	if len(threadScoped) != 0 {
		t.Fatalf("default thread scope must keep pid=100 as exact TID: %+v", threadScoped)
	}
	processScoped, caveats := FindSpanWindows(idx, Query{
		PID: 100, TargetScope: TargetScopeProcess,
		TimeStart: 0.990, TimeEnd: 1.060,
	}, 8)
	if len(processScoped) != 2 || processScoped[0].Thread.PID != 11 || processScoped[1].Thread.PID != 12 {
		t.Fatalf("process scope must admit exactly proven members: %+v caveats=%v", processScoped, caveats)
	}
	for _, span := range processScoped {
		if span.TargetScope != TargetScopeProcess || span.ProcessMembershipSource == "" {
			t.Fatalf("process membership authority missing: %+v", span)
		}
	}
	if !containsSubstring(caveats, "membership_authority=thread_tgid_or_trace_mark_span_pid") ||
		!containsSubstring(caveats, "matched_member_tids=11,12") {
		t.Fatalf("process-scope coverage caveat missing: %v", caveats)
	}
}

func TestProcessScopeMemberIncarnationConflictFailsClosed(t *testing.T) {
	idx := &Index{
		Events: []Event{
			{Line: 1, Ts: 1.000, Type: EventTraceMark, Comm: "ui", PID: 11, TGID: 100, SpanAction: "B", SpanName: "Choreographer#doFrame"},
			{Line: 2, Ts: 1.010, Type: EventTraceMark, Comm: "ui", PID: 11, TGID: 100, SpanAction: "E"},
		},
		FirstTs: 1.000, LastTs: 1.010,
		threadIncarnationFailures: []threadIncarnationConflict{{
			PID: 11, PreviousLine: 1, PreviousTs: 1.000,
			BoundaryLine: 2, BoundaryTs: 1.005, Signal: "sched_wakeup_new",
		}},
	}
	spans, caveats := FindSpanWindows(idx, Query{
		PID: 100, TargetScope: TargetScopeProcess,
		TimeStart: 0.990, TimeEnd: 1.020,
	}, 8)
	if len(spans) != 0 || !containsSubstring(caveats, "process_scope_membership_fail_closed=true") {
		t.Fatalf("member generation conflict must withdraw process-scoped spans: spans=%+v caveats=%v", spans, caveats)
	}
}

func TestResolveFrameTargetProcessScopeLocksOnlyProvenFrameMember(t *testing.T) {
	q := Query{
		PID: 100, TargetScope: TargetScopeProcess,
		TimeStart: 1.000, TimeEnd: 1.020, TimeStartSet: true, TimeEndSet: true,
	}
	frame := FrameTimelineResult{Items: []FrameTimelineItem{{
		Index: 1, Thread: ThreadRef{Comm: "ui", PID: 11, TGID: 100},
		TargetScope: TargetScopeProcess, ProcessID: 100, ProcessMembershipSource: "thread_tgid",
		Role: "ui", Phase: "ui_traversal", Name: "Choreographer#doFrame",
		RoleAuthority: &FrameRoleAuthority{Role: "ui", Kind: "pipeline_stage_role", Source: "trace_span_name_semantics", Confidence: 0.9},
		StartTs:       1.001, EndTs: 1.010, StartLine: 10, EndLine: 11,
	}}}
	resolution := ResolveFrameTarget(nil, q, frame)
	if resolution.Target.PID != 11 ||
		resolution.TargetScope != TargetScopeProcess ||
		resolution.ProcessID != 100 ||
		resolution.Source != "process_scope_frame_thread_unique" ||
		resolution.SelectedFrame == nil ||
		resolution.SelectedFrame.MembershipAuthority != "thread_tgid" {
		t.Fatalf("process-scope frame target resolution drifted: %+v", resolution)
	}
	analysisQ := applyFrameTargetResolution(q, resolution)
	if analysisQ.PID != 11 || analysisQ.TargetScope != TargetScopeThread {
		t.Fatalf("downstream scheduler analysis must return to exact thread scope: %+v", analysisQ)
	}

	unproven := frame
	unproven.Items[0].ProcessMembershipSource = ""
	unprovenResolution := ResolveFrameTarget(nil, q, unproven)
	if unprovenResolution.Target.PID != 0 || !strings.Contains(unprovenResolution.Source, "no_ui_candidate") {
		t.Fatalf("unproven process membership must not lock a frame thread: %+v", unprovenResolution)
	}
}

func TestResolveFrameTargetProcessScopeFailsClosedForMultipleProvenUICandidates(t *testing.T) {
	q := Query{
		PID: 100, TargetScope: TargetScopeProcess,
		TimeStart: 1.000, TimeEnd: 1.040, TimeStartSet: true, TimeEndSet: true,
	}
	frame := FrameTimelineResult{Items: []FrameTimelineItem{
		{
			Index: 1, Thread: ThreadRef{Comm: "ui-a", PID: 11, TGID: 100},
			TargetScope: TargetScopeProcess, ProcessID: 100, ProcessMembershipSource: "thread_tgid",
			Role: "ui", Phase: "ui_traversal", Name: "Choreographer#doFrame A",
			RoleAuthority: &FrameRoleAuthority{Role: "ui", Kind: "pipeline_stage_role", Source: "trace_span_name_semantics", Confidence: 0.9},
			StartTs:       1.001, EndTs: 1.010, StartLine: 10, EndLine: 11,
		},
		{
			Index: 2, Thread: ThreadRef{Comm: "ui-b", PID: 12, TGID: 100},
			TargetScope: TargetScopeProcess, ProcessID: 100, ProcessMembershipSource: "thread_tgid",
			Role: "ui", Phase: "ui_traversal", Name: "Choreographer#doFrame B",
			RoleAuthority: &FrameRoleAuthority{Role: "ui", Kind: "pipeline_stage_role", Source: "trace_span_name_semantics", Confidence: 0.9},
			StartTs:       1.020, EndTs: 1.030, StartLine: 20, EndLine: 21,
		},
	}}
	resolution := ResolveFrameTarget(nil, q, frame)
	if resolution.Target.PID != 0 ||
		resolution.Source != "process_scope_ambiguous_frame_candidate" ||
		resolution.SelectedFrame != nil ||
		len(resolution.Candidates) != 2 ||
		!containsSubstring(resolution.Caveats, "found 2 UI/main-like member threads") {
		t.Fatalf("multiple process UI candidates must fail closed without auto-locking: %+v", resolution)
	}
	analysisQ := applyFrameTargetResolution(q, resolution)
	if analysisQ.PID != 100 || analysisQ.TargetScope != TargetScopeProcess {
		t.Fatalf("ambiguous process scope must preserve the original process selector: %+v", analysisQ)
	}
}
