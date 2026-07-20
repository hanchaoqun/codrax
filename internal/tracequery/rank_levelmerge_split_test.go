package tracequery

// rank_levelmerge_split_test.go — LEVELMERGE-1 件2 engine pins (方案 P 区间
// 分账, user ruling 2026-07-18; ledger real_trace_campaign_20260705.md).
//
// Pass-level pins over splitAggregateGatedRunnableShare:
//   - the A+B == pre-split value identity (GATED-CAL three-way identity
//     precedent) with the union-first multi-claimant measure;
//   - the clamp arm (window measure bounds wall clock, never the runnable
//     account) and the full-claim rewrite (B==0 → the seat itself becomes the
//     demoted constituent row);
//   - claim eligibility negatives (gated zero / pure running-deficit /
//     demoted claimant / envelope-only inventory / self exemption);
//   - fail-open disclosure arm (partial typed inventory → 裁定④ clause,
//     published values untouched);
//   - idempotency (build → enrich double pass), truncation routing
//     (constituent rows ride the demotedSide lane, never candidate seats)
//     and the claim-visibility summary patch.

import (
	"strings"
	"testing"
)

func levelMergeTestAggregateSeat(pid int, runnableMs float64, occ ...TimeWindow) RootCauseRankItem {
	item := RootCauseRankItem{
		Type:              "runnable_wait",
		Thread:            ThreadRef{PID: pid, Comm: "dep_worker"},
		Source:            "wakeup_chain.aggregated_impacts",
		Causality:         "on_wakeup_chain",
		ChainRelevance:    "on_chain",
		DominantState:     string(StateRunnable),
		RunnableMs:        runnableMs,
		EffectiveImpactMs: runnableMs,
		ImpactMs:          runnableMs,
		ProjectedImpactMs: runnableMs,
		Confidence:        0.82,
		ChainDepth:        1,
		LineStart:         100,
		LineEnd:           200,
	}
	for _, w := range occ {
		item.OccurrenceWindows = append(item.OccurrenceWindows, WakeupCausalOccurrence{Window: w})
	}
	if len(occ) > 0 {
		item.StartTs = occ[0].StartTs
		item.EndTs = occ[len(occ)-1].EndTs
	}
	return item
}

func levelMergeTestInversionSeat(pid int, gatedRunnable, gatedDeficit float64, window TimeWindow) RootCauseRankItem {
	gated := gatedRunnable + gatedDeficit
	return RootCauseRankItem{
		Type:                  "priority_inversion_candidate",
		Thread:                ThreadRef{PID: pid, Comm: "dep_worker"},
		Source:                "wakeup_chain.causal_impacts",
		Causality:             "on_wakeup_chain",
		ChainRelevance:        "on_chain",
		DominantState:         string(StateRunning),
		GatedRunnableMs:       gatedRunnable,
		GatedRunningDeficitMs: gatedDeficit,
		EffectiveImpactMs:     gated,
		ImpactMs:              gated,
		RunnableMs:            gatedRunnable,
		RunningMs:             8,
		Confidence:            0.91,
		ChainDepth:            2,
		StartTs:               window.StartTs,
		EndTs:                 window.EndTs,
		LineStart:             300,
		LineEnd:               340,
	}
}

func levelMergeFindConstituent(items []RootCauseRankItem) (RootCauseRankItem, bool) {
	for _, item := range items {
		if item.GatedShareConstituentSeat {
			return item, true
		}
	}
	return RootCauseRankItem{}, false
}

