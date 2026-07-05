package tracequery

// timeline_clamp_summary_pin_test.go — E1-a pin (RTC-R1 e1, 2026-07-05).
//
// Specimen shape: a scheduler interval that crosses the query window edge and
// gets clamped. The pre-fix order minted Interval.Summary at construction from
// the UNCLAMPED duration and never regenerated it after clampIntervals, while
// evidenceFromTimeline republished that stale Summary verbatim — so one tool
// result said "running ... 0.000ms" on the timeline row (clamped, correct) and
// "running for 0.987 ms" in the evidence pack (stale), and the model trusted
// the pack. These tests pin all three faces (typed fields / Summary prose /
// evidence-pack fact) to the SAME clamped value with the actual_* dual-ledger
// disclosure; restoring the old mint-before-clamp order goes red on the exact
// string assertions below.

import (
	"fmt"
	"strings"
	"testing"
)

func TestThreadTimelineClampRegeneratesSummaryAcrossFaces(t *testing.T) {
	idx := buildSampleIndex(t)
	// sampleTrace: app-20 runs 1.220000..1.260000. TimeEnd=1.22 cuts that
	// running interval to zero width — the exact RTC e1 specimen shape
	// ("running ...0.000ms" row vs "running for 40.000 ms" pack).
	tl := ThreadTimeline(idx, Query{PID: 20, TimeStart: 1.0, TimeEnd: 1.22})
	var clamped *Interval
	for i := range tl.Intervals {
		if tl.Intervals[i].State == StateRunning {
			clamped = &tl.Intervals[i]
		}
	}
	if clamped == nil {
		t.Fatalf("expected the window-edge running interval to survive clamping, got %+v", tl.Intervals)
	}
	if clamped.DurationMs != 0 {
		t.Fatalf("expected zero clamped duration at the window edge, got %+v", clamped)
	}
	if !clamped.WindowClamped() || clamped.ActualDurationMs <= 0 {
		t.Fatalf("expected WindowClamped with positive actual duration: %+v", clamped)
	}
	// Face 2 (Summary prose): regenerated from CLAMPED typed fields, with the
	// dual-ledger actual_* disclosure. A stale pre-clamp Summary would read
	// "running for 40.000 ms" and fail here.
	wantSummary := fmt.Sprintf("running for 0.000 ms actual_duration=%.3fms actual_window=%.6f..%.6f",
		clamped.ActualDurationMs, clamped.ActualStartTs, clamped.ActualEndTs)
	if clamped.Summary != wantSummary {
		t.Fatalf("clamped interval Summary not regenerated from clamped fields:\n got %q\nwant %q", clamped.Summary, wantSummary)
	}
	// Face 3 (evidence pack): evidenceFromTimeline republishes Interval.Summary
	// verbatim, so the pack row must carry the same clamped figure.
	pack := evidenceFromTimeline(tl)
	foundPackFace := false
	for _, fact := range pack {
		if fact.Summary == clamped.Summary && fact.StartTs == clamped.StartTs && fact.EndTs == clamped.EndTs {
			foundPackFace = true
		}
		if strings.Contains(fact.Summary, "running for 40.000 ms") {
			t.Fatalf("evidence pack republished the stale unclamped summary: %q", fact.Summary)
		}
	}
	if !foundPackFace {
		t.Fatalf("evidence pack lost the clamped interval summary %q: %+v", clamped.Summary, pack)
	}
	// Non-specimen guarantee: fully in-window intervals keep the historical
	// byte form with NO actual_* tokens (regeneration is a no-op for them).
	foundInside := false
	for _, it := range tl.Intervals {
		if it.State != StateSSleep {
			continue
		}
		foundInside = true
		if it.Summary != fmt.Sprintf("s_sleep for %.3f ms", it.DurationMs) {
			t.Fatalf("in-window interval summary changed form: %q", it.Summary)
		}
		if strings.Contains(it.Summary, "actual_") || it.WindowClamped() {
			t.Fatalf("unclamped interval must not carry the dual-ledger tokens: %+v", it)
		}
	}
	if !foundInside {
		t.Fatalf("expected an in-window s_sleep interval as the unclamped control: %+v", tl.Intervals)
	}
}

