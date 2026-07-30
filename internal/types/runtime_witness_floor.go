package types

// FABGATE-2 (2026-07-30): the runtime-observation witness floor.
//
// FABGATE-1 (answer_surface_plan.go, applyRuntimeTraceSourceOptionalSurfacePlan
// else-branch) closed the MINTING side of the customer-incident fabrication:
// a bare path reference with nothing behind it no longer mints the
// source-optional waiver. This file adds the FACTORY-FLOOR side: a single
// explicit invariant — "the source-optional flags may flip only when at
// least one runtime witness exists" — enforced at the one choke point where
// the flags flip, plus a contract-check tripwire (orchestrator side) that
// fires if an answer ever ships with an active runtime-grounding
// disposition over a witnessless run.
//
// Analysis (2026-07-30) shows BOTH minting lanes already imply the floor
// today: the authority lane's every disjunct requires ≥1 ledger observation
// (sufficiency demands non-empty addressable candidates), and the
// else-lane is guarded by FABGATE-1's four discriminators. The floor is
// therefore an INVARIANT, not a behavior change — its value is that any
// future regression in either lane (or a stale cached plan) trips a loud,
// typed signal instead of re-opening the fabrication hole. Same pattern as
// LEDGER-TRIPWIRE: guard structurally-impossible-today states cheaply.
//
// All inputs are precise typed signals (attachment boolean, typed ledger
// record scans) per the precise-signals-for-hard-gates red line.

// RuntimeSourceOptionalWaiverWitnessFloor reports whether the run carries
// at least one witness capable of backing a source-optional
// externally-grounded answer: an attached runtime artifact, a runtime
// trace/log artifact record in the ledger, a deterministic trace-query
// observation, any runtime observation record, or any external
// observation the sufficiency assessor itself counts as an addressable
// candidate. ROUND-3 SWEEP R8: the floor must be a SUPERSET of what the
// authority mint lane accepts — its first form omitted the sufficiency
// candidate origins (external documents, web pages, MCP resources,
// command measurements, VCS metadata, ...), which would have silently
// DENIED waivers for legitimately external-observation-sufficient turns
// and falsified the "invariant, not a behavior change" claim. Sharing
// externalObservationSufficiencyCandidates keeps the two sides from
// forking (same-family mirror discipline, CSP #63).
func RuntimeSourceOptionalWaiverWitnessFloor(attachedRuntimeArtifact bool, ledger ObservationLedger) bool {
	if attachedRuntimeArtifact {
		return true
	}
	if observationLedgerHasRuntimeTraceArtifact(ledger) ||
		observationLedgerHasRuntimeLogArtifact(ledger) ||
		observationLedgerHasTraceQueryRuntimeObservation(ledger) {
		return true
	}
	for _, record := range ledger.Records {
		if runtimeSourceAuthorityRuntimeRecord(record) {
			return true
		}
	}
	return len(externalObservationSufficiencyCandidates(ledger.Records)) > 0
}

// RuntimeGroundingClaimsWitnessless reports the tripwire condition the
// factory-floor contract check fires on: the compiled answer surface plan
// carries an ACTIVE runtime-grounding disposition whose citation policy
// tells the finalizer to cite runtime observations, while the run holds
// zero runtime witnesses of any class. With FABGATE-1 and the minting
// floor in place this state is structurally unreachable; reaching it
// means a minting lane regressed or a stale plan leaked — the exact
// authorization surface behind the customer-incident fabrication.
func RuntimeGroundingClaimsWitnessless(plan *AnswerSurfacePlan, attachedRuntimeArtifact bool, ledger ObservationLedger) bool {
	if plan == nil || plan.RuntimeGroundingDisposition == nil {
		return false
	}
	disp := plan.RuntimeGroundingDisposition
	if !disp.IsActive() || disp.CitationPolicy != RuntimeGroundingCitationRuntimeObservation {
		return false
	}
	return !RuntimeSourceOptionalWaiverWitnessFloor(attachedRuntimeArtifact, ledger)
}
