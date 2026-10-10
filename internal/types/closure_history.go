package types

import "strings"

// InvestigationClosureHistoryEntry preserves a superseded model-authored
// closure for audit. Membership records lifecycle, not factual authority or
// support for the reason. It is not part of ordinary investigation narrative.
type InvestigationClosureHistoryEntry struct {
	Reason     string `json:"reason"`
	ResultKind string `json:"result_kind,omitempty"`
}

// MergeInvestigationClosureHistory records replacements at the snapshot merge
// boundary. It never classifies model prose, migrates old narrative wrappers,
// or creates evidence bindings. Legacy snapshots without this field remain
// valid; their original InvestigationNotes are left untouched.
func MergeInvestigationClosureHistory(prior, current *TurnAArtifacts) []InvestigationClosureHistoryEntry {
	var out []InvestigationClosureHistoryEntry
	seen := map[InvestigationClosureHistoryEntry]bool{}
	appendEntry := func(entry InvestigationClosureHistoryEntry) {
		if !seen[entry] {
			seen[entry] = true
			out = append(out, entry)
		}
	}
	if prior != nil {
		for _, entry := range prior.SupersededClosures {
			appendEntry(entry)
		}
	}
	if current != nil {
		for _, entry := range current.SupersededClosures {
			appendEntry(entry)
		}
	}
	if prior != nil && current != nil &&
		strings.TrimSpace(prior.AcceptedClosureReason) != "" &&
		strings.TrimSpace(current.AcceptedClosureReason) != "" &&
		(prior.AcceptedClosureReason != current.AcceptedClosureReason ||
			current.AcceptedResultKind != "" && prior.AcceptedResultKind != current.AcceptedResultKind) {
		appendEntry(InvestigationClosureHistoryEntry{
			Reason: prior.AcceptedClosureReason, ResultKind: prior.AcceptedResultKind,
		})
	}
	return out
}
