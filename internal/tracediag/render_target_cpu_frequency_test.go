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
)

// Result's schema pin only sees the enclosing account pointer. Explicitly
// adjudicate this new nested optional carrier without re-signing that hash.
func TestB1633DiagTargetFrequencyFieldDisposition(t *testing.T) {
	for _, tc := range []struct {
		typ  reflect.Type
		want []string
	}{
		{reflect.TypeOf(tracequery.TargetWindowCPURunning{}), []string{
			"CPU|int|cpu", "RunningMs|float64|running_ms", "SegmentCount|int|segment_count,omitempty",
			"StartTs|float64|start_ts,omitempty", "EndTs|float64|end_ts,omitempty",
			"LineStart|int|line_start,omitempty", "LineEnd|int|line_end,omitempty",
			"RepresentativeFrequency|*tracequery.TargetWindowCPURepresentativeFrequency|representative_frequency,omitempty",
		}},
		{reflect.TypeOf(tracequery.TargetWindowCPURepresentativeFrequency{}), []string{
			"FrequencyKHz|int64|frequency_khz", "Caliber|string|caliber",
			"ClusterDonorCPU|*int|cluster_donor_cpu,omitempty", "ClusterDonorSource|string|cluster_donor_source,omitempty",
		}},
	} {
		var got []string
		for i := 0; i < tc.typ.NumField(); i++ {
			field := tc.typ.Field(i)
			if field.PkgPath == "" {
				got = append(got, field.Name+"|"+field.Type.String()+"|"+field.Tag.Get("json"))
			}
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%s needs explicit scalar/source/nested disposition: got=%q want=%q", tc.typ, got, tc.want)
		}
	}
}

func TestB1633DiagActualRunPreservesOptionalTargetFrequency(t *testing.T) {
	const running = `<idle>-0 (-----) [001] .... 1.000000: sched_switch: prev_comm=swapper/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=101 next_prio=120
target-101 (101) [001] .... 1.010000: sched_switch: prev_comm=target prev_pid=101 prev_prio=120 prev_state=S ==> next_comm=swapper/1 next_pid=0 next_prio=120
`
	for _, tc := range []struct {
		name, samples, wantFrequency string
		wantDonor                    bool
	}{
		{"donor_cpu_zero", "<idle>-0 (-----) [000] .... 0.990000: cpu_frequency: state=1000000 cpu_id=0\n", "1000000", true},
		{"own_sample", "<idle>-0 (-----) [001] .... 0.990000: cpu_frequency: state=1200000 cpu_id=1\n", "1200000", false},
		{"unknown", "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "target_frequency.ftrace")
			if err := os.WriteFile(path, []byte(tc.samples+running), 0600); err != nil {
				t.Fatal(err)
			}
			idx, err := tracequery.BuildIndex(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			for _, view := range []string{"window_stats", "frame_root_cause_bundle"} {
				t.Run(view, func(t *testing.T) {
					result := tracequery.Run(idx, tracequery.Query{View: view, PID: 101, TimeStart: 1, TimeEnd: 1.01, CoreTopology: "small=0-1"})
					account := result.TargetWindowStates
					prefix := "target_window_states.running_by_cpu[0]"
					if view == "frame_root_cause_bundle" {
						if result.FrameRootCauseBundle == nil {
							t.Fatal("actual bundle missing")
						}
						account = result.FrameRootCauseBundle.TargetWindowStates
						prefix = "frame_root_cause_bundle." + prefix
					}
					if account == nil || len(account.RunningByCPU) != 1 {
						t.Fatalf("actual target CPU account missing: %+v", account)
					}
					frequency := account.RunningByCPU[0].RepresentativeFrequency
					if (frequency == nil) != (tc.wantFrequency == "") {
						t.Fatalf("fixture did not reach expected known/unknown producer: %+v", frequency)
					}
					if tc.wantDonor && (frequency.ClusterDonorCPU == nil || *frequency.ClusterDonorCPU != 0) {
						t.Fatalf("fixture must have a real donor CPU0, not nil/unknown: %+v", frequency)
					}
					before, _ := json.Marshal(result)
					step := &Step{View: view, effMaxLines: 10000}
					body := renderStepBody(step, stepOutcome{result: &result})
					report := strings.Join(body.lines, "\n")
					if !strings.Contains(report, prefix+": cpu=1 running_ms=10.000 segment_count=1 start_ts=1.000000 end_ts=1.010000") {
						t.Fatalf("original target time/CPU/coordinate detail changed:\n%s", report)
					}
					if tc.wantFrequency == "" {
						if strings.Contains(report, prefix+".representative_frequency") {
							t.Fatalf("nil frequency must not become zero or a sampled row:\n%s", report)
						}
					} else {
						want := prefix + ".representative_frequency: frequency_khz=" + tc.wantFrequency + " caliber=" + tracequery.TargetWindowCPURepresentativeFrequencyCaliber
						if strings.Count(report, want) != 1 {
							t.Fatalf("nested sample/caliber must render once and stay attached to its exact target row:\n%s", report)
						}
						if tc.wantDonor {
							if !strings.Contains(report, "cluster_donor_source=explicit_topology") || !strings.Contains(report, prefix+".representative_frequency.cluster_donor_cpu: 0") {
								t.Fatalf("generic pointer walk must preserve known donor CPU0 and its source:\n%s", report)
							}
						} else if strings.Contains(report, prefix+".representative_frequency.cluster_donor_cpu") {
							t.Fatalf("own frequency must not manufacture a donor:\n%s", report)
						}
					}
					again := renderStepBody(step, stepOutcome{result: &result})
					after, _ := json.Marshal(result)
					if !bytes.Equal(before, after) || !reflect.DeepEqual(body, again) {
						t.Fatal("diagnostic rendering mutated engine data or lost idempotence")
					}
					// Removing only the optional field must remove only its nested
					// detail lines, not a primary duration/coordinate/key-first row.
					account.RunningByCPU[0].RepresentativeFrequency = nil
					baseline := renderStepBody(step, stepOutcome{result: &result})
					account.RunningByCPU[0].RepresentativeFrequency = frequency
					var withoutOptional []string
					for _, line := range body.lines {
						if !strings.Contains(line, prefix+".representative_frequency") {
							withoutOptional = append(withoutOptional, line)
						}
					}
					if !reflect.DeepEqual(withoutOptional, baseline.lines) {
						t.Fatalf("optional detail altered unrelated diagnostic lines:\nnew=%v\nbase=%v", withoutOptional, baseline.lines)
					}
				})
			}
		})
	}
}
