package agent

import "github.com/hanchaoqun/codrax/internal/types"

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
// ReadFiles / ToolResults / EvidenceItems / FlowFindings / TerminalEvidenceCount
// to the same semantics.
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
//   - ToolResults: concat. Tool calls are a time-ordered event stream;
//     a legitimate investigation may grep the same pattern twice across
//     windows, and dropping one call would misreport the investigation.
//   - AcceptedClosureReason / AcceptedResultKind: current when present,
//     otherwise prior. Later windows may contain the accepted completion
//     rationale after a repair, but a closure-only retry must not erase a
//     previously accepted rationale with an empty value.
//   - AcceptedAggregateFacts: current when present, otherwise prior.
//     These are structured model-emitted closure facts, so the last
//     successful completion window owns them.
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
	merged.ReadFiles = mergeStrings(prior.ReadFiles, current.ReadFiles)
	merged.ToolResults = append(append([]types.ToolResult(nil), prior.ToolResults...), current.ToolResults...)
	if merged.AcceptedClosureReason == "" {
		merged.AcceptedClosureReason = prior.AcceptedClosureReason
	}
	if merged.AcceptedResultKind == "" {
		merged.AcceptedResultKind = prior.AcceptedResultKind
	}
	if len(merged.AcceptedAggregateFacts) == 0 {
		merged.AcceptedAggregateFacts = prior.AcceptedAggregateFacts
	}
	merged.EvidenceItems = mergeEvidenceItems(prior.EvidenceItems, current.EvidenceItems)
	merged.FlowFindings = mergeFlowFindings(prior.FlowFindings, current.FlowFindings)
	if prior.TerminalEvidenceCount > merged.TerminalEvidenceCount {
		merged.TerminalEvidenceCount = prior.TerminalEvidenceCount
	}
	return merged
}
