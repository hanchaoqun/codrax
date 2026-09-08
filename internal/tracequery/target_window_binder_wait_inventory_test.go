package tracequery

import (
	"context"
	"fmt"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
)

func b1607BinderInventory(t *testing.T, result Result) *TargetWindowBinderWaitInventory {
	t.Helper()
	account := b1607InventoryResultAccount(t, result)
	if account.BinderWaitInventory == nil {
		t.Fatal("actual Run omitted the independent verified Binder wait inventory")
	}
	got := account.BinderWaitInventory
	if got.TargetSleepCount != got.ConfirmedCount+got.UnresolvedCandidateCount+got.RemainingUnassociatedCount {
		t.Fatalf("sleep-interval categories do not partition the constructed input: %+v", got)
	}
	if got.Scope != "indexed_target_verified_closed_waits" || got.CausalAttributionStatus != "not_assessed" {
		t.Fatalf("verified wait accounting must not claim capture completeness or root-cause authority: %+v", got)
	}
	return got
}

func TestB1607BActualRunClosesBinderOutsideChainBudgets(t *testing.T) {
	idx := buildTraceIndex(t, "closed-binder.ftrace", donghuP9TrueBinderWaitTrace)
	for _, view := range []string{"window_stats", "wakeup_chain", "root_cause_rank", "frame_root_cause_bundle"} {
		t.Run(view, func(t *testing.T) {
			q := Query{View: view, PID: 17267, TimeStart: 13762.8355, TimeEnd: 13762.8375, MinDurationMs: 100, MaxBranches: 1, MaxDepth: 1, MaxChainNodes: 1, Limit: 1}
			result := Run(idx, q)
			got := b1607BinderInventory(t, result)
			if got.ScanStatus != "complete" || got.ConfirmedCount != 1 || got.Emitted != 1 || math.Abs(got.ConfirmedMs-1.409) > 1e-6 {
				t.Fatalf("strict closed wait lost to chain selection: %+v", got)
			}
			row := got.Occurrences[0]
			if row.RequestTransactionID != 12145859 || row.ReplyTransactionID != 12145860 || row.ClosureTs != 13762.837270 || row.ReplyReceiveTs <= row.ClosureTs {
				t.Fatalf("request/reply IDs and actual post-wake receipt must remain distinct: %+v", row)
			}
			if result.WakeupChain != nil {
				chain := BuildWakeupChain(idx, q)
				stampStateAccountPublicationKeys(&chain, nil)
				if !reflect.DeepEqual(*result.WakeupChain, chain) {
					t.Fatal("inventory changed original causal chain or its root evidence")
				}
			}
		})
	}
}

func TestB1607BActualRunH1FivePhysicalClosures(t *testing.T) {
	content, err := os.ReadFile("../../eval/fixtures/real_traces/donghu.ftrace")
	if err != nil {
		t.Fatal(err)
	}
	idx := buildTraceIndex(t, "h1-binder-census.ftrace", string(content))
	q := Query{View: "window_stats", PID: 17267, TimeStart: 13762.812, TimeEnd: 13763.092, MinDurationMs: 100, MaxBranches: 1, MaxDepth: 1, Limit: 1}
	got := b1607BinderInventory(t, Run(idx, q))
	if got.ConfirmedCount != 5 || math.Abs(got.ConfirmedMs-3.094) > 1e-6 {
		t.Fatalf("five physical closed waits, not the selected 1.409ms chain donor, own this account: %+v", got)
	}
	want := []float64{1.409, .924, .068, .120, .573}
	for i, row := range got.Occurrences {
		if math.Abs(row.DurationMs-want[i]) > 1e-6 {
			t.Fatalf("physical interval %d: %+v", i, row)
		}
	}
}

func TestB1607BActualRunClippedWaitKeepsPhysicalClosure(t *testing.T) {
	for _, state := range []string{"S", "D"} {
		t.Run(state, func(t *testing.T) {
			idx := buildTraceIndex(t, "binder-clipped.ftrace", strings.Replace(donghuP9TrueBinderWaitTrace, "prev_state=S", "prev_state="+state, 1))
			q := Query{View: "window_stats", PID: 17267, TimeStart: 13762.836, TimeEnd: 13762.837}
			got := b1607BinderInventory(t, Run(idx, q))
			if got.ConfirmedCount != 1 || math.Abs(got.ConfirmedMs-1) > 1e-6 || !got.Occurrences[0].WindowClamped() || got.Occurrences[0].ClosureTs != 13762.837270 {
				t.Fatalf("window clipping must not become either a synthetic wake or a lost physical closure: %+v", got)
			}
		})
	}
}

