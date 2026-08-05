package types

import "strings"

// sourceInventoryDemoteFactsOutsidePrincipalScope prevents a model-authored
// member set from becoming a hard principal obligation when every one of its
// row-local supports resolves uniquely to a requested-role observation row
// outside the typed principal source scope. It deliberately does not inspect
// the fact label, request prose, answer prose, symbol spelling conventions, or
// language-specific test naming. Mixed, unresolved, ambiguous, unlocated, and
// all-source sets fail closed and retain their existing role.
func sourceInventoryDemoteFactsOutsidePrincipalScope(
	facts []AnswerAggregateFact,
	observation SourceInventoryObservation,
	rowSet SourceInventoryPrincipalRowSet,
	rm RequestModel,
) []AnswerAggregateFact {
	if len(facts) == 0 || !observation.IsActive() || !rowSet.Active || rowSet.PrincipalTotal == 0 ||
		len(rowSet.PrincipalRoles) == 0 ||
		rowSet.PrincipalScope == SourceScopeAll {
		return facts
	}
	requestedRoles := make(map[AnswerCandidateRole]bool, len(rowSet.PrincipalRoles))
	for _, role := range rowSet.PrincipalRoles {
		if role != "" && role != AnswerCandidateRoleUnknown {
			requestedRoles[role] = true
		}
	}
	if len(requestedRoles) == 0 {
		return facts
	}
	rowsByLabel := sourceInventoryObservedRowsByLabel(observation, requestedRoles)
	if len(rowsByLabel) == 0 {
		return facts
	}
	out := cloneAnswerAggregateFacts(facts)
	for i := range out {
		fact := &out[i]
		if fact.Kind != AnswerAggregateMemberSet || len(fact.Members) == 0 ||
			strings.TrimSpace(fact.Provenance) == SourceInventoryPrincipalRowSetAggregateProvenance ||
			AnswerAggregateFactRoleForRequest(*fact, &rm) != AnswerAggregateRolePrincipalAnswer {
			continue
		}
		allOutside := true
		for memberIdx, member := range fact.Members {
			ref := ""
			if memberIdx < len(fact.SupportRefs) {
				ref = fact.SupportRefs[memberIdx]
			}
			row, ok := sourceInventoryResolveObservedScopeRow(member, ref, rowsByLabel)
			if !ok || row.sourceClass == SourcePathRoleUnknown ||
				SourceScopeAllowsPathRole(rowSet.PrincipalScope, row.sourceClass) {
				allOutside = false
				break
			}
		}
		if !allOutside {
			continue
		}
		fact.Role = AnswerAggregateRoleSupportingCoverage
		fact.Provenance = appendAggregateProvenance(fact.Provenance, "demoted:outside_source_inventory_principal_scope")
	}
	return out
}

type sourceInventoryObservedScopeRow struct {
	role        AnswerCandidateRole
	sourceClass SourcePathRole
	file        string
	line        int
}

func sourceInventoryObservedRowsByLabel(
	observation SourceInventoryObservation,
	requestedRoles map[AnswerCandidateRole]bool,
) map[string][]sourceInventoryObservedScopeRow {
	out := make(map[string][]sourceInventoryObservedScopeRow)
	for _, set := range observation.Sets {
		for _, member := range set.Members {
			role := member.Role
			if role == "" || role == AnswerCandidateRoleUnknown {
				role = set.Role
			}
			if !requestedRoles[role] {
				continue
			}
			label := aggregateMemberSetProjectionMemberKey(sourceInventoryPrincipalRowMemberLabel(SourceInventoryRow{Role: role, Member: member}))
			file := normalizeAnswerSupportPath(member.File)
			if label == "" || file == "" || member.Line <= 0 {
				continue
			}
			class := sourceInventoryObservationPathClass(member.SourceClass, member.File)
			out[label] = append(out[label], sourceInventoryObservedScopeRow{
				role: role, sourceClass: class, file: file, line: member.Line,
			})
		}
	}
	return out
}

func sourceInventoryResolveObservedScopeRow(
	member string,
	ref string,
	rowsByLabel map[string][]sourceInventoryObservedScopeRow,
) (sourceInventoryObservedScopeRow, bool) {
	surface := normalizeAggregateMemberSupportSurface(member, ref)
	if strings.TrimSpace(surface.label) == "" || !surface.hasLoc {
		return sourceInventoryObservedScopeRow{}, false
	}
	label := aggregateMemberSetProjectionMemberKey(surface.label)
	candidates := rowsByLabel[label]
	var resolved sourceInventoryObservedScopeRow
	matches := 0
	for _, candidate := range candidates {
		if !sourceInventoryScopeFactLocationMatches(surface.loc, candidate.file, candidate.line) {
			continue
		}
		resolved = candidate
		matches++
	}
	return resolved, matches == 1
}

func sourceInventoryScopeFactLocationMatches(loc AnswerSourceLocationSurface, candidateFile string, candidateLine int) bool {
	factFile := normalizeAnswerSupportPath(loc.File)
	candidateFile = normalizeAnswerSupportPath(candidateFile)
	if factFile == "" || candidateFile == "" || loc.LineStart <= 0 || candidateLine <= 0 || loc.LineStart != candidateLine {
		return false
	}
	return factFile == candidateFile ||
		strings.HasSuffix(candidateFile, "/"+factFile) ||
		strings.HasSuffix(factFile, "/"+candidateFile)
}
