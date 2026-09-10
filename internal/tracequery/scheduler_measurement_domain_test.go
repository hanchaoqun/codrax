package tracequery

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Start at the real engine publication boundary. The JSON assertion deliberately
// predates the optional carrier so a missing receipt is a semantic RED, not a
// compile failure against a proposed API.
func b1638b1WireDomain(t *testing.T, value any) map[string]any {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(body, &object); err != nil {
		t.Fatal(err)
	}
	domain, ok := object["measurement_domain"].(map[string]any)
	if !ok || domain["partition_id"] == "" {
		t.Fatalf("published scheduler partition lacks its measurement receipt: %s", body)
	}
	if domain["status"] != "constructed_partition" || domain["method"] != "thread_timeline" ||
		domain["version"] != float64(1) || domain["target_tid"] != float64(55) {
		t.Fatalf("receipt changed its bounded, target-owned meaning: %+v", domain)
	}
	return domain
}

func TestB1638B1ActualRunMeasurementDomain(t *testing.T) {
	path := b1636HeadlessTrace(t, false)
	idx, err := BuildIndex(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(idx.Events)
	q := b1636Query()
	q.View = "thread_timeline"
	result := Run(idx, q)
	if result.Timeline == nil || len(result.Timeline.Intervals) == 0 || result.TargetWindowStates == nil {
		t.Fatalf("fixture must publish a real timeline and account: %+v", result)
	}
	want := b1638b1WireDomain(t, result.Timeline)
	if got := b1638b1WireDomain(t, result.TargetWindowStates); !reflect.DeepEqual(got, want) {
		t.Fatalf("account must carry exactly its own native timeline receipt: got=%v want=%v", got, want)
	}
	for _, view := range []string{"window_stats", "root_cause_rank", "frame_root_cause_bundle"} {
		t.Run(view, func(t *testing.T) {
			viewQ := q
			viewQ.View = view
			viewQ.MinDurationMs, viewQ.MaxBranches, viewQ.MaxDepth, viewQ.MaxChainNodes, viewQ.Limit = 50, 1, 1, 1, 1
			got := Run(idx, viewQ)
			account := got.TargetWindowStates
			if got.FrameRootCauseBundle != nil {
				account = got.FrameRootCauseBundle.TargetWindowStates
			}
			if account == nil {
				t.Fatal("fixture lost its target account")
			}
			if receipt := b1638b1WireDomain(t, account); !reflect.DeepEqual(receipt, want) {
				t.Fatalf("presentation/chain caps cannot change the complete native partition: got=%v want=%v", receipt, want)
			}
			if account.RunningMs != result.TargetWindowStates.RunningMs || account.RunnableMs != result.TargetWindowStates.RunnableMs ||
				account.SleepMs != result.TargetWindowStates.SleepMs || account.TotalMs != result.TargetWindowStates.TotalMs {
				t.Fatalf("metadata must not change measured state values: %+v", account)
			}
		})
	}
	after, _ := json.Marshal(idx.Events)
	if !bytes.Equal(before, after) {
		t.Fatal("measurement publication mutated source events")
	}
}

func TestB1638B1ActualRunLineFilterSeparatesMeasurements(t *testing.T) {
	idx, err := BuildIndex(t.Context(), b1636HeadlessTrace(t, false))
	if err != nil {
		t.Fatal(err)
	}
	q := b1636Query()
	q.View = "thread_timeline"
	full := Run(idx, q)
	fullDomain := b1638b1WireDomain(t, full.Timeline)
	q.LineStart, q.LineEnd = 1, 4
	partial := Run(idx, q)
	partialDomain := b1638b1WireDomain(t, partial.Timeline)
	if fullDomain["partition_id"] == partialDomain["partition_id"] {
		t.Fatal("same time bounds cannot collapse a line-filtered open tail into the unfiltered measurement")
	}
	if partialDomain["query_line_start"] != float64(1) || partialDomain["query_line_end"] != float64(4) {
		t.Fatalf("receipt must preserve the actual producer line filter: %+v", partialDomain)
	}
	if !reflect.DeepEqual(partialDomain, b1638b1WireDomain(t, partial.TargetWindowStates)) {
		t.Fatal("filtered account borrowed the full query receipt")
	}
}

func b1638b1NativePartition() TimelineResult {
	thread := ThreadRef{PID: 55, TGID: 55, Comm: "frag"}
	return TimelineResult{
		Thread: thread, Window: TimeWindow{StartTs: 1, EndTs: 2},
		HeadState: &TimelineHeadState{Status: "unknown", BoundaryTs: 1, Reason: "no_prior_scheduler_state_for_target"},
		Intervals: []Interval{{Thread: thread, State: StateSSleep, StartTs: 1.1, EndTs: 1.9, DurationMs: 800,
			ActualStartTs: 1.05, ActualEndTs: 1.95, ActualDurationMs: 900,
			StartLine: 10, EndLine: 20, WakeupLine: 21, PrevStateRaw: "S",
			BlockedReasonCaller: "wait", BlockedReasonLine: 19, BlockedReasonIOWaitKnown: true}},
	}
}

func b1638b1ClonePartition(in TimelineResult) TimelineResult {
	out := cloneTimelineResult(in)
	if in.HeadState != nil {
		head := *in.HeadState
		out.HeadState = &head
	}
	return out
}

func TestB1638B1HashOwnsWholeTypedPartitionNotTotalsOrProse(t *testing.T) {
	q := Query{TimeStart: 1, TimeEnd: 2, TimeStartSet: true, TimeEndSet: true}
	base := b1638b1NativePartition()
	before, _ := json.Marshal(base)
	want := buildTimelineMeasurementDomain(q, base)
	if want == nil {
		t.Fatal("positive native partition with an unknown head must retain a bounded receipt")
	}
	changes := map[string]func(*TimelineResult){
		"state":             func(tl *TimelineResult) { tl.Intervals[0].State = StateDSleep },
		"start":             func(tl *TimelineResult) { tl.Intervals[0].StartTs = math.Nextafter(1.1, 2) },
		"end":               func(tl *TimelineResult) { tl.Intervals[0].EndTs = math.Nextafter(1.9, 2) },
		"duration":          func(tl *TimelineResult) { tl.Intervals[0].DurationMs++ },
		"actual_start":      func(tl *TimelineResult) { tl.Intervals[0].ActualStartTs = 1.04 },
		"actual_end":        func(tl *TimelineResult) { tl.Intervals[0].ActualEndTs = 1.96 },
		"actual_duration":   func(tl *TimelineResult) { tl.Intervals[0].ActualDurationMs++ },
		"start_line":        func(tl *TimelineResult) { tl.Intervals[0].StartLine++ },
		"synthetic_tail":    func(tl *TimelineResult) { tl.Intervals[0].EndLine = 0 },
		"wakeup":            func(tl *TimelineResult) { tl.Intervals[0].WakeupLine++ },
		"raw_state":         func(tl *TimelineResult) { tl.Intervals[0].PrevStateRaw = "D" },
		"cpu_zero_known":    func(tl *TimelineResult) { tl.Intervals[0].CPUKnown = true },
		"cpu":               func(tl *TimelineResult) { tl.Intervals[0].CPU = 1 },
		"caller":            func(tl *TimelineResult) { tl.Intervals[0].BlockedReasonCaller = "another" },
		"reason_line":       func(tl *TimelineResult) { tl.Intervals[0].BlockedReasonLine++ },
		"iowait":            func(tl *TimelineResult) { tl.Intervals[0].BlockedReasonIOWait = 1 },
		"iowait_unknown":    func(tl *TimelineResult) { tl.Intervals[0].BlockedReasonIOWaitKnown = false },
		"thread_name":       func(tl *TimelineResult) { tl.Intervals[0].Thread.Comm = "renamed" },
		"tgid":              func(tl *TimelineResult) { tl.Intervals[0].Thread.TGID = 99 },
		"window":            func(tl *TimelineResult) { tl.Window.EndTs = 2.1 },
		"head_absent":       func(tl *TimelineResult) { tl.HeadState = nil },
		"head_status":       func(tl *TimelineResult) { tl.HeadState.Status = "recovered" },
		"head_boundary":     func(tl *TimelineResult) { tl.HeadState.BoundaryTs = 1.01 },
		"head_state":        func(tl *TimelineResult) { tl.HeadState.State = StateRunning },
		"head_actual_start": func(tl *TimelineResult) { tl.HeadState.ActualStartTs = .9 },
		"head_line":         func(tl *TimelineResult) { tl.HeadState.SourceLine = 5 },
		"head_reason":       func(tl *TimelineResult) { tl.HeadState.Reason = "snapshot_incomplete" },
		"zero_width_audit_row": func(tl *TimelineResult) {
			iv := tl.Intervals[0]
			iv.StartTs, iv.EndTs, iv.DurationMs = 2, 2, 0
			tl.Intervals = append(tl.Intervals, iv)
		},
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			tl := b1638b1ClonePartition(base)
			change(&tl)
			got := buildTimelineMeasurementDomain(q, tl)
			if got == nil || got.PartitionID == want.PartitionID {
				t.Fatalf("different typed measurement was lost: got=%+v want=%+v", got, want)
			}
		})
	}
	wording := b1638b1ClonePartition(base)
	wording.Caveats = []string{"different display wording"}
	wording.Intervals[0].Summary = "different summary"
	if got := buildTimelineMeasurementDomain(q, wording); !reflect.DeepEqual(got, want) {
		t.Fatal("prose became a measurement identity input")
	}
	after, _ := json.Marshal(base)
	if !bytes.Equal(before, after) {
		t.Fatal("receipt builder mutated its native partition")
	}
}

