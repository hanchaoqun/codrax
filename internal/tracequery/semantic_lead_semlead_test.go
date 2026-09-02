package tracequery

// semantic_lead_semlead_test.go — SEM-LEAD engine-half pins (ledger
// real_trace_campaign_20260705.md §29.7-2 ②, 2026-07-10): the published
// effective attribution of on-chain semantic span work is the REAL window
// projection (family true total — 792-textup witness: 有效归因 214.561ms 表值
// = 102.172 × 2.10 leaked the score multiplier); the deterministic
// hidden-cost boost stays ENGINE-INTERNAL on RankSortBoostedEffectiveMs
// (sort/Score channel), and the semantic_multiplier=/hidden_cost_boost=
// internal tokens never reach the Summary (红线: no internal tokens in answer
// prose). Family grain pinned here with a REAL engine mint (fixture 引擎实铸);
// the single-span grain is pinned by the evolved
// TestRootCauseRankPromotesOnChainSemanticRuntimeSpanWork.

import (
	"strings"
	"testing"
)

const semLeadTextureFamilyTrace = `
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (100) [002] .... 5.000400: tracing_mark_write: B|200|Texture upload(15573) 1140x1856
     worker-200 (100) [002] .... 5.001000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (100) [002] .... 5.002500: tracing_mark_write: E|200
     worker-200 (100) [002] .... 5.002600: tracing_mark_write: B|200|Texture upload(15563) 1140x1140
     worker-200 (100) [002] .... 5.005800: tracing_mark_write: E|200
     worker-200 (100) [002] .... 5.006000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.006500: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
`

