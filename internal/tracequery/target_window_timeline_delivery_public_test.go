package tracequery

// Public Run regressions for delivering an already-computed native timeline
// account. No test widens the original query, selects a cause, or reparses prose.
// Existing identity/admission, G12, lifecycle and cancellation pins stay intact.

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
)

func timelineDeliveryJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func timelineDeliveryAccount(t *testing.T, result Result) *TargetWindowStateAccount {
	t.Helper()
	tl, a := result.Timeline, result.TargetWindowStates
	if tl == nil || a == nil {
		t.Fatalf("completed native thread timeline must deliver its existing account: timeline=%+v account=%+v", tl, a)
	}
	if a.Thread != tl.Thread || a.Window != tl.Window ||
		!reflect.DeepEqual(a.MeasurementDomain, tl.MeasurementDomain) {
		t.Fatalf("account must retain the native identity/window/partition receipt: timeline=%+v account=%+v", tl, a)
	}
	if math.Abs(a.WindowMs-(tl.Window.EndTs-tl.Window.StartTs)*1000) > 1e-6 ||
		math.Abs(a.TotalMs-a.RunningMs-a.RunnableMs-a.SleepMs-a.DStateMs-a.IOWaitMs) > 1e-6 {
		t.Fatalf("publication changed the existing disjoint clock accounting: %+v", a)
	}
	if result.RootCauseRank != nil || result.WakeupChain != nil || result.FrameRootCauseBundle != nil || result.WindowStats != nil {
		t.Fatal("timeline delivery must not execute a causal or resource view")
	}
	return a
}

func TestTimelineAccountDeliveryActualUnboundedC2(t *testing.T) {
	raw, err := os.ReadFile("../../eval/fixtures/real_traces/donghu_tieba_frame.systrace")
	if err != nil {
		t.Fatal(err)
	}
	idx := buildTraceIndex(t, "timeline-delivery-c2.systrace", string(raw))
	q := Query{View: "thread_timeline", PID: 59566, Limit: 1, MinDurationMs: 100}
	queryBefore := timelineDeliveryJSON(t, q)
	eventsBefore := timelineDeliveryJSON(t, idx.Events)
	result := Run(idx, q)
	a := timelineDeliveryAccount(t, result)
	if len(result.Timeline.Intervals) <= 12 {
		t.Fatal("the real witness must exceed the ordinary twelve-interval display")
	}
	if a.Thread.PID != 59566 || a.DStateMs != 0 || math.Abs(a.IOWaitMs-0.635) > 1e-6 ||
		a.WaitOccurrenceStatus != "complete" || a.WaitOccurrenceTotal != 3 || len(a.WaitOccurrences) != 3 {
		t.Fatalf("all three native D-opened IO waits must survive the timeline display cap: %+v", a)
	}
	rawDIOCount, lastNativeIndex := 0, -1
	for i, interval := range result.Timeline.Intervals {
		if interval.State == StateIOWait {
			if interval.PrevStateRaw != "D" {
				t.Fatalf("native IO row lost D provenance: %+v", interval)
			}
			rawDIOCount++
			lastNativeIndex = i
		}
	}
	if rawDIOCount != 3 || lastNativeIndex < 12 {
		t.Fatalf("fixture must retain its late third D+IO interval: count=%d last=%d", rawDIOCount, lastNativeIndex)
	}
	for _, row := range a.WaitOccurrences {
		if row.State != StateIOWait || !row.IOWaitKnown || !row.IOWait ||
			row.StartLine <= 0 || row.EndLine <= 0 || row.ReasonLine <= 0 {
			t.Fatalf("published row must retain the original state, marker and line evidence: %+v", row)
		}
	}
	if a.BinderWaitInventory != nil {
		t.Fatal("delivery of an unbounded existing timeline must not start Binder pairing")
	}
	bounded := Run(idx, Query{View: "thread_timeline", PID: 59566,
		TimeStart: 34579.471400, TimeEnd: 34579.471600, TimeStartSet: true, TimeEndSet: true})
	clipped := timelineDeliveryAccount(t, bounded)
	// This narrow query does not include the blocked-reason marker. Preserve
	// the native D-only classification; delivery cannot borrow an IO marker
	// from outside its actual query to relabel the clipped interval.
	if math.Abs(clipped.DStateMs-0.2) > 1e-6 || clipped.IOWaitMs != 0 || clipped.WaitOccurrenceTotal != 1 ||
		len(clipped.WaitOccurrences) != 1 || !clipped.WaitOccurrences[0].WindowClamped || clipped.WaitOccurrences[0].IOWaitKnown {
		t.Fatalf("explicit narrow window must retain its native D ruler without an out-of-window IO marker: %+v", clipped)
	}
	if string(queryBefore) != string(timelineDeliveryJSON(t, q)) ||
		string(eventsBefore) != string(timelineDeliveryJSON(t, idx.Events)) {
		t.Fatal("delivery must not mutate caller endpoints or raw indexed events")
	}
}

