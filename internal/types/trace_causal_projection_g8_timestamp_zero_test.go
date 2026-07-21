package types

// §12-6 timestamp-zero fixture (AUDITFIX-C, §29.183 G8, 2026-07-21): a legal
// rebased [0,end] trace window must flow through the PRODUCTION compile lane
// like any other window — anchor window present, within-window markers
// minted, duration (the 占窗%/bar denominator) positive, and the two
// fallback-anchor parsers accepting the wire's "0.000000..X" shapes — while
// the (0,0) zero-value ABSENCE encoding stays excluded (negative arms). Every
// assertion here reddens when the shared predicate
// (TraceCausalProjectionWindowPresent) is mutated back to `start > 0`.

import (
	"fmt"
	"testing"
)

func g8ZeroRecord(id, predicate, claimKey, subject, object string, impact float64, span ObservationSpan, notes ...string) ObservationRecord {
	base := []string{fmt.Sprintf("impact_ms=%.3f", impact), fmt.Sprintf("cumulative_impact_ms=%.3f", impact)}
	return ObservationRecord{
		ID:              id,
		Origin:          AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		GroundingPolicy: ClaimGroundingHard,
		Predicate:       predicate,
		ClaimKey:        claimKey,
		Subject:         subject,
		Object:          object,
		Value:           fmt.Sprintf("%.3f", impact),
		Unit:            "ms",
		Confidence:      0.8,
		Span:            span,
		RichNotes:       append(base, notes...),
	}
}

// The frame-anchor lane: a frame_target_resolution record whose Span is the
// explicit [0,end] query window (the production emit shape for
// `--trace-window 0..X` on a rebased trace) must anchor the 关注窗口, mint a
// positive WindowDurationMS and stamp within-window verdicts — including on a
// node whose own span starts at exactly 0.
func TestG8TimestampZeroFrameAnchorWindow(t *testing.T) {
	got := TraceCausalProjectionFromObservationRecords([]ObservationRecord{
		{
			ID: "anchor", Origin: AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: ClaimGroundingHard, Predicate: "frame_target_resolution",
			ClaimKey: "frame_target_resolution:f", Subject: "app-1", Object: "frame",
			Span:      ObservationSpan{StartTs: 0, EndTs: 0.2},
			RichNotes: []string{"window_source=query_window"},
		},
		g8ZeroRecord("rc-zero-start", "root_cause_primary", "root_cause_primary:a", "worker-1", "running",
			7.0, ObservationSpan{LineStart: 30, LineEnd: 40, StartTs: 0, EndTs: 0.04},
			"rank=1", "tier=primary"),
		g8ZeroRecord("rc-inside", "root_cause_primary", "root_cause_primary:b", "worker-2", "sleep_wait",
			5.0, ObservationSpan{LineStart: 50, LineEnd: 60, StartTs: 0.05, EndTs: 0.12},
			"rank=2", "tier=primary"),
		g8ZeroRecord("rc-outside", "root_cause_primary", "root_cause_primary:c", "worker-3", "io_wait",
			3.0, ObservationSpan{LineStart: 70, LineEnd: 80, StartTs: 0.25, EndTs: 0.30},
			"rank=3", "tier=primary"),
		g8ZeroRecord("rc-absent", "root_cause_primary", "root_cause_primary:d", "worker-4", "runnable_wait",
			2.0, ObservationSpan{LineStart: 90, LineEnd: 95}),
	})
	if got.WindowStartTs != 0 || got.WindowEndTs != 0.2 {
		t.Fatalf("the [0,end] frame anchor must set the 关注窗口: %v..%v", got.WindowStartTs, got.WindowEndTs)
	}
	if ms := got.WindowDurationMS(); ms != 200 {
		t.Fatalf("the [0,end] window must keep its duration (占窗%%/bar denominator), got %v", ms)
	}
	within := map[string]*bool{}
	for _, node := range got.PrimaryRootCauses {
		within[node.Subject] = node.WithinRequestedWindow
	}
	if v := within["worker-1"]; v == nil || !*v {
		t.Fatalf("a node span starting at exactly ts=0 must earn within=true, got %v", within["worker-1"])
	}
	if v := within["worker-2"]; v == nil || !*v {
		t.Fatalf("an inside node must earn within=true, got %v", within["worker-2"])
	}
	if v := within["worker-3"]; v == nil || *v {
		t.Fatalf("an outside node must earn within=false, got %v", within["worker-3"])
	}
	// Negative arm: the (0,0) absence encoding never mints a verdict.
	if v := within["worker-4"]; v != nil {
		t.Fatalf("the (0,0) absence pair must stay unmarked, got %v", *v)
	}
}

