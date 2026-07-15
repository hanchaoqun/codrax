package tracequery

// rank_chain_anchor_rspa_rnb2_test.go — RNB-2 engine pins (2026-07-15):
//
//	件3 (§29.88 W3 病③): the EN remainder sentence's ownership bracket forks
//	    on the typed zero-anchored form — no seat holds a 0.000 anchored
//	    share, so neither "(owned by the chain seat)" nor the D1 unpublished
//	    downgrade may render (both would name a nonexistent holder);
//	件6 (§29.90 残留③ P4): the divergent double-account sentence names TWO
//	    anchored quantities (this seat's own share vs the pid-census ledger
//	    Σ) — when the typed floats differ each mention carries its account
//	    qualifier so one word never puns two values; µs-equal pairs keep the
//	    pinned bytes.

import (
	"strings"
	"testing"
)

// 件3 EN sister: anchored==0 speaks the no-share bracket; anchored>0 keeps the
// ownership claim byte-identically.
func TestRNB2RemainderSummaryZeroAnchoredSpeaksNoShare(t *testing.T) {
	zero := rspaRemainderSummary(ThreadRef{PID: 6666, Comm: "workShark"}, "D/IO blocking", 9.272, 0, 9.272)
	if !strings.Contains(zero, rspaSummaryNoAnchoredShare) {
		t.Fatalf("zero-anchored remainder summary must speak the no-share bracket: %q", zero)
	}
	if strings.Contains(zero, rspaSummaryOwnedByChainSeat) ||
		strings.Contains(zero, rspaSummaryOwnedByChainSeatUnpublished) {
		t.Fatalf("zero-anchored remainder summary must not claim any owner: %q", zero)
	}
	owned := rspaRemainderSummary(ThreadRef{PID: 6666, Comm: "workShark"}, "D/IO blocking", 33.159, 3.598, 36.757)
	if !strings.Contains(owned, rspaSummaryOwnedByChainSeat) {
		t.Fatalf("positive-anchored remainder summary must keep the ownership claim: %q", owned)
	}
}

// 件6: the divergent sentence's two anchored mentions disambiguate exactly
// when the typed floats differ (row-level account vs pid-census Σ), and keep
// the pinned bytes when µs-equal.
func TestRNB2DivergentSummaryDisambiguatesTwoAnchoredValues(t *testing.T) {
	thread := ThreadRef{PID: 59953, Comm: "sat"}
	differing := rspaRemainderSummaryDivergent(thread, "runnable (scheduling-pressure candidate)", 2.438, 8.338, 10.776, 5.0, 7.1)
	if !strings.Contains(differing, "anchored inside typed dependency windows (this seat's own account)") ||
		!strings.Contains(differing, "the pid-wide anchored ledger sum is 7.100ms") {
		t.Fatalf("differing anchored values must each carry their account qualifier: %q", differing)
	}
	equal := rspaRemainderSummaryDivergent(thread, "runnable (scheduling-pressure candidate)", 0.017, 2.266, 2.283, 2.181, 2.266)
	if !strings.Contains(equal, "2.266ms anchored inside typed dependency windows + this remainder") ||
		!strings.Contains(equal, "the anchored ledger sum is 2.266ms") {
		t.Fatalf("µs-equal pairs must keep the pinned bytes: %q", equal)
	}
	if strings.Contains(equal, "this seat's own account") || strings.Contains(equal, "pid-wide") {
		t.Fatalf("the qualifier must not ride equal pairs: %q", equal)
	}
}

// 件3 twin-patch no-op by construction: the zero form carries no ownership
// substring, so the twin-visibility rewrite (whose 「seat not published」 claim
// would be equally false on a share nobody holds) leaves it byte-identical.
func TestRNB2TwinVisibilityPatchNoOpOnZeroAnchoredForm(t *testing.T) {
	items := []RootCauseRankItem{{
		Type: "d_state_or_io_wait", Thread: ThreadRef{PID: 6666, Comm: "workShark"},
		DominantState: string(StateDSleep), ChainRelevance: "adjacent",
		DStateMs: 9.272, ImpactMs: 9.272, CumulativeImpactMs: 9.272,
		ChainAnchoredMs: 0, ChainAnchorFullMs: 9.272, ChainAnchorRemainderSeat: true,
		Summary: rspaRemainderSummary(ThreadRef{PID: 6666, Comm: "workShark"}, "D/IO blocking", 9.272, 0, 9.272),
	}}
	before := items[0].Summary
	rspaPatchSummariesForTwinVisibility(items)
	if items[0].Summary != before {
		t.Fatalf("twin patch must be a no-op on the zero-anchored form:\nbefore %q\nafter  %q", before, items[0].Summary)
	}
	if strings.Contains(items[0].Summary, "not on the published board") {
		t.Fatalf("the unpublished downgrade must never ride a zero-anchored share: %q", items[0].Summary)
	}
}
