package types

import "strings"

func sourceInventoryDemoteAttributeMemberSetFacts(facts []AnswerAggregateFact, observation SourceInventoryObservation, rm RequestModel) []AnswerAggregateFact {
	if len(facts) == 0 || rm.SourceInventoryProfile == nil || !rm.SourceInventoryProfile.Active() ||
		rm.SourceInventoryProfile.RequiresPrincipalRole(AnswerCandidateRolePackage) ||
		rm.SourceInventoryProfile.RequiresPrincipalRole(AnswerCandidateRoleImportPath) {
		return facts
	}
	attrMembers := sourceInventoryAttributeMemberKeys(observation, AnswerCandidateRolePackage)
	if len(attrMembers) == 0 {
		return facts
	}
	out := cloneAnswerAggregateFacts(facts)
	changed := false
	for i := range out {
		if !sourceInventoryAggregateFactIsAttributeMemberSet(out[i], attrMembers) {
			continue
		}
		out[i].Role = AnswerAggregateRoleSupportingCoverage
		out[i].Provenance = appendAggregateFactProvenance(out[i].Provenance, "demoted:source_inventory_attribute_member_set")
		changed = true
	}
	if !changed {
		return facts
	}
	return out
}

func sourceInventoryAttributeMemberKeys(observation SourceInventoryObservation, role AnswerCandidateRole) map[string]bool {
	observation = CloneSourceInventoryObservation(observation)
	out := map[string]bool{}
	for _, set := range observation.Sets {
		for _, member := range set.Members {
			for _, attr := range member.Attributes {
				if attr.Role != role {
					continue
				}
				if key := aggregateMemberSetProjectionMemberKey(attr.Name); key != "" {
					out[key] = true
				}
			}
		}
	}
	return out
}

func sourceInventoryAggregateFactIsAttributeMemberSet(fact AnswerAggregateFact, attrMembers map[string]bool) bool {
	if len(attrMembers) == 0 || fact.Kind != AnswerAggregateMemberSet || !answerAggregateFactCarriesCompleteMemberSet(fact) || len(fact.Members) == 0 {
		return false
	}
	role := NormalizeAnswerAggregateRole(fact.Role)
	if role == AnswerAggregateRoleSupportingCoverage || role == AnswerAggregateRoleAuditLedger {
		return false
	}
	for _, member := range fact.Members {
		key := aggregateMemberSetProjectionMemberKey(member)
		if key == "" {
			key = strings.ToLower(strings.TrimSpace(member))
		}
		if key == "" || !attrMembers[key] {
			return false
		}
	}
	return true
}
