package agent

import "github.com/hanchaoqun/codrax/internal/types"

// TurnAArtifacts.ToolResults snapshot bounds. The slice is captured per
// explorer window and concatenated across retry windows; without a bound a
// long multi-window investigation copies an ever-growing slice into the
// criterion environment and the observation-ledger input on every dispatch.
// pruneToolHistory does NOT bound this slice — it only stubs LLM message
// history — so the bound lives at the two snapshot sites: the per-window
// capture in explorer.ParseOutput and the cross-window concat in
// mergeTurnAArtifactsWithPrior.
//
// The caps are deliberately generous: a typical window stays far below them,
// so default behavior is unchanged; only pathological accumulation is cut.
// Both bounds drop OLDEST results first and keep chronological order.
const (
	// turnAToolResultsWindowCountCap / turnAToolResultsWindowByteCap bound
	// one window's contribution at capture time.
	turnAToolResultsWindowCountCap = 160
	turnAToolResultsWindowByteCap  = 1 << 20 // 1 MiB
	// turnAToolResultsMergedCountCap / turnAToolResultsMergedByteCap bound
	// the cross-window concatenation in mergeTurnAArtifactsWithPrior.
	turnAToolResultsMergedCountCap = 320
	turnAToolResultsMergedByteCap  = 2 << 20 // 2 MiB
)

// turnAToolResultBytes is the deterministic byte accounting for the snapshot
// bound: the carried text surfaces of the result plus its attached typed
// observation rows. It intentionally over-counts a little (fixed per-row
// overhead) rather than marshaling for an exact figure.
func turnAToolResultBytes(r types.ToolResult) int {
	n := len(r.ToolName) + len(r.Summary) + len(r.RawRef) + 64
	for _, obs := range r.Observations {
		n += len(obs.ID) + len(obs.ClaimKey) + len(obs.Subject) + len(obs.Predicate) +
			len(obs.Object) + len(obs.Value) + len(obs.Summary) + len(obs.RawExcerpt)
		for _, note := range obs.RichNotes {
			n += len(note)
		}
		for _, ref := range obs.SupportRefs {
			n += len(ref)
		}
		n += 64
	}
	return n
}

// boundTurnAToolResults applies the count + byte cap to a chronological tool
// result slice, dropping oldest entries first. Slices already inside the caps
// are returned unchanged (no copy), keeping the default path byte-identical.
//
// Gate preservation: extractor.InvestigationStructurallyEmpty accepts an
// investigation if at least one SUCCESSFUL investigation-class tool result
// survives in this slice. When dropping the oldest prefix would remove every
// such result, the newest one from the dropped prefix is retained at the head
// (still oldest-first order), so the bound can never flip that gate from
// "real investigation" to "structurally empty". The explorer's own
// completionReadiness counters run over the in-window dispatch history before
// the snapshot is taken, so they are unaffected by this bound.
func boundTurnAToolResults(results []types.ToolResult, countCap, byteCap int) []types.ToolResult {
	if len(results) == 0 || countCap <= 0 || byteCap <= 0 {
		return results
	}
	total := 0
	for _, r := range results {
		total += turnAToolResultBytes(r)
	}
	if len(results) <= countCap && total <= byteCap {
		return results
	}
	// Walk newest → oldest, keeping entries while both caps hold.
	start := len(results)
	kept := 0
	keptBytes := 0
	for i := len(results) - 1; i >= 0; i-- {
		size := turnAToolResultBytes(results[i])
		if kept+1 > countCap || keptBytes+size > byteCap {
			break
		}
		kept++
		keptBytes += size
		start = i
	}
	if start == 0 {
		return results
	}
	out := results[start:]
	if !hasSuccessfulInvestigationToolResult(out) {
		for i := start - 1; i >= 0; i-- {
			if results[i].Success && investigationToolKinds[results[i].ToolName] {
				preserved := make([]types.ToolResult, 0, len(out)+1)
				preserved = append(preserved, results[i])
				preserved = append(preserved, out...)
				return preserved
			}
		}
	}
	return append([]types.ToolResult(nil), out...)
}

func hasSuccessfulInvestigationToolResult(results []types.ToolResult) bool {
	for _, r := range results {
		if r.Success && investigationToolKinds[r.ToolName] {
			return true
		}
	}
	return false
}

