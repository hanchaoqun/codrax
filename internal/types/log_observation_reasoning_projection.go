package types

import "strings"

// ProjectLogObservationForReasoning retains navigation-only interpretations
// subject to the peer-error boundary. Answer-grade consumers must additionally
// apply ProjectLogObservationForFacts below.
// LogObservation has no typed link to one of several top-level error occurrences:
// its free-form Subject and Summary cannot supply an identity or causal claim.
// The original bundle remains the lossless audit carrier. This value-copy
// projection preserves the excerpt and its coordinates, never edits an answer,
// and does not inspect the wording of either the interpretation or the excerpt.
// Single-error/operational logs retain their existing advisory interpretation.
func ProjectLogObservationForReasoning(bundle *LogBundle, obs LogObservation) (LogObservation, bool) {
	if obs.SourceBinding != nil {
		obs.LineStart, obs.LineEnd = 0, 0
		if obs.SourceBinding.IsVerified() {
			obs.LineStart, obs.LineEnd = int(obs.SourceBinding.FirstLine), int(obs.SourceBinding.LastLine)
		}
	}
	if bundle == nil || len(bundle.Errors) <= 1 {
		return obs, true
	}
	obs.Subject = ""
	obs.Summary = obs.Evidence
	return obs, strings.TrimSpace(obs.Evidence) != ""
}

// ProjectLogObservationForFacts is the answer-grade projection. A source
// receipt locates Evidence; it does not verify the triager's Subject/Summary.
// Keep those interpretations in the original audit bundle and the explicitly
// advisory exploration view, never in claim identities, evidence references,
// or native observation seeds. This applies equally to operational logs and
// single/multiple-error captures, independent of the wording of any field.
func ProjectLogObservationForFacts(bundle *LogBundle, obs LogObservation) (LogObservation, bool) {
	obs, ok := ProjectLogObservationForReasoning(bundle, obs)
	obs.Subject = ""
	obs.Summary = obs.Evidence
	return obs, ok && strings.TrimSpace(obs.Evidence) != ""
}
