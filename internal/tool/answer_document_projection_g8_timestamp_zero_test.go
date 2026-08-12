package tool

// §12-6 timestamp-zero fixture, render half (AUDITFIX-C, §29.183 G8,
// 2026-07-21): a legal rebased [0,end] anchor window must keep every render
// face a normal window keeps — the 分析窗 header line (no false 「起止未采集」
// wording), the bar full-scale note with the window denominator, and the ◎
// direction-subtotal eligibility of zero-start member envelopes — while the
// (0,0) absence encoding still steps every face down (negative arms). Every
// assertion reddens when the shared predicate
// (types.TraceCausalProjectionWindowPresent) is mutated back to `start > 0`.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func g8ZeroToolProjection(t *testing.T) types.TraceCausalProjection {
	t.Helper()
	records := []types.ObservationRecord{
		{
			ID: "anchor", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: types.ClaimGroundingHard, Predicate: "frame_target_resolution",
			ClaimKey: "frame_target_resolution:f", Subject: "app-1", Object: "frame",
			Span:      types.ObservationSpan{StartTs: 0, EndTs: 0.2},
			RichNotes: []string{"window_source=query_window"},
		},
		{
			ID: "rc-a", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: types.ClaimGroundingHard, Predicate: "root_cause_primary",
			ClaimKey: "root_cause_primary:a", Subject: "worker-1", Object: "sleep_wait",
			Value: "7.000", Unit: "ms", Confidence: 0.8,
			Span:      types.ObservationSpan{LineStart: 30, LineEnd: 40, StartTs: 0, EndTs: 0.05},
			RichNotes: []string{"impact_ms=7.000", "cumulative_impact_ms=7.000", "rank=1", "tier=primary"},
		},
		{
			ID: "rc-b", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: types.ClaimGroundingHard, Predicate: "root_cause_primary",
			ClaimKey: "root_cause_primary:b", Subject: "worker-2", Object: "io_wait",
			Value: "5.000", Unit: "ms", Confidence: 0.8,
			Span:      types.ObservationSpan{LineStart: 50, LineEnd: 60, StartTs: 0.1, EndTs: 0.15},
			RichNotes: []string{"impact_ms=5.000", "cumulative_impact_ms=5.000", "rank=2", "tier=primary"},
		},
	}
	set := types.CompileTraceCausalProjectionSet(types.ObservationLedger{Records: records})
	if len(set.Projections) != 1 {
		t.Fatalf("expected one projection, got %d", len(set.Projections))
	}
	return set.Projections[0]
}

// The 分析窗 header and the bar full-scale note treat [0,end] as a normal
// anchored window: real endpoints, real ms denominator, no false
// 「起止未采集」 word on either face.
func TestG8TimestampZeroWindowFaces(t *testing.T) {
	projection := g8ZeroToolProjection(t)
	if projection.WindowStartTs != 0 || projection.WindowEndTs != 0.2 {
		t.Fatalf("compile must anchor the [0,end] window: %v..%v", projection.WindowStartTs, projection.WindowEndTs)
	}
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	if model.WindowMS != 200 {
		t.Fatalf("the [0,end] window must keep its bar/percent denominator, got %v", model.WindowMS)
	}
	line := runtimeTraceProjWindowLine(projection, model, true)
	if !strings.Contains(line, "分析窗 0.000~0.200s") {
		t.Fatalf("the window header must speak the real [0,end] endpoints, got %q", line)
	}
	if strings.Contains(line, "起止未采集") {
		t.Fatalf("a captured [0,end] window must not wear the absent-window word, got %q", line)
	}
	note := runtimeTraceProjScaleNote(model, true)
	if !strings.Contains(note, "窗口200.000ms") || strings.Contains(note, "起止未采集") {
		t.Fatalf("the bar full-scale note must anchor on the window denominator, got %q", note)
	}
}

// WINFLAG-1 evolution (§29.190④, 2026-07-21): the flag-minted 0-start
// selected_window declaration (the (a) producer's new output on a true
// rebased [0,end] run) is a first-class query-window carrier through the
// projection compile — the strict parser accepts it and the PTV5
// QueryWindows display list carries the real [0,end] pair; the line-anchored
// unset form mints NO note and NO Span ts pair, so the same lanes stay
// honestly empty (the compat/absence arm rides the producer pins in
// trace_query_winflag_test.go).
func TestG8TimestampZeroWinflagSelectedWindowDeclaration(t *testing.T) {
	records := []types.ObservationRecord{
		{
			ID: "anchor", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: types.ClaimGroundingHard, Predicate: "frame_target_resolution",
			ClaimKey: "frame_target_resolution:f", Subject: "app-1", Object: "frame",
			Span:      types.ObservationSpan{StartTs: 0, EndTs: 0.2},
			RichNotes: []string{"window_source=query_window"},
		},
		{
			ID: "rc-a", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: types.ClaimGroundingHard, Predicate: "root_cause_primary",
			ClaimKey: "root_cause_primary:a", Subject: "worker-1", Object: "sleep_wait",
			Value: "7.000", Unit: "ms", Confidence: 0.8,
			Span: types.ObservationSpan{LineStart: 30, LineEnd: 40, StartTs: 0, EndTs: 0.05},
			RichNotes: []string{"impact_ms=7.000", "cumulative_impact_ms=7.000", "rank=1",
				"tier=primary", "selected_window=0.000000..0.200000"},
		},
	}
	set := types.CompileTraceCausalProjectionSet(types.ObservationLedger{Records: records})
	if len(set.Projections) != 1 {
		t.Fatalf("expected one projection, got %d", len(set.Projections))
	}
	projection := set.Projections[0]
	found := false
	for _, window := range projection.QueryWindows {
		if window.StartTs == 0 && window.EndTs == 0.2 {
			found = true
		}
	}
	if !found {
		t.Fatalf("the flag-minted [0,end] selected_window must reach the QueryWindows list: %+v", projection.QueryWindows)
	}
}

// The ◎ direction-subtotal ladder keeps a zero-start faithful envelope in the
// Σ arithmetic (L1) — and the (0,0) absence pair still steps down to L3.
func TestG8TimestampZeroElimSubtotalEligibility(t *testing.T) {
	entry := func(subject string, eff, start, end float64) runtimeTraceProjElimEntry {
		return runtimeTraceProjElimEntry{row: runtimeTraceProjTreeRow{
			HasData: true,
			Node: types.TraceCausalProjectionNode{
				Subject: subject, EffectiveImpactMS: eff,
				StartTs: start, EndTs: end,
			},
		}}
	}
	section := runtimeTraceProjElimSection{
		direction: "frequency_thermal",
		entries: []runtimeTraceProjElimEntry{
			entry("worker-1", 3.0, 0, 0.05),
			entry("worker-2", 2.0, 0.10, 0.15),
		},
	}
	arithmetic, subtotal := runtimeTraceProjElimSectionLadder(section, false, false)
	if arithmetic != elimSectionArithmeticSubtotal || subtotal != 5.0 {
		t.Fatalf("disjoint envelopes with a zero start must keep Σ eligibility, got arithmetic=%v subtotal=%v", arithmetic, subtotal)
	}
	// Negative arm: the (0,0) absence envelope steps the section down to L3.
	section.entries[0] = entry("worker-1", 3.0, 0, 0)
	arithmetic, subtotal = runtimeTraceProjElimSectionLadder(section, false, false)
	if arithmetic != elimSectionArithmeticNone || subtotal != 0 {
		t.Fatalf("the (0,0) absence envelope must still zero the arithmetic, got arithmetic=%v subtotal=%v", arithmetic, subtotal)
	}
}
