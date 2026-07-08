package tracequery

// target_self_state_sym_test.go — SYM 批 engine pins (ledger
// real_trace_campaign_20260705.md §24.13 裁定一, user ruling 2026-07-08):
//
//   S1a ladder transparency — a rank row whose SUBJECT is the analysis target
//       (typed SubjectIsAnalysisTarget) wears RootCauseTierTargetSelfState,
//       neither takes a primary/secondary/tertiary election slot nor shifts
//       the slots of the causal rows below it, and keeps its rank ordinal
//       (榜位照发). Covered family: 自持锁 blocking_span / binder_wait /
//       sleep / runnable / running self rows alike — the arm keys on SUBJECT
//       identity, never on state type.
//   S1b counterpart rows keep competing — the peer/holder side of the SAME
//       contention (subject != target) is untouched (突变自查: re-keying the
//       criterion on a state-type match bites here).
//   S1c the self row never rides the co-primary promotion (opendir_78
//       witness: the target's self-held AssetManager lock — a RESOLVED
//       blocking_span, co-primary eligible pre-SYM — wore rank#1
//       tier=primary and was crowned 主根因 for the target's own jank).
//   S1d stamp identity is tid-first (sameThreadRef, the engine's existing
//       target identity lane): PID equality decides whenever both sides
//       carry a tid; comm only engages when a side has none; an unresolved
//       target stamps nothing.
//   S1e DCS interplay — a semantic compile span hosted ON the target thread
//       keeps deterministic_optimization (the DCS arm wins); BackgroundRank
//       counting is untouched by the self arm (§24.13 scope: 只动选举槽).

import (
	"strings"
	"testing"
)

// --- S1a: ladder transparency + ordinal preservation --------------------------

func TestSYMSelfRowsTransparentToElectionLadder(t *testing.T) {
	items := []RootCauseRankItem{
		{Type: "binder_wait", SubjectIsAnalysisTarget: true, ImpactMs: 90, ChainRelevance: "on_chain", Causality: "on_wakeup_chain"},
		{Type: "workqueue_activity", ImpactMs: 9, ChainRelevance: "on_chain", Causality: "on_wakeup_chain"},
		{Type: "dma_fence_activity", ImpactMs: 5, ChainRelevance: "on_chain", Causality: "on_wakeup_chain"},
		{Type: "sched_stat_accounting", ImpactMs: 2, ChainRelevance: "background"},
	}
	assignRootCauseRanksAndTiers(items)
	if items[0].Tier != RootCauseTierTargetSelfState {
		t.Fatalf("the target's own binder-wait row must wear the self-state tier: %+v", items[0])
	}
	if items[0].Rank != 1 {
		t.Fatalf("榜位照发: the self row keeps its rank ordinal: %+v", items[0])
	}
	// Ladder transparency (不占槽也不移位): the FIRST non-self row is the
	// positional primary, and the ladder below is unshifted.
	if items[1].Tier != "primary" {
		t.Fatalf("the primary election slot must fall to the first non-self row: %+v", items[1])
	}
	if items[2].Tier != "secondary" || items[3].Tier != "tertiary" {
		t.Fatalf("the ladder below the transparent self row must be unshifted: %+v / %+v", items[2], items[3])
	}
}

func TestSYMSelfStateFamilyCoveredBySubjectIdentityNotStateType(t *testing.T) {
	// 全族覆盖: 自持锁 blocking_span / binder_wait / sleep / runnable / running
	// self rows all wear the tier — the arm reads the typed subject identity,
	// never a state/type token list.
	for _, typ := range []string{"blocking_span", "binder_wait", "sleep_wait", "runnable_wait", "running"} {
		items := []RootCauseRankItem{
			{Type: typ, SubjectIsAnalysisTarget: true, ImpactMs: 50, ChainRelevance: "on_chain", Causality: "on_wakeup_chain"},
			{Type: "workqueue_activity", ImpactMs: 9, ChainRelevance: "on_chain", Causality: "on_wakeup_chain"},
		}
		assignRootCauseRanksAndTiers(items)
		if items[0].Tier != RootCauseTierTargetSelfState {
			t.Fatalf("self %s row must wear the self-state tier: %+v", typ, items[0])
		}
		if items[1].Tier != "primary" {
			t.Fatalf("self %s row must not consume the election slot: %+v", typ, items[1])
		}
	}
}

// --- S1b: counterpart rows keep competing (state-type mutation guard) ---------

