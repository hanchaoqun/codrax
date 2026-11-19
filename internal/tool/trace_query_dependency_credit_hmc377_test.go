package tool

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// HMC377 E2 (owner ruling 2026-11-18, batch 377): a thread that only
// delivered a wakeup to the analysed thread is a cause of the analysed
// thread's delay for the part of its own ready / blocked time / deterministic
// work that lies inside the wait that wakeup ended, and for nothing else.
// Time before that wait began, between waits, or after the wakeup stays
// published as supporting context. Every test here runs through the public
// trace_query entry (root_cause_rank view), zero model calls.
//
// Synthetic capture, window [1000.000000, 1000.500000), analysed thread
// app.main-1100:
//
//	S 0.300 ms  [1000.200000, 1000.200300]  ended by dep.small-5500  (ready 0.250 ms inside)
//	D 5.000 ms  [1000.250000, 1000.255000]  ended by lock.holder-6600 (ready 4.000 ms inside, 30.000 ms long before)
//	S 0.100 ms  [1000.310000, 1000.310100]  ended by bg.logger-2200  (ready 0.050 ms inside, 480.000 ms outside)
//	S 40.000 ms [1000.400000, 1000.440000]  ended by dep.worker-4400 (ready 30.000 ms inside, 20.000 ms before)
//
// The expected numbers are hand-computed from the rows of the capture (and
// reproduced by the standalone raw oracle of the batch lane), never read from
// the engine.
func TestHMC377DependencyCreditIsBoundedByTheWaitTheWakeupEnded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dep_credit_min.ftrace")
	if err := os.WriteFile(path, []byte(hmc377DependencyCreditTrace()), 0o644); err != nil {
		t.Fatal(err)
	}
	hmc176DisableExternalProviders(t)
	ctx, _, _ := hmc17NamedPathContext(t)
	const tid = 1100
	r := hmc17NamedQuery(t, ctx, map[string]any{"source": "path", "path": path, "view": "root_cause_rank", "pid": tid, "target_scope": "thread", "time_start": 1000.0, "time_end": 1000.5})
	value := hmc367NativeResult(t, r, "root_cause_rank")
	near := func(a, b float64) bool { return math.Abs(a-b) <= 0.0015 }

	st := value.TargetWindowStates
	if st == nil || !near(st.SleepMs, 40.400) || !near(st.DStateMs+st.IOWaitMs, 5.000) || !near(st.RunnableMs, 0.040) {
		t.Fatalf("the analysed thread's state partition disagrees with the capture: %+v", st)
	}
	waitingMs := st.SleepMs + st.DStateMs + st.IOWaitMs // 45.400
	rank := value.RootCauseRank
	if rank == nil {
		t.Fatal("no native ranking")
	}
	type halves struct{ chain, rest float64 }
	byThread := map[int]*halves{}
	otherWaiting := 0.0
	for _, row := range rank.Items {
		if row.Thread.PID == tid || row.Rank <= 0 {
			continue
		}
		if !hmc377WaitingSeat(row) {
			continue
		}
		h := byThread[row.Thread.PID]
		if h == nil {
			h = &halves{}
			byThread[row.Thread.PID] = h
		}
		if row.ChainRelevance == "on_chain" {
			h.chain += row.EffectiveImpactMs
			otherWaiting += row.EffectiveImpactMs
		} else if row.Type == "runnable_wait" && row.ChainAnchorRemainderSeat {
			h.rest += row.CumulativeImpactMs
		}
	}
	// (1) Invariant: what other threads are charged with, as causes of this
	// thread's waiting, cannot exceed the time this thread spent waiting.
	if otherWaiting > waitingMs+0.0015 {
		t.Errorf("other threads' on-chain ready/blocked time totals %.3fms, more than the %.3fms the analysed thread spent waiting", otherWaiting, waitingMs)
	}
	want := []struct {
		name        string
		pid         int
		chain, rest float64
	}{
		// (2) Background: one wakeup that ended a 0.100 ms wait.
		{"bg.logger", 2200, 0.050, 480.000},
		// (3) Uninterruptible variant, lowest-priority thread the target
		// waited for: the overlap stays a cause, the earlier ready time does
		// not.
		{"lock.holder", 6600, 4.000, 30.000},
		// (4) Fully inside the wait it ended: the whole account stays a cause.
		{"dep.small", 5500, 0.250, 0},
		// (5) Control — the long wait the chain expansion walks: unchanged.
		{"dep.worker", 4400, 30.000, 20.000},
	}
	got := hmc377HalvesDump(byThread)
	for _, w := range want {
		h := byThread[w.pid]
		if h == nil {
			t.Errorf("%s-%d has no ranked seat (published: %s)", w.name, w.pid, got)
			continue
		}
		if !near(h.chain, w.chain) {
			t.Errorf("%s-%d is charged %.3fms as a cause; inside the wait(s) its wakeup ended it was ready for %.3fms", w.name, w.pid, h.chain, w.chain)
		}
		if !near(h.rest, w.rest) {
			t.Errorf("%s-%d: time outside those waits must stay published as supporting context: got %.3fms want %.3fms", w.name, w.pid, h.rest, w.rest)
		}
	}
	// (6) The disclosed wakeup of a priced share is the one that closed the
	// priced wait, and the via word names the host's own direct wakeup.
	for _, row := range rank.Items {
		if row.OnChainBasis != tracequery.RootCauseOnChainBasisHostWakeupEdgeState {
			continue
		}
		wantTs := map[int]float64{2200: 1000.310100, 6600: 1000.255000, 5500: 1000.200300}[row.Thread.PID]
		if wantTs == 0 || math.Abs(row.HostWakeupEdgeAnchorTs-wantTs) > 1e-7 || row.HostWakeupEdgeAnchorVia != tracequery.HostWakeupEdgeAnchorViaDirect {
			t.Errorf("%s-%d discloses wakeup %.6f via %q; want the wakeup %.6f that closed the priced wait, via direct", row.Thread.Comm, row.Thread.PID, row.HostWakeupEdgeAnchorTs, row.HostWakeupEdgeAnchorVia, wantTs)
		}
	}
}

