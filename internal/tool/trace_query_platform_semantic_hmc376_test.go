package tool

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// Platform semantic-work vocabulary, public entry. Every test below enters
// through the real trace_query tool with a named path. The two fixtures are
// verbatim lines of real captures (provenance in each file's header).

func hmc376Query(t *testing.T, path, view string, pid int, start, end float64) tracequery.Result {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	hmc176DisableExternalProviders(t)
	ctx, _, _ := hmc17NamedPathContext(t)
	result := hmc17NamedQuery(t, ctx, map[string]any{
		"source": "path", "path": abs, "view": view, "pid": pid, "target_scope": "thread",
		"time_start": start, "time_end": end,
	})
	return hmc367NativeResult(t, result, view)
}

func hmc376Near(got, want float64) bool { return math.Abs(got-want) < 0.0015 }

func hmc376RowLabel(row tracequery.RootCauseRankItem) string {
	return fmt.Sprintf("%s %s-%d wall=%.3f effective=%.3f tier=%s rank=%d background_rank=%d relevance=%s basis=%s span=%q",
		row.Type, row.Thread.Comm, row.Thread.PID, row.ImpactMs, row.EffectiveImpactMs, row.Tier, row.Rank,
		row.BackgroundRank, row.ChainRelevance, row.OnChainBasis, row.SpanName)
}

func hmc376Board(rank *tracequery.RootCauseRankResult) string {
	var lines []string
	for _, row := range rank.Items {
		lines = append(lines, hmc376RowLabel(row))
	}
	return strings.Join(lines, "\n")
}

// A full collection that runs on the queried thread is deterministic work of
// that thread: it must be the ranked on-chain cause, and the running time it
// occupies must be accounted as deterministic work. Its nested steps are not
// additional causes.
func hmc376AssertArkTSLocalGC(t *testing.T, path string, wantSteps bool) {
	t.Helper()
	const target = 55300
	result := hmc376Query(t, path, "root_cause_rank", target, 2493.675, 2493.760)
	rank := result.RootCauseRank
	if rank == nil {
		t.Fatal("no ranking was published")
	}
	var collection *tracequery.RootCauseRankItem
	for i := range rank.Items {
		row := &rank.Items[i]
		if row.Type != "gc_pause" {
			continue
		}
		for _, step := range []string{"FullGC::Mark", "FullGC::Sweep", "GC::MarkRoots", "GC::ProcessWorkloads", "TriggerMainThreadLocalGC"} {
			if strings.Contains(row.SpanName, step) || strings.Contains(strings.Join(row.MemberRoster, "\n"), step) {
				t.Fatalf("a nested step or a request marker was ranked as a collection of its own: %s", hmc376RowLabel(*row))
			}
		}
		if row.Thread.PID == target {
			if collection != nil {
				t.Fatalf("the one collection on the queried thread was ranked twice:\n%s", hmc376Board(rank))
			}
			collection = row
		}
	}
	if collection == nil {
		t.Fatalf("the 75.656 ms collection on the queried thread is not a ranked cause:\n%s", hmc376Board(rank))
	}
	if collection.ChainRelevance != "on_chain" || collection.Rank != 1 || collection.Tier != "primary" ||
		collection.OnChainBasis != tracequery.RootCauseOnChainBasisSelfDeterministicSpan ||
		!hmc376Near(collection.ImpactMs, 75.656) || !hmc376Near(collection.EffectiveImpactMs, 75.656) ||
		!strings.HasPrefix(collection.SpanName, "H:FullGC::RunPhases") {
		t.Fatalf("the collection on the queried thread must be the first on-chain cause with its measured 75.656 ms: %s", hmc376RowLabel(*collection))
	}
	account := result.TargetWindowStates
	if account == nil || !hmc376Near(account.RunningMs, 74.837) || !hmc376Near(account.DeterministicRunningMs, 73.164) {
		t.Fatalf("running time occupied by the collection is not accounted as deterministic work: %+v", account)
	}
	for _, caveat := range rank.Caveats {
		if strings.Contains(caveat, "naming convention may have changed") &&
			(strings.Contains(caveat, "GC::MarkRoots") || strings.Contains(caveat, "FullGC::")) {
			t.Fatalf("a known collection step is still reported as a naming-drift suspect: %s", caveat)
		}
	}
	stats := hmc376Query(t, path, "window_stats", target, 2493.675, 2493.760).WindowStats
	if stats == nil {
		t.Fatal("no window statistics were published")
	}
	collections, markStep := 0, false
	for _, span := range stats.TraceSpans {
		if span.Thread.PID != target && !strings.Contains(span.Name, "TriggerMainThreadLocalGC") {
			continue
		}
		if strings.HasPrefix(span.Name, "H:FullGC::RunPhases") {
			if span.SemanticClass != "gc_pause" || span.Category != "garbage_collection" || !hmc376Near(span.DurationMs, 75.656) {
				t.Fatalf("the collection span lost its class or its interval: %q class=%q category=%q %.3f ms", span.Name, span.SemanticClass, span.Category, span.DurationMs)
			}
			collections++
			continue
		}
		if span.SemanticClass != "" {
			t.Fatalf("only the collection itself carries a class on the queried thread: %q class=%q", span.Name, span.SemanticClass)
		}
		markStep = markStep || (span.Name == "H:FullGC::Mark" && hmc376Near(span.DurationMs, 71.280))
	}
	if collections != 1 {
		t.Fatalf("exactly one collection span is expected on the queried thread, got %d", collections)
	}
	// In the cut fixture the 71.280 ms mark step is among the listed spans, so
	// "the step is not a class" is observed, not assumed. (In the complete
	// capture longer ordinary spans of other processes take the listing seats.)
	if wantSteps && !markStep {
		t.Fatal("the nested 71.280 ms mark step must be listed as an ordinary span")
	}
}

