package tracequery

// rank_family_recon_case2_test.go — CASE-2 absorbed_into window-encoding
// regression witness (§29.52 独立立案 CASE-2, smr_audit_report_20260712;
// S7-TPF fail-open witness; v5 P1 批, 2026-07-13).
//
// The 62930 (13:41) run minted the G1 family key window from ms-ROUNDED
// endpoints (34579.472000..34579.588000) while the blocking result carried
// the µs-precise query window (34579.472865..34579.587805) — the four-dim
// gate failed on the drifted window identity, the recon fell open, and the
// report seated three parallel rows + the #7 ghost family seat. The 31693
// normal form carried the precise endpoints on BOTH sides. The IOFAM-SELF
// batch removed the rounded-window mint path (方向正确); THIS file is the
// missing regression witness: both lanes must copy their window identity
// from THE SAME typed normalized query endpoints, and the absorbed_into key
// must encode those exact endpoints — never a re-rounded copy.
//
// MUTATION self-checks: re-introducing any ms-rounding on either lane's
// window identity (family StatsWindow* or blocking.Window) reds the
// absorption assertion (gate mismatch → fail-open); re-rounding the key
// encoding reds the verbatim-endpoint assertion.

import (
	"fmt"
	"strings"
	"testing"
)

// TestCase2AbsorbedIntoKeyCarriesTypedQueryWindowEndpoints runs the opendir
// G1 shape under a µs-PRECISE query window (the 31693 normal form): the
// recon must absorb (both lanes share the typed normalized endpoints) and
// the minted key must encode those endpoints verbatim.
func TestCase2AbsorbedIntoKeyCarriesTypedQueryWindowEndpoints(t *testing.T) {
	idx := g1BuildIndex(t, g1OpendirShapeTrace)
	// µs-precise, deliberately NOT ms-aligned endpoints (the 62930 fail-open
	// witness was exactly an ms-aligned re-rounding of such a window).
	start, end := 10.000865, 10.900805
	bundle := BuildFrameRootCauseBundle(idx, Query{PID: 500, TimeStart: start, TimeEnd: end})
	if bundle.RootCauseRank == nil || bundle.CriticalBlocking == nil {
		t.Fatal("bundle must carry both lanes")
	}
	fam := g1FamilyRow(t, *bundle.RootCauseRank)
	if fam.StatsWindowStartTs != start || fam.StatsWindowEndTs != end {
		t.Fatalf("family window identity must be the typed normalized query endpoints, got %.6f..%.6f",
			fam.StatsWindowStartTs, fam.StatsWindowEndTs)
	}
	if bundle.CriticalBlocking.Window.StartTs != start || bundle.CriticalBlocking.Window.EndTs != end {
		t.Fatalf("blocking window identity must be the typed normalized query endpoints, got %+v",
			bundle.CriticalBlocking.Window)
	}
	if fam.AbsorbedChainRows != 6 {
		t.Fatalf("µs-precise window must not break the four-dim gate (62930 fail-open regression), got absorbed=%d",
			fam.AbsorbedChainRows)
	}
	wantWindow := fmt.Sprintf("%.6f..%.6f", start, end)
	if fam.RankFamilyKey == "" || !strings.Contains(fam.RankFamilyKey, wantWindow) {
		t.Fatalf("family key must encode the typed query window endpoints verbatim (%s), got %q",
			wantWindow, fam.RankFamilyKey)
	}
	roundedWindow := fmt.Sprintf("%.6f..%.6f", 10.000000, 10.900000)
	if strings.Contains(fam.RankFamilyKey, roundedWindow) {
		t.Fatalf("family key must never carry an ms-rounded window copy (62930 witness), got %q", fam.RankFamilyKey)
	}
	absorbed := 0
	for _, item := range bundle.CriticalBlocking.Items {
		if !item.AbsorbedByRankFamily {
			continue
		}
		absorbed++
		if item.AbsorbedIntoFamily != fam.RankFamilyKey {
			t.Fatalf("absorbed_into must join the family key verbatim: %q vs %q",
				item.AbsorbedIntoFamily, fam.RankFamilyKey)
		}
		if !strings.Contains(item.AbsorbedIntoFamily, wantWindow) {
			t.Fatalf("absorbed_into key must encode the typed query window endpoints, got %q",
				item.AbsorbedIntoFamily)
		}
	}
	if absorbed != 6 {
		t.Fatalf("all six chain rows must absorb under the precise window, got %d", absorbed)
	}
}

// TestCase2CrossRoundedWindowStaysFailOpen pins the honest fail-open half of
// the 62930 witness: when one lane's window identity HAS drifted (an
// ms-rounded copy — simulated at the unit level), the gate must refuse the
// absorption rather than guess (honest duplicate beats wrong absorption).
func TestCase2CrossRoundedWindowStaysFailOpen(t *testing.T) {
	rank, blocking := case1UnitFamily()
	// The family carries an ms-rounded drifted window while the blocking
	// result kept the precise query endpoints.
	rank.Items[0].StatsWindowStartTs = 3.000000
	rank.Items[0].StatsWindowEndTs = 3.200000
	blocking.Window = TimeWindow{StartTs: 3.000865, EndTs: 3.199805}
	reconcileCriticalBlockingWithRankFamilies(&rank, &blocking)
	if blocking.Items[0].AbsorbedByRankFamily {
		t.Fatalf("a drifted window identity must fail open, got %+v", blocking.Items[0])
	}
	if rank.Items[0].RankFamilyKey != "" || rank.Items[0].AbsorbedChainRows != 0 {
		t.Fatalf("no key may mint on a refused match: %+v", rank.Items[0])
	}
}
