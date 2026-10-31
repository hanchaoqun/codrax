package agent

import "github.com/hanchaoqun/codrax/internal/types"

// A query's subject selector is not ownership: searches may match a payload,
// waker, destination or text. Retain a separate, row-owned writing view rather
// than admitting the whole inventory into the principal observation pool.
func renderAnswerDocScopedTraceEventInventories(ctx *types.AgentContext, accepted, principal types.ObservationLedger) string {
	text, _ := renderAnswerDocScopedTraceEventInventoryView(ctx, accepted, principal)
	return text
}

func renderAnswerDocScopedTraceEventInventoryView(ctx *types.AgentContext, accepted, principal types.ObservationLedger) (string, map[string]bool) {
	return renderAnswerDocScopedTraceEventInventoryViewWithPublication(ctx, accepted, principal, nil)
}

func renderAnswerDocScopedTraceEventInventoryViewWithPublication(ctx *types.AgentContext, accepted, principal types.ObservationLedger, publication *answerDocNativePresentationReceipt) (string, map[string]bool) {
	bridge := buildTraceEventPublicationBridge(ctx)
	ledger, request := answerDocTraceEventInventoryDisplayInput(ctx, accepted, principal)
	if request != nil {
		return renderAnswerDocTraceEventInventoryPublicationViewWithPublication(ledger, bridge, publication, request)
	}
	return renderAnswerDocTraceEventInventoryPublicationViewWithPublication(ledger, bridge, publication)
}

// Both full previews and an omitted-preview navigation receipt use this exact
// source pool. A principal aggregate filter must not erase the original query
// whose individually owned event rows have a separate display projection.
func answerDocTraceEventInventoryDisplayInput(ctx *types.AgentContext, accepted, principal types.ObservationLedger) (types.ObservationLedger, *types.RequestModel) {
	if ctx == nil || ctx.AnalysisIR == nil {
		return principal, nil
	}
	rm := &ctx.AnalysisIR.RequestModel
	p := rm.RuntimeQuestionProfile
	scope := rm.RuntimeArtifactScopeProfile
	hasDisplayScope := answerDocHasUserRuntimeTarget(rm) || scope.HasExplicitTimeWindows() || scope != nil && scope.Comparison != nil
	if p == nil || !p.SuppressesRootCauseRankingPrompt() || !p.CarriesBoundedFactFamilies() || !hasDisplayScope {
		return principal, nil
	}
	// Unlike whole-window aggregates, inventories contain individual events.
	// Project those points below instead of rejecting an entire receipt whose
	// executed lookup includes tolerance outside one or more requested windows.
	// The renderer displays only inventories, but needs the independent typed
	// row-local participant receipts to qualify their finite display members.
	return accepted, rm
}

type traceInventoryScopeProjection struct {
	InputRows               int    `json:"input_rows"`
	RetainedRows            int    `json:"retained_rows"`
	OwnerRowsExcluded       int    `json:"owner_rows_excluded"`
	WindowRowsExcluded      int    `json:"window_rows_excluded"`
	PromptBudgetRowsOmitted int    `json:"prompt_budget_rows_omitted"`
	Basis                   string `json:"basis"`
}

func traceEventInventoryScopeProjection(record types.ObservationRecord, rm *types.RequestModel, rosters ...types.TraceEventParticipantDisplayRoster) (types.ObservationRecord, *traceInventoryScopeProjection) {
	if rm == nil {
		return record, nil
	}
	i := types.CloneTraceEventSearchInventory(record.EventSearchInventory)
	out := &traceInventoryScopeProjection{InputRows: len(i.Rows), Basis: "observed_emitter_or_verified_row_participant_and_request_window_only_not_pairing_or_causality"}
	if types.TraceEventDisplayRequestsDirectWaker(rm) {
		out.Basis = "recorded_wakeup_receiver_tid_and_request_window_only_other_events_keep_participant_scope_not_causality"
	}
	var participants types.TraceEventParticipantDisplayRoster
	if len(rosters) == 1 {
		participants = rosters[0]
	}
	rows := make([]types.TraceEventSearchInventoryRow, 0, len(i.Rows))
	for _, row := range i.Rows {
		if !types.TraceEventRowMatchesDisplayTarget(record, row, rm, participants) {
			out.OwnerRowsExcluded++
			continue
		}
		if !types.TraceEventRowWithinRequestedWindows(row, rm.RuntimeArtifactScopeProfile) {
			out.WindowRowsExcluded++
			continue
		}
		rows = append(rows, row)
	}
	out.RetainedRows = len(rows)
	i.Rows = rows
	i.HandoffRowsOmitted = i.Coverage.Emitted - len(rows)
	i.RowsComplete = i.Coverage.ScopeComplete && i.Coverage.EnumerationComplete && len(rows) == i.Coverage.MatchedTotal
	record.EventSearchInventory = i
	return record, out
}

func traceEventInventoryRowMatchesTarget(row types.TraceEventSearchInventoryRow, rm *types.RequestModel) bool {
	// Other callers need actual emitter ownership, not event participation.
	return types.TraceEventRowMatchesEmitterTarget(row, rm)
}
