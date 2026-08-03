package tool

import (
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// canonicalizeExactTypedRelationFactMembers projects repeated relation
// observations (for example two call sites owned by one caller function) onto
// the accepted typed candidate identity. Distinct same-name candidates in
// different files remain distinct; ambiguous rows fail open.
func canonicalizeExactTypedRelationFactMembers(
	fact types.AnswerAggregateFact,
	candidates []types.TypedRelationCandidate,
) (types.AnswerAggregateFact, int) {
	memberIDs := map[string][]string{}
	sourceIDs := map[string][]string{}
	for _, candidate := range candidates {
		if !candidate.Precision.CoverageGateEligible() {
			continue
		}
		if key := aggregateMemberKey(candidate.Member.Name); key != "" {
			file := canonicalRelationSourcePath(candidate.Member.File)
			memberIDs[key] = appendUniqueStrings(memberIDs[key], "member|"+key+"|"+file)
		}
		if key := aggregateMemberKey(candidate.SourceName); key != "" {
			file := canonicalRelationSourcePath(candidate.SourceFile)
			sourceIDs[key] = appendUniqueStrings(sourceIDs[key], "source|"+key+"|"+file)
		}
	}
	if len(memberIDs) == 0 || len(fact.Members) < 2 {
		return fact, 0
	}

	seen := map[string]bool{}
	keep := make([]int, 0, len(fact.Members))
	for idx, raw := range fact.Members {
		supportFile := relationAggregateMemberSupportFile(fact, idx)
		identities := relationAggregateMemberIdentities(raw, supportFile, memberIDs, sourceIDs)
		if len(identities) != 1 {
			keep = append(keep, idx)
			continue
		}
		if seen[identities[0]] {
			continue
		}
		seen[identities[0]] = true
		keep = append(keep, idx)
	}
	removed := len(fact.Members) - len(keep)
	if removed == 0 {
		return fact, 0
	}
	fact.Members = relationFactStringsAtIndexes(fact.Members, keep)
	fact.MemberNotes = relationFactStringsAtIndexes(fact.MemberNotes, keep)
	fact.SupportRefs = relationFactStringsAtIndexes(fact.SupportRefs, keep)
	fact.Value = strconv.Itoa(len(fact.Members))
	return fact, removed
}

func relationAggregateMemberIdentities(
	raw string,
	supportFile string,
	memberIDs map[string][]string,
	sourceIDs map[string][]string,
) []string {
	var out []string
	for _, display := range aggregateMemberDisplayCandidates(raw) {
		key := aggregateMemberKey(display)
		if key == "" {
			continue
		}
		ids := relationAggregateIdentitiesInSupportFile(memberIDs[key], supportFile)
		for _, id := range ids {
			out = appendUniqueStrings(out, id)
		}
		for _, id := range relationAggregateIdentitiesInSupportFile(sourceIDs[key], supportFile) {
			out = appendUniqueStrings(out, id)
		}
	}
	return out
}

func relationAggregateIdentitiesInSupportFile(ids []string, supportFile string) []string {
	if supportFile == "" || len(ids) == 0 {
		return ids
	}
	var matched []string
	for _, id := range ids {
		if strings.HasSuffix(id, "|"+supportFile) {
			matched = append(matched, id)
		}
	}
	// A structured row anchored in a different file is not the same typed
	// member or relation source merely because its display name matches. A
	// source candidate without a typed file axis also cannot authorize an
	// anchored row; fail open instead of guessing and collapsing it.
	return matched
}

func relationAggregateMemberSupportFile(fact types.AnswerAggregateFact, idx int) string {
	if idx < 0 || idx >= len(fact.SupportRefs) {
		return ""
	}
	_, location, ok := types.ParseAnswerSupportRefMemberLocation(fact.SupportRefs[idx])
	if !ok {
		return ""
	}
	return canonicalRelationSourcePath(location.File)
}

func relationFactStringsAtIndexes(in []string, indexes []int) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(indexes))
	for _, idx := range indexes {
		if idx >= 0 && idx < len(in) {
			out = append(out, in[idx])
		}
	}
	return out
}
