package tool

import (
	"github.com/hanchaoqun/codrax/internal/types"
)

// Narrow only the tool's private derivation input, after checking the intact
// parent receipt. Every raw-text derivation uses these same validated bytes.
func perfTraceExtractionContext(parent *types.BusContext) (*types.BusContext, *types.PerfObservationSourceScope, error) {
	if parent.AttachedTraceExcerpt == nil {
		return parent, nil, nil
	}
	raw, scope, err := parent.AttachedTraceExcerpt.Resolve(parent.Ctx, parent.AttachedHitrace, parent.AttachedTraceMaterial)
	if err != nil {
		return nil, nil, err
	}
	view := parent.ShallowClone()
	view.AttachedHitrace = raw
	return view, &scope, nil
}

func scopePerfObservations(bundle *types.PerfBundle, scope types.PerfObservationSourceScope) {
	localLines := scope.LineEnd - scope.LineStart + 1
	for i := range bundle.Observations {
		obs := &bundle.Observations[i]
		owned := scope
		obs.SourceScope = &owned
		// Out-of-view model locators cannot become parent-preview coordinates.
		if obs.LineStart < 1 || obs.LineStart > localLines || obs.LineEnd < 0 || obs.LineEnd > localLines || obs.LineEnd > 0 && obs.LineEnd < obs.LineStart {
			obs.LineStart, obs.LineEnd = 0, 0
			continue
		}
		obs.LineStart += scope.LineStart - 1
		if obs.LineEnd > 0 {
			obs.LineEnd += scope.LineStart - 1
		}
	}
}
