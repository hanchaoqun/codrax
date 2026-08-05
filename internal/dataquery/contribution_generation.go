package dataquery

import "strings"

// contributionReplacementRequested is the typed admission boundary for a
// repair that supersedes, rather than appends to, the current contribution
// ledger generation. Ordinary compute actions remain additive.
func contributionReplacementRequested(action DataAction) (bool, error) {
	raw := strings.ToLower(strings.TrimSpace(action.Params["replace_contributions"]))
	switch raw {
	case "":
		return false, nil
	case "1", "true", "yes", "y", "on":
		return true, nil
	case "0", "false", "no", "n", "off":
		return false, nil
	default:
		return false, dataActionParamError(
			DataActionComputeContribs,
			"replace_contributions",
			"boolean true/false",
			action.Params["replace_contributions"],
			nil,
		)
	}
}

func removeContributionDerivedRowDecisions(rows []RowDecision, contributions []ContributionRecord) []RowDecision {
	if len(rows) == 0 || len(contributions) == 0 {
		return rows
	}
	stale := map[string]bool{}
	for _, row := range rowDecisionsFromContributions(contributions) {
		if key := rowDecisionRecordIdentityKey(row); key != "" {
			stale[key] = true
		}
	}
	if len(stale) == 0 {
		return rows
	}
	out := make([]RowDecision, 0, len(rows))
	for _, row := range rows {
		if !stale[rowDecisionRecordIdentityKey(row)] {
			out = append(out, row)
		}
	}
	return out
}