func TestB1638B1UnknownInvalidAndCanceledDoNotForgeReceipt(t *testing.T) {
	for name, change := range map[string]func(*Query, *TimelineResult){
		"empty":                func(_ *Query, tl *TimelineResult) { tl.Intervals = nil },
		"unknown_target":       func(_ *Query, tl *TimelineResult) { tl.Thread.PID = 0 },
		"other_target_member":  func(_ *Query, tl *TimelineResult) { tl.Intervals[0].Thread.PID++ },
		"integrity_failure":    func(_ *Query, tl *TimelineResult) { tl.IntegrityFailure = "thread_incarnation_conflict" },
		"nan_interval":         func(_ *Query, tl *TimelineResult) { tl.Intervals[0].EndTs = math.NaN() },
		"nan_head":             func(_ *Query, tl *TimelineResult) { tl.HeadState.BoundaryTs = math.NaN() },
		"infinite_window":      func(_ *Query, tl *TimelineResult) { tl.Window.EndTs = math.Inf(1) },
		"ambiguous_zero_start": func(_ *Query, tl *TimelineResult) { tl.Window.StartTs = 0 },
		"invalid_lines":        func(q *Query, _ *TimelineResult) { q.LineStart, q.LineEnd = 10, 5 },
		"zero_only": func(_ *Query, tl *TimelineResult) {
			tl.Intervals[0].EndTs, tl.Intervals[0].DurationMs = tl.Intervals[0].StartTs, 0
		},
		"canceled": func(q *Query, _ *TimelineResult) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			*q = q.WithRunContext(ctx)
		},
	} {
		t.Run(name, func(t *testing.T) {
			q, tl := Query{}, b1638b1NativePartition()
			change(&q, &tl)
			if got := buildTimelineMeasurementDomain(q, tl); got != nil {
				t.Fatalf("unavailable measurement acquired a constructed receipt: %+v", got)
			}
		})
	}
	zero := b1638b1NativePartition()
	zero.Window.StartTs, zero.Window.StartSet = 0, true
	if got := buildTimelineMeasurementDomain(Query{}, zero); got == nil || got.WindowStartTs != 0 {
		t.Fatal("explicit rebased zero is a legitimate measured window boundary")
	}
}

