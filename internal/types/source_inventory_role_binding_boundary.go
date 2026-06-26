package types

// SourceInventoryCompletionIsSupportOnly reports whether a persisted
// source_inventory_profile should remain a navigation/support carrier rather
// than a completion authority for the principal answer. The boundary is
// schema-only: it consumes typed request lanes and candidate-role enums, never
// raw request text, model rationale, prompt prose, or rendered tool output.
func SourceInventoryCompletionIsSupportOnly(rm RequestModel) bool {
	if rm.SourceInventoryProfile == nil || !rm.SourceInventoryProfile.Active() {
		return false
	}
	return SourceInventoryProfileCompletionIsSupportOnly(rm.SourceInventoryProfile) ||
		SourceInventoryProfileConflictsWithRoleBinding(rm)
}
