package types

import "strings"

// CurrentStatusUnknown is the empty answer-surface value. A current-status
// diagnostic decision is complete only when it carries one of the declared
// non-empty verdicts below.
const CurrentStatusUnknown CurrentStatusVerdict = ""

func AllCurrentStatusVerdicts() []CurrentStatusVerdict {
	return []CurrentStatusVerdict{
		CurrentStatusStillPresent,
		CurrentStatusFixed,
		CurrentStatusNotEnoughEvidence,
	}
}

func (v CurrentStatusVerdict) IsValid() bool {
	if v == CurrentStatusUnknown {
		return true
	}
	for _, declared := range AllCurrentStatusVerdicts() {
		if v == declared {
			return true
		}
	}
	return false
}

func NormalizeCurrentStatusVerdict(raw string) (CurrentStatusVerdict, bool) {
	verdict := CurrentStatusVerdict(strings.TrimSpace(raw))
	if !verdict.IsValid() {
		return CurrentStatusUnknown, false
	}
	return verdict, true
}

func CurrentStatusAllowedVerdicts(contract *CurrentStatusDiagnosticContract) []CurrentStatusVerdict {
	if contract != nil && len(contract.AllowedVerdicts) > 0 {
		out := make([]CurrentStatusVerdict, 0, len(contract.AllowedVerdicts))
		seen := map[CurrentStatusVerdict]bool{}
		for _, verdict := range contract.AllowedVerdicts {
			if verdict == CurrentStatusUnknown || seen[verdict] || !verdict.IsValid() {
				continue
			}
			seen[verdict] = true
			out = append(out, verdict)
		}
		if len(out) > 0 {
			return out
		}
	}
	return AllCurrentStatusVerdicts()
}

func CurrentStatusVerdictAllowed(verdict CurrentStatusVerdict, contract *CurrentStatusDiagnosticContract) bool {
	if verdict == CurrentStatusUnknown || !verdict.IsValid() {
		return false
	}
	for _, allowed := range CurrentStatusAllowedVerdicts(contract) {
		if verdict == allowed {
			return true
		}
	}
	return false
}

func MissingCurrentStatusVerdict(doc *AnswerDocumentV2, contract *CurrentStatusDiagnosticContract) bool {
	block := CurrentStatusDecisionBlock(doc)
	if block == nil {
		return true
	}
	return !CurrentStatusVerdictAllowed(block.CurrentStatusVerdict, contract)
}

func CurrentStatusDecisionBlock(doc *AnswerDocumentV2) *AnswerBlock {
	if doc == nil {
		return nil
	}
	var fallback *AnswerBlock
	for i := range doc.Blocks {
		if doc.Blocks[i].Kind != BlockDecision {
			continue
		}
		if doc.Blocks[i].SurfaceRole == SurfacePrincipal {
			return &doc.Blocks[i]
		}
		if fallback == nil {
			fallback = &doc.Blocks[i]
		}
	}
	return fallback
}
