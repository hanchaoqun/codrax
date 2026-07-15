package tracequery

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unsafe"
)

func blockedReasonStrictLine(fields string) string {
	return "worker-20 (20) [004] .... 1.000000: sched_blocked_reason: " + fields
}

func TestSchedBlockedReasonStrictCanonicalProfiles(t *testing.T) {
	tests := []struct {
		name       string
		fields     string
		wantPID    int
		wantIO     int32
		wantDelay  int32
		delayKnown bool
		wantReason string
	}{
		{name: "donghu delay", fields: "pid=2147483647 iowait=1 caller=worker_thread+0x10/0x20 delay=2147483647", wantPID: 2147483647, wantIO: 1, wantDelay: 2147483647, delayKnown: true, wantReason: "worker_thread+0x10/0x20"},
		{name: "canonical pid zero", fields: "pid=0 iowait=0 caller=idle_marker", wantReason: "idle_marker"},
		{name: "legacy absent delay", fields: "pid=562 iowait=0 caller=mmc_wait_for_req", wantPID: 562, wantReason: "mmc_wait_for_req"},
		{name: "converter opaque", fields: "pid=563 iowait=1 caller=unknown caller_raw=0x11223344 caller_quality=opaque timestamp_source=thread_state_start_projection", wantPID: 563, wantIO: 1, wantReason: "unknown"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			line := blockedReasonStrictLine(tc.fields)
			ev, ok := ParseLine(1, line, newStringInterner())
			if !ok || ev.Type != EventSchedBlockedReason {
				t.Fatalf("canonical row rejected: ok=%t event=%+v", ok, ev)
			}
			if ev.WakeePID != tc.wantPID || ev.IOWait != tc.wantIO || !ev.BlockedReasonIOWaitKnown ||
				ev.BlockedDelay != tc.wantDelay || ev.BlockedDelayKnown != tc.delayKnown || ev.Reason != tc.wantReason {
				t.Fatalf("canonical fields drifted: %+v", ev)
			}
			if failure := blockedReasonValidationFailure(1, line); failure != nil {
				t.Fatalf("canonical row produced integrity failure: %+v", failure)
			}
		})
	}
}

func TestSchedBlockedReasonStrictProductionCensusCompatibility(t *testing.T) {
	tests := []struct {
		name           string
		path           string
		wantRows       int
		wantDelayKnown int
	}{
		{name: "donghu delay profile", path: "../../eval/fixtures/real_traces/donghu.ftrace", wantRows: 438, wantDelayKnown: 438},
		{name: "legacy absent delay profile", path: "../../eval/fixtures/real_traces/donghu_tieba_frame.systrace", wantRows: 441},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			idx, err := BuildIndex(context.Background(), tc.path)
			if err != nil {
				t.Fatal(err)
			}
			rows, delayKnown := 0, 0
			for _, ev := range idx.Events {
				if ev.Type != EventSchedBlockedReason {
					continue
				}
				rows++
				if ev.WakeePID <= 0 || !ev.BlockedReasonIOWaitKnown {
					t.Fatalf("production marker lost strict identity/classification: %+v", ev)
				}
				if ev.BlockedDelayKnown {
					delayKnown++
				}
			}
			if rows != tc.wantRows || delayKnown != tc.wantDelayKnown || len(idx.blockedReasonIntegrityFailures) != 0 || idx.blockedReasonIntegrityFailuresCapped {
				t.Fatalf("production profile drifted: rows=%d delay_known=%d failures=%+v capped=%t", rows, delayKnown, idx.blockedReasonIntegrityFailures, idx.blockedReasonIntegrityFailuresCapped)
			}
		})
	}
}

