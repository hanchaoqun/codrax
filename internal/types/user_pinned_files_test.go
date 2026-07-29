package types

import "testing"

// PIB-5c (ledger docs/design/pi_borrow_analysis_20260729.md §7.5):
// user @path pins are a precise deterministic signal — the forced-read
// coverage predicate treats pinned requests as hard current-source
// obligations.

func TestRequiredFileHintCoverage_UserPinnedFilesArm(t *testing.T) {
	rm := RequestModel{
		UserPinnedFiles: []string{"internal/repl/input.go"},
		AnalyzerHints: AnalyzerHints{RequiredFileHints: []RequiredFileHint{
			{Path: "internal/repl/input.go", Confidence: 1.0},
		}},
	}
	if !RequiredFileHintCurrentSourceCoverageApplies(rm) {
		t.Fatal("a user-pinned request must apply forced-read coverage")
	}

	// Without pins, a plain request with hints keeps the existing
	// (narrower) behavior — this arm must not widen other shapes.
	rm.UserPinnedFiles = nil
	if RequiredFileHintCurrentSourceCoverageApplies(rm) {
		t.Fatal("un-pinned plain request must keep the pre-existing narrow gate")
	}

	// The explicit user source-boundary exclusion outranks the pin
	// arm by clause order: an operator who said "don't read current
	// source" keeps that boundary even with a stray @token.
	rm.UserPinnedFiles = []string{"main.go"}
	rm.ExternalObservationPolicy = &ExternalObservationPolicy{
		CurrentSourceMode: ExternalObservationCurrentSourceExclude,
		ExclusionKind:     ExternalObservationSourceExclusionExplicitUserBoundary,
		SourceQuotes:      []string{"只看日志，不要读代码"},
	}
	if RequiredFileHintCurrentSourceCoverageApplies(rm) {
		t.Fatal("explicit source-exclusion boundary must outrank the pin arm")
	}
}
