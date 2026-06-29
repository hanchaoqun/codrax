package types

import (
	"sort"
	"strconv"
	"strings"
)

const SourceInventoryPrincipalRowSetAggregateProvenance = "system:source_inventory_principal_row_set"

// ProjectSourceInventoryPrincipalRowSetAggregateFacts makes the typed
// source-inventory row universe available to the existing aggregate-fact /
// EnumerationDisplayRows / AnswerSupportPlan contract path. It derives only
// from SourceInventoryObservation + RequestModel typed fields and never reads
// raw request text, model rationale, rendered repo_map output, final-answer
// prose, grep counts, or elapsed-time narratives.
func ProjectSourceInventoryPrincipalRowSetAggregateFacts(facts []AnswerAggregateFact, observation SourceInventoryObservation, rm RequestModel) []AnswerAggregateFact {
	out := cloneAnswerAggregateFacts(facts)
	out = sourceInventoryDemoteAttributeMemberSetFacts(out, observation, rm)
	if sourceInventoryPrincipalRowSetProjectionDisabled(observation, rm) {
		return out
	}
	rowSet := BuildSourceInventoryPrincipalRowSet(SourceInventoryPrincipalRowSetInput{
		Observation:  observation,
		RequestModel: rm,
	})
	if !rowSet.Active || rowSet.PrincipalTotal == 0 {
		return out
	}
	refs := PrincipalAggregateMemberSetFactRefsForRequest(out, &rm)
	rowSet = sourceInventoryFilterPrincipalRowSetToExistingPrincipalFamilies(rowSet, refs)
	rowSet, constrainedToExistingRows := sourceInventoryFilterMixedPrincipalRowSetToExistingRows(rowSet, refs)
	if !rowSet.Active || rowSet.PrincipalTotal == 0 {
		return out
	}
	rowKeys := sourceInventoryPrincipalRowSetKeys(rowSet)
	if len(rowKeys) == 0 {
		return out
	}
	if constrainedToExistingRows && sourceInventoryPrincipalFactsCoverRows(refs, rowKeys) {
		return out
	}
	if sourceInventoryPrincipalFactUniverseComplete(refs, rowKeys) {
		return out
	}
	if sourceInventoryPrincipalRowSetDisjointFromExistingPrincipal(refs, rowSet) {
		return out
	}
	principalIndexes := map[int]bool{}
	for _, ref := range refs {
		principalIndexes[ref.Index] = true
	}
	for i := range out {
		if !principalIndexes[i] {
			continue
		}
		if !sourceInventoryAggregateFactHasAnyRowSetOverlap(out[i], rowKeys) {
			continue
		}
		out[i].Role = AnswerAggregateRoleSupportingCoverage
		out[i].Provenance = appendAggregateProvenance(out[i].Provenance, "demoted:shadowed_by_source_inventory_principal_row_set")
	}
	fact, ok := SourceInventoryPrincipalRowSetAggregateFact(rowSet)
	if !ok {
		return out
	}
	return append(out, fact)
}

// SourceInventoryPrincipalRowSetAggregateFact projects principal row-set rows
// into a member_set aggregate fact compatible with the existing final-answer
// contract path. The returned fact is system-authored from typed inventory
// rows; it does not infer answer content from prose.
func SourceInventoryPrincipalRowSetAggregateFact(rowSet SourceInventoryPrincipalRowSet) (AnswerAggregateFact, bool) {
	rowSet = NormalizeSourceInventoryPrincipalRowSet(rowSet)
	if !rowSet.Active || rowSet.PrincipalTotal == 0 {
		return AnswerAggregateFact{}, false
	}
	rows := sourceInventoryAllPrincipalRows(rowSet)
	if len(rows) == 0 {
		return AnswerAggregateFact{}, false
	}
	members := make([]string, 0, len(rows))
	refs := make([]string, 0, len(rows))
	notes := make([]string, 0, len(rows))
	for _, row := range rows {
		member := sourceInventoryPrincipalRowMemberLabel(row)
		if member == "" {
			continue
		}
		members = append(members, member)
		refs = append(refs, sourceInventoryPrincipalRowSupportRef(row, member))
		notes = append(notes, sourceInventoryPrincipalRowNote(row))
	}
	if len(members) == 0 {
		return AnswerAggregateFact{}, false
	}
	fact := AnswerAggregateFact{
		Kind:        AnswerAggregateMemberSet,
		Label:       "source inventory principal rows",
		Value:       strconv.Itoa(len(members)),
		Role:        AnswerAggregateRolePrincipalAnswer,
		Provenance:  SourceInventoryPrincipalRowSetAggregateProvenance,
		Dimensions:  sourceInventoryPrincipalRowSetDimensions(rowSet),
		Members:     members,
		SupportRefs: refs,
		MemberNotes: notes,
	}
	fact.Members, fact.SupportRefs, fact.MemberNotes = normalizeAggregateMemberSetMemberSupportNoteSurfaces(fact.Members, fact.SupportRefs, fact.MemberNotes)
	fact.Value = strconv.Itoa(len(fact.Members))
	return fact, true
}

