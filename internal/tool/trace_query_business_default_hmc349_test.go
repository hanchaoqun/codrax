package tool

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC349BusinessDefaultUsesOnlyExactNumericQueryScope(t *testing.T) {
	span := tracequery.TraceSpanSummary{Thread: tracequery.ThreadRef{PID: 7, TGID: 100, Comm: "worker"}, SpanPID: 200, Name: "any business"}
	for _, tc := range []struct {
		name string
		ref  types.ObservationSourceRef
		want bool
	}{
		{"legacy_broad", types.ObservationSourceRef{}, true},
		{"named_only_conservative", types.ObservationSourceRef{QueryTargetThread: "work", QueryTargetScope: "thread"}, true},
		{"exact_tid", types.ObservationSourceRef{QueryTargetPID: 7, QueryTargetScope: "thread"}, true},
		{"legacy_tid", types.ObservationSourceRef{QueryTargetPID: 7}, true},
		{"other_tid", types.ObservationSourceRef{QueryTargetPID: 100, QueryTargetScope: "thread"}, false},
		{"marker_pid_is_not_tid", types.ObservationSourceRef{QueryTargetPID: 200, QueryTargetScope: "thread"}, false},
		{"process_tgid", types.ObservationSourceRef{QueryTargetPID: 100, QueryTargetScope: "process"}, true},
		{"process_marker_pid", types.ObservationSourceRef{QueryTargetPID: 200, QueryTargetScope: "process"}, true},
		{"tid_is_not_process", types.ObservationSourceRef{QueryTargetPID: 7, QueryTargetScope: "process"}, false},
		{"unknown_scope_conservative", types.ObservationSourceRef{QueryTargetPID: 99, QueryTargetScope: "future"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if traceQueryBusinessDefaultPresentation(tc.ref, span) != tc.want {
				t.Fatal("display intent borrowed another identity axis or guessed a target")
			}
		})
	}
}

func TestHMC349BusinessPublicQueriesKeepAllNativeSpans(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		pid      int
		scope    string
		defaults int
	}{
		{"broad", 0, "", 2}, {"main_tid", 100, "thread", 1}, {"worker_tid", 200, "thread", 1}, {"unsupported_process_statistics_unchanged", 100, "process", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Mutable: types.NewMutableState("query scope")}
			r := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "window_stats", "pid": tc.pid, "target_scope": tc.scope, "time_start": .999, "time_end": 1.051})
			if tc.scope == tracequery.TargetScopeProcess {
				if r.Success || !strings.Contains(r.Summary, "rejected target_scope=process") {
					t.Fatal("display membership expanded the public view/target-scope contract")
				}
				return
			}
			native := businessSpanSchedulerPublicPayload(t, r)
			before, _ := json.Marshal(native)
			found, defaults := 0, 0
			for _, record := range r.Observations {
				if record.Predicate != types.TraceBusinessSpanPredicate {
					continue
				}
				p, ok := types.DecodeRuntimeMeasurementPublication(record)
				if !ok || len(p.Tables) != 1 {
					t.Fatal("current native business facts lost")
				}
				found++
				if p.Tables[0].DefaultPresentation {
					defaults++
				}
				if p.Tables[0].MemberSet != nil || p.Tables[0].DisplayPopulation != nil {
					t.Fatal("display intent gained population authority")
				}
			}
			if found != 2 || defaults != tc.defaults || len(native.WindowStats.TraceSpans) != 2 {
				t.Fatalf("facts=%d defaults=%d native spans=%d", found, defaults, len(native.WindowStats.TraceSpans))
			}
			after, _ := json.Marshal(native)
			if !bytes.Equal(before, after) {
				t.Fatal("display changed original query payload")
			}
		})
	}
}