func TestTimelineAccountDeliveryEndpointAndViewAdmission(t *testing.T) {
	idx := smrStateAccountIdentityTrace(t)
	for _, tc := range []struct {
		name string
		q    Query
		want bool
	}{
		{"both absent", Query{View: "thread_timeline", PID: 61}, true},
		{"explicit zero start", Query{View: "thread_timeline", PID: 61, TimeStartSet: true, TimeEnd: 1.04, TimeEndSet: true}, true},
		{"legacy bounded", Query{View: "thread_timeline", PID: 61, TimeStart: 1, TimeEnd: 1.04}, true},
		{"missing start", Query{View: "thread_timeline", PID: 61, TimeEnd: 1.04, TimeEndSet: true}, false},
		{"missing end", Query{View: "thread_timeline", PID: 61, TimeStart: 1, TimeStartSet: true}, false},
		{"zero width", Query{View: "thread_timeline", PID: 61, TimeStart: 1.02, TimeEnd: 1.02, TimeStartSet: true, TimeEndSet: true}, false},
		{"reversed", Query{View: "thread_timeline", PID: 61, TimeStart: 1.04, TimeEnd: 1.02}, false},
		{"line-only", Query{View: "thread_timeline", PID: 61, LineStart: 2, LineEnd: 6}, false},
		{"no target", Query{View: "thread_timeline"}, false},
		{"absent target", Query{View: "thread_timeline", PID: 999}, false},
		{"process is not a thread", Query{View: "thread_timeline", PID: 61, TargetScope: TargetScopeProcess}, false},
		{"event cursor unchanged", Query{View: "event_search", PID: 61, Pattern: "sched_switch"}, false},
		{"span view unchanged", Query{View: "span_window", PID: 61, SpanName: "VerifyClass RenamedClass"}, false},
		{"whole stats unchanged", Query{View: "window_stats", PID: 61}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := tc.q
			got := Run(idx, tc.q)
			if (got.TargetWindowStates != nil) != tc.want {
				t.Fatalf("state-account admission changed outside its precise path: query=%+v account=%+v", tc.q, got.TargetWindowStates)
			}
			if !reflect.DeepEqual(before, tc.q) {
				t.Fatal("account publication rewrote caller scope")
			}
		})
	}
}

