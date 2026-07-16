package tracequery

import (
	"encoding/json"
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
	if sample.PerfFields.ThreadIdentityKnown == nil || *sample.PerfFields.ThreadIdentityKnown || sample.PerfFields.Resolution != "resolved" || sample.PerfFields.LifecycleUnverified == nil || *sample.PerfFields.LifecycleUnverified {
		t.Fatalf("bundle scrub did not publish an explicit, provenance-preserving thread hard-negative: %+v", sample.PerfFields)
	}
	if sample.CPU != -1 || sample.PerfFields.CPUKnown == nil || *sample.PerfFields.CPUKnown {
		t.Fatalf("unproven CPU identity was retained: %+v", sample)
	}
	if got := computePerfContext(idx, Query{TimeStart: 10.0, TimeEnd: 10.4}, 8); got == nil || got.SampleCount != 1 || len(got.TopThreads) != 0 || len(got.TopSymbols) != 1 || len(got.TopSymbols[0].Threads) != 0 {
		t.Fatalf("anonymous global perf inventory unavailable: %+v", got)
	}
	if got := perfContextForThread(idx, Query{}, ThreadRef{PID: 20}, 10.0, 10.4, 8); got != nil {
		t.Fatalf("anonymous perf sample attached to a TID: %+v", got)
	}
	if eventMentionsThread(*sample, "app") || eventMentionsPID(*sample, 20) {
		t.Fatal("anonymous perf sample resurrected scrubbed identity through raw/text substring matching")
	}
	encoded, err := json.Marshal(sample)
	if err != nil {
		t.Fatal(err)
	}
	var surface map[string]any
	if err := json.Unmarshal(encoded, &surface); err != nil {
		t.Fatal(err)
	}
	if known, exists := surface["perf_thread_identity_known"]; !exists || known != false {
		t.Fatalf("JSON did not disclose explicit false thread identity: %s", encoded)
	}
	for _, key := range []string{"comm", "pid", "tgid", "perf_pid", "perf_tid", "perf_comm"} {
		if _, exists := surface[key]; exists {
			t.Fatalf("JSON revived scrubbed key %q: %s", key, encoded)
		}
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
	if sample.PerfFields.ThreadIdentityKnown == nil || !*sample.PerfFields.ThreadIdentityKnown || sample.PerfFields.Resolution != "resolved" || sample.PerfFields.LifecycleUnverified == nil || *sample.PerfFields.LifecycleUnverified {
		t.Fatalf("proved bundle identity provenance drifted: %+v", sample.PerfFields)
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

func TestBundlePerfStrictRawCapabilityRetainsThreadAndCPUIdentity(t *testing.T) {
	bundle := writePerfAdmissionBundle(t,
		`,"perf_capability":{"time_domain":"trace_seconds","thread_identity":"present_valid_sample_pid_tid_only","cpu_identity":"present_valid_sample_cpu_else_unknown","trace_query_ready":true}`,
		"")
	idx, err := BuildIndex(t.Context(), bundle)
	if err != nil {
		t.Fatal(err)
	}
	var sample *Event
	for index := range idx.Events {
		if idx.Events[index].Type == EventPerfSample {
			sample = &idx.Events[index]
		}
	}
	if sample == nil || sample.PerfFields == nil || sample.PID != 20 || sample.PerfFields.TID != 20 ||
		sample.CPU != 1 || sample.PerfFields.CPUKnown == nil || !*sample.PerfFields.CPUKnown ||
		sample.PerfFields.ThreadIdentityKnown == nil || !*sample.PerfFields.ThreadIdentityKnown {
		t.Fatalf("strict raw capability lost proved thread/CPU identity: %+v", sample)
	}
	joined := strings.Join(idx.Caveats, "\n")
	for _, want := range []string{
		"thread_identity=present_valid_sample_pid_tid_only",
		"thread_identity_proven=true",
		"cpu_identity=present_valid_sample_cpu_else_unknown",
		"cpu_identity_proven=true",
		"thread_identity_scrubbed=0",
		"cpu_identity_scrubbed=0",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("strict raw admission disclosure missing %q: %s", want, joined)
		}
	}
}

func TestPerfBundleIdentityScrubIsIdempotentForProvenAndUnprovenCapabilities(t *testing.T) {
	makeEvent := func() Event {
		known, verified := true, false
		return Event{
			Type: EventPerfSample, Comm: "app", PID: 20, TGID: 20, CPU: 1,
			PerfFields: &PerfFields{
				PID: 20, TID: 20, Comm: "app", CPUKnown: &known,
				ThreadIdentityKnown: &known, Resolution: "resolved", LifecycleUnverified: &verified,
				Symbol: "Hot::work", SampleKind: "on_cpu",
			},
		}
	}
	for _, tc := range []struct {
		name       string
		capability *traceBundlePerfCapability
		wantKnown  bool
	}{
		{
			name:       "unproven",
			capability: &traceBundlePerfCapability{TraceQueryReady: true},
			wantKnown:  false,
		},
		{
			name: "proven",
			capability: &traceBundlePerfCapability{
				TraceQueryReady: true, ThreadIdentity: "sample_pid_tid_thread_comm", CPUIdentity: "sample_cpu",
			},
			wantKnown: true,
		},
		{
			name: "raw strict proven",
			capability: &traceBundlePerfCapability{
				TraceQueryReady: true,
				ThreadIdentity:  "present_valid_sample_pid_tid_only",
				CPUIdentity:     "present_valid_sample_cpu_else_unknown",
			},
			wantKnown: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			events := []Event{makeEvent()}
			for pass := 0; pass < 2; pass++ {
				var summary perfBundleAdmissionSummary
				events, summary = applyPerfBundleAdmission(events, tc.capability)
				if len(events) != 1 || summary.SharedPerfSamples != 1 {
					t.Fatalf("pass %d lost sample: events=%+v summary=%+v", pass, events, summary)
				}
				pf := events[0].PerfFields
				if pf == nil || pf.ThreadIdentityKnown == nil || *pf.ThreadIdentityKnown != tc.wantKnown || pf.Resolution != "resolved" || pf.LifecycleUnverified == nil || *pf.LifecycleUnverified {
					t.Fatalf("pass %d identity state drifted: %+v", pass, pf)
				}
				if tc.wantKnown {
					if !perfSampleHasTypedThreadIdentity(events[0]) || events[0].PID != 20 || pf.TID != 20 ||
						pf.CPUKnown == nil || !*pf.CPUKnown || events[0].CPU != 1 {
						t.Fatalf("pass %d proved thread/CPU identity was scrubbed: %+v", pass, events[0])
					}
				} else if perfSampleHasTypedThreadIdentity(events[0]) || events[0].PID != 0 || pf.TID != 0 ||
					eventMentionsPID(events[0], 20) || pf.CPUKnown == nil || *pf.CPUKnown || events[0].CPU != -1 {
					t.Fatalf("pass %d unproven thread/CPU identity was revived: %+v", pass, events[0])
				}
			}
		})
	}
}

func TestBundlePerfCapabilityEnumsAreClosedForHardIdentityGates(t *testing.T) {
	if perfThreadIdentityCapabilityProven("available") || perfCPUIdentityCapabilityProven("known") {
		t.Fatal("free-form capability prose became a hard identity proof")
	}
	if !perfThreadIdentityCapabilityProven("pid_tid_from_sample_or_comm") || !perfCPUIdentityCapabilityProven("sample_cpu_when_recorded") {
		t.Fatal("converter-owned capability enum was not recognized")
	}
	if !perfThreadIdentityCapabilityProven("present_valid_sample_pid_tid_only") ||
		!perfCPUIdentityCapabilityProven("present_valid_sample_cpu_else_unknown") {
		t.Fatal("strict raw converter capability enum was not recognized")
	}
}

func TestBundleSystraceFieldCannotReclassifyPerftracePath(t *testing.T) {
	dir := t.TempDir()
	perftrace := filepath.Join(dir, "capture.perftrace")
	bundle := filepath.Join(dir, "capture.tracebundle.json")
	writePerfAdmissionFixture(t, perftrace,
		"app-20 (20) [001] .... 10.000000: sched_switch: prev_comm=app prev_pid=20 prev_prio=120 prev_state=S ==> next_comm=intruder next_pid=99 next_prio=120\n")
	if err := os.WriteFile(bundle, []byte(`{
  "schema":"codrax.tracebundle/v2",
  "capture_id":"sha256:0000000000000000000000000000000000000000000000000000000000000000",
  "version":"test",
  "systrace":"capture.perftrace",
  "artifacts":[{"type":"systrace","path":"capture.perftrace","bytes":1,"sha256":"0000000000000000000000000000000000000000000000000000000000000000"}]
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	idx, err := BuildIndex(t.Context(), bundle)
	if err == nil || idx != nil || !strings.Contains(err.Error(), "conflicts with .perftrace") {
		t.Fatalf("perftrace suffix bypassed V2 type admission: idx=%+v err=%v", idx, err)
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
		"app-20 (20) [001] .... 10.200000: perf_sample: cpu=1 cpu_known=true pid=20 tid=20 thread_comm=app period=100 event=cpu-cycles symbol=Hot::work dso=libhot.so source=fixture sample_kind=on_cpu thread_identity_known=true resolution=resolved lifecycle_unverified=false",
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
	data := []byte(body)
	if strings.HasSuffix(path, ".tracebundle.json") {
		data = traceBundleV2JSONForTest(t, path, data)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
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
