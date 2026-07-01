package tool

import repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"

func sourceInventorySymbolMatchesQuery(sym *repotypes.Symbol, graph *repotypes.Graph, filter sourceInventoryQueryFilter) bool {
	if sym == nil || !filter.Active() {
		return false
	}
	language := sourceInventoryGraphLanguageForFile(graph, sym.File)
	if !sourceInventoryQueryLanguageMatches(language, filter) {
		return false
	}
	note := sourceInventoryCompactNote(sym.Doc)
	parts := []string{
		sym.Name,
		sym.Kind,
		note,
		sym.Parent,
		sym.Receiver,
		sym.Signature,
	}
	parts = append(parts, sourceInventorySurfaceTermsFromGraphNote(note)...)
	parts = append(parts, sourceInventoryConstructSurfaceTerms(sym)...)
	if graph != nil && graph.FileIndex != nil {
		if fi := graph.FileIndex[sym.File]; fi != nil {
			parts = append(parts, fi.Package)
		}
	}
	if len(filter.Tokens) == 0 {
		return true
	}
	return sourceInventoryAnyQueryTokenMatches(parts, filter.Tokens)
}
