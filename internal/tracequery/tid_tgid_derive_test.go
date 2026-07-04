package tracequery

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/logging"
)

// B-3 (§7.11) fixtures. hmtrace's line template has NO "( tgid )" column —
// process attribution must come from the trace_mark span-pid vote, and any
// trace WITH a native TGID column must stay byte-identical (gate off).

// b3HmtraceFixture is the hmtrace-shaped vote fixture:
//   - proc 500 "MainApp": main thread 500 (registration pair + C counter),
//     worker 501 (registration pair + business span) — both run in-window.
//   - proc 600 "OtherApp": main thread 600 registers but never runs;
//     worker 601 runs 20ms.
//   - tid 700 "TieThr": one B vote for 500 and one for 600 → tie → ungrouped.
//   - tid 702 "ZeroSpan": only B|0| → SpanPID guard → no vote.
//   - tid 801: row-header comm drifts RenA→RenB → name-drift guard → skipped.
//   - tid 802 "Efw": one B|500| vote plus two E|600| closes — E must not
//     vote, otherwise 600 would win 2:1.
//
// Registration pairs follow db2systrace.py: row `{task}-{tid}` emits
// `B|{pid}|{process_name}` immediately closed by `E|{pid}|`, main-thread
// name == process name.
const b3HmtraceFixture = `       MainApp-500 [000] d..1 0.900000: tracing_mark_write: B|500|MainApp
       MainApp-500 [000] d..1 0.900001: tracing_mark_write: E|500|
       WorkerA-501 [000] d..1 0.900002: tracing_mark_write: B|500|MainApp
       WorkerA-501 [000] d..1 0.900003: tracing_mark_write: E|500|
      OtherApp-600 [001] d..1 0.900004: tracing_mark_write: B|600|OtherApp
      OtherApp-600 [001] d..1 0.900005: tracing_mark_write: E|600|
      OtherWrk-601 [001] d..1 0.900006: tracing_mark_write: B|600|OtherApp
      OtherWrk-601 [001] d..1 0.900007: tracing_mark_write: E|600|
        TieThr-700 [001] d..1 0.900008: tracing_mark_write: B|500|ambig
        TieThr-700 [001] d..1 0.900009: tracing_mark_write: E|500|
        TieThr-700 [001] d..1 0.900010: tracing_mark_write: B|600|ambig
        TieThr-700 [001] d..1 0.900011: tracing_mark_write: E|600|
      ZeroSpan-702 [001] d..1 0.900012: tracing_mark_write: B|0|weird
          RenA-801 [001] d..1 0.900013: tracing_mark_write: B|500|z
          RenB-801 [001] d..1 0.900014: tracing_mark_write: B|500|z
           Efw-802 [001] d..1 0.900015: tracing_mark_write: B|500|q
           Efw-802 [001] d..1 0.900016: tracing_mark_write: E|600|
           Efw-802 [001] d..1 0.900017: tracing_mark_write: E|600|
          <idle>-0 [000] d..1 1.000000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=MainApp next_pid=500 next_prio=100
       MainApp-500 [000] d..1 1.005000: tracing_mark_write: C|500|frameCount|3
       MainApp-500 [000] d..1 1.010000: sched_wakeup: comm=WorkerA pid=501 prio=100 target_cpu=000
       MainApp-500 [000] d..1 1.020000: sched_switch: prev_comm=MainApp prev_pid=500 prev_prio=100 prev_state=S ==> next_comm=WorkerA next_pid=501 next_prio=100
       WorkerA-501 [000] d..1 1.030000: tracing_mark_write: B|500|doWork
       WorkerA-501 [000] d..1 1.040000: tracing_mark_write: E|500|
       WorkerA-501 [000] d..1 1.060000: sched_switch: prev_comm=WorkerA prev_pid=501 prev_prio=100 prev_state=S ==> next_comm=OtherWrk next_pid=601 next_prio=100
      OtherWrk-601 [000] d..1 1.080000: sched_switch: prev_comm=OtherWrk prev_pid=601 prev_prio=100 prev_state=S ==> next_comm=TieThr next_pid=700 next_prio=100
        TieThr-700 [000] d..1 1.090000: sched_switch: prev_comm=TieThr prev_pid=700 prev_prio=100 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120
`

