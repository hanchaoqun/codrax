package tracequery

// chain_credential_census_test.go — CHAINGUARD-1 pin family (§29.204/§29.204.1
// spec §5, 2026-07-22): INV-CHAINGUARD (chain-channel Rank>0 eff>0 seats of a
// chained board carry ≥1 typed credential stamp), the census closed enum's
// single mint point, the none-seat ▒ demotion + audit caveat, the exemption
// closed set (chainless boards / Rank==0 / eff≤0 / ⌗ side rail / R8 self), the
// (0,end) pseudo-interval second net (ISPGAP 主嫌疑独立网), the post-normalize
// mint timing hole (CHAINGUARD-F3), and the LANE-B RootEvidence node-window
// interval upgrade (件5).

import (
	"strings"
	"testing"
)

func chainguardSeat(typ string, pid int, runnableMs float64) RootCauseRankItem {
	return RootCauseRankItem{
		Type:              typ,
		Thread:            ThreadRef{Comm: "t", PID: pid},
		DominantState:     string(StateRunnable),
		RunnableMs:        runnableMs,
		ImpactMs:          runnableMs,
		EffectiveImpactMs: runnableMs,
		Score:             runnableMs,
	}
}

// TestChainguardCensusVerdictTiers pins the single mint point's closed-enum
// mapping over every stamp lane (S1..S8) plus the zero-stamp none verdict.
func TestChainguardCensusVerdictTiers(t *testing.T) {
	cases := []struct {
		name string
		item RootCauseRankItem
		want string
	}{
		{"self_deterministic_basis", RootCauseRankItem{OnChainBasis: RootCauseOnChainBasisSelfDeterministicSpan}, RootCauseChainCredentialCensusTargetSelf},
		{"self_wall_clock_basis", RootCauseRankItem{OnChainBasis: RootCauseOnChainBasisSelfWallClockInterval}, RootCauseChainCredentialCensusTargetSelf},
		{"self_causality_token", RootCauseRankItem{Causality: RootCauseCausalitySelfWallClock}, RootCauseChainCredentialCensusTargetSelf},
		{"r8_subject_is_target", RootCauseRankItem{SubjectIsAnalysisTarget: true, Causality: "on_wakeup_chain"}, RootCauseChainCredentialCensusTargetSelf},
		{"host_edge_span_basis", RootCauseRankItem{OnChainBasis: RootCauseOnChainBasisHostWakeupEdge}, RootCauseChainCredentialCensusWakeupAnchored},
		{"host_edge_state_basis", RootCauseRankItem{OnChainBasis: RootCauseOnChainBasisHostWakeupEdgeState}, RootCauseChainCredentialCensusWakeupAnchored},
		{"identity_inheritance", RootCauseRankItem{ChainIdentityInheritance: true}, RootCauseChainCredentialCensusMemberInherited},
		{"envelope_level", RootCauseRankItem{ChainCredentialEnvelopeLevel: true}, RootCauseChainCredentialCensusIntervalProven},
		{"rspa_anchored_share", RootCauseRankItem{ChainAnchoredMs: 4.5}, RootCauseChainCredentialCensusIntervalProven},
		{"constructive_wakeup_chain_source", RootCauseRankItem{Source: "wakeup_chain.causal_impacts"}, RootCauseChainCredentialCensusIntervalProven},
		{"measured_overlap_valid_interval", RootCauseRankItem{OverlapMs: 2.0, StartTs: 1.0, EndTs: 1.1}, RootCauseChainCredentialCensusIntervalProven},
		{"zero_stamp_violation", RootCauseRankItem{Causality: "on_wakeup_chain", ChainRelevance: "on_chain"}, RootCauseChainCredentialCensusNone},
	}
	for _, tc := range cases {
		if got := censusChainSeatCredential(tc.item, false); got != tc.want {
			t.Fatalf("%s: census=%q want %q", tc.name, got, tc.want)
		}
	}
}

