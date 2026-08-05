package tool

import (
	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// sourceInventoryProfileSurfaceSelectors collects typed analyzer carriers that
// may name a parser-owned construct family. Callers must intersect these values
// with SurfaceTerms; they are never free-form symbol filters.
func sourceInventoryProfileSurfaceSelectors(ctx *types.BusContext, profile *types.SourceInventoryProfile) []string {
	if profile == nil || !profile.Active() {
		return nil
	}
	selectors := append([]string(nil), profile.SourceQuotes...)
	if ctx != nil && ctx.AnalysisIR != nil {
		selectors = append(selectors, ctx.AnalysisIR.RequestModel.AnalyzerHints.Entities...)
	}
	return sourceInventoryNormalizedSurfaceQuotes(selectors)
}

func sourceInventorySymbolMatchesRequestedSurfaceSelectors(sym *repotypes.Symbol, graph *repotypes.Graph, selectors []string) bool {
	if sym == nil || len(selectors) == 0 {
		return false
	}
	_, terms := sourceInventoryCandidateNoteAndSurfaceTermsFromGraph(sym, sourceInventoryGraphLanguageForFile(graph, sym.File))
	for _, family := range types.SourceInventorySurfaceFamilyKeys(terms) {
		if sourceInventorySurfaceFamilyRequested(selectors, family) {
			return true
		}
	}
	return false
}