func b3WindowQuery() Query {
	return Query{TimeStart: 1.0, TimeEnd: 1.2, TraceFlavorHint: TraceFlavorHarmonyHitrace}
}

func b3ProcessRow(t *testing.T, rows []ProcessCPULoadSummary, pid int) ProcessCPULoadSummary {
	t.Helper()
	for _, row := range rows {
		if row.Process.PID == pid {
			return row
		}
	}
	t.Fatalf("no process row with pid=%d in %+v", pid, rows)
	return ProcessCPULoadSummary{}
}

// TestTidTgidVoteTable pins the vote/guard semantics of the per-index table:
// B and C vote, E never votes, SpanPID<=0 never votes, ties and name-drifted
// tids fail open, and self-registration names land in commByTgid.
func TestTidTgidVoteTable(t *testing.T) {
	idx := buildTraceIndex(t, "b3_vote.htrace", b3HmtraceFixture)
	derive := idx.derivedTidTgid()
	if !derive.enabled() {
		t.Fatal("derivation must be enabled on a TGID-column-less trace with span votes")
	}
	for tid, want := range map[int]int{
		500: 500, // registration B + in-window C self-votes
		501: 500,
		600: 600,
		601: 600,
		700: 0,   // tie (one vote 500, one vote 600) → fail open
		702: 0,   // B|0| guarded by SpanPID>0
		801: 0,   // row-header comm drift RenA→RenB → fail open
		802: 500, // E|600|×2 must NOT vote; the single B|500| wins
	} {
		if got := derive.tgidFor(tid); got != want {
			t.Fatalf("tgidFor(%d) = %d, want %d", tid, got, want)
		}
	}
	if got := derive.commFor(500); got != "MainApp" {
		t.Fatalf("commFor(500) = %q, want MainApp", got)
	}
	if got := derive.commFor(600); got != "OtherApp" {
		t.Fatalf("commFor(600) = %q, want OtherApp", got)
	}
}

// TestTidTgidDerivedProcessGrouping pins the display-layer wiring: process
// rollups group hmtrace threads under their voted tgid, derived rows carry
// the self-explaining marker, the tie thread keeps today's degenerate
// per-thread row WITHOUT the marker, and the disclosure caveat reaches both
// stats.Caveats and the occupancy stanza.
func TestTidTgidDerivedProcessGrouping(t *testing.T) {
	idx := buildTraceIndex(t, "b3_group.htrace", b3HmtraceFixture)
	stats := ComputeWindowStats(idx, b3WindowQuery())

	main := b3ProcessRow(t, stats.ProcessCPULoad, 500)
	if main.ThreadCount != 2 {
		t.Fatalf("pid=500 ThreadCount = %d, want 2 (MainApp 500 + WorkerA 501): %s", main.ThreadCount, main.Summary)
	}
	approxEq(t, "pid=500 RunningMs", main.RunningMs, 60)
	if main.Process.Comm != "MainApp" {
		t.Fatalf("pid=500 process comm = %q, want MainApp", main.Process.Comm)
	}
	if !strings.HasSuffix(main.Summary, tidTgidDerivedRowMarker) {
		t.Fatalf("derived process row must carry the attribution marker: %s", main.Summary)
	}

	other := b3ProcessRow(t, stats.ProcessCPULoad, 600)
	if other.ThreadCount != 1 || other.Process.Comm != "OtherApp" {
		t.Fatalf("pid=600 row = threads=%d comm=%q, want 1/OtherApp: %s", other.ThreadCount, other.Process.Comm, other.Summary)
	}
	approxEq(t, "pid=600 RunningMs", other.RunningMs, 20)
	if !strings.HasSuffix(other.Summary, tidTgidDerivedRowMarker) {
		t.Fatalf("derived process row must carry the attribution marker: %s", other.Summary)
	}

	tie := b3ProcessRow(t, stats.ProcessCPULoad, 700)
	if tie.ThreadCount != 1 {
		t.Fatalf("tie thread must stay its own degenerate row: %s", tie.Summary)
	}
	if strings.Contains(tie.Summary, tidTgidDerivedRowMarker) {
		t.Fatalf("non-derived row must NOT carry the attribution marker: %s", tie.Summary)
	}

	foundStats := false
	for _, caveat := range stats.Caveats {
		if caveat == tidTgidDerivedCaveat {
			foundStats = true
		}
	}
	if !foundStats {
		t.Fatalf("stats.Caveats missing B-3 disclosure: %v", stats.Caveats)
	}
	if stats.CPUOccupancy == nil {
		t.Fatal("expected occupancy stats")
	}
	foundOcc := false
	for _, caveat := range stats.CPUOccupancy.Caveats {
		if caveat == tidTgidDerivedCaveat {
			foundOcc = true
		}
	}
	if !foundOcc {
		t.Fatalf("occupancy caveats missing B-3 disclosure: %v", stats.CPUOccupancy.Caveats)
	}
	occMain := b3ProcessRow(t, stats.CPUOccupancy.TopProcesses, 500)
	if occMain.ThreadCount != 2 || !strings.HasSuffix(occMain.Summary, tidTgidDerivedRowMarker) {
		t.Fatalf("occupancy pid=500 row must group 2 threads and carry the marker: %s", occMain.Summary)
	}
	occTie := b3ProcessRow(t, stats.CPUOccupancy.TopProcesses, 700)
	if strings.Contains(occTie.Summary, tidTgidDerivedRowMarker) {
		t.Fatalf("occupancy tie row must NOT carry the marker: %s", occTie.Summary)
	}
}

