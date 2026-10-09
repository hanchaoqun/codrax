package tracequery

import (
	"math/big"
	"strconv"

	"github.com/hanchaoqun/codrax/internal/tracewire"
)

func processMeasurementNS(seconds float64) (int64, bool) {
	if !isFiniteTraceNumber(seconds) {
		return 0, false
	}
	r, ok := new(big.Rat).SetString(strconv.FormatFloat(seconds, 'f', -1, 64))
	if !ok {
		return 0, false
	}
	r.Mul(r, big.NewRat(1000000000, 1))
	if !r.IsInt() || !r.Num().IsInt64() {
		return 0, false
	}
	return r.Num().Int64(), true
}

func processMeasurementSelection(r tracewire.ProcessMeasureInterval, start, end int64, inclusive bool) (string, *int64, *int64) {
	ts, ok := r.StartNS.Integer()
	if !ok {
		return "", nil, nil
	}
	if ts > end || ts == end && !inclusive {
		return "", nil, nil
	}
	stop, complete := r.EndNS()
	if complete && stop > ts {
		if stop <= start {
			return "", nil, nil
		}
		left, right := max(start, ts), min(end, stop)
		return "interval_overlap", &left, &right
	}
	if ts < start {
		return "", nil, nil
	}
	if complete {
		return "point", &ts, &ts
	}
	return "unknown_duration", &ts, nil
}

func buildProcessMeasurements(idx *Index, q Query) *ProcessMeasurementsResult {
	p := &ProcessMeasurementsResult{Status: "unavailable", SourcePath: idx.Path, Window: ProcessMeasurementsWindow{q.TimeStart, q.TimeEnd, q.timeEndBackfilled}, TargetPID: q.PID, TargetScope: q.TargetScope, Caveats: []string{ProcessMeasurementsTeaching}}
	fail := func(reason string) *ProcessMeasurementsResult {
		p.Rows = nil
		p.TotalRows, p.OmittedRows, p.UnpositionedRows = 0, 0, 0
		p.Caveats = append(p.Caveats, reason)
		return p
	}
	if !resourceStackSingleSource(idx) {
		return fail("requires_complete_identity_mapped_single_source")
	}
	if idx.ProcessMeasureMalformed > 0 {
		return fail("malformed_process_measure_carrier")
	}
	if q.TargetScope != TargetScopeProcess || q.Thread != "" || q.ThreadInput != "" || q.ThreadPIDInferred || q.PID < 0 {
		return fail("process_ownership_does_not_accept_thread_selectors")
	}
	if q.Pattern != "" || len(q.Patterns) > 0 || q.SpanName != "" || len(q.EventTypes) > 0 || len(q.EventNames) > 0 || len(q.EventFieldFilters) > 0 || len(q.TraceMarkActions) > 0 {
		return fail("requires_time_line_and_process_selectors_only")
	}
	start, sok := processMeasurementNS(q.TimeStart)
	end, eok := processMeasurementNS(q.TimeEnd)
	if !sok || !eok || end < start || end == start && !q.timeEndBackfilled {
		return fail("requires_ordered_nanosecond_representable_window")
	}
	seen := map[int64]bool{}
	for _, ev := range idx.Events {
		if q.runCancel.tick() {
			return nil
		}
		if ev.Type != EventProcessMeasureInterval {
			continue
		}
		if ev.PluginFields == nil || ev.PluginFields.ProcessMeasure == nil || !ev.PluginFields.ProcessMeasure.Valid() {
			return fail("invalid_process_measure_record")
		}
		r := *ev.PluginFields.ProcessMeasure
		if seen[r.RowID] {
			p.Rows = nil
			p.TotalRows, p.OmittedRows, p.UnpositionedRows = 0, 0, 0
			return fail("duplicate_process_measure_row_identity")
		}
		seen[r.RowID] = true
		if q.LineStart > 0 && ev.Line < q.LineStart || q.LineEnd > 0 && ev.Line > q.LineEnd {
			continue
		}
		if q.PID > 0 && (r.PID == nil || *r.PID != q.PID) {
			continue
		}
		if _, ok := r.StartNS.Integer(); !ok {
			p.UnpositionedRows++
			continue
		}
		selection, left, right := processMeasurementSelection(r, start, end, q.timeEndBackfilled)
		if selection == "" {
			continue
		}
		p.TotalRows++
		if len(p.Rows) >= ViewCapacityFor(ViewProcessMeasurements).ClampLimit(q.Limit) {
			p.OmittedRows++
			continue
		}
		source, local := idx.Path, ev.Line
		if len(idx.TraceArtifacts) > 0 {
			spans := idx.ResolveArtifactSpans(ev.Line, ev.Line)
			if len(spans) != 1 {
				return fail("source_row_mapping_unavailable")
			}
			source, local = spans[0].SourcePath, spans[0].LocalLineStart
		}
		p.Rows = append(p.Rows, ProcessMeasurementRow{source, ev.Line, local, r, left, right, selection, "unknown"})
	}
	if len(seen) == 0 {
		return fail("no_process_measure_observations")
	}
	p.Status = "available"
	if q.timeStartBackfilled || q.timeEndBackfilled {
		p.Caveats = append(p.Caveats, "Default bounds describe parsed carrier timestamp coordinates, not an independently proven capture range; negative/unknown starts use sorting coordinate zero, and source duration endpoints never enlarge capture bounds.")
	}
	if p.UnpositionedRows > 0 {
		p.Caveats = append(p.Caveats, "Rows without an integer source timestamp cannot be assigned to the requested window; unpositioned_rows is an audit count outside selected total_rows.")
	}
	return p
}

// ValidProcessMeasurements validates exact row selection, not statistical or
// causal meaning. Omitted rows never acquire full-population authority.
func ValidProcessMeasurements(p ProcessMeasurementsResult) bool {
	if p.SourcePath == "" || p.TotalRows < 0 || p.OmittedRows < 0 || p.UnpositionedRows < 0 || len(p.Rows) > ProcessMeasurementsLimit || p.TotalRows != len(p.Rows)+p.OmittedRows {
		return false
	}
	if p.Status == "unavailable" {
		return len(p.Rows) == 0 && p.TotalRows == 0 && p.OmittedRows == 0
	}
	if p.Status != "available" || p.TargetScope != TargetScopeProcess || p.TargetPID < 0 {
		return false
	}
	start, sok := processMeasurementNS(p.Window.StartTs)
	end, eok := processMeasurementNS(p.Window.EndTs)
	if !sok || !eok || end < start || end == start && !p.Window.EndInclusive {
		return false
	}
	seen := map[int64]bool{}
	for _, row := range p.Rows {
		if row.SourcePath == "" || row.Line <= 0 || row.SourceLine <= 0 || row.Unit != "unknown" || !row.Record.Valid() || seen[row.Record.RowID] {
			return false
		}
		seen[row.Record.RowID] = true
		if p.TargetPID > 0 && (row.Record.PID == nil || *row.Record.PID != p.TargetPID) {
			return false
		}
		selection, left, right := processMeasurementSelection(row.Record, start, end, p.Window.EndInclusive)
		if selection == "" || row.Selection != selection || !equalProcessMeasurementPointer(left, row.ClippedStartNS) || !equalProcessMeasurementPointer(right, row.ClippedEndNS) {
			return false
		}
	}
	return true
}

func equalProcessMeasurementPointer(a, b *int64) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}