// 复核修 negative arm (merge review, 2026-07-21): a line-anchored query's
// unset-start pair (0, end>0) reaches the wire under the typed
// window_source=query_window_line_anchored_unbounded_start word — the anchor
// gate must skip it (both the Span lane and the window-note lane), because on
// a non-rebased trace that pair is NOT a window, it is a half-backfilled
// sentinel (宁漏勿假).
func TestG8TimestampZeroLineAnchoredAmbiguousSourceNeverAnchors(t *testing.T) {
	got := TraceCausalProjectionFromObservationRecords([]ObservationRecord{
		{
			ID: "anchor", Origin: AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: ClaimGroundingHard, Predicate: "frame_target_resolution",
			ClaimKey: "frame_target_resolution:f", Subject: "app-1", Object: "frame",
			Span:      ObservationSpan{StartTs: 0, EndTs: 500.011},
			RichNotes: []string{"window_source=query_window_line_anchored_unbounded_start", "window=0.000000..500.011000"},
		},
		g8ZeroRecord("rc", "root_cause_primary", "root_cause_primary:r", "worker-2", "running",
			7.0, ObservationSpan{LineStart: 30, LineEnd: 40}, "rank=1", "tier=primary"),
	})
	if got.WindowStartTs != 0 || got.WindowEndTs != 0 || got.WindowDurationMS() != 0 {
		t.Fatalf("the ambiguous line-anchored (0,end) pair must anchor nothing: %v..%v", got.WindowStartTs, got.WindowEndTs)
	}
}

// The two fallback-anchor parsers accept the wire's zero-start shapes: the
// selected_window rich note ("0.000000..X") and the frame lane's `window`
// note fallback (traceQueryTypedTimeWindow emits "0.000000..X" — only the
// consumer used to reject it). The (0,0) pair stays a non-anchor.
func TestG8TimestampZeroFallbackAnchors(t *testing.T) {
	got := TraceCausalProjectionFromObservationRecords([]ObservationRecord{
		g8ZeroRecord("rc", "root_cause_primary", "root_cause_primary:r", "worker-2", "running",
			7.0, ObservationSpan{LineStart: 30, LineEnd: 40},
			"rank=1", "tier=primary", "selected_window=0.000000..0.200000"),
	})
	if got.WindowStartTs != 0 || got.WindowEndTs != 0.2 {
		t.Fatalf("selected_window=0..X must anchor (§29.183 G8): %v..%v", got.WindowStartTs, got.WindowEndTs)
	}
	got = TraceCausalProjectionFromObservationRecords([]ObservationRecord{
		{
			ID: "anchor", Origin: AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: ClaimGroundingHard, Predicate: "frame_target_resolution",
			ClaimKey: "frame_target_resolution:f", Subject: "app-1", Object: "frame",
			Span:      ObservationSpan{},
			RichNotes: []string{"window_source=query_window", "window=0.000000..0.200000"},
		},
		g8ZeroRecord("rc", "root_cause_primary", "root_cause_primary:r", "worker-2", "running",
			7.0, ObservationSpan{LineStart: 30, LineEnd: 40}, "rank=1", "tier=primary"),
	})
	if got.WindowStartTs != 0 || got.WindowEndTs != 0.2 {
		t.Fatalf("the frame lane's window=0..X note fallback must anchor: %v..%v", got.WindowStartTs, got.WindowEndTs)
	}
	// Negative arm: a frame record whose Span AND window note are both the
	// (0,0) absence shape anchors nothing.
	got = TraceCausalProjectionFromObservationRecords([]ObservationRecord{
		{
			ID: "anchor", Origin: AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: ClaimGroundingHard, Predicate: "frame_target_resolution",
			ClaimKey: "frame_target_resolution:f", Subject: "app-1", Object: "frame",
			Span:      ObservationSpan{},
			RichNotes: []string{"window_source=query_window"},
		},
		g8ZeroRecord("rc", "root_cause_primary", "root_cause_primary:r", "worker-2", "running",
			7.0, ObservationSpan{LineStart: 30, LineEnd: 40}, "rank=1", "tier=primary"),
	})
	if got.WindowStartTs != 0 || got.WindowEndTs != 0 || got.WindowDurationMS() != 0 {
		t.Fatalf("the (0,0) absence shape must anchor nothing: %v..%v", got.WindowStartTs, got.WindowEndTs)
	}
}