// TestTidTgidDerivedRunnableContextMatching pins the thread-side benefit of
// the catalog backfill: WorkerA-501's runnable-wait context resolves its OWN
// process row via the derived tgid (SameProcessLoad=500) and no longer risks
// reporting its own process as the top background process.
func TestTidTgidDerivedRunnableContextMatching(t *testing.T) {
	idx := buildTraceIndex(t, "b3_ctx.htrace", b3HmtraceFixture)
	stats := ComputeWindowStats(idx, b3WindowQuery())
	var worker *RunnableContextSummary
	for i := range stats.RunnableContext {
		if stats.RunnableContext[i].Thread.PID == 501 {
			worker = &stats.RunnableContext[i]
			break
		}
	}
	if worker == nil {
		t.Fatalf("expected runnable context for WorkerA-501: %+v", stats.RunnableContext)
	}
	if worker.Thread.TGID != 500 {
		t.Fatalf("runnable-context thread TGID = %d, want derived 500", worker.Thread.TGID)
	}
	if worker.SameProcessLoad == nil || worker.SameProcessLoad.Process.PID != 500 {
		t.Fatalf("SameProcessLoad must resolve the derived pid=500 row: %+v", worker.SameProcessLoad)
	}
	if worker.TopBackgroundProcess != nil && worker.TopBackgroundProcess.Process.PID == 500 {
		t.Fatalf("own derived process must not surface as top background process: %+v", worker.TopBackgroundProcess)
	}
}

// b3NameBackfillFixture: worker 511 runs in the queried line scope while main
// thread 510's only lines (the registration pair) sit OUTSIDE it — the
// process display name must come from the vote table's self-registration
// name, not from a catalogued line.
const b3NameBackfillFixture = `      MainApp2-510 [000] d..1 0.900000: tracing_mark_write: B|510|MainApp2
      MainApp2-510 [000] d..1 0.900001: tracing_mark_write: E|510|
          <idle>-0 [000] d..1 1.000000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=WorkerB next_pid=511 next_prio=100
       WorkerB-511 [000] d..1 1.010000: tracing_mark_write: B|510|doThing
       WorkerB-511 [000] d..1 1.015000: tracing_mark_write: E|510|
       WorkerB-511 [000] d..1 1.050000: sched_switch: prev_comm=WorkerB prev_pid=511 prev_prio=100 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120
`

