package types

import (
	"sort"
	"strings"
)

// SourceInventoryRowLane is the closed lane classification for source-inventory
// rows after request-scope projection. It is a view over typed observation
// fields, not a semantic claim and not final answer text.
type SourceInventoryRowLane string

const (
	SourceInventoryRowLaneUnknown   SourceInventoryRowLane = ""
	SourceInventoryRowLanePrincipal SourceInventoryRowLane = "principal"
	SourceInventoryRowLaneSupport   SourceInventoryRowLane = "support"
	SourceInventoryRowLaneAudit     SourceInventoryRowLane = "audit"
)

const (
	SourceInventoryRowReasonPrincipalScope = "principal_scope"
	SourceInventoryRowReasonSupportScope   = "support_scope"
	SourceInventoryRowReasonNonPrincipal   = "non_principal_role"
	SourceInventoryRowReasonNoLocation     = "no_location"
	SourceInventoryRowReasonSurfaceFamily  = "non_requested_surface_family"
)

// SourceInventoryPrincipalRowSetInput is the import-cycle-safe input for
// deriving a principal/support/audit row view from a source inventory
// observation. Limits are applied per lane only after totals are computed.
type SourceInventoryPrincipalRowSetInput struct {
	Observation      SourceInventoryObservation `json:"observation,omitempty"`
	RequestModel     RequestModel               `json:"request_model,omitempty"`
	MaxPrincipalRows int                        `json:"max_principal_rows,omitempty"`
	MaxSupportRows   int                        `json:"max_support_rows,omitempty"`
	MaxAuditRows     int                        `json:"max_audit_rows,omitempty"`
}

// SourceInventoryPrincipalRowSet is the typed row-lane view consumed by
// finalizer handoff and future contract/eval consumers. It preserves the full
// total counts while keeping rendered row slices bounded.
type SourceInventoryPrincipalRowSet struct {
	Active               bool                              `json:"active,omitempty"`
	Scopes               []string                          `json:"scopes,omitempty"`
	PrincipalRoles       []AnswerCandidateRole             `json:"principal_roles,omitempty"`
	PrincipalScope       SourceScope                       `json:"principal_scope,omitempty"`
	RepoWidePrincipal    bool                              `json:"repo_wide_principal,omitempty"`
	SourceClasses        []SourceInventorySourceClassCount `json:"source_classes,omitempty"`
	PrincipalRows        []SourceInventoryRow              `json:"principal_rows,omitempty"`
	SupportRows          []SourceInventoryRow              `json:"support_rows,omitempty"`
	AuditRows            []SourceInventoryRow              `json:"audit_rows,omitempty"`
	PrincipalTotal       int                               `json:"principal_total,omitempty"`
	SupportTotal         int                               `json:"support_total,omitempty"`
	AuditTotal           int                               `json:"audit_total,omitempty"`
	PrincipalHiddenCount int                               `json:"principal_hidden_count,omitempty"`
	SupportHiddenCount   int                               `json:"support_hidden_count,omitempty"`
	AuditHiddenCount     int                               `json:"audit_hidden_count,omitempty"`
}

// SourceInventoryRow is one row from SourceInventoryObservation with lane and
// source-class metadata attached.
type SourceInventoryRow struct {
	Lane        SourceInventoryRowLane           `json:"lane,omitempty"`
	ReasonCode  string                           `json:"reason_code,omitempty"`
	SourceClass SourcePathRole                   `json:"source_class,omitempty"`
	Role        AnswerCandidateRole              `json:"role,omitempty"`
	Language    string                           `json:"language,omitempty"`
	Member      SourceInventoryObservationMember `json:"member,omitempty"`
}

type sourceInventoryRowFamilyKey struct {
	role        AnswerCandidateRole
	sourceClass SourcePathRole
	language    string
}