// TestChainguardCensusNoneSeatDemotesToBackground — the violation disposition:
// a zero-stamp on-chain ranked seat (the LANE-G bare-membership satellite
// shape) withdraws its chain claim wholesale (relevance AND causality), lands
// on the honest ▒ background lane with values untouched, keeps the sticky
// census=none violation record, and the chain ordinal space repacks without
// it; the credentialed peer keeps rank#1 and the caveat discloses the seat.
func TestChainguardCensusNoneSeatDemotesToBackground(t *testing.T) {
	proven := chainguardSeat("runnable_wait", 200, 9)
	proven.Causality = "on_wakeup_chain"
	proven.ChainRelevance = "on_chain"
	proven.OverlapMs = 3
	proven.StartTs, proven.EndTs = 1.0, 1.01
	bare := chainguardSeat("scheduler_latency", 300, 5)
	bare.Causality = "on_wakeup_chain"
	bare.ChainRelevance = "on_chain"
	items := []RootCauseRankItem{proven, bare}
	caveat := assignRootCauseRanksAndTiers(items, true, false)
	if caveat == "" || !strings.HasPrefix(caveat, "chain_credential_census:") ||
		!strings.Contains(caveat, "scheduler_latency") {
		t.Fatalf("violation caveat must disclose the demoted seat, got %q", caveat)
	}
	if items[0].Rank != 1 || items[0].ChainCredentialCensus != RootCauseChainCredentialCensusIntervalProven {
		t.Fatalf("credentialed peer must keep rank#1 with its verdict: %+v", items[0])
	}
	demoted := items[1]
	if demoted.ChainRelevance != "background" || demoted.Causality != "background" {
		t.Fatalf("none seat must withdraw the chain claim wholesale: %+v", demoted)
	}
	if demoted.Rank != 0 || demoted.ChainCredentialCensus != RootCauseChainCredentialCensusNone {
		t.Fatalf("none seat must hold no chain ordinal and keep the violation record: %+v", demoted)
	}
	if demoted.RunnableMs != 5 || demoted.EffectiveImpactMs != 5 {
		t.Fatalf("value channels must move zero (值零动): %+v", demoted)
	}
}

// TestChainguardCensusChainlessBoardExempt — §29.36.2 既裁 chainless
// 单宇宙: the whole board is exempt; no census, no demotion, no caveat (the
// ISPGAP-1 display gates own that face).
func TestChainguardCensusChainlessBoardExempt(t *testing.T) {
	bare := chainguardSeat("runnable_wait", 300, 5)
	items := []RootCauseRankItem{bare}
	caveat := assignRootCauseRanksAndTiers(items, false, false)
	if caveat != "" {
		t.Fatalf("chainless board must raise no census caveat, got %q", caveat)
	}
	if items[0].ChainCredentialCensus != "" || items[0].ChainRelevance != "" || items[0].Rank != 1 {
		t.Fatalf("chainless fail-open ordinal space must stay untouched: %+v", items[0])
	}
}

// TestChainguardCensusExemptPopulations — Rank==0 / eff≤0 seats never enter
// the census population: no verdict, no demotion even with zero stamps.
func TestChainguardCensusExemptPopulations(t *testing.T) {
	zeroEff := chainguardSeat("runnable_wait", 300, 0)
	zeroEff.Causality = "on_wakeup_chain"
	zeroEff.ChainRelevance = "on_chain"
	items := []RootCauseRankItem{zeroEff}
	caveat := assignRootCauseRanksAndTiers(items, true, false)
	if caveat != "" {
		t.Fatalf("zero-effective seat is outside the census population, got caveat %q", caveat)
	}
	if items[0].ChainCredentialCensus != "" || items[0].ChainRelevance != "on_chain" {
		t.Fatalf("out-of-population row must stay byte-identical: %+v", items[0])
	}
}