func TestTidTgidNameBackfillWhenMainThreadOutsideScope(t *testing.T) {
	idx := buildTraceIndex(t, "b3_name.htrace", b3NameBackfillFixture)
	q := Query{TimeStart: 1.0, TimeEnd: 1.1, LineStart: 3, TraceFlavorHint: TraceFlavorHarmonyHitrace}
	stats := ComputeWindowStats(idx, q)
	row := b3ProcessRow(t, stats.ProcessCPULoad, 510)
	if row.Process.Comm != "MainApp2" {
		t.Fatalf("derived process display name = %q, want MainApp2 (commByTgid backfill): %s", row.Process.Comm, row.Summary)
	}
	if !strings.HasSuffix(row.Summary, tidTgidDerivedRowMarker) {
		t.Fatalf("derived row must carry the marker: %s", row.Summary)
	}
}

// b3AndroidFixture is the same schedule WITH a native TGID column (including
// the "(-----)" placeholder rows that parse to 0): one positive native TGID
// anywhere disables derivation wholesale, so the with-TGID path stays
// byte-identical — native grouping, no marker, no caveat.
const b3AndroidFixture = `          <idle>-0 (-----) [000] d..1 1.000000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=MainApp next_pid=500 next_prio=100
       MainApp-500 (  500) [000] d..1 1.005000: tracing_mark_write: B|500|MainApp
       MainApp-500 (  500) [000] d..1 1.006000: tracing_mark_write: E|500|
       MainApp-500 (  500) [000] d..1 1.020000: sched_switch: prev_comm=MainApp prev_pid=500 prev_prio=100 prev_state=S ==> next_comm=WorkerA next_pid=501 next_prio=100
       WorkerA-501 (  500) [000] d..1 1.060000: sched_switch: prev_comm=WorkerA prev_pid=501 prev_prio=100 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120
`

func TestTidTgidGateDisabledOnNativeTGIDColumn(t *testing.T) {
	idx := buildTraceIndex(t, "b3_android.systrace", b3AndroidFixture)
	if idx.derivedTidTgid().enabled() {
		t.Fatal("derivation must be disabled when the index carries any positive native TGID")
	}
	stats := ComputeWindowStats(idx, Query{TimeStart: 1.0, TimeEnd: 1.1})
	// Native grouping still works exactly as before.
	main := b3ProcessRow(t, stats.ProcessCPULoad, 500)
	if main.ThreadCount != 2 {
		t.Fatalf("native tgid grouping regressed: threads=%d want 2: %s", main.ThreadCount, main.Summary)
	}
	for _, row := range stats.ProcessCPULoad {
		if strings.Contains(row.Summary, tidTgidDerivedRowMarker) {
			t.Fatalf("with-TGID trace must never carry the derivation marker: %s", row.Summary)
		}
	}
	for _, caveat := range stats.Caveats {
		if caveat == tidTgidDerivedCaveat {
			t.Fatalf("with-TGID trace must never carry the B-3 caveat: %v", stats.Caveats)
		}
	}
	if stats.CPUOccupancy != nil {
		for _, caveat := range stats.CPUOccupancy.Caveats {
			if caveat == tidTgidDerivedCaveat {
				t.Fatalf("with-TGID occupancy must never carry the B-3 caveat: %v", stats.CPUOccupancy.Caveats)
			}
		}
		for _, row := range stats.CPUOccupancy.TopProcesses {
			if strings.Contains(row.Summary, tidTgidDerivedRowMarker) {
				t.Fatalf("with-TGID occupancy row must never carry the marker: %s", row.Summary)
			}
		}
	}
}

// b3GenericFixture has neither a TGID column nor any trace_mark span — no
// votes, so derivation stays off and the degenerate per-thread rows are
// unchanged (fail open, no caveat).
const b3GenericFixture = `          <idle>-0 [000] d..1 1.000000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=taskA next_pid=41 next_prio=100
         taskA-41 [000] d..1 1.020000: sched_switch: prev_comm=taskA prev_pid=41 prev_prio=100 prev_state=S ==> next_comm=taskB next_pid=42 next_prio=100
         taskB-42 [000] d..1 1.050000: sched_switch: prev_comm=taskB prev_pid=42 prev_prio=100 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120
`

