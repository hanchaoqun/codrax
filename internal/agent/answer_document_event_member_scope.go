package agent

import (
	"encoding/json"

	"github.com/hanchaoqun/codrax/internal/types"
)

// An event_search member can also occur as a legacy EvidenceFact. Join only
// the producer's exact point-event mirror, never an adjacent observation or a
// causal interval. The full source envelope includes capture, query, clock,
// payload and tool-call identity; physical line numbers alone are not keys.
type answerDocEventMemberKey struct {
	source    string
	line      int
	timestamp float64
	eventType string
}

type answerDocEventMemberRoster map[answerDocEventMemberKey]bool

func answerDocEventMemberSourceKey(source types.ObservationSourceRef) string {
	if source.QueryScopeID == "" || source.PayloadRef == "" {
		return ""
	}
	encoded, err := json.Marshal(source)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func answerDocEventMemberWindowRoster(ctx *types.AgentContext) answerDocEventMemberRoster {
	if ctx == nil || ctx.AnalysisIR == nil ||
		(ctx.AgentName != types.AgentFinalizer && ctx.Stage != types.StageFinalize) {
		return nil
	}
	rm := &ctx.AnalysisIR.RequestModel
	profile, requested := rm.RuntimeQuestionProfile, rm.RuntimeArtifactScopeProfile
	if profile == nil || !profile.SuppressesRootCauseRankingPrompt() || !profile.CarriesBoundedFactFamilies() ||
		requested == nil || (!requested.HasExplicitTimeWindows() && requested.Comparison == nil) {
		return nil // Causal predecessors and business-only selectors retain their existing lanes.
	}
	scopes := types.BuildRuntimeObservationDisplayScopes(types.ObservationLedgerInputFromAgentContext(ctx, 0))
	roster := answerDocEventMemberRoster{}
	// Bridge keys are complete compiler-owned records, not model JSON. Reuse
	// its narrow validated-publication replay so target/Top-N presentation
	// filtering cannot hide the parent receipt. No second full ledger compile
	// or approximate reversal of source/preflight enrichment is needed here.
	for key := range buildTraceEventPublicationBridge(ctx) {
		var record types.ObservationRecord
		if json.Unmarshal([]byte(key), &record) != nil || !types.IsValidTraceEventSearchInventoryRecord(record) {
			continue
		}
		local, qualified := scopes.Scope(record, requested)
		if !qualified || !local.HasExplicitTimeWindows() {
			continue
		}
		source := answerDocEventMemberSourceKey(record.SourceRef)
		if source == "" {
			continue
		}
		for _, row := range record.EventSearchInventory.Rows {
			member, known := answerDocEventMemberIdentity(source, row)
			if !known {
				continue
			}
			retained := types.TraceEventRowWithinRequestedWindows(row, local)
			// A conflicting retained witness prevents exclusion. Partial query
			// inventories can exclude only their explicitly witnessed members;
			// absent members are unknown, never inferred outside the window.
			roster[member] = roster[member] || retained
		}
	}
	return roster
}

func answerDocEventMemberIdentity(source string, row types.TraceEventSearchInventoryRow) (answerDocEventMemberKey, bool) {
	// Measurement ordering can use zero as an unavailable-time sentinel.
	// Such a point cannot prove membership or nonmembership of a mirror.
	if source == "" || row.Line <= 0 || row.EventType == "" ||
		(row.EventType == "measure_interval" || row.EventType == "process_measure_interval") &&
			(!row.SourceTimeKnown || row.TraceTimeSeconds != row.SourceTimeSeconds) {
		return answerDocEventMemberKey{}, false
	}
	return answerDocEventMemberKey{source, row.Line, row.TraceTimeSeconds, row.EventType}, true
}

func (roster answerDocEventMemberRoster) excludes(record types.ObservationRecord) bool {
	if len(roster) == 0 || types.NativeRuntimeFactPresentationKind(record) != "trace_record" ||
		record.ProvenanceLane != types.ObservationProvenanceArtifactSpan ||
		record.Role != types.AnswerAggregateRoleSupportingCoverage ||
		record.ClaimKey != "evidence_fact:"+record.Predicate || record.Predicate == "" ||
		record.Span.LineStart <= 0 || record.Span.LineStart != record.Span.LineEnd ||
		record.Span.StartTs != record.Span.EndTs {
		return false
	}
	member := answerDocEventMemberKey{answerDocEventMemberSourceKey(record.SourceRef), record.Span.LineStart, record.Span.StartTs, record.Predicate}
	retained, witnessed := roster[member]
	return witnessed && !retained
}
