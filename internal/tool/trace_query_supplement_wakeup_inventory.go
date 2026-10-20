package tool

import (
	"encoding/json"
	"path/filepath"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Only an explicit finite direct-waker request may add this independent
// inventory. State-only and causal requests keep their existing view plan.
// Search PID deliberately remains a participant selector: the native display
// separately selects exact receivers while retaining original query coverage.
func traceSupplementWakeupInventoryViews(ctx *types.BusContext, f traceSupplementFamilyPresence, views []string) []string {
	if ctx == nil || f.WakeupInventory {
		return views
	}
	var rm *types.RequestModel
	if ctx.AnalysisIR != nil {
		rm = &ctx.AnalysisIR.RequestModel
	} else if ctx.Mutable != nil {
		rm = ctx.Mutable.RequestModel()
	}
	target, _, ok := traceSupplementDeriveTarget(ctx)
	if !types.TraceEventDisplayRequestsDirectWaker(rm) || !traceSupplementRequestedArtifactScope(ctx).HasExplicitTimeWindows() ||
		!ok || target.PID <= 0 || target.TargetScope == "process" {
		return views
	}
	// The bounded-fact branch has at most one state/resource view. Do not
	// expand the shared two-view execution budget if that contract changes.
	if len(views) < 2 {
		return append(views, "event_search")
	}
	return views
}

func traceSupplementIsWakeupInventory(r types.ObservationRecord) bool {
	if !types.IsValidTraceEventSearchInventoryRecord(r) {
		return false
	}
	i := r.EventSearchInventory
	q := i.Query
	return q.View == "event_search" && q.PID > 0 && q.TargetScope != "process" &&
		len(q.EventTypes) == 1 && q.EventTypes[0] == "sched_wakeup" &&
		len(q.EventNames) == 0 && len(q.EventFieldFilters) == 0 && len(q.Patterns) == 0 && q.Pattern == "" &&
		q.SpanName == "" && len(q.TraceMarkActions) == 0 && q.LineStart == 0 && q.LineEnd == 0 &&
		q.TimeStartSet && q.TimeEndSet && i.Coverage.ScopeComplete && i.Coverage.EnumerationComplete
	// RowsComplete remains independent: a capped display is not repaired by
	// repeating the same capped query, and must still disclose omitted rows.
}

func traceSupplementWakeupInventoryWindow(r types.ObservationRecord, start, end float64) bool {
	if !traceSupplementIsWakeupInventory(r) {
		return false
	}
	q := r.EventSearchInventory.Query
	if types.TraceCausalProjectionPrincipalValueSameWindow(q.TimeStart, q.TimeEnd, start, end) {
		return true
	}
	// event_search may execute a producer-owned boundary lookup tolerance.
	// Match its precise requested-window receipt, not an arbitrary nearby or
	// enclosing exploratory query. Display still uses the half-open request.
	if traceSupplementRichNoteValue(r.RichNotes, "matching_window_policy") != "lookup_only_boundary_tolerance" ||
		q.TimeStart > start || q.TimeEnd < end {
		return false
	}
	from, to, ok := types.TraceCausalProjectionParseWindowValue(traceSupplementRichNoteValue(r.RichNotes, "requested_window"))
	return ok && types.TraceCausalProjectionPrincipalValueSameWindow(from, to, start, end)
}

// Reuse the existing current-source display contract, then require the exact
// executed query's source/target/window. A same-number query on another capture,
// a serialized inventory, or stale native generation cannot waive this work.
func traceSupplementCurrentWakeupInventory(ctx *types.BusContext, input types.ObservationLedgerInput, path string, target traceQueryRequestTarget) bool {
	if len(traceSupplementWakeupInventoryViews(ctx, traceSupplementFamilyPresence{}, nil)) == 0 {
		return false
	}
	start, end, explicit := traceSupplementRequestedArtifactScope(ctx).ExplicitTimeWindow()
	if !explicit || target.PID <= 0 || target.TargetScope == "process" {
		return false
	}
	if physical, err := filepath.EvalSymlinks(path); err == nil {
		path = physical
	}
	contract := types.BuildRuntimeMeasurementContract(input)
	if contract == nil {
		return false
	}
	current := map[string]bool{}
	for _, table := range contract.Tables {
		if table.EventData != nil {
			current[table.ObservationID] = true
		}
	}
	for _, results := range [][]types.ToolResult{input.ToolResults, input.SystemTraceSupplementResults} {
		for _, result := range results {
			if !result.Success || result.ToolName != "trace_query" {
				continue
			}
			for _, r := range result.Observations {
				if current[r.ID] && types.RuntimeObservationPresentationSourceKey(input, result, r) != "" &&
					types.RuntimeArtifactComparisonPathEqual(r.SourceRef.Path, path, ctx.RepoRoot) &&
					traceSupplementWakeupInventoryWindow(r, start, end) && r.EventSearchInventory.Query.PID == target.PID {
					return true
				}
			}
		}
	}
	return false
}

func traceSupplementMarshalWindowedParams(p traceSupplementCallParams) ([]byte, error) {
	if p.View != "event_search" {
		return json.Marshal(p)
	}
	return json.Marshal(struct {
		traceSupplementCallParams
		EventTypes []string `json:"event_types"`
		Limit      int      `json:"limit"`
	}{p, []string{"sched_wakeup"}, types.TraceEventSearchInventoryRowLimit})
}
