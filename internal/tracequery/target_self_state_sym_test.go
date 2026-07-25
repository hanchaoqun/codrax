package tracequery

// target_self_state_sym_test.go — SYM 批 engine pins (ledger
// real_trace_campaign_20260705.md §24.13 裁定一, user ruling 2026-07-08):
//
//   S1a ladder transparency — a rank row whose SUBJECT is the analysis target
//       (typed SubjectIsAnalysisTarget) AND whose token is 等待症状族 wears
//       RootCauseTierTargetSelfState, neither takes a
//       primary/secondary/tertiary election slot nor shifts the slots of the
//       causal rows below it.
//       EVOLUTION RECORD (SYM-2, §24.17, 2026-07-08): the covered family
//       narrowed to sleep 等唤醒族 / binder_wait / blocking_span; the 自因
//       可拆解族 (runnable / running / IO / D-state) competes again.
//       EVOLUTION RECORD (G9, §27.3 + §28.1 user ruling 2026-07-09): the
//       original "keeps its rank ordinal (榜位照发)" clause is SUPERSEDED —
//       demoted self-symptom rows now carry Rank=0 (no rank-board seat) and
//       the ordinal sequence stays contiguous over the rows that DO show a
//       seat (huadong_79/opendir_79 witness: visible boards read #6/#7/#12
//       with #1-#5 pre-consumed by demoted rows). The affected assertions
//       below evolved in place; election-slot semantics are unchanged.
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
		{Type: "jit_compile", ImpactMs: 2, EffectiveImpactMs: 2, ChainRelevance: "on_chain", Causality: "on_wakeup_chain"},
	}
	assignRootCauseRankOrdinalsAndTiers(items)
	if items[0].Tier != RootCauseTierTargetSelfState {
		t.Fatalf("the target's own binder-wait row must wear the self-state tier: %+v", items[0])
	}
	// EVOLUTION RECORD (G9, §28.1, 2026-07-09): was `Rank != 1` (榜位照发) —
	// the demoted self row now carries NO board ordinal, and the seats it used
	// to pre-consume go to the rows the board actually shows.
	if items[0].Rank != 0 {
		t.Fatalf("G9: the demoted self row must carry no rank ordinal: %+v", items[0])
	}
	// Ladder transparency (不占槽也不移位): the FIRST non-self row is the
	// positional primary, and the ladder below is unshifted.
	if items[1].Tier != "primary" || items[1].Rank != 1 {
		t.Fatalf("the primary election slot and ordinal #1 must fall to the first non-self row: %+v", items[1])
	}
	if items[2].Tier != "secondary" || items[2].Rank != 2 || items[3].Tier != "tertiary" || items[3].Rank != 3 {
		t.Fatalf("the ladder and ordinals below the transparent self row must be contiguous: %+v / %+v", items[2], items[3])
	}
}

func TestSYMSelfWaitSymptomFamilyDemotesSelfCauseFamilyCompetes(t *testing.T) {
	// EVOLUTION RECORD (SYM-2, ledger §24.17, 2026-07-08): this pin replaced
	// TestSYMSelfStateFamilyCoveredBySubjectIdentityNotStateType — §24.13's
	// "subject identity only, never a state/type token list" scope was REVISED
	// by §24.17 into the two-family split. 等待症状族 (sleep 等唤醒族 /
	// binder_wait / blocking_span 自持锁) keeps demoting; the 自因可拆解族
	// (runnable / running / IO / D-state) competes as decomposable self causes.
	for _, typ := range []string{"blocking_span", "binder_wait", "sleep_wait", "fragmented_sleep_wait", "missing_wakeup"} {
		items := []RootCauseRankItem{
			{Type: typ, SubjectIsAnalysisTarget: true, ImpactMs: 50, ChainRelevance: "on_chain", Causality: "on_wakeup_chain"},
			{Type: "workqueue_activity", ImpactMs: 9, ChainRelevance: "on_chain", Causality: "on_wakeup_chain"},
		}
		assignRootCauseRankOrdinalsAndTiers(items)
		if items[0].Tier != RootCauseTierTargetSelfState {
			t.Fatalf("self %s row (等待症状族) must wear the self-state tier: %+v", typ, items[0])
		}
		if items[1].Tier != "primary" {
			t.Fatalf("self %s row must not consume the election slot: %+v", typ, items[1])
		}
	}
	// 自因四态各一不降道 (§24.17): the self row takes the FIRST election slot
	// (primary) and the ladder below shifts normally — actionable self causes.
	for _, typ := range []string{"runnable_wait", "running", "io_latency", "d_state_or_io_wait"} {
		cause := RootCauseRankItem{Type: typ, SubjectIsAnalysisTarget: true, ImpactMs: 50,
			ChainRelevance: "on_chain", Causality: "on_wakeup_chain"}
		switch typ {
		case "runnable_wait":
			cause.RunnableMs = 50
			cause.DominantState = string(StateRunnable)
		case "running":
			cause.RunningMs = 50
			cause.DominantState = string(StateRunning)
			cause.EffectiveImpactMs = 50
		case "io_latency":
			cause.IOWaitMs = 50
			cause.DominantState = string(StateIOWait)
		case "d_state_or_io_wait":
			cause.DStateMs = 50
			cause.DominantState = string(StateDSleep)
		}
		items := []RootCauseRankItem{
			cause,
			{Type: "workqueue_activity", ImpactMs: 9, ChainRelevance: "on_chain", Causality: "on_wakeup_chain"},
		}
		sortRootCauseRankItems(items, true)
		assignRootCauseRankOrdinalsAndTiers(items)
		if items[0].Tier != "primary" {
			t.Fatalf("self %s row (自因可拆解族) must compete and take the primary slot: %+v", typ, items[0])
		}
		// G9 (§28.1): a COMPETING self-cause row carries a board seat — the
		// ordinal channel stays on rank-display rows.
		if items[0].Rank != 1 {
			t.Fatalf("self %s row keeps its ordinal: %+v", typ, items[0])
		}
		if items[1].Tier != "secondary" {
			t.Fatalf("the ladder below a competing self-cause row shifts normally: %+v", items[1])
		}
	}
}