func TestTimelineAccountDeliveryActualWindowAndResolvedRename(t *testing.T) {
	idx := smrStateAccountIdentityTrace(t)
	for _, tc := range []struct {
		name string
		q    Query
	}{
		{"unique old name", Query{View: "thread_timeline", Thread: "oldname"}},
		{"unique accepted span", Query{View: "thread_timeline", PID: 61, SpanName: "VerifyClass RenamedClass"}},
		{"bounded reused timeline", Query{View: "thread_timeline", PID: 61, TimeStart: 1, TimeEnd: 1.04, TimeStartSet: true, TimeEndSet: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := Run(idx, tc.q)
			a := timelineDeliveryAccount(t, r)
			if a.Thread.PID != 61 {
				t.Fatalf("resolved TID was replaced by a comm cursor: %+v", a.Thread)
			}
			if tc.q.SpanName != "" {
				if math.Abs(a.Window.StartTs-1.022) > 1e-9 || math.Abs(a.Window.EndTs-1.028) > 1e-9 {
					t.Fatalf("account must consume the already-derived span window, not index bounds: %+v", a.Window)
				}
			} else if math.Abs(a.SleepIOWaitMs-10) > 1e-6 {
				t.Fatalf("S+iowait refinement must retain renamed TID 61: %+v", a)
			}
			if queryBoundedTimeStart(tc.q) && queryBoundedTimeEnd(tc.q) {
				if a.BinderWaitInventory == nil {
					t.Fatal("bounded timeline must retain its pre-existing Binder enrichment")
				}
				if a.TotalMs >= a.WindowMs {
					t.Fatal("physical artifact tail must not be extrapolated to the requested end")
				}
			} else if a.BinderWaitInventory != nil {
				t.Fatal("new timeline-only admission must not assess Binder closure")
			}
		})
	}
}

func TestTimelineAccountDeliveryKeepsSIOAndBoundedInventory(t *testing.T) {
	t.Run("S marker remains interruptible", func(t *testing.T) {
		r := Run(g12PlatformTrace(t), Query{View: "thread_timeline", PID: 562})
		a := timelineDeliveryAccount(t, r)
		if a.SleepIOWaitMs <= 0 || a.SleepIOWaitMs > a.SleepMs+1e-6 ||
			a.DStateMs != 0 || a.IOWaitMs != 0 {
			t.Fatalf("S+iowait must remain a refinement inside ordinary sleep: %+v", a)
		}
		for _, row := range a.WaitOccurrences {
			if row.State != StateSSleep {
				t.Fatalf("S+iowait was promoted into the D/IO bucket: %+v", row)
			}
		}
	})
	t.Run("capped roster preserves full totals", func(t *testing.T) {
		idx := buildTraceIndex(t, "timeline-delivery-many.ftrace", b1607ManySleepsTrace(60))
		r := Run(idx, Query{View: "thread_timeline", PID: 41, Limit: 1, MinDurationMs: 100, MaxBranches: 1})
		a := timelineDeliveryAccount(t, r)
		if a.WaitOccurrenceTotal != 40 || len(a.WaitOccurrences) != targetWindowWaitOccurrenceCap ||
			a.WaitOccurrenceStatus != "incomplete" || a.RunningMs <= 0 || a.RunnableMs <= 0 || a.SleepMs <= 0 || a.DStateMs <= 0 || a.IOWaitMs <= 0 {
			t.Fatalf("presentation budgets must not truncate the native partition or claim a complete wait roster: %+v", a)
		}
		if a.BinderWaitInventory != nil {
			t.Fatal("large native timeline publication must not add Binder scans")
		}
	})
	t.Run("measured no waits differs from absent", func(t *testing.T) {
		idx := buildTraceIndex(t, "timeline-delivery-zero.ftrace",
			"idle-0 (0) [000] .... 1.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=41 next_prio=20\n"+
				"app-41 (41) [000] .... 1.010000: sched_switch: prev_comm=app prev_pid=41 prev_prio=20 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120\n")
		a := timelineDeliveryAccount(t, Run(idx, Query{View: "thread_timeline", PID: 41}))
		if a.WaitOccurrenceStatus != "complete" || a.WaitOccurrenceTotal != 0 || a.RunningMs <= 0 {
			t.Fatalf("a measured running partition can report no observed waits: %+v", a)
		}
		if got := Run(idx, Query{View: "thread_timeline", PID: 900}).TargetWindowStates; got != nil {
			t.Fatalf("an absent target cannot borrow the measured zero: %+v", got)
		}
	})
	t.Run("empty normalized range is not rescued by inventing zero", func(t *testing.T) {
		idx := buildTraceIndex(t, "timeline-delivery-rebased.ftrace",
			"idle-0 (0) [000] .... 0.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=41 next_prio=20\n"+
				"app-41 (41) [000] .... 0.010000: sched_switch: prev_comm=app prev_pid=41 prev_prio=20 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120\n")
		q := Query{View: "thread_timeline", PID: 41}
		r := Run(idx, q)
		// The existing index uses zero as its unset first timestamp and
		// normalizes this two-event fixture to a zero-width .01.. .01 range.
		// Fixing that producer is separate; this delivery lane must preserve
		// the empty native result, not invent a complete 0.. .01 partition.
		if r.Timeline == nil || len(r.Timeline.Intervals) != 0 || r.TargetWindowStates != nil || q.TimeStartSet || q.TimeEndSet {
			t.Fatalf("empty native result must not be promoted into a zero-based account: query=%+v timeline=%+v account=%+v", q, r.Timeline, r.TargetWindowStates)
		}
	})
}

