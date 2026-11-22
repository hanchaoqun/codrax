package tool

// HMC377 round 12 — batch-owner ruling R-377-12 ② (2026-11-22): a share gets
// a specific holder form on the reader face only when its whole share has one
// disposition — all held by published rows, all held by one successor, all
// passed back to the row, all cut by the engine cap; every mixed disposition
// takes the see-detail form, which names no holder. Values are unchanged.
//
// Review 11 (lens holders; ~/hmc-work/hmc377/fix11/review-holders/evidence/
// F3_*.txt, F4_*.txt, univ/facesH/*) reproduced, as published by the one
// resolver:
//   - a ◇ remainder whose anchored share a withdrawn seat's successor holds
//     printed 「(⛓链上席)」 (H1lmR_w; T6cpuW_w regressed from see-detail);
//   - a satellite held partly by its chain seat and partly by a successor
//     printed 「本线程另一席[E#][E#]计入锚定份…原持有行仅作上下文」 (C2part_w,
//     H1lm_w): one of those holders is an original chain seat;
//   - the R10H-5 clause 「锚定份中 X 的持有行仅作上下文,本行以自身数值计入」 and the
//     satellite's 「持有行仅作上下文,本行以自身数值计入」 keyed on any returned
//     share, so a share returned only in part (the rest held or cap-cut) was
//     worded as returned whole (E2remMix_w, E3satMixW_w).
// The rows below are the published state the engine half
// (internal/tracequery/rank_share_claims_release_r12_hmc377_test.go) proves
// the resolver writes; they run end to end through production:
// traceQueryTypedObservations -> projection -> tree model -> fences. Round-11
// claim fields are set through trace_query_withdrawn_segments_r11_shim_hmc377_test.go.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func hmc377R12Remainder(anchored, full, eff float64, claims ...tracequery.RootCauseShareClaim) tracequery.RootCauseRankItem {
	return tracequery.RootCauseRankItem{Rank: 4, Tier: "tertiary", Type: "runnable_wait", Source: "window_stats", Thread: hmc377R9Dep,
		ImpactMs: eff, CumulativeImpactMs: eff, EffectiveImpactMs: eff, RunnableMs: eff, DominantState: "runnable",
		ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", Confidence: 0.76, StartTs: 10.5, EndTs: 10.505, LineStart: 60, LineEnd: 90,
		ChainAnchoredMs: anchored, ChainAnchorFullMs: full, ChainAnchorRemainderSeat: true, ShareClaims: claims,
		Summary: "binder:458_5-6326 runnable (scheduling-pressure candidate) remainder"}
}

func hmc377R12Satellite(ms, eff, held float64, represented bool, claims ...tracequery.RootCauseShareClaim) tracequery.RootCauseRankItem {
	return tracequery.RootCauseRankItem{Rank: 4, Tier: "tertiary", Type: "runnable_wait", Source: "window_stats", Thread: hmc377R9Dep,
		ImpactMs: eff, CumulativeImpactMs: eff, EffectiveImpactMs: eff, ProjectedImpactMs: eff, RunnableMs: eff, DominantState: "runnable",
		ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", Confidence: 0.78, StartTs: 10.2, EndTs: 10.2 + ms/1000, LineStart: 30, LineEnd: 40,
		ChainAnchorRepresentedByChainSeat: represented, ChainAnchorRepresentedHeldMs: held, ShareClaims: claims,
		Summary: "binder:458_5-6326 runnable satellite"}
}

