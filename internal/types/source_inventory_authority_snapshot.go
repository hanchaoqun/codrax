package types

import (
	"path"
	"strings"
)

// SourceInventoryAuthoritySnapshot is the single read-side authority view for
// source-inventory convergence. It folds the durable observation, typed request
// profile, principal row-set, completion authority, and landing/citation
// readiness into one structured artifact so downstream gates do not each
// rediscover a partial "answer-critical universe".
type SourceInventoryAuthoritySnapshot struct {
	Active                       bool                               `json:"active,omitempty"`
	PrincipalAuthority           bool                               `json:"principal_authority,omitempty"`
	SupportOnly                  bool                               `json:"support_only,omitempty"`
	MechanicalRowsOnly           bool                               `json:"mechanical_rows_only,omitempty"`
	LensExecuted                 bool                               `json:"lens_executed,omitempty"`
	RepoWideRequired             bool                               `json:"repo_wide_required,omitempty"`
	ObservationComplete          bool                               `json:"observation_complete,omitempty"`
	RequiredFileCount            int                                `json:"required_file_count,omitempty"`
	RequiredFilesCovered         bool                               `json:"required_files_covered,omitempty"`
	PrincipalAggregateFactCount  int                                `json:"principal_aggregate_fact_count,omitempty"`
	CanUseMechanicalRowsForCite  bool                               `json:"can_use_mechanical_rows_for_citation,omitempty"`
	CanEnterMechanicalLanding    bool                               `json:"can_enter_mechanical_landing,omitempty"`
	NeedsFollowup                bool                               `json:"needs_followup,omitempty"`
	ReasonCodes                  []string                           `json:"reason_codes,omitempty"`
	RequestedFields              []SourceInventoryRequestedField    `json:"requested_fields,omitempty"`
	PrincipalRoles               []AnswerCandidateRole              `json:"principal_roles,omitempty"`
	PrincipalScope               SourceScope                        `json:"principal_scope,omitempty"`
	CompletionAuthority          SourceInventoryCompletionAuthority `json:"completion_authority,omitempty"`
	FollowupDebt                 SourceInventoryFollowupDebt        `json:"followup_debt,omitempty"`
	PrincipalRowSet              SourceInventoryPrincipalRowSet     `json:"principal_row_set,omitempty"`
	ProjectedPrincipalAggregates []AnswerAggregateFact              `json:"projected_principal_aggregates,omitempty"`
	SourceClasses                []SourceInventorySourceClassCount  `json:"source_classes,omitempty"`
	RepoLanguages                []SourceInventoryLanguageCount     `json:"repo_languages,omitempty"`
}

type SourceInventoryAuthoritySnapshotInput struct {
	Observation               SourceInventoryObservation `json:"observation,omitempty"`
	RequestModel              RequestModel               `json:"request_model,omitempty"`
	ExistingAggregateFacts    []AnswerAggregateFact      `json:"existing_aggregate_facts,omitempty"`
	AcceptedExactUniverse     bool                       `json:"accepted_exact_universe,omitempty"`
	AcceptedRequestedUniverse bool                       `json:"accepted_requested_universe,omitempty"`
	RequiredFiles             []string                   `json:"required_files,omitempty"`
	MaxPrincipalRows          int                        `json:"max_principal_rows,omitempty"`
	MaxSupportRows            int                        `json:"max_support_rows,omitempty"`
	MaxAuditRows              int                        `json:"max_audit_rows,omitempty"`
}

