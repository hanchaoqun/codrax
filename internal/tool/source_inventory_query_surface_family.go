package tool

import (
	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

func (f sourceInventoryQueryFilter) Active() bool {
	return len(f.Tokens) > 0 || len(f.Languages) > 0 || f.RequireSurfaceFamilies || len(f.SurfaceFamilies) > 0
}

func (f sourceInventoryQueryFilter) HasSurfaceFamilies() bool {
	return f.RequireSurfaceFamilies || len(f.SurfaceFamilies) > 0
}

func sourceInventoryQueryFilterForRole(base sourceInventoryQueryFilter, families map[string]bool, requireSurfaceFamilies bool) sourceInventoryQueryFilter {
	if !requireSurfaceFamilies {
		return base
	}
	base.SurfaceFamilies = families
	base.RequireSurfaceFamilies = true
	// Parser-provided families are more precise than broad tokens such as
	// "func". Typed language selection remains active.
	base.Tokens = nil
	return base
}

func sourceInventorySymbolMatchesSurfaceFamily(sym *repotypes.Symbol, note string, families map[string]bool) bool {
	terms := append(sourceInventorySurfaceTermsFromGraphNote(note), sourceInventoryConstructSurfaceTerms(sym)...)
	for _, family := range types.SourceInventorySurfaceFamilyKeys(terms) {
		if families[family] {
			return true
		}
	}
	return false
}
