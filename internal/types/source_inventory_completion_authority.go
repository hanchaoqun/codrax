package types

import "strings"

const (
	SourceInventoryCompletionReasonInactive                  = "inactive"
	SourceInventoryCompletionReasonComplete                  = "complete"
	SourceInventoryCompletionReasonFollowupDebt              = "followup_debt"
	SourceInventoryCompletionReasonIncompleteObservation     = "incomplete_observation"
	SourceInventoryCompletionReasonAcceptedExactUniverse     = "accepted_exact_universe"
	SourceInventoryCompletionReasonAcceptedRequestedUniverse = "accepted_requested_universe"
)

// SourceInventoryCompletionAuthority is the typed pre-complete decision view
// for source-inventory answers. It lets completion gates, repair handoff, and
// retry convergence share one structural signal instead of rebuilding the same
// decision from rendered repair text.
type SourceInventoryCompletionAuthority struct {
	Active                     bool                              `json:"active,omitempty"`
	Blocking                   bool                              `json:"blocking,omitempty"`
	ReasonCode                 string                            `json:"reason_code,omitempty"`
	AcceptedExactUniverse      bool                              `json:"accepted_exact_universe,omitempty"`
	AcceptedRequestedUniverse  bool                              `json:"accepted_requested_universe,omitempty"`
	RepoWideRequired           bool                              `json:"repo_wide_required,omitempty"`
	LensExecuted               bool                              `json:"lens_executed,omitempty"`
	ObservationComplete        bool                              `json:"observation_complete,omitempty"`
	CandidateBudgetTruncated   bool                              `json:"candidate_budget_truncated,omitempty"`
	PageIncomplete             bool                              `json:"page_incomplete,omitempty"`
	NextCursor                 string                            `json:"next_cursor,omitempty"`
	Scopes                     []string                          `json:"scopes,omitempty"`
	Roles                      []AnswerCandidateRole             `json:"roles,omitempty"`
	SourceClasses              []SourceInventorySourceClassCount `json:"source_classes,omitempty"`
	RepoLanguages              []SourceInventoryLanguageCount    `json:"repo_languages,omitempty"`
	IncompleteSets             []AnswerCandidateRole             `json:"incomplete_sets,omitempty"`
	FollowupDebt               SourceInventoryFollowupDebt       `json:"followup_debt,omitempty"`
	RequiresExactUniverseProof bool                              `json:"requires_exact_universe_proof,omitempty"`
}

func (a SourceInventoryCompletionAuthority) IsBlocking() bool {
	return NormalizeSourceInventoryCompletionAuthority(a).Blocking
}

func NormalizeSourceInventoryCompletionAuthority(a SourceInventoryCompletionAuthority) SourceInventoryCompletionAuthority {
	a.ReasonCode = strings.TrimSpace(a.ReasonCode)
	a.NextCursor = strings.TrimSpace(a.NextCursor)
	a.Scopes = sourceInventoryFollowupScopes(a.Scopes)
	a.Roles = normalizeSourceInventoryFollowupRoles(a.Roles)
	a.SourceClasses = cloneSourceInventorySourceClassCounts(a.SourceClasses)
	a.RepoLanguages = cloneSourceInventoryLanguageCounts(a.RepoLanguages)
	a.IncompleteSets = normalizeSourceInventoryFollowupRoles(a.IncompleteSets)
	a.FollowupDebt = NormalizeSourceInventoryFollowupDebt(a.FollowupDebt)
	a.Active = a.Active && a.ReasonCode != ""
	a.Blocking = a.Active && a.Blocking
	return a
}

// BuildSourceInventoryCompletionAuthority folds the durable observation, typed
// request model, and exact-universe aggregate-fact proof into the single view
// used by completion gates. The acceptedExactUniverse argument is deliberately
// precomputed by the caller because aggregate-fact coverage lives in the tool
// package; the authority itself stays import-cycle-free and reusable by read
// scheduler, write/report, and tests.
func BuildSourceInventoryCompletionAuthority(observation SourceInventoryObservation, rm RequestModel, acceptedExactUniverse bool) SourceInventoryCompletionAuthority {
	return BuildSourceInventoryCompletionAuthorityWithOptions(observation, rm, SourceInventoryCompletionAuthorityOptions{
		AcceptedExactUniverse: acceptedExactUniverse,
	})
}

type SourceInventoryCompletionAuthorityOptions struct {
	AcceptedExactUniverse     bool
	AcceptedRequestedUniverse bool
}

func BuildSourceInventoryCompletionAuthorityWithOptions(observation SourceInventoryObservation, rm RequestModel, opts SourceInventoryCompletionAuthorityOptions) SourceInventoryCompletionAuthority {
	repoWideRequired := SourceInventoryRequiresRepoWideLens(rm)
	roles := sourceInventoryCompletionRoles(observation, rm)
	scopes := sourceInventoryFollowupScopes(observation.Scopes)
	if repoWideRequired && sourceInventoryCompletionObservationIncomplete(observation) {
		scopes = []string{"."}
	}
	out := SourceInventoryCompletionAuthority{
		Active:                    SourceInventoryPrincipalAuthorityActive(rm) && observation.IsActive(),
		AcceptedExactUniverse:     opts.AcceptedExactUniverse,
		AcceptedRequestedUniverse: opts.AcceptedRequestedUniverse,
		RepoWideRequired:          repoWideRequired,
		LensExecuted:              SourceInventoryLensExecuted(observation),
		ObservationComplete:       observation.Complete,
		Scopes:                    scopes,
		Roles:                     roles,
		SourceClasses:             cloneSourceInventorySourceClassCounts(observation.SourceClasses),
		RepoLanguages:             cloneSourceInventoryLanguageCounts(observation.RepoLanguages),
		IncompleteSets:            sourceInventoryCompletionIncompleteSets(observation),
	}
	if observation.Execution != nil {
		out.CandidateBudgetTruncated = observation.Execution.CandidateBudgetTruncated
	}
	if observation.Page != nil {
		out.PageIncomplete = !observation.Page.Complete || strings.TrimSpace(observation.Page.NextCursor) != ""
		out.NextCursor = strings.TrimSpace(observation.Page.NextCursor)
	}
	if !out.Active || !out.LensExecuted {
		out.ReasonCode = SourceInventoryCompletionReasonInactive
		return NormalizeSourceInventoryCompletionAuthority(out)
	}
	if !sourceInventoryCompletionObservationIncomplete(observation) {
		out.ReasonCode = SourceInventoryCompletionReasonComplete
		return NormalizeSourceInventoryCompletionAuthority(out)
	}
	if opts.AcceptedExactUniverse && !repoWideRequired {
		out.ReasonCode = SourceInventoryCompletionReasonAcceptedExactUniverse
		return NormalizeSourceInventoryCompletionAuthority(out)
	}
	if opts.AcceptedRequestedUniverse {
		out.ReasonCode = SourceInventoryCompletionReasonAcceptedRequestedUniverse
		return NormalizeSourceInventoryCompletionAuthority(out)
	}
	out.RequiresExactUniverseProof = true
	out.Blocking = true
	out.FollowupDebt = DeriveSourceInventoryFollowupDebt(observation, rm)
	if out.FollowupDebt.IsActive() {
		out.ReasonCode = SourceInventoryCompletionReasonFollowupDebt
		return NormalizeSourceInventoryCompletionAuthority(out)
	}
	out.ReasonCode = SourceInventoryCompletionReasonIncompleteObservation
	return NormalizeSourceInventoryCompletionAuthority(out)
}