// A thread that waits for a collection to finish is stalled by it: on the
// queried thread the wait is the ranked on-chain cause. The same wait seen by
// sibling threads, and the collector's own cycle, stay background.
func hmc376AssertARTCollectionWait(t *testing.T, path string) {
	t.Helper()
	const target, collector = 34044, 34060
	result := hmc376Query(t, path, "root_cause_rank", target, 5775.535, 5775.792)
	rank := result.RootCauseRank
	if rank == nil {
		t.Fatal("no ranking was published")
	}
	var wait *tracequery.RootCauseRankItem
	collectorStillOnChain, siblingRows := false, 0
	for i := range rank.Items {
		row := &rank.Items[i]
		if row.Thread.PID == collector && row.Type == "priority_inversion_candidate" && row.ChainRelevance == "on_chain" {
			collectorStillOnChain = true
		}
		if row.Thread.PID == collector && row.SemanticClass != "" {
			t.Fatalf("a marker of the collector's own cycle carries a work class on the board: %s", hmc376RowLabel(*row))
		}
		if row.Type != "gc_pause" {
			continue
		}
		switch row.Thread.PID {
		case target:
			wait = row
		case collector:
			t.Fatalf("the collector's own cycle was promoted to a ranked collection row: %s", hmc376RowLabel(*row))
		default:
			if row.ChainRelevance != "background" || row.EffectiveImpactMs != 0 || row.Rank != 0 || row.BackgroundRank <= 0 ||
				row.OnChainBasis == tracequery.RootCauseOnChainBasisHostWakeupEdge {
				t.Fatalf("a sibling thread's wait for the same collection must stay background: %s", hmc376RowLabel(*row))
			}
			siblingRows++
		}
	}
	if wait == nil {
		t.Fatalf("the 247.466 ms wait on the queried thread is not a ranked cause:\n%s", hmc376Board(rank))
	}
	// The span lasts 247.466 ms; 247.304 ms of it is the thread's sleep on
	// the dependency chain, which is what the row is priced with.
	if wait.ChainRelevance != "on_chain" || wait.Rank != 1 || wait.Tier != "primary" ||
		!hmc376Near(wait.ImpactMs, 247.304) || !hmc376Near(wait.EffectiveImpactMs, 247.304) || !hmc376Near(wait.ActualImpactMs, 247.466) ||
		wait.SpanName != "GC: Wait For Completion Alloc" {
		t.Fatalf("the wait on the queried thread must be the first on-chain cause: %s", hmc376RowLabel(*wait))
	}
	if siblingRows < 2 {
		t.Fatalf("sibling waits must be present on the background board (observed, not assumed), got %d:\n%s", siblingRows, hmc376Board(rank))
	}
	if !collectorStillOnChain {
		t.Fatalf("the collector thread lost its existing on-chain scheduling row:\n%s", hmc376Board(rank))
	}
	stats := hmc376Query(t, path, "window_stats", target, 5775.535, 5775.792).WindowStats
	if stats == nil {
		t.Fatal("no window statistics were published")
	}
	for _, work := range stats.AsyncFileWork {
		if strings.Contains(work.Name, "GC: Wait For Completion") {
			t.Fatalf("a wait for a collection is published as file work: %+v", work)
		}
	}
	waits, cycle := 0, false
	for _, span := range stats.TraceSpans {
		if span.Thread.PID == collector {
			if span.SemanticClass != "" || span.Category == "garbage_collection" {
				t.Fatalf("a collector-cycle marker carries a work class: %q class=%q category=%q", span.Name, span.SemanticClass, span.Category)
			}
			cycle = cycle || (span.Name == "Background concurrent copying GC" && hmc376Near(span.DurationMs, 252.744))
		}
		if span.Name == "GC: Wait For Completion Alloc" {
			if span.SemanticClass != "gc_pause" || span.Category != "garbage_collection" {
				t.Fatalf("a wait for a collection lost its class: %s-%d class=%q category=%q", span.Thread.Comm, span.Thread.PID, span.SemanticClass, span.Category)
			}
			waits++
		}
	}
	// Eight threads wait for this collection: the queried thread and seven
	// siblings. The collector's cycle overlaps the window for 252.744 ms and
	// is listed as an ordinary span.
	if waits != 8 || !cycle {
		t.Fatalf("expected the 8 waits and the collector's 252.744 ms cycle among the listed spans: waits=%d cycle=%v", waits, cycle)
	}
}

