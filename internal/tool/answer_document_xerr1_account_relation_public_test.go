package tool

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// This is generated raw scheduler input, not a hand-assembled projection.
// Both public views and the answer renderer consume the same captured file.
func xerr1PublicDiagnosticTrace(mixed bool) string {
	type event struct {
		ts   float64
		line string
	}
	var events []event
	add := func(ts float64, cpu, pid int, comm, payload string) {
		events = append(events, event{ts, fmt.Sprintf("%s-%d (%d) [%03d] .... %.6f: %s", comm, pid, pid, cpu, ts, payload)})
	}
	switchIn := func(ts float64, cpu, pid int, comm string) {
		add(ts, cpu, 0, "idle", fmt.Sprintf("sched_switch: prev_comm=idle/%d prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=%s next_pid=%d next_prio=120", cpu, comm, pid))
	}
	switchOut := func(ts float64, cpu, pid int, comm, state string) {
		add(ts, cpu, pid, comm, fmt.Sprintf("sched_switch: prev_comm=%s prev_pid=%d prev_prio=120 prev_state=%s ==> next_comm=idle/%d next_pid=0 next_prio=120", comm, pid, state, cpu))
	}
	wake := func(ts float64, cpu, pid int, comm string) {
		add(ts, 3, 999, "waker", fmt.Sprintf("sched_wakeup: comm=%s pid=%d prio=120 target_cpu=%03d", comm, pid, cpu))
	}
	switchIn(1, 0, 100, "app")
	switchOut(1.010, 0, 100, "app", "S")
	wake(1.020, 1, 100, "app")
	switchIn(1.020, 1, 100, "app")
	if mixed {
		add(1.021, 1, 100, "app", "print: B|100|H:TraversalWait frame")
		switchOut(1.022, 1, 100, "app", "S")
		wake(1.026, 1, 100, "app")
		switchIn(1.026, 1, 100, "app")
		switchOut(1.030, 1, 100, "app", "D")
	} else {
		add(1.029, 1, 100, "app", "print: B|100|H:TraversalWait frame")
		switchOut(1.030, 1, 100, "app", "S")
		// Seven other (thread,CPU) sleep buckets outrank app's CPU1 5ms.
		// app's CPU0 20ms remains as the eighth SleepTop bucket. Its two
		// physical members flank the omitted CPU1 member; the hull encloses it.
		for i := 0; i < 7; i++ {
			ts := 1.001 + float64(i)*.0002
			pid, comm := 200+i, fmt.Sprintf("filler%d", i)
			switchIn(ts, 2, pid, comm)
			switchOut(ts+.0001, 2, pid, comm, "S")
			wake(1.100+float64(i)*.0002, 2, pid, comm)
			switchIn(1.100+float64(i)*.0002, 2, pid, comm)
			switchOut(1.100+float64(i)*.0002+.0001, 2, pid, comm, "X")
		}
	}
	wake(1.035, 0, 100, "app")
	switchIn(1.035, 0, 100, "app")
	add(1.036, 0, 100, "app", "print: E|100")
	switchOut(1.040, 0, 100, "app", "S")
	wake(1.050, 0, 100, "app")
	switchIn(1.050, 0, 100, "app")
	// This real 150ms running segment exceeds 70% of the 200ms window:
	// the churn producer correctly does not replace SleepTop with a full
	// dominant-state account. It also establishes artifact coverage to end.
	add(1.200, 0, 100, "app", "print: C|100|end|1")
	sort.SliceStable(events, func(i, j int) bool { return events[i].ts < events[j].ts })
	var out strings.Builder
	out.WriteString("# tracer: nop\n")
	for _, e := range events {
		out.WriteString(e.line)
		out.WriteByte('\n')
	}
	return out.String()
}

func xerr1PublicDiagnosticQuery(t *testing.T, mixed bool) (types.ToolResult, tracequery.Result, tracequery.Result) {
	t.Helper()
	ctx, path := businessRefTestContext(t, xerr1PublicDiagnosticTrace(mixed))
	result := businessRefTestQuery(t, ctx, map[string]any{
		"source": "path", "path": path, "view": "recipe", "recipe_name": "io_wait", "pid": 100,
		"time_start": 1, "time_end": 1.2, "limit": 32, "trace_flavor": "generic_ftrace",
	})
	native := businessSpanSchedulerPublicPayload(t, result)
	timeline := businessRefTestQuery(t, ctx, map[string]any{
		"source": "path", "path": path, "view": "thread_timeline", "pid": 100,
		"time_start": 1, "time_end": 1.2, "trace_flavor": "generic_ftrace",
	})
	return result, native, businessSpanSchedulerPublicPayload(t, timeline)
}