// The ordinary split: A = |∪claim ∩ ∪occ| carves the claimed share onto a
// demoted constituent row; the surviving seat publishes the residual and the
// A+B==full identity holds to the µs.
func TestGatedShareSplitIdentityOrdinary(t *testing.T) {
	chain := ChainResult{Target: ThreadRef{PID: 100, Comm: "app"}}
	agg := levelMergeTestAggregateSeat(200, 15,
		TimeWindow{StartTs: 10.000, EndTs: 10.010},
		TimeWindow{StartTs: 10.020, EndTs: 10.030})
	inv := levelMergeTestInversionSeat(200, 6, 2, TimeWindow{StartTs: 10.005, EndTs: 10.025})
	items := splitAggregateGatedRunnableShare(chain, []RootCauseRankItem{agg, inv})
	if len(items) != 3 {
		t.Fatalf("expected the constituent twin to mint beside the two seats, got %d rows", len(items))
	}
	b := items[0]
	// claim ∩ occ = [10.005,10.010] + [10.020,10.025] = 10ms; full = 15.
	if !near(b.GatedShareClaimedMs, 10.0, 0.0005) || !near(b.GatedShareFullMs, 15.0, 0.0005) {
		t.Fatalf("claimed/full drifted: %.6f / %.6f", b.GatedShareClaimedMs, b.GatedShareFullMs)
	}
	if !near(b.RunnableMs, 5.0, 0.0005) || !near(b.EffectiveImpactMs, 5.0, 0.0005) {
		t.Fatalf("residual channels drifted: runnable=%.6f eff=%.6f", b.RunnableMs, b.EffectiveImpactMs)
	}
	if !rootCauseItemIsOnChain(b) {
		t.Fatalf("B keeps the chain lane on its own residual segments (纪律③ ordinary form)")
	}
	a, ok := levelMergeFindConstituent(items)
	if !ok {
		t.Fatalf("constituent row missing")
	}
	if !near(a.GatedShareClaimedMs+b.RunnableMs, b.GatedShareFullMs, 0.0005) {
		t.Fatalf("identity A+B==full broken: %.6f + %.6f != %.6f", a.GatedShareClaimedMs, b.RunnableMs, b.GatedShareFullMs)
	}
	if !near(a.EffectiveImpactMs, 10.0, 0.0005) || !near(a.RunnableMs, 10.0, 0.0005) || !near(a.CumulativeImpactMs, 10.0, 0.0005) {
		t.Fatalf("constituent value channels must carry the claimed share, got eff=%.6f runnable=%.6f cum=%.6f",
			a.EffectiveImpactMs, a.RunnableMs, a.CumulativeImpactMs)
	}
	// 链上纪律④: the demoted constituent row wears NO on-chain marker.
	if rootCauseItemIsOnChain(a) || a.ChainRelevance != "adjacent" {
		t.Fatalf("constituent row must ride the adjacent lane, got relevance=%q causality=%q", a.ChainRelevance, a.Causality)
	}
	if len(a.GatedShareClaimSeats) != 1 || a.GatedShareClaimSeats[0] != "300..340" {
		t.Fatalf("claim-seat pointer drifted: %v", a.GatedShareClaimSeats)
	}
	// The inversion seat keeps its full gated composite (Plan-P face 1).
	if !near(items[1].EffectiveImpactMs, 8.0, 0.0005) {
		t.Fatalf("inversion seat value must stay untouched, got %.6f", items[1].EffectiveImpactMs)
	}
	// Cross-seat conservation: post-split pid Σ == pre-split Σ − A.
	preSum := 15.0 + 8.0
	postSum := b.EffectiveImpactMs + items[1].EffectiveImpactMs
	if !near(postSum, preSum-a.GatedShareClaimedMs, 0.0005) {
		t.Fatalf("conservation broken: post=%.6f pre=%.6f A=%.6f", postSum, preSum, a.GatedShareClaimedMs)
	}
}