// BuildSourceInventoryPrincipalRowSet derives principal/support/audit source
// inventory lanes from typed observation and request scope only. It never reads
// raw request text, model rationale, rendered repo_map output, final-answer
// prose, grep counts, or elapsed-time narratives.
func BuildSourceInventoryPrincipalRowSet(input SourceInventoryPrincipalRowSetInput) SourceInventoryPrincipalRowSet {
	observation := CloneSourceInventoryObservation(input.Observation)
	if !observation.IsActive() {
		return SourceInventoryPrincipalRowSet{}
	}
	roles := sourceInventoryRowSetPrincipalRoles(input.RequestModel, observation)
	if len(roles) == 0 {
		return SourceInventoryPrincipalRowSet{}
	}
	roleSet := map[AnswerCandidateRole]bool{}
	for _, role := range roles {
		roleSet[role] = true
	}
	scope, repoWide := sourceInventoryRowSetPrincipalScope(input.RequestModel)
	var principal, support, audit []SourceInventoryRow
	for _, set := range observation.Sets {
		for _, member := range set.Members {
			row := sourceInventoryRowFromMember(member, set.Role, roleSet, scope)
			switch row.Lane {
			case SourceInventoryRowLanePrincipal:
				principal = append(principal, row)
			case SourceInventoryRowLaneSupport:
				support = append(support, row)
			case SourceInventoryRowLaneAudit:
				audit = append(audit, row)
			}
		}
	}
	principal = sourceInventoryFamilyBalancedRows(principal)
	principal, support = sourceInventoryFilterRowsToRequestedSurfaceFamilies(input.RequestModel, principal, support)
	support = sourceInventoryStableRows(support)
	audit = sourceInventoryStableRows(audit)
	out := SourceInventoryPrincipalRowSet{
		Active:            len(principal) > 0 || len(support) > 0 || len(audit) > 0,
		Scopes:            sourceInventoryFollowupScopes(observation.Scopes),
		PrincipalRoles:    roles,
		PrincipalScope:    scope,
		RepoWidePrincipal: repoWide,
		SourceClasses:     cloneSourceInventorySourceClassCounts(observation.SourceClasses),
		PrincipalTotal:    len(principal),
		SupportTotal:      len(support),
		AuditTotal:        len(audit),
	}
	out.PrincipalRows, out.PrincipalHiddenCount = sourceInventoryLimitRows(principal, input.MaxPrincipalRows)
	out.SupportRows, out.SupportHiddenCount = sourceInventoryLimitRows(support, input.MaxSupportRows)
	out.AuditRows, out.AuditHiddenCount = sourceInventoryLimitRows(audit, input.MaxAuditRows)
	if !out.Active {
		return SourceInventoryPrincipalRowSet{}
	}
	return out
}

func sourceInventoryRowSetPrincipalRoles(rm RequestModel, observation SourceInventoryObservation) []AnswerCandidateRole {
	if rm.SourceInventoryProfile != nil && rm.SourceInventoryProfile.Active() {
		if roles := rm.SourceInventoryProfile.PrincipalTargetRoles(); len(roles) > 0 {
			return normalizeSourceInventoryFollowupRoles(roles)
		}
		return normalizeSourceInventoryFollowupRoles(rm.SourceInventoryProfile.TargetRoles)
	}
	var roles []AnswerCandidateRole
	for _, set := range observation.Sets {
		if set.Role != "" && set.Role != AnswerCandidateRoleUnknown {
			roles = append(roles, set.Role)
		}
	}
	return normalizeSourceInventoryFollowupRoles(roles)
}

func sourceInventoryRowSetPrincipalScope(rm RequestModel) (SourceScope, bool) {
	if SourceInventoryRequiresRepoWideLens(rm) {
		return SourceScopeAll, true
	}
	if rm.SourceScopeProfile == nil {
		return SourceScopeProduction, false
	}
	if rm.SourceScopeProfile.IncludeAuxiliaryAsPrincipal {
		return SourceScopeAll, true
	}
	switch rm.SourceScopeProfile.RequestedScope {
	case SourceScopeAll:
		return SourceScopeAll, true
	case SourceScopeTest, SourceScopeDocumentation, SourceScopeAuxiliary, SourceScopeProduction:
		return rm.SourceScopeProfile.RequestedScope, false
	default:
		return SourceScopeProduction, false
	}
}

