package types

import "strings"

// SourceInventoryCitationObligation is the answer-facing citation surface for
// one source-inventory principal row. It is derived only from typed row fields
// and support refs; consumers must not reconstruct this from rendered tables or
// model prose.
type SourceInventoryCitationObligation struct {
	Member      string              `json:"member,omitempty"`
	SupportRef  string              `json:"support_ref,omitempty"`
	File        string              `json:"file,omitempty"`
	Line        int                 `json:"line,omitempty"`
	Role        AnswerCandidateRole `json:"role,omitempty"`
	SourceClass SourcePathRole      `json:"source_class,omitempty"`
	Language    string              `json:"language,omitempty"`
}

// SourceInventoryAnswerAuthorityView is the single answer-facing projection of
// SourceInventoryAuthoritySnapshot. It gives completion gates, finalizer
// prompts, pre-emit checks, status cards, and eval telemetry the same typed
// semantics instead of letting each layer reinterpret the snapshot.
type SourceInventoryAnswerAuthorityView struct {
	Active                               bool                                 `json:"active,omitempty"`
	CanBlockCompletion                   bool                                 `json:"can_block_completion,omitempty"`
	CanOnlyCaveat                        bool                                 `json:"can_only_caveat,omitempty"`
	NeedsFollowup                        bool                                 `json:"needs_followup,omitempty"`
	CompletionReasonCode                 string                               `json:"completion_reason_code,omitempty"`
	FollowupDebt                         SourceInventoryFollowupDebt          `json:"followup_debt,omitempty"`
	CanUseMechanicalRowsForCitation      bool                                 `json:"can_use_mechanical_rows_for_citation,omitempty"`
	CanEnterMechanicalLanding            bool                                 `json:"can_enter_mechanical_landing,omitempty"`
	PrincipalAuthority                   bool                                 `json:"principal_authority,omitempty"`
	PrincipalRoles                       []AnswerCandidateRole                `json:"principal_roles,omitempty"`
	PrincipalScope                       SourceScope                          `json:"principal_scope,omitempty"`
	RepoWidePrincipal                    bool                                 `json:"repo_wide_principal,omitempty"`
	SourceClasses                        []SourceInventorySourceClassCount    `json:"source_classes,omitempty"`
	PrincipalTotal                       int                                  `json:"principal_total,omitempty"`
	SupportTotal                         int                                  `json:"support_total,omitempty"`
	AuditTotal                           int                                  `json:"audit_total,omitempty"`
	PrincipalHiddenCount                 int                                  `json:"principal_hidden_count,omitempty"`
	SupportHiddenCount                   int                                  `json:"support_hidden_count,omitempty"`
	AuditHiddenCount                     int                                  `json:"audit_hidden_count,omitempty"`
	PrincipalRows                        []SourceInventoryRow                 `json:"principal_rows,omitempty"`
	SupportRows                          []SourceInventoryRow                 `json:"support_rows,omitempty"`
	AuditRows                            []SourceInventoryRow                 `json:"audit_rows,omitempty"`
	RequestedUniverse                    SourceInventoryRequestedUniverseView `json:"requested_universe,omitempty"`
	InventoryOutOfUniverseRowsSuppressed int                                  `json:"inventory_out_of_universe_rows_suppressed,omitempty"`
	CitationObligations                  []SourceInventoryCitationObligation  `json:"citation_obligations,omitempty"`
	ReasonCodes                          []string                             `json:"reason_codes,omitempty"`
	CompletionRequiresExactProof         bool                                 `json:"completion_requires_exact_proof,omitempty"`
	CompletionAcceptedExactUniverse      bool                                 `json:"completion_accepted_exact_universe,omitempty"`
	CompletionAcceptedRequestedUniverse  bool                                 `json:"completion_accepted_requested_universe,omitempty"`
}

func BuildSourceInventoryAnswerAuthorityView(snapshot SourceInventoryAuthoritySnapshot) SourceInventoryAnswerAuthorityView {
	snapshot = NormalizeSourceInventoryAuthoritySnapshot(snapshot)
	completion := NormalizeSourceInventoryCompletionAuthority(snapshot.CompletionAuthority)
	rowSet := normalizeSourceInventoryAnswerAuthorityRowSet(snapshot.PrincipalRowSet)
	canOnlyCaveat := SourceInventoryCompletionAuthorityCanOnlyCaveat(completion)
	out := SourceInventoryAnswerAuthorityView{
		Active:                               snapshot.Active || rowSet.Active || completion.Active,
		CanBlockCompletion:                   completion.Blocking && !canOnlyCaveat,
		CanOnlyCaveat:                        canOnlyCaveat,
		NeedsFollowup:                        snapshot.NeedsFollowup,
		CompletionReasonCode:                 completion.ReasonCode,
		FollowupDebt:                         NormalizeSourceInventoryFollowupDebt(snapshot.FollowupDebt),
		CanUseMechanicalRowsForCitation:      snapshot.CanUseMechanicalRowsForCite,
		CanEnterMechanicalLanding:            snapshot.CanEnterMechanicalLanding,
		PrincipalAuthority:                   snapshot.PrincipalAuthority,
		PrincipalRoles:                       normalizeSourceInventoryFollowupRoles(rowSet.PrincipalRoles),
		PrincipalScope:                       rowSet.PrincipalScope,
		RepoWidePrincipal:                    rowSet.RepoWidePrincipal,
		SourceClasses:                        cloneSourceInventorySourceClassCounts(rowSet.SourceClasses),
		PrincipalTotal:                       rowSet.PrincipalTotal,
		SupportTotal:                         rowSet.SupportTotal,
		AuditTotal:                           rowSet.AuditTotal,
		PrincipalHiddenCount:                 rowSet.PrincipalHiddenCount,
		SupportHiddenCount:                   rowSet.SupportHiddenCount,
		AuditHiddenCount:                     rowSet.AuditHiddenCount,
		PrincipalRows:                        append([]SourceInventoryRow(nil), rowSet.PrincipalRows...),
		SupportRows:                          append([]SourceInventoryRow(nil), rowSet.SupportRows...),
		AuditRows:                            append([]SourceInventoryRow(nil), rowSet.AuditRows...),
		RequestedUniverse:                    snapshot.RequestedUniverse,
		InventoryOutOfUniverseRowsSuppressed: snapshot.RequestedUniverse.InventoryOutOfUniverseRowsSuppressed,
		CitationObligations:                  sourceInventoryCitationObligations(rowSet),
		ReasonCodes:                          sourceInventorySnapshotUniqueStrings(snapshot.ReasonCodes),
		CompletionRequiresExactProof:         completion.RequiresExactUniverseProof,
		CompletionAcceptedExactUniverse:      completion.AcceptedExactUniverse,
		CompletionAcceptedRequestedUniverse:  completion.AcceptedRequestedUniverse,
	}
	if out.FollowupDebt.IsActive() {
		out.FollowupDebt = NormalizeSourceInventoryFollowupDebt(out.FollowupDebt)
	}
	return out
}

