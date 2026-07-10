package tracequery

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBundlePerfSchedulerRowsCannotPolluteFullOrWindowHeadWithoutReadyCapability(t *testing.T) {
	for _, tc := range []struct {
		name       string
		capability string
		alignment  string
	}{
		{
			name: "missing_capability",
			alignment: `,"perf_clock_alignments":[{
        "artifact_path":"capture.perftrace",
        "perf_time_domain":"trace_seconds",
        "trace_time_domain":"trace_seconds",
        "offset_sec":0.0,
        "slope":1.0,
        "calibrated":true
      }]`,
		},
		{
			name:       "trace_query_ready_false",
			capability: `,"perf_capability":{"time_domain":"trace_seconds","thread_identity":"sample_pid_tid_thread_comm","cpu_identity":"sample_cpu","trace_query_ready":false}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bundle := writePerfAdmissionBundle(t, tc.capability, tc.alignment)
			full, err := BuildIndex(t.Context(), bundle)
			if err != nil {
				t.Fatal(err)
			}
			windowed, err := BuildIndexWithOptions(t.Context(), bundle, BuildOptions{
				AllowWindowedParse: true,
				TimeStart:          10.0,
				TimeEnd:            10.4,
				TimeStartSet:       true,
				TimeEndSet:         true,
				TimePaddingBefore:  0.05,
				TimePaddingAfter:   0.05,
			})
			if err != nil {
				t.Fatal(err)
			}

			for _, idx := range []*Index{full, windowed} {
				for _, ev := range idx.Events {
					if ev.Type == EventPerfSample || ev.PID == 99 || ev.PrevPID == 20 && ev.NextPID == 99 {
						t.Fatalf("unready perf row entered the shared stream: %+v", ev)
					}
				}
				timeline := ThreadTimeline(idx, Query{PID: 20, TimeStart: 10.0, TimeEnd: 10.4})
				if len(timeline.Intervals) != 1 || timeline.Intervals[0].State != StateRunning || math.Abs(timeline.Intervals[0].DurationMs-400) > 0.001 {
					t.Fatalf("perf scheduler row changed scheduler/head causality: head=%+v intervals=%+v", timeline.HeadState, timeline.Intervals)
				}
				joined := strings.Join(idx.Caveats, "\n")
				if !strings.Contains(joined, "tracebundle_perf_admission") ||
					!strings.Contains(joined, "trace_query_ready=false") ||
					!strings.Contains(joined, "omitted_not_ready=2") ||
					!strings.Contains(joined, "scheduler_or_cpu_rows_omitted=1") {
					t.Fatalf("missing typed perf admission disclosure: %s", joined)
				}
				perfSource := findPerfAdmissionSource(t, idx)
				if perfSource.EventCount != 2 {
					t.Fatalf("physical perf inventory was lost: %+v", perfSource)
				}
			}
		})
	}
}

func TestBundlePerfReadySamplesPreserveInventoryButScrubUnprovenIdentity(t *testing.T) {
	bundle := writePerfAdmissionBundle(t,
		`,"perf_capability":{"time_domain":"trace_seconds","trace_query_ready":true}`,
		"")
	idx, err := BuildIndex(t.Context(), bundle)
	if err != nil {
		t.Fatal(err)
	}

	var sample *Event
	for i := range idx.Events {
		if idx.Events[i].Type == EventSchedSwitch && idx.Events[i].NextPID == 99 {
			t.Fatalf("perf scheduler row entered shared causality: %+v", idx.Events[i])
		}
		if idx.Events[i].Type == EventPerfSample {
			sample = &idx.Events[i]
		}
	}
	if sample == nil || sample.PerfFields == nil || sample.PerfFields.Symbol != "Hot::work" {
		t.Fatalf("safe perf symbol inventory was not retained: %+v", idx.Events)
	}
	if sample.PID != 0 || sample.TGID != 0 || sample.Comm != "" ||
		sample.PerfFields.PID != 0 || sample.PerfFields.TID != 0 || sample.PerfFields.Comm != "" {
		t.Fatalf("unproven thread identity was retained: %+v", sample)
	}
	if sample.CPU != -1 || sample.PerfFields.CPUKnown == nil || *sample.PerfFields.CPUKnown {
		t.Fatalf("unproven CPU identity was retained: %+v", sample)
	}
	if got := computePerfContext(idx, Query{TimeStart: 10.0, TimeEnd: 10.4}, 8); got == nil || got.SampleCount != 1 {
		t.Fatalf("anonymous global perf inventory unavailable: %+v", got)
	}
	if got := perfContextForThread(idx, Query{}, ThreadRef{PID: 20}, 10.0, 10.4, 8); got != nil {
		t.Fatalf("anonymous perf sample attached to a TID: %+v", got)
	}
	if eventMentionsThread(*sample, "app") {
		t.Fatal("anonymous perf sample resurrected scrubbed identity through raw/text substring matching")
	}
	joined := strings.Join(idx.Caveats, "\n")
	for _, want := range []string{
		"shared_perf_samples=1",
		"omitted_non_perf=1",
		"scheduler_or_cpu_rows_omitted=1",
		"thread_identity_proven=false",
		"cpu_identity_proven=false",
		"thread_identity_scrubbed=1",
		"cpu_identity_scrubbed=1",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in admission disclosure: %s", want, joined)
		}
	}
	if source := findPerfAdmissionSource(t, idx); source.EventCount != 2 {
		t.Fatalf("physical perf inventory count mismatch: %+v", source)
	}
}

func TestBundlePerfKnownCapabilityAdmitsSampleIdentityButNeverSchedulerRows(t *testing.T) {
	bundle := writePerfAdmissionBundle(t,
		`,"perf_capability":{"time_domain":"trace_seconds","thread_identity":"sample_pid_tid_thread_comm","cpu_identity":"sample_cpu","trace_query_ready":true}`,
		"")
	idx, err := BuildIndex(t.Context(), bundle)
	if err != nil {
		t.Fatal(err)
	}

	var sample *Event
	for i := range idx.Events {
		if idx.Events[i].Type == EventSchedSwitch && idx.Events[i].NextPID == 99 {
			t.Fatalf("sample capability was incorrectly treated as scheduler provenance: %+v", idx.Events[i])
		}
		if idx.Events[i].Type == EventPerfSample {
			sample = &idx.Events[i]
		}
	}
	if sample == nil || sample.PerfFields == nil || sample.PerfFields.TID != 20 || sample.PerfFields.PID != 20 || sample.CPU != 1 {
		t.Fatalf("valid typed sample identity was not retained: %+v", sample)
	}
	if sample.PerfFields.CPUKnown == nil || !*sample.PerfFields.CPUKnown {
		t.Fatalf("valid typed sample CPU was not retained: %+v", sample)
	}
	if got := perfContextForThread(idx, Query{}, ThreadRef{PID: 20}, 10.0, 10.4, 8); got == nil || got.SampleCount != 1 {
		t.Fatalf("valid typed sample did not attach to its thread: %+v", got)
	}
	joined := strings.Join(idx.Caveats, "\n")
	for _, want := range []string{
		"thread_identity_proven=true",
		"cpu_identity_proven=true",
		"shared_perf_samples=1",
		"scheduler_or_cpu_rows_omitted=1",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in valid admission disclosure: %s", want, joined)
		}
	}
}

func TestBundlePerfCapabilityEnumsAreClosedForHardIdentityGates(t *testing.T) {
	if perfThreadIdentityCapabilityProven("available") || perfCPUIdentityCapabilityProven("known") {
		t.Fatal("free-form capability prose became a hard identity proof")
	}
	if !perfThreadIdentityCapabilityProven("pid_tid_from_sample_or_comm") || !perfCPUIdentityCapabilityProven("sample_cpu_when_recorded") {
		t.Fatal("converter-owned capability enum was not recognized")
	}
}

func TestBundleSystraceFieldCannotReclassifyPerftracePath(t *testing.T) {
	dir := t.TempDir()
	perftrace := filepath.Join(dir, "capture.perftrace")
	bundle := filepath.Join(dir, "capture.tracebundle.json")
	writePerfAdmissionFixture(t, perftrace,
		"app-20 (20) [001] .... 10.000000: sched_switch: prev_comm=app prev_pid=20 prev_prio=120 prev_state=S ==> next_comm=intruder next_pid=99 next_prio=120\n")
	writePerfAdmissionFixture(t, bundle, `{"version":"test","systrace":"capture.perftrace"}`)

	idx, err := BuildIndex(t.Context(), bundle)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Events) != 0 || len(idx.TraceArtifacts) != 1 || idx.TraceArtifacts[0].Kind != "perftrace" {
		t.Fatalf("perftrace suffix bypassed admission through systrace declaration: events=%+v artifacts=%+v", idx.Events, idx.TraceArtifacts)
	}
	joined := strings.Join(idx.Caveats, "\n")
	if !strings.Contains(joined, "tracebundle_perf_admission") || !strings.Contains(joined, "scheduler_or_cpu_rows_omitted=1") {
		t.Fatalf("reclassified perftrace rejection was not disclosed: %s", joined)
	}
}

func writePerfAdmissionBundle(t *testing.T, capabilityFragment, alignmentFragment string) string {
	t.Helper()
	dir := t.TempDir()
	systrace := filepath.Join(dir, "capture.systrace")
	perftrace := filepath.Join(dir, "capture.perftrace")
	bundle := filepath.Join(dir, "capture.tracebundle.json")
	writePerfAdmissionFixture(t, systrace, strings.Join([]string{
		"<idle>-0 (0) [001] .... 9.000000: sched_switch: prev_comm=swapper/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=120",
		"app-20 (20) [001] .... 10.500000: sched_switch: prev_comm=app prev_pid=20 prev_prio=120 prev_state=S ==> next_comm=swapper/1 next_pid=0 next_prio=120",
	}, "\n")+"\n")
	writePerfAdmissionFixture(t, perftrace, strings.Join([]string{
		"app-20 (20) [001] .... 9.500000: sched_switch: prev_comm=app prev_pid=20 prev_prio=120 prev_state=S ==> next_comm=intruder next_pid=99 next_prio=120",
		"app-20 (20) [001] .... 10.200000: perf_sample: cpu=1 cpu_known=true pid=20 tid=20 thread_comm=app period=100 event=cpu-cycles symbol=Hot::work dso=libhot.so source=fixture",
	}, "\n")+"\n")
	manifest := fmt.Sprintf(`{
  "version":"test",
  "systrace":"capture.systrace",
  "artifacts":[
    {"type":"systrace","path":"capture.systrace"},
    {"type":"perftrace","path":"capture.perftrace"%s}
  ]%s
}`, capabilityFragment, alignmentFragment)
	writePerfAdmissionFixture(t, bundle, manifest)
	return bundle
}

func writePerfAdmissionFixture(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func findPerfAdmissionSource(t *testing.T, idx *Index) TraceArtifactSource {
	t.Helper()
	for _, source := range idx.TraceArtifacts {
		if source.Kind == "perftrace" {
			return source
		}
	}
	t.Fatalf("perf source missing: %+v", idx.TraceArtifacts)
	return TraceArtifactSource{}
}
