package agent

import (
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/threadidentity"
	"github.com/hanchaoqun/codrax/internal/types"
)

// A query's subject selector is not ownership: searches may match a payload,
// waker, destination or text. Retain a separate, row-owned writing view rather
// than admitting the whole inventory into the principal observation pool.
func renderAnswerDocScopedTraceEventInventories(ctx *types.AgentContext, accepted, principal types.ObservationLedger) string {
	if ctx == nil || ctx.AnalysisIR == nil {
		return renderAnswerDocTraceEventInventories(principal)
	}
	rm := &ctx.AnalysisIR.RequestModel
	p := rm.RuntimeQuestionProfile
	if p == nil || !p.SuppressesRootCauseRankingPrompt() || !p.CarriesBoundedFactFamilies() || !answerDocHasUserRuntimeTarget(rm) {
		return renderAnswerDocTraceEventInventories(principal)
	}
	// Unlike whole-window aggregates, inventories contain individual events.
	// Project those points below instead of rejecting an entire receipt whose
	// executed lookup includes tolerance outside one or more requested windows.
	view := types.ObservationLedger{}
	for _, row := range accepted.Records {
		if types.IsValidTraceEventSearchInventoryRecord(row) {
			view.Records = append(view.Records, row)
		}
	}
	return renderAnswerDocTraceEventInventories(view, rm)
}

type traceInventoryScopeProjection struct {
	InputRows               int    `json:"input_rows"`
	RetainedRows            int    `json:"retained_rows"`
	OwnerRowsExcluded       int    `json:"owner_rows_excluded"`
	WindowRowsExcluded      int    `json:"window_rows_excluded"`
	PromptBudgetRowsOmitted int    `json:"prompt_budget_rows_omitted"`
	Basis                   string `json:"basis"`
}

func traceEventInventoryScopeProjection(record types.ObservationRecord, rm *types.RequestModel) (types.ObservationRecord, *traceInventoryScopeProjection) {
	if rm == nil {
		return record, nil
	}
	i := types.CloneTraceEventSearchInventory(record.EventSearchInventory)
	out := &traceInventoryScopeProjection{InputRows: len(i.Rows), Basis: "observed_emitter_and_request_window_only_not_pairing_or_causality"}
	rows := make([]types.TraceEventSearchInventoryRow, 0, len(i.Rows))
	windows := rm.RuntimeArtifactScopeProfile.ExplicitTimeWindows()
	for _, row := range i.Rows {
		if !traceEventInventoryRowMatchesTarget(row, rm) {
			out.OwnerRowsExcluded++
			continue
		}
		inside := len(windows) == 0
		for _, window := range windows {
			inside = inside || row.TraceTimeSeconds >= *window.TimeStart && row.TraceTimeSeconds < *window.TimeEnd
		}
		if !inside {
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
	for _, target := range rm.RuntimeTargets {
		if types.RuntimeTargetIsExplorationCursorSource(target.Source) {
			continue
		}
		if target.Kind == types.RuntimeTargetKindProcess {
			if row.EmitterTGIDKnown != nil && *row.EmitterTGIDKnown && target.PID > 0 && target.PID <= types.RuntimeTargetMaxPID && row.EmitterTGID == target.PID {
				return true
			}
			continue // source.owner_pid / marker PID do not establish an emitter.
		}
		if row.EmitterTIDKnown == nil || !*row.EmitterTIDKnown || row.EmitterTID <= 0 {
			continue
		}
		one := &types.RequestModel{RuntimeTargets: []types.RuntimeTarget{target}}
		if target.PID > 0 || threadidentity.Parse(target.Thread).HasPID {
			if types.ObservationRecordMatchesUserRuntimeTarget(types.ObservationRecord{Subject: strconv.Itoa(row.EmitterTID)}, one) {
				return true
			}
			continue // an equal comm cannot override a different explicit TID.
		}
		if name := threadidentity.CleanName(target.Thread); name != "" && strings.EqualFold(name, strings.TrimSpace(row.Comm)) {
			return true // observed name only; no unique TID or lifecycle inferred.
		}
	}
	return false
}
