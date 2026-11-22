package tool

// HMC377 round 11 — batch-owner ruling R-377-11 ① (2026-11-21): one carrier
// per instant across every share relation.
//
// The engine half (rank_share_claims_segments_r11_hmc377_test.go) proves what
// the publication writes on the published copy of the review-10 universes
// (~/hmc-work/hmc377/fix10/review-holders/univ). A tool-package universe
// cannot carry the engine's claim instants, so these pins start from that
// published state — the rows exactly as the one resolver leaves them (the
// gate-withdrawn inversion seat as a context row, the successors' typed
// claims, ShareReturnedMs) — and run it end to end through production:
// traceQueryTypedObservations -> CompileTraceCausalProjectionSet -> tree
// model -> ◎ board -> zh/en fences. The overlay universes that run the gate's
// withdrawal itself end to end are evidence only
// (~/hmc-work/hmc377/fix11/hold/univ, oracle11 --equal).
//
// Hand oracles, from the universe definitions (thread 6326, raw ready time):
//   - UR3c_w (raw 3.000): the residual 1.097 + the constituent 1.903 (the
//     withdrawn claim seat's successor); the 1.500 satellite is counted at the
//     constituent and stays off ◎ under its own footnote;
//   - UR4mix_w (raw 6.903): the chain seat 0.853 + the ◇ remainder 6.050
//     (its 5.000 plus the 1.050 the withdrawn seat held);
//   - UR6_w (raw 3.000): the residual takes back the carved share (3.000);
//   - UR4_w (raw 1.903): the satellite carries its whole account and says so
//     (register R10H-5).
// Round-11 fields are set through trace_query_withdrawn_segments_r11_shim_hmc377_test.go
// (the red run on b2325eba6 swaps in no-op setters).