func TestB1638B1ReceiptCopiesAndLegacyRemainIndependent(t *testing.T) {
	tl := b1638b1NativePartition()
	tl.MeasurementDomain = buildTimelineMeasurementDomain(Query{}, tl)
	account := buildTargetWindowStateAccount(nil, tl, true, tl.Thread, tl.Window, nil)
	copy := cloneTimelineResult(tl)
	if account == nil || account.MeasurementDomain == nil || copy.MeasurementDomain == nil ||
		account.MeasurementDomain == tl.MeasurementDomain || copy.MeasurementDomain == tl.MeasurementDomain ||
		!reflect.DeepEqual(account.MeasurementDomain, tl.MeasurementDomain) || !reflect.DeepEqual(copy.MeasurementDomain, tl.MeasurementDomain) {
		t.Fatal("receipt transfer must be exact and independently owned")
	}
	copy.MeasurementDomain.PartitionID = "changed copy"
	account.MeasurementDomain.PartitionID = "changed account"
	if tl.MeasurementDomain.PartitionID == copy.MeasurementDomain.PartitionID || tl.MeasurementDomain.PartitionID == account.MeasurementDomain.PartitionID {
		t.Fatal("consumer copy modified the cached/native source")
	}
	legacy := b1638b1NativePartition()
	legacyAccount := buildTargetWindowStateAccount(nil, legacy, true, legacy.Thread, legacy.Window, nil)
	if legacyAccount == nil || legacyAccount.MeasurementDomain != nil || cloneTimelineResult(legacy).MeasurementDomain != nil {
		t.Fatal("legacy measured values must stay available without a fabricated receipt")
	}
	body, err := json.Marshal(tl.MeasurementDomain)
	if err != nil {
		t.Fatal(err)
	}
	var round types.TraceSchedulerMeasurementDomain
	if err := json.Unmarshal(body, &round); err != nil || !reflect.DeepEqual(&round, tl.MeasurementDomain) {
		t.Fatalf("JSON changed the receipt: %v", err)
	}
}