// Full-claim clamp: the window measure exceeds the runnable account → A
// clamps to the account, B==0, and the seat itself becomes the constituent
// row (no zero-value competing husk).
func TestGatedShareSplitFullClaimRewrite(t *testing.T) {
	chain := ChainResult{Target: ThreadRef{PID: 100}}
	agg := levelMergeTestAggregateSeat(200, 4,
		TimeWindow{StartTs: 10.000, EndTs: 10.004},
		TimeWindow{StartTs: 10.006, EndTs: 10.009})
	inv := levelMergeTestInversionSeat(200, 4, 0, TimeWindow{StartTs: 10.000, EndTs: 10.010})
	items := splitAggregateGatedRunnableShare(chain, []RootCauseRankItem{agg, inv})
	if len(items) != 2 {
		t.Fatalf("full claim must rewrite in place (no clone), got %d rows", len(items))
	}
	a := items[0]
	if !a.GatedShareConstituentSeat {
		t.Fatalf("fully-claimed seat must become the constituent row")
	}
	if !near(a.GatedShareClaimedMs, 4.0, 0.0005) || !near(a.GatedShareFullMs, 4.0, 0.0005) {
		t.Fatalf("clamp drifted: claimed=%.6f full=%.6f (measure 7ms must clamp to the 4ms account)",
			a.GatedShareClaimedMs, a.GatedShareFullMs)
	}
	if rootCauseItemIsOnChain(a) {
		t.Fatalf("the fully-claimed constituent row must not keep the chain lane (凭证段全被 claim 带走)")
	}
}

// Multi-claimant measure: overlapping claimant windows union FIRST — the
// claimed share is the union-intersection measure, never a per-claimant Σ.
func TestGatedShareSplitMultiClaimantUnion(t *testing.T) {
	chain := ChainResult{Target: ThreadRef{PID: 100}}
	agg := levelMergeTestAggregateSeat(200, 30,
		TimeWindow{StartTs: 10.000, EndTs: 10.020},
		TimeWindow{StartTs: 10.040, EndTs: 10.050})
	invA := levelMergeTestInversionSeat(200, 3, 0, TimeWindow{StartTs: 10.000, EndTs: 10.010})
	invB := levelMergeTestInversionSeat(200, 3, 0, TimeWindow{StartTs: 10.005, EndTs: 10.015})
	invB.LineStart, invB.LineEnd = 400, 440
	items := splitAggregateGatedRunnableShare(chain, []RootCauseRankItem{agg, invA, invB})
	b := items[0]
	// union claim = [10.000,10.015] → ∩ occ = 15ms; per-claimant Σ would be
	// 10 + 10 = 20ms (double-subtraction, forbidden).
	if !near(b.GatedShareClaimedMs, 15.0, 0.0005) {
		t.Fatalf("multi-claimant claim must be the union measure 15ms, got %.6f", b.GatedShareClaimedMs)
	}
	if !near(b.RunnableMs, 15.0, 0.0005) {
		t.Fatalf("residual drifted: %.6f", b.RunnableMs)
	}
	if len(b.GatedShareClaimSeats) != 2 {
		t.Fatalf("both claim seats must ride the pointer, got %v", b.GatedShareClaimSeats)
	}
}