// Bounds-only shape (PTV4 review finding, 2026-07-05): an interval whose
// actual BOUNDS are set but whose ActualDurationMs is zero is still
// WindowClamped, and every face must publish the bounds-DERIVED duration via
// the single ActualDurationMsResolved fallback authority. A bare
// ActualDurationMs read on any face would publish actual_duration=0.000ms
// there while the Summary face carries the derived value — breaking the
// three-face-same-value pin for exactly this shape.
func TestClampIntervalsBoundsOnlyActualDurationResolvedAcrossFaces(t *testing.T) {
	boundsOnly := Interval{
		Thread:  ThreadRef{Comm: "app", PID: 20},
		State:   StateRunning,
		StartTs: 1.22, EndTs: 1.26, DurationMs: 40,
		ActualStartTs: 1.22, ActualEndTs: 1.26, // ActualDurationMs deliberately 0
		StartLine: 5, EndLine: 6,
		Summary: "running for 40.000 ms",
	}
	out := clampIntervals([]Interval{boundsOnly}, Query{TimeStart: 1.0, TimeEnd: 1.22})
	if len(out) != 1 {
		t.Fatalf("expected the bounds-only interval to survive clamping: %+v", out)
	}
	it := out[0]
	if !it.WindowClamped() || it.DurationMs != 0 || it.ActualDurationMs != 0 {
		t.Fatalf("specimen must stay bounds-only and window-clamped: %+v", it)
	}
	resolved := it.ActualDurationMsResolved()
	if resolved <= 0 {
		t.Fatalf("resolved actual duration must fall back to the bounds, got %v", resolved)
	}
	wantSummary := fmt.Sprintf("running for 0.000 ms actual_duration=%.3fms actual_window=%.6f..%.6f",
		resolved, it.ActualStartTs, it.ActualEndTs)
	if it.Summary != wantSummary {
		t.Fatalf("bounds-only Summary must use the resolved fallback duration:\n got %q\nwant %q", it.Summary, wantSummary)
	}
	if strings.Contains(it.Summary, "actual_duration=0.000ms") {
		t.Fatalf("bounds-only Summary published a bare zero actual duration: %q", it.Summary)
	}
	// Pack face: republishes the same Summary verbatim.
	pack := evidenceFromTimeline(TimelineResult{Thread: it.Thread, Intervals: out})
	if len(pack) != 1 || pack[0].Summary != it.Summary {
		t.Fatalf("pack face must carry the identical resolved summary: %+v", pack)
	}
}

func TestThreadTimelineClampPreservesBlockedReasonSuffix(t *testing.T) {
	idx := buildTraceIndex(t, "clamp_suffix.systrace", resourceTrace)
	// resourceTrace: worker-30 D-sleeps 2.030..2.150 (enriched to io_wait via
	// the sched_blocked_reason at 2.040). TimeEnd=2.10 clamps it to 70ms.
	tl := ThreadTimeline(idx, Query{PID: 30, TimeStart: 2.02, TimeEnd: 2.10})
	var clamped *Interval
	for i := range tl.Intervals {
		if tl.Intervals[i].State == StateIOWait {
			clamped = &tl.Intervals[i]
		}
	}
	if clamped == nil {
		t.Fatalf("expected the enriched io_wait interval, got %+v", tl.Intervals)
	}
	wantSummary := fmt.Sprintf("io_wait for %.3f ms actual_duration=%.3fms actual_window=%.6f..%.6f; sched_blocked_reason iowait=1 caller=fscache_page_wait_on_page_bit",
		clamped.DurationMs, clamped.ActualDurationMs, clamped.ActualStartTs, clamped.ActualEndTs)
	if clamped.Summary != wantSummary {
		t.Fatalf("clamped enriched Summary lost the suffix or the dual ledger:\n got %q\nwant %q", clamped.Summary, wantSummary)
	}
	if clamped.DurationMs >= clamped.ActualDurationMs {
		t.Fatalf("specimen must actually be clamped (in-window < actual): %+v", clamped)
	}
}