// TestHMC377SemanticWorkIsOnChainOnlyInsideTheWaitTheWakeupEnded — the same
// bound for deterministic work (class verification, texture upload) done by a
// thread other than the analysed one: it is a cause of the analysed thread's
// delay only for the part that lies inside a wait which that thread's own
// wakeup ended. Work that finished before the wait began, and work of a
// thread whose wakeup found the analysed thread running, stay published as
// optimisation context and never take a root-cause seat.
func TestHMC377SemanticWorkIsOnChainOnlyInsideTheWaitTheWakeupEnded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dep_credit_spans.ftrace")
	if err := os.WriteFile(path, []byte(hmc377DependencyCreditTraceWith(true)), 0o644); err != nil {
		t.Fatal(err)
	}
	hmc176DisableExternalProviders(t)
	ctx, _, _ := hmc17NamedPathContext(t)
	r := hmc17NamedQuery(t, ctx, map[string]any{"source": "path", "path": path, "view": "root_cause_rank", "pid": 1100, "target_scope": "thread", "time_start": 1000.0, "time_end": 1000.5})
	value := hmc367NativeResult(t, r, "root_cause_rank")
	rank := value.RootCauseRank
	if rank == nil {
		t.Fatal("no native ranking")
	}
	near := func(a, b float64) bool { return math.Abs(a-b) <= 0.0015 }
	onChain := map[string]float64{}
	elsewhere := map[string]float64{}
	var dump []string
	for _, row := range rank.Items {
		if row.SemanticClass == "" {
			continue
		}
		key := fmt.Sprintf("%s-%d/%s", row.Thread.Comm, row.Thread.PID, row.SemanticClass)
		dump = append(dump, fmt.Sprintf("%s lane=%s rank=%d effective=%.3f raw=%.3f", key, row.ChainRelevance, row.Rank, row.EffectiveImpactMs, row.CumulativeImpactMs))
		if row.ChainRelevance == "on_chain" && row.Rank > 0 {
			onChain[key] += row.EffectiveImpactMs
		} else {
			elsewhere[key] += row.CumulativeImpactMs
		}
	}
	sort.Strings(dump)
	board := strings.Join(dump, "\n")
	// lock.holder: only the 0.300 ms inside the D wait it ended (not the
	// 0.080 ms verification finished 120 ms earlier).
	if got := onChain["lock.holder-6600/class_verification"]; !near(got, 0.300) {
		t.Errorf("lock.holder class verification charged %.3fms; only 0.300ms lies inside the wait its wakeup ended\n%s", got, board)
	}
	// The earlier 0.080 ms verification is context only: published off the
	// chain with its raw value, or displaced by the board cap — never priced.
	if got := elsewhere["lock.holder-6600/class_verification"]; got != 0 && !near(got, 0.080) {
		t.Errorf("lock.holder's earlier class verification must stay context with its raw 0.080ms, got %.3fms\n%s", got, board)
	}
	// bg.logger: 0.040 ms of the 0.130 ms span lies inside the 0.100 ms wait;
	// the 0.090 ms after the wakeup stays published as context.
	if got := onChain["bg.logger-2200/class_verification"]; !near(got, 0.040) {
		t.Errorf("bg.logger class verification charged %.3fms; only 0.040ms lies inside the wait its wakeup ended\n%s", got, board)
	}
	if got := elsewhere["bg.logger-2200/class_verification"]; !near(got, 0.090) {
		t.Errorf("bg.logger's verification after the wakeup must stay published as context (0.090ms), got %.3fms\n%s", got, board)
	}
	// lock.holder JIT compilation reaching over both ends of the 5.000 ms wait
	// it ended: 5.000 ms on the chain; the 120.045 ms outside (119.995 before
	// the wait + 0.050 after the wakeup) stays published as context — its
	// value is the sum of the two pieces, never their 125.045 ms extent.
	if got := onChain["lock.holder-6600/jit_compile"]; !near(got, 5.000) {
		t.Errorf("lock.holder JIT compilation charged %.3fms; 5.000ms lies inside the wait its wakeup ended\n%s", got, board)
	}
	if got := elsewhere["lock.holder-6600/jit_compile"]; !near(got, 120.045) {
		t.Errorf("lock.holder JIT compilation outside the wait must stay published as context with 120.045ms (two pieces), got %.3fms\n%s", got, board)
	}
	// cpu.hog: its wakeup found the analysed thread running — no cause, the
	// work stays visible with its raw 5.000 ms.
	if got := onChain["cpu.hog-3300/texture_upload"]; got > 0.0015 {
		t.Errorf("cpu.hog texture upload charged %.3fms although its wakeup ended no wait of the analysed thread\n%s", got, board)
	}
	if got := elsewhere["cpu.hog-3300/texture_upload"]; !near(got, 5.000) {
		t.Errorf("cpu.hog texture upload must stay published as context with its raw 5.000ms, got %.3fms\n%s", got, board)
	}
}

