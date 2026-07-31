package tool

import (
	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

func (f sourceInventoryQueryFilter) Active() bool {
	return len(f.Tokens) > 0 || len(f.Languages) > 0 || len(f.SurfaceFamilies) > 0
}

func (f sourceInventoryQueryFilter) HasSurfaceFamilies() bool {
	return len(f.SurfaceFamilies) > 0
}

func sourceInventoryQueryFilterForRole(base sourceInventoryQueryFilter, families map[string]bool) sourceInventoryQueryFilter {
	if len(families) == 0 {
		return base
	}
	base.SurfaceFamilies = families
	// Parser-provided families are more precise than broad tokens such as
	// "func". Typed language selection remains active.
	base.Tokens = nil
	return base
}

func sourceInventorySymbolMatchesSurfaceFamily(sym *repotypes.Symbol, note string, families map[string]bool) bool {
	terms := append(sourceInventorySurfaceTermsFromGraphNote(note), sourceInventoryConstructSurfaceTerms(sym)...)
	family := types.SourceInventorySurfaceFamilyKey(terms)
	return family != "" && families[family]
}
