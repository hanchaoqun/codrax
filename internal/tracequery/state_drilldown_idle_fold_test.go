package tracequery

import (
	"fmt"
	"strings"
	"testing"
)

// TestStateDrilldownFoldsWholeWindowIdleSleepers pins the berlin.systrace
// (2026-07-03) idle-noise fix: top_sleep candidates whose cumulative sleep
// covers (>=99% of) the entire selected window are idle service threads
// (AudioOut/DNS/FFRT parked between jobs) — the customer's 101ms window
// surfaced 15+ impact=101.000 rows with zero root-cause information. They
// must leave the ranked plan and land on the typed fold summary instead.
func TestStateDrilldownFoldsWholeWindowIdleSleepers(t *testing.T) {
	stats := WindowStats{
		// 101ms window mirroring the customer case.
		Window: TimeWindow{StartTs: 100.000, EndTs: 100.101},
		SleepTop: []ThreadDuration{
			{Thread: ThreadRef{Comm: "AudioOut", PID: 11}, DurationMs: 101.0, LineStart: 10, LineEnd: 20},
			{Thread: ThreadRef{Comm: "DNSResolver", PID: 12}, DurationMs: 100.5, LineStart: 30, LineEnd: 40},
			{Thread: ThreadRef{Comm: "worker", PID: 13}, DurationMs: 80.0, LineStart: 50, LineEnd: 60},
		},
	}
	plan, fold := buildStateDrilldownPlan(stats, 12)
	if findStateDrilldownStepForTest(plan, 11, "top_sleep", string(StateSSleep)) != nil ||
		findStateDrilldownStepForTest(plan, 12, "top_sleep", string(StateSSleep)) != nil {
		t.Fatalf("whole-window sleepers must be folded out of the plan: %+v", plan)
	}
	worker := findStateDrilldownStepForTest(plan, 13, "top_sleep", string(StateSSleep))
	if worker == nil {
		t.Fatalf("sub-window sleeper must stay a ranked candidate: %+v", plan)
	}
	if fold == nil || fold.Count != 2 {
		t.Fatalf("fold must count exactly the whole-window sleepers, got %+v", fold)
	}
	if len(fold.Threads) != 2 || fold.Threads[0] != "AudioOut-11" || fold.Threads[1] != "DNSResolver-12" {
		t.Fatalf("fold must carry thread labels in SleepTop order, got %+v", fold.Threads)
	}
}

// TestStateDrilldownIdleFoldLeavesSubThresholdSleepersAlone pins the 99%
// boundary direction and the no-window guard: a sleeper below the threshold
// keeps its ranked row, and without a window duration the fold is
// structurally inert (windowMs=0 — same guard the Significant proportion
// already uses).
func TestStateDrilldownIdleFoldLeavesSubThresholdSleepersAlone(t *testing.T) {
	stats := WindowStats{
		// 1000ms window; 985ms sleep = 98.5% < 99% threshold.
		Window: TimeWindow{StartTs: 1.0, EndTs: 2.0},
		SleepTop: []ThreadDuration{
			{Thread: ThreadRef{Comm: "mostly-idle", PID: 21}, DurationMs: 985.0, LineStart: 10, LineEnd: 20},
		},
	}
	plan, fold := buildStateDrilldownPlan(stats, 12)
	if findStateDrilldownStepForTest(plan, 21, "top_sleep", string(StateSSleep)) == nil {
		t.Fatalf("98.5%% sleeper must keep its ranked row: %+v", plan)
	}
	if fold != nil {
		t.Fatalf("nothing crossed the whole-window threshold, fold must stay nil: %+v", fold)
	}

	noWindow := WindowStats{
		SleepTop: []ThreadDuration{
			{Thread: ThreadRef{Comm: "unknowable", PID: 22}, DurationMs: 5000.0, LineStart: 10, LineEnd: 20},
		},
	}
	plan, fold = buildStateDrilldownPlan(noWindow, 12)
	if findStateDrilldownStepForTest(plan, 22, "top_sleep", string(StateSSleep)) == nil {
		t.Fatalf("without a window duration the fold must not fire: %+v", plan)
	}
	if fold != nil {
		t.Fatalf("windowMs=0 must keep the fold nil: %+v", fold)
	}
}