// Claim-eligibility negatives: every arm keeps the aggregate byte-identical.
func TestGatedShareSplitClaimEligibilityNegatives(t *testing.T) {
	chain := ChainResult{Target: ThreadRef{PID: 100}}
	base := func() RootCauseRankItem {
		return levelMergeTestAggregateSeat(200, 15,
			TimeWindow{StartTs: 10.000, EndTs: 10.010},
			TimeWindow{StartTs: 10.020, EndTs: 10.030})
	}
	window := TimeWindow{StartTs: 10.005, EndTs: 10.025}
	cases := []struct {
		name string
		inv  func() RootCauseRankItem
	}{
		{"gated_zero_claims_nothing", func() RootCauseRankItem {
			inv := levelMergeTestInversionSeat(200, 6, 0, window)
			inv.EffectiveImpactMs = 0 // gated-to-zero: authoritative, holds no seat
			return inv
		}},
		{"pure_running_deficit_claims_no_runnable", func() RootCauseRankItem {
			return levelMergeTestInversionSeat(200, 0, 5, window)
		}},
		{"demoted_claimant_out_of_population", func() RootCauseRankItem {
			inv := levelMergeTestInversionSeat(200, 6, 2, window)
			inv.ChainCredentialLaneDemoted = true
			return inv
		}},
		{"envelope_only_inventory_never_claims", func() RootCauseRankItem {
			inv := levelMergeTestInversionSeat(200, 6, 2, window)
			inv.MemberCount = 3 // multi-member seat, no member inventory → StartTs..EndTs is a hull
			return inv
		}},
		{"self_claimant_exempt", func() RootCauseRankItem {
			inv := levelMergeTestInversionSeat(200, 6, 2, window)
			inv.SubjectIsAnalysisTarget = true
			return inv
		}},
		{"other_pid_never_claims", func() RootCauseRankItem {
			return levelMergeTestInversionSeat(201, 6, 2, window)
		}},
		// 修补轮 件6 (2026-07-18): targeted negative arms for the Type and
		// Source conjuncts of the claim eligibility — each case satisfies
		// EVERY other conjunct (positive gated runnable, positive effective,
		// on-chain, competing seat, real singleton segment inventory,
		// same pid), so deleting the named conjunct alone flips it into a
		// claimant and reds this pin.
		{"non_inversion_type_with_gated_runnable_never_claims", func() RootCauseRankItem {
			inv := levelMergeTestInversionSeat(200, 6, 2, window)
			inv.Type = "runnable_wait" // wears a gated composite but is NOT an inversion row
			inv.DominantState = string(StateRunnable)
			return inv
		}},
		{"non_wakeup_chain_source_never_claims", func() RootCauseRankItem {
			inv := levelMergeTestInversionSeat(200, 6, 2, window)
			inv.Source = "window_stats" // a non-chain lane never claims chain runnable share
			return inv
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			agg := base()
			items := splitAggregateGatedRunnableShare(chain, []RootCauseRankItem{agg, tc.inv()})
			if len(items) != 2 {
				t.Fatalf("no split expected, got %d rows", len(items))
			}
			got := items[0]
			if got.GatedShareFullMs != 0 || got.GatedShareClaimedMs != 0 || got.GatedShareOverlapDisclosureMs != 0 {
				t.Fatalf("aggregate must stay untouched (typed fields minted: %+v)", got)
			}
			if !near(got.RunnableMs, 15.0, 0.0005) || !near(got.EffectiveImpactMs, 15.0, 0.0005) {
				t.Fatalf("aggregate values must stay untouched: runnable=%.6f eff=%.6f", got.RunnableMs, got.EffectiveImpactMs)
			}
		})
	}
}

// The self aggregate is exempt from the claimed-against population (XLANE-1
// self-exemption 既裁).
func TestGatedShareSplitSelfAggregateExempt(t *testing.T) {
	chain := ChainResult{Target: ThreadRef{PID: 200, Comm: "dep_worker"}}
	agg := levelMergeTestAggregateSeat(200, 15,
		TimeWindow{StartTs: 10.000, EndTs: 10.010},
		TimeWindow{StartTs: 10.020, EndTs: 10.030})
	agg.SubjectIsAnalysisTarget = true
	inv := levelMergeTestInversionSeat(200, 6, 2, TimeWindow{StartTs: 10.005, EndTs: 10.025})
	inv.SubjectIsAnalysisTarget = true
	items := splitAggregateGatedRunnableShare(chain, []RootCauseRankItem{agg, inv})
	if len(items) != 2 || items[0].GatedShareFullMs != 0 || items[0].GatedShareOverlapDisclosureMs != 0 {
		t.Fatalf("self rows must stay out of the split population")
	}
}