func xerr1PublicDiagnosticPair(t *testing.T, result types.ToolResult, native tracequery.Result) (types.TraceCausalProjection, *tracequery.CriticalBlockingCandidate, tracequery.RootCauseRankItem) {
	t.Helper()
	if native.WindowStats == nil || native.RootCauseRank == nil || native.CriticalBlocking == nil {
		t.Fatalf("public recipe omitted required native faces: %+v", native)
	}
	var blocking *tracequery.CriticalBlockingCandidate
	for i := range native.CriticalBlocking.Items {
		item := &native.CriticalBlocking.Items[i]
		if item.Type == "blocking_span" && item.Thread.PID == 100 {
			blocking = item
		}
	}
	var sleep tracequery.RootCauseRankItem
	count := 0
	for _, item := range native.RootCauseRank.Items {
		if item.Type == "sleep_wait" && item.Thread.PID == 100 {
			sleep, count = item, count+1
		}
	}
	if blocking == nil || blocking.BlockingValueBasis != tracequery.BlockingValueBasisWaitSegments || blocking.BlockingKind != "" || count != 1 {
		for _, item := range native.RootCauseRank.Items {
			t.Logf("native rank: %s/%d %s %.3fms source=%s", item.Thread.Comm, item.Thread.PID, item.Type, item.ImpactMs, item.Source)
		}
		t.Fatalf("public producer did not retain one payloadless blocking + sleep seat: blocking=%+v count=%d", blocking, count)
	}
	if sleep.StartTs > blocking.StartTs || sleep.EndTs < blocking.EndTs {
		t.Fatalf("fixture does not witness containing hulls: sleep=%+v blocking=%+v", sleep, blocking)
	}
	projection := types.TraceCausalProjectionFromObservationRecords(result.Observations)
	t.Logf("public producer: sleep=%.3fms source=%s hull=%.6f..%.6f state_account_key=%q; blocking=%.3fms S=%.3f D=%.3f IO=%.3f interval=%.6f..%.6f; both selected window=1.000000..1.200000", sleep.SleepMs, sleep.Source, sleep.StartTs, sleep.EndTs, sleep.StateAccountKey, blocking.DurationMs, blocking.WaitSleepMs, blocking.WaitDStateMs, blocking.WaitIOWaitMs, blocking.StartTs, blocking.EndTs)
	return projection, blocking, sleep
}

func TestXERR1PublicSleepTopHoleMustNotClaimPhysicalContainment(t *testing.T) {
	result, native, timeline := xerr1PublicDiagnosticQuery(t, false)
	projection, blocking, sleep := xerr1PublicDiagnosticPair(t, result, native)
	if len(native.WindowStats.SleepTop) != 8 || math.Abs(sleep.SleepMs-20) > 1e-6 || math.Abs(blocking.WaitSleepMs-5) > 1e-6 {
		t.Fatalf("top8 producer witness drifted: top=%+v sleep=%+v blocking=%+v", native.WindowStats.SleepTop, sleep, blocking)
	}
	appBuckets := 0
	for _, bucket := range native.WindowStats.SleepTop {
		if bucket.Thread.PID == 100 {
			appBuckets++
			if bucket.CPU != 0 || math.Abs(bucket.DurationMs-20) > 1e-6 {
				t.Fatalf("CPU1 middle sleep was not omitted: %+v", bucket)
			}
		}
	}
	if appBuckets != 1 || timeline.Timeline == nil {
		t.Fatalf("missing retained CPU0 bucket or public timeline: buckets=%d timeline=%+v", appBuckets, timeline.Timeline)
	}
	var sleepIntervals []tracequery.Interval
	for _, interval := range timeline.Timeline.Intervals {
		if interval.State == tracequery.StateSSleep {
			sleepIntervals = append(sleepIntervals, interval)
		}
	}
	if len(sleepIntervals) != 3 || math.Abs(sleepIntervals[0].StartTs-1.010) > 1e-9 || math.Abs(sleepIntervals[0].EndTs-1.020) > 1e-9 || math.Abs(sleepIntervals[1].StartTs-1.030) > 1e-9 || math.Abs(sleepIntervals[1].EndTs-1.035) > 1e-9 || math.Abs(sleepIntervals[2].StartTs-1.040) > 1e-9 || math.Abs(sleepIntervals[2].EndTs-1.050) > 1e-9 {
		t.Fatalf("public exact timeline does not prove the hole: %+v", sleepIntervals)
	}
	t.Log("public SleepTop has 8 buckets: retained app CPU0 members [1.010,1.020]+[1.040,1.050]; omitted app CPU1 [1.030,1.035]; actual timeline confirms retained∩blocking sleep=0ms although hull contains blocking interval")
	xerr1PublicDiagnosticRender(t, result, projection, false)
}

