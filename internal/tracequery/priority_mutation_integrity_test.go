package tracequery

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMalformedPriorityMutationHeaderPoisonsPriorityOnly(t *testing.T) {
	for _, tc := range []struct {
		name        string
		mutation    string
		wantKnownTs bool
	}{
		{
			name:        "invalid CPU retains exact point",
			mutation:    `boost-9 (9) [bad] .... 1.100000: sched_pi_setprio: comm=app pid=20 oldprio=20 newprio=40`,
			wantKnownTs: true,
		},
		{
			name:     "invalid timestamp withdraws source range",
			mutation: `boost-9 (9) [000] .... NaN: sched_pi_setprio: comm=app pid=20 oldprio=20 newprio=40`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := writeSchedulerIntegrityTrace(t, "priority-mutation.systrace",
				`idle-0 (0) [000] .... 1.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=20`,
				tc.mutation,
				`app-20 (20) [000] .... 2.000000: sched_switch: prev_comm=app prev_pid=20 prev_prio=20 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120`,
			)
			idx, err := BuildIndex(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			if len(idx.schedulerRowIntegrityFailures) != 1 {
				t.Fatalf("malformed priority mutation did not retain one bounded poison: %+v", idx.schedulerRowIntegrityFailures)
			}
			failure := idx.schedulerRowIntegrityFailures[0]
			if failure.EventName != "sched_pi_setprio" || failure.AffectsAllPIDs || len(failure.PIDs) != 1 || failure.PIDs[0] != 20 {
				t.Fatalf("priority mutation subject/header provenance drifted: %+v", failure)
			}
			if tc.wantKnownTs != finitePriorityTimestamp(failure.Ts) {
				t.Fatalf("timestamp authority = %t, want %t: %+v", finitePriorityTimestamp(failure.Ts), tc.wantKnownTs, failure)
			}
			if stateFailure := schedulerStateIntegrityFailureForQuery(idx, Query{TimeStart: 1, TimeEnd: 2}, 20); stateFailure != nil {
				t.Fatalf("priority-only poison erased scheduler state: %+v", stateFailure)
			}
			stats := ComputeWindowStats(idx, Query{TimeStart: 1, TimeEnd: 2})
			if len(stats.TopRunning) == 0 || stats.TopRunning[0].Thread.PID != 20 || stats.TopRunning[0].DurationMs <= 0 {
				t.Fatalf("valid running timeline disappeared behind priority-only poison: %+v", stats)
			}
			verdict := newPriorityPointAuthority(idx).pointVerdictAt(20, priorityTestPoint("artifact:0", 1.5, 2), priorityPointAt)
			if verdict.hardEvidence() {
				t.Fatalf("malformed mutation retained hard priority range: %+v", verdict)
			}
		})
	}
}

func TestPriorityMutationLookalikeAndCapIsolation(t *testing.T) {
	lookalike := `logger-7 (7) [000] .... 1.100000: print: customer text "boost-9 (9) [bad] .... NaN: sched_pi_setprio: pid=20"`
	if failure := schedulerRejectedRowFailure(1, lookalike); failure != nil {
		t.Fatalf("print payload lookalike gained mutation authority: %+v", failure)
	}

	idx := &Index{TimestampOrder: TraceTimestampOrderMonotonic, Events: []Event{
		{Line: 1, Ts: 1, Type: EventSchedSwitch, NextPID: 20, NextPrio: 20},
		{Line: 200, Ts: 2, Type: EventSchedSwitch, PrevPID: 20, PrevPrio: 20, PrevState: "S"},
	}}
	for i := 0; i <= schedulerRowIntegrityFailureCap; i++ {
		appendSchedulerRowIntegrityFailure(idx, schedulerRowIntegrityFailure{
			EventName: "sched_pi_setprio", Line: i + 2, Ts: 1.1 + float64(i)/1000,
			PIDs: []int{20}, Fields: []string{"parser_rejected_row"},
		})
	}
	if !idx.priorityMutationIntegrityFailuresCapped || idx.schedulerRowIntegrityFailuresCapped ||
		len(idx.schedulerRowIntegrityFailures) != schedulerRowIntegrityFailureCap {
		t.Fatalf("priority-only audit cap contaminated scheduler-state lane: priority_cap=%t scheduler_cap=%t rows=%d",
			idx.priorityMutationIntegrityFailuresCapped, idx.schedulerRowIntegrityFailuresCapped, len(idx.schedulerRowIntegrityFailures))
	}
	if failure := schedulerRowIntegrityFailureForQuery(idx, Query{TimeStart: 1, TimeEnd: 2}, 20); failure != nil {
		t.Fatalf("priority-only cap surfaced as scheduler-state corruption: %+v", failure)
	}
	if verdict := newPriorityPointAuthority(idx).pointVerdictAt(20, priorityTestPoint("compat:index", 1.5, 100), priorityPointAt); verdict.hardEvidence() {
		t.Fatalf("priority mutation cap silently retained closed-range authority: %+v", verdict)
	}

	appendSchedulerRowIntegrityFailure(idx, schedulerRowIntegrityFailure{
		EventName: "sched_switch", Line: 300, Ts: math.Inf(1), AffectsAllPIDs: true,
		Fields: []string{"parser_rejected_row"},
	})
	if failure := schedulerRowIntegrityFailureForQuery(idx, Query{}, 20); failure == nil || !strings.Contains(failure.reason(), "sched_switch") {
		t.Fatalf("independent scheduler-state witness was starved by priority cap: %+v", failure)
	}
}