func sourceInventoryRowFromMember(
	member SourceInventoryObservationMember,
	setRole AnswerCandidateRole,
	principalRoles map[AnswerCandidateRole]bool,
	scope SourceScope,
) SourceInventoryRow {
	role := member.Role
	if role == "" || role == AnswerCandidateRoleUnknown {
		role = setRole
		member.Role = role
	}
	class := ClassifySourcePathRole(member.File)
	language := strings.ToLower(strings.TrimSpace(member.Language))
	row := SourceInventoryRow{
		SourceClass: class,
		Role:        role,
		Language:    language,
		Member:      cloneSourceInventoryObservationMember(member),
	}
	if !principalRoles[role] {
		row.Lane = SourceInventoryRowLaneAudit
		row.ReasonCode = SourceInventoryRowReasonNonPrincipal
		return row
	}
	if strings.TrimSpace(member.File) == "" && strings.TrimSpace(member.SupportRef) == "" {
		row.Lane = SourceInventoryRowLaneSupport
		row.ReasonCode = SourceInventoryRowReasonNoLocation
		return row
	}
	if SourceScopeAllowsPathRole(scope, class) {
		row.Lane = SourceInventoryRowLanePrincipal
		row.ReasonCode = SourceInventoryRowReasonPrincipalScope
		return row
	}
	row.Lane = SourceInventoryRowLaneSupport
	row.ReasonCode = SourceInventoryRowReasonSupportScope
	return row
}

func cloneSourceInventoryObservationMember(member SourceInventoryObservationMember) SourceInventoryObservationMember {
	out := member
	out.Provenance = append([]string(nil), member.Provenance...)
	out.SurfaceTerms = append([]string(nil), member.SurfaceTerms...)
	out.Attributes = append([]SourceInventoryObservationAttribute(nil), member.Attributes...)
	for i := range out.Attributes {
		out.Attributes[i].SurfaceTerms = append([]string(nil), member.Attributes[i].SurfaceTerms...)
	}
	return out
}

func sourceInventoryFamilyBalancedRows(rows []SourceInventoryRow) []SourceInventoryRow {
	if len(rows) <= 1 {
		return append([]SourceInventoryRow(nil), rows...)
	}
	ordered := sourceInventoryStableRows(rows)
	groups := map[sourceInventoryRowFamilyKey][]SourceInventoryRow{}
	var groupOrder []sourceInventoryRowFamilyKey
	seenGroup := map[sourceInventoryRowFamilyKey]bool{}
	for _, row := range ordered {
		key := sourceInventoryRowFamily(row)
		if !seenGroup[key] {
			seenGroup[key] = true
			groupOrder = append(groupOrder, key)
		}
		groups[key] = append(groups[key], row)
	}
	out := make([]SourceInventoryRow, 0, len(ordered))
	for {
		added := false
		for _, key := range groupOrder {
			group := groups[key]
			if len(group) == 0 {
				continue
			}
			out = append(out, group[0])
			groups[key] = group[1:]
			added = true
		}
		if !added {
			break
		}
	}
	return out
}

func sourceInventoryStableRows(rows []SourceInventoryRow) []SourceInventoryRow {
	out := append([]SourceInventoryRow(nil), rows...)
	sort.SliceStable(out, func(i, j int) bool {
		return sourceInventoryRowSortKey(out[i]) < sourceInventoryRowSortKey(out[j])
	})
	return out
}

func sourceInventoryRowSortKey(row SourceInventoryRow) string {
	member := row.Member
	return strings.Join([]string{
		string(row.Role),
		string(row.SourceClass),
		row.Language,
		strings.TrimSpace(member.File),
		strings.TrimSpace(member.Name),
		strings.TrimSpace(member.Key),
		strings.TrimSpace(member.SupportRef),
	}, "\x00")
}

func sourceInventoryRowFamily(row SourceInventoryRow) sourceInventoryRowFamilyKey {
	class := row.SourceClass
	if class == SourcePathRoleUnknown {
		class = SourcePathRoleProduction
	}
	language := strings.ToLower(strings.TrimSpace(row.Language))
	if language == "" {
		language = "unknown"
	}
	return sourceInventoryRowFamilyKey{
		role:        row.Role,
		sourceClass: class,
		language:    language,
	}
}

func sourceInventoryLimitRows(rows []SourceInventoryRow, maxRows int) ([]SourceInventoryRow, int) {
	if len(rows) == 0 {
		return nil, 0
	}
	if maxRows <= 0 || maxRows >= len(rows) {
		return append([]SourceInventoryRow(nil), rows...), 0
	}
	return append([]SourceInventoryRow(nil), rows[:maxRows]...), len(rows) - maxRows
}