func TestTidTgidGateDisabledWithoutSpanVotes(t *testing.T) {
	idx := buildTraceIndex(t, "b3_generic.trace", b3GenericFixture)
	if idx.derivedTidTgid().enabled() {
		t.Fatal("derivation must be disabled when no trace_mark span votes exist")
	}
	stats := ComputeWindowStats(idx, Query{TimeStart: 1.0, TimeEnd: 1.1})
	for _, pid := range []int{41, 42} {
		row := b3ProcessRow(t, stats.ProcessCPULoad, pid)
		if row.ThreadCount != 1 || strings.Contains(row.Summary, tidTgidDerivedRowMarker) {
			t.Fatalf("span-less trace must keep unmarked per-thread rows: %s", row.Summary)
		}
	}
	for _, caveat := range stats.Caveats {
		if caveat == tidTgidDerivedCaveat {
			t.Fatalf("span-less trace must not carry the B-3 caveat: %v", stats.Caveats)
		}
	}
}

// TestTidTgidCaveatWordingPinned pins the caveat wording itself (F1/F2):
//   - F1: the disclosure must warn that pid= filtering/drilldown still keys
//     on row-header tids only and does NOT follow the derived grouping — in
//     BOTH languages — otherwise an LLM reader drills a derived tgid and
//     publishes numbers that contradict the grouped process row.
//   - F2: the guard only detects row-header comm DRIFT; a reused tid that
//     keeps the same comm is undetectable, so the caveat must not promise
//     "reused" tids stay ungrouped (over-claim removed, EN now matches the
//     honest zh wording 改名线程不归组).
func TestTidTgidCaveatWordingPinned(t *testing.T) {
	for _, want := range []string{
		"ties and renamed tids stay ungrouped",
		"pid= filtering/drilldown still matches row-header tids only and does not follow the derived grouping",
		"平票/改名线程不归组",
		"pid= 过滤/钻取仍只匹配行头 tid,不追随派生分组",
	} {
		if !strings.Contains(tidTgidDerivedCaveat, want) {
			t.Fatalf("caveat lost pinned wording %q: %s", want, tidTgidDerivedCaveat)
		}
	}
	if strings.Contains(tidTgidDerivedCaveat, "reused") {
		t.Fatalf("caveat must not promise reused-tid detection — the guard only tests comm drift: %s", tidTgidDerivedCaveat)
	}
}

// b3NamePollutionFixture (F3): main thread 520's marks in the index are
// business spans only — an outer non-H: span with a child (fails the
// zero-child pair shape) and an adjacent-closed `H:` leaf (fails the
// closed-token guard) — and NO registration pair. Neither name may be
// promoted to a process display name; grouping itself still applies (the
// vote table is name-independent).
const b3NamePollutionFixture = `      BusyMain-520 [000] d..1 0.900000: tracing_mark_write: B|520|renderLoop
      BusyMain-520 [000] d..1 0.900001: tracing_mark_write: B|520|H:innerStep
      BusyMain-520 [000] d..1 0.900002: tracing_mark_write: E|520|
      BusyMain-520 [000] d..1 0.900003: tracing_mark_write: E|520|
      BusyMain-520 [000] d..1 0.900004: tracing_mark_write: B|520|H:LoopOnce
      BusyMain-520 [000] d..1 0.900005: tracing_mark_write: E|520|
          <idle>-0 [000] d..1 1.000000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=BusyWrk next_pid=521 next_prio=100
       BusyWrk-521 [000] d..1 1.010000: tracing_mark_write: B|520|H:doThing
       BusyWrk-521 [000] d..1 1.015000: tracing_mark_write: E|520|
       BusyWrk-521 [000] d..1 1.050000: sched_switch: prev_comm=BusyWrk prev_pid=521 prev_prio=100 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120
`