import (
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc377R11Context(ms float64, l0, l1 int) tracequery.RootCauseRankItem {
	it := tracequery.RootCauseRankItem{Rank: 0, Tier: tracequery.RootCauseTierContextOnly, Type: "unknown_state", Source: "wakeup_chain.causal_impacts", Thread: hmc377R9Dep,
		ImpactMs: ms, CumulativeImpactMs: ms, RunnableMs: ms, DominantState: "runnable",
		PriorityRelationCaliber: "closed_range_stable", ChainRelevance: "on_chain", Causality: "on_wakeup_chain", Confidence: 0.8,
		StartTs: 10.2, EndTs: 10.201903, LineStart: l0, LineEnd: l1,
		Summary: "scheduler state retained as context: priority relation caliber \"closed_range_stable\" does not authorize a finite positive inversion root"}
	return it
}

func hmc377R11Claim(part tracequery.RootCauseShareClaimPart, role tracequery.RootCauseShareHolderRole, ms float64, resolved, represented bool,
	resolvedMs, returnedMs float64, successor bool, holders ...string) tracequery.RootCauseShareClaim {
	c := tracequery.RootCauseShareClaim{Part: part, Role: role, ShareMs: ms, Resolved: resolved, Exact: resolved && len(holders) == 1,
		Represented: represented, HolderKeys: holders}
	hmc377R11SetClaim(&c, resolvedMs, returnedMs, successor)
	return c
}

func hmc377R11Result(items ...tracequery.RootCauseRankItem) tracequery.Result {
	return tracequery.Result{
		TraceArtifacts: []tracequery.TraceArtifactSource{{SourcePath: "capture.ftrace", Kind: "ftrace", CausalCompatible: true}},
		WakeupChain: &tracequery.ChainResult{Target: hmc377R9Target, Window: tracequery.TimeWindow{StartTs: 10, EndTs: 11},
			CausalImpacts: []tracequery.WakeupCausalImpact{{Thread: hmc377R9Other, ChainDepth: 1, DominantState: "runnable", DominantImpactMs: 30, TotalMs: 30,
				RunnableMs: 30, OnChain: true, LineStart: 5, LineEnd: 6, Window: tracequery.TimeWindow{StartTs: 10.1, EndTs: 10.13}}}},
		RootCauseRank: &tracequery.RootCauseRankResult{Window: tracequery.TimeWindow{StartTs: 10, EndTs: 11},
			Items: append(append([]tracequery.RootCauseRankItem{hmc377R9OtherSeat()}, items...), hmc377R11DStateSeat(), hmc377R11PublishedInversion())},
	}
}

// hmc377R11PublishedInversion is a third thread's published inversion seat:
// its 行3 gated-composite breakdown keeps the 「=」 breakdown legend lit beside
// the split rows' 「全账 X ms = …」 identity (the levelmerge fixture
// convention).
func hmc377R11PublishedInversion() tracequery.RootCauseRankItem {
	return tracequery.RootCauseRankItem{Rank: 5, Tier: "tertiary", Type: "priority_inversion_candidate", Source: "wakeup_chain.causal_impacts",
		Thread:   tracequery.ThreadRef{Comm: "pool", PID: 6500},
		ImpactMs: 2.5, CumulativeImpactMs: 2.5, EffectiveImpactMs: 2.5, RunnableMs: 2.0, RunningMs: 0.5, DominantState: "runnable",
		GatedRunnableMs: 2.0, GatedRunningDeficitMs: 0.5,
		PriorityRelationCaliber: "closed_range_stable", PriorityRelationProvenLowerMs: 5, PriorityRelationArtifactSources: []string{"artifact:0"},
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain", Confidence: 0.8, StartTs: 10.7, EndTs: 10.7025, LineStart: 700, LineEnd: 710}
}

// hmc377R11DStateSeat is another thread's D-state chain seat: it keeps the ⛓
// glyph's icon legend lit beside the ◎ channel word (the representative-
// fixture convention, xlane2 同款).
func hmc377R11DStateSeat() tracequery.RootCauseRankItem {
	return tracequery.RootCauseRankItem{Rank: 6, Tier: "tertiary", Type: "d_state_or_io_wait", Source: "wakeup_chain.causal_impacts",
		Thread:   tracequery.ThreadRef{Comm: "worker", PID: 300},
		ImpactMs: 3.0, CumulativeImpactMs: 3.0, EffectiveImpactMs: 3.0, DStateMs: 3.0, DominantState: "d_sleep",
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain", Confidence: 0.8, StartTs: 10.6, EndTs: 10.603, LineStart: 600, LineEnd: 605}
}

func hmc377R11Projection(result tracequery.Result) types.TraceCausalProjection {
	records := traceQueryTypedObservations(result, "capture.ftrace", "payload", "raw", "", time.Unix(0, 0).UTC())
	set := types.CompileTraceCausalProjectionSet(types.ObservationLedger{Records: records})
	if len(set.Projections) != 1 {
		return types.TraceCausalProjection{}
	}
	return set.Projections[0]
}

// hmc377R11UR3cWithdrawn — UR3c_w as the publication leaves it: the claim seat
// (lines 10..20) withdrawn, the constituent (seat-5) its successor.
func hmc377R11UR3cWithdrawn() tracequery.Result {
	b := tracequery.RootCauseRankItem{Rank: 3, Tier: "secondary", Type: "runnable_wait", Source: "wakeup_chain.aggregated_impacts", Thread: hmc377R9Dep,
		ImpactMs: 1.097, CumulativeImpactMs: 3.0, EffectiveImpactMs: 1.097, ProjectedImpactMs: 1.097, RunnableMs: 1.097, DominantState: "runnable", ChainDepth: 2,
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain", Confidence: 0.8, StartTs: 10.2, EndTs: 10.401097, LineStart: 50, LineEnd: 60,
		GatedShareClaimedMs: 1.903, GatedShareFullMs: 3.0, GatedShareClaimSeats: []string{"10..20"}, GatedShareClaimWithdrawn: true,
		ShareClaims: []tracequery.RootCauseShareClaim{hmc377R11Claim(hmc377R11GatedPart(), hmc377R11GatedRole(),
			1.903, true, false, 1.903, 0, true, "seat-5")},
		Summary: "binder:458_5-6326 runnable aggregate residual 1.097ms after the interval-accounting split: 1.903ms of the 3.000ms scheduling-demand account lies inside the same thread's priority-inversion seat branch window(s) and is counted by another row of this thread on the published board (the claiming inversion seat is published only as context, tier context_only) — claimed + residual = full account (one segment set, two disjoint shares, additive back)"}
	sat := tracequery.RootCauseRankItem{Rank: 4, Tier: "tertiary", Type: "runnable_wait", Source: "window_stats", Thread: hmc377R9Dep,
		ImpactMs: 1.5, CumulativeImpactMs: 1.5, EffectiveImpactMs: 1.5, ProjectedImpactMs: 1.5, RunnableMs: 1.5, DominantState: "runnable",
		ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", Confidence: 0.78, StartTs: 10.2, EndTs: 10.2015, LineStart: 30, LineEnd: 40,
		ChainAnchorRepresentedByChainSeat: true,
		ShareClaims: []tracequery.RootCauseShareClaim{hmc377R11Claim(tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderChainSeat,
			1.5, true, true, 1.5, 0, true, "seat-5")},
		Summary: "binder:458_5-6326 runnable satellite; anchored share counted by another row of this thread: this satellite's whole account (1.500ms of 1.500ms) lies inside the thread's dependency windows; the row that held that time is published only as context, tier context_only, and another row of this thread on the published board counts it — this diagnostic projection rides the adjacent lane whole with every published value unchanged"}
	a := tracequery.RootCauseRankItem{Rank: 0, Tier: "adjacent", Type: "runnable_wait", Source: "wakeup_chain.aggregated_impacts", Thread: hmc377R9Dep,
		ImpactMs: 1.903, CumulativeImpactMs: 1.903, EffectiveImpactMs: 1.903, ProjectedImpactMs: 1.903, RunnableMs: 1.903, DominantState: "runnable", ChainDepth: 2,
		ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", Confidence: 0.8, StartTs: 10.2, EndTs: 10.401097, LineStart: 50, LineEnd: 60,
		GatedShareClaimedMs: 1.903, GatedShareFullMs: 3.0, GatedShareClaimSeats: []string{"10..20"}, GatedShareConstituentSeat: true, GatedShareClaimWithdrawn: true,
		ShareHolderKey: "seat-5",
		ShareClaims: []tracequery.RootCauseShareClaim{hmc377R11Claim(hmc377R11GatedPart(), hmc377R11GatedRole(),
			1.903, false, false, 0, 1.903, false)},
		Summary: "binder:458_5-6326 runnable share 1.903ms of the 3.000ms aggregate account overlapping the same thread's priority-inversion seat branch window(s) (branch-window overlap measure) (the claiming inversion seat is published only as context, tier context_only) — constituent share: it rides the adjacent lane and counts this share with its own value"}
	hmc377R11SetReturned(&a, 1.903)
	return hmc377R11Result(hmc377R11Context(1.903, 10, 20), b, sat, a)
}

// hmc377R11UR4mixWithdrawn — UR4mix_w as published: the inversion seat
// holding W1 withdrawn, the chain seat (seat-3) still holding W2; the ◇
// remainder took W1 back.
func hmc377R11UR4mixWithdrawn() tracequery.Result {
	run := tracequery.RootCauseRankItem{Rank: 3, Tier: "secondary", Type: "runnable_wait", Source: "wakeup_chain.causal_impacts", Thread: hmc377R9Dep,
		ImpactMs: 0.853, CumulativeImpactMs: 0.853, EffectiveImpactMs: 0.853, RunnableMs: 0.853, DominantState: "runnable",
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain", Confidence: 0.8, StartTs: 10.20105, EndTs: 10.201903, LineStart: 21, LineEnd: 25, ShareHolderKey: "seat-3"}
	rem := tracequery.RootCauseRankItem{Rank: 4, Tier: "tertiary", Type: "runnable_wait", Source: "window_stats", Thread: hmc377R9Dep,
		ImpactMs: 6.05, CumulativeImpactMs: 6.05, EffectiveImpactMs: 6.05, RunnableMs: 6.05, DominantState: "runnable",
		ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", Confidence: 0.76, StartTs: 10.5, EndTs: 10.505, LineStart: 60, LineEnd: 90,
		ChainAnchoredMs: 1.903, ChainAnchorFullMs: 6.903, ChainAnchorRemainderSeat: true,
		ShareClaims: []tracequery.RootCauseShareClaim{hmc377R11Claim(tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderChainSeat,
			1.903, false, false, 0.853, 1.05, false, "seat-3")},
		Summary: "binder:458_5-6326 runnable (scheduling-pressure candidate) remainder 5.000ms outside its wakeup-dependency windows (no chain credential for these segments); full-window account 6.903ms = 1.903ms anchored inside typed dependency windows (0.853ms of it owned by the chain seat; 1.050ms whose owning row is published only as context, tier context_only, so this row counts them with its own value) + this remainder — same segment set, mutually disjoint, additive back to the full account"}
	hmc377R11SetReturned(&rem, 1.05)
	return hmc377R11Result(hmc377R11Context(1.05, 10, 20), run, rem)
}

// hmc377R11UR6Withdrawn — UR6_w as published: the constituent cut by the side
// cap, the claim seat withdrawn; the residual took the carved share back.
func hmc377R11UR6Withdrawn() tracequery.Result {
	b := tracequery.RootCauseRankItem{Rank: 3, Tier: "secondary", Type: "runnable_wait", Source: "wakeup_chain.aggregated_impacts", Thread: hmc377R9Dep,
		ImpactMs: 3.0, CumulativeImpactMs: 3.0, EffectiveImpactMs: 3.0, ProjectedImpactMs: 3.0, RunnableMs: 3.0, DominantState: "runnable", ChainDepth: 2,
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain", Confidence: 0.8, StartTs: 10.2, EndTs: 10.401097, LineStart: 50, LineEnd: 60,
		GatedShareClaimedMs: 1.903, GatedShareFullMs: 3.0, GatedShareClaimSeats: []string{"10..20"}, GatedShareClaimWithdrawn: true,
		ShareClaims: []tracequery.RootCauseShareClaim{hmc377R11Claim(hmc377R11GatedPart(), hmc377R11GatedRole(),
			1.903, false, false, 0, 1.903, false)},
		Summary: "binder:458_5-6326 runnable aggregate residual 1.097ms after the interval-accounting split: 1.903ms of the 3.000ms scheduling-demand account lies inside the same thread's priority-inversion seat branch window(s) and this seat counts it with its own value (the claiming inversion seat is published only as context, tier context_only) — claimed + residual = full account (one segment set, two disjoint shares, additive back)"}
	hmc377R11SetReturned(&b, 1.903)
	return hmc377R11Result(hmc377R11Context(1.903, 10, 20), b)
}

// hmc377R11UR4Withdrawn — UR4_w as published: both holders of the satellite's
// share withdrawn, the satellite took the whole share back.
func hmc377R11UR4Withdrawn() tracequery.Result {
	sat := tracequery.RootCauseRankItem{Rank: 4, Tier: "tertiary", Type: "runnable_wait", Source: "window_stats", Thread: hmc377R9Dep,
		ImpactMs: 1.903, CumulativeImpactMs: 1.903, EffectiveImpactMs: 1.903, ProjectedImpactMs: 1.903, RunnableMs: 1.903, DominantState: "runnable",
		ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", Confidence: 0.78, StartTs: 10.2, EndTs: 10.201903, LineStart: 30, LineEnd: 40,
		ShareClaims: []tracequery.RootCauseShareClaim{hmc377R11Claim(tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderChainSeat,
			1.903, false, true, 0, 1.903, false)},
		Summary: "binder:458_5-6326 runnable satellite; this satellite's whole account (1.903ms of 1.903ms) lies inside the thread's dependency windows, but the row holding that time is published only as context, tier context_only — this diagnostic projection keeps its own value on the adjacent lane"}
	hmc377R11SetReturned(&sat, 1.903)
	ctx2 := hmc377R11Context(0.853, 21, 25)
	return hmc377R11Result(hmc377R11Context(1.05, 10, 20), ctx2, sat)
}

func hmc377R11DepSum(carriers []string) float64 {
	sum := 0.0
	for _, c := range carriers {
		if at := strings.LastIndex(c, "/"); at >= 0 {
			if v, err := strconv.ParseFloat(c[at+1:], 64); err == nil {
				sum += v
			}
		}
	}
	return sum
}

func TestHMC377R11SuccessorOfAWithdrawnClaimSeatIsNamedNeverAChainSeat(t *testing.T) {
	f := hmc377R9Run(t, hmc377R11UR3cWithdrawn())
	zh, en := hmc377R7Squash(f.zh), hmc377R7Squash(f.en)
	if got := hmc377R11DepSum(f.depCarriers); math.Abs(got-3.0) > 0.0015 || len(f.depCarriers) != 2 {
		t.Errorf("◎ carriers %v Σ %.3f, hand oracle [residual 1.097, constituent 1.903] Σ = raw 3.000\n%s", f.depCarriers, got, f.dump())
	}
	model := buildRuntimeTraceProjTreeModel(hmc377R11Projection(hmc377R11UR3cWithdrawn()), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	var aTag string
	for _, r := range runtimeTraceProjSMR1AllRows(&model) {
		if r.HasData && r.Node.GatedShareConstituentSeat {
			aTag = r.EvidenceTag
		}
	}
	if aTag == "" {
		t.Fatalf("the constituent row must be drawn\n%s", f.dump())
	}
	for _, want := range []string{
		"本线程另一席[" + aTag + "]计入锚定份:本席账目全额锚定于 typed 唤醒依赖窗内(有凭证),原持有行仅作上下文,同段物理时间由[" + aTag + "]以自身数值计入",
		"分账构成份:本行 1.903ms 为全账 3.000ms 中与本线程反转席分支窗重叠的份额;该反转席仅作上下文(不参与根因排序),此份额由本行自计",
		"分账残余席:全账 3.000ms = 构成份 1.903ms(与本线程反转席分支窗重叠,该席仅作上下文,由本线程构成份行自计) + 本席残余 1.097ms",
	} {
		if !strings.Contains(zh, hmc377R7Squash(want)) {
			t.Errorf("zh face missing %q\n%s", want, f.zh)
		}
	}
	if want := "anchored share counted at [" + aTag + "], another seat of this thread"; !strings.Contains(en, hmc377R7Squash(want)) {
		t.Errorf("en face missing %q\n%s", want, f.en)
	}
	for _, banned := range []string{"锚定份由链席[" + aTag + "]", "代表(整席不入链上榜)"} {
		if strings.Contains(zh, hmc377R7Squash(banned)) {
			t.Errorf("the successor is called a chain seat (%q)\n%s", banned, f.zh)
		}
	}
	elimZH := hmc377R7Squash(f.elimZH)
	if !strings.Contains(elimZH, hmc377R7Squash("已由本线程另一席计入")) || strings.Contains(elimZH, hmc377R7Squash("已由链上席代表")) {
		t.Errorf("the ◎ footnote must count the satellite under the successor label\n%s", f.elimZH)
	}
	if w := hmc377R7RetiredAbsenceWord(f.zh + f.en + f.elimZH + f.elimEN); w != "" {
		t.Errorf("absence word %q\n%s", w, f.dump())
	}
}

func TestHMC377R11RowThatTookBackAWithdrawnHoldersShareCountsIt(t *testing.T) {
	t.Run("UR4mix_w: ◇ remainder", func(t *testing.T) {
		f := hmc377R9Run(t, hmc377R11UR4mixWithdrawn())
		if got := hmc377R11DepSum(f.depCarriers); math.Abs(got-6.903) > 0.0015 {
			t.Errorf("◎ carriers %v Σ %.3f, hand oracle [chain seat 0.853, remainder 6.050] Σ = raw 6.903\n%s", f.depCarriers, got, f.dump())
		}
		// EVOLUTION RECORD (HMC377 round 12, batch-owner ruling R-377-12 ②):
		// the anchored share has two dispositions here (0.853 held by the
		// chain seat on the board, 1.050 passed back to this row), so the
		// split line keeps its see-detail word and the clause that worded a
		// share returned in part as one disposition is removed (values
		// unchanged: the row still counts 6.050).
		//   old zh: "…+本行其余5.000ms(无链上凭证);锚定份中 1.050ms 的持有行仅作上下文,
		//           本行以自身数值计入,本行共计 6.050ms"
		//   old en: "1.050ms of the anchored share is held by a row that is
		//           context only, so this row counts it in its own account:
		//           6.050ms in all"
		//   new: the split line alone (evidence review-holders/evidence/F3_*.txt,
		//        E2remMix_w; fix12/hold/univ/faces_e4).
		if want := "同源二分:全窗6.903ms=锚定1.903ms(锚定席见明细)+本行其余5.000ms(无链上凭证)"; !strings.Contains(hmc377R7Squash(f.zh), hmc377R7Squash(want)) ||
			strings.Contains(hmc377R7Squash(f.zh), hmc377R7Squash("本行以自身数值计入")) {
			t.Errorf("zh face: want %q and no returned clause\n%s", want, f.zh)
		}
		if strings.Contains(hmc377R7Squash(f.en), hmc377R7Squash("so this row counts it in its own account")) {
			t.Errorf("en face words a share returned in part as returned whole\n%s", f.en)
		}
		var returned []string
		for _, n := range hmc377R10Notes(t, hmc377R11UR4mixWithdrawn(), hmc377R11ShareReturnedNoteKey) {
			returned = append(returned, n)
		}
		if strings.Join(returned, ",") != hmc377R11ShareReturnedNoteKey+"=1.050" {
			t.Errorf("the typed returned share rides the record note: %v", returned)
		}
		var claims []string
		for _, n := range hmc377R10Notes(t, hmc377R11UR4mixWithdrawn(), types.TraceNoteKeyShareClaims) {
			claims = append(claims, n)
		}
		if want := types.TraceNoteKeyShareClaims + "=anchored/chain_seat/0/0/0/1.903/3/r=0.853,t=1.050,s=0"; len(claims) != 1 || claims[0] != want {
			t.Errorf("the extended claim entry: %v, want %q", claims, want)
		}
	})
	t.Run("UR6_w: residual takes back the carved share", func(t *testing.T) {
		f := hmc377R9Run(t, hmc377R11UR6Withdrawn())
		if len(f.depCarriers) != 1 || !strings.HasSuffix(f.depCarriers[0], "/root_cause_secondary/3.000") {
			t.Errorf("◎ carriers %v, hand oracle [residual 3.000] = raw 3.000\n%s", f.depCarriers, f.dump())
		}
		if want := "分账残余席:全账 3.000ms = 构成份 1.903ms(与本线程反转席分支窗重叠,该席仅作上下文,由本席自计) + 本席残余 1.097ms(同一段集两不重叠份,可加还原全账);本席计 3.000ms"; !strings.Contains(hmc377R7Squash(f.zh), hmc377R7Squash(want)) {
			t.Errorf("zh face missing %q\n%s", want, f.zh)
		}
		if strings.Contains(f.zh, "构成份行") {
			t.Errorf("the residual names a constituent row that is not published\n%s", f.zh)
		}
	})
	t.Run("UR4_w: satellite carrier (register R10H-5)", func(t *testing.T) {
		f := hmc377R9Run(t, hmc377R11UR4Withdrawn())
		if len(f.depCarriers) != 1 || !strings.HasSuffix(f.depCarriers[0], "/root_cause_adjacent/1.903") {
			t.Errorf("◎ carriers %v, hand oracle [satellite 1.903] = raw 1.903\n%s", f.depCarriers, f.dump())
		}
		zh := hmc377R7Squash(f.zh)
		if !strings.Contains(zh, hmc377R7Squash("锚定份的持有行仅作上下文,本行以自身数值计入:本席账目全额锚定于 typed 唤醒依赖窗内(有凭证),原持有行不参与根因排序,本行记 ◇ 邻近")) ||
			strings.Contains(zh, hmc377R7Squash("锚定份由本线程另一席持有(见明细)")) {
			t.Errorf("the carrier satellite must say it counts the share, not that another seat holds it\n%s", f.zh)
		}
	})
}

// Register R10T-0 (review 10: holders 7 = population 3 = pins 0): the
// detail-row half of the gate's decision — a gate-withdrawn row published as
// an ABSORBED detail row (lossless detail of a chain seat) holds nothing and
// claims nothing either: its split stamp and its claim on its remainder twin
// go with its seat, and no face prints its own half of the split in held form.
// Review 10 showed both halves untested: TM10 (the publisher hands the engine
// no detail decisions) and RM11 (the engine ignores them) passed every suite
// while the F5 leak returned on detail rows
// (~/hmc-work/hmc377/fix10/review-pins/mutant/probe_absorbed*.log).
func TestHMC377R11WithdrawnDetailRowPrintsNoSplitOfItsOwn(t *testing.T) {
	result := hmc377R10F5Universe()
	rank := *result.RootCauseRank
	inv := rank.Items[1]
	inv.Rank, inv.AbsorbedByRankFamily, inv.AbsorbedIntoFamily = 0, true, "priority_inversion"
	rank.Items = []tracequery.RootCauseRankItem{rank.Items[0], rank.Items[2]}
	rank.AbsorbedItems = []tracequery.RootCauseRankItem{inv}
	result.RootCauseRank = &rank
	published := traceQueryPriorityResultForPublication(result)
	d := published.RootCauseRank.AbsorbedItems[0]
	if d.Type == "priority_inversion_candidate" {
		t.Fatalf("precondition: the gate withdraws the absorbed inversion row (proven lower 2.896 < effective 5.793)")
	}
	if d.ChainAnchoredMs != 0 || d.ChainAnchorFullMs != 0 || len(d.ShareClaims) != 0 || d.ShareHolderKey != "" {
		t.Errorf("the withdrawn detail row keeps its split/claims: anchored=%.3f full=%.3f claims=%+v key=%q",
			d.ChainAnchoredMs, d.ChainAnchorFullMs, d.ShareClaims, d.ShareHolderKey)
	}
	if r := result.RootCauseRank.AbsorbedItems[0]; r.ChainAnchorFullMs != 1.130 || len(r.ShareClaims) != 1 {
		t.Errorf("publication mutated the engine detail row")
	}
	f := hmc377R9Run(t, result)
	zh, en := hmc377R7Squash(f.zh), hmc377R7Squash(f.en)
	for _, banned := range []string{"本行锚定0.820ms+其余0.310ms", "(◇余段席)"} {
		if strings.Contains(zh, hmc377R7Squash(banned)) {
			t.Errorf("the withdrawn detail row prints its split in held form (%q)\n%s", banned, f.zh)
		}
	}
	if strings.Contains(en, hmc377R7Squash("(◇ remainder seat)")) || strings.Contains(en, hmc377R7Squash("this row 0.820ms anchored")) {
		t.Errorf("the withdrawn detail row prints its split in held form (en)\n%s", f.en)
	}
}

// Register R10T-2 (TM14): the ×N merged constituent row whose members' claim
// seats were withdrawn — the per-seat ledger clear zeroed its claimed/full
// figures, the typed marker and decision survive — says the members count
// their shares themselves and enters ◎ (review 10: the mutant that drops this
// arm passed every suite, ~/hmc-work/hmc377/fix10/review-pins/mutant/full_tool_TM14.log).
func TestHMC377R11MergedWithdrawnConstituentRowSelfExplains(t *testing.T) {
	for _, zh := range []bool{true, false} {
		projection := hmc377R10WithdrawnSplitProjection()
		for i := range projection.AdjacentCauses {
			if projection.AdjacentCauses[i].EvidenceID == "r10-a" {
				a := &projection.AdjacentCauses[i]
				a.GatedShareClaimedMS, a.GatedShareFullMS, a.GatedShareClaimSeats, a.MergedCount = 0, 0, nil, 2
			}
		}
		model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
		fence := runtimeTraceProjTreeFence(model, zh)
		want := "分账构成份(合并行):各成员份额与同线程反转席分支窗重叠;该反转席仅作上下文(不参与根因排序),成员份额由本行自计,成员级分账见明细行"
		if !zh {
			want = "split-account constituent share (merged row): every member share overlaps the branch windows of the same thread's priority-inversion seat; that seat is context only (not ranked), so this row counts the member shares with its own value"
		}
		if !strings.Contains(hmc377R7Squash(fence), hmc377R7Squash(want)) {
			t.Errorf("zh=%t: the merged withdrawn constituent must self-explain %q\n%s", zh, want, fence)
		}
		a := xlane2FindRowByEvidenceID(&model, "r10-a")
		if a == nil || !runtimeTraceProjElimEligible(*a) {
			t.Errorf("zh=%t: the merged withdrawn constituent carries its members' shares on ◎", zh)
		}
	}
}
