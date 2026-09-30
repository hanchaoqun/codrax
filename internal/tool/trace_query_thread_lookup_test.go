package tool

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestThreadLookupEmitToPublicProcessProfileWithoutCausalFocus(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	raw := "查看线程10所属进程在1.000到1.020秒的线程状态和业务热点，数据在events.systrace"
	payload := `{"intent":"explain","scenario":"generic","complexity":"moderate","keywords":["trace","process","threads"],"entities":["events.systrace"],"question_kind":"mechanism","runtime_target_profile":{"declaration":"no_named_target","confidence":0.95},"runtime_thread_lookups":[{"pid":10,"source_quote":"线程10所属进程"}]}`
	r, mu := runEmitAnalysisPayload(t, raw, withV4Required(payload))
	if !r.Success {
		t.Fatalf("emit: %s", r.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || len(rm.RuntimeThreadLookups) != 1 || len(rm.RuntimeTargets) != 0 {
		t.Fatalf("lookup promoted or lost: %+v", rm)
	}
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_process_profile/events.systrace")
	ctx := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Mutable: mu, AnalysisIR: &types.AnalysisIR{RequestModel: *rm}}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "process_profile", "time_start": 1, "time_end": 1.02})
	result, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil || !result.Success {
		t.Fatalf("profile: %v %+v", err, result)
	}
	found := false
	for _, record := range result.Observations {
		if p, ok := DecodeTraceProcessProfile(record); ok && p.TGID == 10 && p.ThreadCount == 3 && p.UnavailableThreads == 1 {
			found = true
		}
	}
	if !found {
		t.Fatal("lookup did not recover complete process membership")
	}
	if len(mu.RequestModel().RuntimeTargets) != 0 || len(ctx.AnalysisIR.RequestModel.RuntimeTargets) != 0 {
		t.Fatal("autofill elected diagnostic focus/cursor")
	}
}

func TestThreadLookupInheritanceBoundaries(t *testing.T) {
	rm := types.RequestModel{RawRequest: "线程10所属进程和线程11所属进程", RuntimeThreadLookups: []types.RuntimeThreadLookup{{PID: 10, SourceQuote: "线程10所属进程"}}}
	ctx := &types.BusContext{AnalysisIR: &types.AnalysisIR{RequestModel: rm}, Mutable: types.NewMutableState(rm.RawRequest)}
	for _, view := range []string{"process_profile", "thread_timeline", "window_stats", "scheduler_latency_stats"} {
		var input traceQueryParams
		if err := json.Unmarshal([]byte(`{"time_start":1,"time_end":1.02}`), &input); err != nil {
			t.Fatal(err)
		}
		input.View = view
		p, _ := traceQueryApplyRequestModelTarget(ctx, input)
		if p.PID.Int() != 10 || p.TimeStart != input.TimeStart || p.TimeEnd != input.TimeEnd {
			t.Fatalf("lost lookup/window %s: %+v", view, p)
		}
	}
	for _, view := range []string{"root_causes", "critical_path", "frame_jank", "event_search", "field_inventory"} {
		p, _ := traceQueryApplyRequestModelTarget(ctx, traceQueryParams{View: view})
		if p.PID.Int() != 0 {
			t.Fatalf("lookup became causal/global focus: %s", view)
		}
	}
	p, _ := traceQueryApplyRequestModelTarget(ctx, traceQueryParams{View: "process_profile", PID: 11})
	if p.PID.Int() != 11 {
		t.Fatal("explicit selector overwritten")
	}
	ctx.AnalysisIR.RequestModel.RuntimeThreadLookups = append(rm.RuntimeThreadLookups, types.RuntimeThreadLookup{PID: 11, SourceQuote: "线程11所属进程"})
	ctx.Mutable.SetRequestModel(types.RequestModel{RuntimeTargets: []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 12, Source: "exploration_cursor"}}})
	p, _ = traceQueryApplyRequestModelTarget(ctx, traceQueryParams{View: "process_profile"})
	if p.PID.Int() != 0 {
		t.Fatal("ambiguous lookup arbitrarily selected")
	}
	ctx.AnalysisIR.RequestModel = rm
	p, _ = traceQueryApplyRequestModelTarget(ctx, traceQueryParams{View: "process_profile"})
	if p.PID.Int() != 10 {
		t.Fatal("exploration cursor replaced current-request lookup")
	}
	ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 11, Source: "user_explicit"}}
	p, _ = traceQueryApplyRequestModelTarget(ctx, traceQueryParams{View: "process_profile"})
	if p.PID.Int() != 11 {
		t.Fatal("actual answer subject overwritten")
	}
}

func TestThreadLookupValidationAndCanonicalIdentity(t *testing.T) {
	raw := "看ui-10所属进程"
	for _, in := range [][]types.RuntimeThreadLookup{
		{{PID: -1, SourceQuote: raw}}, {{PID: types.RuntimeTargetMaxPID + 1, SourceQuote: raw}}, {{SourceQuote: raw}}, {{PID: 10, SourceQuote: "tool prose"}},
		make([]types.RuntimeThreadLookup, 9),
	} {
		if _, reason := parseRuntimeThreadLookups(raw, in); reason == "" {
			t.Fatalf("accepted malformed lookup %+v", in)
		}
	}
	ctx := &types.BusContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{RawRequest: raw, RuntimeThreadLookups: []types.RuntimeThreadLookup{{Thread: "ui-10", SourceQuote: raw}, {PID: 10, SourceQuote: raw}}}}}
	p, _ := traceQueryApplyRequestModelTarget(ctx, traceQueryParams{View: "process_profile"})
	if p.PID.Int() != 10 {
		t.Fatal("equivalent identities treated as ambiguous")
	}
	ctx.AnalysisIR.RequestModel.RuntimeThreadLookups[0].PID = 11
	p, _ = traceQueryApplyRequestModelTarget(ctx, traceQueryParams{View: "process_profile"})
	if p.PID.Int() != 0 {
		t.Fatal("conflicting identity inherited")
	}
	if !strings.Contains(string((&EmitAnalysis{}).Parameters()), types.RuntimeThreadLookupTeaching) {
		t.Fatal("schema teaching absent")
	}
}
