package types

// trace_causal_projection_valueless_fold_g12_test.go — G12-ENG 修根 pins
// (§29.1, real_trace_campaign_20260705.md, 2026-07-09).
//
// 勘察结论 (compile-level reproduction below, byte-matching the huadong_79_01
// E23 witness): the suspected "同段双归属" NEVER existed in the engine. E23 was
// the on-chain overflow fold of
//   (a) the target's ×5 R2 aggregate of five REAL disjoint binder waits
//       (1.035+2.215+2.918+3.527+4.577 = 14.272ms — correct single
//       attribution), and
//   (b) hmfs_discard's ×4 R2 aggregate of four ZERO-duration sched_
//       blocked_reason marker rows (the S+iowait platform form: the thread's
//       IO wait is an S-state sleep + iowait=1 marker, so no D interval and
//       no duration exists — conf 0.82, the fold's min confidence).
// The fold's min–max has always ranged over POSITIVE member displays only
// while MergedCount counted every member, so the render claimed
// "2线程取最大(单项14.272~14.272ms)" — fabricating a second 14.272ms observation
// under hmfs_discard's subject and triggering the customer-site raw-trace
// audit (g12_report.txt: zero prev_state=D over the whole region).
//
// MUTATION self-checks:
//   - dropping the valueless accounting in traceCausalProjectionOverflowFoldRow
//     (valuelessRows never incremented) reds
//     TestG12OverflowFoldE23AssemblyReproduction (MergedValuelessCount==0);
//   - dropping the R2-merge accounting reds
//     TestG12SameKindMergeCountsValuelessMembers.

import (
	"fmt"
	"testing"
)

// g12BinderWaitRecords models the five real binder waits of the target from
// the huadong_79_01 wire dump (verbatim values and line coordinates): each
// wait published a rank-lane copy (root_cause_target_self_state -> binder_wait)
// and a root_evidence audit copy sharing subject+value+lines, which the R1
// same-fact merge folds per wait before the R2 ×5 aggregate forms.
func g12BinderWaitRecords() []ObservationRecord {
	waits := []struct {
		ms      float64
		lineEnd int
	}{
		{1.035, 1029665},
		{2.215, 1038035},
		{2.918, 1043697},
		{3.527, 1029159},
		{4.577, 1031705},
	}
	var out []ObservationRecord
	for i, w := range waits {
		out = append(out, ObservationRecord{
			ID:              fmt.Sprintf("trace_query:q5#root_cause:%d", i+1),
			Producer:        "trace_query",
			Origin:          AnswerEvidenceOriginRuntimeArtifact,
			GroundingPolicy: ClaimGroundingHard,
			SourceRef:       ObservationSourceRef{Path: "berlin.systrace"},
			ClaimKey:        "root_cause_target_self_state:binder_wait",
			Subject:         "oney.hmn.berlin-42591",
			Predicate:       "root_cause_target_self_state",
			Object:          "binder_wait",
			Value:           fmt.Sprintf("%.3f", w.ms),
			Unit:            "ms",
			Span:            ObservationSpan{LineStart: 1017021, LineEnd: w.lineEnd},
			SupportRefs:     []string{fmt.Sprintf("berlin.systrace#L1017021-L%d", w.lineEnd)},
			RichNotes: []string{
				fmt.Sprintf("cumulative_impact_ms=%.3f", w.ms),
				"source=wakeup_chain",
				"causality=on_wakeup_chain",
				"chain_relevance=on_chain",
				"tier=target_self_state",
			},
			Confidence: 0.86,
		})
		out = append(out, ObservationRecord{
			ID:              fmt.Sprintf("trace_query:q5#root_evidence:%d", i+1),
			Producer:        "trace_query",
			Origin:          AnswerEvidenceOriginRuntimeArtifact,
			GroundingPolicy: ClaimGroundingHard,
			SourceRef:       ObservationSourceRef{Path: "berlin.systrace"},
			ClaimKey:        "root_evidence:binder_wait",
			Subject:         "oney.hmn.berlin-42591",
			Predicate:       "binder_wait",
			Value:           fmt.Sprintf("%.3f", w.ms),
			Unit:            "ms",
			Span:            ObservationSpan{LineStart: 1017021, LineEnd: w.lineEnd},
			SupportRefs:     []string{fmt.Sprintf("berlin.systrace#L1017021-L%d", w.lineEnd)},
			Confidence:      0.86,
		})
	}
	return out
}

