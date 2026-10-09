package types

import (
	"sort"
	"strconv"
	"strings"
)

// Display candidates are deliberately permissive. They are not object
// identity: qualifiers, source coordinates, and opaque source references must
// survive a roster union even when two rows can use the same short label.
func aggregateMemberIdentitySurfaceKey(member string) string {
	member = trimAggregateMemberSurface(member)
	if member == "" {
		return ""
	}
	if left, right, ok := aggregateRelationSurfaceParts(member); ok {
		return "relation:" + trimAggregateMemberSurface(left) + "\x00" + trimAggregateMemberSurface(right)
	}
	return "literal:" + member
}

func aggregateMemberIdentityKey(surface aggregateMemberSupportSurface) string {
	key := aggregateMemberIdentitySurfaceKey(surface.member)
	if key == "" {
		return ""
	}
	if surface.hasLoc {
		return key + "\x00loc:" + aggregateMemberIdentityLocationKey(surface.loc) + "\x00ref:" + aggregateMemberOpaqueSourceRef(surface)
	}
	if ref := strings.TrimSpace(surface.ref); ref != "" {
		return key + "\x00ref:" + ref
	}
	return key
}

func aggregateMemberIdentityLocationAgrees(a, b aggregateMemberSupportSurface) bool {
	return a.hasLoc && b.hasLoc &&
		aggregateMemberIdentityLocationKey(a.loc) == aggregateMemberIdentityLocationKey(b.loc) &&
		aggregateMemberOpaqueSourceRef(a) == aggregateMemberOpaqueSourceRef(b)
}

func aggregateMemberOpaqueSourceRef(surface aggregateMemberSupportSurface) string {
	ref := strings.TrimSpace(surface.ref)
	if _, _, ok := ParseAnswerSupportRefMemberLocation(ref); ok {
		return ""
	}
	return ref
}

func aggregateMemberIdentityLocationKey(loc AnswerSourceLocationSurface) string {
	// Repository/source paths are not universally case-insensitive. Display
	// lookup's case-folded key cannot establish source identity.
	return displayAnswerLocationFile(loc.File) + ":" + strconv.Itoa(loc.LineStart)
}

func aggregateMemberIdentityQualifier(member string) string {
	if _, qualifier, ok := AnswerAggregateDecoratedLabelParts(member); ok {
		return trimAggregateMemberSurface(qualifier)
	}
	return ""
}

// A known location permits a full spelling and its exact shorter display form,
// not two conflicting qualified names which merely happen to share a tail.
func aggregateMemberIdentityLabelsCompatible(a, b string) bool {
	return aggregateMemberIdentityHasDisplayForm(a, b) || aggregateMemberIdentityHasDisplayForm(b, a)
}

func aggregateMemberIdentityHasDisplayForm(full, short string) bool {
	want := aggregateMemberIdentitySurfaceKey(short)
	forms := AnswerAggregateMemberDisplayCandidates(full)
	if base, ok := aggregateRelationCallableSignatureBase(full); ok {
		forms = append(forms, base)
	}
	for _, form := range forms {
		if aggregateMemberIdentitySurfaceKey(form) == want {
			return true
		}
	}
	return false
}

func aggregateMemberFactIdentityKey(fact AnswerAggregateFact) string {
	keys := make([]string, 0, len(fact.Members))
	for i, member := range fact.Members {
		ref := ""
		if i < len(fact.SupportRefs) {
			ref = fact.SupportRefs[i]
		}
		keys = append(keys, aggregateMemberIdentityKey(normalizeAggregateMemberSupportSurface(member, ref)))
	}
	sort.Strings(keys)
	return strings.Join(keys, "\x1f")
}

func aggregateMemberSetEntrySurface(fact AnswerAggregateFact, i int) aggregateMemberSupportSurface {
	ref := ""
	if i < len(fact.SupportRefs) {
		ref = fact.SupportRefs[i]
	}
	return normalizeAggregateMemberSupportSurface(fact.Members[i], ref)
}