func TestSYMCounterpartRowsKeepCompeting(t *testing.T) {
	// The peer side of the same contention family (subject != target — the
	// flag is NOT stamped) competes through the ladder exactly as before.
	// 突变自查 guard: an implementation that re-keys the exclusion on the
	// state/type token (e.g. "every binder_wait/blocking_span row") bites
	// here — these are the opendir_02 counterpart shapes (RxComputationT 持锁
	// / peer binder wait) that MUST stay electable.
	items := []RootCauseRankItem{
		{Type: "binder_wait", ImpactMs: 40, ChainRelevance: "on_chain", Causality: "on_wakeup_chain"},
		{Type: "blocking_span", ImpactMs: 30, ChainRelevance: "on_chain", Causality: "on_wakeup_chain",
			BlockingKind: "monitor_contention", BlockingPeer: ThreadRef{Comm: "waiter", PID: 77}},
	}
	assignRootCauseRanksAndTiers(items)
	if items[0].Tier != "primary" {
		t.Fatalf("a counterpart binder-wait row must keep the positional primary tier: %+v", items[0])
	}
	if items[1].Tier != "primary" {
		t.Fatalf("a counterpart resolved lock row must keep its co-primary lane: %+v", items[1])
	}
}

// --- S1c: the self row never rides the co-primary promotion -------------------

func TestSYMSelfRowNeverRidesCoPrimary(t *testing.T) {
	// opendir_78 E1 shape: a RESOLVED self-held lock (BlockingKind +
	// resolved peer — the exact typed pair the co-primary blocking_span arm
	// admits) placed after two non-self rows. Pre-SYM the co-primary
	// promotion made it tier=primary regardless of its ladder position.
	items := []RootCauseRankItem{
		{Type: "workqueue_activity", ImpactMs: 9, ChainRelevance: "on_chain", Causality: "on_wakeup_chain"},
		{Type: "dma_fence_activity", ImpactMs: 5, ChainRelevance: "on_chain", Causality: "on_wakeup_chain"},
		{Type: "blocking_span", SubjectIsAnalysisTarget: true, ImpactMs: 115.944,
			ChainRelevance: "on_chain", Causality: "on_wakeup_chain",
			BlockingKind: "monitor_contention", SubjectIsLockHolder: true,
			BlockingPeer: ThreadRef{Comm: "LegoHandler", PID: 16865}},
	}
	assignRootCauseRanksAndTiers(items)
	if items[2].Tier != RootCauseTierTargetSelfState {
		t.Fatalf("the self-held resolved lock must wear the self-state tier, never co-primary: %+v", items[2])
	}
	if items[2].Rank != 3 {
		t.Fatalf("榜位照发: the self lock row keeps its ordinal: %+v", items[2])
	}
}

// --- S1d: tid-first stamp identity ---------------------------------------------

func TestSYMStampTidFirstIdentity(t *testing.T) {
	target := ThreadRef{Comm: "ui", PID: 100}
	items := []RootCauseRankItem{
		{Type: "binder_wait", Thread: ThreadRef{Comm: "other-name", PID: 100}}, // tid match, name drift
		{Type: "binder_wait", Thread: ThreadRef{Comm: "ui", PID: 101}},         // tid mismatch, same comm
		{Type: "binder_wait", Thread: ThreadRef{Comm: "ui"}},                   // no tid → comm lane
		{Type: "supply_pressure", Thread: ThreadRef{}},                         // aggregate row, no subject
	}
	stampRootCauseRankAnalysisTargetSubject(items, target)
	if !items[0].SubjectIsAnalysisTarget {
		t.Fatalf("tid-first: PID equality decides even when the label drifted (双名并列): %+v", items[0])
	}
	if items[1].SubjectIsAnalysisTarget {
		t.Fatalf("tid-first: a same-comm row with a DIFFERENT tid is another thread: %+v", items[1])
	}
	if !items[2].SubjectIsAnalysisTarget {
		t.Fatalf("the comm arm engages only when a side has no tid: %+v", items[2])
	}
	if items[3].SubjectIsAnalysisTarget {
		t.Fatalf("a subjectless aggregate row never matches: %+v", items[3])
	}
}

func TestSYMUnresolvedTargetStampsNothing(t *testing.T) {
	// Absence never guesses: an untargeted rank leaves every row competing.
	items := []RootCauseRankItem{
		{Type: "binder_wait", Thread: ThreadRef{Comm: "ui", PID: 100}, ImpactMs: 40, ChainRelevance: "on_chain", Causality: "on_wakeup_chain"},
	}
	stampRootCauseRankAnalysisTargetSubject(items, ThreadRef{})
	if items[0].SubjectIsAnalysisTarget {
		t.Fatalf("an unresolved target must stamp nothing: %+v", items[0])
	}
	assignRootCauseRanksAndTiers(items)
	if items[0].Tier != "primary" {
		t.Fatalf("the untargeted ladder is byte-identical to pre-SYM: %+v", items[0])
	}
}