func hmc377R12ChainRun(ms float64, l0 int, key string) tracequery.RootCauseRankItem {
	return tracequery.RootCauseRankItem{Rank: 3, Tier: "secondary", Type: "runnable_wait", Source: "wakeup_chain.causal_impacts", Thread: hmc377R9Dep,
		ImpactMs: ms, CumulativeImpactMs: ms, EffectiveImpactMs: ms, RunnableMs: ms, DominantState: "runnable",
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain", Confidence: 0.8, StartTs: 10.20105, EndTs: 10.201903, LineStart: l0, LineEnd: l0 + 4, ShareHolderKey: key}
}

func TestHMC377R12ReaderWordsNameAHolderOnlyForOneDisposition(t *testing.T) {
	anchored, chainSeat := tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderChainSeat
	t.Run("a ◇ share held by a withdrawn seat's successor is never 「(⛓链上席)」 (H1lmR_w, T6cpuW_w)", func(t *testing.T) {
		succ := hmc377R12ChainRun(1.903, 21, "seat-3")
		succ.Type, succ.Source, succ.ChainRelevance, succ.Causality = "cpu_affinity_or_cpuset", "window_stats.cpu_constraints", "adjacent", "adjacent_to_wakeup_chain"
		rem := hmc377R12Remainder(1.903, 3.903, 2.0, hmc377R11Claim(anchored, chainSeat, 1.903, true, false, 1.903, 0, true, "seat-3"))
		f := hmc377R9Run(t, hmc377R11Result(hmc377R11Context(1.903, 10, 20), succ, rem))
		zh, en := hmc377R7Squash(f.zh), hmc377R7Squash(f.en)
		if strings.Contains(zh, hmc377R7Squash("(⛓链上席)")) || !strings.Contains(zh, hmc377R7Squash("锚定1.903ms(锚定席见明细)")) {
			t.Errorf("zh\n%s", f.zh)
		}
		if strings.Contains(en, hmc377R7Squash("(⛓ chain seat)")) || !strings.Contains(en, hmc377R7Squash("(anchored seat: see the detail index)")) {
			t.Errorf("en\n%s", f.en)
		}
	})
	t.Run("a satellite held by its chain seat and by a successor takes the see-detail form (C2part_w)", func(t *testing.T) {
		run := hmc377R12ChainRun(0.853, 21, "seat-3")
		rem := hmc377R12Remainder(1.903, 6.903, 6.05)
		rem.ShareHolderKey = "seat-4"
		sat := hmc377R12Satellite(1.903, 1.903, 0, true, hmc377R11Claim(anchored, chainSeat, 1.903, true, true, 1.903, 0, true, "seat-3", "seat-4"))
		f := hmc377R9Run(t, hmc377R11Result(hmc377R11Context(1.05, 10, 20), run, rem, sat))
		zh, en := hmc377R7Squash(f.zh), hmc377R7Squash(f.en)
		if strings.Contains(zh, hmc377R7Squash("计入锚定份")) || !strings.Contains(zh, hmc377R7Squash("锚定份由本线程另一席持有(见明细):本席账目全额锚定于")) {
			t.Errorf("zh\n%s", f.zh)
		}
		if strings.Contains(en, hmc377R7Squash("another seat of this thread: this seat")) || !strings.Contains(en, hmc377R7Squash("anchored share held by another seat of this thread (see the detail index)")) {
			t.Errorf("en\n%s", f.en)
		}
	})
	t.Run("a satellite held whole by one successor keeps the successor form (T1ab_w)", func(t *testing.T) {
		a := hmc377R12Satellite(1.903, 1.903, 0, false, hmc377R11Claim(anchored, chainSeat, 1.903, false, true, 0, 1.903, false))
		a.ShareHolderKey = "seat-3"
		hmc377R11SetReturned(&a, 1.903)
		b := hmc377R12Satellite(1.903, 1.903, 0, true, hmc377R11Claim(anchored, chainSeat, 1.903, true, true, 1.903, 0, true, "seat-3"))
		b.Type, b.LineStart, b.LineEnd = "scheduler_latency", 36, 46
		f := hmc377R9Run(t, hmc377R11Result(hmc377R11Context(1.903, 10, 20), a, b))
		if !strings.Contains(hmc377R7Squash(f.zh), hmc377R7Squash("计入锚定份:本席账目全额锚定于")) {
			t.Errorf("zh\n%s", f.zh)
		}
	})
	t.Run("the ◇ 「本行以自身数值计入」 clause states a share returned whole only (UR4rem_w / E2remMix_w)", func(t *testing.T) {
		whole := hmc377R12Remainder(1.903, 6.903, 6.903, hmc377R11Claim(anchored, chainSeat, 1.903, false, false, 0, 1.903, false))
		hmc377R11SetReturned(&whole, 1.903)
		f := hmc377R9Run(t, hmc377R11Result(hmc377R11Context(1.903, 10, 20), whole))
		if !strings.Contains(hmc377R7Squash(f.zh), hmc377R7Squash("锚定份中 1.903ms 的持有行仅作上下文,本行以自身数值计入,本行共计 6.903ms")) {
			t.Errorf("all returned:\n%s", f.zh)
		}
		run := hmc377R12ChainRun(0.853, 21, "seat-3")
		part := hmc377R12Remainder(2.903, 7.903, 6.05, hmc377R11Claim(anchored, chainSeat, 2.903, false, false, 0.853, 1.05, false, "seat-3"))
		hmc377R11SetReturned(&part, 1.05)
		f = hmc377R9Run(t, hmc377R11Result(hmc377R11Context(1.05, 10, 20), run, part))
		if strings.Contains(hmc377R7Squash(f.zh), hmc377R7Squash("本行以自身数值计入")) || strings.Contains(hmc377R7Squash(f.en), hmc377R7Squash("so this row counts it in its own account")) {
			t.Errorf("a share returned in part is worded as returned whole:\n%s\n%s", f.zh, f.en)
		}
	})
	t.Run("a satellite's returned form states a share returned whole only (E3satMixW_w)", func(t *testing.T) {
		sat := hmc377R12Satellite(2.903, 2.903, 0, false, hmc377R11Claim(anchored, chainSeat, 2.903, false, true, 0, 1.05, false))
		hmc377R11SetReturned(&sat, 1.05)
		f := hmc377R9Run(t, hmc377R11Result(hmc377R11Context(1.05, 10, 20), sat))
		zh := hmc377R7Squash(f.zh)
		if strings.Contains(zh, hmc377R7Squash("锚定份的持有行仅作上下文,本行以自身数值计入")) || !strings.Contains(zh, hmc377R7Squash("锚定份由本线程另一席持有(见明细)")) {
			t.Errorf("zh\n%s", f.zh)
		}
	})
	t.Run("a partly represented share held by a successor names no chain seat (T05)", func(t *testing.T) {
		rem := hmc377R12Remainder(1.903, 6.903, 6.05)
		rem.ShareHolderKey = "seat-3"
		sat := hmc377R12Satellite(1.853, 1.0, 0.853, false, hmc377R11Claim(anchored, chainSeat, 1.853, false, true, 0.853, 1.0, true, "seat-3"))
		hmc377R11SetReturned(&sat, 1.0)
		f := hmc377R9Run(t, hmc377R11Result(hmc377R11Context(2.903, 10, 20), rem, sat))
		zh := hmc377R7Squash(f.zh)
		if strings.Contains(zh, hmc377R7Squash("锚定份部分由链席")) || !strings.Contains(zh, hmc377R7Squash("锚定份部分由本线程另一席持有(见明细)")) {
			t.Errorf("zh\n%s", f.zh)
		}
	})
	t.Run("a share held in part names the holder of that part (T06, UR1cut_c)", func(t *testing.T) {
		inv := hmc377R11PublishedInversion()
		inv.Thread, inv.StartTs, inv.EndTs, inv.LineStart, inv.LineEnd, inv.ShareHolderKey = hmc377R9Dep, 10.2, 10.20105, 10, 20, "seat-2"
		sat := hmc377R12Satellite(1.903, 0.853, 1.05, false, hmc377R11Claim(anchored, chainSeat, 1.903, false, true, 1.05, 0, false, "seat-2"))
		f := hmc377R9Run(t, hmc377R11Result(inv, sat))
		if !strings.Contains(hmc377R7Squash(f.zh), hmc377R7Squash("锚定份部分由链席[E")) {
			t.Errorf("zh\n%s", f.zh)
		}
	})
	t.Run("a split share counted in parts takes the see-detail forms (T16)", func(t *testing.T) {
		gated := func(resolved bool, resolvedMs, returnedMs float64, successor bool, holders ...string) tracequery.RootCauseShareClaim {
			return hmc377R11Claim(hmc377R11GatedPart(), hmc377R11GatedRole(), 1.903, resolved, false, resolvedMs, returnedMs, successor, holders...)
		}
		a := tracequery.RootCauseRankItem{Rank: 0, Tier: "adjacent", Type: "runnable_wait", Source: "wakeup_chain.aggregated_impacts", Thread: hmc377R9Dep,
			ImpactMs: 1.0, CumulativeImpactMs: 1.0, EffectiveImpactMs: 1.0, ProjectedImpactMs: 1.0, RunnableMs: 1.0, DominantState: "runnable", ChainDepth: 2,
			ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", Confidence: 0.8, StartTs: 10.2, EndTs: 10.401097, LineStart: 50, LineEnd: 60,
			GatedShareClaimedMs: 1.903, GatedShareFullMs: 3.0, GatedShareConstituentSeat: true, GatedShareClaimWithdrawn: true,
			ShareClaims: []tracequery.RootCauseShareClaim{gated(false, 0.903, 1.0, false, "seat-5")}}
		hmc377R11SetReturned(&a, 1.0)
		b := tracequery.RootCauseRankItem{Rank: 3, Tier: "secondary", Type: "runnable_wait", Source: "wakeup_chain.aggregated_impacts", Thread: hmc377R9Dep,
			ImpactMs: 2.097, CumulativeImpactMs: 3.0, EffectiveImpactMs: 2.097, ProjectedImpactMs: 2.097, RunnableMs: 2.097, DominantState: "runnable", ChainDepth: 2,
			ChainRelevance: "on_chain", Causality: "on_wakeup_chain", Confidence: 0.8, StartTs: 10.2, EndTs: 10.401097, LineStart: 50, LineEnd: 60,
			GatedShareClaimedMs: 1.903, GatedShareFullMs: 3.0, GatedShareClaimWithdrawn: true,
			ShareClaims: []tracequery.RootCauseShareClaim{gated(false, 0.903, 1.0, false, "seat-5")}}
		hmc377R11SetReturned(&b, 1.0)
		inv := hmc377R11PublishedInversion()
		inv.Thread, inv.LineStart, inv.LineEnd, inv.ShareHolderKey = hmc377R9Dep, 70, 80, "seat-5"
		f := hmc377R9Run(t, hmc377R11Result(hmc377R11Context(1.0, 10, 20), a, b, inv))
		zh, en := hmc377R7Squash(f.zh), hmc377R7Squash(f.en)
		if !strings.Contains(zh, hmc377R7Squash("分账构成份:本行原份 1.903ms 为全账 3.000ms 中与本线程反转席分支窗重叠的份额;各部分计入行见明细")) ||
			!strings.Contains(en, hmc377R7Squash("for the row that counts each part, see the detail index")) {
			t.Errorf("constituent:\n%s\n%s", f.zh, f.en)
		}
		if strings.Contains(zh, hmc377R7Squash("由本席自计")) || strings.Contains(zh, hmc377R7Squash("本席计")) {
			t.Errorf("the residual must not say it counts a share it took back only in part:\n%s", f.zh)
		}
	})
}

// Register R11T-3 (mutant T19): a record valued at an own-work account part's
// price keeps its wall on the cumulative lane; that wall is disclosed as the
// record's window-projection total, never as a chain cumulative beside the
// price (R-377-11 ③). The split-family capture of the own-work pins
// (trace_query_own_work_account_hmc377_test.go): own-work part 30.000 ms of a
// 35.000 ms window projection, the on-chain part 5.000 ms of 20.000 ms.
func TestHMC377R12OwnWorkPricedRowShowsNoChainCumulative(t *testing.T) {
	faces := hmc377R11Query(t, hmc377R11SplitFamilyCapture(t), 10.0, 10.2)
	for _, lang := range []struct{ name, face string }{{"zh", faces.zh}, {"en", faces.en}} {
		t.Run(lang.name, func(t *testing.T) {
			for _, banned := range []string{"链上累计35.000ms", "链上累计20.000ms", "chain cum 35.000ms", "chain cum 20.000ms"} {
				if strings.Contains(lang.face, banned) {
					t.Errorf("a row valued at its own-work part's price wears its wall as a chain cumulative: %q", banned)
				}
			}
		})
	}
}
