package tool

// trace_query_wakeup_edge_census_test.go — WAKE-CENSUS publication pins
// (立案 §29.58, 2026-07-13, docs/design/real_trace_campaign_20260705.md;
// PRC-F1 witness「OS_IPC_14_34911 ×4」三重假; WAKE-CENSUS-D 2A 换源 §29.58.4:
// the count is now the window-total raw sched_wakeup inventory with the typed
// exit split — the assertions below track that caliber).
//
// The load-bearing red/green pin: the census record's count must equal the
// engine's window-total raw inventory while the per-edge observation rows
// stop at the typed family row cap. MUTATION self-checks:
//   - re-deriving the census count from the capped edge-row face (the exact
//     PRC-F1 defect direction: 截断库存二次聚合) reds
//     TestTraceQueryWakeupEdgeCensusCountsFullInventoryPastRowCap — the
//     emitted Value would read the row cap (32 by default), not the true 35;
//   - dropping the tool-face cap-adjust arithmetic reds
//     TestTraceQueryWakeupEdgeCensusRowCapAdjustsOverflow — the overflow
//     notes would under-report the pairs/edges this face itself trimmed.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// wakeupCensusOverCapTrace generates a production-parseable trace where
// target app-100 sleeps `cycles` times, each sleep woken by the SAME waker
// (waker-200) — so the engine chain mints `cycles` edges of ONE pair, which
// overflows the typed family row cap on the per-edge face while the census
// keeps the whole-inventory count (产线实铸形纪律: the engine parses this
// text and BuildWakeupChain itself mints every edge and the census).
func wakeupCensusOverCapTrace(cycles int) string {
	var b strings.Builder
	b.WriteString("        app-100 (100) [001] .... 4.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52\n")
	for i := 0; i < cycles; i++ {
		base := 5.0 + float64(i)*0.010
		fmt.Fprintf(&b, "        app-100 (100) [001] .... %.6f: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120\n", base)
		fmt.Fprintf(&b, "       waker-200 (200) [002] .... %.6f: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001\n", base+0.002)
		fmt.Fprintf(&b, "        app-100 (100) [001] .... %.6f: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52\n", base+0.0021)
	}
	return b.String()
}

