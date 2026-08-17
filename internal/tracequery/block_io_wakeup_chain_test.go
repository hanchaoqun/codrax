package tracequery

import (
	"fmt"
	"strings"
	"testing"
)

// The customer shape is a short business-thread request completed by an IRQ
// which immediately wakes that same issuer.  Nine longer background requests
// intentionally occupy the public Top-8 so this also pins that display
// capacity can never erase a causal request from rank/blocking consumers.
func TestBlockIOCompletionWakeupUsesFullExactPairCensus(t *testing.T) {
	var trace strings.Builder
	for i := 0; i < 9; i++ {
		pid := 1000 + i
		issue := 1.010 + float64(i)*0.010
		complete := issue + 0.005
		sector := 800000 + i*16
		fmt.Fprintf(&trace, "bg-%d (%d) [003] .... %.6f: block_rq_issue: 12,80 RCVHS 4096 () %d + 8 [bg]\n", pid, pid, issue, sector)
		fmt.Fprintf(&trace, "udk-irq-3-70 (2) [003] .... %.6f: block_rq_complete: 12,80 RCVHS () %d + 8 [0]\n", complete, sector)
	}
	trace.WriteString(`
com.tencent.mm-25827 (25827) [004] .... 1.150000: block_rq_issue: 12,80 RCVHS 32768 () 923339752 + 64 [com.tencent.mm]
com.tencent.mm-25827 (25827) [004] .... 1.150050: sched_switch: prev_comm=com.tencent.mm prev_pid=25827 prev_prio=53 prev_state=S ==> next_comm=idle/4 next_pid=0 next_prio=120
udk-irq-4-80 (2) [004] .... 1.150275: block_rq_complete: 12,80 RCVHS () 923339752 + 64 [0]
udk-irq-4-80 (2) [004] .... 1.150290: sched_wakeup: comm=com.tencent.mm pid=25827 prio=53 target_cpu=004
com.tencent.mm-25827 (25827) [004] .... 1.150330: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=com.tencent.mm next_pid=25827 next_prio=53
`)
	idx := buildTraceIndex(t, "customer_block_io_wakeup.systrace", trace.String())
	q := Query{PID: 25827, TimeStart: 1.0, TimeEnd: 1.2, MaxDepth: 4, MinDurationMs: 0.01, Limit: 32}
	stats := ComputeWindowStats(idx, q)
	if len(stats.IOLatencies) != 8 || len(stats.ioLatencyCensus) != 10 || stats.IOLatencyOverflowCount != 2 {
		t.Fatalf("public view/full census split is wrong: view=%d census=%d overflow=%d", len(stats.IOLatencies), len(stats.ioLatencyCensus), stats.IOLatencyOverflowCount)
	}
	for _, io := range stats.IOLatencies {
		if io.IssueThread.PID == 25827 {
			t.Fatalf("the short customer request must really sit below the public Top-8 in this fixture: %+v", stats.IOLatencies)
		}
	}
	var customer *IOLatencySummary
	for i := range stats.ioLatencyCensus {
		if stats.ioLatencyCensus[i].Sector == 923339752 {
			customer = &stats.ioLatencyCensus[i]
			break
		}
	}
	if customer == nil || customer.EndpointFamily != blockEndpointFamilyRQ || customer.WaitCaliber != BlockIOWaitCaliberIssueToComplete || !customer.CompletionWokeIssuer || customer.WakeupLine <= customer.CompleteLine {
		t.Fatalf("customer request lost its exact wait/wakeup credential: %+v", customer)
	}
	if customer.DurationMs < 0.274 || customer.DurationMs > 0.276 {
		t.Fatalf("issue→complete request wait must be 0.275ms, got %+v", customer)
	}
	if customer.CausalWaitCaliber != BlockIOCausalWaitCaliberCompletionClosedIssuerBlocked || customer.IssuerBlockedState != string(StateSSleep) || customer.IssuerBlockedMs < 0.239 || customer.IssuerBlockedMs > 0.241 {
		t.Fatalf("the response-impact ruler must be S switch-out→completion wake (0.240ms), independent from request residence: %+v", customer)
	}
	chain := BuildWakeupChain(idx, q)
	foundEdge := false
	for _, edge := range chain.Edges {
		// The header's runtime TID is authoritative for the directed edge;
		// parenthesized TGID/namespace identity (2 in this fixture) must not
		// replace the IRQ thread TID (80).
		if edge.Waker.PID == customer.CompleteThread.PID && edge.Wakee.PID == 25827 && edge.WakeupLine == customer.WakeupLine {
			foundEdge = true
			break
		}
	}
	if !foundEdge {
		t.Fatalf("the exact completion→issuer wake must remain a first-class wakeup-chain edge: wake_line=%d edges=%+v", customer.WakeupLine, chain.Edges)
	}

	rank := BuildRootCauseRank(idx, q)
	foundRank := false
	for _, item := range rank.Items {
		if item.Type == "io_latency" && item.Thread.PID == 25827 {
			foundRank = true
			if item.ChainRelevance != "on_chain" || !item.ResourceCompletionClosure || !strings.Contains(item.MemberKey, "family=block_rq") || !strings.Contains(item.MemberKey, "len=64") || item.EffectiveImpactMs < 0.239 || item.EffectiveImpactMs > 0.241 {
				t.Fatalf("full-census customer IO did not retain its on-chain exact identity: %+v", item)
			}
			if !strings.Contains(item.Summary, "request_residence=0.275ms is mechanism evidence and is not additive") {
				t.Fatalf("rank summary must disclose both non-additive rulers: %+v", item)
			}
		}
	}
	if !foundRank {
		t.Fatalf("short causal request was swallowed by the display cap: %+v", rank.Items)
	}
	blocking := BuildCriticalBlockingCalls(idx, q)
	foundBlocking := false
	for _, item := range blocking.Items {
		if item.Type == "io_latency" && item.Thread.PID == 25827 {
			foundBlocking = item.DurationMs > 0.239 && item.DurationMs < 0.241 && strings.Contains(item.Summary, "response_blocked(s_sleep, completion_closed)=0.240ms")
		}
	}
	if !foundBlocking {
		t.Fatalf("critical-blocking face did not consume the full typed IO census: %+v", blocking.Items)
	}
}

