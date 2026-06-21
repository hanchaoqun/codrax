package types

// SourceInventorySourceClassesComplete reports whether the source-class
// universe contains at least one positive known class and every such class is
// complete. Unknown or zero-count rows are ignored so placeholder census rows
// cannot close the universe.
func SourceInventorySourceClassesComplete(classes []SourceInventorySourceClassCount) bool {
	if len(classes) == 0 {
		return false
	}
	seen := false
	for _, class := range classes {
		if class.Role == SourcePathRoleUnknown || class.Count <= 0 {
			continue
		}
		seen = true
		if !class.Complete {
			return false
		}
	}
	return seen
}
