package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// sourceInventoryAppendCanonicalExactUniverseMember prevents repeated exact
// lens observations from manufacturing additional completion obligations for
// the same declaration. The identity is deliberately declaration-coordinate
// based and fully typed: role/scope are already fixed by the caller's group,
// while location, row-local surface family, and member identity must all be
// present and equal before a row is collapsed. Ambiguous same-line declarations
// therefore remain visible (fail open), as do rows without exact coordinates.
func sourceInventoryAppendCanonicalExactUniverseMember(
	members []types.SourceInventoryObservationMember,
	candidate types.SourceInventoryObservationMember,
) []types.SourceInventoryObservationMember {
	key := sourceInventoryExactUniverseMemberCanonicalKey(candidate)
	if key == "" {
		return append(members, candidate)
	}
	for _, member := range members {
		if sourceInventoryExactUniverseMemberCanonicalKey(member) == key {
			return members
		}
	}
	return append(members, candidate)
}

func sourceInventoryExactUniverseMemberCanonicalKey(member types.SourceInventoryObservationMember) string {
	location := sourceInventoryDuplicateRowLocationKey(member)
	if location == "" {
		return ""
	}
	family := strings.TrimSpace(types.SourceInventorySurfaceFamilyKey(member.SurfaceTerms))
	if family == "" {
		return ""
	}
	identity := strings.TrimSpace(member.Name)
	if identity == "" {
		identity = strings.TrimSpace(member.Key)
	}
	if member.Role == types.AnswerCandidateRoleFile || member.Role == types.AnswerCandidateRoleConfigFile {
		identity = strings.TrimSpace(member.File)
	}
	identity = sourceInventoryUniverseSurfaceKey(identity)
	if identity == "" {
		return ""
	}
	return strings.Join([]string{location, strings.ToLower(family), identity}, "\x00")
}
