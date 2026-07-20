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

// TestINTERFLOOR1DemoteFreesCapSlotOnFullBoard — TAILHYG-1 cap6 corner pin
// (§29.157 备案 P3-1; 丙组 candidate §29.160): the de-minimis demote
// `continue`s BEFORE the cap accounting, so on a full-cap board the demoted
// pair frees its roster slot and the next keep pair — previously cap-blocked
// — deterministically emits. 方向合裁定 (噪声不再挤占显著披露); this pin makes
// the slot-freeing behavior load-bearing. Shape: one running hub × 8
// same-thread partners, pairs sorted by overlap desc = 5 keeps (8..4ms), one
// demote (3ms class_verification pair: floor 0.05×min(100,100)=5ms > 3), one
// keep (2ms), one keep (1ms). Real code: K1-K5 fill slots 1-5, the demote
// frees its slot, the 2ms keep takes slot 6, the 1ms keep cap-drops. Mutation
// arm (demote 改不腾位 — rosterLen++ before the demote continue): the 2ms
// keep pair is cap-blocked and the hub roster holds only 5 keep entries → red.
func TestINTERFLOOR1DemoteFreesCapSlotOnFullBoard(t *testing.T) {
	hub := axiomv2SelfRunningSeat(1)
	hub.selfGapRunningIntervals = []foldInterval{{start: 17729.480, end: 17729.580}} // 100ms union
	hub.EffectiveImpactMs = 100.0                                                    // demote floor rides min(both effs)
	items := []RootCauseRankItem{hub}
	// Keep partners: jit_compile, eff 10 → pair floor 0.05×10 = 0.5ms; every
	// keep overlap (8,7,6,5,4,2,1ms) clears it. Disjoint spans nested inside
	// the hub union → overlap == own length.
	keepLens := []float64{8, 7, 6, 5, 4, 2, 1} // ms
	for k, lenMs := range keepLens {
		partner := axiomv2SemanticFamilySeat(2 + k)
		partner.Type = "jit_compile"
		partner.SemanticClass = "jit_compile"
		partner.MemberCount = 1
		partner.semanticMemberIntervals = nil
		partner.EffectiveImpactMs = 10.0
		partner.StartTs = 17729.480 + float64(k+1)*0.010
		partner.EndTs = partner.StartTs + lenMs/1000
		partner.LineStart = 50000 + k*100
		partner.LineEnd = partner.LineStart + 10
		items = append(items, partner)
	}
	// Demote partner: class_verification, eff 100 → pair floor 0.05×100 = 5ms;
	// its 3ms overlap demotes AND sorts BEFORE the 2ms/1ms keep pairs.
	demote := axiomv2SemanticFamilySeat(10)
	demote.MemberCount = 1
	demote.semanticMemberIntervals = nil
	demote.EffectiveImpactMs = 100.0
	demote.StartTs = 17729.5665
	demote.EndTs = demote.StartTs + 0.003
	demote.LineStart, demote.LineEnd = 60000, 60010
	items = append(items, demote)
	rank := axiomv2Rank(items...)
	stampRootCauseFixDirections(&rank)
	stampCrossDirectionDisclosureAndConservation(&rank)
	hubOut := rank.Items[0]
	// 帽内计数守恒: the roster is full and every slot holds a KEEP pair — the
	// demote consumed no capacity.
	if len(hubOut.CrossDirectionOverlaps) != RootCauseCrossDirectionOverlapPartnerCap {
		t.Fatalf("帽内计数守恒: hub roster must fill to the cap with keep pairs, got %d want %d",
			len(hubOut.CrossDirectionOverlaps), RootCauseCrossDirectionOverlapPartnerCap)
	}
	// The freed slot goes to the 2ms keep pair (deterministic: pairs process in
	// overlap-desc order, the demote never touches rosterLen).
	last := hubOut.CrossDirectionOverlaps[RootCauseCrossDirectionOverlapPartnerCap-1]
	if diff := last.OverlapMs - 2.0; diff > 0.001 || diff < -0.001 {
		t.Fatalf("腾名额发射: slot 6 must hold the previously cap-blocked 2ms keep pair, got %.3f", last.OverlapMs)
	}
	keep2 := rank.Items[6] // the 2ms keep partner (keepLens index 5)
	if len(keep2.CrossDirectionOverlaps) != 1 || len(keep2.CrossDirectionOverlapUndisclosed) != 0 {
		t.Fatalf("腾名额发射: the 2ms keep partner must emit its symmetric entry, got %d/%v",
			len(keep2.CrossDirectionOverlaps), keep2.CrossDirectionOverlapUndisclosed)
	}
	// The demoted pair rides the undisclosed lane on both sides — no roster
	// entry, no caveat (noise discipline).
	demoteOut := rank.Items[8]
	if len(demoteOut.CrossDirectionOverlaps) != 0 ||
		len(demoteOut.CrossDirectionOverlapUndisclosed) != 1 || demoteOut.CrossDirectionOverlapUndisclosed[0] != "running" {
		t.Fatalf("demote partner lane drifted: %+v", demoteOut)
	}
	// The 1ms keep pair stays honestly cap-dropped (the demote freed ONE slot,
	// not the cap): undisclosed on both sides + the capacity caveat.
	keep1 := rank.Items[7]
	if len(keep1.CrossDirectionOverlaps) != 0 ||
		len(keep1.CrossDirectionOverlapUndisclosed) != 1 || keep1.CrossDirectionOverlapUndisclosed[0] != "running" {
		t.Fatalf("over-cap 1ms keep pair lane drifted: %+v", keep1)
	}
	if got := hubOut.CrossDirectionOverlapUndisclosed; len(got) != 2 ||
		got[0] != "class_verification" || got[1] != "jit_compile" {
		t.Fatalf("hub undisclosed tokens must record the demote then the cap drop: %v", got)
	}
	capCaveats := 0
	for _, caveat := range rank.Caveats {
		if strings.Contains(caveat, "3.000ms") {
			t.Fatalf("the demoted pair must mint no caveat (噪声不立案): %q", caveat)
		}
		if strings.HasPrefix(caveat, "cross_direction_overlap:") {
			capCaveats++
			if !strings.Contains(caveat, "1.000ms") {
				t.Fatalf("the capacity caveat must describe the 1ms cap-dropped pair: %q", caveat)
			}
		}
	}
	if capCaveats != 1 {
		t.Fatalf("exactly one capacity caveat (the 1ms drop), got %d: %v", capCaveats, rank.Caveats)
	}
	// 值通道零动.
	if hubOut.EffectiveImpactMs != 100.0 {
		t.Fatalf("value channel moved on the hub: %.3f", hubOut.EffectiveImpactMs)
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