// TestChainguardCensusPseudoIntervalOverlapNotACredential — the ISPGAP 主嫌疑
// independent second net (spec S6): a (0,end) pseudo-interval's prefix-envelope
// overlap is NOT a stamp (0=unset sentinel), so the seat demotes; the SAME
// shape on a determined rebased [0,end] run (WINFLAG member-side semantics)
// keeps its measured-overlap credential byte-identically.
func TestChainguardCensusPseudoIntervalOverlapNotACredential(t *testing.T) {
	pseudo := chainguardSeat("d_state_or_io_wait", 1225, 144.5)
	pseudo.DominantState = string(StateDSleep)
	pseudo.DStateMs, pseudo.RunnableMs = 144.5, 0
	pseudo.Causality = "on_wakeup_chain"
	pseudo.ChainRelevance = "on_chain"
	pseudo.OverlapMs = 144.5
	pseudo.StartTs, pseudo.EndTs = 0, 1.4
	items := []RootCauseRankItem{pseudo}
	if caveat := assignRootCauseRanksAndTiers(items, true, false); caveat == "" {
		t.Fatal("the (0,end) pseudo-interval overlap must not count as a credential")
	}
	if items[0].ChainRelevance != "background" || items[0].ChainCredentialCensus != RootCauseChainCredentialCensusNone {
		t.Fatalf("pseudo-interval seat must demote with the violation record: %+v", items[0])
	}
	rebased := []RootCauseRankItem{pseudo}
	if caveat := assignRootCauseRanksAndTiers(rebased, true, true); caveat != "" {
		t.Fatalf("a determined [0,end] run keeps ts==0 members usable (WINFLAG), got %q", caveat)
	}
	if rebased[0].ChainCredentialCensus != RootCauseChainCredentialCensusIntervalProven || rebased[0].Rank != 1 {
		t.Fatalf("rebased-run seat must keep its measured-overlap credential: %+v", rebased[0])
	}
}

// TestChainguardCensusIdempotentAcrossRepublish — P5: the census recomputes
// idempotently at each publication (the enrich lane re-runs the authority);
// a demoted seat stays background with its sticky none record and the second
// pass raises no duplicate caveat.
func TestChainguardCensusIdempotentAcrossRepublish(t *testing.T) {
	proven := chainguardSeat("runnable_wait", 200, 9)
	proven.Causality = "on_wakeup_chain"
	proven.ChainRelevance = "on_chain"
	proven.OverlapMs = 3
	proven.StartTs, proven.EndTs = 1.0, 1.01
	bare := chainguardSeat("scheduler_latency", 300, 5)
	bare.Causality = "on_wakeup_chain"
	bare.ChainRelevance = "on_chain"
	items := []RootCauseRankItem{proven, bare}
	if caveat := assignRootCauseRanksAndTiers(items, true, false); caveat == "" {
		t.Fatal("first publication must demote the zero-stamp seat")
	}
	if caveat := assignRootCauseRanksAndTiers(items, true, false); caveat != "" {
		t.Fatalf("re-publication over the settled board must raise no second caveat, got %q", caveat)
	}
	if items[1].ChainRelevance != "background" || items[1].ChainCredentialCensus != RootCauseChainCredentialCensusNone {
		t.Fatalf("the violation record must survive the re-publication (sticky admission history): %+v", items[1])
	}
	if items[0].Rank != 1 || items[0].ChainCredentialCensus != RootCauseChainCredentialCensusIntervalProven {
		t.Fatalf("credentialed seat must stay stable across republish: %+v", items[0])
	}
}

// TestChainguardCensusPostNormalizeMintCannotEscape — CHAINGUARD-F3 timing
// hole, mechanical guard: a row minted AFTER normalize with EMPTY relevance
// rides the chain channel fail-open — the census (living inside the ordinal
// publication authority) still catches it: zero stamps → ▒ + caveat.
func TestChainguardCensusPostNormalizeMintCannotEscape(t *testing.T) {
	proven := chainguardSeat("runnable_wait", 200, 9)
	proven.Causality = "on_wakeup_chain"
	proven.ChainRelevance = "on_chain"
	proven.OverlapMs = 3
	proven.StartTs, proven.EndTs = 1.0, 1.01
	items := []RootCauseRankItem{proven}
	normalizeRootCauseChainRelevance(items, true)
	// The escape shape: a post-normalize mint lane appends a bare row with
	// no relevance/causality at all (the empty token rides the chain channel
	// through the ordinal fail-open).
	items = append(items, chainguardSeat("runnable_wait", 999, 150))
	caveat := assignRootCauseRanksAndTiers(items, true, false)
	if caveat == "" || !strings.Contains(caveat, "999") && !strings.Contains(caveat, "runnable_wait") {
		t.Fatalf("the post-normalize mint must be caught by the census, got %q", caveat)
	}
	if items[1].ChainRelevance != "background" || items[1].Rank != 0 ||
		items[1].ChainCredentialCensus != RootCauseChainCredentialCensusNone {
		t.Fatalf("the escaped mint must demote with the violation record: %+v", items[1])
	}
	if items[0].Rank != 1 {
		t.Fatalf("the credentialed seat must keep rank#1 after the repack: %+v", items[0])
	}
}

