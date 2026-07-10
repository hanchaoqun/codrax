package tool

import (
	"strings"
	"testing"
)

// TestTraceQueryDescriptionReplaceArmsAllFire is the audit #70 anti-drift pin
// (campaign ledger §29.26 审计总收账, 2026-07-11).
//
// Description()/Parameters() are assembled by chaining strings.Replace over
// one large base literal with VERBATIM old-string arguments. If a future edit
// rewords the base literal, an arm's old-string silently stops matching, the
// Replace no-ops, tests stay green, and the LLM keeps being taught retired
// semantics — exactly the snapshot-without-recompile failure shape recorded
// as a prior campaign lesson. The semantic span-work arms already have pins
// (TestDCSTraceQueryDescriptionSpeaksOnChainElectionAndOffChainBackground);
// this test covers every previously-unpinned arm: each pins one distinctive
// fragment of its NEW wording (presence proves the arm fired), and fragments
// that are fully replaced by an arm are banned so retired teaching cannot
// resurface.
func TestTraceQueryDescriptionReplaceArmsAllFire(t *testing.T) {
	description := (&TraceQuery{}).Description()
	for _, want := range []string{
		// tracebundle caveat-expansion arm.
		"role=resolver_index means the DB table was consumed for joins/indexes",
		"tracebundle_trace_db_coverage",
		// provenance-aware composite arm (previously unpinned).
		"builds a provenance-aware .systrace+.perftrace composite",
		"resolves to a physical source artifact, local line, and time domain",
		// state_drilldown extension arm (previously unpinned).
		"The state_drilldown rows are the state-first handoff",
		"window_proportion",
		// selected_window / follow-up-window extension arm (previously
		// unpinned).
		"mode=index_event_limit",
		"sub-50ms windows are micro-probes",
		// G/H/N/I track-marker arm.
		"Trace markers include B/E/C/S/F/G/H/N/I rows",
		"trace_track_spans",
	} {
		if !strings.Contains(description, want) {
			t.Fatalf("trace_query description Replace arm did not fire (missing %q) — the base literal drifted out from under a verbatim old-string; realign the Replace arm or fold it into the base literal", want)
		}
	}
	for _, banned := range []string{
		// Fully replaced by the provenance-aware composite arm: the
		// unconditional-merge teaching is retired (unproven sibling perf
		// clocks are isolated, ledger-only visibility).
		"merges sibling .systrace+.perftrace pairs",
		// Fully replaced by the G/H/N/I track-marker arm.
		"Trace markers include B/E/C/S/F rows:",
	} {
		if strings.Contains(description, banned) {
			t.Fatalf("retired trace_query description wording resurfaced: %q — a Replace arm no longer matches its old-string", banned)
		}
	}

	schema := string((&TraceQuery{}).Parameters())
	if want := "texture upload, and explicit GC pauses (ordinary primary/secondary/tertiary election when on-chain; background_rank only when off-chain)"; !strings.Contains(schema, want) {
		t.Fatalf("trace_query Parameters() schema Replace arm did not fire (missing %q)", want)
	}
	if banned := "tier=deterministic_optimization when on-chain, background_rank position when not"; strings.Contains(schema, banned) {
		t.Fatalf("retired trace_query schema wording resurfaced: %q", banned)
	}
}
