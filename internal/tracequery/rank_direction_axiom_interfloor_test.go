package tracequery

// rank_direction_axiom_interfloor_test.go — INTERFLOOR-1 pins (user ruling
// §29.150③, 2026-07-19: 极小交集判噪音,相对形地板):
//
//	pin① 阈门双向: an overlap below ratio × min(published eff) demotes to the
//	     undisclosed typed token lane on BOTH sides (roster empty, sentence
//	     face dead) while an overlap above the floor keeps the full mutual
//	     disclosure — same seats, only the overlap size differs.
//	pin② undisclosed 记号在案: the demoted pair's typed audit record is
//	     present on both sides and NO caveat is minted (noise-reduction is
//	     the ruling's goal — the caveat face is user-visible 立案素材; the
//	     carrier-absent 件3 lane keeps its caveat, distinct shape).
//	pin③ 值通道零动: the demote path changes no published value.
//	pin④ 相对形结构 pin (禁绝对 ms 常数, R-15-e 红线): scaling every
//	     interval and every published eff by 1000× flips NO verdict — the
//	     floor must be a product with a published seat value; an absolute ms
//	     floor breaks scale invariance and turns this red.

import (
	"strings"
	"testing"
)

// interfloorPair builds the E5×E26-shaped same-thread cross-direction pair
// with a parameterized overlap and scale. All durations multiply by scale;
// the semantic seat's single member interval lies fully inside the running
// union, so the typed intersection == overlapSec × 1000 ms.
func interfloorPair(overlapSec, scale float64) RootCauseRankResult {
	run := axiomv2SelfRunningSeat(4)
	sem := axiomv2SemanticFamilySeat(1)
	base := 100.0
	run.selfGapRunningIntervals = []foldInterval{
		{start: base + 0.010*scale, end: base + 0.110*scale}, // 100ms × scale
	}
	run.EffectiveImpactMs = 4.843 * scale
	run.ImpactMs = 107.084 * scale
	run.ProjectedImpactMs = run.ImpactMs
	run.CumulativeImpactMs = run.ImpactMs
	sem.semanticMemberIntervals = []foldInterval{
		{start: base + 0.020*scale, end: base + 0.020*scale + overlapSec*scale},
	}
	sem.EffectiveImpactMs = 9.586 * scale
	sem.ImpactMs = 9.586 * scale
	sem.ProjectedImpactMs = sem.ImpactMs
	sem.CumulativeImpactMs = sem.ImpactMs
	rank := RootCauseRankResult{
		Target: ThreadRef{Comm: "ease.cloudmusic", PID: 63993},
		Window: TimeWindow{StartTs: base, EndTs: base + 0.160*scale},
		Items:  []RootCauseRankItem{sem, run},
	}
	stampRootCauseFixDirections(&rank)
	stampCrossDirectionDisclosureAndConservation(&rank)
	return rank
}

// min(eff) = 4.843 × scale ⇒ floor = 0.05 × 4.843 × scale ≈ 0.242ms × scale.
// 0.2ms×scale sits below it (tiny arm), 0.3ms×scale above it (keep arm) —
// both a comfortable ±20% away from the boundary (no float-dust coupling).
const interfloorTinySec = 0.0002
const interfloorKeepSec = 0.0003

func TestINTERFLOOR1DeMinimisGateBothDirections(t *testing.T) {
	// ── tiny arm: demote to the undisclosed typed token lane. ──
	rank := interfloorPair(interfloorTinySec, 1)
	sem, run := rank.Items[0], rank.Items[1]
	if len(sem.CrossDirectionOverlaps) != 0 || len(run.CrossDirectionOverlaps) != 0 {
		t.Fatalf("pin① 极小降道: the sub-floor pair must mint no roster entry, got %d/%d",
			len(sem.CrossDirectionOverlaps), len(run.CrossDirectionOverlaps))
	}
	if got := sem.CrossDirectionOverlapUndisclosed; len(got) != 1 || got[0] != "running" {
		t.Fatalf("pin② undisclosed 记号在案 (semantic side): %v", got)
	}
	if got := run.CrossDirectionOverlapUndisclosed; len(got) != 1 || got[0] != "class_verification" {
		t.Fatalf("pin② undisclosed 记号在案 (running side): %v", got)
	}
	for _, caveat := range rank.Caveats {
		if strings.HasPrefix(caveat, "cross_direction_overlap:") {
			t.Fatalf("pin② 降噪不立案: the de-minimis demote must not mint the carrier-absent caveat: %q", caveat)
		}
	}
	// pin③ 值通道零动.
	if sem.EffectiveImpactMs != 9.586 || run.EffectiveImpactMs != 4.843 ||
		sem.ImpactMs != 9.586 || run.ImpactMs != 107.084 {
		t.Fatalf("pin③ 值通道零动 violated: sem eff=%.3f impact=%.3f run eff=%.3f impact=%.3f",
			sem.EffectiveImpactMs, sem.ImpactMs, run.EffectiveImpactMs, run.ImpactMs)
	}

	// ── keep arm: above the floor the full mutual disclosure stands. ──
	rank = interfloorPair(interfloorKeepSec, 1)
	sem, run = rank.Items[0], rank.Items[1]
	if len(sem.CrossDirectionOverlaps) != 1 || len(run.CrossDirectionOverlaps) != 1 {
		t.Fatalf("pin① 显著保发: the above-floor pair must keep symmetric roster entries, got %d/%d",
			len(sem.CrossDirectionOverlaps), len(run.CrossDirectionOverlaps))
	}
	wantOverlap := interfloorKeepSec * 1000
	if diff := sem.CrossDirectionOverlaps[0].OverlapMs - wantOverlap; diff > 0.001 || diff < -0.001 {
		t.Fatalf("keep arm overlap drifted: got %.6f want %.6f", sem.CrossDirectionOverlaps[0].OverlapMs, wantOverlap)
	}
	if len(sem.CrossDirectionOverlapUndisclosed) != 0 || len(run.CrossDirectionOverlapUndisclosed) != 0 {
		t.Fatalf("keep arm must not wear the undisclosed token: %v / %v",
			sem.CrossDirectionOverlapUndisclosed, run.CrossDirectionOverlapUndisclosed)
	}
}

func TestINTERFLOOR1RelativeFloorScaleInvariance(t *testing.T) {
	// pin④ (R-15-e 禁绝对 ms 常数): the SAME shapes at 1×/1000× must reach
	// the SAME verdicts. An absolute-ms floor mutant flips at least one cell
	// of this 2×2 (a small absolute floor keeps the 1000× tiny arm; a large
	// one demotes the 1× keep arm).
	for _, scale := range []float64{1, 1000} {
		rank := interfloorPair(interfloorTinySec, scale)
		if len(rank.Items[0].CrossDirectionOverlaps) != 0 || len(rank.Items[1].CrossDirectionOverlaps) != 0 {
			t.Fatalf("scale %.0f: the tiny arm must demote (relative floor scales with the seats)", scale)
		}
		if len(rank.Items[0].CrossDirectionOverlapUndisclosed) != 1 {
			t.Fatalf("scale %.0f: the tiny arm must keep its undisclosed record", scale)
		}
		rank = interfloorPair(interfloorKeepSec, scale)
		if len(rank.Items[0].CrossDirectionOverlaps) != 1 || len(rank.Items[1].CrossDirectionOverlaps) != 1 {
			t.Fatalf("scale %.0f: the keep arm must keep disclosing (relative floor scales with the seats)", scale)
		}
	}
}
