package orchestrator

import "github.com/hanchaoqun/codrax/internal/types"

// copyDegradedRequiredFileLanes carries the analyzer's file-relevance lanes
// into the degraded-recovery IR: the L3 / L4 typed channels
// (RequiredFileHints / IrrelevantFiles, 2026-05-10 P2 audit follow-up) and
// the V4-4 (§40.22) unresolved-owner marker that describes those hints.
// These signals were emitted by the LLM and survived the emit-time decoder,
// so they are trustworthy independent of whatever made a later gate reject;
// dropping them would force the explorer back to the deterministic resolver
// right after the LLM made its strongest file-relevance judgement, and
// dropping the marker would silently hide the ownership gap from the
// explorer guide. Slices and the marker are copied, never aliased.
func copyDegradedRequiredFileLanes(dst *types.AnalyzerHints, src types.AnalyzerHints) {
	dst.RequiredFileHints = append([]types.RequiredFileHint(nil), src.RequiredFileHints...)
	dst.DimensionOwnerUnresolved = src.DimensionOwnerUnresolved.Clone()
	dst.IrrelevantFiles = dedupedStrings(src.IrrelevantFiles)
}