func TestBlockIOCompletionWakeupUsesDStateResponseRuler(t *testing.T) {
	idx := buildTraceIndex(t, "block_io_d_wait.systrace", `
app-40 (40) [001] .... 5.000000: block_rq_issue: 8,0 R 4096 () 123 + 8 [app]
app-40 (40) [001] .... 5.000020: sched_switch: prev_comm=app prev_pid=40 prev_prio=20 prev_state=D ==> next_comm=idle/1 next_pid=0 next_prio=120
irq-2 (2) [001] .... 5.000100: block_rq_complete: 8,0 R () 123 + 8 [0]
irq-2 (2) [001] .... 5.000110: sched_wakeup: comm=app pid=40 prio=20 target_cpu=001
`)
	stats := ComputeWindowStats(idx, Query{PID: 40, TimeStart: 5, TimeEnd: 5.001})
	if len(stats.IOLatencies) != 1 {
		t.Fatalf("expected one exact request: %+v", stats.IOLatencies)
	}
	io := stats.IOLatencies[0]
	if !io.CompletionWokeIssuer || io.IssuerBlockedState != string(StateDSleep) || io.IssuerBlockedMs < 0.089 || io.IssuerBlockedMs > 0.091 {
		t.Fatalf("D and S must share the exact completion-closed response ruler: %+v", io)
	}
}

func TestBlockIOCompletionWakeupRejectsClosedEarlierSleep(t *testing.T) {
	idx := buildTraceIndex(t, "block_io_prior_wake.systrace", `
app-40 (40) [001] .... 6.000000: block_rq_issue: 8,0 R 4096 () 123 + 8 [app]
app-40 (40) [001] .... 6.000020: sched_switch: prev_comm=app prev_pid=40 prev_prio=20 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
other-3 (3) [001] .... 6.000050: sched_wakeup: comm=app pid=40 prio=20 target_cpu=001
idle-0 (0) [001] .... 6.000060: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=40 next_prio=20
irq-2 (2) [001] .... 6.000100: block_rq_complete: 8,0 R () 123 + 8 [0]
irq-2 (2) [001] .... 6.000110: sched_wakeup: comm=app pid=40 prio=20 target_cpu=001
`)
	stats := ComputeWindowStats(idx, Query{PID: 40, TimeStart: 6, TimeEnd: 6.001})
	if len(stats.IOLatencies) != 1 || stats.IOLatencies[0].CompletionWokeIssuer || stats.IOLatencies[0].IssuerBlockedMs != 0 {
		t.Fatalf("a sleep already closed before request completion must not be re-owned by a later wake: %+v", stats.IOLatencies)
	}
}