func TestWindowHeadMalformedPriorityMutationDoesNotEraseCarryIn(t *testing.T) {
	path := writeSchedulerIntegrityTrace(t, "priority-head.systrace",
		`idle-0 (0) [000] .... 1.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=20`,
		`boost-9 (9) [000] .... NaN: sched_pi_setprio: comm=app pid=20 oldprio=20 newprio=40`,
		`app-20 (20) [000] .... 2.000000: sched_switch: prev_comm=app prev_pid=20 prev_prio=20 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120`,
	)
	idx, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
		TimeStart: 1.5, TimeStartSet: true, TimeEnd: 1.6, TimeEndSet: true,
		AllowWindowedParse: true, ScopePID: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	head := schedulerHeadForQuery(idx, Query{PID: 20, TimeStart: 1.5, TimeEnd: 1.6})
	if head == nil || !head.Complete || head.Threads[20].State != StateRunning {
		t.Fatalf("priority-only malformed prefix erased window-head carry-in: %+v", head)
	}
	if len(idx.schedulerRowIntegrityFailures) != 1 || idx.schedulerRowIntegrityFailures[0].EventName != "sched_pi_setprio" {
		t.Fatalf("window head lost priority poison provenance: %+v", idx.schedulerRowIntegrityFailures)
	}
	if !math.IsNaN(idx.schedulerRowIntegrityFailures[0].Ts) {
		t.Fatalf("malformed timestamp gained synthetic point authority: %+v", idx.schedulerRowIntegrityFailures[0])
	}
	if verdict := newPriorityPointAuthority(idx).pointVerdictAt(20, priorityTestPoint("artifact:0", 1.55, 3), priorityPointAt); verdict.hardEvidence() {
		t.Fatalf("window-head malformed mutation retained a hard priority range: %+v", verdict)
	}
}

