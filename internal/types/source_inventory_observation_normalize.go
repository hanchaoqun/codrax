package types

func normalizeSourceInventoryObservation(in SourceInventoryObservation) SourceInventoryObservation {
	in.SourceClasses = normalizeSourceInventorySourceClassCounts(in.SourceClasses)
	if len(in.Sets) == 0 && len(in.SourceClasses) == 0 {
		if in.LensExecutedEmpty() {
			// §29.122 LENSBURN 病B: the executed-empty lens carrier is a typed
			// execution fact, not row content. Collapsing it to the zero
			// observation here is exactly what made an empty shelf structurally
			// indistinguishable from "lens never ran"; preserve the carrier
			// (Active + execution state + provenance/scopes) with no row-level
			// fields to normalize.
			return SourceInventoryObservation{
				Active:          true,
				Complete:        in.Complete,
				Scopes:          in.Scopes,
				QueryPathScopes: in.QueryPathScopes,
				Provenance:      in.Provenance,
				Lens:            in.Lens,
				Execution:       in.Execution,
			}
		}
		return SourceInventoryObservation{}
	}
	in.Active = true
	for i := range in.Sets {
		in.Sets[i].Count = len(in.Sets[i].Members)
		if in.Sets[i].Total < in.Sets[i].Count {
			in.Sets[i].Total = in.Sets[i].Count
		}
	}
	in.CompleteLenses = normalizeSourceInventoryObservationCompleteLenses(in)
	return in
}