// fail-open disclosure arm: an at-cap occurrence inventory is ambiguous (the
// engine trim may have dropped members — the exact-cap precedent), so the
// overlap is DISCLOSED over the available real segments and every published
// value stays untouched.
func TestGatedShareSplitDisclosureOnPartialInventory(t *testing.T) {
	chain := ChainResult{Target: ThreadRef{PID: 100}}
	var occ []TimeWindow
	for i := 0; i < wakeupCausalAggregateOccurrenceCap; i++ {
		start := 10.000 + float64(i)*0.010
		occ = append(occ, TimeWindow{StartTs: start, EndTs: start + 0.005})
	}
	agg := levelMergeTestAggregateSeat(200, 30, occ...)
	inv := levelMergeTestInversionSeat(200, 6, 2, TimeWindow{StartTs: 10.000, EndTs: 10.012})
	items := splitAggregateGatedRunnableShare(chain, []RootCauseRankItem{agg, inv})
	if len(items) != 2 {
		t.Fatalf("disclosure arm must not mint rows, got %d", len(items))
	}
	got := items[0]
	if got.GatedShareFullMs != 0 || got.GatedShareConstituentSeat {
		t.Fatalf("disclosure arm must not split values")
	}
	// overlap = [10.000,10.005] + [10.010,10.012] = 7ms over the available
	// real segments (a lower bound, never a split input).
	if !near(got.GatedShareOverlapDisclosureMs, 7.0, 0.0005) {
		t.Fatalf("disclosure overlap drifted: %.6f", got.GatedShareOverlapDisclosureMs)
	}
	if !near(got.RunnableMs, 30.0, 0.0005) || !near(got.EffectiveImpactMs, 30.0, 0.0005) {
		t.Fatalf("disclosure arm must keep the published value untouched")
	}
	if !strings.Contains(got.Summary, "no value split is performed") {
		t.Fatalf("disclosure sentence missing: %q", got.Summary)
	}
}

// No physical overlap → byte-identical (both seats keep full publications and
// no typed field mints).
func TestGatedShareSplitDisjointWindowsUntouched(t *testing.T) {
	chain := ChainResult{Target: ThreadRef{PID: 100}}
	agg := levelMergeTestAggregateSeat(200, 15,
		TimeWindow{StartTs: 10.000, EndTs: 10.010},
		TimeWindow{StartTs: 10.020, EndTs: 10.030})
	inv := levelMergeTestInversionSeat(200, 6, 2, TimeWindow{StartTs: 10.050, EndTs: 10.060})
	items := splitAggregateGatedRunnableShare(chain, []RootCauseRankItem{agg, inv})
	if len(items) != 2 {
		t.Fatalf("no rows expected, got %d", len(items))
	}
	if items[0].GatedShareFullMs != 0 || items[0].GatedShareOverlapDisclosureMs != 0 {
		t.Fatalf("disjoint windows must stay byte-identical")
	}
}

// Idempotency: the enrich lane re-runs the pass over already-split rows —
// values, row count and typed fields must not move again.
func TestGatedShareSplitIdempotent(t *testing.T) {
	chain := ChainResult{Target: ThreadRef{PID: 100}}
	agg := levelMergeTestAggregateSeat(200, 15,
		TimeWindow{StartTs: 10.000, EndTs: 10.010},
		TimeWindow{StartTs: 10.020, EndTs: 10.030})
	inv := levelMergeTestInversionSeat(200, 6, 2, TimeWindow{StartTs: 10.005, EndTs: 10.025})
	once := splitAggregateGatedRunnableShare(chain, []RootCauseRankItem{agg, inv})
	twice := splitAggregateGatedRunnableShare(chain, append([]RootCauseRankItem(nil), once...))
	if len(twice) != len(once) {
		t.Fatalf("second pass minted rows: %d → %d", len(once), len(twice))
	}
	for i := range once {
		if once[i].RunnableMs != twice[i].RunnableMs || once[i].EffectiveImpactMs != twice[i].EffectiveImpactMs ||
			once[i].GatedShareClaimedMs != twice[i].GatedShareClaimedMs || once[i].Summary != twice[i].Summary {
			t.Fatalf("row %d drifted on the second pass", i)
		}
	}
}