func TestXERR1PublicMixedWaitMustNotCallAllWaitSleepTime(t *testing.T) {
	result, native, timeline := xerr1PublicDiagnosticQuery(t, true)
	projection, blocking, sleep := xerr1PublicDiagnosticPair(t, result, native)
	if math.Abs(blocking.WaitSleepMs-4) > 1e-6 || math.Abs(blocking.WaitDStateMs-5) > 1e-6 || math.Abs(blocking.DurationMs-9) > 1e-6 || math.Abs(sleep.SleepMs-24) > 1e-6 || timeline.Timeline == nil {
		t.Fatalf("public mixed S/D witness drifted: blocking=%+v sleep=%+v timeline=%+v", blocking, sleep, timeline.Timeline)
	}
	dFound := false
	for _, interval := range timeline.Timeline.Intervals {
		if interval.State == tracequery.StateDSleep && math.Abs(interval.StartTs-1.030) < 1e-9 && math.Abs(interval.EndTs-1.035) < 1e-9 {
			dFound = true
		}
	}
	if !dFound {
		t.Fatal("actual public timeline did not retain the disjoint D-state wait member")
	}
	t.Log("actual public timeline proves S[1.022,1.026] + D[1.030,1.035] = blocking 9ms; only the 4ms S component belongs to the 24ms sleep seat")
	xerr1PublicDiagnosticRender(t, result, projection, true)
}

func xerr1PublicDiagnosticRender(t *testing.T, result types.ToolResult, projection types.TraceCausalProjection, mixed bool) {
	t.Helper()
	before, _ := json.Marshal(result.Observations)
	projectionBefore, _ := json.Marshal(projection)
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), lang == "zh")
			var paired []*runtimeTraceProjTreeRow
			var blockingRef, sleepRef string
			for _, row := range runtimeTraceProjSMR1AllRows(&model) {
				if row.BlockingWaitSleepRef != "" || row.BlockingWaitSleepPeerRef != "" {
					paired = append(paired, row)
				}
				if row.BlockingWaitSleepRef != "" {
					blockingRef, sleepRef = row.EvidenceTag, row.BlockingWaitSleepRef
				}
			}
			if len(paired) != 2 || blockingRef == "" || sleepRef == "" {
				t.Fatalf("production account navigation lost its two evidence references: %+v", paired)
			}
			face := p3mRenderUserFace(t, result.Observations, lang)
			for _, row := range paired {
				wantValue := 20.0
				if mixed {
					wantValue = 24
				}
				if row.Node.TypeToken == "blocking_span" {
					wantValue = 5
					if mixed {
						wantValue = 9
					}
				} else if row.BlockingWaitSleepPeerRef != blockingRef {
					t.Errorf("reverse reference no longer points to the blocking row: %q", row.BlockingWaitSleepPeerRef)
				}
				if math.Abs(row.Node.ImpactMS-wantValue) > 1e-6 {
					t.Errorf("display changed %s's measured value: %.3f", row.EvidenceTag, row.Node.ImpactMS)
				}
				found := false
				for _, line := range strings.Split(face, "\n") {
					if strings.Contains(line, "["+row.EvidenceTag+"]") && strings.Contains(line, fmt.Sprintf("%.3fms", wantValue)) {
						found = true
					}
				}
				if !found {
					t.Errorf("final renderer lost value %.3fms beside its own [%s] reference", wantValue, row.EvidenceTag)
				}
			}
			want := []string{
				"等待账目对照:本行含已测睡眠分量;同线程睡眠统计见[" + sleepRef + "];实际分量关系未证,不能直接相加",
				"等待账目对照:同线程阻塞等待[" + blockingRef + "]按睡眠+D态+IO等待计量;实际分量关系未证,不能直接相加",
			}
			if lang == "en" {
				want = []string{
					"wait-account comparison: this wait includes measured sleep; see [" + sleepRef + "] for a sleep measurement of the same thread; actual component relation unproven, do not add directly",
					"wait-account comparison: [" + blockingRef + "] measures the same thread's sleep+D-state+IO wait; actual component relation unproven, do not add directly",
				}
			}
			for _, wording := range want {
				if !strings.Contains(xerr1CollapseFence(face), xerr1CollapseFence(wording)) {
					t.Errorf("final renderer lost neutral account navigation %q", wording)
				}
			}
			for _, wording := range []string{"physically inside", "同段物理时间", "wait segments fall inside this seat's physical time", "sleep 席", "sleep seat"} {
				if strings.Contains(xerr1CollapseFence(face), xerr1CollapseFence(wording)) {
					t.Errorf("PUBLIC FALSE PHYSICAL RELATION: %q", wording)
				}
			}
		})
	}
	after, _ := json.Marshal(result.Observations)
	projectionAfter, _ := json.Marshal(projection)
	if string(before) != string(after) || string(projectionBefore) != string(projectionAfter) {
		t.Fatal("render mutated producer values, evidence references, or projection authority")
	}
}
