package types

import "strings"

// VerificationProbeExecutionGranularityNote describes the existing executor's
// receipt boundary, not a new verification result. These admitted reference
// rows prove whole-probe success plus changed-source identity coupling. They
// do not carry executor-owned, per-reference or per-method invocation records.
// Keep this presentation independent of probe code, report prose and language;
// neither the note nor its absence may authorize or reject a contract witness.
func VerificationProbeExecutionGranularityNote(rec VerificationConfidenceRecord) string {
	if strings.TrimSpace(rec.Source) != "verification_probe" || strings.TrimSpace(rec.Status) != "satisfied" {
		return ""
	}
	switch strings.TrimSpace(rec.Category) {
	case "probe_contract_refs", "probe_soft_contract_refs", "probe_placement_refs":
	default:
		return ""
	}
	witness, ok := VerificationConfidenceRecordWitnessKind(rec)
	if !ok || witness != WriteBehaviorWitnessVerificationProbe {
		return ""
	}
	return "Whole-probe pass + changed-target binding; plan refs are not per-contract/method execution receipts or runtime coverage."
}

// VerificationConfidenceDisplayDetail retains the original observation and
// adds its execution boundary only in a view. Callers must not write it back
// into the report or treat it as a reason code, obligation, or witness kind.
func VerificationConfidenceDisplayDetail(rec VerificationConfidenceRecord) string {
	detail := strings.TrimSpace(rec.Detail)
	if note := VerificationProbeExecutionGranularityNote(rec); note != "" {
		// Put the boundary first so bounded context views do not lose it behind
		// a long existing observation detail.
		return strings.TrimSpace(note + " " + detail)
	}
	return detail
}
