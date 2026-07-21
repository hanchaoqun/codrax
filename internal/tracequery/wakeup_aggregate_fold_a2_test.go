package tracequery

// wakeup_aggregate_fold_a2_test.go — A2 件5② engine arm (§29.179 A 批委托,
// 2026-07-21): the aggregate top-8 trim fold carries the MAX member's label +
// dominant state (MaxSubject/MaxStateKind — the folded_max_subject /
// folded_max_state_kind wire carriers behind the RUN2FIX-A 件2 max-member
// disclosure). All-or-nothing: an unlabeled max member clears both.
//
// MUTATION self-check: dropping the MaxSubject capture at the max update reds
// TestA2AggregateFoldCarriesMaxMemberIdentity.

import "testing"

func TestA2AggregateFoldCarriesMaxMemberIdentity(t *testing.T) {
	fold := foldWakeupCausalAggregateOverflow([]WakeupCausalAggregate{
		{Thread: ThreadRef{PID: 11, Comm: "small"}, DominantState: string(StateRunnable), DominantImpactMs: 1.5, LineStart: 10, LineEnd: 20},
		{Thread: ThreadRef{PID: 12, Comm: "big"}, DominantState: string(StateSSleep), DominantImpactMs: 47.282, LineStart: 30, LineEnd: 40},
		{Thread: ThreadRef{PID: 13, Comm: "mid"}, DominantState: string(StateRunning), DominantImpactMs: 9.0, LineStart: 50, LineEnd: 60},
	})
	if fold == nil {
		t.Fatalf("fold must mint on overflow")
	}
	if fold.MaxSubject != "big-12" || fold.MaxStateKind != string(StateSSleep) {
		t.Fatalf("件5②: the fold must carry the max member's identity, got %q/%q", fold.MaxSubject, fold.MaxStateKind)
	}
	// 负臂: an unlabeled max member clears both (宁漏勿假).
	unlabeled := foldWakeupCausalAggregateOverflow([]WakeupCausalAggregate{
		{Thread: ThreadRef{PID: 11, Comm: "small"}, DominantState: string(StateRunnable), DominantImpactMs: 1.5},
		{Thread: ThreadRef{}, DominantState: string(StateSSleep), DominantImpactMs: 47.282},
	})
	if unlabeled == nil {
		t.Fatalf("fold must mint on overflow")
	}
	if unlabeled.MaxSubject != "" || unlabeled.MaxStateKind != "" {
		t.Fatalf("件5② 负臂: an unlabeled max member must clear both carriers, got %q/%q",
			unlabeled.MaxSubject, unlabeled.MaxStateKind)
	}
}
