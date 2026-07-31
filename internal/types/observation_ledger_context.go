package types

import "strings"

// ObservationLedgerInputFromAgentContext projects the already-accepted agent
// context carriers into the single ledger input shape. It is intentionally a
// shallow, deterministic adapter: no raw user/model prose is parsed to infer
// origin or policy.
func ObservationLedgerInputFromAgentContext(ctx *AgentContext, evidenceLimit int) ObservationLedgerInput {
	if ctx == nil {
		return ObservationLedgerInput{}
	}
	var (
		requestModel    *RequestModel
		answerContract  *AnswerContract
		logBundle       *LogBundle
		perfBundle      *PerfBundle
		aggregateFacts  []AnswerAggregateFact
		toolResults     []ToolResult
		mcpResponses    []MCPResponse
		evidenceItems   []EvidenceItem
		sourceInventory SourceInventoryObservation
	)
	if ctx.AnalysisIR != nil {
		// CSR #64 ruling 1 (2026-07-05): the ledger-input RequestModel goes
		// through the SAME bundle-backfill helper as the authority snapshot
		// (RuntimeSourceAuthorityRequestModelFromAgentContext). The analyzer
		// RM copy often lacks the LogTriage/PerfTrace bundle (it only lives
		// in Mutable), so HasExternalOnlyRuntimeArtifact()/
		// CurrentSourceLaneDecision() lied on the ledger face: in a mixed
		// (non-exclude) attached-artifact run every model aggregate fact with
		// no origin-lane hit fell through to the terminal current_source
		// fallback and fake-satisfied the source lane (the same pollution
		// class as CSP #63, mixed-run face). With the shared helper the two
		// faces of one run can no longer disagree on the lane.
		requestModel = RuntimeSourceAuthorityRequestModelFromAgentContext(ctx)
		answerContract = &ctx.AnalysisIR.AnswerContract
		logBundle = ctx.AnalysisIR.RequestModel.LogTriage
		perfBundle = ctx.AnalysisIR.RequestModel.PerfTrace
	}
	evidenceItems = appendObservationLedgerEvidence(evidenceItems, evidenceLimit, ctx.EvidenceItems...)
	if ctx.Mutable != nil {
		aggregateFacts = ctx.Mutable.StableInvestigationAggregateFacts()
		sourceInventory = ctx.Mutable.SourceInventoryObservation()
		if logBundle == nil {
			logBundle = ctx.Mutable.LogTriage()
		}
		if perfBundle == nil {
			perfBundle = ctx.Mutable.PerfTrace()
		}
		if ta := ctx.Mutable.TurnAArtifacts(); ta != nil {
			// XGAP-FIX ① cross-turn mirror — see answer_surface_plan.go.
			aggregateFacts = MergeAnswerAggregateFacts(aggregateFacts,
				SupersedeOrdinalMemberSetFactsByLabel(ta.AcceptedAggregateFacts, aggregateFacts))
			evidenceItems = appendObservationLedgerEvidence(evidenceItems, evidenceLimit, ta.EvidenceItems...)
			toolResults = append([]ToolResult(nil), ta.ToolResults...)
			if len(ctx.MCPResponses) == 0 {
				mcpResponses = append([]MCPResponse(nil), ta.MCPResponses...)
			}
			sourceInventory = MergeSourceInventoryObservation(sourceInventory, ta.SourceInventoryObservation)
		}
		toolResults = mergeObservationLedgerToolResults(toolResults, ctx.Mutable.DispatchToolResults())
		if len(evidenceItems) < normalizedObservationLedgerEvidenceLimit(evidenceLimit) || evidenceLimit <= 0 {
			evidenceItems = appendObservationLedgerEvidence(evidenceItems, evidenceLimit, ctx.Mutable.EmittedEvidence()...)
		}
	}
	return ObservationLedgerInput{
		EvidenceItems:              evidenceItems,
		AggregateFacts:             aggregateFacts,
		SourceInventoryObservation: sourceInventory,
		ToolResults:                toolResults,
		RuntimeArtifactPreflight:   ctx.RuntimeArtifactPreflight,
		RepoRoot:                   ctx.RepoRoot,
		// SUPP-CORE single merge point (AgentContext side): the dedicated
		// system supplement lane rides its own input field so compile-side
		// provenance stamping stays structural.
		SystemTraceSupplementResults: ctx.Mutable.SystemTraceSupplementResults(),
		LogBundle:                    logBundle,
		PerfBundle:                   perfBundle,
		MCPResponses:                 append(mcpResponses, ctx.MCPResponses...),
		RequestModel:                 requestModel,
		AnswerContract:               answerContract,
	}
}

