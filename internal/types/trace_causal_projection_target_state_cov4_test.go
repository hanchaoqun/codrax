package types

// trace_causal_projection_target_state_cov4_test.go — §29.27② COV-4
// compile-side pins (ledger docs/design/real_trace_campaign_20260705.md, user
// ruling 2026-07-11): the target_window_states record attaches to the
// projection as the typed TargetStateAccount ONLY when its typed
// selected_window matches the resolved anchor window within the principal-
// value µs tolerance (禁猜 — a partition that cannot prove its window makes no window
// claim); ties resolve to the largest TotalMS (record-order independent).

import (
	"fmt"
	"math"
	"testing"
)

func cov4StateRecord(id, subject, window string, total float64) ObservationRecord {
	notes := []string{
		"running=109.270", "runnable=8.226", "sleep=28.507", "d_state=4.039",
		"io_wait=1.340", "deterministic_running=11.833", "window_ms=151.382",
	}
	if window != "" {
		notes = append(notes, window)
	}
	notes = append(notes, "total="+fmtCov4(total))
	return ObservationRecord{
		ID: id, Origin: AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
		GroundingPolicy: ClaimGroundingHard, Predicate: "target_window_states",
		ClaimKey: "target_window_states:" + subject, Subject: subject,
		Object: "state_partition", Value: fmtCov4(total), Unit: "ms",
		Span:      ObservationSpan{LineStart: 10, LineEnd: 20},
		RichNotes: notes,
	}
}

func fmtCov4(v float64) string {
	return fmt.Sprintf("%.3f", v)
}

const cov4AnchorWindow = "selected_window=100.000000..100.151382"

func TestCov4TargetStateAccountAttachAdmission(t *testing.T) {
	anchor := rn12Obs("root-1", "root_cause_primary", "root_cause_primary:x",
		"shadowhook-64305", "runnable_wait", "26.392", 26.392, 3, 9,
		"rank=1", "tier=primary", "chain_relevance=on_chain",
		"causality=on_wakeup_chain", "chain_depth=1", "dominant_state=runnable",
		cov4AnchorWindow)

	// Same-window record attaches with every typed lane.
	projection := TraceCausalProjectionFromObservationRecords([]ObservationRecord{
		anchor,
		cov4StateRecord("obs-a", "ease.app-63993", cov4AnchorWindow, 151.382),
	})
	account := projection.TargetStateAccount
	if account == nil {
		t.Fatalf("a same-window target_window_states record must attach")
	}
	if account.Subject != "ease.app-63993" || math.Abs(account.RunningMS-109.270) > 1e-9 ||
		math.Abs(account.DeterministicRunningMS-11.833) > 1e-9 ||
		math.Abs(account.IOWaitMS-1.340) > 1e-9 || account.EvidenceID != "obs-a" {
		t.Fatalf("typed lanes drifted: %+v", account)
	}
	if math.Abs(account.WindowStartTs-100.0) > 1e-9 || math.Abs(account.WindowEndTs-100.151382) > 1e-9 {
		t.Fatalf("the account must carry its typed window identity: %+v", account)
	}

	// A record whose window differs from the anchor never attaches (禁猜).
	other := TraceCausalProjectionFromObservationRecords([]ObservationRecord{
		anchor,
		cov4StateRecord("obs-b", "ease.app-63993", "selected_window=200.000000..200.151382", 151.382),
	})
	if other.TargetStateAccount != nil {
		t.Fatalf("a different-window partition must not attach: %+v", other.TargetStateAccount)
	}

	// A record with NO selected_window note never collects.
	windowless := TraceCausalProjectionFromObservationRecords([]ObservationRecord{
		anchor,
		cov4StateRecord("obs-c", "ease.app-63993", "", 151.382),
	})
	if windowless.TargetStateAccount != nil {
		t.Fatalf("a windowless partition must not attach: %+v", windowless.TargetStateAccount)
	}

	// Ties (same window re-queried): the largest TotalMS wins, in either
	// record order (deterministic, order-independent).
	small := cov4StateRecord("obs-small", "ease.app-63993", cov4AnchorWindow, 120.000)
	large := cov4StateRecord("obs-large", "ease.app-63993", cov4AnchorWindow, 151.382)
	for _, records := range [][]ObservationRecord{
		{anchor, small, large},
		{anchor, large, small},
	} {
		got := TraceCausalProjectionFromObservationRecords(records)
		if got.TargetStateAccount == nil || got.TargetStateAccount.EvidenceID != "obs-large" {
			t.Fatalf("the largest-total candidate must win independent of order: %+v", got.TargetStateAccount)
		}
	}

	// A projection with NO anchor window attaches nothing.
	anchorless := TraceCausalProjectionFromObservationRecords([]ObservationRecord{
		cov4StateRecord("obs-d", "ease.app-63993", cov4AnchorWindow, 151.382),
	})
	if anchorless.TargetStateAccount != nil {
		t.Fatalf("no anchor window → no attach (禁猜): %+v", anchorless.TargetStateAccount)
	}
}

func TestCov4TargetStateAccountDoesNotBorrowAdjacentExplorationWindow(t *testing.T) {
	anchor := rn12Obs("root-exact", "root_cause_primary", "root_cause_primary:x",
		"app-100", "runnable_wait", "8.300", 8.3, 3, 9,
		"rank=1", "tier=primary", "chain_relevance=on_chain",
		"causality=on_wakeup_chain", "chain_depth=1", "dominant_state=runnable",
		"selected_window=1.000000..1.010000")
	exact := cov4StateRecord("state-exact", "app-100", "selected_window=1.000000..1.010000", 10.000)
	adjacent := cov4StateRecord("state-adjacent", "app-100", "selected_window=1.000000..1.011000", 10.020)

	projection := TraceCausalProjectionFromObservationRecords([]ObservationRecord{anchor, adjacent, exact})
	if projection.TargetStateAccount == nil || projection.TargetStateAccount.EvidenceID != "state-exact" {
		t.Fatalf("principal value authority borrowed an adjacent exploration window: %+v", projection.TargetStateAccount)
	}
}