func TestAnchorSeekWindowHeadMalformedTimestampPriorityMutationStaysPoisonOnly(t *testing.T) {
	resetAnchorCaches()
	t.Cleanup(resetAnchorCaches)
	dir := t.TempDir()
	path := filepath.Join(dir, "priority-head-anchor.systrace")
	var body strings.Builder
	body.WriteString("# tracer: nop\n")
	body.WriteString("idle-0 (0) [000] .... 100.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=20\n")
	body.WriteString("boost-9 (9) [000] .... NaN: sched_pi_setprio: comm=app pid=20 oldprio=20 newprio=40\n")
	for i := 0; i < 2*traceAnchorLineInterval+256; i++ {
		ts := 100.000001 + float64(i)*0.0001
		fmt.Fprintf(&body, "noise-9 (9) [001] .... %.6f: sched_wakeup: comm=noise pid=9 prio=20 target_cpu=001\n", ts)
	}
	if err := os.WriteFile(path, []byte(body.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	windowStart := 100.000001 + float64(2*traceAnchorLineInterval+64)*0.0001
	opts := BuildOptions{
		TimeStart: windowStart, TimeStartSet: true,
		TimeEnd: windowStart + 0.005, TimeEndSet: true,
		AllowWindowedParse: true, ScopePID: 20,
	}
	cold, err := BuildIndexWithOptions(context.Background(), path, opts)
	if err != nil {
		t.Fatal(err)
	}
	// Preserve the complete-file anchor proof while forcing the second build
	// through the actual seek path instead of the exact index-cache hit.
	indexCache = newTraceIndexCache(traceIndexCacheBudgetBytes)
	warm, err := BuildIndexWithOptions(context.Background(), path, opts)
	if err != nil {
		t.Fatal(err)
	}
	if warm.ScannedLineCount >= cold.ScannedLineCount {
		t.Fatalf("fixture did not exercise warm anchor seek: cold=%d warm=%d", cold.ScannedLineCount, warm.ScannedLineCount)
	}
	head := schedulerHeadForQuery(warm, Query{PID: 20, TimeStart: windowStart, TimeEnd: windowStart + 0.005})
	if head == nil || !head.Complete || head.Threads[20].State != StateRunning {
		t.Fatalf("priority-only malformed prefix erased warm carry-in state: %+v", head)
	}
	if len(warm.schedulerRowIntegrityFailures) != 1 ||
		warm.schedulerRowIntegrityFailures[0].EventName != "sched_pi_setprio" ||
		!math.IsNaN(warm.schedulerRowIntegrityFailures[0].Ts) {
		t.Fatalf("warm seek lost source-global priority poison: %+v", warm.schedulerRowIntegrityFailures)
	}
	if verdict := newPriorityPointAuthority(warm).pointVerdictAt(20, priorityTestPoint("artifact:0", windowStart+0.001, 2*traceAnchorLineInterval+70), priorityPointAt); verdict.hardEvidence() {
		t.Fatalf("warm seek rebuilt a hard priority range across malformed mutation: %+v", verdict)
	}
}

func TestAnchorEOFMutationAuditClosesUnknownTimestampBeyondWarmWindow(t *testing.T) {
	resetAnchorCaches()
	t.Cleanup(resetAnchorCaches)
	dir := t.TempDir()
	path := filepath.Join(dir, "priority-after-window-anchor.systrace")
	var body strings.Builder
	body.WriteString("# tracer: nop\n")
	body.WriteString("idle-0 (0) [000] .... 100.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=20\n")
	mutationAt := 2*traceAnchorLineInterval + 128
	mutationLine := 0
	for i := 0; i < 3*traceAnchorLineInterval; i++ {
		if i == mutationAt {
			mutationLine = i + 3 // comment + opening switch + zero-based noise
			body.WriteString("boost-9 (9) [000] .... NaN: sched_pi_setprio: comm=app pid=20 oldprio=20 newprio=40\n")
			continue
		}
		ts := 100.000001 + float64(i)*0.0001
		fmt.Fprintf(&body, "noise-9 (9) [001] .... %.6f: sched_wakeup: comm=noise pid=9 prio=20 target_cpu=001\n", ts)
	}
	if err := os.WriteFile(path, []byte(body.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	windowStart := 100.000001 + float64(traceAnchorLineInterval+64)*0.0001
	opts := BuildOptions{
		TimeStart: windowStart, TimeStartSet: true,
		TimeEnd: windowStart + 0.005, TimeEndSet: true,
		AllowWindowedParse: true, ScopePID: 20,
	}
	cold, err := BuildIndexWithOptions(context.Background(), path, opts)
	if err != nil {
		t.Fatal(err)
	}
	if cold.TimestampOrder != TraceTimestampOrderMonotonic || len(cold.schedulerRowIntegrityFailures) != 1 {
		t.Fatalf("cold EOF scan did not establish order plus source-global mutation poison: order=%v failures=%+v",
			cold.TimestampOrder, cold.schedulerRowIntegrityFailures)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	anchors := anchorCache.load(traceAnchorKeyForInfo(cold.Path, info))
	if anchors == nil || !anchors.PriorityMutationAuditComplete ||
		len(anchors.PriorityMutationIntegrityFailures) != 1 ||
		anchors.PriorityMutationIntegrityFailures[0].Line != mutationLine {
		t.Fatalf("EOF anchor record lost the bounded mutation audit: %+v", anchors)
	}

	indexCache = newTraceIndexCache(traceIndexCacheBudgetBytes)
	warm, err := BuildIndexWithOptions(context.Background(), path, opts)
	if err != nil {
		t.Fatal(err)
	}
	if warm.ScannedLineCount >= mutationLine || warm.ScannedLineCount >= cold.ScannedLineCount {
		t.Fatalf("fixture did not exercise pre-mutation warm early-stop: mutation=%d cold=%d warm=%d",
			mutationLine, cold.ScannedLineCount, warm.ScannedLineCount)
	}
	if len(warm.schedulerRowIntegrityFailures) != 1 ||
		warm.schedulerRowIntegrityFailures[0].Line != mutationLine ||
		!math.IsNaN(warm.schedulerRowIntegrityFailures[0].Ts) {
		t.Fatalf("warm early-stop lost cached source-global mutation poison: %+v", warm.schedulerRowIntegrityFailures)
	}
	head := schedulerHeadForQuery(warm, Query{PID: 20, TimeStart: windowStart, TimeEnd: windowStart + 0.005})
	if head == nil || !head.Complete || head.Threads[20].State != StateRunning {
		t.Fatalf("cached priority-only poison erased scheduler carry-in: %+v", head)
	}
	if verdict := newPriorityPointAuthority(warm).pointVerdictAt(20, priorityTestPoint("artifact:0", windowStart+0.001, traceAnchorLineInterval+70), priorityPointAt); verdict.hardEvidence() {
		t.Fatalf("warm early-stop rebuilt a hard priority range across source-global poison: %+v", verdict)
	}
}

func TestColdPaddingSuffixProofImportsMutationAuditIntoCurrentIndex(t *testing.T) {
	resetAnchorCaches()
	t.Cleanup(resetAnchorCaches)
	path := writeSchedulerIntegrityTrace(t, "priority-padding-suffix.systrace",
		`idle-0 (0) [000] .... 1.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=20`,
		`noise-9 (9) [001] .... 1.200000: sched_wakeup: comm=noise pid=9 prio=20 target_cpu=001`,
		`boost-9 (9) [000] .... NaN: sched_pi_setprio: comm=app pid=20 oldprio=20 newprio=40`,
		`app-20 (20) [000] .... 2.000000: sched_switch: prev_comm=app prev_pid=20 prev_prio=20 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120`,
	)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	idx, err := parseFile(t.Context(), path, info.Size(), info.ModTime().UnixNano(), BuildOptions{
		TimeStart: 1.0, TimeStartSet: true,
		TimeEnd: 1.1, TimeEndSet: true,
		TimePaddingAfter: 1.0, AllowWindowedParse: true, MaxEvents: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !idx.PaddingTruncated || idx.TimestampOrder != TraceTimestampOrderMonotonic {
		t.Fatalf("fixture did not take padding-truncation suffix-proof path: truncated=%t order=%v", idx.PaddingTruncated, idx.TimestampOrder)
	}
	if len(idx.schedulerRowIntegrityFailures) != 1 || idx.schedulerRowIntegrityFailures[0].Line != 3 ||
		!math.IsNaN(idx.schedulerRowIntegrityFailures[0].Ts) {
		t.Fatalf("cold current index lost suffix-only mutation audit: %+v", idx.schedulerRowIntegrityFailures)
	}
	if verdict := newPriorityPointAuthority(idx).pointVerdictAt(20, priorityTestPoint("artifact:0", 1.05, 1), priorityPointAt); verdict.hardEvidence() {
		t.Fatalf("cold padding-truncated query retained hard priority through suffix poison: %+v", verdict)
	}
}

func TestStreamingEOFRecordersPublishCompletePriorityMutationAudit(t *testing.T) {
	path := writeSchedulerIntegrityTrace(t, "priority-stream-eof.systrace",
		`idle-0 (0) [000] .... 1.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=20`,
		`boost-9 (9) [000] .... NaN: binder_set_priority: pid=20 prio=40`,
		`app-20 (20) [000] .... 2.000000: sched_switch: prev_comm=app prev_pid=20 prev_prio=20 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120`,
	)
	q := Query{TimeStart: 1.0, TimeEnd: 1.5, TimeStartSet: true, TimeEndSet: true}
	for _, tc := range []struct {
		name string
		run  func() error
	}{
		{name: "event_search", run: func() error { _, err := StreamEventSearch(t.Context(), path, q); return err }},
		{name: "state_cluster", run: func() error { _, err := StreamStateCluster(t.Context(), path, q, 8); return err }},
		{name: "window_sweep", run: func() error { _, err := StreamWindowSweep(t.Context(), path, q); return err }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetAnchorCaches()
			if err := tc.run(); err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			set := anchorCache.load(traceAnchorKeyForInfo(canonicalTraceIndexPath(path), info))
			if set == nil || !set.PriorityMutationAuditComplete ||
				len(set.PriorityMutationIntegrityFailures) != 1 ||
				set.PriorityMutationIntegrityFailures[0].EventName != "binder_set_priority" {
				t.Fatalf("stream recorder lost EOF mutation audit: %+v", set)
			}
		})
	}
	resetAnchorCaches()
}

func TestTraceBundlePriorityMutationPoisonAndCapRemainSourceLocal(t *testing.T) {
	dir := t.TempDir()
	poisonedPath := filepath.Join(dir, "poisoned.systrace")
	bundlePath := filepath.Join(dir, "priority.tracebundle.json")
	var poisoned strings.Builder
	poisoned.WriteString(`app-20 (20) [000] .... 1.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=20` + "\n")
	for i := 0; i <= schedulerRowIntegrityFailureCap; i++ {
		fmt.Fprintf(&poisoned, "boost-9 (9) [bad] .... %.6f: sched_pi_setprio: comm=app pid=20 oldprio=20 newprio=40\n", 1.1+float64(i)/1000)
	}
	poisoned.WriteString(`app-20 (20) [000] .... 2.000000: sched_switch: prev_comm=app prev_pid=20 prev_prio=20 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120` + "\n")
	if err := os.WriteFile(poisonedPath, []byte(poisoned.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	writeBundleProvenanceFixture(t, bundlePath, `{
  "version":"test",
  "systrace":"poisoned.systrace",
  "artifacts":[
	{"type":"systrace","path":"poisoned.systrace"}
  ]
}`)
	idx, err := BuildIndex(context.Background(), bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	if !idx.priorityMutationIntegrityFailuresCapped || idx.priorityMutationIntegrityOverflowGlobal ||
		len(idx.priorityMutationIntegrityOverflowSources) != 1 ||
		canonicalTraceIndexPath(idx.priorityMutationIntegrityOverflowSources[0]) != canonicalTraceIndexPath(poisonedPath) {
		t.Fatalf("bundle priority cap lost source-local scope: capped=%t global=%t sources=%v",
			idx.priorityMutationIntegrityFailuresCapped, idx.priorityMutationIntegrityOverflowGlobal, idx.priorityMutationIntegrityOverflowSources)
	}
	if idx.schedulerRowIntegrityFailuresCapped {
		t.Fatal("priority-only child cap contaminated composite scheduler-state cap")
	}
	authority := newPriorityPointAuthority(idx)
	poisonedLine := idx.TraceArtifacts[0].VirtualLineBase + 2
	if got := authority.pointVerdictAt(20, priorityTestPoint("artifact:0", 1.5, poisonedLine), priorityPointAt); got.hardEvidence() {
		t.Fatalf("capped child retained hard priority range: %+v", got)
	}

	// The real bundle proves cap transport. A two-causal-source Index below
	// pins the authority's source-local consumption independently of current
	// manifest admission policy (which intentionally permits one systrace
	// causal authority unless an explicit cross-source alignment exists).
	siblings := prioritySchedulerPoisonBundleIndex()
	markPriorityMutationIntegrityOverflow(siblings, "/trace/source-a.systrace")
	siblingAuthority := newPriorityPointAuthority(siblings)
	if got := siblingAuthority.pointVerdictAt(200, priorityTestPoint("artifact:0", 5.005, 3), priorityPointAt); got.hardEvidence() {
		t.Fatalf("source-capped artifact retained hard priority range: %+v", got)
	}
	if got := siblingAuthority.pointVerdictAt(300, priorityTestPoint("artifact:1", 5.005, 103), priorityPointAt); !got.hardEvidence() || got.Priority != 30 {
		t.Fatalf("priority cap poisoned a healthy admitted sibling: %+v", got)
	}
}