// ObservationLedgerInputFromBusContext is the orchestrator/reviewer companion to
// ObservationLedgerInputFromAgentContext. When a Turn A handoff exists, its tool
// results are preferred over the full bus history so analyzer/pre-scan noise does
// not become answer-grade review context.
func ObservationLedgerInputFromBusContext(bus *BusContext, evidenceLimit int) ObservationLedgerInput {
	if bus == nil {
		return ObservationLedgerInput{}
	}
	var (
		requestModel    *RequestModel
		answerContract  *AnswerContract
		logBundle       *LogBundle
		perfBundle      *PerfBundle
		aggregateFacts  []AnswerAggregateFact
		toolResults     []ToolResult
		mcpResponses    []MCPResponse
		evidenceItems   []EvidenceItem
		sourceInventory SourceInventoryObservation
	)
	if bus.AnalysisIR != nil {
		// CSR #64 ruling 1: same-source bundle backfill as the authority
		// snapshot (see the AgentContext twin above for the full rationale).
		requestModel = RuntimeSourceAuthorityRequestModelFromBusContext(bus)
		answerContract = &bus.AnalysisIR.AnswerContract
		logBundle = bus.AnalysisIR.RequestModel.LogTriage
		perfBundle = bus.AnalysisIR.RequestModel.PerfTrace
	}
	evidenceItems = appendObservationLedgerEvidence(evidenceItems, evidenceLimit, bus.EvidenceItems...)
	toolResults = append([]ToolResult(nil), bus.ToolResults...)
	mcpResponses = append([]MCPResponse(nil), bus.MCPResponses...)
	if bus.Mutable != nil {
		aggregateFacts = bus.Mutable.StableInvestigationAggregateFacts()
		sourceInventory = bus.Mutable.SourceInventoryObservation()
		if logBundle == nil {
			logBundle = bus.Mutable.LogTriage()
		}
		if perfBundle == nil {
			perfBundle = bus.Mutable.PerfTrace()
		}
		if ta := bus.Mutable.TurnAArtifacts(); ta != nil {
			// XGAP-FIX ① cross-turn mirror — see answer_surface_plan.go.
			aggregateFacts = MergeAnswerAggregateFacts(aggregateFacts,
				SupersedeOrdinalMemberSetFactsByLabel(ta.AcceptedAggregateFacts, aggregateFacts))
			evidenceItems = appendObservationLedgerEvidence(evidenceItems, evidenceLimit, ta.EvidenceItems...)
			if len(ta.ToolResults) > 0 {
				toolResults = append([]ToolResult(nil), ta.ToolResults...)
			}
			if len(mcpResponses) == 0 {
				mcpResponses = append([]MCPResponse(nil), ta.MCPResponses...)
			}
			sourceInventory = MergeSourceInventoryObservation(sourceInventory, ta.SourceInventoryObservation)
		}
		toolResults = mergeObservationLedgerToolResults(toolResults, bus.Mutable.DispatchToolResults())
	}
	var supplementResults []ToolResult
	if bus.Mutable != nil {
		// SUPP-CORE single merge point (BusContext side) — twin of the
		// AgentContext builder above.
		supplementResults = bus.Mutable.SystemTraceSupplementResults()
	}
	return ObservationLedgerInput{
		EvidenceItems:                evidenceItems,
		AggregateFacts:               aggregateFacts,
		SourceInventoryObservation:   sourceInventory,
		ToolResults:                  toolResults,
		RuntimeArtifactPreflight:     bus.RuntimeArtifactPreflight,
		RepoRoot:                     bus.RepoRoot,
		SystemTraceSupplementResults: supplementResults,
		LogBundle:                    logBundle,
		PerfBundle:                   perfBundle,
		MCPResponses:                 mcpResponses,
		RequestModel:                 requestModel,
		AnswerContract:               answerContract,
	}
}

func mergeObservationLedgerToolResults(base, extra []ToolResult) []ToolResult {
	if len(extra) == 0 {
		return base
	}
	out := append([]ToolResult(nil), base...)
	seen := make(map[string]bool, len(base)+len(extra))
	for _, r := range out {
		seen[observationLedgerToolResultKey(r)] = true
	}
	for _, r := range extra {
		key := observationLedgerToolResultKey(r)
		if key != "" && seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, r)
	}
	return out
}

func observationLedgerToolResultKey(r ToolResult) string {
	return strings.TrimSpace(r.ToolName) + "\x00" + strings.TrimSpace(r.RawRef) + "\x00" + strings.TrimSpace(r.Summary)
}

func appendObservationLedgerEvidence(dst []EvidenceItem, limit int, items ...EvidenceItem) []EvidenceItem {
	if len(items) == 0 {
		return dst
	}
	max := normalizedObservationLedgerEvidenceLimit(limit)
	seen := make(map[string]struct{}, len(dst)+len(items))
	for _, existing := range dst {
		if id := observationLedgerEvidenceID(existing); id != "" {
			seen[id] = struct{}{}
		}
	}
	for _, item := range items {
		if max > 0 && len(dst) >= max {
			return dst
		}
		id := observationLedgerEvidenceID(item)
		if id != "" {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
		}
		if strings.TrimSpace(item.ID) == "" {
			item.ID = id
		}
		dst = append(dst, item)
	}
	return dst
}

func normalizedObservationLedgerEvidenceLimit(limit int) int {
	if limit <= 0 {
		return 0
	}
	return limit
}

func observationLedgerEvidenceID(item EvidenceItem) string {
	if id := strings.TrimSpace(item.ID); id != "" {
		return id
	}
	return StableEvidenceID(item)
}