// mergeTurnAArtifactsWithPrior folds a prior TurnAArtifacts snapshot into the
// current explorer window's snapshot so intra-Run retry windows accumulate
// rather than overwrite.
//
// The DAG scheduler may requeue an explore node (SuccessCriteria failed,
// validation_feedback backtrack, contract retry) and re-dispatch the
// explorer. BaseAgent starts each dispatch with a fresh ReAct history, so
// the `toolResults` / `readFilesList` visible in explorer.ParseOutput only
// reflect the CURRENT window. Without this merge a second-window dispatch
// that only greps (no read_file) would erase Window 1's rich ReadFiles and
// drive extractorInvestigationEmpty's R4 fail-loud even though Turn A
// collected plenty.
//
// e.investigationNotes and ctx.Mutable.EmittedEvidence() already accumulate
// cross-window via their respective reset gates; this helper brings
// ReadFiles / ToolResults / EvidenceItems / FlowFindings / source-inventory
// carriers / TerminalEvidenceCount to the same semantics.
//
// Per-field rules:
//
//   - UserQuestion: current (invariant across windows of the same Run; if
//     it differs, the cross-run reset has already fired upstream and prior
//     should be nil).
//   - InvestigationNotes: current — the evaluator's own e.investigationNotes
//     field is already appended to each window, so it is the authoritative
//     cumulative value.
//   - ReadFiles: mergeStrings(prior, current). Dedupe + union.
//   - ToolResults: concat, then boundTurnAToolResults with the merged
//     count/byte caps. Tool calls are a time-ordered event stream; a
//     legitimate investigation may grep the same pattern twice across
//     windows, so nothing is deduplicated — but the concatenation must
//     not grow without limit across retry windows, so the oldest entries
//     are dropped once the merged caps are exceeded (keeping at least one
//     successful investigation-class result for the structural-empty gate).
//   - MCPResponses: concat. MCP calls are also an external observation event
//     stream; preserving them in the frozen handoff keeps typed MCP rows
//     available across retry windows without reclassifying them as source
//     citations.
//   - AcceptedClosureReason / AcceptedResultKind: current when present,
//     otherwise prior. Later windows may contain the accepted completion
//     rationale after a repair, but a closure-only retry must not erase a
//     previously accepted rationale with an empty value.
//   - AcceptedAggregateFacts: current when present, otherwise prior.
//     These are structured model-emitted closure facts, so the last
//     successful completion window owns them.
//   - SourceInventoryAdvisory / SourceInventoryObservation: merge prior +
//     current. A later closure-only retry can legitimately produce no fresh
//     graph inventory, but it must not erase the source-inventory handoff that
//     extractor/finalizer use to preserve bounded member sets.
//   - RuntimeObservationOnlyCompletion: logical OR. Once a retry
//     window has accepted an external artifact-only completion, later
//     closure-only windows must not erase the fact that no repo reads
//     were required.
//   - EvidenceItems: mergeEvidenceItems(prior, current). Already ID-deduped.
//   - FlowFindings: mergeFlowFindings(prior, current). Already ID-deduped.
//   - TerminalEvidenceCount: max(prior, current). Must not regress — a
//     Window 2 that filters out some chains (e.g. mechanism-kind filter
//     drops all answer chains) should not overwrite Window 1's higher
//     baseline, which Turn B's cardinality validator depends on.
func mergeTurnAArtifactsWithPrior(prior *types.TurnAArtifacts, current types.TurnAArtifacts) types.TurnAArtifacts {
	if prior == nil {
		return current
	}
	merged := current
	merged.InvestigationNotes = types.PreserveSupersededClosureReasonNote(merged.InvestigationNotes, prior.AcceptedClosureReason, current.AcceptedClosureReason)
	merged.ReadFiles = mergeStrings(prior.ReadFiles, current.ReadFiles)
	merged.ToolResults = boundTurnAToolResults(
		append(append([]types.ToolResult(nil), prior.ToolResults...), current.ToolResults...),
		turnAToolResultsMergedCountCap,
		turnAToolResultsMergedByteCap,
	)
	merged.MCPResponses = append(append([]types.MCPResponse(nil), prior.MCPResponses...), current.MCPResponses...)
	if merged.AcceptedClosureReason == "" {
		merged.AcceptedClosureReason = prior.AcceptedClosureReason
	}
	if merged.AcceptedResultKind == "" {
		merged.AcceptedResultKind = prior.AcceptedResultKind
	}
	if len(merged.AcceptedAggregateFacts) == 0 {
		merged.AcceptedAggregateFacts = prior.AcceptedAggregateFacts
	}
	merged.SourceInventoryAdvisory = types.MergeSourceInventoryAdvisory(prior.SourceInventoryAdvisory, current.SourceInventoryAdvisory)
	merged.SourceInventoryObservation = types.MergeSourceInventoryObservation(prior.SourceInventoryObservation, current.SourceInventoryObservation)
	if !merged.SourceInventoryObservation.IsActive() && merged.SourceInventoryAdvisory.IsActive() {
		merged.SourceInventoryObservation = types.SourceInventoryObservationFromAdvisory(merged.SourceInventoryAdvisory)
	}
	merged.RuntimeObservationOnlyCompletion = prior.RuntimeObservationOnlyCompletion || current.RuntimeObservationOnlyCompletion
	merged.EvidenceItems, _ = MergeEvidenceItemsIfChanged(prior.EvidenceItems, current.EvidenceItems)
	merged.FlowFindings, _ = MergeFlowFindingsIfChanged(prior.FlowFindings, current.FlowFindings)
	if prior.TerminalEvidenceCount > merged.TerminalEvidenceCount {
		merged.TerminalEvidenceCount = prior.TerminalEvidenceCount
	}
	return merged
}