func TestSchedBlockedReasonStrictDimensionsFailIndependently(t *testing.T) {
	tests := []struct {
		name           string
		fields         string
		wantType       EventType
		wantPID        int
		wantIOKnown    bool
		wantIO         int32
		wantDelayKnown bool
		wantReason     string
		wantFailure    string
		wantScopedPIDs []int
		wantAffectsAll bool
	}{
		{name: "overflow cannot wrap to one", fields: "pid=562 iowait=4294967297 caller=f2fs_wait_on_block delay=7", wantType: EventSchedBlockedReason, wantPID: 562, wantDelayKnown: true, wantReason: "f2fs_wait_on_block", wantFailure: "iowait_invalid", wantScopedPIDs: []int{562}},
		{name: "iowait duplicate same value", fields: "pid=562 iowait=1 caller=f2fs iowait=1 delay=7", wantType: EventSchedBlockedReason, wantPID: 562, wantDelayKnown: true, wantReason: "f2fs", wantFailure: "iowait_duplicate", wantScopedPIDs: []int{562}},
		{name: "delay bad leaves identity and io", fields: "pid=562 iowait=1 caller=f2fs delay=-1", wantType: EventSchedBlockedReason, wantPID: 562, wantIOKnown: true, wantIO: 1, wantReason: "f2fs", wantFailure: "delay_invalid", wantScopedPIDs: []int{562}},
		{name: "zero delay leaves identity and io", fields: "pid=562 iowait=1 caller=f2fs delay=0", wantType: EventSchedBlockedReason, wantPID: 562, wantIOKnown: true, wantIO: 1, wantReason: "f2fs", wantFailure: "delay_invalid", wantScopedPIDs: []int{562}},
		{name: "delay duplicate leaves identity and io", fields: "pid=562 iowait=1 caller=f2fs delay=7 delay=7", wantType: EventSchedBlockedReason, wantPID: 562, wantIOKnown: true, wantIO: 1, wantReason: "f2fs", wantFailure: "delay_duplicate", wantScopedPIDs: []int{562}},
		{name: "caller duplicate withdraws cause only", fields: "pid=562 iowait=1 caller=real caller=forged", wantType: EventSchedBlockedReason, wantPID: 562, wantIOKnown: true, wantIO: 1, wantReason: "unknown", wantFailure: "caller_duplicate", wantScopedPIDs: []int{562}},
		{name: "pid zero soft failure stays scoped", fields: "pid=0 iowait=2 caller=idle_marker", wantType: EventSchedBlockedReason, wantReason: "idle_marker", wantFailure: "iowait_invalid", wantScopedPIDs: []int{0}},
		{name: "duplicate pid zero stays scoped", fields: "pid=0 iowait=1 caller=idle_marker pid=0", wantType: EventUnknown, wantFailure: "pid_duplicate", wantScopedPIDs: []int{0}},
		{name: "pid conflict binds no thread", fields: "pid=562 iowait=1 caller=f2fs pid=777", wantType: EventUnknown, wantFailure: "pid_duplicate", wantScopedPIDs: []int{562, 777}},
		{name: "pid overflow binds no thread", fields: "pid=2147483648 iowait=1 caller=f2fs", wantType: EventUnknown, wantFailure: "pid_invalid", wantAffectsAll: true},
		{name: "missing pid binds no thread", fields: "iowait=1 caller=f2fs", wantType: EventUnknown, wantFailure: "pid_missing", wantAffectsAll: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			line := blockedReasonStrictLine(tc.fields)
			ev, ok := ParseLine(1, line, newStringInterner())
			if !ok || ev.Type != tc.wantType {
				t.Fatalf("unexpected event admission: ok=%t event=%+v", ok, ev)
			}
			if ev.WakeePID != tc.wantPID || ev.BlockedReasonIOWaitKnown != tc.wantIOKnown || ev.IOWait != tc.wantIO ||
				ev.BlockedDelayKnown != tc.wantDelayKnown || ev.Reason != tc.wantReason {
				t.Fatalf("field-local degradation failed: %+v", ev)
			}
			failure := blockedReasonValidationFailure(1, line)
			if failure == nil || !containsSubstring(failure.Fields, tc.wantFailure) {
				t.Fatalf("missing typed failure %q: %+v", tc.wantFailure, failure)
			}
			if failure.AffectsAllPIDs != tc.wantAffectsAll || !sameInts(failure.PIDs, tc.wantScopedPIDs) {
				t.Fatalf("failure scope drifted: %+v", failure)
			}
		})
	}
}

