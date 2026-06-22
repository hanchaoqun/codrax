package types

func normalizeSourceInventoryObservation(in SourceInventoryObservation) SourceInventoryObservation {
	in.SourceClasses = normalizeSourceInventorySourceClassCounts(in.SourceClasses)
	if len(in.Sets) == 0 && len(in.SourceClasses) == 0 {
		return SourceInventoryObservation{}
	}
	in.Active = true
	for i := range in.Sets {
		in.Sets[i].Count = len(in.Sets[i].Members)
		if in.Sets[i].Total < in.Sets[i].Count {
			in.Sets[i].Total = in.Sets[i].Count
		}
	}
	return in
}