func TestSYM2WaitSymptomClosedSetIsRegistryLane(t *testing.T) {
	// SYM-2 (§24.17) closed-set pin — the demotion predicate reads the
	// causal-token registry's lane column (wakeup_chain + lock_contention),
	// never prose. 突变自查: re-widening the predicate to全量 subject==target,
	// mis-assigning a lane in the registry, or hand-listing the wrong family
	// all bite here.
	// EVOLUTION RECORD (P9 §29.42 案1, 2026-07-12): pacing_idle joins the
	// wakeup-chain lane (waiting for the next frame tick), so the lane-derived
	// predicate is TRUE for it — honest family membership. The SYM demotion
	// consequence never applies in practice: assignRootCauseRanksAndTiers
	// demotes every pacing_idle row to RootCauseTierContextOnly through the
	// dedicated type arm BEFORE the self-symptom arm runs (any subject, not
	// just the target). Pinned in binder_attribution_p9_test.go.
	// EVOLUTION RECORD (GAP-B2, 2026-07-25): periodic_idle — pacing_idle's
	// generic-periodic sibling — registered on the same wakeup-chain lane, so
	// the lane-derived predicate is TRUE for it by the same honest-membership
	// argument; the dedicated ContextOnly type arm (query.go
	// assignRootCauseRanksAndTiers) already names both tokens.
	wantWait := map[string]bool{
		"sleep_wait": true, "fragmented_sleep_wait": true, "missing_wakeup": true,
		"binder_wait": true, "blocking_span": true, "pacing_idle": true,
		"periodic_idle": true,
	}
	for _, token := range CausalTokenUniverse() {
		spec, _ := CausalTokenSpecFor(token)
		if !spec.RowToken {
			continue
		}
		got := rootCauseItemIsTargetWaitSymptomType(RootCauseRankItem{Type: token})
		if got != wantWait[token] {
			t.Fatalf("wait-symptom closed set drifted at %q: got %v want %v", token, got, wantWait[token])
		}
	}
	// An unregistered token never demotes (absence never demotes).
	if rootCauseItemIsTargetWaitSymptomType(RootCauseRankItem{Type: "made_up_token"}) {
		t.Fatalf("unregistered tokens must compete")
	}
}