func TestSemLeadFamilyPublishesRealTotalKeepsBoostInternal(t *testing.T) {
	idx := buildTraceIndex(t, "semlead_texture_family.systrace", semLeadTextureFamilyTrace)
	rank := BuildRootCauseRank(idx, Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.007, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	var fam *RootCauseRankItem
	for i := range rank.Items {
		if rank.Items[i].Type == "texture_upload" {
			fam = &rank.Items[i]
			break
		}
	}
	if fam == nil {
		t.Fatalf("expected a texture_upload family rank contender: %+v", rank.Items)
	}
	if fam.MemberCount != 2 || fam.SemanticClass != "texture_upload" {
		t.Fatalf("expected the ×2 same-thread family form: %+v", fam)
	}
	// EVOLUTION RECORD (审计 #66, §29.25 处置委托 + §29.26 待主会话落账,
	// 2026-07-10): this assertion was FLIPPED from
	// Tier==RootCauseTierDeterministicOptimization (the §29.22 as-built
	// independent tier) to Tier=="primary" when the engine retired the tier
	// mint — 追认 as the fuller reading of §29.7-2 全权参赛 (direct
	// primary/secondary/tertiary competition). The adjudicated tier-WORD
	// identity ("确定性优化候选") now rides the typed SemanticClass token on
	// the display faces (types.go RootCauseTierDeterministicOptimization
	// record).
	if fam.ChainRelevance != "on_chain" || fam.Tier == RootCauseTierContextOnly ||
		fam.OnChainBasis != RootCauseOnChainBasisSemanticChainIntervalRelation {
		t.Fatalf("the non-target family must seat on-chain by its interval credential (R4): %+v", fam)
	}
	// CROWNSEM-1 (§40.28 ①, restoring R4): the family's exact intersection IS
	// its published effective; the boost stays engine-internal (SEM-LEAD).
	if fam.EffectiveImpactMs <= 0 || fam.EffectiveImpactMs != fam.ProjectedImpactMs {
		t.Fatalf("interval-credentialed family must price its intersection: %+v", fam)
	}
	if fam.Rank <= 0 {
		t.Fatalf("a priced on-chain semantic family competes for an ordinal: %+v", rank.Items)
	}
	if strings.Contains(fam.Summary, "hidden_cost_boost") || strings.Contains(fam.Summary, "semantic_multiplier") {
		t.Fatalf("internal ranking tokens must not leak into the family summary: %q", fam.Summary)
	}
	if strings.Contains(fam.Summary, "effective_impact=0.000ms") ||
		!strings.Contains(fam.Summary, "priced on-chain per R4") {
		t.Fatalf("the family summary must state the priced credential caliber: %q", fam.Summary)
	}
}

// --- 复核 P1-1 (§29.22 修向(a)) pins ------------------------------------------

// semLeadRealBelowPrimaryTrace is the 复核 real<primary form: an on-chain
// D/IO wait of 8.100ms beats the texture family's REAL total (5.300ms) while
// the family's internal boost (×2.10 → 11.130ms) would have flipped the
// order. The ordinal must follow the PUBLISHED value (ordinal ≡ board ≡
// badges; §7.30 S1 — a synthetic ranking score never publishes as an ms hard
// fact, and the ordinal is a published face). Worker runs only ~0.3ms so no
// running/churn row outgrows the boost — the premise "the boost WOULD have
// jumped a competitor" holds by construction and is asserted.
const semLeadRealBelowPrimaryTrace = `
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (100) [002] .... 5.000400: tracing_mark_write: B|200|Texture upload(15573) 1140x1856
     worker-200 (100) [002] .... 5.002500: tracing_mark_write: E|200
     worker-200 (100) [002] .... 5.002600: tracing_mark_write: B|200|Texture upload(15563) 1140x1140
     worker-200 (100) [002] .... 5.005800: tracing_mark_write: E|200
     worker-200 (100) [002] .... 5.005900: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (100) [002] .... 5.006000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=D ==> next_comm=idle/2 next_pid=0 next_prio=120
      irq-2 (2) [002] .... 5.006100: sched_blocked_reason: pid=200 iowait=1 caller=f2fs_wait_on_block
      irq-2 (2) [002] .... 5.014100: sched_wakeup: comm=worker pid=200 prio=20 target_cpu=002
     worker-200 (100) [002] .... 5.014200: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (100) [002] .... 5.014300: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
     worker-200 (100) [002] .... 5.014400: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
        app-100 (100) [001] .... 5.014800: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
`

func TestSemLeadOrdinalFollowsPublishedEffectiveNotBoost(t *testing.T) {
	idx := buildTraceIndex(t, "semlead_real_below_primary.systrace", semLeadRealBelowPrimaryTrace)
	rank := BuildRootCauseRank(idx, Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.015, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	var fam *RootCauseRankItem
	for i := range rank.Items {
		item := &rank.Items[i]
		if item.Type == "texture_upload" {
			fam = item
		}
	}
	// CROWNSEM-1 (§40.28 ①): the credentialed non-target family enters the
	// ordinal board by its PUBLISHED effective (the boost stays a same-value
	// tie-break on the sort channel, never the published value).
	if fam == nil || fam.Rank <= 0 || fam.Tier == RootCauseTierContextOnly || fam.EffectiveImpactMs <= 0 ||
		(fam.RankSortBoostedEffectiveMs != 0 && fam.RankSortBoostedEffectiveMs <= fam.EffectiveImpactMs) {
		t.Fatalf("credentialed semantic family must seat by its published effective: %+v", rank.Items)
	}
}

// TestSemLeadBoostIsSameEffectiveTieBreak — 复核 P1-1 soft face #1: at EQUAL
// published effective on the on-chain tier, the boosted Score decides — the
// semantic family outranks the same-value non-semantic row; with the boost
// absent (control) the line-order tie-break decides instead. Unit grain on
// the pure sort function (the guards are typed-field comparisons only).
func TestSemLeadBoostIsSameEffectiveTieBreak(t *testing.T) {
	mk := func(boost float64) []RootCauseRankItem {
		other := RootCauseRankItem{
			Type: "d_state_or_io_wait", Thread: ThreadRef{Comm: "io", PID: 300},
			ImpactMs: 5.3, EffectiveImpactMs: 5.3, DStateMs: 5.3, Confidence: 0.55,
			ChainRelevance: "on_chain", Causality: "on_wakeup_chain", LineStart: 1,
		}
		other.Score = rootCauseRankScoreBasisMs(other) * other.Confidence * rootCauseItemScoreWeight(other)
		sem := RootCauseRankItem{
			Type: "texture_upload", SemanticClass: "texture_upload",
			Thread:   ThreadRef{Comm: "worker", PID: 200},
			ImpactMs: 5.3, EffectiveImpactMs: 5.3, Confidence: 0.55,
			RankSortBoostedEffectiveMs: boost,
			ChainRelevance:             "on_chain", Causality: "on_wakeup_chain", LineStart: 9,
		}
		sem.Score = rootCauseRankScoreBasisMs(sem) * sem.Confidence * rootCauseItemScoreWeight(sem)
		return []RootCauseRankItem{other, sem}
	}
	boosted := mk(11.13)
	sortRootCauseRankItems(boosted, true)
	if boosted[0].Type != "texture_upload" {
		t.Fatalf("at equal published effective the boosted Score must break the tie: %+v", boosted)
	}
	control := mk(0)
	sortRootCauseRankItems(control, true)
	if control[0].Type != "d_state_or_io_wait" {
		t.Fatalf("without the boost the legacy line-order tie-break decides: %+v", control)
	}
}

// TestSemLeadBoostCannotDisplaceStrictCapacityPrefix: the hidden semantic
// boost is only a same-effective tie-break. It cannot redeem a reserved seat
// or evict a row with larger published effective attribution.
func TestSemLeadBoostCannotDisplaceStrictCapacityPrefix(t *testing.T) {
	items := make([]RootCauseRankItem, 0, 8)
	for i := 0; i < 4; i++ {
		items = append(items, RootCauseRankItem{
			Type: "d_state_or_io_wait", Thread: ThreadRef{Comm: "io", PID: 300 + i},
			ImpactMs: 50 - float64(i), EffectiveImpactMs: 50 - float64(i), DStateMs: 50 - float64(i),
			ChainRelevance: "on_chain", LineStart: i + 1,
		})
	}
	mkSem := func(class string, pid int, eff, boost float64) RootCauseRankItem {
		return RootCauseRankItem{
			Type: class, SemanticClass: class, Thread: ThreadRef{Comm: "w", PID: pid},
			ImpactMs: eff, EffectiveImpactMs: eff, RankSortBoostedEffectiveMs: boost,
			ChainRelevance: "on_chain", LineStart: 100 + pid,
		}
	}
	// Four below-cut semantic families: the SMALLEST published value carries
	// the LARGEST boost and still must not win a seat.
	items = append(items,
		mkSem("jit_compile", 1, 4.0, 0),
		mkSem("shader_compile", 2, 3.0, 0),
		mkSem("class_verification", 3, 2.0, 0),
		mkSem("texture_upload", 4, 1.0, 9.9),
	)
	sortRootCauseRankItems(items, true)
	out, _ := selectRootCauseRankCapSurvivors(items, 4)
	if len(out) != 4 {
		t.Fatalf("limit must hold: %d", len(out))
	}
	kept := map[string]bool{}
	for _, item := range out {
		if item.SemanticClass != "" {
			kept[item.SemanticClass] = true
		}
	}
	if len(kept) != 0 {
		t.Fatalf("semantic boost must not displace the strict effective prefix: %+v rows=%+v", kept, out)
	}
}
