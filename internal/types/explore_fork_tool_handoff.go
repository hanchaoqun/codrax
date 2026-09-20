package types

import "reflect"

// MergeExploreForkPublishedTools retains completed producer-owned tool data
// from a sibling whose model work is no longer needed. Unlike MergeExploreFork,
// it does not accept the sibling's closure, aggregates, notes, evidence, repairs,
// or execution signals. Cancellation ends future work, not past observations.
//
// The dispatch buffer is essential: a canceled ParseOutput can return before
// writing TurnAArtifacts. The snapshot delta also covers a completed sibling.
// Call only after the worker has stopped; the orchestrator serializes merges.
func (m *MutableState) MergeExploreForkPublishedTools(fork *MutableState) []ToolResult {
	if m == nil || fork == nil || m == fork {
		return nil
	}
	fork.mu.RLock()
	var candidates []ToolResult
	if fork.turnAArtifacts != nil {
		results := fork.turnAArtifacts.ToolResults
		candidates = append(candidates, cloneTraceBusinessSpanToolResults(results[clampMergeSliceBase(fork.exploreForkTurnABaseToolLen, len(results)):])...)
	}
	candidates = append(candidates, cloneTraceBusinessSpanToolResults(fork.dispatchToolResults)...)
	fork.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()
	snapshot := cloneTurnAArtifactsPtr(m.turnAArtifacts)
	if snapshot == nil {
		snapshot = &TurnAArtifacts{}
	}
	var added []ToolResult
	for _, result := range candidates {
		if !completedProducerToolResult(result) || containsExactToolResult(snapshot.ToolResults, result) {
			continue
		}
		snapshot.ToolResults = append(snapshot.ToolResults, result)
		added = append(added, result)
		// Replay only the existing producer-publication registrations. Their
		// private receipts retain the same generation checks; no authority is
		// reconstructed from summaries or fork-wide model state.
		m.traceQueryRuntimeObservationCount += traceQueryRuntimeObservationToolResultCount(result)
		for _, ref := range traceQueryPublishedBlobRefsFromToolResult(result) {
			m.registerTraceQueryBlobRefLocked(ref)
		}
		m.registerArtifactReadNavigationResultLocked(result)
		m.registerTraceQuerySourceReadLocked(result)
		m.registerTraceBusinessSpanRefsLocked(result)
	}
	if len(added) == 0 {
		return nil
	}
	var truncation *ToolResultTruncationSummary
	snapshot.ToolResults, truncation = BoundTurnAToolResultsWithTruncation(
		snapshot.ToolResults, turnAArtifactsMutableToolResultsCountCap,
		turnAArtifactsMutableToolResultsByteCap, PreserveSuccessfulToolResultWithPayload)
	snapshot.ToolResultTruncation = MergeToolResultTruncationSummaries(snapshot.ToolResultTruncation, truncation)
	// Keep the typed mirror in sync regardless of whether the winning fork
	// was merged before or after this data-only handoff. Explicit carriers
	// and accepted evidence here belong to the parent snapshot, never to the
	// losing fork; observation refs are derived from the retained tools.
	snapshot.HandoffCarriers = ToolHandoffCarriersFromTurnAInputs(snapshot.ToolResults, snapshot.EvidenceItems, snapshot.HandoffCarriers)
	m.turnAArtifacts = snapshot
	m.turnAArtifactsRevision++
	m.cachedLabelSupport = nil
	m.cachedLabelSupportSource = nil
	m.bumpAnswerSurfaceRevisionLocked()
	return cloneTraceBusinessSpanToolResults(added)
}

// These carriers are published by deterministic tool implementations, not by
// model emit schemas. Bare summary/raw-ref text, arbitrary observation notes,
// and model-authored emit acknowledgements cannot enter this recovery lane.
func completedProducerToolResult(result ToolResult) bool {
	if !result.Success {
		return false
	}
	return (runtimeObservationProducerIsDeterministicQuery(result.ToolName) && toolResultCarriesDeterministicRuntimeObservation(result)) ||
		result.CommandMeasurement != nil || result.VCSHistory != nil ||
		result.ReadCoverage != nil || result.RuntimeArtifactRead != nil || result.SourceInventory != nil
}

func containsExactToolResult(results []ToolResult, candidate ToolResult) bool {
	for _, result := range results {
		if reflect.DeepEqual(result, candidate) {
			return true
		}
	}
	return false
}
