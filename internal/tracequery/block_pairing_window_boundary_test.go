package tracequery

import (
	"math"
	"testing"
)

// ENG audit #2 (P1, §29.25 处置委托 2026-07-10): computeBlockIOLatencies used
// to drop done endpoints from the INPUT stream when ev.Ts < q.TimeStart while
// admitting starts unconditionally. A pre-window complete therefore left its
// lane open at depth 1, and the next same-identity complete in the window
// paired against the stale start — minting a fabricated cross-boundary
// latency with zero caveats (the discarded done also never reached
// UnpairedDoneCount while the pre-window start still bumped Count: the
// account contradicted itself). Pairing now runs over the whole line-window
// stream like the interrupt/trace-mark lanes; window relevance is decided per
// completed pair.

func windowBlockLatencyRow(stats WindowStats, dev string) *StorageLatencySummary {
	for i := range stats.StorageLatencyByLayer {
		row := &stats.StorageLatencyByLayer[i]
		if row.Layer == "block" && row.Dev == dev {
			return row
		}
	}
	return nil
}

func TestBlockPairingPreWindowDoneClosesLaneNoFabricatedLatency(t *testing.T) {
	idx := buildTraceIndex(t, "block_boundary.systrace", `
 io-40 (40) [003] .... 1.000000: block_rq_issue: 8,0 R 4096 () 1000 + 8 [io]
 irq-2 (2) [003] .... 1.200000: block_rq_complete: 8,0 R () 1000 + 8 [0]
 irq-2 (2) [003] .... 2.500000: block_rq_complete: 8,0 R () 1000 + 8 [0]
`)
	stats := ComputeWindowStats(idx, Query{TimeStart: 2.0, TimeEnd: 3.0})
	if len(stats.IOLatencies) != 0 {
		t.Fatalf("pre-window pair must close its lane; the orphan in-window complete minted a fabricated cross-boundary latency: %+v", stats.IOLatencies)
	}
	row := windowBlockLatencyRow(stats, "8,0")
	if row == nil || row.UnpairedDoneCount != 1 || row.PairedCount != 0 {
		t.Fatalf("orphan in-window complete must be booked as unpaired done: %+v", row)
	}
	if containsSubstring(stats.Caveats, "block_io_pairing_ambiguous") {
		t.Fatalf("no ambiguity exists in this shape: %+v", stats.Caveats)
	}
	if !containsSubstring(stats.Caveats, "block_io_pairing_unpaired") {
		t.Fatalf("unpaired done must be disclosed: %+v", stats.Caveats)
	}
}

func TestBlockPairingFullWindowControlKeepsExactPair(t *testing.T) {
	idx := buildTraceIndex(t, "block_full_window.systrace", `
 io-40 (40) [003] .... 1.000000: block_rq_issue: 8,0 R 4096 () 1000 + 8 [io]
 irq-2 (2) [003] .... 1.200000: block_rq_complete: 8,0 R () 1000 + 8 [0]
 irq-2 (2) [003] .... 2.500000: block_rq_complete: 8,0 R () 1000 + 8 [0]
`)
	stats := ComputeWindowStats(idx, Query{TimeStart: 0.5, TimeEnd: 3.0})
	if len(stats.IOLatencies) != 1 || math.Abs(stats.IOLatencies[0].DurationMs-200) > 0.001 {
		t.Fatalf("full-window ground truth is one 200ms pair: %+v", stats.IOLatencies)
	}
	row := windowBlockLatencyRow(stats, "8,0")
	if row == nil || row.PairedCount != 1 || row.UnpairedDoneCount != 1 {
		t.Fatalf("full-window account is one pair plus one orphan done: %+v", row)
	}
}