func TestB1638B1FullInventoryStreamingAndPhysicalAccountIdentityRemainSeparate(t *testing.T) {
	small := b1638b1NativePartition()
	large := b1638b1NativePartition()
	large.Window.EndTs = 5000
	large.Intervals = make([]Interval, 4096)
	for i := range large.Intervals {
		iv := small.Intervals[0]
		iv.StartTs, iv.EndTs, iv.DurationMs = float64(i+1), float64(i+2), 1000
		iv.ActualStartTs, iv.ActualEndTs, iv.ActualDurationMs = iv.StartTs, iv.EndTs, iv.DurationMs
		large.Intervals[i] = iv
	}
	want := buildTimelineMeasurementDomain(Query{}, large)
	large.Intervals[len(large.Intervals)-1].BlockedReasonIOWait = 1
	if got := buildTimelineMeasurementDomain(Query{}, large); got == nil || got.PartitionID == want.PartitionID {
		t.Fatal("receipt stopped at a display/credential cap before the final native segment")
	}
	smallAllocs := testing.AllocsPerRun(3, func() { _ = buildTimelineMeasurementDomain(Query{}, small) })
	largeAllocs := testing.AllocsPerRun(3, func() { _ = buildTimelineMeasurementDomain(Query{}, large) })
	if largeAllocs > smallAllocs+1 {
		t.Fatalf("streaming hash allocations grew with inventory: small=%.0f large=%.0f", smallAllocs, largeAllocs)
	}
	wider := b1638b1ClonePartition(small)
	wider.Window.EndTs = 3
	if buildTimelineMeasurementDomain(Query{}, small).PartitionID == buildTimelineMeasurementDomain(Query{}, wider).PartitionID {
		t.Fatal("measurement receipt lost its actual query window")
	}
	segments := []foldInterval{{start: 1.1, end: 1.9}}
	physical := stateAccountIdentity(small.Thread, string(StateSSleep), small.Window, segments, 800)
	if physical == "" || physical != stateAccountIdentity(wider.Thread, string(StateSSleep), wider.Window, segments, 800) {
		t.Fatal("separate exact physical account identity must retain cross-window convergence")
	}
}