// TestStateDrilldownIdleFoldCapsThreadListKeepsExactCount pins the fold's
// display bound: the thread-label list stops at 8 entries while the count
// stays exact for every folded sleeper.
func TestStateDrilldownIdleFoldCapsThreadListKeepsExactCount(t *testing.T) {
	stats := WindowStats{
		Window: TimeWindow{StartTs: 10.0, EndTs: 10.1},
	}
	for i := 0; i < 10; i++ {
		stats.SleepTop = append(stats.SleepTop, ThreadDuration{
			Thread:     ThreadRef{Comm: fmt.Sprintf("idle-%d", i), PID: 100 + i},
			DurationMs: 100.0,
			LineStart:  10, LineEnd: 20,
		})
	}
	plan, fold := buildStateDrilldownPlan(stats, 12)
	for i := 0; i < 10; i++ {
		if findStateDrilldownStepForTest(plan, 100+i, "top_sleep", string(StateSSleep)) != nil {
			t.Fatalf("all whole-window sleepers must be folded: %+v", plan)
		}
	}
	if fold == nil || fold.Count != 10 {
		t.Fatalf("count must stay exact beyond the list cap, got %+v", fold)
	}
	if len(fold.Threads) != 8 {
		t.Fatalf("thread-label list must cap at 8, got %d: %+v", len(fold.Threads), fold.Threads)
	}
}

// TestStateDrilldownIdleFoldExemptsPinnedTarget pins the QF2 fix: the
// query's explicitly pinned target thread (exact pid or verbatim comm match)
// is NEVER folded, even when its sleep spans the whole window — a victim UI
// thread sync-blocked across a 101ms jank window satisfies the same signal
// as an idle audio sink, and folding the investigation subject as
// no-activity noise reverses the drilldown guidance. Non-pinned whole-window
// sleepers keep folding, and near-miss identities (different pid, comm
// substring) get no exemption — the match is a precise signal, not a fuzzy
// one.
func TestStateDrilldownIdleFoldExemptsPinnedTarget(t *testing.T) {
	stats := WindowStats{
		Window: TimeWindow{StartTs: 100.000, EndTs: 100.101},
		SleepTop: []ThreadDuration{
			{Thread: ThreadRef{Comm: "UIThread", PID: 41}, DurationMs: 101.0, LineStart: 10, LineEnd: 20},
			{Thread: ThreadRef{Comm: "AudioOut", PID: 42}, DurationMs: 101.0, LineStart: 30, LineEnd: 40},
		},
	}

	// Pinned by exact pid: the target keeps its ranked drilldown row.
	plan, fold := buildStateDrilldownPlanForTarget(stats, 12, 41, "")
	if findStateDrilldownStepForTest(plan, 41, "top_sleep", string(StateSSleep)) == nil {
		t.Fatalf("pid-pinned whole-window sleeper must keep its ranked row: %+v", plan)
	}
	if findStateDrilldownStepForTest(plan, 42, "top_sleep", string(StateSSleep)) != nil {
		t.Fatalf("non-pinned whole-window sleeper must still fold: %+v", plan)
	}
	if fold == nil || fold.Count != 1 || len(fold.Threads) != 1 || fold.Threads[0] != "AudioOut-42" {
		t.Fatalf("fold must carry only the non-pinned sleeper, got %+v", fold)
	}

	// Pinned by verbatim comm: same exemption without a pid.
	plan, fold = buildStateDrilldownPlanForTarget(stats, 12, 0, "UIThread")
	if findStateDrilldownStepForTest(plan, 41, "top_sleep", string(StateSSleep)) == nil {
		t.Fatalf("comm-pinned whole-window sleeper must keep its ranked row: %+v", plan)
	}
	if fold == nil || fold.Count != 1 {
		t.Fatalf("comm pin must exempt exactly one thread, got %+v", fold)
	}

	// Near-miss identities are NOT exempt: different pid, comm substring.
	plan, fold = buildStateDrilldownPlanForTarget(stats, 12, 43, "UIThr")
	if findStateDrilldownStepForTest(plan, 41, "top_sleep", string(StateSSleep)) != nil ||
		findStateDrilldownStepForTest(plan, 42, "top_sleep", string(StateSSleep)) != nil {
		t.Fatalf("substring/other-pid selectors must not exempt anything: %+v", plan)
	}
	if fold == nil || fold.Count != 2 {
		t.Fatalf("both sleepers must fold under a non-matching pin, got %+v", fold)
	}

	// The unpinned compatibility wrapper behaves exactly as before.
	plan, fold = buildStateDrilldownPlan(stats, 12)
	if findStateDrilldownStepForTest(plan, 41, "top_sleep", string(StateSSleep)) != nil {
		t.Fatalf("unpinned wrapper must fold all whole-window sleepers: %+v", plan)
	}
	if fold == nil || fold.Count != 2 {
		t.Fatalf("unpinned wrapper fold must count both sleepers, got %+v", fold)
	}
}

