package types

// trace_causal_projection_levelmerge_test.go — LEVELMERGE-1 修补轮 (2026-07-18)
// aggregation-authority pins for the gated-share split family (方案 P 区间分账):
// the ×N per-seat-ledger clear (件2⑤) and the anchorForm account-identity
// forks (件2⑥), both on the ONE types-side merge/grouping authority.

import "testing"

func levelmergeTestConstituentNode(impact float64, lineStart int) TraceCausalProjectionNode {
	return TraceCausalProjectionNode{
		Role: TraceCausalRoleRootCauseContext, EvidenceID: "lm-agg-a",
		Subject: "dep_worker-200", Predicate: "root_cause_tertiary",
		Object: "runnable_wait", TypeToken: "runnable_wait", StateKind: "runnable",
		ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain",
		ImpactMS: impact, CumulativeImpactMS: impact, EffectiveImpactMS: impact,
		LineStart: lineStart, LineEnd: lineStart + 10, Confidence: 0.8,
		GatedShareClaimedMS: impact, GatedShareFullMS: impact + 5,
		GatedShareConstituentSeat: true,
		GatedShareClaimSeats:      []string{"300..305"},
	}
}

// 件2⑤ pin (M8): the ×N Σ row must not wear one member's claimed/full
// decomposition, its claim-seat pointers, or a member's fail-open overlap
// clause (「本行」 grammar has no true referent on a member Σ) — while the
// typed constituent MARKER survives the clear (the ◎ census, the merged
// row-2 self-explanation and the anchorForm fork all key on the bool, not
// the cleared floats).
func TestTraceCausalProjectionMergeClearsGatedShareAccounts(t *testing.T) {
	seed := levelmergeTestConstituentNode(10, 100)
	twin := levelmergeTestConstituentNode(8, 200)
	twin.EvidenceID = "lm-agg-a2"
	merged := TraceCausalProjectionMergeOccurrenceRows([]TraceCausalProjectionNode{seed, twin})
	if merged.GatedShareClaimedMS != 0 || merged.GatedShareFullMS != 0 ||
		merged.GatedShareClaimSeats != nil || merged.GatedShareOverlapDisclosureMS != 0 {
		t.Fatalf("the ×N Σ row must clear the per-seat gated-share accounts: claimed=%.3f full=%.3f seats=%v overlap=%.3f",
			merged.GatedShareClaimedMS, merged.GatedShareFullMS, merged.GatedShareClaimSeats, merged.GatedShareOverlapDisclosureMS)
	}
	if !merged.GatedShareConstituentSeat {
		t.Fatalf("the typed constituent marker must survive the clear (载 ◎ census + merged row-2 self-explanation + anchorForm fork)")
	}

	// The fail-open disclosure clause clears the same way when any member
	// carried one.
	discSeed := levelmergeTestConstituentNode(30, 400)
	discSeed.GatedShareConstituentSeat = false
	discSeed.GatedShareClaimedMS, discSeed.GatedShareFullMS = 0, 0
	discSeed.GatedShareOverlapDisclosureMS = 2.5
	discTwin := levelmergeTestConstituentNode(6, 500)
	discTwin.GatedShareConstituentSeat = false
	discTwin.GatedShareClaimedMS, discTwin.GatedShareFullMS = 0, 0
	discTwin.GatedShareClaimSeats = nil
	discTwin.EvidenceID = "lm-agg-d2"
	merged = TraceCausalProjectionMergeOccurrenceRows([]TraceCausalProjectionNode{discSeed, discTwin})
	if merged.GatedShareOverlapDisclosureMS != 0 || merged.GatedShareClaimSeats != nil {
		t.Fatalf("the ×N Σ row must clear the member's overlap disclosure: overlap=%.3f seats=%v",
			merged.GatedShareOverlapDisclosureMS, merged.GatedShareClaimSeats)
	}
}

// 件2⑥ pin (M9): the anchorForm account-identity forks — the demoted A
// constituent row and the carved residual B seat are each their OWN account
// form (never re-Σ with plain rows or with each other); the marker beats the
// value on the A row; a plain row stays "".
func TestTraceCausalProjectionAnchorFormKeyGatedShareForks(t *testing.T) {
	constituent := TraceCausalProjectionNode{GatedShareConstituentSeat: true, GatedShareFullMS: 15}
	if got := traceCausalProjectionAnchorFormKey(constituent); got != "gated_share_constituent" {
		t.Fatalf("constituent fork drifted: %q (the marker beats the value — the A row never re-Σs with plain rows or its residual twin)", got)
	}
	residual := TraceCausalProjectionNode{GatedShareFullMS: 15}
	if got := traceCausalProjectionAnchorFormKey(residual); got != "gated_share_residual" {
		t.Fatalf("residual fork drifted: %q (a carved account must never re-Σ with plain full accounts)", got)
	}
	if got := traceCausalProjectionAnchorFormKey(TraceCausalProjectionNode{}); got != "" {
		t.Fatalf("a plain row must keep the empty form key, got %q", got)
	}
	// The fail-open disclosure row deliberately keeps "" — its published
	// value IS the plain full account (no carve), and the ×N clear drops the
	// clause on any merged row.
	disclosure := TraceCausalProjectionNode{GatedShareOverlapDisclosureMS: 2.5}
	if got := traceCausalProjectionAnchorFormKey(disclosure); got != "" {
		t.Fatalf("the disclosure row publishes the plain full account and keeps the empty form key, got %q", got)
	}
}