func buildWakeupCensusOverCapChain(t *testing.T, cycles int) tracequery.ChainResult {
	t.Helper()
	path := filepath.Join(t.TempDir(), "wake_census_overcap.systrace")
	if err := os.WriteFile(path, []byte(wakeupCensusOverCapTrace(cycles)), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	chain := tracequery.BuildWakeupChain(idx, tracequery.Query{
		PID: 100, TimeStart: 5.0, TimeEnd: 5.0 + float64(cycles)*0.010 + 0.005,
		MaxDepth: 4, MaxBranches: cycles + 5, MinDurationMs: 0.5,
		TraceFlavorHint: tracequery.TraceFlavorHarmonyHitrace,
	})
	return chain
}

// TestTraceQueryWakeupEdgeCensusCountsFullInventoryPastRowCap — the census
// count is the whole-inventory truth while the per-edge rows are capped.
func TestTraceQueryWakeupEdgeCensusCountsFullInventoryPastRowCap(t *testing.T) {
	cycles := traceQueryWidthTypedFamilyRowCap() + 3
	chain := buildWakeupCensusOverCapChain(t, cycles)
	if len(chain.Edges) != cycles {
		t.Fatalf("fixture drifted: engine must mint %d edges (one per sleep cycle), got %d", cycles, len(chain.Edges))
	}
	result := tracequery.Result{
		View: "wakeup_chain", SourcePath: "/traces/wake_census_overcap.systrace",
		TimeStart: 5.0, TimeEnd: 5.0 + float64(cycles)*0.010 + 0.005,
		WakeupChain: &chain,
	}
	records := traceQueryTypedObservations(result, "wake_census_overcap.systrace", "payload-ref", "raw-ref", "", time.Unix(1752300000, 0).UTC())

	edgeRows := 0
	var censusRecords []types.ObservationRecord
	for _, record := range records {
		switch record.Predicate {
		case "wakeup_chain_edge":
			edgeRows++
		case "wakeup_edge_census":
			censusRecords = append(censusRecords, record)
		}
	}
	// The per-edge face is capped — this is the truncated inventory a census
	// must NEVER be re-derived from.
	if want := traceQueryWidthTypedFamilyRowCap(); edgeRows != want {
		t.Fatalf("per-edge rows must stop at the typed family row cap %d, got %d", want, edgeRows)
	}
	if len(censusRecords) != 1 {
		t.Fatalf("one waker → wakee pair must mint exactly one census record, got %d", len(censusRecords))
	}
	census := censusRecords[0]
	// THE pin: whole-inventory count (35 with the default cap of 32) — a
	// census aggregated from the capped edge-row face reads 32 here and reds.
	if census.Value != fmt.Sprintf("%d", cycles) {
		t.Fatalf("census count must cover the FULL edge inventory (%d), got %q", cycles, census.Value)
	}
	if census.Subject != "waker-200" || census.Object != "app-100" {
		t.Fatalf("census direction must be the sched_wakeup source truth waker-200 → app-100, got %s → %s", census.Subject, census.Object)
	}
	notes := map[string]string{}
	for _, note := range census.RichNotes {
		if key, value, ok := strings.Cut(note, "="); ok {
			notes[key] = value
		}
	}
	if notes[types.TraceNoteKeyWakeupEdgeCensusFirstTs] != "5.002000" {
		t.Fatalf("census first_ts must be the first cycle's wakeup, got %q", notes[types.TraceNoteKeyWakeupEdgeCensusFirstTs])
	}
	if want := fmt.Sprintf("%.6f", 5.0+float64(cycles-1)*0.010+0.002); notes[types.TraceNoteKeyWakeupEdgeCensusLastTs] != want {
		t.Fatalf("census last_ts must be the last cycle's wakeup %s, got %q", want, notes[types.TraceNoteKeyWakeupEdgeCensusLastTs])
	}
	// One pair, below every cap: the overflow notes must be ABSENT (absence
	// ⇔ 0 ⇔ complete enumeration — the context feed's completeness gate).
	for _, key := range []string{types.TraceNoteKeyWakeupEdgeCensusOverflowPairs, types.TraceNoteKeyWakeupEdgeCensusOverflowEdges} {
		if _, ok := notes[key]; ok {
			t.Fatalf("zero overflow must omit %s, got notes %v", key, census.RichNotes)
		}
	}
	// WAKE-CENSUS-D 2A (§29.58.4): the typed exit split rides the record —
	// every cycle here is an S-sleep exit — and the summary speaks the
	// window-total caliber (never the retired observed-edges wording).
	if notes[types.TraceNoteKeyWakeupEdgeCensusSleepExit] != fmt.Sprintf("%d", cycles) {
		t.Fatalf("census sleep_exit must count every S exit (%d), got notes %v", cycles, census.RichNotes)
	}
	for _, key := range []string{types.TraceNoteKeyWakeupEdgeCensusDExit, types.TraceNoteKeyWakeupEdgeCensusOtherExit} {
		if _, ok := notes[key]; ok {
			t.Fatalf("zero exit buckets zero-drop their notes, got %s in %v", key, census.RichNotes)
		}
	}
	if !strings.Contains(census.Summary, "window-total") || strings.Contains(census.Summary, "every measured wakeup edge of this pair") {
		t.Fatalf("census summary must speak the window-total caliber: %q", census.Summary)
	}
}

// TestTraceQueryWakeupEdgeCensusBannerFace — the query-time text face carries
// the census rows and the explicit overflow (blocked_reason_census banner
// 同构), so a reader of the edge rows never re-counts them.
func TestTraceQueryWakeupEdgeCensusBannerFace(t *testing.T) {
	chain := tracequery.ChainResult{
		Target: tracequery.ThreadRef{Comm: "app", PID: 100},
		Window: tracequery.TimeWindow{StartTs: 5.0, EndTs: 6.0},
		WakeupEdgeCensus: []tracequery.WakeupEdgeCensusPair{{
			Waker: tracequery.ThreadRef{Comm: "waker", PID: 200},
			Wakee: tracequery.ThreadRef{Comm: "app", PID: 100},
			Count: 35, SleepExitCount: 23, DExitCount: 12,
			FirstTs: 5.002, LastTs: 5.342,
		}},
		WakeupEdgeCensusOverflowPairs: 2,
		WakeupEdgeCensusOverflowEdges: 5,
	}
	result := tracequery.Result{View: "wakeup_chain", SourcePath: "/traces/x.systrace", TimeStart: 5.0, TimeEnd: 6.0, WakeupChain: &chain}
	summary := traceQuerySummary(result, traceQueryParams{View: "wakeup_chain"}, "x.systrace", "payload-ref")
	for _, want := range []string{
		// WAKE-CENSUS-D 2A: the exit split rides the banner row (one label
		// renderer with the engine face).
		"- wakeup_edge_census waker-200 -> app-100 count=35 sleep_exit=23 d_exit=12 other_exit=0 first=5.002000 last=5.342000",
		"- wakeup_edge_census_overflow pairs=2 edges=5 (beyond the census pair cap)",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("wakeup_chain banner missing %q:\n%s", want, summary)
		}
	}
	// Zero overflow keeps the banner silent on the overflow line.
	chain.WakeupEdgeCensusOverflowPairs, chain.WakeupEdgeCensusOverflowEdges = 0, 0
	summary = traceQuerySummary(result, traceQueryParams{View: "wakeup_chain"}, "x.systrace", "payload-ref")
	if strings.Contains(summary, "wakeup_edge_census_overflow") {
		t.Fatalf("zero census overflow must not mint an overflow banner line:\n%s", summary)
	}
}