func BuildSourceInventoryAuthoritySnapshot(input SourceInventoryAuthoritySnapshotInput) SourceInventoryAuthoritySnapshot {
	observation := CloneSourceInventoryObservation(input.Observation)
	rm := input.RequestModel
	profile := rm.SourceInventoryProfile
	completion := BuildSourceInventoryCompletionAuthorityWithOptions(observation, rm, SourceInventoryCompletionAuthorityOptions{
		AcceptedExactUniverse:     input.AcceptedExactUniverse,
		AcceptedRequestedUniverse: input.AcceptedRequestedUniverse,
	})
	rowSet := BuildSourceInventoryPrincipalRowSet(SourceInventoryPrincipalRowSetInput{
		Observation:      observation,
		RequestModel:     rm,
		MaxPrincipalRows: sourceInventorySnapshotLimit(input.MaxPrincipalRows),
		MaxSupportRows:   sourceInventorySnapshotLimit(input.MaxSupportRows),
		MaxAuditRows:     sourceInventorySnapshotLimit(input.MaxAuditRows),
	})
	projected := ProjectSourceInventoryPrincipalRowSetAggregateFacts(input.ExistingAggregateFacts, observation, rm)
	principalRefs := PrincipalAggregateMemberSetFactRefsForRequest(projected, &rm)
	var followup SourceInventoryFollowupDebt
	if completion.IsBlocking() {
		followup = DeriveSourceInventoryFollowupDebtWithRequiredFiles(observation, rm, input.RequiredFiles)
	}
	requiredFiles := sourceInventorySnapshotRequiredFileSet(input.RequiredFiles)
	requiredCovered := sourceInventorySnapshotRequiredFilesCovered(observation, requiredFiles)

	out := SourceInventoryAuthoritySnapshot{
		Active:                       observation.IsActive() || SourceInventoryPrincipalAuthorityActive(rm),
		PrincipalAuthority:           SourceInventoryPrincipalAuthorityActive(rm),
		SupportOnly:                  SourceInventoryCompletionIsSupportOnly(rm),
		LensExecuted:                 SourceInventoryLensExecuted(observation),
		RepoWideRequired:             SourceInventoryRequiresRepoWideLens(rm),
		ObservationComplete:          observation.Complete,
		RequiredFileCount:            len(requiredFiles),
		RequiredFilesCovered:         requiredCovered,
		PrincipalAggregateFactCount:  len(principalRefs),
		CanUseMechanicalRowsForCite:  false,
		CanEnterMechanicalLanding:    false,
		NeedsFollowup:                completion.IsBlocking(),
		RequestedFields:              cloneSourceInventoryRequestedFields(nil),
		PrincipalRoles:               normalizeSourceInventoryFollowupRoles(rowSet.PrincipalRoles),
		PrincipalScope:               rowSet.PrincipalScope,
		CompletionAuthority:          completion,
		FollowupDebt:                 followup,
		PrincipalRowSet:              rowSet,
		ProjectedPrincipalAggregates: append([]AnswerAggregateFact(nil), projected...),
		SourceClasses:                cloneSourceInventorySourceClassCounts(observation.SourceClasses),
		RepoLanguages:                cloneSourceInventoryLanguageCounts(observation.RepoLanguages),
	}
	if profile != nil {
		out.MechanicalRowsOnly = profile.MechanicalRowsOnly()
		out.RequestedFields = cloneSourceInventoryRequestedFields(profile.RequestedFields)
	}
	out.CanUseMechanicalRowsForCite = out.PrincipalAuthority &&
		out.MechanicalRowsOnly &&
		out.RequiredFilesCovered &&
		out.PrincipalAggregateFactCount > 0 &&
		!out.NeedsFollowup
	out.CanEnterMechanicalLanding = out.CanUseMechanicalRowsForCite &&
		out.LensExecuted &&
		!out.SupportOnly
	out.ReasonCodes = sourceInventorySnapshotReasonCodes(out)
	return out
}

func sourceInventorySnapshotLimit(n int) int {
	if n > 0 {
		return n
	}
	return 32
}

func cloneSourceInventoryRequestedFields(in []SourceInventoryRequestedField) []SourceInventoryRequestedField {
	if len(in) == 0 {
		return nil
	}
	out := make([]SourceInventoryRequestedField, 0, len(in))
	seen := map[SourceInventoryRequestedField]bool{}
	for _, field := range in {
		if field == "" || !field.IsValid() || seen[field] {
			continue
		}
		seen[field] = true
		out = append(out, field)
	}
	return out
}

func sourceInventorySnapshotRequiredFilesCovered(observation SourceInventoryObservation, required map[string]bool) bool {
	if len(required) == 0 {
		return true
	}
	covered := map[string]bool{}
	for _, set := range observation.Sets {
		for _, member := range set.Members {
			sourceInventorySnapshotMarkCovered(covered, member.File)
			for _, attr := range member.Attributes {
				sourceInventorySnapshotMarkCovered(covered, attr.File)
			}
		}
	}
	for _, lens := range observation.CompleteLenses {
		if lens.Count != 0 {
			continue
		}
		for _, scope := range lens.Scopes {
			scope = sourceInventorySnapshotCleanPath(scope)
			if scope == "" {
				continue
			}
			for file := range required {
				if sourceInventorySnapshotPathCoveredByScope(file, scope) {
					covered[file] = true
				}
			}
		}
	}
	for file := range required {
		if !covered[file] {
			return false
		}
	}
	return true
}

func sourceInventorySnapshotRequiredFileSet(files []string) map[string]bool {
	out := map[string]bool{}
	for _, file := range files {
		file = sourceInventorySnapshotCleanPath(file)
		if file != "" && file != "." {
			out[file] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func sourceInventorySnapshotMarkCovered(covered map[string]bool, file string) {
	file = sourceInventorySnapshotCleanPath(file)
	if file != "" {
		covered[file] = true
	}
}

func sourceInventorySnapshotCleanPath(raw string) string {
	clean := strings.TrimSpace(strings.ReplaceAll(raw, `\`, `/`))
	clean = path.Clean(clean)
	clean = strings.Trim(strings.ReplaceAll(clean, `\`, `/`), "/")
	if clean == "" {
		return ""
	}
	return clean
}

func sourceInventorySnapshotPathCoveredByScope(file, scope string) bool {
	file = sourceInventorySnapshotCleanPath(file)
	scope = sourceInventorySnapshotCleanPath(scope)
	if file == "" || scope == "" {
		return false
	}
	return scope == "." || file == scope || strings.HasPrefix(file, scope+"/")
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