func TestSchedBlockedReasonIntegrityLedgerSurvivesWindowDerive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "blocked-integrity.systrace")
	content := strings.Join([]string{
		// A closing marker before TimeStart is unrelated to every interval
		// carried into the selected window and must not hitchhike through the
		// scheduler-head snapshot.
		"worker-20 (20) [004] .... 1.000000: sched_blocked_reason: pid=562 iowait=4294967297 caller=f2fs_wait_on_block delay=7",
		"worker-562 (562) [004] .... 2.000000: sched_switch: prev_comm=worker-562 prev_pid=562 prev_prio=120 prev_state=D ==> next_comm=idle next_pid=0 next_prio=120",
		"worker-20 (20) [004] .... 2.600000: sched_blocked_reason: pid=562 iowait=4294967297 caller=f2fs_wait_on_block delay=7",
		"idle-0 (0) [004] .... 3.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker-562 next_pid=562 next_prio=120",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	full, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if len(full.blockedReasonIntegrityFailures) != 2 || full.blockedReasonIntegrityFailuresCapped {
		t.Fatalf("full index lost field-local ledger: %+v capped=%t", full.blockedReasonIntegrityFailures, full.blockedReasonIntegrityFailuresCapped)
	}
	windowed, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
		TimeStart: 2.5, TimeEnd: 3.1, TimeStartSet: true, TimeEndSet: true, AllowWindowedParse: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(windowed.blockedReasonIntegrityFailures) != 1 || windowed.blockedReasonIntegrityFailures[0].PIDs[0] != 562 {
		t.Fatalf("derived window did not retain only the in-window closing-marker failure: %+v", windowed.blockedReasonIntegrityFailures)
	}
	if windowed.blockedReasonIntegrityFailures[0].Ts != 2.6 {
		t.Fatalf("pre-window closing marker leaked through head carry-in: %+v", windowed.blockedReasonIntegrityFailures)
	}
	if schedulerStateIntegrityFailureForQuery(windowed, Query{TimeStart: 2.5, TimeEnd: 3.1}, 562) != nil {
		t.Fatal("blocked-reason metadata degradation must not poison the base scheduler state machine")
	}
	caveats := blockedReasonIntegrityCaveats(windowed, Query{TimeStart: 2.5, TimeEnd: 3.1}, 562)
	if !containsSubstring(caveats, "blocked_reason_integrity_degraded=true") || !containsSubstring(caveats, "iowait_invalid") {
		t.Fatalf("typed disclosure missing: %v", caveats)
	}
}

func TestSchedBlockedReasonIntegrityLedgerResourceAccounting(t *testing.T) {
	failure := blockedReasonIntegrityFailure{
		Line: 2, Ts: 1, CPU: 4, PIDs: []int{562, 777},
		Fields: []string{"iowait_duplicate", "delay_invalid"}, SourcePath: "/trace/a.systrace",
	}
	idx := &Index{}
	base := traceIndexCacheCost(idx)
	appendBlockedReasonIntegrityFailure(idx, failure)
	want := int64(unsafe.Sizeof(failure)) + int64(len(failure.SourcePath)) +
		int64(len(failure.PIDs))*int64(unsafe.Sizeof(int(0)))
	for _, field := range failure.Fields {
		want += int64(len(field))
	}
	if got := traceIndexCacheCost(idx) - base; got != want {
		t.Fatalf("index LRU undercharged blocked-reason ledger: got=%d want=%d", got, want)
	}
}

func TestSchedBlockedReasonOverflowScopeRebasesCompositeProvenance(t *testing.T) {
	offset, slope := 10.0, 2.0
	source := TraceArtifactSource{
		SourcePath: "/trace/a.systrace", VirtualLineBase: 100,
		ClockAlignment: TraceClockAlignmentAffine, ClockOffsetSec: &offset, ClockSlope: &slope,
	}
	scope := blockedReasonIntegrityOverflowScope{
		Set: true, MinLine: 2, MaxLine: 4, MinTs: 1, MaxTs: 3, PIDs: []int{562, 777},
	}
	mapped, ok := mapBlockedReasonIntegrityOverflowScope(scope, source)
	if !ok {
		t.Fatal("exact affine overflow scope was rejected")
	}
	if mapped.MinLine != 102 || mapped.MaxLine != 104 || mapped.MinTs != 12 || mapped.MaxTs != 16 ||
		!sameInts(mapped.PIDs, scope.PIDs) {
		t.Fatalf("overflow provenance did not rebase exactly: %+v", mapped)
	}
	if !blockedReasonIntegrityOverflowRelevantToQuery(mapped, Query{TimeStart: 11, TimeEnd: 13}, 562) ||
		blockedReasonIntegrityOverflowRelevantToQuery(mapped, Query{TimeStart: 16.1, TimeEnd: 17}, 562) ||
		blockedReasonIntegrityOverflowRelevantToQuery(mapped, Query{LineStart: 103, LineEnd: 103}, 999) {
		t.Fatalf("rebased overflow query scope drifted: %+v", mapped)
	}
}

func TestSchedBlockedReasonOverflowPIDDomainsDoNotCrossProductTime(t *testing.T) {
	idx := &Index{}
	for i := 0; i < blockedReasonIntegrityFailureCap; i++ {
		appendBlockedReasonIntegrityFailure(idx, blockedReasonIntegrityFailure{
			Line: i + 1, Ts: 0.5, PIDs: []int{99}, Fields: []string{"delay_invalid"},
		})
	}
	appendBlockedReasonIntegrityFailure(idx, blockedReasonIntegrityFailure{Line: 100, Ts: 1, PIDs: []int{10}, Fields: []string{"pid_duplicate"}})
	appendBlockedReasonIntegrityFailure(idx, blockedReasonIntegrityFailure{Line: 200, Ts: 10, PIDs: []int{20}, Fields: []string{"pid_duplicate"}})
	if blockedReasonRefinementCappedForQuery(idx, Query{TimeStart: 9, TimeEnd: 11}, 10) ||
		blockedReasonRefinementCappedForQuery(idx, Query{TimeStart: 0.9, TimeEnd: 1.1}, 20) ||
		blockedReasonRefinementCappedForQuery(idx, Query{TimeStart: 5, TimeEnd: 6}, 0) {
		t.Fatalf("PID union cross-producted disjoint physical domains: %+v", idx.blockedReasonIdentityOverflow)
	}
	if !blockedReasonRefinementCappedForQuery(idx, Query{TimeStart: 0.9, TimeEnd: 1.1}, 10) ||
		!blockedReasonRefinementCappedForQuery(idx, Query{TimeStart: 9, TimeEnd: 11}, 20) {
		t.Fatalf("exact PID domains lost their own physical rows: %+v", idx.blockedReasonIdentityOverflow)
	}
}

func TestMapBlockedReasonOverflowScopeDoesNotMutateSourceDomains(t *testing.T) {
	offset := 10.0
	source := TraceArtifactSource{VirtualLineBase: 100, ClockAlignment: TraceClockAlignmentAffine, ClockOffsetSec: &offset}
	scope := blockedReasonIntegrityOverflowScope{
		Set: true, MinLine: 1, MaxLine: 2, MinTs: 1, MaxTs: 2, PIDs: []int{42},
		PIDDomains: []blockedReasonIntegrityPIDDomain{{PID: 42, MinLine: 2, MaxLine: 2, MinTs: 2, MaxTs: 2}},
	}
	original := scope.clone()
	mapped, ok := mapBlockedReasonIntegrityOverflowScope(scope, source)
	if !ok {
		t.Fatal("exact offset mapping was rejected")
	}
	if !reflect.DeepEqual(scope, original) {
		t.Fatalf("mapping mutated shared source domains: before=%+v after=%+v", original, scope)
	}
	if mapped.PIDDomains[0].MinLine != 102 || mapped.PIDDomains[0].MinTs != 12 {
		t.Fatalf("mapped domain drifted: %+v", mapped.PIDDomains)
	}
}

func TestSchedBlockedReasonPIDZeroIdentityOverflowNeverConsumesPositiveBudget(t *testing.T) {
	idx := &Index{}
	for i := 0; i < blockedReasonIntegrityFailureCap; i++ {
		appendBlockedReasonIntegrityFailure(idx, blockedReasonIntegrityFailure{
			Line: i + 1, Ts: 0.5, PIDs: []int{99}, Fields: []string{"delay_invalid"},
		})
	}
	for i := 0; i < schedulerPIDCandidateScopeCap+1; i++ {
		appendBlockedReasonIntegrityFailure(idx, blockedReasonIntegrityFailure{
			Line: 100 + i, Ts: 1 + float64(i)/1000, PIDs: []int{0}, Fields: []string{"pid_duplicate"},
		})
	}
	if idx.blockedReasonIdentityOverflow.Set || blockedReasonRefinementCappedForQuery(idx, Query{TimeStart: 1, TimeEnd: 2}, 42) {
		t.Fatalf("pid=0 identity rows consumed positive-thread hard budget: %+v", idx.blockedReasonIdentityOverflow)
	}
}

func TestSchedBlockedReasonRepeatedPIDOverflowNeverEscalatesToSiblingPIDs(t *testing.T) {
	idx := &Index{}
	for i := 0; i < blockedReasonIntegrityFailureCap; i++ {
		appendBlockedReasonIntegrityFailure(idx, blockedReasonIntegrityFailure{
			Line: i + 1, Ts: 0.5, PIDs: []int{99}, Fields: []string{"delay_invalid"},
		})
	}
	for i := 0; i < schedulerPIDCandidateScopeCap+1; i++ {
		appendBlockedReasonIntegrityFailure(idx, blockedReasonIntegrityFailure{
			Line: 100 + i, Ts: 1 + float64(i)/1000, PIDs: []int{42}, Fields: []string{"pid_duplicate"},
		})
	}
	q := Query{TimeStart: 1, TimeEnd: 2}
	if idx.blockedReasonIdentityOverflow.AffectsAllPIDs || blockedReasonRefinementCappedForQuery(idx, q, 99) ||
		!blockedReasonRefinementCappedForQuery(idx, q, 42) {
		t.Fatalf("same-PID domain pressure escalated into sibling identity scope: %+v", idx.blockedReasonIdentityOverflow)
	}
	if len(idx.blockedReasonIdentityOverflow.PIDDomains) != 1 || idx.blockedReasonIdentityOverflow.PIDDomains[0].PID != 42 {
		t.Fatalf("same PID did not merge into one bounded domain: %+v", idx.blockedReasonIdentityOverflow.PIDDomains)
	}
}

func TestSchedBlockedReasonIntegrityCapIsQueryScopedAndNeverPoisonsSchedulerBaseState(t *testing.T) {
	idx := &Index{}
	for i := 0; i < blockedReasonIntegrityFailureCap; i++ {
		appendBlockedReasonIntegrityFailure(idx, blockedReasonIntegrityFailure{
			Line: i + 1, Ts: 1, PIDs: []int{20}, Fields: []string{"delay_invalid"},
		})
	}
	// This failure is dropped from the item ledger but its bounded overflow
	// scope retains time, line, and PID identity.
	appendBlockedReasonIntegrityFailure(idx, blockedReasonIntegrityFailure{
		Line: 100, Ts: 10, PIDs: []int{10}, Fields: []string{"pid_duplicate"},
	})
	idx.Events = []Event{
		{Type: EventSchedSwitch, Ts: 1, CPU: 0, PrevPID: 10, PrevState: "R", NextPID: 20},
		{Type: EventSchedSwitch, Ts: 2, CPU: 0, PrevPID: 20, PrevState: "R", NextPID: 10},
	}
	q := Query{TimeStart: 1, TimeEnd: 2}
	if schedulerStateIntegrityFailureForQuery(idx, q, 10) != nil {
		t.Fatal("blocked-reason audit cap must not poison independent scheduler transitions")
	}
	if caveats := blockedReasonIntegrityCaveats(idx, q, 10); containsSubstring(caveats, "blocked_reason_integrity_audit_truncated=true") {
		t.Fatalf("a future/PID-scoped overflow polluted an earlier disjoint query: %v", caveats)
	}
	if caveats := blockedReasonIntegrityCaveats(idx, Query{TimeStart: 9, TimeEnd: 11}, 10); !containsSubstring(caveats, "blocked_reason_integrity_audit_truncated=true") {
		t.Fatalf("relevant overflow scope was not disclosed: %v", caveats)
	}
	if caveats := blockedReasonIntegrityCaveats(idx, Query{TimeStart: 12, TimeEnd: 13}, 10); containsSubstring(caveats, "blocked_reason_integrity_audit_truncated=true") {
		t.Fatalf("overflow scope escaped beyond its last physical marker: %v", caveats)
	}
	idx.Events = append(idx.Events,
		Event{Type: EventSchedBlockedReason, Ts: 10.5, WakeePID: 10, IOWait: 1, BlockedReasonIOWaitKnown: true},
		Event{Type: EventSchedBlockedReason, Ts: 10.5, WakeePID: 20, IOWait: 1, BlockedReasonIOWaitKnown: true},
	)
	reasons := blockedReasonsByPID(idx, Query{TimeStart: 9, TimeEnd: 11})
	if len(reasons[10]) != 1 || len(reasons[20]) != 1 {
		t.Fatalf("raw marker inventory must survive until interval-local adjudication: %+v", reasons)
	}
	if !blockedReasonRefinementUnavailableForInterval(idx, Query{TimeStart: 9, TimeEnd: 11}, 10, 9, 10.5, true) ||
		blockedReasonRefinementUnavailableForInterval(idx, Query{TimeStart: 9, TimeEnd: 11}, 20, 9, 10.5, true) {
		t.Fatalf("interval-local overflow scope did not preserve the unrelated PID: %+v", idx.blockedReasonIdentityOverflow)
	}
}

func TestSchedBlockedReasonSoftFieldOverflowNeverWithdrawsDIO(t *testing.T) {
	for _, field := range []string{"iowait_invalid", "caller_duplicate", "delay_invalid"} {
		t.Run(field, func(t *testing.T) {
			idx := &Index{}
			for i := 0; i < blockedReasonIntegrityFailureCap; i++ {
				appendBlockedReasonIntegrityFailure(idx, blockedReasonIntegrityFailure{
					Line: i + 1, Ts: 1, PIDs: []int{20}, Fields: []string{"delay_invalid"},
				})
			}
			appendBlockedReasonIntegrityFailure(idx, blockedReasonIntegrityFailure{
				Line: 100, Ts: 2, PIDs: []int{20}, Fields: []string{field},
			})
			q := Query{TimeStart: 1, TimeEnd: 3}
			if blockedReasonRefinementCappedForQuery(idx, q, 20) {
				t.Fatalf("soft-field audit overflow withdrew D/IO: %+v", idx.blockedReasonIdentityOverflow)
			}
			caveats := blockedReasonIntegrityCaveats(idx, q, 20)
			if !containsSubstring(caveats, "blocked_reason_integrity_audit_truncated=true") ||
				!containsSubstring(caveats, "per-event typed known/unknown fields") {
				t.Fatalf("soft-field overflow caveat is inaccurate: %v", caveats)
			}
		})
	}
}

func TestSchedBlockedReasonPIDZeroOverflowNeverPoisonsPositiveTID(t *testing.T) {
	idx := &Index{}
	for i := 0; i <= blockedReasonIntegrityFailureCap; i++ {
		appendBlockedReasonIntegrityFailure(idx, blockedReasonIntegrityFailure{
			Line: i + 1, Ts: 1 + float64(i)/1000, PIDs: []int{0}, Fields: []string{"iowait_invalid"},
		})
	}
	q := Query{TimeStart: 1, TimeEnd: 2}
	if caveats := blockedReasonIntegrityCaveats(idx, q, 562); len(caveats) != 0 {
		t.Fatalf("pid=0 overflow polluted positive target: %v", caveats)
	}
	if caveats := blockedReasonIntegrityCaveats(idx, q, 0); !containsSubstring(caveats, "blocked_reason_integrity_audit_truncated=true") {
		t.Fatalf("global inventory lost pid=0 overflow disclosure: %v", caveats)
	}
	if blockedReasonRefinementCappedForQuery(idx, q, 0) {
		t.Fatal("pid=0-only overflow withdrew positive-thread refinement in a global view")
	}
	idx.Events = []Event{{Type: EventSchedBlockedReason, Ts: 1.5, WakeePID: 562, IOWait: 1, BlockedReasonIOWaitKnown: true}}
	if got := blockedReasonsByPID(idx, q)[562]; len(got) != 1 {
		t.Fatalf("pid=0-only overflow hid a valid positive marker: %+v", got)
	}
}

func TestSchedBlockedReasonPureBadPIDOverflowDoesNotBindAnyPID(t *testing.T) {
	idx := &Index{}
	for i := 0; i < blockedReasonIntegrityFailureCap; i++ {
		appendBlockedReasonIntegrityFailure(idx, blockedReasonIntegrityFailure{
			Line: i + 1, Ts: 1, PIDs: []int{i + 1}, Fields: []string{"delay_invalid"},
		})
	}
	appendBlockedReasonIntegrityFailure(idx, blockedReasonIntegrityFailure{
		Line: 100, Ts: 2, AffectsAllPIDs: true, Fields: []string{"pid_invalid"},
	})
	q := Query{TimeStart: 1, TimeEnd: 3}
	if blockedReasonRefinementCappedForQuery(idx, q, 20) || blockedReasonRefinementCappedForQuery(idx, q, 999) {
		t.Fatalf("a wholly malformed PID acquired authority over canonical siblings: general=%+v identity=%+v", idx.blockedReasonIntegrityOverflow, idx.blockedReasonIdentityOverflow)
	}
	if caveats := blockedReasonIntegrityCaveats(idx, q, 20); !containsSubstring(caveats, "blocked_reason_integrity_audit_truncated=true") {
		t.Fatalf("audit truncation must remain disclosed without binding a target: %v", caveats)
	}
}

func TestSchedBlockedReasonStrictIndexedAndStreamScanParity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "blocked-stream.systrace")
	content := blockedReasonStrictLine("pid=562 iowait=4294967297 caller=f2fs_wait_on_block delay=7") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	indexed, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	var streamed []Event
	shell, err := StreamScan(context.Background(), path, TraceFlavorAuto, func(ev Event) bool {
		streamed = append(streamed, ev)
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(indexed.Events) != 1 || len(streamed) != 1 || !reflect.DeepEqual(indexed.Events[0], streamed[0]) {
		t.Fatalf("indexed/stream event parity failed: indexed=%+v streamed=%+v", indexed.Events, streamed)
	}
	for lane, candidate := range map[string]*Index{"indexed": indexed, "stream": shell} {
		if len(candidate.blockedReasonIntegrityFailures) != 1 ||
			!containsSubstring(candidate.blockedReasonIntegrityFailures[0].Fields, "iowait_invalid") {
			t.Fatalf("%s lost typed integrity ledger: %+v", lane, candidate.blockedReasonIntegrityFailures)
		}
	}
}

func TestStreamEventSearchAuditsBlockedReasonAfterUnrelatedTailRow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "blocked-stream-tail.systrace")
	content := strings.Join([]string{
		"idle-0 (0) [004] .... 0.999000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=562 next_prio=120",
		"worker-562 (562) [004] .... 1.000001: tracing_mark_write: B|562|ordinary-tail-row",
		"worker-20 (20) [004] .... 1.000002: sched_blocked_reason: pid=562 iowait=2 caller=f2fs_wait_on_block delay=7",
		"worker-562 (562) [004] .... 1.000010: tracing_mark_write: E|562",
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// Complete the shared anchor first so the streaming lane has a proven
	// monotonic early-stop authority; this is the exact warm path that used to
	// stop on the unrelated +1us row and miss the malformed +2us marker.
	indexed, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	q := Query{View: "event_search", PID: 562, TimeStart: 0.99, TimeEnd: 1.0, Limit: 8}
	if caveats := blockedReasonIntegrityCaveats(indexed, q, q.PID); !containsSubstring(caveats, "iowait_invalid") {
		t.Fatalf("indexed tail audit missing: %v", caveats)
	}

	streamed, err := StreamEventSearch(context.Background(), path, q)
	if err != nil {
		t.Fatal(err)
	}
	if !containsSubstring(streamed.Caveats, "iowait_invalid") {
		t.Fatalf("stream tail audit diverged after unrelated row: %v", streamed.Caveats)
	}
	invalidCaveats := 0
	for _, caveat := range streamed.Caveats {
		if strings.Contains(caveat, "iowait_invalid") {
			invalidCaveats++
		}
	}
	if invalidCaveats != 1 {
		t.Fatalf("stream integrity caveat must publish exactly once, got %d: %v", invalidCaveats, streamed.Caveats)
	}
	for _, event := range streamed.Events {
		if event.Ts > q.TimeEnd {
			t.Fatalf("audit-only closing tail leaked into event results: %+v", event)
		}
	}
}

func sameInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
