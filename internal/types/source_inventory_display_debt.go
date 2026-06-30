package types

import "strings"

const (
	SourceInventoryRefinementReasonCandidateBudgetTruncated = "source_inventory_candidate_budget_truncated"
	SourceInventoryRefinementReasonPageIncomplete           = "source_inventory_page_incomplete"
)

// SourceInventoryAuthoritySuppressesStaleDisplayDebt reports whether the
// source-inventory authority has already accepted the requested or exact
// universe, so older broad-lens refinement hints should stay in audit logs
// instead of prompting answer caveats or another user-visible validation pass.
func SourceInventoryAuthoritySuppressesStaleDisplayDebt(snapshot SourceInventoryAuthoritySnapshot) bool {
	view := BuildSourceInventoryAnswerAuthorityView(snapshot)
	if !view.Active || view.NeedsFollowup || view.CanBlockCompletion {
		return false
	}
	authority := NormalizeSourceInventoryCompletionAuthority(snapshot.CompletionAuthority)
	if authority.AcceptedExactUniverse || authority.AcceptedRequestedUniverse {
		return true
	}
	switch authority.ReasonCode {
	case SourceInventoryCompletionReasonAcceptedExactUniverse, SourceInventoryCompletionReasonAcceptedRequestedUniverse:
		return true
	default:
		return false
	}
}

// StripSourceInventoryStaleDisplayDebtFromToolResult returns a prompt-facing
// copy of result with stale source-inventory refinement debt removed. It never
// mutates the underlying ledger result, and keeps observation/evidence refs so
// downstream stages can still see the typed facts.
func StripSourceInventoryStaleDisplayDebtFromToolResult(result ToolResult) ToolResult {
	if result.Refinement != nil && ToolRefinementIsSourceInventoryDisplayDebt(*result.Refinement, result.ToolName, result.Observations, result.Handoff) {
		result.Refinement = nil
	}
	if result.Handoff != nil {
		carrier := StripSourceInventoryStaleDisplayDebtFromToolHandoffCarrier(*result.Handoff)
		if carrier.Empty() {
			result.Handoff = nil
		} else {
			result.Handoff = &carrier
		}
	}
	return result
}

// StripSourceInventoryStaleDisplayDebtFromToolHandoffCarrier removes only the
// stale refinement/action fields. Accepted evidence and observation refs remain
// available for finalization and audit.
func StripSourceInventoryStaleDisplayDebtFromToolHandoffCarrier(carrier ToolHandoffCarrier) ToolHandoffCarrier {
	carrier = NormalizeToolHandoffCarrier(carrier)
	if carrier.Refinement != nil && ToolRefinementIsSourceInventoryDisplayDebt(*carrier.Refinement, carrier.ToolName, nil, &carrier) {
		carrier.Refinement = nil
	}
	if toolHandoffReasonIsSourceInventoryDisplayDebt(carrier.ReasonCode) {
		carrier.ReasonCode = ""
	}
	if carrier.ReasonCode == "" {
		switch {
		case carrier.RepairCode != "":
			carrier.ReasonCode = carrier.RepairCode
		case carrier.Refinement != nil:
			carrier.ReasonCode = carrier.Refinement.ReasonCode
		case len(carrier.ObservationRefs) > 0:
			carrier.ReasonCode = "tool_observation_handoff"
		case len(carrier.AcceptedEvidence) > 0:
			carrier.ReasonCode = "accepted_evidence_handoff"
		}
	}
	return NormalizeToolHandoffCarrier(carrier)
}

func ToolRefinementIsSourceInventoryDisplayDebt(
	refinement ToolRefinementHint,
	toolName string,
	observations []ObservationRecord,
	carrier *ToolHandoffCarrier,
) bool {
	refinement = NormalizeToolRefinementHint(refinement)
	if refinement.Empty() {
		return false
	}
	if toolHandoffReasonIsSourceInventoryDisplayDebt(refinement.ReasonCode) {
		return true
	}
	if strings.TrimSpace(toolName) != "repo_map" {
		return false
	}
	if sourceInventoryPreferredParams(refinement.PreferredParams) {
		return refinement.CandidateBudgetTruncated || refinement.NextCursor != ""
	}
	if carrier != nil && toolHandoffCarrierHasSourceInventoryObservation(*carrier) {
		return refinement.CandidateBudgetTruncated || refinement.NextCursor != ""
	}
	return toolResultObservationsHaveSourceInventory(observations) &&
		(refinement.CandidateBudgetTruncated || refinement.NextCursor != "")
}

func toolHandoffReasonIsSourceInventoryDisplayDebt(reason string) bool {
	switch strings.TrimSpace(reason) {
	case SourceInventoryRefinementReasonCandidateBudgetTruncated,
		SourceInventoryRefinementReasonPageIncomplete,
		"candidate_budget_truncated":
		return true
	default:
		return false
	}
}

func sourceInventoryPreferredParams(params map[string]string) bool {
	for key, value := range params {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "view" && value == "source_inventory" {
			return true
		}
	}
	return false
}

func toolHandoffCarrierHasSourceInventoryObservation(carrier ToolHandoffCarrier) bool {
	for _, ref := range carrier.ObservationRefs {
		if toolObservationRefIsSourceInventory(ref) {
			return true
		}
	}
	return false
}

func toolResultObservationsHaveSourceInventory(observations []ObservationRecord) bool {
	for _, observation := range observations {
		if strings.TrimSpace(observation.ClaimKey) == "source_inventory" ||
			strings.TrimSpace(observation.Value) == string(RepoMapNavigationRouteSourceInventory) ||
			strings.TrimSpace(observation.Predicate) == RepoMapNavigationObservationPredicate {
			return true
		}
	}
	return false
}

func toolObservationRefIsSourceInventory(ref ToolObservationRef) bool {
	claim := strings.TrimSpace(ref.ClaimKey)
	return claim == "source_inventory" || strings.HasPrefix(claim, "source_inventory:")
}
