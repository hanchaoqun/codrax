package types

import (
	"path"
	"strings"
)

const (
	SourceInventoryFollowupDebtMissingSourceClass = "missing_source_class_family"
	SourceInventoryFollowupDebtPagination         = "pagination_debt"

	sourceInventoryFollowupMaxScopes = 8
)

type SourceInventoryFollowupDebt struct {
	Active           bool                     `json:"active,omitempty"`
	ReasonCode       string                   `json:"reason_code,omitempty"`
	Query            SourceInventoryLensQuery `json:"query,omitempty"`
	MissingClasses   []SourcePathRole         `json:"missing_classes,omitempty"`
	CoveredClasses   []SourcePathRole         `json:"covered_classes,omitempty"`
	MissingLanguages []string                 `json:"missing_languages,omitempty"`
	Roles            []AnswerCandidateRole    `json:"roles,omitempty"`
}

func (d SourceInventoryFollowupDebt) IsActive() bool {
	return NormalizeSourceInventoryFollowupDebt(d).Active
}

func NormalizeSourceInventoryFollowupDebt(d SourceInventoryFollowupDebt) SourceInventoryFollowupDebt {
	d.ReasonCode = strings.TrimSpace(d.ReasonCode)
	d.Query = normalizeSourceInventoryFollowupQuery(d.Query)
	d = normalizeSourceInventoryFollowupCursor(d)
	d.MissingClasses = normalizeSourceInventoryPathRoles(d.MissingClasses)
	d.CoveredClasses = normalizeSourceInventoryPathRoles(d.CoveredClasses)
	d.MissingLanguages = normalizeSourceInventoryFollowupStrings(d.MissingLanguages)
	d.Roles = normalizeSourceInventoryFollowupRoles(d.Roles)
	d.Active = d.Active && d.ReasonCode != "" && len(d.Query.Roles) > 0
	return d
}

func DeriveSourceInventoryFollowupDebt(observation SourceInventoryObservation, rm RequestModel) SourceInventoryFollowupDebt {
	if !observation.IsActive() || !sourceInventoryObservationBudgetDebt(observation) {
		return SourceInventoryFollowupDebt{}
	}
	roles := sourceInventoryFollowupPrincipalRoles(observation, rm)
	if len(roles) == 0 {
		return SourceInventoryFollowupDebt{}
	}
	covered := sourceInventoryObservedPathRoles(observation)
	missingClasses, scopes := sourceInventoryMissingClassScopes(observation.SourceClasses, covered)
	query := SourceInventoryLensQuery{
		Path:              ".",
		Scopes:            scopes,
		Roles:             roles,
		IncludeCounts:     true,
		IncludeAttributes: false,
		TopN:              24,
	}
	reason := SourceInventoryFollowupDebtMissingSourceClass
	if len(missingClasses) == 0 {
		reason = SourceInventoryFollowupDebtPagination
		query.Scopes = sourceInventoryFollowupScopes(observation.Scopes)
		if observation.Page != nil {
			query.Cursor = strings.TrimSpace(observation.Page.NextCursor)
			query.Offset = observation.Page.Offset + observation.Page.Emitted
		}
	}
	return NormalizeSourceInventoryFollowupDebt(SourceInventoryFollowupDebt{
		Active:           true,
		ReasonCode:       reason,
		Query:            query,
		MissingClasses:   missingClasses,
		CoveredClasses:   sourceInventoryCoveredClassList(covered),
		MissingLanguages: sourceInventoryMissingLanguages(observation),
		Roles:            roles,
	})
}

func sourceInventoryObservationBudgetDebt(observation SourceInventoryObservation) bool {
	if observation.Execution != nil && observation.Execution.CandidateBudgetTruncated {
		return true
	}
	return observation.Page != nil && (!observation.Page.Complete || strings.TrimSpace(observation.Page.NextCursor) != "")
}

func sourceInventoryFollowupPrincipalRoles(observation SourceInventoryObservation, rm RequestModel) []AnswerCandidateRole {
	var roles []AnswerCandidateRole
	if rm.SourceInventoryProfile != nil && rm.SourceInventoryProfile.Active() {
		roles = rm.SourceInventoryProfile.PrincipalTargetRoles()
		if len(roles) == 0 {
			roles = append([]AnswerCandidateRole(nil), rm.SourceInventoryProfile.TargetRoles...)
		}
	}
	if len(roles) == 0 {
		for _, set := range observation.Sets {
			if set.Role != "" && set.Role != AnswerCandidateRoleUnknown {
				roles = append(roles, set.Role)
			}
		}
	}
	return normalizeSourceInventoryFollowupRoles(roles)
}