// TestTraceQueryWakeupEdgeCensusRowCapAdjustsOverflow — when THIS face's row
// cap trims census pairs (a config-lowered cap; the engine pair cap of 16
// sits below the default row cap of 32, so production defaults never reach
// this arm), the trimmed pairs/edges fold into the disclosed overflow — the
// notes never under-report. The census pairs here wake NON-target threads,
// so the plain-trim arm (no immunity) stays exercised. Hand-built census
// rows: the engine pair-cap const cannot mint more pairs than 16, so the
// defensive arithmetic is only reachable with a constructed over-cap list
// (the engine-minted lane is pinned by the full-inventory test above).
func TestTraceQueryWakeupEdgeCensusRowCapAdjustsOverflow(t *testing.T) {
	rowCap := traceQueryWidthTypedFamilyRowCap()
	var census []tracequery.WakeupEdgeCensusPair
	for i := 0; i < rowCap+2; i++ {
		census = append(census, tracequery.WakeupEdgeCensusPair{
			Waker: tracequery.ThreadRef{Comm: fmt.Sprintf("waker%03d", i), PID: 200 + i},
			Wakee: tracequery.ThreadRef{Comm: "mid", PID: 900},
			Count: 3, FirstTs: 5.001, LastTs: 5.009,
		})
	}
	chain := tracequery.ChainResult{
		Target:           tracequery.ThreadRef{Comm: "app", PID: 100},
		Window:           tracequery.TimeWindow{StartTs: 5.0, EndTs: 6.0},
		WakeupEdgeCensus: census,
		// The engine's own pair-cap overflow rides beneath the face trim.
		WakeupEdgeCensusOverflowPairs: 4,
		WakeupEdgeCensusOverflowEdges: 9,
	}
	result := tracequery.Result{View: "wakeup_chain", SourcePath: "/traces/x.systrace", TimeStart: 5.0, TimeEnd: 6.0, WakeupChain: &chain}
	records := traceQueryTypedObservations(result, "x.systrace", "payload-ref", "raw-ref", "", time.Unix(1752300000, 0).UTC())
	var censusRecords []types.ObservationRecord
	for _, record := range records {
		if record.Predicate == "wakeup_edge_census" {
			censusRecords = append(censusRecords, record)
		}
	}
	if len(censusRecords) != rowCap {
		t.Fatalf("the census face must stop at the row cap %d, got %d", rowCap, len(censusRecords))
	}
	// Adjusted disclosure: engine overflow (4 pairs / 9 edges) + the 2 pairs
	// (2×3 edges) trimmed by THIS face.
	wantPairs := fmt.Sprintf("%s=%d", types.TraceNoteKeyWakeupEdgeCensusOverflowPairs, 4+2)
	wantEdges := fmt.Sprintf("%s=%d", types.TraceNoteKeyWakeupEdgeCensusOverflowEdges, 9+6)
	for _, record := range censusRecords {
		joined := strings.Join(record.RichNotes, "\n")
		if !strings.Contains(joined, wantPairs) || !strings.Contains(joined, wantEdges) {
			t.Fatalf("every census record must disclose the ADJUSTED overflow (%s, %s), got %v", wantPairs, wantEdges, record.RichNotes)
		}
	}
}