// TestChainguardRootEvidenceSeatNodeWindow — 件5 (LANE-B) unit half: the ONE
// same-pid positive node window resolves; zero windows, ambiguous windows and
// non-positive windows never guess.
func TestChainguardRootEvidenceSeatNodeWindow(t *testing.T) {
	chain := ChainResult{Nodes: []ChainNode{
		{Thread: ThreadRef{PID: 300}, Window: TimeWindow{StartTs: 1.0, EndTs: 1.2}},
		{Thread: ThreadRef{PID: 400}, Window: TimeWindow{StartTs: 1.0, EndTs: 1.1}},
		{Thread: ThreadRef{PID: 400}, Window: TimeWindow{StartTs: 1.3, EndTs: 1.4}},
		{Thread: ThreadRef{PID: 500}, Window: TimeWindow{StartTs: 2.0, EndTs: 2.0}},
	}}
	if window, ok := rootEvidenceSeatNodeWindow(chain, ThreadRef{PID: 300}); !ok ||
		window.StartTs != 1.0 || window.EndTs != 1.2 {
		t.Fatalf("single positive node window must resolve, got %+v %v", window, ok)
	}
	if _, ok := rootEvidenceSeatNodeWindow(chain, ThreadRef{PID: 400}); ok {
		t.Fatal("ambiguous multi-window pid must never guess an interval")
	}
	if _, ok := rootEvidenceSeatNodeWindow(chain, ThreadRef{PID: 500}); ok {
		t.Fatal("non-positive node window must never resolve")
	}
	if _, ok := rootEvidenceSeatNodeWindow(chain, ThreadRef{PID: 600}); ok {
		t.Fatal("absent pid must never resolve")
	}
}

// TestChainguardRootEvidenceSeatIntervalUpgrade — 件5 (LANE-B) end-to-end
// half: a NON-target state RootEvidence seat whose pid holds exactly ONE
// chain node window publishes that window as its own typed interval — the
// enrich then MEASURES the same-pid overlap (real measurement of a published
// envelope, not the retired ONCHAIN-FIX-1 fabrication) and the census
// upgrades the seat 身份继承档 → 区间档. The ambiguous multi-node sibling
// keeps the §29.134 identity-inheritance arm byte-identically (negative arm).
func TestChainguardRootEvidenceSeatIntervalUpgrade(t *testing.T) {
	target := ThreadRef{Comm: "app", PID: 100}
	dep := ThreadRef{Comm: "io-dep", PID: 300}
	multi := ThreadRef{Comm: "io-multi", PID: 400}
	chain := ChainResult{
		Target: target,
		Nodes: []ChainNode{
			{ID: "target", Thread: target, Window: TimeWindow{StartTs: 5.000, EndTs: 5.010}},
			{ID: "dep", Thread: dep, Depth: 1, Window: TimeWindow{StartTs: 5.002, EndTs: 5.006}},
			{ID: "multi-a", Thread: multi, Depth: 1, Window: TimeWindow{StartTs: 5.001, EndTs: 5.003}},
			{ID: "multi-b", Thread: multi, Depth: 2, Window: TimeWindow{StartTs: 5.007, EndTs: 5.009}},
		},
		RootEvidence: []RootEvidence{{
			Type: "io_wait", Thread: dep, DurationMs: 2,
			LineStart: 60, LineEnd: 62, Summary: "dep io wait", Confidence: 0.8,
		}, {
			Type: "io_wait", Thread: multi, DurationMs: 1,
			LineStart: 70, LineEnd: 72, Summary: "multi io wait", Confidence: 0.8,
		}},
	}
	rank := buildRootCauseRankFrom(nil, Query{TimeStart: 5.000, TimeEnd: 5.010, Limit: 12}, chain, WindowStats{})
	var single, ambiguous *RootCauseRankItem
	for i := range rank.Items {
		item := &rank.Items[i]
		if item.Type != "io_wait" || item.Source != "wakeup_chain" {
			continue
		}
		switch item.Thread.PID {
		case dep.PID:
			single = item
		case multi.PID:
			ambiguous = item
		}
	}
	if single == nil {
		t.Fatalf("the single-node dep RootEvidence seat must mint: %+v", rank.Items)
	}
	if single.StartTs != 5.002 || single.EndTs != 5.006 {
		t.Fatalf("件5: the seat must publish its root node window as its typed interval: %+v", single)
	}
	if single.ChainRelevance != "on_chain" || single.OverlapMs <= 0 {
		t.Fatalf("件5: the published envelope must yield a MEASURED same-pid overlap: %+v", single)
	}
	if single.ChainIdentityInheritance {
		t.Fatalf("件5: the interval-carrying seat no longer rides the identity-inheritance arm: %+v", single)
	}
	if single.ChainCredentialCensus != RootCauseChainCredentialCensusIntervalProven {
		t.Fatalf("件5: the census must record the 区间档 upgrade, got %q", single.ChainCredentialCensus)
	}
	if ambiguous == nil {
		t.Fatalf("the multi-node RootEvidence seat must mint: %+v", rank.Items)
	}
	if ambiguous.EndTs > ambiguous.StartTs {
		t.Fatalf("负臂: an ambiguous multi-window pid must never guess an interval: %+v", ambiguous)
	}
	if !ambiguous.ChainIdentityInheritance || ambiguous.ChainRelevance != "on_chain" {
		t.Fatalf("负臂: the interval-less sibling keeps the §29.134 inheritance keep byte-identically: %+v", ambiguous)
	}
	if ambiguous.ChainCredentialCensus != RootCauseChainCredentialCensusMemberInherited {
		t.Fatalf("负臂: the inheritance stamp IS its census credential, got %q", ambiguous.ChainCredentialCensus)
	}
}