// Truncation routing: the constituent row rides the demotedSide disclosure
// lane (sideTotal), never a candidate seat.
func TestGatedShareConstituentRidesSideLane(t *testing.T) {
	chain := ChainResult{Target: ThreadRef{PID: 100}}
	agg := levelMergeTestAggregateSeat(200, 15,
		TimeWindow{StartTs: 10.000, EndTs: 10.010},
		TimeWindow{StartTs: 10.020, EndTs: 10.030})
	inv := levelMergeTestInversionSeat(200, 6, 2, TimeWindow{StartTs: 10.005, EndTs: 10.025})
	items := splitAggregateGatedRunnableShare(chain, []RootCauseRankItem{agg, inv})
	out, _, candidateTotal, candidateEmitted, sideTotal, sideEmitted := truncateRootCauseRankCandidatesAndSideRows(items, 8)
	if candidateTotal != 2 || candidateEmitted != 2 {
		t.Fatalf("the B seat and the inversion seat are the only candidates, got %d/%d", candidateTotal, candidateEmitted)
	}
	if sideTotal != 1 || sideEmitted != 1 {
		t.Fatalf("the constituent row must ride the side lane, got %d/%d", sideTotal, sideEmitted)
	}
	if _, ok := levelMergeFindConstituent(out); !ok {
		t.Fatalf("the constituent row must survive truncation on the side lane")
	}
	assignRootCauseRanksAndTiers(out)
	for _, item := range out {
		if item.GatedShareConstituentSeat && rootCauseOrdinalChannel(item) != rootCauseOrdinalChannelAdjacent {
			t.Fatalf("constituent row must sit on the adjacent ordinal channel")
		}
	}
}

// The claim-visibility patch: when truncation killed the claiming inversion
// seat, the split sentences downgrade their co-publication claim.
func TestGatedShareClaimVisibilityPatch(t *testing.T) {
	chain := ChainResult{Target: ThreadRef{PID: 100}}
	agg := levelMergeTestAggregateSeat(200, 15,
		TimeWindow{StartTs: 10.000, EndTs: 10.010},
		TimeWindow{StartTs: 10.020, EndTs: 10.030})
	inv := levelMergeTestInversionSeat(200, 6, 2, TimeWindow{StartTs: 10.005, EndTs: 10.025})
	items := splitAggregateGatedRunnableShare(chain, []RootCauseRankItem{agg, inv})
	var withoutClaimant []RootCauseRankItem
	for _, item := range items {
		if item.Type == "priority_inversion_candidate" {
			continue
		}
		withoutClaimant = append(withoutClaimant, item)
	}
	patchGatedShareSummariesForClaimVisibility(withoutClaimant)
	for _, item := range withoutClaimant {
		if item.GatedShareFullMs > 0 && !strings.Contains(item.Summary, levelMergeSummaryClaimSeatUnpublished) {
			t.Fatalf("unpublished claimant must downgrade the sentence: %q", item.Summary)
		}
		if strings.Contains(item.Summary, levelMergeSummaryClaimSeatOnBoard) {
			t.Fatalf("stale on-board claim survived the patch: %q", item.Summary)
		}
	}
	// Published claimant → sentences untouched (idempotent negative).
	patchGatedShareSummariesForClaimVisibility(items)
	for _, item := range items {
		if item.GatedShareFullMs > 0 && !strings.Contains(item.Summary, levelMergeSummaryClaimSeatOnBoard) {
			t.Fatalf("published claimant must keep the on-board claim: %q", item.Summary)
		}
	}
}