// g12BlockedReasonRecords models hmfs_discard's four ZERO-duration
// sched_blocked_reason critical_blocking marker rows (the S+iowait platform
// form leaves no measurable interval; conf 0.82 is the blocked_reason lane's
// verbatim confidence and E23's published min).
func g12BlockedReasonRecords() []ObservationRecord {
	lines := []int{1484314, 1490697, 1502018, 1625582}
	var out []ObservationRecord
	for i, ln := range lines {
		out = append(out, ObservationRecord{
			ID:              fmt.Sprintf("trace_query:q4#critical_blocking:%d", i+1),
			Producer:        "trace_query",
			Origin:          AnswerEvidenceOriginRuntimeArtifact,
			GroundingPolicy: ClaimGroundingHard,
			SourceRef:       ObservationSourceRef{Path: "berlin.systrace"},
			ClaimKey:        "critical_blocking:blocked_reason",
			Subject:         "hmfs_discard-26-562",
			Predicate:       "critical_blocking",
			Object:          "blocked_reason",
			Value:           "",
			Unit:            "ms",
			Span:            ObservationSpan{LineStart: ln, LineEnd: ln},
			SupportRefs:     []string{fmt.Sprintf("berlin.systrace#L%d", ln)},
			RichNotes: []string{
				"type=blocked_reason",
				"chain_relevance=on_chain",
			},
			Confidence: 0.82,
		})
	}
	return out
}

// g12FillerRecords occupies the on-chain roster cap so the two aggregates
// overflow into the fold row (the huadong tree rendered its cap-full on-chain
// board above E23 the same way).
func g12FillerRecords(n int) []ObservationRecord {
	var out []ObservationRecord
	for i := 0; i < n; i++ {
		out = append(out, ObservationRecord{
			ID:              fmt.Sprintf("trace_query:q4#filler:%d", i+1),
			Producer:        "trace_query",
			Origin:          AnswerEvidenceOriginRuntimeArtifact,
			GroundingPolicy: ClaimGroundingHard,
			SourceRef:       ObservationSourceRef{Path: "berlin.systrace"},
			ClaimKey:        "critical_blocking:io_latency",
			Subject:         fmt.Sprintf("filler-thread-%d", i+1),
			Predicate:       "critical_blocking",
			Object:          fmt.Sprintf("peer-%d", i+1),
			Value:           fmt.Sprintf("%.3f", 20.0+float64(i)),
			Unit:            "ms",
			Span:            ObservationSpan{LineStart: 2000000 + i*10, LineEnd: 2000005 + i*10},
			SupportRefs:     []string{fmt.Sprintf("berlin.systrace#L%d", 2000000+i*10)},
			RichNotes: []string{
				"type=io_latency",
				"chain_relevance=on_chain",
			},
			Confidence: 0.86,
		})
	}
	return out
}

