package types

import (
	"sort"
	"strings"
)

const SourceInventoryRowReasonOutOfRequestedUniverse = "out_of_requested_universe"

// SourceInventoryRequestedUniverseView is the answer-facing requested-universe
// boundary for source-inventory rows. It is derived only from typed inventory
// rows, source-class/language/path metadata, source scope, explicit exclusion
// policy, and principal aggregate refs. It must not read raw user prose, model
// rationale, rendered repo_map summaries, eval labels, or final-answer text.
type SourceInventoryRequestedUniverseView struct {
	Active                               bool                  `json:"active,omitempty"`
	PrincipalRoles                       []AnswerCandidateRole `json:"principal_roles,omitempty"`
	PrincipalScope                       SourceScope           `json:"principal_scope,omitempty"`
	RepoWidePrincipal                    bool                  `json:"repo_wide_principal,omitempty"`
	Languages                            []string              `json:"languages,omitempty"`
	SourceClasses                        []SourcePathRole      `json:"source_classes,omitempty"`
	SurfaceFamilies                      []string              `json:"surface_families,omitempty"`
	ExplicitlyExcludedSourceClasses      []SourcePathRole      `json:"explicitly_excluded_source_classes,omitempty"`
	PrincipalRowCount                    int                   `json:"principal_row_count,omitempty"`
	SupportRowCount                      int                   `json:"support_row_count,omitempty"`
	AuditRowCount                        int                   `json:"audit_row_count,omitempty"`
	InventoryOutOfUniverseRowsSuppressed int                   `json:"inventory_out_of_universe_rows_suppressed,omitempty"`
	ReasonCodes                          []string              `json:"reason_codes,omitempty"`
}

func sourceInventoryApplyRequestedUniverse(
	rowSet SourceInventoryPrincipalRowSet,
	rm RequestModel,
	projected []AnswerAggregateFact,
) (SourceInventoryPrincipalRowSet, SourceInventoryRequestedUniverseView) {
	if !rowSet.Active {
		return rowSet, SourceInventoryRequestedUniverseView{}
	}
	refs := PrincipalAggregateMemberSetFactRefsForRequest(projected, &rm)
	filtered, suppressed, reasons := sourceInventoryFilterRowSetToProjectedRequestedUniverse(rowSet, refs)
	view := sourceInventoryBuildRequestedUniverseView(filtered, rm, suppressed, reasons)
	return filtered, view
}

func sourceInventoryFilterRowSetToProjectedRequestedUniverse(
	rowSet SourceInventoryPrincipalRowSet,
	refs []AnswerAggregateFactRef,
) (SourceInventoryPrincipalRowSet, int, []string) {
	if !rowSet.Active || len(rowSet.PrincipalRows) == 0 || len(refs) == 0 {
		return rowSet, 0, nil
	}
	if keys := sourceInventorySystemPrincipalRefRowKeys(refs); len(keys) > 0 {
		filtered, suppressed := sourceInventoryFilterPrincipalRowsToKeys(rowSet, keys)
		if suppressed > 0 {
			return filtered, suppressed, []string{SourceInventoryRowReasonOutOfRequestedUniverse, "system_principal_projection"}
		}
		return filtered, 0, nil
	}
	before := rowSet.PrincipalTotal
	filtered := sourceInventoryFilterPrincipalRowSetToExistingPrincipalFamilies(rowSet, refs)
	filtered, constrained := sourceInventoryFilterMixedPrincipalRowSetToExistingRows(filtered, refs)
	suppressed := before - filtered.PrincipalTotal
	if suppressed <= 0 {
		return filtered, 0, nil
	}
	reasons := []string{SourceInventoryRowReasonOutOfRequestedUniverse, "existing_principal_family"}
	if constrained {
		reasons = append(reasons, SourceInventoryRowReasonMixedUniverse)
	}
	return filtered, suppressed, reasons
}

