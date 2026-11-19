package tool

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Platform semantic-work vocabulary, public entry. Every test below enters
// through the real trace_query tool with a named path. The two fixtures are
// verbatim lines of real captures (provenance in each file's header).

func hmc376Query(t *testing.T, path, view string, pid int, start, end float64) tracequery.Result {
	t.Helper()
	_, native := hmc376QueryTarget(t, path, view, map[string]any{"pid": pid, "target_scope": "thread"}, start, end)
	return native
}

// hmc376QueryTarget addresses the target with the given selector and returns
// the published tool result together with its native payload.
func hmc376QueryTarget(t *testing.T, path, view string, selector map[string]any, start, end float64) (types.ToolResult, tracequery.Result) {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	hmc176DisableExternalProviders(t)
	ctx, _, _ := hmc17NamedPathContext(t)
	params := map[string]any{"source": "path", "path": abs, "view": view, "time_start": start, "time_end": end}
	for key, value := range selector {
		params[key] = value
	}
	result := hmc17NamedQuery(t, ctx, params)
	return result, hmc367NativeResult(t, result, view)
}

func hmc376RecordNote(record types.ObservationRecord, key string) string {
	for _, note := range record.RichNotes {
		if value, ok := strings.CutPrefix(note, key+"="); ok {
			return value
		}
	}
	return ""
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
	published, result := hmc376QueryTarget(t, path, "root_cause_rank", map[string]any{"pid": target, "target_scope": "thread"}, 5775.535, 5775.792)
	rank := result.RootCauseRank
	if rank == nil {
		t.Fatal("no ranking was published")
	}
	var wait *tracequery.RootCauseRankItem
	collectorStillOnChain := false
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
	// Sibling waits must be observed, not assumed. They are published on the
	// typed semantic channel; the rank board gives them a seat only when no
	// other row needs it, and on this window it is full, so the channel is
	// where this assertion now reads them.
	siblingRows := 0
	for _, record := range published.Observations {
		if record.Predicate != "trace_semantic_span" || hmc376RecordNote(record, types.TraceNoteKeySpanName) != "GC: Wait For Completion Alloc" ||
			strings.HasSuffix(record.Subject, "-34044") {
			continue
		}
		if hmc376RecordNote(record, types.TraceNoteKeyChainRelevance) == "on_chain" {
			t.Fatalf("a sibling thread's wait for the same collection must stay off the chain: %s %v", record.Subject, record.RichNotes)
		}
		siblingRows++
	}
	if siblingRows != 7 {
		t.Fatalf("the 7 sibling waits must be present on the semantic channel (observed, not assumed), got %d", siblingRows)
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
	if platform != 13 {
		t.Fatalf("the duration seats and the 8 added seats hold all 13 collection spans, kept %d", platform)
	}
	if _, crowded := kept(hmc376SyntheticCapture(t, "H:FullGC::RunPhases;GCReason2", "GC: Wait For Completion Alloc", 30, 20)); crowded != 16 {
		t.Fatalf("31 collection spans fill 8 duration seats and 8 added seats, kept %d", crowded)
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
		if strings.Contains(caveat, "semantic span bound kept 16/21 classified span(s) across 21 family/families; omitted 5 span(s) from 5 whole family/families") {
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

// --- seats of the query target and of its dependency chain -----------------------

type hmc376Line struct {
	ts   float64
	text string
}

type hmc376Trace struct{ lines []hmc376Line }

func (c *hmc376Trace) add(ts float64, comm string, tid, tgid, cpu int, format string, args ...any) {
	c.lines = append(c.lines, hmc376Line{ts, fmt.Sprintf("%16s-%-5d (%7d) [%03d] .... %.6f: %s", comm, tid, tgid, cpu, ts, fmt.Sprintf(format, args...))})
}

func (c *hmc376Trace) run(ts float64, comm string, tid, tgid, cpu int) {
	c.add(ts, "swapper", 0, 0, cpu, "sched_switch: prev_comm=swapper prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=%s next_pid=%d next_prio=120", comm, tid)
}

func (c *hmc376Trace) sleep(ts float64, comm string, tid, tgid, cpu int) {
	c.add(ts, comm, tid, tgid, cpu, "sched_switch: prev_comm=%s prev_pid=%d prev_prio=120 prev_state=S ==> next_comm=swapper next_pid=0 next_prio=120", comm, tid)
}

func (c *hmc376Trace) span(start, end float64, comm string, tid, tgid, cpu int, name string) {
	c.add(start, comm, tid, tgid, cpu, "tracing_mark_write: B|%d|%s", tgid, name)
	c.add(end, comm, tid, tgid, cpu, "tracing_mark_write: E|%d", tgid)
}

func (c *hmc376Trace) write(t *testing.T) string {
	t.Helper()
	sort.SliceStable(c.lines, func(i, j int) bool { return c.lines[i].ts < c.lines[j].ts })
	var b strings.Builder
	b.WriteString("# tracer: nop\n")
	for _, line := range c.lines {
		b.WriteString(line.text + "\n")
	}
	path := filepath.Join(t.TempDir(), "synthetic.ftrace")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// hmc376WakerCapture: main-100 sleeps from 10.010 to 10.200 and is woken by
// waker-150, which records wakerMarker over the given interval. offChain other
// threads, in another process, run longer collections through the window.
func hmc376WakerCapture(t *testing.T, wakerMarker string, wakerStart, wakerEnd float64, offChain int) string {
	t.Helper()
	var c hmc376Trace
	c.run(10.000000, "main", 100, 100, 1)
	c.run(10.000100, "waker", 150, 100, 2)
	c.sleep(10.010000, "main", 100, 100, 1)
	for i := 0; i < offChain; i++ {
		c.span(10.001000+float64(i)*1e-5, 10.280000+float64(i)*1e-5, fmt.Sprintf("js-%d", 300+i), 300+i, 300, 3, "H:FullGC::RunPhases;GCReason2")
	}
	c.span(wakerStart, wakerEnd, "waker", 150, 100, 2, wakerMarker)
	c.add(10.200000, "waker", 150, 100, 2, "sched_wakeup: comm=main pid=100 prio=120 target_cpu=001")
	c.run(10.200100, "main", 100, 100, 1)
	c.sleep(10.300000, "main", 100, 100, 1)
	return c.write(t)
}

func hmc376OwnRow(rank *tracequery.RootCauseRankResult, tid int, typ string) *tracequery.RootCauseRankItem {
	for i := range rank.Items {
		if rank.Items[i].Thread.PID == tid && rank.Items[i].Type == typ {
			return &rank.Items[i]
		}
	}
	return nil
}

// A thread addressed by name and the same thread addressed by id get the same
// seats, whatever the number of longer sibling waits.
func TestHMC376TargetAddressedByNameSeatsLikeTargetAddressedByID(t *testing.T) {
	for _, siblings := range []int{3, 8, 20} {
		path := hmc376SyntheticCapture(t, "GC: Wait For Completion Alloc", "GC: Wait For Completion Alloc", siblings, 0)
		_, byID := hmc376QueryTarget(t, path, "root_cause_rank", map[string]any{"pid": 100}, 10.0, 10.32)
		_, byName := hmc376QueryTarget(t, path, "root_cause_rank", map[string]any{"thread": "main"}, 10.0, 10.32)
		id, name := hmc376OwnRow(byID.RootCauseRank, 100, "gc_pause"), hmc376OwnRow(byName.RootCauseRank, 100, "gc_pause")
		if id == nil || name == nil {
			t.Fatalf("%d siblings: the target's own wait lost its seat: by id=%v by name=%v\n%s", siblings, id != nil, name != nil, hmc376Board(byName.RootCauseRank))
		}
		if hmc376RowLabel(*id) != hmc376RowLabel(*name) || name.ChainRelevance != "on_chain" || name.Rank != 1 || !hmc376Near(name.EffectiveImpactMs, 199.9) {
			t.Fatalf("%d siblings: the two ways of addressing the target differ:\n%s\n%s", siblings, hmc376RowLabel(*id), hmc376RowLabel(*name))
		}
	}
}

// A collection on the thread that wakes the target is an on-chain cause. Longer
// collections in unrelated processes never evict it, and a wait recorded on
// that thread inside its chain window is treated the same way.
func TestHMC376ChainMemberCollectionKeepsItsSeat(t *testing.T) {
	for _, marker := range []string{"H:FullGC::RunPhases;GCReason2", "GC: Wait For Completion Alloc"} {
		for _, offChain := range []int{0, 7, 8, 12} {
			path := hmc376WakerCapture(t, marker, 10.050, 10.190, offChain)
			rank := hmc376Query(t, path, "root_cause_rank", 100, 10.0, 10.3).RootCauseRank
			row := hmc376OwnRow(rank, 150, "gc_pause")
			if row == nil {
				t.Fatalf("%q with %d off-chain collections: the waker's on-chain span lost its seat:\n%s", marker, offChain, hmc376Board(rank))
			}
			if row.ChainRelevance != "on_chain" || row.OnChainBasis != tracequery.RootCauseOnChainBasisSemanticChainIntervalRelation ||
				!hmc376Near(row.ImpactMs, 140) || row.Rank < 1 {
				t.Fatalf("%q with %d off-chain collections: %s", marker, offChain, hmc376RowLabel(*row))
			}
		}
	}
}

// The target's own collections keep their duration seats and take the added
// platform seats before any shorter span of another thread does.
func TestHMC376TargetCollectionsFillTheirSeatsFirst(t *testing.T) {
	var last string
	for _, others := range []int{0, 6, 8, 12} {
		var c hmc376Trace
		c.run(10.000000, "main", 100, 100, 1)
		for i := 0; i < 10; i++ {
			start := 10.001 + float64(i)*0.011
			c.span(start, start+0.010, "main", 100, 100, 1, "H:PartialGC::RunPhases0;GCReason12")
		}
		c.sleep(10.130000, "main", 100, 100, 1)
		for i := 0; i < others; i++ {
			c.span(10.002+float64(i)*1e-4, 10.007+float64(i)*1e-4, fmt.Sprintf("js-%d", 300+i), 300+i, 300, 3, "H:PartialGC::RunPhases0;GCReason12")
		}
		c.add(10.200000, "waker", 150, 100, 2, "sched_wakeup: comm=main pid=100 prio=120 target_cpu=001")
		c.run(10.200100, "main", 100, 100, 1)
		c.sleep(10.240000, "main", 100, 100, 1)
		result := hmc376Query(t, c.write(t), "root_cause_rank", 100, 10.0, 10.25)
		row := hmc376OwnRow(result.RootCauseRank, 100, "gc_pause")
		if row == nil || !hmc376Near(row.ImpactMs, 100) || !hmc376Near(result.TargetWindowStates.DeterministicRunningMs, 100) {
			t.Fatalf("%d other threads: the target keeps all ten collections (100 ms): row=%v deterministic=%.3f\n%s", others, row != nil, result.TargetWindowStates.DeterministicRunningMs, hmc376Board(result.RootCauseRank))
		}
		label := hmc376RowLabel(*row)
		if last != "" && label != last {
			t.Fatalf("%d other threads changed the target's row:\n%s\n%s", others, last, label)
		}
		last = label
	}
}

// An on-chain thread's collection that loses every seat at the bound stays
// visible on the business mention face; while it holds a seat it is only there.
func TestHMC376OmittedOnChainCollectionStaysMentioned(t *testing.T) {
	const marker = "H:FullGC::RunPhases;GCReason2"
	mention := func(rank *tracequery.RootCauseRankResult) *tracequery.BusinessSpanMention {
		if rank.BusinessSpanMentions == nil {
			return nil
		}
		for i := range rank.BusinessSpanMentions.Families {
			if family := &rank.BusinessSpanMentions.Families[i]; family.Thread.PID == 150 && family.Name == marker {
				return family
			}
		}
		return nil
	}
	// The collection runs after the wakeup, outside the waker's chain window,
	// so the seat tiers give it no priority over longer collections elsewhere.
	seated := hmc376Query(t, hmc376WakerCapture(t, marker, 10.205, 10.225, 15), "root_cause_rank", 100, 10.0, 10.3).RootCauseRank
	if hmc376OwnRow(seated, 150, "gc_pause") == nil || mention(seated) != nil {
		t.Fatalf("with a free seat the collection is a ranked row and nothing else:\n%s", hmc376Board(seated))
	}
	omitted := hmc376Query(t, hmc376WakerCapture(t, marker, 10.205, 10.225, 16), "root_cause_rank", 100, 10.0, 10.3).RootCauseRank
	family := mention(omitted)
	if hmc376OwnRow(omitted, 150, "gc_pause") != nil || family == nil {
		t.Fatalf("without a seat the collection must stay mentioned: mention=%v\n%s", family != nil, hmc376Board(omitted))
	}
	if !hmc376Near(family.TotalMs, 20) || family.OnChainBasis != tracequery.BusinessSpanMentionBasisChainMember || family.HiddenCount != 1 {
		t.Fatalf("the mention must carry the measured work and its typed basis: %+v", family)
	}
}

// --- later bounded faces --------------------------------------------------------

func hmc376Projection(result types.ToolResult) types.TraceCausalProjection {
	return types.CompileTraceCausalProjection(types.ObservationLedger{Records: result.Observations})
}

func hmc376ProjectionRow(bucket string, node types.TraceCausalProjectionNode) string {
	return fmt.Sprintf("%s|%s|%s|%s|%.3f|%d", bucket, node.Subject, node.SemanticClass, node.TypeToken, node.ImpactMS, node.MergedCount)
}

func hmc376ProjectionRows(projection types.TraceCausalProjection) []string {
	var rows []string
	for _, face := range []struct {
		bucket string
		nodes  []types.TraceCausalProjectionNode
	}{
		{"onchain", projection.OnChainCauses}, {"adjacent", projection.AdjacentCauses},
		{"background", projection.BackgroundCauses}, {"semantic", projection.SemanticSpans},
	} {
		for _, node := range face.nodes {
			rows = append(rows, hmc376ProjectionRow(face.bucket, node))
		}
	}
	return rows
}

// Seven longer sibling waits arrive in a window whose off-chain semantic detail
// is already full. Every row published without them stays published.
func TestHMC376OffChainPlatformRowsNeverTakeLaterSeats(t *testing.T) {
	publish := func(sibling string) (types.ToolResult, *tracequery.RootCauseRankResult) {
		result, native := hmc376QueryTarget(t, hmc376SyntheticCapture(t, "", sibling, 7, 16), "root_cause_rank", map[string]any{"pid": 100, "target_scope": "thread"}, 10.0, 10.32)
		return result, native.RootCauseRank
	}
	controlResult, controlRank := publish("worker idle")
	denseResult, denseRank := publish("GC: Wait For Completion Alloc")
	verification := func(result types.ToolResult) (rows []string) {
		for _, node := range hmc376Projection(result).SemanticSpans {
			if node.SemanticClass == "class_verification" {
				rows = append(rows, hmc376ProjectionRow("semantic", node))
			}
		}
		sort.Strings(rows)
		return rows
	}
	control, dense := verification(controlResult), verification(denseResult)
	if len(control) != 16 || strings.Join(control, "\n") != strings.Join(dense, "\n") {
		t.Fatalf("the semantic detail lost a class-verification row:\ncontrol %d:\n%s\ndense %d:\n%s", len(control), strings.Join(control, "\n"), len(dense), strings.Join(dense, "\n"))
	}
	published := map[string]bool{}
	for _, row := range denseRank.Items {
		published[fmt.Sprintf("%s %d %.3f", row.Type, row.Thread.PID, row.ImpactMs)] = true
	}
	for _, row := range controlRank.Items {
		if row.SpanName == "worker idle" {
			continue
		}
		if !published[fmt.Sprintf("%s %d %.3f", row.Type, row.Thread.PID, row.ImpactMs)] {
			t.Fatalf("a rank row lost its seat: %s\n%s", hmc376RowLabel(row), hmc376Board(denseRank))
		}
	}
	projected := map[string]bool{}
	for _, row := range hmc376ProjectionRows(hmc376Projection(denseResult)) {
		projected[row] = true
	}
	idleWorker := regexp.MustCompile(`\|worker-\d+\|\|trace_span\|`)
	for _, row := range hmc376ProjectionRows(hmc376Projection(controlResult)) {
		if strings.HasSuffix(row, "|0") && !idleWorker.MatchString(row) && !projected[row] {
			t.Fatalf("a projection row lost its seat: %s", row)
		}
	}
}

// hmc376BaseRows reads the rows the build without the vocabulary published.
func hmc376BaseRows(t *testing.T, scope string) (rank, projection []string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "hmc376_netease_base_rows.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if row, ok := strings.CutPrefix(line, "RANK "+scope+"|"); ok {
			rank = append(rank, row)
		}
		if row, ok := strings.CutPrefix(line, "PROJ "+scope+"|"); ok {
			projection = append(projection, row)
		}
	}
	if len(rank) == 0 || len(projection) == 0 {
		t.Fatalf("no base rows recorded for %s", scope)
	}
	return rank, projection
}

// The one row that changes identity is the target's own wait: it was an
// anonymous span and is now the classified on-chain cause.
func hmc376AssertNetEaseBaseRowsStayPublished(t *testing.T, path, scope string) {
	t.Helper()
	const target = "ease.cloudmusic-34044"
	result, native := hmc376QueryTarget(t, path, "root_cause_rank", map[string]any{"pid": 34044, "target_scope": "thread"}, 5775.535, 5775.792)
	baseRank, baseProjection := hmc376BaseRows(t, scope)
	publishedRank := map[string]bool{}
	for _, row := range native.RootCauseRank.Items {
		publishedRank[fmt.Sprintf("%s|%s-%d|%.3f|%.3f|%s", row.Type, row.Thread.Comm, row.Thread.PID, row.ImpactMs, row.EffectiveImpactMs, row.ChainRelevance)] = true
	}
	for _, row := range baseRank {
		if row == "trace_span|"+target+"|247.466|0.000|adjacent" {
			if !publishedRank["gc_pause|"+target+"|247.304|247.304|on_chain"] {
				t.Fatalf("the target's own wait must be published as the classified on-chain cause:\n%s", hmc376Board(native.RootCauseRank))
			}
			continue
		}
		if !publishedRank[row] {
			t.Errorf("rank row published before the vocabulary is gone: %s", row)
		}
	}
	rows := hmc376ProjectionRows(hmc376Projection(result))
	publishedProjection := map[string]bool{}
	mergedByBucket := map[string]int{}
	for _, row := range rows {
		publishedProjection[row] = true
		fields := strings.Split(row, "|")
		if merged, _ := strconv.Atoi(fields[len(fields)-1]); merged > mergedByBucket[fields[0]] {
			mergedByBucket[fields[0]] = merged
		}
	}
	for _, row := range baseProjection {
		fields := strings.Split(row, "|")
		if merged, _ := strconv.Atoi(fields[len(fields)-1]); merged > 0 {
			// A folded row counts its members; the fold may only grow.
			if mergedByBucket[fields[0]] < merged {
				t.Errorf("folded row published before the vocabulary shrank: %s", row)
			}
			continue
		}
		if fields[1] == target && fields[3] == "trace_span" {
			continue
		}
		if !publishedProjection[row] {
			t.Errorf("projection row published before the vocabulary is gone: %s", row)
		}
	}
}

func TestHMC376NetEaseWindowKeepsEveryPreviouslyPublishedRow(t *testing.T) {
	hmc376AssertNetEaseBaseRowsStayPublished(t, filepath.Join("testdata", "hmc376_art_gc_wait_main.ftrace"), "fixture")
}

func TestHMC376WholeOriginalNetEaseWindowKeepsEveryPreviouslyPublishedRow(t *testing.T) {
	hmc376AssertNetEaseBaseRowsStayPublished(t, hmc376Original(t, "CODRAX_HMC376_NETEASE_GCWAIT",
		"ae52285a00c4afce43e48bae519615110973052370537e7c53f5e4351073f2e0", 106869666), "whole")
}

// Addressing a real thread by name on the complete capture: the 145.305 ms wait
// of a thread that is not the first in the seat order stays its first cause.
func TestHMC376WholeOriginalTargetAddressedByName(t *testing.T) {
	path := hmc376Original(t, "CODRAX_HMC376_NETEASE_GCWAIT",
		"ae52285a00c4afce43e48bae519615110973052370537e7c53f5e4351073f2e0", 106869666)
	_, byID := hmc376QueryTarget(t, path, "root_cause_rank", map[string]any{"pid": 34235}, 5774.0, 5777.0)
	_, byName := hmc376QueryTarget(t, path, "root_cause_rank", map[string]any{"thread": "DefaultExecutor"}, 5774.0, 5777.0)
	id, name := hmc376OwnRow(byID.RootCauseRank, 34235, "gc_pause"), hmc376OwnRow(byName.RootCauseRank, 34235, "gc_pause")
	if id == nil || name == nil || hmc376RowLabel(*id) != hmc376RowLabel(*name) || !hmc376Near(name.EffectiveImpactMs, 145.305) {
		t.Fatalf("the target addressed by name must keep its 145.305 ms wait: by id=%v by name=%v", id != nil, name != nil)
	}
}

// --- the Vulkan pipeline marker in a real render-service capture ------------------

// The fixture is exported from the callstack, sched_slice and instant tables of
// a real capture (provenance in its header): the five pipeline creations, their
// ancestors and the shader loads nested in them, with their recorded names,
// times and nesting.
func TestHMC376VulkanPipelineCreationFromARealCapture(t *testing.T) {
	const fixture, start, end = "hmc376_vk_pipeline_render_service.ftrace", 1474.453, 1474.517
	path := filepath.Join("testdata", fixture)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	wrappers := 0
	for _, line := range strings.Split(string(raw), "\n") {
		_, marker, ok := strings.Cut(line, "tracing_mark_write: B|1833|")
		if !ok || !strings.HasPrefix(marker, "H:ShaderDfx") {
			continue
		}
		wrappers++
		hit, known := tracequery.TraceSpanVocabulary(marker)
		if !known || hit.ID != "rs.shaderdfx" || tracequery.TraceSpanSemanticClass(marker) != "" {
			t.Fatalf("the diagnostics wrapper is a recognised marker without a class: %q %+v", marker, hit)
		}
	}
	if wrappers != 5 {
		t.Fatalf("expected the 5 diagnostics wrappers of the capture, got %d", wrappers)
	}

	stats := hmc376Query(t, path, "window_stats", 2424, start, end).WindowStats
	if stats == nil {
		t.Fatal("no window statistics were published")
	}
	durations := map[string][]string{}
	for _, span := range stats.TraceSpans {
		if span.SemanticClass != "" {
			if span.SemanticClass != "shader_compile" {
				t.Fatalf("unexpected class on %q: %s", span.Name, span.SemanticClass)
			}
			durations[span.Name] = append(durations[span.Name], fmt.Sprintf("%s-%d %.6f", span.Thread.Comm, span.Thread.PID, span.DurationMs))
		}
	}
	want := map[string][]string{
		"H:vkCreateGraphicsPipelines": {"DDGRTask__0-2997 3.374844", "DDGRTask__1-2998 4.929219", "RSUniRenderThre-2424 4.334688", "RSUniRenderThre-2424 6.555312", "RSUniRenderThre-2424 8.572968"},
		"H:Load/Compile Shaders":      {"DDGRTask__0-2997 1.569219", "DDGRTask__1-2998 3.582969", "RSUniRenderThre-2424 4.140782", "RSUniRenderThre-2424 6.278593", "RSUniRenderThre-2424 8.406094"},
	}
	for name, rows := range want {
		sort.Strings(durations[name])
		if strings.Join(durations[name], ",") != strings.Join(rows, ",") {
			t.Fatalf("%s:\n got %v\nwant %v", name, durations[name], rows)
		}
	}

	// On the queried render thread the creation and the load nested in it are
	// one stretch of work: 6.555312 + 4.334688 ms outside the chain windows,
	// counted once.
	result := hmc376Query(t, path, "root_cause_rank", 2424, start, end)
	row := hmc376OwnRow(result.RootCauseRank, 2424, "shader_compile")
	if row == nil || row.OnChainBasis != tracequery.RootCauseOnChainBasisSelfDeterministicSpan || row.ChainRelevance != "on_chain" ||
		row.Rank != 1 || row.Tier != "primary" || !hmc376Near(row.EffectiveImpactMs, 10.890) || row.SpanName != "H:vkCreateGraphicsPipelines" {
		t.Fatalf("the pipeline creation on the queried thread must be its first cause, nested work counted once:\n%s", hmc376Board(result.RootCauseRank))
	}
}