func TestSYM2SelfCauseRowUsesStrictPositionalElection(t *testing.T) {
	// §24.17: a competing self-cause row is eligible for the ordinary strict
	// positional election; the opendir-shape control stays on the WAIT family.
	items := []RootCauseRankItem{
		{Type: "workqueue_activity", ImpactMs: 9, ChainRelevance: "on_chain", Causality: "on_wakeup_chain"},
		{Type: "d_state_or_io_wait", SubjectIsAnalysisTarget: true, ImpactMs: 40,
			DStateMs: 40, DominantState: string(StateDSleep),
			ChainRelevance: "on_chain", Causality: "on_wakeup_chain"},
	}
	sortRootCauseRankItems(items, true)
	assignRootCauseRankOrdinalsAndTiers(items)
	if items[0].Type != "d_state_or_io_wait" || items[0].Tier != "primary" || items[1].Tier != "secondary" {
		t.Fatalf("a larger self D-state cause must win by strict measured position: %+v", items)
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
	sortRootCauseRankItems(items, true)
	assignRootCauseRankOrdinalsAndTiers(items)
	if items[0].Tier != "primary" {
		t.Fatalf("a counterpart binder-wait row must keep the positional primary tier: %+v", items[0])
	}
	if items[1].Tier != "secondary" {
		t.Fatalf("a counterpart resolved lock row must keep competing at its strict position: %+v", items[1])
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
	assignRootCauseRankOrdinalsAndTiers(items)
	if items[2].Tier != RootCauseTierTargetSelfState {
		t.Fatalf("the self-held resolved lock must wear the self-state tier, never co-primary: %+v", items[2])
	}
	// EVOLUTION RECORD (G9, §28.1, 2026-07-09): was `Rank != 3` (榜位照发) —
	// the demoted self lock row carries no board ordinal.
	if items[2].Rank != 0 {
		t.Fatalf("G9: the demoted self lock row must carry no rank ordinal: %+v", items[2])
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
	assignRootCauseRankOrdinalsAndTiers(items)
	if items[0].Tier != "primary" {
		t.Fatalf("the untargeted ladder is byte-identical to pre-SYM: %+v", items[0])
	}
}

// --- S1e: DCS interplay + BackgroundRank scope ---------------------------------

func TestSYMSemanticSpanOnTargetParticipatesAsCauseNotWaitSymptom(t *testing.T) {
	// A semantic span hosted ON the target thread remains actionable causal
	// work, not a wait-on-counterpart symptom. It participates normally and is
	// also mentioned through the independent optimization channel.
	items := []RootCauseRankItem{
		{Type: "jit_compile", SubjectIsAnalysisTarget: true, ImpactMs: 10, EffectiveImpactMs: 10, ChainRelevance: "on_chain", Causality: "on_wakeup_chain"},
		{Type: "workqueue_activity", ImpactMs: 9, ChainRelevance: "on_chain", Causality: "on_wakeup_chain"},
	}
	sortRootCauseRankItems(items, true)
	assignRootCauseRankOrdinalsAndTiers(items)
	if items[0].Tier != "primary" {
		t.Fatalf("a target-hosted on-chain compile span can be primary: %+v", items[0])
	}
	if items[1].Tier != "secondary" {
		t.Fatalf("semantic primary must consume the first positional slot: %+v", items[1])
	}
}

func TestSYMBackgroundRankCountingUntouched(t *testing.T) {
	// §24.13 scope guard (BackgroundRank 不涉): a NON-chain self row still
	// counts a background board position (the DCS F-2 position semantics:
	// every published non-on-chain row counts) while the FIELD stays stamped
	// on semantic rows only.
	items := []RootCauseRankItem{
		{Type: "sleep_wait", SubjectIsAnalysisTarget: true, ImpactMs: 12}, // non-chain self row
		{Type: "jit_compile", ImpactMs: 5, EffectiveImpactMs: 5},          // non-chain semantic span
	}
	assignRootCauseRankOrdinalsAndTiers(items)
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
	// target-only by collection — escape the typed identity, while the
	// build-side stamp kept every other pin green (假 PASS). This pin is the
	// 全称断言 over a full Run(): EVERY row whose subject tid equals the query
	// target carries the identity stamp, and the enrich-minted
	// scheduler_latency witness must be present. 突变自查 F3: deleting the
	// enrich-side stampRootCauseRankAnalysisTargetSubject call reds this.
	//
	// EVOLUTION RECORD (SYM-2, ledger §24.17, 2026-07-08): the §24.13-era 全称
	// "no target-subject row wears an election tier" is REVISED — the stamp
	// stays full-population (identity fact), but only the 等待症状族 demotes;
	// the enrich-minted scheduler_latency self rows (自因 runnable family) now
	// COMPETE and wear election-ladder tiers again (participation witness).
	idx := buildSampleIndex(t)
	res := Run(idx, Query{View: "root_cause_rank", PID: 20, TimeStart: 1.10, TimeEnd: 1.22, Limit: 12})
	if res.RootCauseRank == nil || len(res.RootCauseRank.Items) == 0 {
		t.Fatalf("expected ranked root-cause candidates, got %+v", res.RootCauseRank)
	}
	sawEnrichWitness := false
	sawSelfCauseElection := false
	for _, item := range res.RootCauseRank.Items {
		if item.Thread.PID != 20 {
			continue
		}
		if !item.SubjectIsAnalysisTarget {
			t.Fatalf("every target-subject row must carry the typed self identity (enrich additions included): %+v", item)
		}
		if item.Source == "scheduler_latency_stats" {
			sawEnrichWitness = true
		}
		isWait := rootCauseItemIsTargetWaitSymptomType(item)
		switch item.Tier {
		case "primary", "secondary", "tertiary":
			if isWait {
				t.Fatalf("a target-subject 等待症状族 row must never wear an election-ladder tier (§24.17): %+v", item)
			}
			sawSelfCauseElection = true
		case RootCauseTierTargetSelfState:
			if !isWait {
				t.Fatalf("the self tier must only land on the 等待症状族 (§24.17): %+v", item)
			}
		}
	}
	if !sawEnrichWitness {
		t.Fatalf("fixture drifted: expected an ENRICH-minted scheduler_latency row for the target: %+v", res.RootCauseRank.Items)
	}
	if !sawSelfCauseElection {
		t.Fatalf("participation witness drifted: expected a competing 自因族 target row wearing an election tier: %+v", res.RootCauseRank.Items)
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