func hmc377WaitingSeat(row tracequery.RootCauseRankItem) bool {
	return row.DominantState == "runnable" || row.DominantState == "d_sleep" || row.DominantState == "io_wait"
}

func hmc377HalvesDump[T any](m map[int]*T) string {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%d: %+v", k, *m[k]))
	}
	return strings.Join(parts, "; ")
}

func hmc377DependencyCreditTrace() string { return hmc377DependencyCreditTraceWith(false) }

// hmc377DependencyCreditTraceWith optionally adds deterministic semantic spans
// on threads other than the analysed one:
//
//	lock.holder  VerifyClass com.example.Early  1000.130010..1000.130090 (0.080 ms) long before the wait it later ends
//	lock.holder  VerifyClass com.example.Inside 1000.254600..1000.254900 (0.300 ms) inside the D wait it ends
//	bg.logger    VerifyClass com.example.Across 1000.310060..1000.310190 (0.130 ms) 0.040 ms inside the wait, 0.090 ms after the wakeup
//	cpu.hog      Texture upload                  1000.290000..1000.295000 (5.000 ms) then wakes app.main at 1000.300000 while app.main is running
//	lock.holder  JIT compiling …Holder.run()     1000.130005..1000.255050 (125.045 ms, enclosing both verifications) reaching
//	             over both ends of the D wait it ends: 5.000 ms inside, 119.995 ms before the wait began, 0.050 ms after the wakeup
func hmc377DependencyCreditTraceWith(spans bool) string {
	type task struct {
		comm string
		pid  int
		prio int
		tgid int
	}
	idle0, idle1 := task{"swapper/0", 0, 120, 0}, task{"swapper/1", 0, 120, 0}
	T := task{"app.main", 1100, 110, 1100}
	X := task{"cpu.hog", 3300, 120, 3300}
	B := task{"bg.logger", 2200, 130, 2200}
	K := task{"lock.holder", 6600, 139, 1100}
	D := task{"dep.small", 5500, 120, 1100}
	W := task{"dep.worker", 4400, 120, 1100}
	type line struct {
		us   int64
		seq  int
		text string
	}
	var rows []line
	row := func(us int64, cpu int, header task, body string) {
		rows = append(rows, line{us, len(rows), fmt.Sprintf("%16s-%-5d (%5d) [%03d] .... %d.%06d: %s", header.comm, header.pid, header.tgid, cpu, us/1000000, us%1000000, body)})
	}
	sw := func(us int64, cpu int, prev task, state string, next task) {
		row(us, cpu, next, fmt.Sprintf("sched_switch: prev_comm=%s prev_pid=%d prev_prio=%d prev_state=%s ==> next_comm=%s next_pid=%d next_prio=%d", prev.comm, prev.pid, prev.prio, state, next.comm, next.pid, next.prio))
	}
	wk := func(us int64, cpu int, waker, wakee task, target int) {
		row(us, cpu, waker, fmt.Sprintf("sched_wakeup: comm=%s pid=%d prio=%d target_cpu=%03d", wakee.comm, wakee.pid, wakee.prio, target))
	}
	span := func(begin, end int64, cpu int, owner task, name string) {
		row(begin, cpu, owner, fmt.Sprintf("tracing_mark_write: B|%d|%s", owner.tgid, name))
		row(end, cpu, owner, fmt.Sprintf("tracing_mark_write: E|%d", owner.tgid))
	}
	sw(999990000, 0, idle0, "R", T)
	sw(999990010, 1, idle1, "R", X)
	wk(1000010000, 1, X, B, 1)
	wk(1000100000, 1, X, K, 1)
	sw(1000130000, 1, X, "R", K)
	sw(1000130100, 1, K, "S", X)
	sw(1000200000, 0, T, "S", idle0)
	wk(1000200020, 1, X, D, 1)
	sw(1000200270, 1, X, "R", D)
	wk(1000200300, 1, D, T, 0)
	sw(1000200310, 0, idle0, "R", T)
	sw(1000200400, 1, D, "S", X)
	sw(1000250000, 0, T, "D", idle0)
	wk(1000250500, 1, X, K, 1)
	sw(1000254500, 1, X, "R", K)
	wk(1000255000, 1, K, T, 0)
	sw(1000255010, 0, idle0, "R", T)
	sw(1000255100, 1, K, "S", X)
	sw(1000310000, 0, T, "S", idle0)
	sw(1000310050, 1, X, "R", B)
	wk(1000310100, 1, B, T, 0)
	sw(1000310110, 0, idle0, "R", T)
	sw(1000310200, 1, B, "S", X)
	wk(1000320000, 1, X, B, 1)
	wk(1000360000, 1, X, W, 1)
	sw(1000380000, 1, X, "R", W)
	sw(1000381000, 1, W, "S", X)
	sw(1000400000, 0, T, "S", idle0)
	wk(1000405000, 1, X, W, 1)
	sw(1000435000, 1, X, "R", W)
	wk(1000440000, 1, W, T, 0)
	sw(1000440010, 0, idle0, "R", T)
	sw(1000441000, 1, W, "S", X)
	sw(1000510000, 0, T, "S", idle0)
	sw(1000510010, 1, X, "S", idle1)
	if spans {
		span(1000130010, 1000130090, 1, K, "VerifyClass com.example.Early")
		span(1000254600, 1000254900, 1, K, "VerifyClass com.example.Inside")
		span(1000310060, 1000310190, 1, B, "VerifyClass com.example.Across")
		span(1000290000, 1000295000, 1, X, "Texture upload")
		// B/E marks nest per thread: the enclosing compilation opens first and
		// closes last.
		row(1000130005, 1, K, fmt.Sprintf("tracing_mark_write: B|%d|JIT compiling void com.example.Holder.run() (kind=Optimized)", K.tgid))
		row(1000255050, 1, K, fmt.Sprintf("tracing_mark_write: E|%d", K.tgid))
		wk(1000300000, 1, X, T, 0) // the analysed thread is running: this wakeup ends no wait
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].us != rows[j].us {
			return rows[i].us < rows[j].us
		}
		return rows[i].seq < rows[j].seq
	})
	var b strings.Builder
	b.WriteString("# tracer: nop\n#\n#           TASK-PID    TGID   CPU#  ||||    TIMESTAMP  FUNCTION\n#              | |        |      |   ||||       |         |\n")
	for _, r := range rows {
		b.WriteString(r.text)
		b.WriteString("\n")
	}
	return b.String()
}

