package types

// TraceCausalProjectionTargetStateAccountFromRecord reads the existing account
// fields without selecting a source, changing values, or granting join authority.
func TraceCausalProjectionTargetStateAccountFromRecord(record ObservationRecord) (TraceCausalProjectionTargetStateAccount, bool) {
	candidate, ok := traceCausalProjectionTargetStateCandidateFromRecord(record)
	return candidate.Account, ok
}