// --- S1e: DCS interplay + BackgroundRank scope ---------------------------------

func TestSYMSemanticSpanOnTargetKeepsOptimizationTier(t *testing.T) {
	// A compile span hosted ON the target thread stays a deterministic
	// optimization point (the DCS arm wins over the self arm): actionable
	// guidance, not a symptom. DCS pin 族不扰.
	items := []RootCauseRankItem{
		{Type: "jit_compile", SubjectIsAnalysisTarget: true, ImpactMs: 5, ChainRelevance: "on_chain", Causality: "on_wakeup_chain"},
		{Type: "workqueue_activity", ImpactMs: 9, ChainRelevance: "on_chain", Causality: "on_wakeup_chain"},
	}
	assignRootCauseRanksAndTiers(items)
	if items[0].Tier != RootCauseTierDeterministicOptimization {
		t.Fatalf("a target-hosted on-chain compile span keeps the optimization tier: %+v", items[0])
	}
	if items[1].Tier != "primary" {
		t.Fatalf("ladder unshifted: %+v", items[1])
	}
}

func TestSYMBackgroundRankCountingUntouched(t *testing.T) {
	// §24.13 scope guard (BackgroundRank 不涉): a NON-chain self row still
	// counts a background board position (the DCS F-2 position semantics:
	// every published non-on-chain row counts) while the FIELD stays stamped
	// on semantic rows only.
	items := []RootCauseRankItem{
		{Type: "sleep_wait", SubjectIsAnalysisTarget: true, ImpactMs: 12}, // non-chain self row
		{Type: "jit_compile", ImpactMs: 5},                                // non-chain semantic span
	}
	assignRootCauseRanksAndTiers(items)
	if items[0].Tier != RootCauseTierTargetSelfState || items[0].BackgroundRank != 0 {
		t.Fatalf("the non-chain self row wears the self tier and never the background_rank FIELD: %+v", items[0])
	}
	if items[1].BackgroundRank != 2 {
		t.Fatalf("the background position must still COUNT the self row (F-2 semantics untouched): %+v", items[1])
	}
}

// --- 复核 F3: the ENRICH pass stamps its own additions ---------------------------

func TestSYMEnrichAdditionsCarrySelfStamp(t *testing.T) {
	// 复核 F3 (自设突变曾全绿存活): removing the enrich-side re-stamp let the
	// scheduler_latency lane — the highest-frequency ENRICH-minted family, and
	// target-only by collection — resurrect the target's own runnable wait as
	// tier=primary, while the build-side stamp kept every other pin green
	// (假 PASS). This pin is the 全称断言 over a full Run(): NO row whose
	// subject tid equals the query target may wear an election-ladder tier,
	// and the enrich-minted scheduler_latency witness must be present and
	// self-stamped. 突变自查 F3: deleting the enrich-side
	// stampRootCauseRankAnalysisTargetSubject call reds this.
	idx := buildSampleIndex(t)
	res := Run(idx, Query{View: "root_cause_rank", PID: 20, TimeStart: 1.10, TimeEnd: 1.22, Limit: 12})
	if res.RootCauseRank == nil || len(res.RootCauseRank.Items) == 0 {
		t.Fatalf("expected ranked root-cause candidates, got %+v", res.RootCauseRank)
	}
	sawEnrichWitness := false
	for _, item := range res.RootCauseRank.Items {
		if item.Thread.PID != 20 {
			continue
		}
		if item.Source == "scheduler_latency_stats" {
			sawEnrichWitness = true
		}
		if !item.SubjectIsAnalysisTarget {
			t.Fatalf("every target-subject row must carry the typed self identity (enrich additions included): %+v", item)
		}
		switch item.Tier {
		case "primary", "secondary", "tertiary":
			t.Fatalf("a target-subject row must never wear an election-ladder tier (§24.13 全称): %+v", item)
		}
	}
	if !sawEnrichWitness {
		t.Fatalf("fixture drifted: expected an ENRICH-minted scheduler_latency row for the target: %+v", res.RootCauseRank.Items)
	}
}

// --- wire face: the tier token reaches the predicate/claim-key face ------------

func TestSYMTierTokenIsWireStable(t *testing.T) {
	// The display primary bucket excludes self rows via the root_cause_primary
	// prefix mismatch — the token itself is load-bearing wire vocabulary.
	if RootCauseTierTargetSelfState != "target_self_state" {
		t.Fatalf("wire token pinned by §24.13: %q", RootCauseTierTargetSelfState)
	}
	if strings.HasPrefix("root_cause_"+RootCauseTierTargetSelfState, "root_cause_primary") {
		t.Fatalf("the self tier predicate must never match the primary prefix")
	}
}