// --- whole originals (opt-in; skip when the variable is unset, fail when the
// file is not the pinned original) ------------------------------------------

// TestHMC377DouyinRebootDependencyCreditStaysInsideTheWaits — Douyin Lite,
// device reboot cold start, capture 5, TID 11105, window
// [140.911304, 141.835458) = 924.154 ms. Independent raw oracle (batch lane
// dep_oracle.py): the target waited 315.621 ms (S 273.049 + D/IO 42.572);
// 1413 wakeup rows from 99 threads; the ready + D time of every waking thread
// inside the waits its own wakeups ended totals 21.376 ms; logd.writer-7067
// ended one 0.072 ms wait and was ready 0.050 ms inside it (729.909 ms in the
// window); #bd-ticket-io-3-11316 was ready 8.626 ms inside the 8.659 ms wait
// it ended. The largest share of a single thread inside the waits it ended is
// 8.676 ms; of an indivisible inversion seat 0.518 ms (sysmgr-reclaim0).
func TestHMC377DouyinRebootDependencyCreditStaysInsideTheWaits(t *testing.T) {
	path := hmc376Original(t, "CODRAX_HMC377_DOUYIN_REBOOT",
		"8087138a5810347675fe7764974f5103cad4a9a71e56d7d1702d65da2256892e", 138438125)
	hmc176DisableExternalProviders(t)
	ctx, _, _ := hmc17NamedPathContext(t)
	const tid = 11105
	r := hmc17NamedQuery(t, ctx, map[string]any{"source": "path", "path": path, "view": "root_cause_rank", "pid": tid, "target_scope": "thread", "time_start": 140.911304, "time_end": 141.835458})
	value := hmc367NativeResult(t, r, "root_cause_rank")
	rank := value.RootCauseRank
	if rank == nil || value.TargetWindowStates == nil {
		t.Fatal("no native ranking")
	}
	st := value.TargetWindowStates
	waitingMs := st.SleepMs + st.DStateMs + st.IOWaitMs
	if math.Abs(waitingMs-315.621) > 0.002 {
		t.Fatalf("target waiting drifted from the raw oracle: %.3f", waitingMs)
	}
	otherWaiting, hostEdgeState := 0.0, 0.0
	var ticket *tracequery.RootCauseRankItem
	var board []string
	for i := range rank.Items {
		row := &rank.Items[i]
		board = append(board, hmc376RowLabel(*row))
		if row.Thread.PID != tid && row.Rank > 0 && row.ChainRelevance == "on_chain" && hmc377WaitingSeat(*row) {
			otherWaiting += row.EffectiveImpactMs
		}
		if row.OnChainBasis == tracequery.RootCauseOnChainBasisHostWakeupEdgeState {
			hostEdgeState += row.EffectiveImpactMs
		}
		if row.Thread.PID == 7067 && row.ChainRelevance == "on_chain" && row.EffectiveImpactMs > 0.050+0.002 {
			t.Errorf("logd.writer-7067 is charged %.3fms as a cause; inside the one wait its wakeup ended it was ready for 0.050ms", row.EffectiveImpactMs)
		}
		if row.Thread.PID == 7812 && row.ChainRelevance == "on_chain" && row.EffectiveImpactMs > 0.222+0.002 {
			t.Errorf("android.bg-7812 is charged %.3fms as a cause; inside the waits its wakeups ended it was ready for 0.222ms", row.EffectiveImpactMs)
		}
		if row.Thread.PID == 11316 && row.Rank > 0 && row.ChainRelevance == "on_chain" && row.RunnableMs > 0 {
			ticket = row
		}
	}
	if otherWaiting > waitingMs+0.002 {
		t.Errorf("other threads' on-chain ready/blocked seats total %.3fms, more than the %.3fms the analysed thread waited\n%s", otherWaiting, waitingMs, strings.Join(board, "\n"))
	}
	if hostEdgeState > 21.376+0.002 {
		t.Errorf("seats priced by a wakeup toward the analysed thread total %.3fms; the raw share inside the waits those wakeups ended is 21.376ms", hostEdgeState)
	}
	// The real dependency the old reading evicted from the board returns.
	if ticket == nil || math.Abs(ticket.RunnableMs-8.626) > 0.05 {
		t.Errorf("#bd-ticket-io-3-11316 (ready 8.626ms inside the 8.659ms wait it ended) must hold a published chain seat\n%s", strings.Join(board, "\n"))
	}
	for _, d := range rank.GatedCompositeEdgeShareDisclosures {
		if d.PreMs > 0.518+0.002 {
			t.Errorf("disclosure %s-%d claims %.3fms inside the waits its wakeups ended; the raw maximum is 0.518ms", d.Thread.Comm, d.Thread.PID, d.PreMs)
		}
	}
}