func sourceInventoryObservedPathRoles(observation SourceInventoryObservation) map[SourcePathRole]bool {
	out := map[SourcePathRole]bool{}
	add := func(file string) {
		role := ClassifySourcePathRole(file)
		if role != SourcePathRoleUnknown {
			out[role] = true
		}
	}
	for _, set := range observation.Sets {
		for _, member := range set.Members {
			add(member.File)
			for _, attr := range member.Attributes {
				add(attr.File)
			}
		}
	}
	return out
}

func sourceInventoryMissingClassScopes(classes []SourceInventorySourceClassCount, covered map[SourcePathRole]bool) ([]SourcePathRole, []string) {
	var missing []SourcePathRole
	var scopes []string
	for _, class := range classes {
		if class.Role == SourcePathRoleUnknown || class.Count <= 0 || covered[class.Role] {
			continue
		}
		missing = append(missing, class.Role)
		for _, sample := range sourceInventoryFollowupClassSamples(class) {
			if scope := sourceInventoryFollowupScopeForSample(sample); scope != "" {
				scopes = append(scopes, scope)
			}
		}
	}
	return normalizeSourceInventoryPathRoles(missing), sourceInventoryFollowupScopes(scopes)
}

func sourceInventoryMissingLanguages(observation SourceInventoryObservation) []string {
	var out []string
	for _, lang := range observation.RepoLanguages {
		if lang.Count > 0 && strings.TrimSpace(lang.Language) != "" && !lang.InScope {
			out = append(out, lang.Language)
		}
	}
	return normalizeSourceInventoryFollowupStrings(out)
}

func sourceInventoryFollowupScopeForSample(sample string) string {
	sample = strings.Trim(strings.ReplaceAll(strings.TrimSpace(sample), `\`, `/`), "/")
	if sample == "" {
		return ""
	}
	dir := path.Dir(sample)
	if dir == "." || dir == "/" {
		return "."
	}
	return dir
}

func sourceInventoryFollowupScopes(scopes []string) []string {
	out := normalizeSourceInventoryFollowupStrings(scopes)
	if len(out) == 0 {
		return []string{"."}
	}
	if len(out) > sourceInventoryFollowupMaxScopes {
		return out[:sourceInventoryFollowupMaxScopes]
	}
	return out
}

func normalizeSourceInventoryFollowupQuery(q SourceInventoryLensQuery) SourceInventoryLensQuery {
	q.Path = strings.Trim(strings.ReplaceAll(strings.TrimSpace(q.Path), `\`, `/`), "/")
	if q.Path == "" {
		q.Path = "."
	}
	q.Scopes = sourceInventoryFollowupScopes(q.Scopes)
	q.Roles = normalizeSourceInventoryFollowupRoles(q.Roles)
	q.AttributeRoles = normalizeSourceInventoryFollowupRoles(q.AttributeRoles)
	q.Query = strings.TrimSpace(q.Query)
	q.Cursor = strings.TrimSpace(q.Cursor)
	if q.TopN < 0 {
		q.TopN = 0
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	return q
}

func normalizeSourceInventoryFollowupRoles(in []AnswerCandidateRole) []AnswerCandidateRole {
	seen := map[AnswerCandidateRole]bool{}
	var out []AnswerCandidateRole
	for _, role := range in {
		if role == "" || role == AnswerCandidateRoleUnknown || seen[role] {
			continue
		}
		seen[role] = true
		out = append(out, role)
	}
	return out
}

func normalizeSourceInventoryPathRoles(in []SourcePathRole) []SourcePathRole {
	seen := map[SourcePathRole]bool{}
	var out []SourcePathRole
	for _, role := range in {
		if role == "" || role == SourcePathRoleUnknown || seen[role] {
			continue
		}
		seen[role] = true
		out = append(out, role)
	}
	return out
}

func sourceInventoryCoveredClassList(covered map[SourcePathRole]bool) []SourcePathRole {
	var out []SourcePathRole
	for role := range covered {
		out = append(out, role)
	}
	return normalizeSourceInventoryPathRoles(out)
}
