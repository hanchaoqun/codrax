package tracediag

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestBusinessSpanSchedulerDetailFieldDisposition(t *testing.T) {
	typ := reflect.TypeOf(tracequery.TraceSpanSchedulerStates{})
	var got []string
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		got = append(got, f.Name+"|"+f.Type.String()+"|"+f.Tag.Get("json"))
	}
	want := []string{
		"SourcePath|string|source_path", "Thread|tracequery.ThreadRef|thread", "Window|tracequery.TimeWindow|window",
		"Coverage|string|coverage", "MeasurementDomain|*types.TraceSchedulerMeasurementDomain|measurement_domain,omitempty",
		"RunningMs|float64|running_ms", "RunnableMs|float64|runnable_ms", "SleepMs|float64|sleep_ms",
		"DStateMs|float64|d_state_ms", "IOWaitMs|float64|io_wait_ms", "SleepIOWaitMs|float64|sleep_io_wait_ms",
		"AccountedMs|float64|accounted_ms", "HeadState|*tracequery.TimelineHeadState|head_state,omitempty",
		"IntegrityFailure|string|integrity_failure,omitempty", "Caveats|[]string|caveats,omitempty",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("marker-local fields need explicit scalar/coordinate/quality disposition: %q", got)
	}
	owner := reflect.TypeOf(tracequery.TraceSpanSummary{})
	f, ok := owner.FieldByName("SchedulerStates")
	if !ok || f.Type != reflect.PointerTo(typ) || f.Tag.Get("json") != "scheduler_states,omitempty" || policySkipsDetailField(&nonEventDetailPolicy, owner, f.Name) {
		t.Fatalf("marker-local account must remain one optional typed detail child: %+v", f)
	}
}

func TestBusinessSpanSchedulerDetailKeepsZeroUnknownAndSource(t *testing.T) {
	for _, coverage := range []string{"complete", "partial", "unavailable", "future"} {
		states := &tracequery.TraceSpanSchedulerStates{SourcePath: "/collection/private/capture.systrace",
			Thread:   tracequery.ThreadRef{PID: 55, Comm: "worker"},
			Window:   tracequery.TimeWindow{StartTs: 6793224, EndTs: 6793224.04},
			Coverage: coverage, RunningMs: 40, AccountedMs: 40, Caveats: []string{"marker-scope-caveat"},
		}
		res := tracequery.Result{WindowStats: &tracequery.WindowStats{TraceSpans: []tracequery.TraceSpanSummary{{Name: "business", SchedulerStates: states}}}}
		before, _ := json.Marshal(res)
		for _, policy := range []*detailRenderPolicy{nil, &nonEventDetailPolicy} {
			var lines []string
			renderResultDetailWithPolicy(&res, func(s string) { lines = append(lines, s) }, policy)
			out := strings.Join(lines, "\n")
			for _, want := range []string{"scheduler_states:", "capture.systrace", "6793224.000000", "6793224.040000", "marker-scope-caveat"} {
				if !strings.Contains(out, want) {
					t.Errorf("missing %q: %s", want, out)
				}
			}
			if strings.Contains(out, "/collection/private") || strings.Contains(out, "e+") {
				t.Fatal(out)
			}
			measured := coverage == "complete" || coverage == "partial"
			if strings.Contains(out, "等待调度=0.000 ms") != measured || strings.Contains(out, "运行=40.000 ms") != measured {
				t.Fatalf("zero/unknown conflation (%s): %s", coverage, out)
			}
			if !measured && !strings.Contains(out, "不能按零处理") {
				t.Fatal(out)
			}
		}
		after, _ := json.Marshal(res)
		if string(before) != string(after) {
			t.Fatal("render mutated the source account")
		}
	}
}