func TestBlockIOCompletionWakeupRejectsWrongWakeeAndPreCompletionRows(t *testing.T) {
	idx := buildTraceIndex(t, "block_io_false_closure.systrace", `
app-40 (40) [001] .... 2.000000: block_rq_issue: 8,0 R 4096 () 123 + 8 [app]
irq-2 (2) [001] .... 2.000050: sched_wakeup: comm=app pid=40 prio=20 target_cpu=001
irq-2 (2) [001] .... 2.000100: block_rq_complete: 8,0 R () 123 + 8 [0]
irq-2 (2) [001] .... 2.000110: sched_wakeup: comm=other pid=41 prio=20 target_cpu=001
`)
	stats := ComputeWindowStats(idx, Query{TimeStart: 2.0, TimeEnd: 2.001})
	if len(stats.IOLatencies) != 1 {
		t.Fatalf("expected one exact request pair: %+v", stats.IOLatencies)
	}
	if stats.IOLatencies[0].CompletionWokeIssuer || stats.IOLatencies[0].WakeupLine != 0 {
		t.Fatalf("wrong-wakee/pre-completion rows must not mint a closure: %+v", stats.IOLatencies[0])
	}
}

func TestBlockIOCompletionWakeupAssignsOneWakeToBatchReleaseCompletion(t *testing.T) {
	idx := buildTraceIndex(t, "block_io_ambiguous_completion_wake.systrace", `
app-40 (40) [001] .... 3.000000: block_rq_issue: 8,0 R 4096 () 123 + 8 [app]
app-40 (40) [001] .... 3.000010: block_rq_issue: 8,0 R 4096 () 456 + 8 [app]
app-40 (40) [001] .... 3.000020: sched_switch: prev_comm=app prev_pid=40 prev_prio=20 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
irq-2 (2) [001] .... 3.000100: block_rq_complete: 8,0 R () 123 + 8 [0]
irq-2 (2) [001] .... 3.000110: block_rq_complete: 8,0 R () 456 + 8 [0]
irq-2 (2) [001] .... 3.000120: sched_wakeup: comm=app pid=40 prio=20 target_cpu=001
`)
	stats := ComputeWindowStats(idx, Query{TimeStart: 3.0, TimeEnd: 3.001})
	if len(stats.IOLatencies) != 2 {
		t.Fatalf("expected two exact request pairs: %+v", stats.IOLatencies)
	}
	for _, io := range stats.IOLatencies {
		want := io.Sector == 456
		if io.CompletionWokeIssuer != want {
			t.Fatalf("one wake must belong only to the final physical completion in the batch: %+v", stats.IOLatencies)
		}
	}
}

func TestBlockIOCompletionWakeupRequiresIssuerBlockingTransition(t *testing.T) {
	idx := buildTraceIndex(t, "block_io_async_issue.systrace", `
app-40 (40) [001] .... 4.000000: block_rq_issue: 8,0 R 4096 () 123 + 8 [app]
irq-2 (2) [001] .... 4.000100: block_rq_complete: 8,0 R () 123 + 8 [0]
irq-2 (2) [001] .... 4.000110: sched_wakeup: comm=app pid=40 prio=20 target_cpu=001
`)
	stats := ComputeWindowStats(idx, Query{TimeStart: 4.0, TimeEnd: 4.001})
	if len(stats.IOLatencies) != 1 {
		t.Fatalf("expected one exact request pair: %+v", stats.IOLatencies)
	}
	if stats.IOLatencies[0].CompletionWokeIssuer {
		t.Fatalf("issue/complete plus proximity is not enough without an issuer blocking transition: %+v", stats.IOLatencies[0])
	}
}

func TestBlockIOCompletionWakeupRejectsTerminalIssuerState(t *testing.T) {
	idx := buildTraceIndex(t, "block_io_stopped_issue.systrace", `
app-40 (40) [001] .... 4.100000: block_rq_issue: 8,0 R 4096 () 123 + 8 [app]
app-40 (40) [001] .... 4.100020: sched_switch: prev_comm=app prev_pid=40 prev_prio=20 prev_state=T ==> next_comm=idle/1 next_pid=0 next_prio=120
irq-2 (2) [001] .... 4.100100: block_rq_complete: 8,0 R () 123 + 8 [0]
irq-2 (2) [001] .... 4.100110: sched_wakeup: comm=app pid=40 prio=20 target_cpu=001
`)
	stats := ComputeWindowStats(idx, Query{TimeStart: 4.1, TimeEnd: 4.101})
	if len(stats.IOLatencies) != 1 || stats.IOLatencies[0].CompletionWokeIssuer {
		t.Fatalf("stopped/dead/unknown exits are not IO response-wait states: %+v", stats.IOLatencies)
	}
}