func TestB1638B1ActualRunKeepsAllStatesBeyondInventoryDisplayCap(t *testing.T) {
	idx := buildTraceIndex(t, "full-partition.ftrace", b1607ManySleepsTrace(39))
	q := Query{View: "thread_timeline", PID: 41, TimeStart: 10, TimeEnd: 10.041,
		TimeStartSet: true, TimeEndSet: true, MinDurationMs: 100, MaxBranches: 1, Limit: 1}
	r := Run(idx, q)
	if r.Timeline == nil || r.TargetWindowStates == nil || r.Timeline.MeasurementDomain == nil {
		t.Fatal("actual native all-state fixture did not publish")
	}
	a := r.TargetWindowStates
	if a.RunningMs <= 0 || a.RunnableMs <= 0 || a.SleepMs <= 0 || a.DStateMs <= 0 || a.IOWaitMs <= 0 ||
		a.SleepInventory == nil || a.SleepInventory.Total != 39 || a.SleepInventory.Emitted != 32 {
		t.Fatalf("fixture must distinguish complete native partition from bounded display: %+v", a)
	}
	if !reflect.DeepEqual(a.MeasurementDomain, r.Timeline.MeasurementDomain) {
		t.Fatal("display-capped account changed its full native partition receipt")
	}
	changed := b1638b1ClonePartition(*r.Timeline)
	changed.Intervals[len(changed.Intervals)-1].PrevStateRaw += "+changed"
	if got := buildTimelineMeasurementDomain(q, changed); got == nil || got.PartitionID == a.MeasurementDomain.PartitionID {
		t.Fatal("actual final native interval was omitted from the receipt")
	}
}

func TestB1638B1ActualRunIntegrityAndUnknownHeadRemainDistinct(t *testing.T) {
	path := b1636HeadlessTrace(t, false)
	idx := buildSchedulerCarryWindow(t, path, 1, 1.1)
	q := b1636Query()
	q.View = "thread_timeline"
	r := Run(idx, q)
	if r.Timeline == nil || r.Timeline.HeadState == nil || r.Timeline.HeadState.Status != "unknown" ||
		r.Timeline.MeasurementDomain == nil || r.Timeline.MeasurementDomain.Status != "constructed_partition" {
		t.Fatalf("unknown head must stay unknown alongside the limited receipt: %+v", r.Timeline)
	}
	conflict := writeSchedulerCarryTrace(t, "reincarnated.ftrace",
		" idle-0 (0) [000] .... 0.700000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=old next_pid=55 next_prio=120",
		" old-55 (55) [000] .... 0.800000: sched_switch: prev_comm=old prev_pid=55 prev_prio=120 prev_state=X ==> next_comm=idle next_pid=0 next_prio=120",
		" creator-7 (7) [000] .... 1.010000: sched_wakeup_new: comm=frag pid=55 prio=120 target_cpu=000",
		" idle-0 (0) [000] .... 1.030000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=frag next_pid=55 next_prio=120",
		" frag-55 (55) [000] .... 1.100000: sched_switch: prev_comm=frag prev_pid=55 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120")
	idx, err := BuildIndex(t.Context(), conflict)
	if err != nil {
		t.Fatal(err)
	}
	q.TimeStart = .7
	r = Run(idx, q)
	if r.Timeline == nil || r.Timeline.IntegrityFailure != "thread_incarnation_conflict" ||
		r.Timeline.MeasurementDomain != nil || r.TargetWindowStates != nil {
		t.Fatalf("integrity failure must not acquire receipt or zero account: %+v", r.Timeline)
	}
}

func TestB1638B1HashFieldCensusRequiresExplicitFutureReview(t *testing.T) {
	// Keep hash-input evolution explicit. Summary/Caveats and the receipt
	// itself are the only deliberately excluded fields of these native shapes.
	for _, tc := range []struct {
		value any
		names string
	}{
		{ThreadRef{}, "Comm PID TGID"},
		{TimelineHeadState{}, "Status BoundaryTs State ActualStartTs SourceLine Reason"},
		{Interval{}, "Thread State StartTs EndTs DurationMs CPU CPUKnown ActualStartTs ActualEndTs ActualDurationMs StartLine EndLine WakeupLine PrevStateRaw Summary BlockedReasonCaller BlockedReasonLine BlockedReasonIOWait BlockedReasonIOWaitKnown"},
	} {
		typeOf := reflect.TypeOf(tc.value)
		var got []string
		for i := 0; i < typeOf.NumField(); i++ {
			got = append(got, typeOf.Field(i).Name)
		}
		if strings.Join(got, " ") != tc.names {
			t.Fatalf("%s changed: explicitly review the measurement hash input set, got %v", typeOf.Name(), got)
		}
	}
}
