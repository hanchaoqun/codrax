package types

// EvidenceIsDerivationCandidate reads a system-owned limitation, not a score,
// model confidence, source snippet, or answer text. An unset bit preserves the
// existing evidence policy; it is not a new claim of proof.
func EvidenceIsDerivationCandidate(item EvidenceItem) bool {
	return item.DerivationCandidate
}

// EvidenceDerivationBoundary is shared by the model-facing evidence surfaces.
// It qualifies the system's extracted claim without rewriting that claim or
// preventing the model from inspecting and reasoning about the original code.
func EvidenceDerivationBoundary(item EvidenceItem) string {
	if !EvidenceIsDerivationCandidate(item) {
		return ""
	}
	return "Candidate derivation: the cited source is an inspection lead; this extraction does not prove the operation, an exclusive binding, or the answer."
}
