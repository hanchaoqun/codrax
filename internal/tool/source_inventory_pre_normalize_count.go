package tool

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// canonicalizeRequestBoundSourceInventoryMemberSetCounts repairs only the
// mechanical count field of a source-inventory principal member_set before the
// generic aggregate validator runs. Authority comes from the same exact,
// executable request-bound roster that will own the post-normalize projection;
// this is not a general license to reinterpret arbitrary model member sets.
func canonicalizeRequestBoundSourceInventoryMemberSetCounts(
	ctx *types.BusContext,
	raw []types.AnswerAggregateFact,
) ([]types.AnswerAggregateFact, []string) {
	if ctx == nil || ctx.Mutable == nil || ctx.AnalysisIR == nil || len(raw) == 0 {
		return raw, nil
	}
	rm := ctx.AnalysisIR.RequestModel
	if rm.SourceInventoryProfile == nil || !rm.SourceInventoryProfile.Active() || types.HasTypedRelationMemberSetShape(rm) {
		return raw, nil
	}
	authority, ok := types.SourceInventoryRequestBoundPrincipalRowSetAggregateFact(
		types.SourceInventoryObservationFromMutable(ctx.Mutable), rm,
	)
	if !ok || len(authority.Members) == 0 {
		return raw, nil
	}
	authorityKeys := sourceInventoryAggregateMemberSurfaceKeys(authority.Members)
	if len(authorityKeys) == 0 {
		return raw, nil
	}
	out := cloneCompletionAggregateFacts(raw)
	var notes []string
	for i := range out {
		fact := &out[i]
		if fact.Kind != types.AnswerAggregateMemberSet || len(fact.Members) == 0 ||
			types.AnswerAggregateFactRoleForRequest(*fact, &rm) != types.AnswerAggregateRolePrincipalAnswer ||
			!sourceInventoryAggregateMembersOverlapAuthority(fact.Members, authorityKeys) {
			continue
		}
		value, err := strconv.Atoi(strings.TrimSpace(fact.Value))
		if err != nil || value < 0 || value == len(fact.Members) {
			continue
		}
		fact.Value = strconv.Itoa(len(fact.Members))
		notes = append(notes, fmt.Sprintf(
			"canonicalized request-bound source-inventory member_set %q count from %d to structured members length %d before exact typed roster projection",
			strings.TrimSpace(fact.Label), value, len(fact.Members),
		))
	}
	if len(notes) == 0 {
		return raw, nil
	}
	return out, notes
}

func sourceInventoryAggregateMemberSurfaceKeys(members []string) map[string]bool {
	out := make(map[string]bool, len(members))
	for _, member := range members {
		if key := types.AnswerAggregateMemberSurfaceKey(member); key != "" {
			out[key] = true
		}
	}
	return out
}

func sourceInventoryAggregateMembersOverlapAuthority(members []string, authority map[string]bool) bool {
	for _, member := range members {
		if authority[types.AnswerAggregateMemberSurfaceKey(member)] {
			return true
		}
	}
	return false
}