func TestTimelineAccountDeliveryFailsClosedOnIdentityAndIntegrity(t *testing.T) {
	t.Run("same comm different TIDs", func(t *testing.T) {
		trace := "idle-0 (0) [000] .... 1.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=shared next_pid=41 next_prio=20\n" +
			"shared-41 (41) [000] .... 1.010000: sched_switch: prev_comm=shared prev_pid=41 prev_prio=20 prev_state=S ==> next_comm=shared next_pid=42 next_prio=20\n" +
			"shared-42 (42) [000] .... 1.020000: sched_switch: prev_comm=shared prev_pid=42 prev_prio=20 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120\n"
		idx := buildTraceIndex(t, "timeline-delivery-ambiguous.ftrace", trace)
		if r := Run(idx, Query{View: "thread_timeline", Thread: "shared"}); r.TargetWindowStates != nil ||
			r.Timeline == nil || len(r.Timeline.Intervals) != 0 {
			t.Fatalf("ambiguous comm must not select or merge two targets: %+v", r)
		}
		for _, pid := range []int{41, 42} {
			a := timelineDeliveryAccount(t, Run(idx, Query{View: "thread_timeline", PID: pid, Thread: "shared"}))
			if a.Thread.PID != pid {
				t.Fatalf("exact TID changed: want=%d got=%+v", pid, a.Thread)
			}
		}
	})
	t.Run("same TID new incarnation", func(t *testing.T) {
		trace := "idle-0 (0) [000] .... 1.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=old next_pid=41 next_prio=20\n" +
			"old-41 (41) [000] .... 1.010000: sched_switch: prev_comm=old prev_pid=41 prev_prio=20 prev_state=X ==> next_comm=idle next_pid=0 next_prio=120\n" +
			"creator-7 (7) [000] .... 1.020000: sched_wakeup_new: comm=new pid=41 prio=20 target_cpu=000\n" +
			"idle-0 (0) [000] .... 1.030000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=new next_pid=41 next_prio=20\n" +
			"new-41 (41) [000] .... 1.040000: sched_switch: prev_comm=new prev_pid=41 prev_prio=20 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120\n"
		r := Run(buildTraceIndex(t, "timeline-delivery-incarnation.ftrace", trace), Query{View: "thread_timeline", PID: 41})
		if r.Timeline == nil || r.Timeline.IntegrityFailure != "thread_incarnation_conflict" ||
			len(r.Timeline.Intervals) != 0 || r.TargetWindowStates != nil || len(r.LifecycleSuppressions) == 0 {
			t.Fatalf("lifecycle failure must stay disclosed, never rebuilt as zero: %+v", r)
		}
	})
	t.Run("malformed scheduler row", func(t *testing.T) {
		lines := schedulerIntegrityFixture("app-20 (20) [000] .... 1.100000: sched_switch: prev_comm=app prev_pid=20 prev_prio=20 prev_state=S ==> next_comm=idle/0 next_prio=120")
		idx := buildTraceIndex(t, "timeline-delivery-malformed.ftrace", strings.Join(lines, "\n")+"\n")
		r := Run(idx, Query{View: "thread_timeline", PID: 20})
		if r.Timeline == nil || r.Timeline.IntegrityFailure == "" ||
			len(r.Timeline.Intervals) != 0 || r.TargetWindowStates != nil {
			t.Fatalf("incomplete scheduler input must not yield a complete zero account: %+v", r)
		}
	})
}

