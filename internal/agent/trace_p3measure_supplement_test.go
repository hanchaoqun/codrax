package agent

// trace_p3measure_supplement_test.go — P3MEASURE-1 model-face absence pin
// (§29.169 双不可见, 2026-07-20): the evidence-supplement lane (the 4-note
// audit window the LLM reads beside a rank row) must NEVER select a p3m_*
// silent-measurement note — neither through the pass-through allowlist nor
// through any per-family priority table. The p3m_* family is display_only
// audit wire with no model face (advisory-only red line; the tool-side
// consumer-absence pin covers the source tree, this pin covers the ONE
// runtime selection point).

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestP3MeasureNotesNeverReachSupplementFace(t *testing.T) {
	// Structural half: no allowlist prefix and no priority table entry may
	// ever admit the family.
	for _, prefix := range traceQueryObservationSupplementAllowedNotePrefixes {
		if strings.HasPrefix(prefix, "p3m_") {
			t.Fatalf("the supplement allowlist admits the silent-measurement family: %q (advisory-only red line)", prefix)
		}
	}
	// Behavioral half: a rank record carrying the full p3m_* wire renders
	// its supplement window without any of it (both languages).
	record := types.ObservationRecord{
		RichNotes: []string{
			"type=runnable_wait",
			types.TraceNoteKeyP3MCounterfactualValidMS + "=4.400",
			types.TraceNoteKeyP3MCounterfactualInvalidMS + "=6.400",
			types.TraceNoteKeyP3MEdgeWitnessedMS + "=0.800",
			types.TraceNoteKeyP3MDisposition + "=measured_segment_join",
			types.TraceNoteKeyP3MCoverage + "=families:[periodic_pinned]",
			"chain_relevance=on_chain",
			"causality=on_wakeup_chain",
		},
	}
	for _, zh := range []bool{true, false} {
		face := traceQueryObservationSupplementNotes(record, zh)
		if strings.Contains(face, "p3m_") {
			t.Fatalf("the supplement model face (zh=%v) leaked a silent-measurement note: %q", zh, face)
		}
		want := "causal position: on the proved chain"
		if zh {
			want = "因果位置：链上"
		}
		if !strings.Contains(face, want) {
			t.Fatalf("fixture: the ordinary notes must still fill the window (zh=%v): %q", zh, face)
		}
	}
}
