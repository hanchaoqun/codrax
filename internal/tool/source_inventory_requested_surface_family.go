package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

type sourceInventoryRequestedSurfaceFamily struct {
	role   types.AnswerCandidateRole
	family string
}

// sourceInventoryRequestedSurfaceFamiliesByRole intersects analyzer-validated
// SourceQuotes with construct families actually emitted by the parser graph.
// New parser adapters participate by publishing base+specific SurfaceTerms;
// this layer owns no language or construct keyword list.
func sourceInventoryRequestedSurfaceFamiliesByRole(
	ctx *types.BusContext,
	index *sourceInventoryGraphSymbolIndex,
	scopes []string,
	profile *types.SourceInventoryProfile,
) map[types.AnswerCandidateRole]map[string]bool {
	out := map[types.AnswerCandidateRole]map[string]bool{}
	if profile == nil || !profile.Active() || index == nil || len(profile.SourceQuotes) == 0 {
		return out
	}
	quotes := sourceInventoryNormalizedSurfaceQuotes(profile.SourceQuotes)
	scopeFilter := newSourceInventoryScopeFilter(ctx)
	var visibility *types.AnswerVisibilityProfile
	if ctx != nil && ctx.AnalysisIR != nil {
		visibility = ctx.AnalysisIR.RequestModel.AnswerVisibilityProfile
	}
	for _, sym := range index.all {
		if sym == nil ||
			!sourceInventoryFileInScopes(sym.File, scopes) ||
			!scopeFilter.SourceInRequestedScope(sym.File) ||
			!sourceInventorySymbolMatchesVisibility(sym, visibility) {
			continue
		}
		role, ok := aggregateAnswerCandidateRoleForSymbol(sym)
		if !ok || !profile.RequiresPrincipalRole(role) {
			continue
		}
		note := sourceInventoryCompactNote(sym.Doc)
		terms := append(sourceInventorySurfaceTermsFromGraphNote(note), sourceInventoryConstructSurfaceTerms(sym)...)
		for _, family := range types.SourceInventorySurfaceFamilyKeys(terms) {
			if !sourceInventorySurfaceFamilyRequested(quotes, family) {
				continue
			}
			if out[role] == nil {
				out[role] = map[string]bool{}
			}
			out[role][family] = true
		}
	}
	return out
}

func sourceInventoryNormalizedSurfaceQuotes(raw []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, quote := range raw {
		quote = sourceInventoryNormalizeSurfaceQuote(quote)
		if quote != "" && !seen[quote] {
			seen[quote] = true
			out = append(out, quote)
		}
	}
	return out
}

func sourceInventorySurfaceFamilyRequested(quotes []string, family string) bool {
	family = types.SourceInventorySurfaceTermKey(family)
	if family == "" {
		return false
	}
	needle := " " + family + " "
	for _, quote := range quotes {
		if strings.Contains(" "+quote+" ", needle) {
			return true
		}
	}
	return false
}
