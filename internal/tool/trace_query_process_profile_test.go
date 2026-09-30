package tool

import (
	"encoding/json"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
	"path/filepath"
	"strings"
	"testing"
)

func TestProcessProfilePublicToolAndTypedHandoff(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_process_profile/events.systrace")
	ctx := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Mutable: types.NewMutableState("show process threads")}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "process_profile", "pid": 10, "time_start": 1, "time_end": 1.02, "limit": 2})
	r, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil || !r.Success {
		t.Fatalf("public query: %v %+v", err, r)
	}
	var record types.ObservationRecord
	for _, candidate := range r.Observations {
		if candidate.Predicate == TraceProcessProfilePredicate {
			record = candidate
		}
	}
	p, ok := DecodeTraceProcessProfile(record)
	if !ok || p.ThreadCount != 3 || p.OmittedThreads != 1 || p.UnavailableThreads != 1 || len(p.Threads) != 2 {
		t.Fatalf("handoff lost producer census: %+v %+v", record, p)
	}
	if !strings.Contains(r.Summary, "原生省略=1") {
		t.Fatal("preview lost capacity")
	}
	for _, mutate := range []func(*types.ObservationRecord){
		func(r *types.ObservationRecord) { r.SourceRef.QueryScopeID = "" },
		func(r *types.ObservationRecord) { r.SourceRef.QueryWindowStartTs = 0 },
		func(r *types.ObservationRecord) { r.Role = types.AnswerAggregateRolePrincipalAnswer },
		func(r *types.ObservationRecord) { r.SourceRef.Path += ".other" },
		func(r *types.ObservationRecord) { r.Object = "causal" },
	} {
		copy := record
		mutate(&copy)
		if _, ok := DecodeTraceProcessProfile(copy); ok {
			t.Fatalf("accepted mismatched proof: %+v", copy)
		}
	}
	copy := p
	copy.Threads = append([]tracequery.ProcessProfileThread(nil), p.Threads...)
	copy.Threads[0].RunningWindowPct = new(float64)
	data, _ := json.Marshal(copy)
	corrupt := record
	corrupt.RichNotes = []string{types.TraceNoteKeyProcessProfile + "=" + string(data)}
	if _, ok := DecodeTraceProcessProfile(corrupt); ok {
		t.Fatal("accepted incompatible percentage")
	}
	if !strings.Contains(string((&TraceQuery{}).Parameters()), "process_profile.threads") {
		t.Fatal("missing field teaching")
	}
}

func TestProcessProfileMissingSelectorIsRepairNotCaptureAbsence(t *testing.T) {
	r, err := (&TraceQuery{}).Execute(nil, json.RawMessage(`{"view":"process_profile","time_start":1,"time_end":1.02}`))
	if err != nil || r.Success || r.Repair == nil || r.Repair.Code != "trace_query_source_thread_required" || len(r.Observations) > 0 || r.RawRef != "" {
		t.Fatalf("missing selector became data: %v %+v", err, r)
	}
	for _, p := range []traceQueryParams{{View: "process_profile", PID: 10}, {View: "process_profile", Thread: "ui"}, {View: "window_stats"}} {
		if traceQueryProcessProfileInputRepair(p) != nil {
			t.Fatal("explicit selector or unrelated view rejected")
		}
	}
	text := TraceProcessProfileText(tracequery.ProcessProfile{Status: "unavailable", Reason: "native_identity_missing"}, 12)
	if strings.Contains(text, "成员=0") || !strings.Contains(text, "不能按零处理") {
		t.Fatal("unavailable rendered as zero census")
	}
}

func TestProcessProfileKeepsTypedTargetAutocomplete(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_process_profile/events.systrace")
	ctx := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Mutable: types.NewMutableState("process overview"), AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{RuntimeTargets: []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 10, Source: "user_explicit", Confidence: 0.95}}}}}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "process_profile", "time_start": 1, "time_end": 1.02})
	r, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil || !r.Success {
		t.Fatalf("typed target no longer auto-completed: %v %+v", err, r)
	}
	for _, record := range r.Observations {
		if p, ok := DecodeTraceProcessProfile(record); ok && p.TGID == 10 && p.ThreadCount == 3 {
			return
		}
	}
	t.Fatal("inherited target lost typed process account")
}
