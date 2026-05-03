package types

import "strings"

// ExactResolutionTargetIsConfigKey reports whether the exact-resolution
// contract is about a concrete config key.
func ExactResolutionTargetIsConfigKey(c *ExactResolutionContract) bool {
	if c == nil {
		return false
	}
	if c.TargetKind == SubjectConfigKey {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(c.TargetLabel), "config key")
}