func sourceInventorySystemPrincipalRefRowKeys(refs []AnswerAggregateFactRef) map[string]bool {
	out := map[string]bool{}
	for _, ref := range refs {
		if strings.TrimSpace(ref.Fact.Provenance) != SourceInventoryPrincipalRowSetAggregateProvenance {
			continue
		}
		if aggregateMemberSetSupportCoverage(ref.Fact) == 0 {
			continue
		}
		for key := range sourceInventoryAggregateFactRowKeys(ref.Fact) {
			if key != "" {
				out[key] = true
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func sourceInventoryFilterPrincipalRowsToKeys(rowSet SourceInventoryPrincipalRowSet, keys map[string]bool) (SourceInventoryPrincipalRowSet, int) {
	if len(keys) == 0 || len(rowSet.PrincipalRows) == 0 {
		return rowSet, 0
	}
	filtered := make([]SourceInventoryRow, 0, len(rowSet.PrincipalRows))
	demoted := make([]SourceInventoryRow, 0, len(rowSet.PrincipalRows))
	for _, row := range rowSet.PrincipalRows {
		key := sourceInventoryPrincipalRowKey(row)
		if key != "" && keys[key] {
			filtered = append(filtered, row)
			continue
		}
		row.Lane = SourceInventoryRowLaneAudit
		row.ReasonCode = SourceInventoryRowReasonOutOfRequestedUniverse
		demoted = append(demoted, row)
	}
	if len(filtered) == 0 || len(filtered) == len(rowSet.PrincipalRows) {
		return rowSet, 0
	}
	rowSet.PrincipalRows = filtered
	rowSet.AuditRows = append(rowSet.AuditRows, demoted...)
	rowSet.PrincipalHiddenCount = 0
	rowSet.PrincipalTotal = len(filtered)
	rowSet.AuditTotal += len(demoted)
	return rowSet, len(demoted)
}

func sourceInventoryBuildRequestedUniverseView(
	rowSet SourceInventoryPrincipalRowSet,
	rm RequestModel,
	suppressed int,
	reasons []string,
) SourceInventoryRequestedUniverseView {
	rowSet = NormalizeSourceInventoryPrincipalRowSet(rowSet)
	if !rowSet.Active {
		return SourceInventoryRequestedUniverseView{}
	}
	view := SourceInventoryRequestedUniverseView{
		Active:                               true,
		PrincipalRoles:                       normalizeSourceInventoryFollowupRoles(rowSet.PrincipalRoles),
		PrincipalScope:                       rowSet.PrincipalScope,
		RepoWidePrincipal:                    rowSet.RepoWidePrincipal,
		Languages:                            sourceInventoryRequestedUniverseLanguages(rowSet.PrincipalRows),
		SourceClasses:                        sourceInventoryRequestedUniverseSourceClasses(rowSet.PrincipalRows),
		SurfaceFamilies:                      sourceInventoryRequestedUniverseSurfaceFamilies(rowSet.PrincipalRows),
		ExplicitlyExcludedSourceClasses:      sourceInventoryExplicitlyExcludedSourceClasses(rm),
		PrincipalRowCount:                    rowSet.PrincipalTotal,
		SupportRowCount:                      rowSet.SupportTotal,
		AuditRowCount:                        rowSet.AuditTotal,
		InventoryOutOfUniverseRowsSuppressed: suppressed,
		ReasonCodes:                          sourceInventorySnapshotUniqueStrings(reasons),
	}
	return view
}

func sourceInventoryRequestedUniverseLanguages(rows []SourceInventoryRow) []string {
	seen := map[string]bool{}
	var out []string
	for _, row := range rows {
		lang := strings.ToLower(strings.TrimSpace(row.Language))
		if lang == "" || seen[lang] {
			continue
		}
		seen[lang] = true
		out = append(out, lang)
	}
	sort.Strings(out)
	return out
}

func sourceInventoryRequestedUniverseSourceClasses(rows []SourceInventoryRow) []SourcePathRole {
	seen := map[SourcePathRole]bool{}
	var out []SourcePathRole
	for _, row := range rows {
		class := row.SourceClass
		if class == "" || class == SourcePathRoleUnknown || seen[class] {
			continue
		}
		seen[class] = true
		out = append(out, class)
	}
	sort.SliceStable(out, func(i, j int) bool { return string(out[i]) < string(out[j]) })
	return out
}

func sourceInventoryRequestedUniverseSurfaceFamilies(rows []SourceInventoryRow) []string {
	seen := map[string]bool{}
	var out []string
	for _, row := range rows {
		family := sourceInventoryRowSurfaceFamily(row)
		if family == "" || seen[family] {
			continue
		}
		seen[family] = true
		out = append(out, family)
	}
	sort.Strings(out)
	return out
}

func sourceInventoryExplicitlyExcludedSourceClasses(rm RequestModel) []SourcePathRole {
	policy := rm.AnswerExclusionPolicy
	if policy == nil || !policy.Active() {
		return nil
	}
	seen := map[SourcePathRole]bool{}
	var out []SourcePathRole
	add := func(class SourcePathRole) {
		if class == "" || class == SourcePathRoleUnknown || seen[class] {
			return
		}
		seen[class] = true
		out = append(out, class)
	}
	for _, role := range policy.ExcludedCandidateRoles {
		switch role {
		case AnswerCandidateRoleTest:
			add(SourcePathRoleTest)
		case AnswerCandidateRoleDocumentation:
			add(SourcePathRoleDocumentation)
		case AnswerCandidateRoleExample:
			add(SourcePathRoleExample)
		case AnswerCandidateRoleFixture:
			add(SourcePathRoleFixture)
		case AnswerCandidateRoleGenerated:
			add(SourcePathRoleGenerated)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return string(out[i]) < string(out[j]) })
	return out
}

func sourceInventoryLimitPrincipalRowSet(rowSet SourceInventoryPrincipalRowSet, maxPrincipal, maxSupport, maxAudit int) SourceInventoryPrincipalRowSet {
	if !rowSet.Active {
		return rowSet
	}
	principal := append([]SourceInventoryRow(nil), rowSet.PrincipalRows...)
	support := append([]SourceInventoryRow(nil), rowSet.SupportRows...)
	audit := append([]SourceInventoryRow(nil), rowSet.AuditRows...)
	rowSet.PrincipalTotal = len(principal)
	rowSet.SupportTotal = len(support)
	rowSet.AuditTotal = len(audit)
	rowSet.PrincipalRows, rowSet.PrincipalHiddenCount = sourceInventoryLimitRows(principal, maxPrincipal)
	rowSet.SupportRows, rowSet.SupportHiddenCount = sourceInventoryLimitRows(support, maxSupport)
	rowSet.AuditRows, rowSet.AuditHiddenCount = sourceInventoryLimitRows(audit, maxAudit)
	rowSet.Active = rowSet.PrincipalTotal > 0
	return rowSet
}