// TestG12OverflowFoldE23AssemblyReproduction reproduces the huadong_79_01 E23
// witness end-to-end at the compile level and pins the corrected typed
// accounting. Every legacy field must match the production report byte-form
// (merged_count=2, min=max=14.272, both subjects, conf 0.82, lines
// 1017021–1625582) — the fix adds ONLY the valueless disclosure; no published
// value moves (保守方向: nothing increases, the fabricated second 14.272
// observation disappears from the render, not from the roster).
func TestG12OverflowFoldE23AssemblyReproduction(t *testing.T) {
	records := append(g12BinderWaitRecords(), g12BlockedReasonRecords()...)
	records = append(records, g12FillerRecords(traceCausalProjectionOnChainLimit)...)
	p := TraceCausalProjectionFromObservationRecords(records)
	if len(p.OnChainCauses) != traceCausalProjectionOnChainLimit+1 {
		t.Fatalf("expected cap-full on-chain bucket + fold, got %d rows", len(p.OnChainCauses))
	}
	fold := p.OnChainCauses[len(p.OnChainCauses)-1]
	if !fold.OnChainOverflowFold {
		t.Fatalf("last on-chain row must be the overflow fold: %+v", fold)
	}
	if fold.MergedCount != 2 {
		t.Fatalf("E23 form: two folded members (×5 binder aggregate + ×4 zero blocked_reason aggregate), got %d", fold.MergedCount)
	}
	// The ×5 aggregate SUMs in float order — compare inside the µs tie band
	// (the same strictness DIAG-A1 uses).
	near := func(v float64) bool { return v > 14.272-0.0005 && v < 14.272+0.0005 }
	if !near(fold.MergedMinMS) || !near(fold.MergedMaxMS) {
		t.Fatalf("the valued range stays the single real member value 14.272: min=%.6f max=%.6f", fold.MergedMinMS, fold.MergedMaxMS)
	}
	if !near(fold.ImpactMS) {
		t.Fatalf("the published fold value stays the member MAX 14.272: %.6f", fold.ImpactMS)
	}
	if len(fold.MergedSubjects) != 2 {
		t.Fatalf("both member subjects stay on the roster (lossless): %v", fold.MergedSubjects)
	}
	if fold.Confidence != 0.82 {
		t.Fatalf("fold min-confidence stays the blocked_reason lane's 0.82: %.2f", fold.Confidence)
	}
	// THE FIX (修前红): the zero-duration member is typed-counted, so the
	// render can stop claiming "×2 each 14.272~14.272ms" — the fabricated
	// same-segment double that sent the customer to a raw-trace audit.
	if fold.MergedValuelessCount != 1 {
		t.Fatalf("the ×4 zero-duration blocked_reason aggregate must count as ONE valueless member, got %d", fold.MergedValuelessCount)
	}
	// The DIAG-A1 tie roster must NOT name the valueless member — only rows
	// that actually carry the max value are same-value witnesses.
	for _, member := range fold.SameValueMembers {
		if member.Subject == "hmfs_discard-26-562" {
			t.Fatalf("a zero-duration member must never appear as a same-value witness: %+v", fold.SameValueMembers)
		}
	}
}

// TestG12OverflowFoldValuelessAccounting pins the constructor arithmetic on
// the three member shapes: ordinary zero member, absorbed fold member
// (F1 计数吸收 twin), and the all-valued 负向 control.
func TestG12OverflowFoldValuelessAccounting(t *testing.T) {
	valued := TraceCausalProjectionNode{Subject: "a-1", ImpactMS: 14.272, CumulativeImpactMS: 14.272, EvidenceID: "e1", Confidence: 0.86, LineStart: 10, LineEnd: 20}
	zero := TraceCausalProjectionNode{Subject: "b-2", EvidenceID: "e2", Confidence: 0.82, LineStart: 30, LineEnd: 40}
	fold := traceCausalProjectionOverflowFoldRow([]TraceCausalProjectionNode{valued, zero})
	if fold.MergedCount != 2 || fold.MergedValuelessCount != 1 {
		t.Fatalf("mixed fold: count=2 valueless=1, got count=%d valueless=%d", fold.MergedCount, fold.MergedValuelessCount)
	}
	if fold.MergedMinMS != 14.272 || fold.MergedMaxMS != 14.272 {
		t.Fatalf("range stays over the valued member only: min=%.3f max=%.3f", fold.MergedMinMS, fold.MergedMaxMS)
	}

	// F1 计数吸收 twin: an absorbed fold member contributes its OWN row and
	// valueless counts.
	inner := traceCausalProjectionOverflowFoldRow([]TraceCausalProjectionNode{valued, zero})
	outer := traceCausalProjectionOverflowFoldRow([]TraceCausalProjectionNode{
		{Subject: "c-3", ImpactMS: 5, CumulativeImpactMS: 5, EvidenceID: "e3", Confidence: 0.8, LineStart: 50, LineEnd: 60},
		inner,
	})
	if outer.MergedCount != 3 || outer.MergedValuelessCount != 1 {
		t.Fatalf("fold-of-fold absorption: rows=3 valueless=1, got rows=%d valueless=%d", outer.MergedCount, outer.MergedValuelessCount)
	}

	// 负向: all-valued folds carry zero valueless members (legacy render
	// byte-identity is pinned on the tool side).
	allValued := traceCausalProjectionOverflowFoldRow([]TraceCausalProjectionNode{
		valued,
		{Subject: "d-4", ImpactMS: 3.5, CumulativeImpactMS: 3.5, EvidenceID: "e4", Confidence: 0.8, LineStart: 70, LineEnd: 80},
	})
	if allValued.MergedValuelessCount != 0 {
		t.Fatalf("all-valued fold must not count valueless members: %d", allValued.MergedValuelessCount)
	}
}