// 修补轮 件1 (P1, 2026-07-18): a VS-1 periodic-source aggregate seat's
// published authority is the DISCOUNTED composite runnable + lateness
// (probe shape: eff 19 = 15 runnable + 4 lateness) — the split pass must
// never re-carve it (pre-fix disease: A+B == 15 erased the 4ms lateness
// share and both halves carried PeriodicSource=true into the periodic-Σ
// consumers). Honest form = the ruling-④ disclosure arm: overlap measured
// and disclosed, every published value untouched, zero rows minted.
func TestGatedShareSplitPeriodicSourceSeatDisclosureOnly(t *testing.T) {
	chain := ChainResult{Target: ThreadRef{PID: 100}}
	periodicAgg := func() RootCauseRankItem {
		agg := levelMergeTestAggregateSeat(200, 15,
			TimeWindow{StartTs: 10.000, EndTs: 10.010},
			TimeWindow{StartTs: 10.020, EndTs: 10.030})
		agg.PeriodicSource = true
		agg.DetectedPeriodMs = 16.6
		agg.LatenessMs = 4
		agg.EffectiveImpactMs = 19 // published authority = runnable 15 + lateness 4
		return agg
	}
	inv := levelMergeTestInversionSeat(200, 6, 2, TimeWindow{StartTs: 10.005, EndTs: 10.025})
	items := splitAggregateGatedRunnableShare(chain, []RootCauseRankItem{periodicAgg(), inv})
	if len(items) != 2 {
		t.Fatalf("the periodic seat must never mint A/B rows, got %d", len(items))
	}
	got := items[0]
	if got.GatedShareFullMs != 0 || got.GatedShareConstituentSeat {
		t.Fatalf("the periodic seat must never take the value split (pre-fix disease: A+B==15 erased the lateness share)")
	}
	if !near(got.EffectiveImpactMs, 19.0, 0.0005) || !near(got.RunnableMs, 15.0, 0.0005) ||
		!near(got.LatenessMs, 4.0, 0.0005) {
		t.Fatalf("periodic published values must stay untouched: eff=%.6f runnable=%.6f lateness=%.6f",
			got.EffectiveImpactMs, got.RunnableMs, got.LatenessMs)
	}
	if !got.PeriodicSource {
		t.Fatalf("the PeriodicSource marker must survive")
	}
	// overlap = [10.005,10.010] + [10.020,10.025] = 10ms over real segments.
	if !near(got.GatedShareOverlapDisclosureMs, 10.0, 0.0005) {
		t.Fatalf("disclosure overlap drifted: %.6f", got.GatedShareOverlapDisclosureMs)
	}
	if !strings.Contains(got.Summary, "VS-1 periodic discounted composite") ||
		!strings.Contains(got.Summary, "no value split is performed") {
		t.Fatalf("periodic disclosure sentence missing: %q", got.Summary)
	}
	if !near(items[1].EffectiveImpactMs, 8.0, 0.0005) {
		t.Fatalf("the inversion seat must stay untouched")
	}
	// Disjoint windows: nothing measurable → byte-identical, no disclosure.
	invFar := levelMergeTestInversionSeat(200, 6, 2, TimeWindow{StartTs: 10.050, EndTs: 10.060})
	items = splitAggregateGatedRunnableShare(chain, []RootCauseRankItem{periodicAgg(), invFar})
	if items[0].GatedShareOverlapDisclosureMs != 0 || items[0].GatedShareFullMs != 0 {
		t.Fatalf("a disjoint periodic seat must stay byte-identical")
	}
}