// The ×N merge envelope-start fold adopts a member's legal ts=0 start (0
// stopped being the accumulator's unset sentinel) while the (0,0) absence
// pair still never folds — and the pre-G8 positive behavior is untouched.
func TestG8TimestampZeroMergeEnvelopeStartFold(t *testing.T) {
	nodes := []TraceCausalProjectionNode{
		{Subject: "w", StartTs: 2, EndTs: 8},
		{Subject: "w", StartTs: 0, EndTs: 5},
		{Subject: "w"}, // (0,0) absence — must not fold an envelope start
	}
	aggregate := traceCausalProjectionMergeSameKindMembers(nodes, 0, []int{0, 1, 2})
	if aggregate.StartTs != 0 || aggregate.EndTs != 8 {
		t.Fatalf("the merged envelope must adopt the real [0,5] member start: %v..%v", aggregate.StartTs, aggregate.EndTs)
	}
	// Seed [0,end]: a later positive member must NOT overwrite the legal 0.
	nodes = []TraceCausalProjectionNode{
		{Subject: "w", StartTs: 0, EndTs: 5},
		{Subject: "w", StartTs: 2, EndTs: 8},
	}
	aggregate = traceCausalProjectionMergeSameKindMembers(nodes, 0, []int{0, 1})
	if aggregate.StartTs != 0 || aggregate.EndTs != 8 {
		t.Fatalf("a [0,end] seed start must survive the fold: %v..%v", aggregate.StartTs, aggregate.EndTs)
	}
	// Absence-only members leave the seed envelope untouched (pre-G8 arm).
	nodes = []TraceCausalProjectionNode{
		{Subject: "w", StartTs: 3, EndTs: 4},
		{Subject: "w"},
	}
	aggregate = traceCausalProjectionMergeSameKindMembers(nodes, 0, []int{0, 1})
	if aggregate.StartTs != 3 || aggregate.EndTs != 4 {
		t.Fatalf("absence members must not bend the envelope: %v..%v", aggregate.StartTs, aggregate.EndTs)
	}
}

// The WO-G2 instant-marker recognizer accepts a marker sitting at a rebased
// zero-neighbourhood timestamp (start arm only — zero-length instants at
// positive ts were always in) while the (0,0) absence pair stays out.
func TestG8TimestampZeroInstantMarkerStartArm(t *testing.T) {
	marker := TraceCausalProjectionNode{Subject: "w", Object: "wake", StartTs: 0, EndTs: 0.000001}
	if !traceCausalProjectionZeroValueMarkerRow(marker) {
		t.Fatalf("a zero-start instant marker must qualify (§29.183 G8 start arm)")
	}
	absent := TraceCausalProjectionNode{Subject: "w", Object: "wake"}
	if traceCausalProjectionZeroValueMarkerRow(absent) {
		t.Fatalf("the (0,0) absence pair must never qualify as a marker")
	}
	negative := TraceCausalProjectionNode{Subject: "w", Object: "wake", StartTs: -0.000001, EndTs: 0}
	if traceCausalProjectionZeroValueMarkerRow(negative) {
		t.Fatalf("a negative-start pair must never qualify as a marker")
	}
}