func NormalizeSourceInventoryPrincipalRowSet(rowSet SourceInventoryPrincipalRowSet) SourceInventoryPrincipalRowSet {
	rowSet.Scopes = sourceInventoryFollowupScopes(rowSet.Scopes)
	rowSet.PrincipalRoles = normalizeSourceInventoryFollowupRoles(rowSet.PrincipalRoles)
	rowSet.SourceClasses = cloneSourceInventorySourceClassCounts(rowSet.SourceClasses)
	rowSet.PrincipalRows = sourceInventoryStableRows(rowSet.PrincipalRows)
	rowSet.SupportRows = sourceInventoryStableRows(rowSet.SupportRows)
	rowSet.AuditRows = sourceInventoryStableRows(rowSet.AuditRows)
	rowSet.PrincipalTotal = maxSourceInventoryObservationTotal(rowSet.PrincipalTotal, len(rowSet.PrincipalRows))
	rowSet.SupportTotal = maxSourceInventoryObservationTotal(rowSet.SupportTotal, len(rowSet.SupportRows))
	rowSet.AuditTotal = maxSourceInventoryObservationTotal(rowSet.AuditTotal, len(rowSet.AuditRows))
	rowSet.Active = rowSet.Active && rowSet.PrincipalTotal > 0
	return rowSet
}

func sourceInventoryAllPrincipalRows(rowSet SourceInventoryPrincipalRowSet) []SourceInventoryRow {
	rows := append([]SourceInventoryRow(nil), rowSet.PrincipalRows...)
	return sourceInventoryFamilyBalancedRows(rows)
}

func sourceInventoryAggregateFactHasAnyRowSetOverlap(fact AnswerAggregateFact, rowKeys map[string]bool) bool {
	for key := range sourceInventoryAggregateFactRowKeys(fact) {
		if rowKeys[key] {
			return true
		}
		if label := sourceInventoryRowKeyLabelPart(key); label != "" {
			for want := range rowKeys {
				if label == sourceInventoryRowKeyLabelPart(want) {
					return true
				}
			}
		}
	}
	return false
}

func sourceInventoryPrincipalRowSetKeys(rowSet SourceInventoryPrincipalRowSet) map[string]bool {
	out := map[string]bool{}
	for _, row := range sourceInventoryAllPrincipalRows(rowSet) {
		if key := sourceInventoryPrincipalRowKey(row); key != "" {
			out[key] = true
		}
	}
	return out
}

func sourceInventoryAggregateFactRowKeys(fact AnswerAggregateFact) map[string]bool {
	out := map[string]bool{}
	for i, member := range fact.Members {
		ref := ""
		if i < len(fact.SupportRefs) {
			ref = fact.SupportRefs[i]
		}
		surface := normalizeAggregateMemberSupportSurface(member, ref)
		label := strings.TrimSpace(surface.label)
		if label == "" {
			label = aggregateMemberSupportSurfaceLabel(member)
		}
		location := ""
		if surface.hasLoc {
			location = aggregateMemberStartLocation(surface.loc)
		}
		if key := sourceInventoryPrincipalRowKeyParts(label, location); key != "" {
			out[key] = true
		}
	}
	return out
}

func sourceInventoryPrincipalRowKey(row SourceInventoryRow) string {
	return sourceInventoryPrincipalRowKeyParts(sourceInventoryPrincipalRowMemberLabel(row), sourceInventoryPrincipalRowLocation(row))
}