// TestChainguardChurnSameAccountNegativeArms — 件4 negative arms: the relaxed
// same-account absorption fires ONLY on the µs-exact scalar + exact dominant
// state identity; a different value or a different dominant state keeps the
// honest two-row publication byte-identically.
func TestChainguardChurnSameAccountNegativeArms(t *testing.T) {
	member := chainguardSeat("runnable_wait", 20, 5)
	member.Source = "window_stats"
	member.StatsWindowStartTs, member.StatsWindowEndTs = 11.0, 11.008
	member.LineStart, member.LineEnd = 3, 22
	churn := chainguardSeat("fragmented_runnable_wait", 20, 4.5)
	churn.Source = "window_stats.state_churn"
	churn.StatsWindowStartTs, churn.StatsWindowEndTs = 11.0, 11.008
	churn.LineStart, churn.LineEnd = 5, 20
	rank := &RootCauseRankResult{Items: []RootCauseRankItem{member, churn}}
	reconcileExactCrossTypeRankSeats(rank)
	if len(rank.Items) != 2 || len(rank.AbsorbedItems) != 0 {
		t.Fatalf("value mismatch must keep both seats: items=%d absorbed=%d", len(rank.Items), len(rank.AbsorbedItems))
	}
	churn.RunnableMs, churn.ImpactMs, churn.EffectiveImpactMs = 5, 5, 5
	churn.DominantState = string(StateRunning)
	rank = &RootCauseRankResult{Items: []RootCauseRankItem{member, churn}}
	reconcileExactCrossTypeRankSeats(rank)
	if len(rank.Items) != 2 || len(rank.AbsorbedItems) != 0 {
		t.Fatalf("dominant-state mismatch must keep both seats: items=%d absorbed=%d", len(rank.Items), len(rank.AbsorbedItems))
	}
	churn.DominantState = string(StateRunnable)
	rank = &RootCauseRankResult{Items: []RootCauseRankItem{member, churn}}
	reconcileExactCrossTypeRankSeats(rank)
	if len(rank.Items) != 1 || len(rank.AbsorbedItems) != 1 {
		t.Fatalf("the µs-exact same-account pair must absorb into one seat: items=%d absorbed=%d", len(rank.Items), len(rank.AbsorbedItems))
	}
}