// TestWindowedIndexParseCaveatReportsTruncatedBoundary pins the QF5 fix on
// the query-side caveat surface (placed in this batch-scoped file alongside
// the other 2026-07-03 berlin.systrace pins): when a windowed build's
// padding tail was truncated at the event budget, the windowed_index_parse
// caveat must report the REAL parse boundary (Index.PaddingTruncatedLastTs,
// filled parse-side) instead of the padded index end — otherwise it directly
// contradicts the PaddingTruncatedNote caveat emitted alongside.
func TestWindowedIndexParseCaveatReportsTruncatedBoundary(t *testing.T) {
	idx := &Index{
		Windowed:               true,
		IndexTimeStart:         1.000,
		IndexTimeEnd:           2.701,
		IndexLineStart:         100,
		IndexLineEnd:           900,
		ParsedKnown:            1,
		PaddingTruncated:       true,
		PaddingTruncatedNote:   "index budget hit after request window fully parsed (parsed through ts=2.310000); padding tail truncated",
		PaddingTruncatedLastTs: 2.310,
	}
	caveats := resultCaveats(idx, Query{}, Result{})
	windowed := ""
	for _, c := range caveats {
		if strings.HasPrefix(c, "windowed_index_parse=true") {
			windowed = c
			break
		}
	}
	if windowed == "" {
		t.Fatalf("windowed_index_parse caveat missing: %+v", caveats)
	}
	if !strings.Contains(windowed, "time 1.000000..2.310000 seconds") {
		t.Fatalf("caveat must report the truncated parse boundary, got: %s", windowed)
	}
	if strings.Contains(windowed, "2.701000") {
		t.Fatalf("caveat must not claim the unparsed padded end, got: %s", windowed)
	}
	if !strings.Contains(windowed, "padding tail truncated at the event budget") {
		t.Fatalf("caveat must state why the boundary is short of the padded end, got: %s", windowed)
	}
	// Non-contradiction pin: the boundary this caveat reports is the same
	// one the PaddingTruncatedNote states.
	if !strings.Contains(idx.PaddingTruncatedNote, "ts=2.310000") {
		t.Fatalf("fixture note must carry the same boundary: %s", idx.PaddingTruncatedNote)
	}

	// Control: an untruncated windowed build keeps reporting the full padded
	// range, byte-identical to the pre-QF5 caveat.
	control := &Index{
		Windowed:       true,
		IndexTimeStart: 1.000,
		IndexTimeEnd:   2.701,
		IndexLineStart: 100,
		IndexLineEnd:   900,
		ParsedKnown:    1,
	}
	caveats = resultCaveats(control, Query{}, Result{})
	found := false
	for _, c := range caveats {
		if strings.Contains(c, "time 1.000000..2.701000 seconds") && !strings.Contains(c, "padding tail truncated") {
			found = true
		}
	}
	if !found {
		t.Fatalf("untruncated build must keep the full-range caveat: %+v", caveats)
	}
}

// TestStateDrilldownIdleFoldKeepsFragmentedSleepFilter pins that the
// pre-existing fragmented-sleep filter (visible-but-non-recursive churn rows
// replace the raw top_sleep row) is untouched by the idle fold, which runs
// before it: a fragmented sleeper below the whole-window threshold is still
// dropped from top_sleep without being counted as idle.
func TestStateDrilldownIdleFoldKeepsFragmentedSleepFilter(t *testing.T) {
	stats := WindowStats{
		// 200ms window; 60ms fragmented sleep = 30%, far below the fold.
		Window: TimeWindow{StartTs: 5.0, EndTs: 5.2},
		SleepTop: []ThreadDuration{
			{Thread: ThreadRef{Comm: "fragmented", PID: 31}, DurationMs: 60, LineStart: 10, LineEnd: 20},
		},
		StateChurn: []ThreadStateChurnSummary{{
			Thread:        ThreadRef{Comm: "fragmented", PID: 31},
			DominantState: string(StateSSleep),
			SleepMs:       60,
			TotalMs:       80,
			FragmentCount: 5,
			StateSwitches: 4,
			MaxSegmentMs:  8,
			LineStart:     10,
			LineEnd:       20,
		}},
	}
	plan, fold := buildStateDrilldownPlan(stats, 12)
	if findStateDrilldownStepForTest(plan, 31, "top_sleep", string(StateSSleep)) != nil {
		t.Fatalf("fragmented sleeper must still be dropped from top_sleep: %+v", plan)
	}
	if findStateDrilldownStepForTest(plan, 31, "state_churn", string(StateSSleep)) == nil {
		t.Fatalf("fragmented sleeper must stay visible via its churn row: %+v", plan)
	}
	if fold != nil {
		t.Fatalf("fragmented sub-threshold sleeper must not be counted idle: %+v", fold)
	}
}
