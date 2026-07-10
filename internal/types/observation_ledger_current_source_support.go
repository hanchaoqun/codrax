package types

import "strings"

// currentSourceSupportWitness is a deterministic current-checkout coordinate
// that was already accepted before aggregate-fact ledger compilation. Model
// authored SupportRefs are deliberately not witnesses by themselves.
type currentSourceSupportWitness struct {
	path      string
	lineStart int
	lineEnd   int
	status    GroundingStatus
}

// currentSourceSupportBinding is the canonical coordinate/status copied onto
// an aggregate record after every exact current-source SupportRef on that fact
// has matched an accepted witness.
type currentSourceSupportBinding struct {
	path      string
	lineStart int
	lineEnd   int
	status    GroundingStatus
}

type currentSourceSupportWitnessIndex struct {
	witnesses []currentSourceSupportWitness
}

// compileCurrentSourceSupportWitnessIndex consumes only typed, already
// validated producer surfaces:
//   - EvidenceItem rows whose grounding pass returned Grounded/Recovered; and
//   - successful ToolResult typed carriers (read coverage/source inventory)
//     that compile to a Grounded/Recovered current-source line record.
//
// It never parses ToolResult.Summary and never treats an aggregate SupportRef
// as self-authenticating evidence.
func compileCurrentSourceSupportWitnessIndex(items []EvidenceItem, results []ToolResult) currentSourceSupportWitnessIndex {
	var out currentSourceSupportWitnessIndex
	appendRecord := func(record ObservationRecord) {
		if record.Origin != AnswerEvidenceOriginCurrentSource ||
			record.SourceRef.Kind != ObservationSourceCurrentSource ||
			!currentSourceSupportGroundingAccepted(record.GroundingStatus) ||
			strings.TrimSpace(record.SourceRef.Path) == "" ||
			record.Span.LineStart <= 0 {
			return
		}
		out.append(currentSourceSupportWitness{
			path:      record.SourceRef.Path,
			lineStart: record.Span.LineStart,
			lineEnd:   record.Span.LineEnd,
			status:    record.GroundingStatus,
		})
	}

	compileEvidenceItemObservations(items, appendRecord)
	seen := map[string]bool{}
	for i, result := range results {
		if !result.Success {
			continue
		}
		// Typed carriers are the only ToolResult lane admitted here. The
		// legacy summary parser is intentionally outside this proof index.
		compileToolResultCarrierObservations(i, result, nil, seen, appendRecord)
	}
	return out
}

func currentSourceSupportGroundingAccepted(status GroundingStatus) bool {
	return status == GroundingGrounded || status == GroundingRecovered
}

func (idx *currentSourceSupportWitnessIndex) append(witness currentSourceSupportWitness) {
	witness.path = displayAnswerLocationFile(witness.path)
	if witness.path == "" || witness.lineStart <= 0 || !currentSourceSupportGroundingAccepted(witness.status) {
		return
	}
	if witness.lineEnd < witness.lineStart {
		witness.lineEnd = witness.lineStart
	}
	for i := range idx.witnesses {
		existing := &idx.witnesses[i]
		if normalizeAnswerLocationFile(existing.path) != normalizeAnswerLocationFile(witness.path) ||
			existing.lineStart != witness.lineStart || existing.lineEnd != witness.lineEnd {
			continue
		}
		if currentSourceSupportGroundingRank(witness.status) > currentSourceSupportGroundingRank(existing.status) {
			existing.status = witness.status
		}
		return
	}
	idx.witnesses = append(idx.witnesses, witness)
}

// bindFact verifies every source-shaped SupportRef on fact. Requiring the
// complete declared coordinate set prevents one real anchor from laundering a
// second model-invented file:line while still allowing non-source provenance
// refs (runtime/VCS/etc.) to coexist on a mixed-origin fact.
func (idx currentSourceSupportWitnessIndex) bindFact(fact AnswerAggregateFact) (currentSourceSupportBinding, bool) {
	var best currentSourceSupportBinding
	found := false
	for _, raw := range fact.SupportRefs {
		_, location, ok := ParseAnswerSupportRefMemberLocation(raw)
		if !ok || strings.TrimSpace(location.File) == "" || location.LineStart <= 0 {
			continue
		}
		binding, matched := idx.bindLocation(location)
		if !matched {
			return currentSourceSupportBinding{}, false
		}
		if !found || currentSourceSupportGroundingRank(binding.status) > currentSourceSupportGroundingRank(best.status) {
			best = binding
		}
		found = true
	}
	return best, found
}

func (idx currentSourceSupportWitnessIndex) bindLocation(location AnswerSourceLocationSurface) (currentSourceSupportBinding, bool) {
	wantPath := normalizeAnswerLocationFile(location.File)
	wantStart := location.LineStart
	wantEnd := location.LineEnd
	if wantPath == "" || wantStart <= 0 {
		return currentSourceSupportBinding{}, false
	}
	if wantEnd < wantStart {
		wantEnd = wantStart
	}

	// Exact normalized paths win over suffix-compatible display paths. When a
	// model uses a basename/short path, it is admitted only if it resolves to
	// one unique witnessed path; ambiguity fails closed.
	exact := map[string]currentSourceSupportBinding{}
	compatible := map[string]currentSourceSupportBinding{}
	for _, witness := range idx.witnesses {
		witnessPath := normalizeAnswerLocationFile(witness.path)
		if witnessPath == "" || witness.lineStart > wantStart || witness.lineEnd < wantEnd {
			continue
		}
		binding := currentSourceSupportBinding{
			path:      witness.path,
			lineStart: wantStart,
			lineEnd:   wantEnd,
			status:    witness.status,
		}
		if witnessPath == wantPath {
			mergeCurrentSourceSupportBinding(exact, witnessPath, binding)
			continue
		}
		if answerLocationFileMatches(location.File, witness.path) {
			mergeCurrentSourceSupportBinding(compatible, witnessPath, binding)
		}
	}
	if binding, ok := uniqueCurrentSourceSupportBinding(exact); ok {
		return binding, true
	}
	return uniqueCurrentSourceSupportBinding(compatible)
}

func mergeCurrentSourceSupportBinding(dst map[string]currentSourceSupportBinding, key string, candidate currentSourceSupportBinding) {
	if existing, ok := dst[key]; ok && currentSourceSupportGroundingRank(existing.status) >= currentSourceSupportGroundingRank(candidate.status) {
		return
	}
	dst[key] = candidate
}

func uniqueCurrentSourceSupportBinding(candidates map[string]currentSourceSupportBinding) (currentSourceSupportBinding, bool) {
	if len(candidates) != 1 {
		return currentSourceSupportBinding{}, false
	}
	for _, binding := range candidates {
		return binding, true
	}
	return currentSourceSupportBinding{}, false
}

func currentSourceSupportGroundingRank(status GroundingStatus) int {
	switch status {
	case GroundingGrounded:
		return 2
	case GroundingRecovered:
		return 1
	default:
		return 0
	}
}