// TestHMC377HoneybirdRebootKeepsTheWalkedDependencyFirst — the positive
// long-window control: Honeybird (蜂鸟众包) device reboot cold start, capture
// 5, TID 10566, window [162.372184, 163.616645) = 1244.461 ms. Raw: the
// target waited 940.056 ms; SGLOAD-10769 was ready 135.656 ms inside the
// 237.962 ms wait it ended (the chain expansion walks it); android.bg-7768's
// wakeups ended 0.036 ms of waits.
func TestHMC377HoneybirdRebootKeepsTheWalkedDependencyFirst(t *testing.T) {
	path := hmc376Original(t, "CODRAX_HMC377_HONEYBIRD_REBOOT",
		"54405241db707b901057b41a5c94e69a3fc233425fdf7a1f745e43585cf248d4", 124269104)
	hmc176DisableExternalProviders(t)
	ctx, _, _ := hmc17NamedPathContext(t)
	const tid = 10566
	r := hmc17NamedQuery(t, ctx, map[string]any{"source": "path", "path": path, "view": "root_cause_rank", "pid": tid, "target_scope": "thread", "time_start": 162.372184, "time_end": 163.616645})
	value := hmc367NativeResult(t, r, "root_cause_rank")
	rank := value.RootCauseRank
	if rank == nil || value.TargetWindowStates == nil {
		t.Fatal("no native ranking")
	}
	st := value.TargetWindowStates
	waitingMs := st.SleepMs + st.DStateMs + st.IOWaitMs
	otherWaiting := 0.0
	var first *tracequery.RootCauseRankItem
	var board []string
	for i := range rank.Items {
		row := &rank.Items[i]
		board = append(board, hmc376RowLabel(*row))
		if row.Rank == 1 && row.ChainRelevance == "on_chain" {
			first = row
		}
		if row.Thread.PID != tid && row.Rank > 0 && row.ChainRelevance == "on_chain" && hmc377WaitingSeat(*row) {
			otherWaiting += row.EffectiveImpactMs
		}
		if row.Thread.PID == 7768 && row.ChainRelevance == "on_chain" && row.EffectiveImpactMs > 0.036+0.002 {
			t.Errorf("android.bg-7768 is charged %.3fms as a cause; its wakeups ended 0.036ms of waits", row.EffectiveImpactMs)
		}
	}
	if first == nil || first.Thread.PID != 10769 || math.Abs(first.RunnableMs-135.656) > 0.002 {
		t.Errorf("SGLOAD-10769 (ready 135.656ms inside the 237.962ms wait it ended) must stay the first cause\n%s", strings.Join(board, "\n"))
	}
	if otherWaiting > waitingMs+0.002 {
		t.Errorf("other threads' on-chain ready/blocked seats total %.3fms, more than the %.3fms the analysed thread waited", otherWaiting, waitingMs)
	}
}