func normalizeSourceInventoryAnswerAuthorityRowSet(rowSet SourceInventoryPrincipalRowSet) SourceInventoryPrincipalRowSet {
	rowSet.Scopes = sourceInventoryFollowupScopes(rowSet.Scopes)
	rowSet.PrincipalRoles = normalizeSourceInventoryFollowupRoles(rowSet.PrincipalRoles)
	rowSet.SourceClasses = cloneSourceInventorySourceClassCounts(rowSet.SourceClasses)
	rowSet.PrincipalRows = append([]SourceInventoryRow(nil), rowSet.PrincipalRows...)
	rowSet.SupportRows = append([]SourceInventoryRow(nil), rowSet.SupportRows...)
	rowSet.AuditRows = append([]SourceInventoryRow(nil), rowSet.AuditRows...)
	rowSet.PrincipalTotal = maxSourceInventoryObservationTotal(rowSet.PrincipalTotal, len(rowSet.PrincipalRows))
	rowSet.SupportTotal = maxSourceInventoryObservationTotal(rowSet.SupportTotal, len(rowSet.SupportRows))
	rowSet.AuditTotal = maxSourceInventoryObservationTotal(rowSet.AuditTotal, len(rowSet.AuditRows))
	rowSet.Active = rowSet.Active && rowSet.PrincipalTotal > 0
	return rowSet
}

// SourceInventoryCompletionAuthorityCanOnlyCaveat reports whether the
// completion authority is observing non-precise navigation debt. Such debt can
// be preserved as a typed caveat, but must not hard-block investigation closure.
func SourceInventoryCompletionAuthorityCanOnlyCaveat(authority SourceInventoryCompletionAuthority) bool {
	authority = NormalizeSourceInventoryCompletionAuthority(authority)
	if !authority.Blocking {
		return false
	}
	if SourceInventoryCompletionAuthorityHasExecutableMissingClass(authority) {
		return false
	}
	switch authority.ReasonCode {
	case "", SourceInventoryCompletionReasonIncompleteObservation, SourceInventoryCompletionReasonFollowupDebt:
		return true
	default:
		return false
	}
}

func SourceInventoryCompletionAuthorityHasExecutableMissingClass(authority SourceInventoryCompletionAuthority) bool {
	debt := NormalizeSourceInventoryFollowupDebt(authority.FollowupDebt)
	return debt.IsActive() &&
		debt.ReasonCode == SourceInventoryFollowupDebtMissingSourceClass &&
		len(debt.Query.Scopes) > 0 &&
		len(debt.MissingClasses) > 0 &&
		len(debt.CoveredClasses) > 0 &&
		len(debt.MissingLanguages) > 0
}

func sourceInventoryCitationObligations(rowSet SourceInventoryPrincipalRowSet) []SourceInventoryCitationObligation {
	rowSet = NormalizeSourceInventoryPrincipalRowSet(rowSet)
	if !rowSet.Active {
		return nil
	}
	out := make([]SourceInventoryCitationObligation, 0, len(rowSet.PrincipalRows))
	for _, row := range rowSet.PrincipalRows {
		member := sourceInventoryPrincipalRowMemberLabel(row)
		if member == "" {
			continue
		}
		supportRef := sourceInventoryPrincipalRowSupportRef(row, member)
		file := strings.TrimSpace(strings.ReplaceAll(row.Member.File, `\`, `/`))
		line := row.Member.Line
		if file == "" || line <= 0 {
			if _, loc, ok := ParseAnswerSupportRefMemberLocation(supportRef); ok {
				if file == "" {
					file = strings.TrimSpace(strings.ReplaceAll(loc.File, `\`, `/`))
				}
				if line <= 0 {
					line = loc.LineStart
				}
			}
		}
		out = append(out, SourceInventoryCitationObligation{
			Member:      member,
			SupportRef:  supportRef,
			File:        file,
			Line:        line,
			Role:        row.Role,
			SourceClass: row.SourceClass,
			Language:    strings.ToLower(strings.TrimSpace(row.Language)),
		})
	}
	return out
}