// The false-ambiguity variant: a completed pre-window pair plus a genuine
// in-window pair on the same identity used to be judged an overlapping cohort
// (because the pre-window done was invisible) — the genuine pair was withheld
// with a factually wrong 'overlapping identical endpoint identities' caveat.
func TestBlockPairingIdentityReuseAcrossBoundaryIsNotAmbiguous(t *testing.T) {
	for _, tc := range []struct {
		name, issue1, done1, issue2, done2 string
		wantMs                             float64
	}{
		{
			name:   "sector_identity_reuse",
			issue1: ` io-40 (40) [003] .... 1.000000: block_rq_issue: 8,0 R 4096 () 1000 + 8 [io]`,
			done1:  ` irq-2 (2) [003] .... 1.200000: block_rq_complete: 8,0 R () 1000 + 8 [0]`,
			issue2: ` io-40 (40) [003] .... 2.200000: block_rq_issue: 8,0 R 4096 () 1000 + 8 [io]`,
			done2:  ` irq-2 (2) [003] .... 2.500000: block_rq_complete: 8,0 R () 1000 + 8 [0]`,
			wantMs: 300,
		},
		{
			// Flush requests share one identity per device (F/FS 0+0), so this
			// shape needs no sector reuse coincidence at all — periodic
			// journal/fsync flushes crossing time_start made the false cohort
			// suppression near-certain on real windowed queries.
			name:   "flush_shared_identity",
			issue1: ` jbd2-88 (88) [001] .... 1.000000: block_rq_issue: 8,0 FS 0 () 0 + 0 [jbd2]`,
			done1:  ` irq-2 (2) [001] .... 1.100000: block_rq_complete: 8,0 FS () 0 + 0 [0]`,
			issue2: ` app-20 (20) [002] .... 2.200000: block_rq_issue: 8,0 FS 0 () 0 + 0 [app]`,
			done2:  ` irq-2 (2) [002] .... 2.450000: block_rq_complete: 8,0 FS () 0 + 0 [0]`,
			wantMs: 250,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			idx := buildTraceIndex(t, "block_reuse_"+tc.name+".systrace",
				tc.issue1+"\n"+tc.done1+"\n"+tc.issue2+"\n"+tc.done2+"\n")
			stats := ComputeWindowStats(idx, Query{TimeStart: 2.0, TimeEnd: 3.0})
			if len(stats.IOLatencies) != 1 || math.Abs(stats.IOLatencies[0].DurationMs-tc.wantMs) > 0.001 {
				t.Fatalf("the genuine in-window pair must publish (%vms): %+v", tc.wantMs, stats.IOLatencies)
			}
			if containsSubstring(stats.Caveats, "block_io_pairing_ambiguous") {
				t.Fatalf("sequential (never overlapping) requests must not be called ambiguous: %+v", stats.Caveats)
			}
			row := windowBlockLatencyRow(stats, "8,0")
			if row == nil || row.PairedCount != 1 || row.AmbiguousCohortCount != 0 || row.PairingSuppressedCount != 0 {
				t.Fatalf("in-window account is exactly one clean pair: %+v", row)
			}
		})
	}
}

// Boundary-crossing REAL IO stays published: a pre-window issue completed
// inside the window is genuine elapsed latency (the carry-in projection pin
// TestRootCauseRankBlockIOCarryInRanksOnlyWindowProjection depends on it).
func TestBlockPairingStraddlingPairStaysPublished(t *testing.T) {
	idx := buildTraceIndex(t, "block_straddle.systrace", `
 io-40 (40) [003] .... 1.500000: block_rq_issue: 8,0 R 4096 () 1000 + 8 [io]
 irq-2 (2) [003] .... 2.500000: block_rq_complete: 8,0 R () 1000 + 8 [0]
`)
	stats := ComputeWindowStats(idx, Query{TimeStart: 2.0, TimeEnd: 3.0})
	if len(stats.IOLatencies) != 1 || math.Abs(stats.IOLatencies[0].DurationMs-1000) > 0.001 {
		t.Fatalf("pair overlapping the window keeps its real duration: %+v", stats.IOLatencies)
	}
}

// A request wholly before the window must leave the window's account empty —
// no zero-count summary rows, no unpaired disclosures.
func TestBlockPairingFullyPreWindowPairLeavesNoAccount(t *testing.T) {
	idx := buildTraceIndex(t, "block_prewindow.systrace", `
 io-40 (40) [003] .... 1.000000: block_rq_issue: 8,0 R 4096 () 1000 + 8 [io]
 irq-2 (2) [003] .... 1.200000: block_rq_complete: 8,0 R () 1000 + 8 [0]
`)
	stats := ComputeWindowStats(idx, Query{TimeStart: 2.0, TimeEnd: 3.0})
	if len(stats.IOLatencies) != 0 {
		t.Fatalf("wholly pre-window pair must not publish latency in this window: %+v", stats.IOLatencies)
	}
	if row := windowBlockLatencyRow(stats, "8,0"); row != nil {
		t.Fatalf("no in-window block activity means no block summary row: %+v", row)
	}
	if containsSubstring(stats.Caveats, "block_io_pairing_unpaired") {
		t.Fatalf("nothing in-window is unpaired: %+v", stats.Caveats)
	}
}