func TestHMC376ArkTSLocalGCOnTargetIsDeterministicWork(t *testing.T) {
	hmc376AssertArkTSLocalGC(t, filepath.Join("testdata", "hmc376_arkts_fullgc_main.ftrace"), true)
}

func TestHMC376ARTGCWaitOnTargetIsOnChainAndSiblingsStayBackground(t *testing.T) {
	hmc376AssertARTCollectionWait(t, filepath.Join("testdata", "hmc376_art_gc_wait_main.ftrace"))
}

// hmc376SyntheticCapture writes one process (100): its main thread sleeps for
// 200 ms inside targetMarker and is woken by a runtime thread; siblings
// worker-200.. each record one siblingMarker span that starts earlier and
// ends later (so every sibling span is longer than the target's); verifiers
// verify-300.. each record one class-verification span.
func hmc376SyntheticCapture(t *testing.T, targetMarker, siblingMarker string, siblings, verifiers int) string {
	t.Helper()
	var b strings.Builder
	add := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	add("# tracer: nop")
	add("            main-100   (    100) [001] ....  10.000000: sched_switch: prev_comm=swapper prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=main next_pid=100 next_prio=120")
	for i := 0; i < siblings; i++ {
		add("          worker-%d   (    100) [002] ....  10.0010%02d: tracing_mark_write: B|100|%s", 200+i, i, siblingMarker)
	}
	for i := 0; i < verifiers; i++ {
		add("          verify-%d   (    100) [003] ....  10.0020%02d: tracing_mark_write: B|100|VerifyClass com.example.Type%d", 300+i, i, i)
	}
	if targetMarker != "" {
		add("            main-100   (    100) [001] ....  10.050000: tracing_mark_write: B|100|%s", targetMarker)
	}
	add("            main-100   (    100) [001] ....  10.050100: sched_switch: prev_comm=main prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=swapper next_pid=0 next_prio=120")
	add("  HeapTaskDaemon-150   (    100) [004] ....  10.250000: sched_wakeup: comm=main pid=100 prio=120 target_cpu=001")
	add("            main-100   (    100) [001] ....  10.250100: sched_switch: prev_comm=swapper prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=main next_pid=100 next_prio=120")
	if targetMarker != "" {
		add("            main-100   (    100) [001] ....  10.250200: tracing_mark_write: E|100")
	}
	for i := 0; i < verifiers; i++ {
		add("          verify-%d   (    100) [003] ....  10.2600%02d: tracing_mark_write: E|100", 300+i, 2*i)
	}
	for i := 0; i < siblings; i++ {
		add("          worker-%d   (    100) [002] ....  10.3010%02d: tracing_mark_write: E|100", 200+i, i)
	}
	add("            main-100   (    100) [001] ....  10.320000: sched_switch: prev_comm=main prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=swapper next_pid=0 next_prio=120")
	path := filepath.Join(t.TempDir(), "synthetic.ftrace")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// The spans a platform marker newly classifies never compete with spans that
// were already classified: the kept set of the latter is the same with and
// without the former in the window.
func TestHMC376PlatformSpansNeverTakeBuiltInSeats(t *testing.T) {
	kept := func(path string) (verification []string, platform int) {
		stats := hmc376Query(t, path, "window_stats", 100, 10.0, 10.32).WindowStats
		if stats == nil {
			t.Fatal("no window statistics were published")
		}
		for _, span := range stats.TraceSpans {
			switch span.SemanticClass {
			case "class_verification":
				verification = append(verification, fmt.Sprintf("%s-%d %s %.3f", span.Thread.Comm, span.Thread.PID, span.Name, span.DurationMs))
			case "gc_pause":
				platform++
			}
		}
		return verification, platform
	}
	without, none := kept(hmc376SyntheticCapture(t, "", "worker idle", 12, 20))
	with, platform := kept(hmc376SyntheticCapture(t, "H:FullGC::RunPhases;GCReason2", "GC: Wait For Completion Alloc", 12, 20))
	if len(without) != 16 || none != 0 {
		t.Fatalf("control window must keep 16 of 20 class-verification spans and no collection span: kept=%d collection=%d", len(without), none)
	}
	if strings.Join(with, "\n") != strings.Join(without, "\n") {
		t.Fatalf("platform spans changed the kept class-verification set:\nwithout:\n%s\nwith:\n%s", strings.Join(without, "\n"), strings.Join(with, "\n"))
	}
	if platform != 8 {
		t.Fatalf("13 collection spans must fill their own 8 seats, kept %d", platform)
	}
}

// Twenty sibling threads wait for the same collection longer than the
// queried thread does. The queried thread's own wait keeps its seat and is
// the ranked on-chain cause; the omitted sibling families are counted.
func TestHMC376TargetPlatformFamilyKeepsItsSeat(t *testing.T) {
	path := hmc376SyntheticCapture(t, "GC: Wait For Completion Alloc", "GC: Wait For Completion Alloc", 20, 0)
	rank := hmc376Query(t, path, "root_cause_rank", 100, 10.0, 10.32).RootCauseRank
	if rank == nil {
		t.Fatal("no ranking was published")
	}
	var own *tracequery.RootCauseRankItem
	for i := range rank.Items {
		row := &rank.Items[i]
		if row.Type != "gc_pause" {
			continue
		}
		if row.Thread.PID == 100 {
			own = row
		} else if row.ChainRelevance != "background" || row.EffectiveImpactMs != 0 {
			t.Fatalf("a sibling thread's wait must stay background: %s", hmc376RowLabel(*row))
		}
	}
	if own == nil {
		t.Fatalf("the queried thread's own wait lost its seat to longer sibling waits:\n%s\ncaveats: %s", hmc376Board(rank), strings.Join(rank.Caveats, "\n"))
	}
	// The span lasts 200.2 ms; 199.9 ms of it is the thread's sleep on the
	// dependency chain, which is what the row is priced with.
	if own.ChainRelevance != "on_chain" || own.Rank != 1 || own.Tier != "primary" ||
		!hmc376Near(own.ImpactMs, 199.9) || !hmc376Near(own.EffectiveImpactMs, 199.9) {
		t.Fatalf("the queried thread's own wait must be the first on-chain cause: %s", hmc376RowLabel(*own))
	}
	disclosed := false
	for _, caveat := range rank.Caveats {
		if strings.Contains(caveat, "semantic span bound kept 8/21 classified span(s) across 21 family/families; omitted 13 span(s) from 13 whole family/families") {
			disclosed = true
		}
	}
	if !disclosed {
		t.Fatalf("the omitted sibling families must be counted in the disclosure: %s", strings.Join(rank.Caveats, "\n"))
	}
}

func hmc376Original(t *testing.T, variable, digest string, size int64) string {
	t.Helper()
	path := os.Getenv(variable)
	if path == "" {
		t.Skipf("requires the complete original capture in %s", variable)
	}
	if gotDigest, gotSize := hmc176bRepresentativeDigest(t, path); gotDigest != digest || gotSize != size {
		t.Fatalf("%s is not the original capture: sha256=%s bytes=%d", variable, gotDigest, gotSize)
	}
	return path
}

// The same two assertions on the complete original captures the fixtures
// were cut from.
func TestHMC376WholeOriginalArkTSLocalGC(t *testing.T) {
	hmc376AssertArkTSLocalGC(t, hmc376Original(t, "CODRAX_HMC376_GALLERY_FULLGC",
		"d27ff641787dfa0768928e0ce3d11a7d2a6caa0f2910c941fae3e97f2256e22e", 245420208), false)
}

func TestHMC376WholeOriginalARTGCWait(t *testing.T) {
	hmc376AssertARTCollectionWait(t, hmc376Original(t, "CODRAX_HMC376_NETEASE_GCWAIT",
		"ae52285a00c4afce43e48bae519615110973052370537e7c53f5e4351073f2e0", 106869666))
}