// TestG12SameValueTieMicroValueBandGuard (复核 P2-3): the projection tie
// collector's v<=0 guard has ONE load-bearing corner — the micro-value band
// (max=0.0003 < the 0.0005 tie band), where a zero-value member sits
// |0−max| inside the band and would enter the roster as a fabricated tie
// witness. The guard keeps it out; the two real 0.0003 members disclose.
// Deleting the v<=0 guard reds this pin (mutation-verified).
func TestG12SameValueTieMicroValueBandGuard(t *testing.T) {
	members := []TraceCausalProjectionNode{
		{Subject: "micro-a", ImpactMS: 0.0003, LineStart: 10, LineEnd: 11},
		{Subject: "micro-b", ImpactMS: 0.0003, LineStart: 20, LineEnd: 21},
		{Subject: "zero-c", LineStart: 30, LineEnd: 31},
	}
	ties := traceCausalProjectionSameValueFoldMembers(members, 0.0003)
	if len(ties) != 2 {
		t.Fatalf("exactly the two real 0.0003 members tie, got %+v", ties)
	}
	for _, tie := range ties {
		if tie.Subject == "zero-c" {
			t.Fatalf("a zero-value member must never enter the micro-band roster: %+v", ties)
		}
	}
}

// TestG12SameKindMergeCountsValuelessMembers pins the R2 ×N constructor's
// twin accounting: a mixed same-(subject,object) group counts its
// non-positive-display members; an all-valued group counts zero.
func TestG12SameKindMergeCountsValuelessMembers(t *testing.T) {
	nodes := []TraceCausalProjectionNode{
		{Subject: "w-1", Object: "blocked_reason", ImpactMS: 2.0, CumulativeImpactMS: 2.0, EvidenceID: "m1"},
		{Subject: "w-1", Object: "blocked_reason", EvidenceID: "m2"},
		{Subject: "w-1", Object: "blocked_reason", ImpactMS: 3.0, CumulativeImpactMS: 3.0, EvidenceID: "m3"},
	}
	merged := traceCausalProjectionMergeSameKindMembers(nodes, 0, []int{0, 1, 2})
	if merged.MergedCount != 3 || merged.MergedValuelessCount != 1 {
		t.Fatalf("mixed R2 group: count=3 valueless=1, got count=%d valueless=%d", merged.MergedCount, merged.MergedValuelessCount)
	}
	if merged.MergedMinMS != 2.0 || merged.MergedMaxMS != 3.0 {
		t.Fatalf("range stays over positive displays: min=%.3f max=%.3f", merged.MergedMinMS, merged.MergedMaxMS)
	}
	allValued := traceCausalProjectionMergeSameKindMembers([]TraceCausalProjectionNode{
		{Subject: "w-1", Object: "io", ImpactMS: 1, CumulativeImpactMS: 1, EvidenceID: "m4"},
		{Subject: "w-1", Object: "io", ImpactMS: 2, CumulativeImpactMS: 2, EvidenceID: "m5"},
		{Subject: "w-1", Object: "io", ImpactMS: 3, CumulativeImpactMS: 3, EvidenceID: "m6"},
	}, 0, []int{0, 1, 2})
	if allValued.MergedValuelessCount != 0 {
		t.Fatalf("all-valued R2 group must not count valueless members: %d", allValued.MergedValuelessCount)
	}
}
