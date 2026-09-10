package tracediag

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The optional receipt is a compact scalar descriptor, not an event roster or
// a completeness verdict. Typed detail renders it before bulk intervals.
// Nested account changes do not change Result's pointer-only schema hash.
func TestB1638B1DiagMeasurementFieldDisposition(t *testing.T) {
	typ := reflect.TypeOf(types.TraceSchedulerMeasurementDomain{})
	var got []string
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		got = append(got, f.Name+"|"+f.Type.String()+"|"+f.Tag.Get("json"))
	}
	want := []string{
		"Version|int|version", "Status|string|status", "Method|string|method", "TargetTID|int|target_tid",
		"WindowStartTs|float64|window_start_ts", "WindowEndTs|float64|window_end_ts",
		"QueryLineStart|int|query_line_start", "QueryLineEnd|int|query_line_end", "PartitionID|string|partition_id",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("receipt fields need explicit scalar/coordinate disposition: got=%q want=%q", got, want)
	}
	// B2 adds two independently owned native-stream carriers. Their optional
	// descriptors use the same explicit nine-field renderer, not a bulk skip.
	for _, owner := range []reflect.Type{reflect.TypeOf(tracequery.TimelineResult{}), reflect.TypeOf(tracequery.TargetWindowStateAccount{}),
		reflect.TypeOf(tracequery.ThreadDuration{}), reflect.TypeOf(tracequery.ThreadStateChurnSummary{})} {
		field, ok := owner.FieldByName("MeasurementDomain")
		if !ok || field.Type != reflect.PointerTo(typ) || field.Tag.Get("json") != "measurement_domain,omitempty" {
			t.Fatalf("%s must retain one optional, typed receipt: %+v", owner, field)
		}
		if policySkipsDetailField(&nonEventDetailPolicy, owner, "MeasurementDomain") {
			t.Fatalf("%s receipt was hidden from its generic detail owner", owner)
		}
	}
}

func TestB1638B1DiagActualMeasurementDomain(t *testing.T) {
	path := filepath.Join(t.TempDir(), "partition.ftrace")
	const body = `<idle>-0 (-----) [000] .... 6793224.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=55 next_prio=120
worker-55 (55) [000] .... 6793224.010000: sched_switch: prev_comm=worker prev_pid=55 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
<idle>-0 (-----) [000] .... 6793224.020000: sched_wakeup: comm=worker pid=55 prio=120 target_cpu=0
<idle>-0 (-----) [000] .... 6793224.030000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=55 next_prio=120
worker-55 (55) [000] .... 6793224.040000: sched_switch: prev_comm=worker prev_pid=55 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
`
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	for _, view := range []string{"thread_timeline", "window_stats", "frame_root_cause_bundle"} {
		t.Run(view, func(t *testing.T) {
			result := tracequery.Run(idx, tracequery.Query{View: view, PID: 55, TimeStart: 6793224, TimeEnd: 6793224.04})
			account := result.TargetWindowStates
			prefix := "target_window_states.measurement_domain:"
			if result.FrameRootCauseBundle != nil {
				account = result.FrameRootCauseBundle.TargetWindowStates
				prefix = "frame_root_cause_bundle." + prefix
			}
			if account == nil || account.MeasurementDomain == nil {
				t.Fatal("actual engine must publish the target's constructed partition")
			}
			before, _ := json.Marshal(result)
			lines := renderStepBody(&Step{View: view, effMaxLines: 10000}, stepOutcome{result: &result}).lines
			report := strings.Join(lines, "\n")
			found := 0
			for _, line := range lines {
				if !strings.Contains(line, prefix) {
					continue
				}
				found++
				for _, want := range []string{"version=1", "status=constructed_partition", "method=thread_timeline", "target_tid=55",
					"query_line_start=0", "query_line_end=0",
					"window_start_ts=6793224.000000", "window_end_ts=6793224.040000", "partition_id=" + account.MeasurementDomain.PartitionID} {
					if !strings.Contains(line, want) {
						t.Errorf("receipt must keep exact identity/caliber and fixed-point coordinates; missing %q: %s", want, line)
					}
				}
				if strings.Contains(line, "e+") || strings.Contains(line, "status=complete") {
					t.Errorf("metadata became scientific notation or a capture completeness claim: %s", line)
				}
			}
			if found != 1 {
				t.Fatalf("expected exactly one account receipt, found %d:\n%s", found, report)
			}
			if result.Timeline != nil {
				at := strings.Index(report, "timeline.measurement_domain:")
				firstInterval := strings.Index(report, "timeline.intervals[")
				if at < 0 || firstInterval < at {
					t.Fatal("compact receipt must precede bulk timeline intervals")
				}
			}
			after, _ := json.Marshal(result)
			if !bytes.Equal(before, after) {
				t.Fatal("diagnostic rendering mutated the measured result")
			}
			account.MeasurementDomain = nil
			if result.Timeline != nil {
				result.Timeline.MeasurementDomain = nil
			}
			legacy := strings.Join(renderStepBody(&Step{View: view, effMaxLines: 10000}, stepOutcome{result: &result}).lines, "\n")
			// Only the B1 owners were removed. Independent B2 native-stream
			// receipts must remain visible rather than being suppressed too.
			if strings.Contains(legacy, prefix) || strings.Contains(legacy, "timeline.measurement_domain:") {
				t.Fatal("an absent legacy account/timeline receipt must not be manufactured")
			}
		})
	}
}

func TestB1638B1DiagActualZeroOriginRemainsExplicit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "zero-origin.ftrace")
	const body = `<idle>-0 (-----) [000] .... 0.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=55 next_prio=120
worker-55 (55) [000] .... 0.010000: sched_switch: prev_comm=worker prev_pid=55 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
<idle>-0 (-----) [000] .... 0.020000: sched_wakeup: comm=worker pid=55 prio=120 target_cpu=0
<idle>-0 (-----) [000] .... 0.030000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=55 next_prio=120
`
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	r := tracequery.Run(idx, tracequery.Query{View: "thread_timeline", PID: 55,
		TimeStart: 0, TimeEnd: .03, TimeStartSet: true, TimeEndSet: true})
	if r.Timeline == nil || r.Timeline.MeasurementDomain == nil || r.TargetWindowStates == nil ||
		r.Timeline.MeasurementDomain.WindowStartTs != 0 {
		t.Fatal("actual engine fixture must retain a legitimate, explicitly selected zero origin")
	}
	before, _ := json.Marshal(r)
	for _, policy := range []*detailRenderPolicy{nil, &nonEventDetailPolicy} {
		var lines []string
		renderResultDetailWithPolicy(&r, func(line string) { lines = append(lines, line) }, policy)
		for _, prefix := range []string{"timeline.measurement_domain:", "target_window_states.measurement_domain:"} {
			found := 0
			for _, line := range lines {
				if !strings.Contains(line, prefix) {
					continue
				}
				found++
				for _, want := range []string{"window_start_ts=0.000000", "window_end_ts=0.030000", "query_line_start=0", "query_line_end=0"} {
					if !strings.Contains(line, want) {
						t.Errorf("required coordinate/filter sentinel became absent, missing %s: %s", want, line)
					}
				}
			}
			if found != 1 {
				t.Fatalf("expected one %s receipt; got %d", prefix, found)
			}
		}
	}
	after, _ := json.Marshal(r)
	if !bytes.Equal(before, after) {
		t.Fatal("zero-coordinate rendering changed the native account")
	}
}
