package types

// SourceInventoryAdvisoryRoleLabel returns the display label for advisory
// candidate sets. It intentionally distinguishes the package role's source
// inventory usage from a language-specific package answer: in this lane the
// role is a bounded package/directory/module scope carrier.
func SourceInventoryAdvisoryRoleLabel(role AnswerCandidateRole) string {
	switch role {
	case AnswerCandidateRolePackage:
		return "package/directory/module scope"
	case AnswerCandidateRoleConfigFile:
		return "configuration/manifest file"
	case AnswerCandidateRoleUnknown:
		return "candidate"
	default:
		return string(role)
	}
}