// TestTidTgidNameBackfillRejectsBusinessSpanNames pins F3: commByTgid only
// accepts the confirmed registration-pair shape (self-B immediately closed by
// its own E, zero child marks, name not `H:`-prefixed). Business span names
// must never become process names; with no confirmed pair the derived row
// keeps the degenerate no-name label (fail open), while tgid grouping and the
// attribution marker are unaffected. The registration-pair happy path stays
// pinned by TestTidTgidVoteTable (commFor) and
// TestTidTgidNameBackfillWhenMainThreadOutsideScope.
func TestTidTgidNameBackfillRejectsBusinessSpanNames(t *testing.T) {
	idx := buildTraceIndex(t, "b3_name_pollution.htrace", b3NamePollutionFixture)
	derive := idx.derivedTidTgid()
	if !derive.enabled() {
		t.Fatal("derivation must stay enabled: name rejection must not disable grouping")
	}
	if got := derive.tgidFor(521); got != 520 {
		t.Fatalf("tgidFor(521) = %d, want 520 (votes are name-independent)", got)
	}
	if got := derive.commFor(520); got != "" {
		t.Fatalf("commFor(520) = %q, want \"\" — business span names must not be promoted to process names", got)
	}
	// Display layer, main thread outside the queried line scope: the derived
	// process row groups the worker and carries the marker, but its display
	// name stays empty instead of borrowing any business span name.
	q := Query{TimeStart: 1.0, TimeEnd: 1.1, LineStart: 7, TraceFlavorHint: TraceFlavorHarmonyHitrace}
	stats := ComputeWindowStats(idx, q)
	row := b3ProcessRow(t, stats.ProcessCPULoad, 520)
	if row.Process.Comm != "" {
		t.Fatalf("derived process display name = %q, want empty (no confirmed registration pair): %s", row.Process.Comm, row.Summary)
	}
	for _, bad := range []string{"renderLoop", "H:innerStep", "H:LoopOnce", "H:doThing"} {
		if strings.Contains(row.Process.Comm, bad) {
			t.Fatalf("business span name %q leaked into the process display name: %s", bad, row.Summary)
		}
	}
	if !strings.HasSuffix(row.Summary, tidTgidDerivedRowMarker) {
		t.Fatalf("derived row must still carry the attribution marker: %s", row.Summary)
	}
}

// withTempTracequeryLogger installs a fresh on-disk Default logger writing to
// a per-test directory (same pattern as render.withTempLogger), restores the
// previous Default on exit, and returns a getter for the accumulated log
// content.
func withTempTracequeryLogger(t *testing.T) func() string {
	t.Helper()
	dir := t.TempDir()
	prev := logging.Default
	lg, err := logging.NewFromFlags(dir, "warning", false)
	if err != nil {
		t.Fatalf("NewFromFlags: %v", err)
	}
	logging.SetDefault(lg)
	t.Cleanup(func() {
		_ = lg.Close()
		logging.SetDefault(prev)
	})
	return func() string {
		entries, err := filepath.Glob(filepath.Join(dir, "codrax-*.log"))
		if err != nil {
			t.Fatalf("glob log files: %v", err)
		}
		var sb strings.Builder
		for _, f := range entries {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatalf("read %s: %v", f, err)
			}
			sb.Write(data)
		}
		return sb.String()
	}
}

// TestTidTgidGateDisableLogsWarning pins F4: the native-TGID disable path is
// not silent. A mixed artifact — one stray native-TGID line inside an hmtrace
// capture — disables derivation wholesale and fails open to ungrouped rows;
// without this WARN the grouping would vanish with no operator-visible trail.
func TestTidTgidGateDisableLogsWarning(t *testing.T) {
	getLogs := withTempTracequeryLogger(t)
	idx := buildTraceIndex(t, "b3_warnlog.systrace", b3AndroidFixture)
	if buildTidTgidDerivation(idx.Events) != nil {
		t.Fatal("derivation must be nil on a native-TGID index")
	}
	logs := getLogs()
	if !strings.Contains(logs, "tracequery: native TGID present") ||
		!strings.Contains(logs, "SpanPID derivation disabled for this index") {
		t.Fatalf("native-TGID disable must log the F4 warning, got logs: %q", logs)
	}
	if !strings.Contains(logs, "WARN tracequery: native TGID present") {
		t.Fatalf("F4 disable log must be at WARN level, got logs: %q", logs)
	}
}