func sourceInventoryPrincipalRowKeyParts(label, location string) string {
	labelKey := aggregateMemberSetProjectionMemberKey(label)
	locationKey := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(location, `\`, `/`)))
	if labelKey == "" && locationKey == "" {
		return ""
	}
	return labelKey + "\x00" + locationKey
}

func sourceInventoryRowKeySetsEqual(a, b map[string]bool) bool {
	if len(a) == 0 || len(a) != len(b) {
		return false
	}
	for key := range a {
		if !b[key] {
			return false
		}
	}
	return true
}

func sourceInventoryRowKeyLabelPart(key string) string {
	if key == "" {
		return ""
	}
	if idx := strings.Index(key, "\x00"); idx >= 0 {
		return key[:idx]
	}
	return key
}

func sourceInventoryPrincipalRowMemberLabel(row SourceInventoryRow) string {
	member := row.Member
	for _, raw := range []string{member.Name, member.Key} {
		if s := strings.TrimSpace(raw); s != "" {
			return s
		}
	}
	if label, _, ok := ParseAnswerSupportRefMemberLocation(member.SupportRef); ok && strings.TrimSpace(label) != "" {
		return strings.TrimSpace(label)
	}
	if loc := sourceInventoryPrincipalRowLocation(row); loc != "" {
		return loc
	}
	return ""
}

func sourceInventoryPrincipalRowSupportRef(row SourceInventoryRow, label string) string {
	if label == "" {
		label = sourceInventoryPrincipalRowMemberLabel(row)
	}
	if label == "" {
		return ""
	}
	if _, loc, ok := ParseAnswerSupportRefMemberLocation(row.Member.SupportRef); ok {
		return formatAggregateMemberSupportRef(label, loc)
	}
	file := strings.TrimSpace(strings.ReplaceAll(row.Member.File, `\`, `/`))
	if file == "" || row.Member.Line <= 0 {
		return ""
	}
	return formatAggregateMemberSupportRef(label, AnswerSourceLocationSurface{File: file, LineStart: row.Member.Line})
}

func sourceInventoryPrincipalRowLocation(row SourceInventoryRow) string {
	if _, loc, ok := ParseAnswerSupportRefMemberLocation(row.Member.SupportRef); ok {
		return aggregateMemberStartLocation(loc)
	}
	file := strings.TrimSpace(strings.ReplaceAll(row.Member.File, `\`, `/`))
	if file == "" || row.Member.Line <= 0 {
		return ""
	}
	return aggregateMemberStartLocation(AnswerSourceLocationSurface{File: file, LineStart: row.Member.Line})
}

func sourceInventoryPrincipalRowNote(row SourceInventoryRow) string {
	var parts []string
	for _, note := range sourceInventoryPrincipalRowVisibleNoteSegments(row.Member.Note) {
		if note != "" {
			parts = append(parts, note)
		}
	}
	return strings.Join(parts, "; ")
}

func sourceInventoryPrincipalRowSetDimensions(rowSet SourceInventoryPrincipalRowSet) []AnswerAggregateDimension {
	var out []AnswerAggregateDimension
	add := func(name, value string) {
		name = strings.TrimSpace(name)
		value = strings.TrimSpace(value)
		if name == "" || value == "" || len(out) >= maxAnswerAggregateDimensions {
			return
		}
		out = append(out, AnswerAggregateDimension{Name: name, Value: value})
	}
	add("source_inventory", "principal_row_set")
	add("source_scope", string(rowSet.PrincipalScope))
	if rowSet.RepoWidePrincipal {
		add("repo_wide", "true")
	}
	add("principal_roles", answerCandidateRolesSummary(rowSet.PrincipalRoles))
	add("source_classes", sourceInventoryClassSummary(rowSet.SourceClasses))
	return out
}

func answerCandidateRolesSummary(roles []AnswerCandidateRole) string {
	roles = normalizeSourceInventoryFollowupRoles(roles)
	if len(roles) == 0 {
		return ""
	}
	out := make([]string, 0, len(roles))
	for _, role := range roles {
		if role != "" && role != AnswerCandidateRoleUnknown {
			out = append(out, string(role))
		}
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

func sourceInventoryClassSummary(classes []SourceInventorySourceClassCount) string {
	if len(classes) == 0 {
		return ""
	}
	type item struct {
		role  SourcePathRole
		count int
	}
	items := make([]item, 0, len(classes))
	for _, class := range cloneSourceInventorySourceClassCounts(classes) {
		if class.Role == "" || class.Role == SourcePathRoleUnknown || class.Count <= 0 {
			continue
		}
		items = append(items, item{role: class.Role, count: class.Count})
	}
	sort.SliceStable(items, func(i, j int) bool {
		return string(items[i].role) < string(items[j].role)
	})
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, string(item.role)+":"+strconv.Itoa(item.count))
	}
	return strings.Join(out, ",")
}

func appendAggregateProvenance(existing, next string) string {
	existing = strings.TrimSpace(existing)
	next = strings.TrimSpace(next)
	if next == "" {
		return existing
	}
	if existing == "" {
		return next
	}
	for _, part := range strings.Split(existing, ";") {
		if strings.TrimSpace(part) == next {
			return existing
		}
	}
	return existing + ";" + next
}