func b1607DropTraceLine(trace, marker string) string {
	var lines []string
	for _, line := range strings.Split(trace, "\n") {
		if !strings.Contains(line, marker) {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func TestB1607BActualRunNeverPromotesIncompleteOrOtherMechanisms(t *testing.T) {
	for _, tc := range []struct {
		name, trace string
		unresolved  bool
	}{
		{"oneway", strings.Replace(donghuP9TrueBinderWaitTrace, "flags=0x10", "flags=0x11", 1), false},
		{"unknown flags", strings.Replace(donghuP9TrueBinderWaitTrace, "flags=0x10", "flags=invalid", 1), true},
		{"missing request receive", b1607DropTraceLine(donghuP9TrueBinderWaitTrace, "binder_transaction_received: transaction=12145859"), true},
		{"missing reply send", b1607DropTraceLine(donghuP9TrueBinderWaitTrace, "binder_transaction: transaction=12145860"), true},
		{"missing reply receive", b1607DropTraceLine(donghuP9TrueBinderWaitTrace, "binder_transaction_received: transaction=12145860"), true},
		{"missing physical wake", b1607DropTraceLine(donghuP9TrueBinderWaitTrace, "13762.837270: sched_wakeup"), true},
		{"only waking", strings.Replace(donghuP9TrueBinderWaitTrace, "13762.837270: sched_wakeup", "13762.837270: sched_waking", 1), true},
		{"wrong wake peer", strings.Replace(donghuP9TrueBinderWaitTrace, "binder:496_9-10961 ( 9743) [004] .... 13762.837270", "other-99999 ( 9743) [004] .... 13762.837270", 1), true},
		{"wrong reply target", strings.Replace(donghuP9TrueBinderWaitTrace, "dest_proc=17267 dest_thread=17267 reply=1", "dest_proc=17267 dest_thread=99999 reply=1", 1), true},
		{"namespace hint mismatch", strings.Replace(donghuP9TrueBinderWaitTrace, "dest_proc=9743 dest_thread=0", "dest_proc=496 dest_thread=0", 1), true},
		{"wrong received peer", strings.Replace(donghuP9TrueBinderWaitTrace, "binder:496_9-10961 ( 9743) [004] .... 13762.835943", "other-99999 ( 9743) [004] .... 13762.835943", 1), true},
		{"same timestamp two wakes", strings.Replace(donghuP9TrueBinderWaitTrace, " .ugc.aweme.lite-17267 (17267) [004] .... 13762.837301", " other-99999 (9743) [004] .... 13762.837270: sched_wakeup: comm=.ugc.aweme.lite pid=17267 prio=53 target_cpu=004\n .ugc.aweme.lite-17267 (17267) [004] .... 13762.837301", 1), true},
		{"earlier target wake", strings.Replace(donghuP9TrueBinderWaitTrace, "    binder:496_9-10961 ( 9743) [004] .... 13762.837261", " other-99999 (9743) [004] .... 13762.836500: sched_wakeup: comm=.ugc.aweme.lite pid=17267 prio=53 target_cpu=004\n    binder:496_9-10961 ( 9743) [004] .... 13762.837261", 1), true},
		{"malformed hidden concurrent request", strings.Replace(donghuP9TrueBinderWaitTrace, " .ugc.aweme.lite-17267 (17267) [004] .... 13762.835834", " .ugc.aweme.lite-17267 (17267) [004] .... 13762.835820: binder_transaction: transaction=bad dest_proc=9743 dest_thread=0 reply=0 flags=0x10 code=0x19\n .ugc.aweme.lite-17267 (17267) [004] .... 13762.835834", 1), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			idx := buildTraceIndex(t, "unresolved.ftrace", tc.trace)
			q := Query{View: "window_stats", PID: 17267, TimeStart: 13762.8355, TimeEnd: 13762.8375}
			got := b1607BinderInventory(t, Run(idx, q))
			if got.ConfirmedCount != 0 || got.ConfirmedMs != 0 || len(got.Occurrences) != 0 || (tc.unresolved && got.UnresolvedCandidateCount == 0) {
				t.Fatalf("incomplete or unrelated observation was promoted: %+v", got)
			}
		})
	}
}

func b1607RepeatedClosedBinderTrace(count int) string {
	var out strings.Builder
	for i := 0; i < count; i++ {
		start := 10.001 + float64(i)*.001
		state := "S"
		if i%3 != 0 {
			state = "D"
		}
		// Deliberately reuse IDs in fully closed sequential cohorts.
		fmt.Fprintf(&out, "target-41 (41) [000] .... %.6f: binder_transaction: transaction=7 dest_proc=51 dest_thread=51 reply=0 flags=0x0 code=0x1\n", start-.0001)
		fmt.Fprintf(&out, "target-41 (41) [000] .... %.6f: sched_switch: prev_comm=target prev_pid=41 prev_prio=120 prev_state=%s ==> next_comm=server next_pid=51 next_prio=120\n", start, state)
		fmt.Fprintf(&out, "server-51 (51) [000] .... %.6f: binder_transaction_received: transaction=7\n", start+.00001)
		if i%3 == 2 {
			fmt.Fprintf(&out, "server-51 (51) [000] .... %.6f: sched_blocked_reason: pid=41 iowait=1 caller=io_schedule\n", start+.00002)
		}
		fmt.Fprintf(&out, "server-51 (51) [000] .... %.6f: binder_transaction: transaction=8 dest_proc=41 dest_thread=41 reply=1 flags=0x0 code=0x0\n", start+.00015)
		fmt.Fprintf(&out, "server-51 (51) [000] .... %.6f: sched_wakeup: comm=target pid=41 prio=120 target_cpu=000\n", start+.0002)
		fmt.Fprintf(&out, "server-51 (51) [000] .... %.6f: sched_switch: prev_comm=server prev_pid=51 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=120\n", start+.00025)
		fmt.Fprintf(&out, "target-41 (41) [000] .... %.6f: binder_transaction_received: transaction=8\n", start+.0003)
	}
	return out.String()
}

func TestB1607BActualRunSequentialReuseAndCapHaveIndependentDomains(t *testing.T) {
	idx := buildTraceIndex(t, "many-binder.ftrace", b1607RepeatedClosedBinderTrace(39))
	var first *TargetWindowBinderWaitInventory
	for _, view := range []string{"window_stats", "wakeup_chain", "root_cause_rank", "frame_root_cause_bundle"} {
		q := Query{View: view, PID: 41, TimeStart: 10, TimeEnd: 10.041, MinDurationMs: 100, MaxBranches: 1, MaxDepth: 1, MaxChainNodes: 1, Limit: 1}
		got := b1607BinderInventory(t, Run(idx, q))
		if got.ScanStatus != "complete" || got.OutputStatus != "incomplete" || got.ConfirmedCount != 39 || got.Emitted != 32 || math.Abs(got.ConfirmedMs-7.8) > 1e-7 {
			t.Fatalf("all S/D/IO closed sub-ms occurrences must precede the display cap: %+v", got)
		}
		if first == nil {
			first = got
		} else if !reflect.DeepEqual(first, got) {
			t.Fatal("view selection changed independent closed wait accounting")
		}
		for i, row := range got.Occurrences {
			if row.Ordinal != i+1 || row.RequestTransactionID != 7 || row.ReplyTransactionID != 8 {
				t.Fatalf("sequential physical ID reuse was guessed or overwritten: %+v", row)
			}
			if i%3 == 2 && row.State != StateIOWait {
				t.Fatalf("independent Binder account changed the original IO state: %+v", row)
			}
		}
	}
}

func TestB1607BReasonLocatorIsNotTheClosureAuthority(t *testing.T) {
	idx := buildTraceIndex(t, "reason-locator.ftrace", donghuP9TrueBinderWaitTrace)
	q := Query{PID: 17267, TimeStart: 13762.8355, TimeEnd: 13762.8375}
	tl := ThreadTimeline(idx, q)
	for i := range tl.Intervals {
		if binderInventorySleep(tl.Intervals[i]) {
			tl.Intervals[i].EndLine = 4 // request receive, deliberately not a wake
		}
	}
	before := cloneTimelineResult(tl)
	got := buildTargetWindowBinderWaitInventory(idx, q, tl, queryResultTimeWindow(q))
	if got == nil || got.ConfirmedCount != 1 || got.Occurrences[0].EndLine != 4 || got.Occurrences[0].ClosureLine == 4 {
		t.Fatalf("exact independent wake should survive an unrelated original locator: %+v", got)
	}
	if !reflect.DeepEqual(tl, before) || !reflect.DeepEqual(got, buildTargetWindowBinderWaitInventory(idx, q, tl, queryResultTimeWindow(q))) {
		t.Fatal("builder modified the original timeline or is not idempotent")
	}
}

func TestB1607BSourceTopologyAndCancellationDoNotPublishHalfProof(t *testing.T) {
	for _, kind := range []string{"windowed", "relation scoped", "source split", "source unknown", "generation audit cap"} {
		t.Run(kind, func(t *testing.T) {
			idx := buildTraceIndex(t, "source-scope.ftrace", donghuP9TrueBinderWaitTrace)
			q := Query{PID: 17267, TimeStart: 13762.8355, TimeEnd: 13762.8375}
			tl := ThreadTimeline(idx, q)
			switch kind {
			case "windowed":
				idx.Windowed = true
			case "relation scoped":
				idx.RelationScoped = true
			case "source split":
				idx.TraceArtifacts = []TraceArtifactSource{{SourcePath: "/a.ftrace", LocalLineCount: 5, CausalCompatible: true}, {SourcePath: "/b.ftrace", LocalLineCount: 100, VirtualLineBase: 5, CausalCompatible: true}}
			case "source unknown":
				idx.TraceArtifacts = []TraceArtifactSource{{SourcePath: "/elsewhere.ftrace", LocalLineCount: 1, VirtualLineBase: 100, CausalCompatible: true}}
			case "generation audit cap":
				// Initialize the lazy authority first; ThreadTimeline does not
				// necessarily need this metadata cache on every fixture.
				ensureThreadGenerationMetadata(idx)
				idx.generationMetadataCapped = true
			}
			got := buildTargetWindowBinderWaitInventory(idx, q, tl, queryResultTimeWindow(q))
			if got == nil || got.ConfirmedCount != 0 || got.ConfirmedMs != 0 {
				t.Fatalf("incomplete physical or lifecycle authority minted closure: %+v", got)
			}
			if (kind == "windowed" || kind == "relation scoped" || kind == "source unknown") && got.ScanStatus == "complete" {
				t.Fatalf("partial endpoint scan claimed completeness: %+v", got)
			}
		})
	}
	idx := buildTraceIndex(t, "cancel-binder.ftrace", donghuP9TrueBinderWaitTrace)
	q := Query{PID: 17267, TimeStart: 13762.8355, TimeEnd: 13762.8375}
	tl := ThreadTimeline(idx, q)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	q.runCancel = &runCancelState{ctx: ctx, units: runCancelSampleMask - int64(len(idx.Events)) - 1}
	if got := buildTargetWindowBinderWaitInventory(idx, q, tl, queryResultTimeWindow(q)); got != nil || !q.runCancel.fired() {
		t.Fatalf("cancellation during the inventory's own event pass published a partial account: %+v", got)
	}
}

func TestB1607BOutstandingTransactionsAndLifecycleAreNotGuessed(t *testing.T) {
	for _, tc := range []struct{ name, trace string }{
		{"multiple pending", strings.Replace(donghuP9TrueBinderWaitTrace, " .ugc.aweme.lite-17267 (17267) [004] .... 13762.835834", " .ugc.aweme.lite-17267 (17267) [004] .... 13762.835820: binder_transaction: transaction=12145870 dest_proc=9743 dest_thread=0 reply=0 flags=0x0 code=0x1\n .ugc.aweme.lite-17267 (17267) [004] .... 13762.835834", 1)},
		{"overlapping ID", strings.Replace(donghuP9TrueBinderWaitTrace, " .ugc.aweme.lite-17267 (17267) [004] .... 13762.835834", " .ugc.aweme.lite-17267 (17267) [004] .... 13762.835820: binder_transaction: transaction=12145859 dest_proc=9743 dest_thread=0 reply=0 flags=0x0 code=0x1\n .ugc.aweme.lite-17267 (17267) [004] .... 13762.835834", 1)},
		{"callback during pending call", strings.Replace(donghuP9TrueBinderWaitTrace, "    binder:496_9-10961 ( 9743) [004] .... 13762.837261", " callback-123 (123) [004] .... 13762.836000: binder_transaction: transaction=12145870 dest_proc=17267 dest_thread=17267 reply=0 flags=0x0 code=0x1\n .ugc.aweme.lite-17267 (17267) [004] .... 13762.836100: binder_transaction_received: transaction=12145870\n    binder:496_9-10961 ( 9743) [004] .... 13762.837261", 1)},
		{"peer reused", strings.Replace(donghuP9TrueBinderWaitTrace, "    binder:496_9-10961 ( 9743) [004] .... 13762.837261", " creator-99 (99) [004] .... 13762.836100: sched_wakeup_new: comm=new_peer pid=10961 prio=53 target_cpu=004\n    binder:496_9-10961 ( 9743) [004] .... 13762.837261", 1)},
		{"earlier incomplete reply", strings.Replace(donghuP9TrueBinderWaitTrace, "    binder:496_9-10961 ( 9743) [004] .... 13762.837261", " binder:496_9-10961 (9743) [004] .... 13762.836100: binder_transaction: transaction=12145870 dest_proc=17267 dest_thread=17267 reply=1 flags=0x0 code=0x0\n    binder:496_9-10961 ( 9743) [004] .... 13762.837261", 1)},
		{"earlier unknown peer message", strings.Replace(donghuP9TrueBinderWaitTrace, "    binder:496_9-10961 ( 9743) [004] .... 13762.837261", " binder:496_9-10961 (9743) [004] .... 13762.836100: binder_transaction: transaction=12145870 dest_proc=17267 dest_thread=17267 reply=bad flags=0x0 code=0x0\n    binder:496_9-10961 ( 9743) [004] .... 13762.837261", 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			idx := buildTraceIndex(t, "pending-domain.ftrace", tc.trace)
			got := b1607BinderInventory(t, Run(idx, Query{View: "window_stats", PID: 17267, TimeStart: 13762.8355, TimeEnd: 13762.8375}))
			if got.ConfirmedCount != 0 || got.UnresolvedCandidateCount == 0 {
				t.Fatalf("ambiguous stack or reused peer identity became a unique request: %+v", got)
			}
		})
	}
}

func TestB1607BKnownPeerProcessContradictionCannotCertifyClosure(t *testing.T) {
	trace := strings.Replace(donghuP9TrueBinderWaitTrace, "binder:496_9-10961 ( 9743) [004] .... 13762.837261", "binder:496_9-10961 (99999) [004] .... 13762.837261", 1)
	idx := buildTraceIndex(t, "contradictory-peer-process.ftrace", trace)
	got := b1607BinderInventory(t, Run(idx, Query{View: "window_stats", PID: 17267, TimeStart: 13762.8355, TimeEnd: 13762.8375}))
	if got.ConfirmedCount != 0 || got.UnresolvedCandidateCount == 0 {
		t.Fatalf("same numeric TID with contradictory observed process identity became a closed peer: %+v", got)
	}
	unknown := strings.Replace(donghuP9TrueBinderWaitTrace, "binder:496_9-10961 ( 9743) [004] .... 13762.837261", "binder:496_9-10961 (    0) [004] .... 13762.837261", 1)
	unknownIndex := buildTraceIndex(t, "missing-peer-process.ftrace", unknown)
	unknownResult := b1607BinderInventory(t, Run(unknownIndex, Query{View: "window_stats", PID: 17267, TimeStart: 13762.8355, TimeEnd: 13762.8375}))
	if unknownResult.ConfirmedCount != 1 {
		t.Fatalf("unknown supplementary process metadata was treated as an observed contradiction: %+v", unknownResult)
	}
}

func TestB1607BActualRunClientIncarnationResetCannotCloseOldRequest(t *testing.T) {
	trace := strings.Replace(donghuP9TrueBinderWaitTrace, "13762.837270: sched_wakeup:", "13762.837270: sched_wakeup_new:", 1)
	idx := buildTraceIndex(t, "client-reused.ftrace", trace)
	for _, view := range []string{"window_stats", "wakeup_chain", "frame_root_cause_bundle"} {
		q := Query{View: view, PID: 17267, TimeStart: 13762.8355, TimeEnd: 13762.8375}
		if timeline := ThreadTimeline(idx, q); timeline.IntegrityFailure != "thread_incarnation_conflict" {
			t.Fatalf("fixture must exercise the existing exact client-incarnation gate: %+v", timeline)
		}
		result := Run(idx, q)
		if result.TargetWindowStates != nil || (result.FrameRootCauseBundle != nil && result.FrameRootCauseBundle.TargetWindowStates != nil) {
			t.Fatalf("a new client using the same numeric TID inherited the old request's account: %+v", result)
		}
	}
}

func TestB1607BEndpointBudgetRetainsDenominatorWithoutFalseCompleteness(t *testing.T) {
	idx := buildTraceIndex(t, "endpoint-cap.ftrace", donghuP9TrueBinderWaitTrace)
	q := Query{PID: 17267, TimeStart: 13762.8355, TimeEnd: 13762.8375}
	tl := ThreadTimeline(idx, q)
	for i := 0; i <= binderPairingEndpointBudget; i++ {
		idx.Events = append(idx.Events, binderPairingSend(100+i, 14000+float64(i)*.001, 99, 100+i))
	}
	// This in-memory boundary fixture owns one explicit physical line scope.
	idx.TraceArtifacts = []TraceArtifactSource{{SourcePath: idx.Path, LocalLineCount: binderPairingEndpointBudget + 200, CausalCompatible: true}}
	got := buildTargetWindowBinderWaitInventory(idx, q, tl, queryResultTimeWindow(q))
	if got == nil || got.ScanStatus == "complete" || got.TargetSleepCount != 1 || got.ConfirmedCount != 0 || got.UnresolvedCandidateCount != 1 {
		t.Fatalf("bounded endpoint audit became exhaustive or erased the sleep denominator: %+v", got)
	}
}

func TestB1607BUnknownHeadAndOpenTailDoNotCreateClosedWaits(t *testing.T) {
	path := writeSchedulerCarryTrace(t, "unknown-binder.systrace",
		"idle-0 (0) [000] .... 9.500000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=server next_pid=51 next_prio=120",
		"server-51 (51) [000] .... 10.002000: sched_switch: prev_comm=server prev_pid=51 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=120",
		"target-41 (41) [000] .... 10.002900: binder_transaction: transaction=7 dest_proc=51 dest_thread=51 reply=0 flags=0x0 code=0x1",
		"target-41 (41) [000] .... 10.003000: sched_switch: prev_comm=target prev_pid=41 prev_prio=120 prev_state=D ==> next_comm=server next_pid=51 next_prio=120",
		"server-51 (51) [000] .... 10.003010: binder_transaction_received: transaction=7",
		"server-51 (51) [000] .... 10.003050: sched_blocked_reason: pid=41 iowait=1 caller=io_schedule",
		"server-51 (51) [000] .... 10.010000: sched_switch: prev_comm=server prev_pid=51 prev_prio=120 prev_state=R ==> next_comm=idle next_pid=0 next_prio=120",
	)
	idx := buildSchedulerCarryWindow(t, path, 10, 10.009)
	got := b1607BinderInventory(t, Run(idx, Query{View: "window_stats", PID: 41, TimeStart: 10, TimeEnd: 10.009}))
	if got.HeadState == nil || got.HeadState.Status != "unknown" || got.TargetSleepCount != 1 || got.ConfirmedCount != 0 || got.ScanStatus == "complete" {
		t.Fatalf("unknown prefix or reason locator on an open tail was promoted to closure: %+v", got)
	}
}

func TestB1607BUnionAndHeadCopyPreserveSourceData(t *testing.T) {
	idx := buildTraceIndex(t, "union-binder.ftrace", donghuP9TrueBinderWaitTrace)
	q := Query{PID: 17267, TimeStart: 13762.8355, TimeEnd: 13762.8375}
	tl := ThreadTimeline(idx, q)
	tl.HeadState = &TimelineHeadState{Status: "observed_in_index", BoundaryTs: q.TimeStart}
	for _, interval := range tl.Intervals {
		if binderInventorySleep(interval) {
			tl.Intervals = append(tl.Intervals, interval)
			break
		}
	}
	got := buildTargetWindowBinderWaitInventory(idx, q, tl, queryResultTimeWindow(q))
	if got == nil || got.ConfirmedCount != 2 || math.Abs(got.ConfirmedMs-1.409) > 1e-6 {
		t.Fatalf("overlapping input occurrences were added instead of unioned: %+v", got)
	}
	got.HeadState.Status = "test mutation"
	if tl.HeadState.Status != "observed_in_index" {
		t.Fatal("inventory aliases mutable source head state")
	}
}
