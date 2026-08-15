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
	if want := "ordinary primary/secondary/tertiary election only with positive target-self or exact typed target-wait/completion attribution"; !strings.Contains(schema, want) {
		t.Fatalf("trace_query Parameters() schema Replace arm did not fire (missing %q)", want)
	}
	if banned := "tier=deterministic_optimization when on-chain, background_rank position when not"; strings.Contains(schema, banned) {
		t.Fatalf("retired trace_query schema wording resurfaced: %q", banned)
	}
}

// TestTraceQueryRootCauseClosedMatrixContractPinned keeps Description() and
// Parameters() on the same authoritative participation contract. A drift on
// either surface can teach a smaller model to republish raw wall time or a
// context-only aggregate as a ranked cause even when the engine is correct.
func TestTraceQueryRootCauseClosedMatrixContractPinned(t *testing.T) {
	surfaces := map[string]string{
		"description": (&TraceQuery{}).Description(),
		"parameters":  string((&TraceQuery{}).Parameters()),
	}
	for name, surface := range surfaces {
		for _, want := range []string{
			"authoritative closed typed effective-impact matrix",
			"runnable uses the full typed runnable duration",
			"running uses only the CAP/compute-supply deficit, and a missing or zero deficit is context_only",
			"target-thread deterministic semantic work",
			"may enter the ordinary strict positional ranking from its typed self interval",
			"A non-target semantic span intersecting a typed chain interval or preceding its host's wakeup edge is relation-only",
			"publish effective_impact_ms=0 until an exact typed target-wait or semantic-completion binding exists",
			"interval overlap or a wakeup edge alone proves neither",
			"must still be mentioned as a deterministic optimization point outside Top N",
			"off-chain semantic work is background-only",
			"periodic sources use VS-1 effective impact",
			"D-state and IO use a mutually exclusive typed sum",
			"ordinary sleep, unknown state, generic trace spans, and aggregate CPU/IO/supply pressure are context_only",
			"Only rank #1 is primary",
			"later positive contenders are secondary/tertiary",
		} {
			if !strings.Contains(surface, want) {
				t.Fatalf("trace_query %s missing closed-matrix contract fragment %q", name, want)
			}
		}
		for _, banned := range []string{
			"co-primary",
			"tier=deterministic_optimization",
			"for non-semantic rows effective_impact_ms defaults to cumulative_impact_ms",
			"non-semantic rows default effective_impact_ms to cumulative_impact_ms",
			"dominant cumulative state can still rank as the primary cause",
			"fragmented state_churn candidates",
			"to let it compete with other causes",
			"state_churn itself never takes a rank-board seat",
			"state_churn is a context-only output section",
			"context-only state_churn diagnostics",
			"the target's own runnable/running/IO/D-state rows compete normally as decomposable self causes",
		} {
			if strings.Contains(surface, banned) {
				t.Fatalf("retired trace_query %s root-cause contract resurfaced: %q", name, banned)
			}
		}
	}
	for name, wants := range map[string][]string{
		"description": {
			"Its dominant typed state maps through the same closed matrix",
			"fragmented running participates only through a positive CAP/compute-supply deficit",
			"cross-type reconciliation absorbs it so the account occupies one rank-board seat while the churn diagnostics remain published",
		},
		"parameters": {
			"state_churn diagnostics plus their closed-matrix dominant-state mapping",
			"state_churn keeps its diagnostics while its dominant state follows the closed matrix and same-account one-seat reconciliation",
		},
	} {
		for _, want := range wants {
			if !strings.Contains(surfaces[name], want) {
				t.Fatalf("trace_query %s missing state_churn participation contract fragment %q", name, want)
			}
		}
	}
}