// 修补轮 件1 顺带 belt: a family-fold survivor (typed MemberFoldCaliber) is a
// caliber-computed account and never enters the split population — argued
// unreachable through the production mint chain (fold survivors never carry
// the aggregated_impacts source), pinned as pure defense in depth.
func TestGatedShareSplitMemberFoldCaliberBeltUntouched(t *testing.T) {
	chain := ChainResult{Target: ThreadRef{PID: 100}}
	agg := levelMergeTestAggregateSeat(200, 15,
		TimeWindow{StartTs: 10.000, EndTs: 10.010},
		TimeWindow{StartTs: 10.020, EndTs: 10.030})
	agg.MemberCount = 2
	agg.MemberFoldCaliber = RootCauseMemberFoldCaliberSumDisjoint
	inv := levelMergeTestInversionSeat(200, 6, 2, TimeWindow{StartTs: 10.005, EndTs: 10.025})
	items := splitAggregateGatedRunnableShare(chain, []RootCauseRankItem{agg, inv})
	if len(items) != 2 || items[0].GatedShareFullMs != 0 || items[0].GatedShareOverlapDisclosureMs != 0 {
		t.Fatalf("a fold-caliber seat must stay byte-identical (belt)")
	}
	if !near(items[0].RunnableMs, 15.0, 0.0005) {
		t.Fatalf("belt must not move values: %.6f", items[0].RunnableMs)
	}
}

// 修补轮 件3 (2026-07-18, 对抗官探针形收编): the residual-credential
// re-verification arm is REACHABLE — occ window measure (10ms) < published
// account (15ms) with the claim union covering every occurrence window →
// residual value 5ms survives while ZERO residual credential segments do.
// B then honestly demotes through the R4 credential family: typed
// ChainCredentialLaneDemoted, adjacent lane, demotedSide truncation routing
// (◇ + 披露, spec 纪律③ literal), residual value untouched by the demotion.
func TestGatedShareResidualCredentialDemotionArm(t *testing.T) {
	chain := ChainResult{Target: ThreadRef{PID: 100}}
	agg := levelMergeTestAggregateSeat(200, 15, TimeWindow{StartTs: 10.000, EndTs: 10.010})
	inv := levelMergeTestInversionSeat(200, 6, 2, TimeWindow{StartTs: 10.000, EndTs: 10.010})
	items := splitAggregateGatedRunnableShare(chain, []RootCauseRankItem{agg, inv})
	if len(items) != 3 {
		t.Fatalf("the ordinary split must still mint the constituent twin, got %d rows", len(items))
	}
	b := items[0]
	if !near(b.GatedShareClaimedMs, 10.0, 0.0005) || !near(b.RunnableMs, 5.0, 0.0005) ||
		!near(b.EffectiveImpactMs, 5.0, 0.0005) {
		t.Fatalf("split values drifted: claimed=%.6f runnable=%.6f eff=%.6f",
			b.GatedShareClaimedMs, b.RunnableMs, b.EffectiveImpactMs)
	}
	if !b.ChainCredentialLaneDemoted {
		t.Fatalf("B must demote through the typed R4 credential lane when zero residual segments survive")
	}
	if rootCauseItemIsOnChain(b) || b.ChainRelevance != "adjacent" {
		t.Fatalf("the demoted residual seat must ride the adjacent lane, got causality=%q relevance=%q",
			b.Causality, b.ChainRelevance)
	}
	if !strings.Contains(b.Summary, "no residual credential segment survives") {
		t.Fatalf("demotion disclosure missing: %q", b.Summary)
	}
	if _, ok := levelMergeFindConstituent(items); !ok {
		t.Fatalf("the constituent twin must still mint")
	}
	// Truncation: the demoted B rides the ◇ side lane, never a candidate.
	out, _, candidateTotal, _, sideTotal, _ := truncateRootCauseRankCandidatesAndSideRows(items, 8)
	if candidateTotal != 1 {
		t.Fatalf("only the inversion seat competes, got %d candidates", candidateTotal)
	}
	if sideTotal != 2 {
		t.Fatalf("B and the constituent twin must both ride the side lane, got %d", sideTotal)
	}
	found := false
	for _, item := range out {
		if item.ChainCredentialLaneDemoted && item.GatedShareFullMs > 0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("the demoted residual seat must survive truncation on the side lane")
	}
}
