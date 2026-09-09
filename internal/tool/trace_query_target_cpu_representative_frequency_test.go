package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1633ActualExecutePublishesTargetRepresentativeFrequency(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "representative.ftrace")
	body := `<idle>-0 (-----) [000] .... 0.900000: cpu_frequency: state=1280000 cpu_id=0
<idle>-0 (-----) [000] .... 1.000000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=101 next_prio=120
target-101 (101) [000] .... 1.050000: sched_switch: prev_comm=target prev_pid=101 prev_prio=120 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120
<idle>-0 (-----) [000] .... 1.100000: cpu_frequency: state=2000000 cpu_id=0
<idle>-0 (-----) [000] .... 1.200000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=101 next_prio=120
target-101 (101) [000] .... 1.250000: sched_switch: prev_comm=target prev_pid=101 prev_prio=120 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120
<idle>-0 (-----) [000] .... 1.300000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=other next_pid=202 next_prio=120
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "pid": 101, "time_start": 1, "time_end": 1.3})
	result, err := (&TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
	if err != nil || !result.Success {
		t.Fatalf("actual Execute failed: %v %+v", err, result)
	}
	var target, account *types.ObservationRecord
	for i := range result.Observations {
		r := &result.Observations[i]
		if r.Predicate == "target_cpu_running" {
			target = r
		}
		if r.Predicate == "target_window_states" {
			account = r
		}
	}
	if target == nil || account == nil || target.Value != "100.000" || target.Object != "cpu=0" || target.Subject != "target-101" {
		t.Fatalf("original target running account must be retained: target=%+v account=%+v", target, account)
	}
	if target.SourceRef.QueryScopeID == "" || target.SourceRef.PayloadRef == "" || target.ObservedAt == "" || !reflect.DeepEqual(target.SourceRef, account.SourceRef) || target.ObservedAt != account.ObservedAt {
		t.Fatalf("new field must keep the actual account source: %+v / %+v", target, account)
	}
	for _, want := range []string{"target_cpu_running_representative_frequency_khz=2000000", "target_cpu_running_representative_frequency_caliber=last_positive_running_segment_start"} {
		if !slices.Contains(target.RichNotes, want) {
			t.Errorf("actual target row dropped %q: %v", want, target.RichNotes)
		}
	}
	for _, note := range target.RichNotes {
		if strings.HasPrefix(note, "freq=") || strings.HasPrefix(note, "cpu=") {
			t.Errorf("dedicated target fact leaked into generic frequency channel: %q", note)
		}
	}
	for _, want := range []string{"representative_frequency=2000000kHz", "not constant frequency or residency"} {
		if !strings.Contains(result.Summary, want) {
			t.Errorf("actual target roster missing %q", want)
		}
	}
}

func TestB1633TargetFrequencyPublicationKeepsUnknownAndExactIdentity(t *testing.T) {
	donor0, donor2, negative := 0, 2, -1
	valid := func() *tracequery.TargetWindowCPURepresentativeFrequency {
		return &tracequery.TargetWindowCPURepresentativeFrequency{FrequencyKHz: 1280000, Caliber: tracequery.TargetWindowCPURepresentativeFrequencyCaliber}
	}
	cases := []struct {
		name      string
		frequency *tracequery.TargetWindowCPURepresentativeFrequency
		published bool
		donor     string
	}{
		{"nil", nil, false, ""}, {"own", valid(), true, ""},
		{"zero", &tracequery.TargetWindowCPURepresentativeFrequency{Caliber: tracequery.TargetWindowCPURepresentativeFrequencyCaliber}, false, ""},
		{"negative", &tracequery.TargetWindowCPURepresentativeFrequency{FrequencyKHz: -1, Caliber: tracequery.TargetWindowCPURepresentativeFrequencyCaliber}, false, ""},
		{"unknown_caliber", &tracequery.TargetWindowCPURepresentativeFrequency{FrequencyKHz: 1280000, Caliber: "other"}, false, ""},
		{"source_without_donor", &tracequery.TargetWindowCPURepresentativeFrequency{FrequencyKHz: 1280000, Caliber: tracequery.TargetWindowCPURepresentativeFrequencyCaliber, ClusterDonorSource: tracequery.ClusterFreqSourceExplicit}, false, ""},
		{"donor_without_source", &tracequery.TargetWindowCPURepresentativeFrequency{FrequencyKHz: 1280000, Caliber: tracequery.TargetWindowCPURepresentativeFrequencyCaliber, ClusterDonorCPU: &donor0}, false, ""},
		{"negative_donor", &tracequery.TargetWindowCPURepresentativeFrequency{FrequencyKHz: 1280000, Caliber: tracequery.TargetWindowCPURepresentativeFrequencyCaliber, ClusterDonorCPU: &negative, ClusterDonorSource: tracequery.ClusterFreqSourceExplicit}, false, ""},
		{"self_donor", &tracequery.TargetWindowCPURepresentativeFrequency{FrequencyKHz: 1280000, Caliber: tracequery.TargetWindowCPURepresentativeFrequencyCaliber, ClusterDonorCPU: &donor2, ClusterDonorSource: tracequery.ClusterFreqSourceExplicit}, false, ""},
		{"unknown_donor_source", &tracequery.TargetWindowCPURepresentativeFrequency{FrequencyKHz: 1280000, Caliber: tracequery.TargetWindowCPURepresentativeFrequencyCaliber, ClusterDonorCPU: &donor0, ClusterDonorSource: "unproven"}, false, ""},
		{"unsupported_keyed_rail", &tracequery.TargetWindowCPURepresentativeFrequency{FrequencyKHz: 1280000, Caliber: tracequery.TargetWindowCPURepresentativeFrequencyCaliber, ClusterDonorCPU: &donor0, ClusterDonorSource: tracequery.ClusterFreqSourceKeyedRail}, false, ""},
		{"explicit_donor_cpu_zero", &tracequery.TargetWindowCPURepresentativeFrequency{FrequencyKHz: 1280000, Caliber: tracequery.TargetWindowCPURepresentativeFrequencyCaliber, ClusterDonorCPU: &donor0, ClusterDonorSource: tracequery.ClusterFreqSourceExplicit}, true, "explicit topology"},
		{"derived_donor", &tracequery.TargetWindowCPURepresentativeFrequency{FrequencyKHz: 1280000, Caliber: tracequery.TargetWindowCPURepresentativeFrequencyCaliber, ClusterDonorCPU: &donor0, ClusterDonorSource: tracequery.ClusterFreqSourceDerived}, true, "frequency-change-derived cluster"},
	}
	ref := types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, Path: "/captures/source.ftrace", PayloadRef: "/results/source.json", RawRef: "/results/source.txt", QueryScopeID: "opaque-query"}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			account := &tracequery.TargetWindowStateAccount{Thread: tracequery.ThreadRef{Comm: "target", PID: 101}, Window: tracequery.TimeWindow{StartTs: 1, EndTs: 2}, RunningMs: 25, RunningCPUKnownMs: 25, RunningCPURosterStatus: "complete", RunningCPUAssignmentStatus: "complete", RunningCPURosterTotal: 1, RunningCPURosterEmitted: 1, RunningByCPU: []tracequery.TargetWindowCPURunning{{CPU: 2, RunningMs: 25, SegmentCount: 3, StartTs: 1.2, EndTs: 1.8, LineStart: 10, LineEnd: 40}}}
			baseRows := traceQueryTargetCPURunningObservations(account, "target-101", ref, "scope", "time")
			var baseText strings.Builder
			writeTraceTargetCPURunningRoster(&baseText, account)
			account.RunningByCPU[0].RepresentativeFrequency = tc.frequency
			before, _ := json.Marshal(account)
			rows := traceQueryTargetCPURunningObservations(account, "target-101", ref, "scope", "time")
			var text strings.Builder
			writeTraceTargetCPURunningRoster(&text, account)
			after, _ := json.Marshal(account)
			if string(before) != string(after) {
				t.Fatal("publication mutated original account")
			}
			if !tc.published {
				if !reflect.DeepEqual(rows, baseRows) || text.String() != baseText.String() {
					t.Fatalf("unknown/invalid carrier fabricated frequency: rows=%+v text=%s", rows, text.String())
				}
				return
			}
			if len(rows) != 1 || !strings.Contains(text.String(), "representative_frequency=1280000kHz") {
				t.Fatalf("valid target frequency missing: %+v / %s", rows, text.String())
			}
			wantNotes := []string{
				"target_cpu_running_representative_frequency_khz=1280000",
				"target_cpu_running_representative_frequency_caliber=last_positive_running_segment_start",
			}
			if tc.frequency.ClusterDonorCPU != nil {
				wantNotes = append(wantNotes,
					"target_cpu_running_representative_frequency_donor_cpu=0",
					"target_cpu_running_representative_frequency_donor_source="+tc.frequency.ClusterDonorSource)
			}
			var frequencyNotes []string
			var retained []string
			for _, note := range rows[0].RichNotes {
				if !strings.HasPrefix(note, "target_cpu_running_representative_frequency_") {
					retained = append(retained, note)
				} else {
					frequencyNotes = append(frequencyNotes, note)
				}
			}
			if !reflect.DeepEqual(frequencyNotes, wantNotes) || !strings.Contains(rows[0].Summary, "not constant frequency or residency") {
				t.Fatalf("both faces must preserve exact caliber and optional donor: notes=%v summary=%s", frequencyNotes, rows[0].Summary)
			}
			rows[0].RichNotes = retained
			rows[0].Summary = baseRows[0].Summary
			if !reflect.DeepEqual(rows, baseRows) {
				t.Fatalf("frequency publication changed original identity/value/range/source: %+v / %+v", rows, baseRows)
			}
			if tc.donor != "" && (!strings.Contains(text.String(), "CPU0") || !strings.Contains(text.String(), tc.donor)) {
				t.Fatalf("donor source was not disclosed: %s", text.String())
			}
		})
	}
}

func TestB1633ActualExecuteDistinguishesSharedAndMissingFrequency(t *testing.T) {
	for _, withFrequency := range []bool{true, false} {
		t.Run(map[bool]string{true: "shared_from_cpu_zero", false: "missing"}[withFrequency], func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "shared.ftrace")
			body := ""
			if withFrequency {
				body = "<idle>-0 (-----) [000] .... 0.900000: cpu_frequency: state=1280000 cpu_id=0\n"
			}
			body += "<idle>-0 (-----) [001] .... 1.000000: sched_switch: prev_comm=swapper/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=101 next_prio=120\n" +
				"target-101 (101) [001] .... 1.050000: sched_switch: prev_comm=target prev_pid=101 prev_prio=120 prev_state=S ==> next_comm=swapper/1 next_pid=0 next_prio=120\n"
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "pid": 101, "time_start": 1, "time_end": 1.05, "core_topology": "middle=0-1"})
			result, err := (&TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
			if err != nil || !result.Success {
				t.Fatalf("Execute failed: %v %+v", err, result)
			}
			var row *types.ObservationRecord
			for i := range result.Observations {
				if result.Observations[i].Predicate == "target_cpu_running" {
					row = &result.Observations[i]
				}
			}
			if row == nil || row.Value != "50.000" || row.Object != "cpu=1" {
				t.Fatalf("known running time must survive missing frequency: %+v", row)
			}
			if withFrequency {
				for _, want := range []string{"target_cpu_running_representative_frequency_khz=1280000", "target_cpu_running_representative_frequency_donor_cpu=0", "target_cpu_running_representative_frequency_donor_source=explicit_topology"} {
					if !slices.Contains(row.RichNotes, want) {
						t.Errorf("actual donor provenance missing %q: %v", want, row.RichNotes)
					}
				}
				if !strings.Contains(result.Summary, "cluster-shared from CPU0, explicit topology") {
					t.Fatal("actual shared timeline was disguised as an own-CPU sample")
				}
			} else {
				for _, note := range row.RichNotes {
					if strings.HasPrefix(note, "target_cpu_running_representative_frequency_") {
						t.Errorf("missing frequency became an invented value: %q", note)
					}
				}
				if strings.Contains(result.Summary, "representative_frequency=") {
					t.Fatal("missing frequency was published as a measured sample")
				}
			}
		})
	}
}

func TestB1633TargetFrequencyDoesNotAssignUnknownCPU(t *testing.T) {
	row := tracequery.TargetWindowCPURunning{CPU: -1, RunningMs: 1, RepresentativeFrequency: &tracequery.TargetWindowCPURepresentativeFrequency{FrequencyKHz: 1280000, Caliber: tracequery.TargetWindowCPURepresentativeFrequencyCaliber}}
	if notes, text := traceQueryTargetCPURepresentativeFrequency(row); len(notes) != 0 || text != "" {
		t.Fatalf("frequency must not invent an assigned CPU: %v %q", notes, text)
	}
}