func TestTimelineAccountDeliveryPreservesUnknownHeadAndCancellation(t *testing.T) {
	path := b1636HeadlessTrace(t, false)
	idx := buildSchedulerCarryWindow(t, path, 1, 1.1)
	q := b1636Query()
	q.View = "thread_timeline"
	r := Run(idx, q)
	a := timelineDeliveryAccount(t, r)
	if r.Timeline.HeadState == nil || r.Timeline.HeadState.Status != "unknown" ||
		a.SleepInventory == nil || a.SleepInventory.HeadState == nil || a.SleepInventory.HeadState.Status != "unknown" {
		t.Fatalf("reuse cannot upgrade an unknown scheduler head: timeline=%+v account=%+v", r.Timeline, a)
	}
	plainIndex := runCancelIndex(t)
	plainQuery := Query{View: "thread_timeline", PID: 200}
	plain := Run(plainIndex, plainQuery)
	timelineDeliveryAccount(t, plain)
	armed, stop := context.WithCancel(context.Background())
	defer stop()
	if string(timelineDeliveryJSON(t, plain)) != string(timelineDeliveryJSON(t, Run(plainIndex, plainQuery.WithRunContext(armed)))) {
		t.Fatal("an armed but untriggered context changed timeline delivery")
	}
	dead, cancel := context.WithCancel(context.Background())
	cancel()
	canceled := Run(plainIndex, plainQuery.WithRunContext(dead))
	if canceled.ViewCancellation == nil || canceled.Timeline != nil || canceled.TargetWindowStates != nil {
		t.Fatalf("cancellation must not reconstruct discarded timeline/account data: %+v", canceled)
	}
}

func TestTimelineAccountDeliveryReuseAndBinderCallShape(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "query.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var run *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "Run" {
			run = fn
		}
	}
	if run == nil {
		t.Fatal("Run missing")
	}
	reuse, binderOldAdmission := false, false
	mints := 0
	ast.Inspect(run.Body, func(node ast.Node) bool {
		if call, ok := node.(*ast.CallExpr); ok && smrCalledName(call) == "buildTargetWindowStateAccount" {
			mints++
		}
		stmt, ok := node.(*ast.IfStmt)
		if !ok {
			return true
		}
		if otherwise, ok := stmt.Else.(*ast.BlockStmt); ok && smrContainsCall(otherwise, "targetWindowTimeline") {
			hasNativeCopy := false
			ast.Inspect(stmt.Body, func(n ast.Node) bool {
				if ptr, ok := n.(*ast.StarExpr); ok {
					if selector, ok := ptr.X.(*ast.SelectorExpr); ok && selector.Sel.Name == "Timeline" {
						if base, ok := selector.X.(*ast.Ident); ok && base.Name == "res" {
							hasNativeCopy = true
						}
					}
				}
				return true
			})
			reuse = reuse || hasNativeCopy && !smrContainsCall(stmt.Body, "targetWindowTimeline") &&
				!smrContainsCall(stmt.Body, "ThreadTimeline")
		}
		if smrContainsCall(stmt.Body, "buildTargetWindowBinderWaitInventory") {
			ast.Inspect(stmt.Cond, func(n ast.Node) bool {
				if neg, ok := n.(*ast.UnaryExpr); ok && neg.Op == token.NOT {
					if id, ok := neg.X.(*ast.Ident); ok && id.Name == "returnedTimelineAccount" {
						binderOldAdmission = true
					}
				}
				return true
			})
		}
		return true
	})
	if mints != 1 || !reuse || !binderOldAdmission {
		t.Fatalf("one mint, native reuse and old-only Binder enrichment required: mints=%d reuse=%t binderOldAdmission=%t", mints, reuse, binderOldAdmission)
	}
}
