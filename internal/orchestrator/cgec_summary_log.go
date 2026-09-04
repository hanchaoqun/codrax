package orchestrator

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/logging"
)

// cgec_summary_log.go — the end-of-task "[CGEC] summary" operator
// telemetry line (moved out of orchestrator.go in §40.52 when the IR
// delivery ratchet tripped; concern file, no behaviour change).

// emitCGECSummary renders the per-task CGEC counter snapshot to the
// operator trace. Always emits a single line so operators can grep
// [CGEC] summary even on no-op tasks — a "no enforcer fired" line
// is a positive signal that the closure is quiet, which is itself
// diagnostic information. Called at the end of runTaskGraph after
// all stages have exited.
//
// This line is log-only: it carries internal counter names that are
// noise to end users, and the renderer already surfaces task
// completion via stage-end events. Operator-facing only.
func (o *Orchestrator) emitCGECSummary() {
	if o.busCtx == nil || o.busCtx.Mutable == nil {
		return
	}
	closure := o.busCtx.Mutable.EvidenceClosure()
	stats := closure.Stats()
	// CGEC frequency bridge: high-frequency CGEC events get a
	// SOFT violation entry so operators see the storm in the
	// closure ledger / by_field tally, not just the raw counter
	// in the [CGEC] summary INFO line. Telemetry-only (Severity=
	// Soft per types.DeriveSeverity); never blocks shipping.
	emitCGECStormViolations(closure, stats)
	// The "[CGEC] summary: " prefix is passed straight to the logger
	// (§40.52: the literal is operator telemetry, and the glossary lint
	// excludes only direct logger arguments); the body below is
	// byte-identical to the pre-§40.52 line for log parsers.
	var line string
	if !stats.HasActivity() {
		line = "no enforcer fired (contract quiet)"
	} else {
		line = fmt.Sprintf(
			"chains_demoted=%d unverified=%d repairs_raised=%d expand_search=%d shape_swap=%d pre_complete_downgrades=%d forced_reads=%d stall_soft=%d stall_hard=%d",
			stats.ChainsDemoted, stats.UnverifiedFinds, stats.RepairsRaised,
			stats.ExpandSearchRaised, stats.ViewSwapRaised,
			stats.PreCompleteDowngrades, stats.ForcedReads,
			stats.StallSoftHits, stats.StallHardHits)
		// Session 11 F1: extended summary with ViolationLedger view.
		// Keep the extension tail-appended so existing log parsers
		// that match the pre-session-11 prefix still work. The tail
		// prints nothing when the ledger is empty (zero cost when
		// F1 hookups are a no-op on a healthy run).
		if suffix := formatCGECViolationLedgerSummary(closure, stats); suffix != "" {
			line = line + suffix
		}
	}
	// Phase 5 (Semantic Surface Contract, 2026-05-02) — richness
	// telemetry tail. Prints optional_facets_covered=N/M when the
	// active question had any FacetOptional / TierEnrichment entries.
	// Always tail-appended so legacy log parsers stay byte-stable
	// on Runs without optional facets.
	if covered, total := o.computeRichnessCoverage(); total > 0 {
		line = fmt.Sprintf("%s optional_facets_covered=%d/%d", line, covered, total)
	}
	logging.Info("[CGEC] summary: %s", line)

	// Block 1 (architecture overhaul 2026-05-02) — stage-wise
	// breakdown line. Distinct INFO record so existing log parsers
	// that grep the [CGEC] summary: prefix do not parse the
	// stage-wise data accidentally. Empty (no stage activity) prints
	// nothing — the byte-identical pre-2026-05-02 path stays clean
	// for healthy Runs.
	if snapshot := closure.StageHealthSnapshot(); len(snapshot) > 0 {
		stageLine := formatStageHealthSnapshot(snapshot)
		if stageLine != "" {
			logging.Info("[CGEC] perStage: %s", stageLine)
		}
	}

	// B5-F2 / B5-F3 richness telemetry surface. Each signal
	// renders a single WARN line so operators can grep the silent
	// rule firings (facet softening / family underrepresentation).
	// Gated by pipeline_richness_softening_warn (default true);
	// telemetry is always recorded regardless of the WARN gate.
	if richnessSofteningWarnEnabled() {
		for _, sig := range o.busCtx.Mutable.RichnessTelemetry() {
			logging.Warning("[richness] %s family=%s facet_id=%s facet_kind=%s buckets=%d reason=%s",
				sig.Kind, sig.Family, sig.FacetID, sig.FacetKind, sig.BucketCount, sig.Reason)
		}
	}

	// EVALFIX-2E (CLASS 5): "[degrade] ledger:" operator aggregate —
	// concern file degradation_ledger_log.go.
	emitDegradationLedgerSummary(o.busCtx.Mutable)
}