// TestTraceQueryWakeupEdgeCensusRowCapTargetImmunity — 修复轮 件5 (P3-2):
// the tool-face trim carries the engine's target-wakee immunity — a
// config-lowered row cap must never re-evict the anchor's own waker pairs
// (the donghu shape). The two target-wakee pairs sit at the END of the
// census (the exact rows an order-blind [:rowCap] trim drops); both must
// publish, with the evicted non-target pairs folded into the disclosure.
func TestTraceQueryWakeupEdgeCensusRowCapTargetImmunity(t *testing.T) {
	rowCap := traceQueryWidthTypedFamilyRowCap()
	var census []tracequery.WakeupEdgeCensusPair
	for i := 0; i < rowCap; i++ {
		census = append(census, tracequery.WakeupEdgeCensusPair{
			Waker: tracequery.ThreadRef{Comm: fmt.Sprintf("waker%03d", i), PID: 200 + i},
			Wakee: tracequery.ThreadRef{Comm: "mid", PID: 900},
			Count: 3, FirstTs: 5.001, LastTs: 5.009,
		})
	}
	census = append(census,
		tracequery.WakeupEdgeCensusPair{
			Waker: tracequery.ThreadRef{Comm: "zzz-waker1", PID: 700},
			Wakee: tracequery.ThreadRef{Comm: "app", PID: 100},
			Count: 1, FirstTs: 5.100, LastTs: 5.100,
		},
		tracequery.WakeupEdgeCensusPair{
			Waker: tracequery.ThreadRef{Comm: "zzz-waker2", PID: 701},
			Wakee: tracequery.ThreadRef{Comm: "app", PID: 100},
			Count: 1, FirstTs: 5.200, LastTs: 5.200,
		},
	)
	chain := tracequery.ChainResult{
		Target:           tracequery.ThreadRef{Comm: "app", PID: 100},
		Window:           tracequery.TimeWindow{StartTs: 5.0, EndTs: 6.0},
		WakeupEdgeCensus: census,
	}
	result := tracequery.Result{View: "wakeup_chain", SourcePath: "/traces/x.systrace", TimeStart: 5.0, TimeEnd: 6.0, WakeupChain: &chain}
	records := traceQueryTypedObservations(result, "x.systrace", "payload-ref", "raw-ref", "", time.Unix(1752300000, 0).UTC())
	targetPairs, censusRecords := 0, 0
	var overflowNote string
	for _, record := range records {
		if record.Predicate != "wakeup_edge_census" {
			continue
		}
		censusRecords++
		if record.Object == "app-100" {
			targetPairs++
		}
		for _, note := range record.RichNotes {
			if strings.HasPrefix(note, types.TraceNoteKeyWakeupEdgeCensusOverflowPairs+"=") {
				overflowNote = note
			}
		}
	}
	if targetPairs != 2 {
		t.Fatalf("BOTH target-wakee pairs must survive the row-cap trim, got %d of %d census records", targetPairs, censusRecords)
	}
	if censusRecords != rowCap {
		t.Fatalf("immunity fills the remaining seats in census order (want %d records), got %d", rowCap, censusRecords)
	}
	// Two non-target pairs (2x3 edges) were evicted in the target pairs' favor.
	if want := fmt.Sprintf("%s=2", types.TraceNoteKeyWakeupEdgeCensusOverflowPairs); overflowNote != want {
		t.Fatalf("the eviction must fold into the disclosed overflow (%s), got %q", want, overflowNote)
	}
}
