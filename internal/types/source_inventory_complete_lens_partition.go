package types

// sourceInventoryCompleteLensClassPartitions derives exact source-class
// sub-lenses from one complete per-query role set. The combined lens remains
// authoritative for an explicit all-source request; these partitions let a
// narrower production/test/auxiliary principal universe consume the same
// complete query without comparing its row count with the combined total.
//
// Every member must have a deterministic class (the persisted typed field, or
// the canonical path classifier used for legacy rows). If any row remains
// unknown, no partition is minted: an apparently complete known-class subset
// could otherwise hide unclassified rows that belong to that class.
func sourceInventoryCompleteLensClassPartitions(
	base SourceInventoryCompleteLens,
	members []SourceInventoryObservationMember,
) []SourceInventoryCompleteLens {
	if len(members) == 0 {
		return nil
	}
	byClass := map[SourcePathRole][]SourceInventoryObservationMember{}
	for _, member := range members {
		class := sourceInventoryObservationPathClass(member.SourceClass, member.File)
		if class == "" || class == SourcePathRoleUnknown {
			return nil
		}
		member.SourceClass = class
		byClass[class] = append(byClass[class], member)
	}
	out := make([]SourceInventoryCompleteLens, 0, len(byClass))
	for class, classMembers := range byClass {
		lens := SourceInventoryCompleteLens{
			Role:            base.Role,
			Scopes:          append([]string(nil), base.Scopes...),
			QueryPathScopes: append([]string(nil), base.QueryPathScopes...),
			SourceClasses:   []SourcePathRole{class},
			Count:           len(classMembers),
			Total:           len(classMembers),
			Provenance:      append([]string(nil), base.Provenance...),
		}
		// Populate language and surface-family axes from this class only.
		// Scopes are deliberately omitted here because a directory such as
		// "pkg" classifies as production even when this partition contains
		// only pkg/*_test.go rows.
		sourceInventoryCompleteLensPopulateSurface(&lens, nil, classMembers)
		lens.SourceClasses = []SourcePathRole{class}
		out = append(out, lens)
	}
	return out
}
