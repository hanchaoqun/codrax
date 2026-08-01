package types

import "strings"

func sourceInventoryDemotePrincipalRowsForTypedRelation(rowSet SourceInventoryPrincipalRowSet) SourceInventoryPrincipalRowSet {
	rowSet = NormalizeSourceInventoryPrincipalRowSet(rowSet)
	for _, row := range rowSet.PrincipalRows {
		row.Lane = SourceInventoryRowLaneSupport
		row.ReasonCode = "typed_relation_principal_authority"
		rowSet.SupportRows = append(rowSet.SupportRows, row)
	}
	rowSet.PrincipalRows = nil
	rowSet.PrincipalTotal = 0
	rowSet.PrincipalHiddenCount = 0
	rowSet.SupportTotal = len(rowSet.SupportRows)
	rowSet.Active = false
	return NormalizeSourceInventoryPrincipalRowSet(rowSet)
}

func sourceInventorySnapshotReasonCodes(s SourceInventoryAuthoritySnapshot) []string {
	var out []string
	add := func(code string) {
		code = strings.TrimSpace(code)
		if code != "" {
			out = append(out, code)
		}
	}
	if !s.Active {
		add("inactive")
	}
	if s.SupportOnly {
		add("support_only")
	}
	if s.NeedsFollowup {
		add("followup_required")
	}
	if !s.RequiredFilesCovered {
		add("required_files_uncovered")
	}
	if s.PrincipalAggregateFactCount == 0 && s.PrincipalAuthority {
		add("principal_aggregate_fact_missing")
	}
	if s.CanEnterMechanicalLanding {
		add("mechanical_landing_ready")
	}
	add(s.CompletionAuthority.ReasonCode)
	add(s.FollowupDebt.ReasonCode)
	return sourceInventorySnapshotUniqueStrings(out)
}

func sourceInventorySnapshotUniqueStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range in {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
