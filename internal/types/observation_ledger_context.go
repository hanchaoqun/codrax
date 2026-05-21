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
		requestModel   *RequestModel
		answerContract *AnswerContract
		logBundle      *LogBundle
		perfBundle     *PerfBundle
		aggregateFacts []AnswerAggregateFact
		toolResults    []ToolResult
		evidenceItems  []EvidenceItem
	)
	if ctx.AnalysisIR != nil {
		requestModel = &ctx.AnalysisIR.RequestModel
		answerContract = &ctx.AnalysisIR.AnswerContract
		logBundle = ctx.AnalysisIR.RequestModel.LogTriage
		perfBundle = ctx.AnalysisIR.RequestModel.PerfTrace
	}
	evidenceItems = appendObservationLedgerEvidence(evidenceItems, evidenceLimit, ctx.EvidenceItems...)
	if ctx.Mutable != nil {
		aggregateFacts = ctx.Mutable.StableInvestigationAggregateFacts()
		if logBundle == nil {
			logBundle = ctx.Mutable.LogTriage()
		}
		if perfBundle == nil {
			perfBundle = ctx.Mutable.PerfTrace()
		}
		if ta := ctx.Mutable.TurnAArtifacts(); ta != nil {
			evidenceItems = appendObservationLedgerEvidence(evidenceItems, evidenceLimit, ta.EvidenceItems...)
			toolResults = append([]ToolResult(nil), ta.ToolResults...)
		}
		if len(evidenceItems) < normalizedObservationLedgerEvidenceLimit(evidenceLimit) || evidenceLimit <= 0 {
			evidenceItems = appendObservationLedgerEvidence(evidenceItems, evidenceLimit, ctx.Mutable.EmittedEvidence()...)
		}
	}
	return ObservationLedgerInput{
		EvidenceItems:  evidenceItems,
		AggregateFacts: aggregateFacts,
		ToolResults:    toolResults,
		LogBundle:      logBundle,
		PerfBundle:     perfBundle,
		MCPResponses:   append([]MCPResponse(nil), ctx.MCPResponses...),
		RequestModel:   requestModel,
		AnswerContract: answerContract,
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
		requestModel   *RequestModel
		answerContract *AnswerContract
		logBundle      *LogBundle
		perfBundle     *PerfBundle
		aggregateFacts []AnswerAggregateFact
		toolResults    []ToolResult
		evidenceItems  []EvidenceItem
	)
	if bus.AnalysisIR != nil {
		requestModel = &bus.AnalysisIR.RequestModel
		answerContract = &bus.AnalysisIR.AnswerContract
		logBundle = bus.AnalysisIR.RequestModel.LogTriage
		perfBundle = bus.AnalysisIR.RequestModel.PerfTrace
	}
	evidenceItems = appendObservationLedgerEvidence(evidenceItems, evidenceLimit, bus.EvidenceItems...)
	toolResults = append([]ToolResult(nil), bus.ToolResults...)
	if bus.Mutable != nil {
		aggregateFacts = bus.Mutable.StableInvestigationAggregateFacts()
		if logBundle == nil {
			logBundle = bus.Mutable.LogTriage()
		}
		if perfBundle == nil {
			perfBundle = bus.Mutable.PerfTrace()
		}
		if ta := bus.Mutable.TurnAArtifacts(); ta != nil {
			evidenceItems = appendObservationLedgerEvidence(evidenceItems, evidenceLimit, ta.EvidenceItems...)
			if len(ta.ToolResults) > 0 {
				toolResults = append([]ToolResult(nil), ta.ToolResults...)
			}
		}
	}
	return ObservationLedgerInput{
		EvidenceItems:  evidenceItems,
		AggregateFacts: aggregateFacts,
		ToolResults:    toolResults,
		LogBundle:      logBundle,
		PerfBundle:     perfBundle,
		MCPResponses:   append([]MCPResponse(nil), bus.MCPResponses...),
		RequestModel:   requestModel,
		AnswerContract: answerContract,
	}
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
