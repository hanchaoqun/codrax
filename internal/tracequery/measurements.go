package tracequery

import "github.com/hanchaoqun/codrax/internal/tracewire"

const ViewMeasurements = "measurements"
const MeasurementsLimit = 64
const MeasurementsTeaching = "Raw measure records retain exact SQLite values, signed source timestamps/durations and filter metadata. A unique filter reference is not a hardware/CPU/process/thread identity; source_arg_set_id is raw. Units, active-state meaning and physical resource identity require an independently verified protocol. Explicit intervals are clipped to the window; unknown duration is never extended, overlapping rows remain separate. No thread selector, implicit target inheritance, derived GPU statistics or causal authority."

type MeasurementsResult struct {
	Status           string                    `json:"status"`
	SourcePath       string                    `json:"source_path"`
	Window           ProcessMeasurementsWindow `json:"window"`
	Rows             []MeasurementRow          `json:"rows,omitempty"`
	TotalRows        int                       `json:"total_rows"`
	OmittedRows      int                       `json:"omitted_rows"`
	UnpositionedRows int                       `json:"unpositioned_rows"`
	Caveats          []string                  `json:"caveats,omitempty"`
}
type MeasurementRow struct {
	SourcePath     string                    `json:"source_path"`
	Line           int                       `json:"line"`
	SourceLine     int                       `json:"source_line"`
	Record         tracewire.MeasureInterval `json:"record"`
	ClippedStartNS *int64                    `json:"clipped_start_ns,omitempty,string"`
	ClippedEndNS   *int64                    `json:"clipped_end_ns,omitempty,string"`
	Selection      string                    `json:"selection"`
	Unit           string                    `json:"unit"`
}

func measurementSelection(r tracewire.MeasureInterval, start, end int64, inclusive bool) (string, *int64, *int64) {
	ts, ok := r.StartNS.Integer()
	if !ok || ts > end || ts == end && !inclusive {
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

func buildMeasurements(idx *Index, q Query) *MeasurementsResult {
	p := &MeasurementsResult{Status: "unavailable", SourcePath: idx.Path, Window: ProcessMeasurementsWindow{q.TimeStart, q.TimeEnd, q.timeEndBackfilled}, Caveats: []string{MeasurementsTeaching}}
	fail := func(reason string) *MeasurementsResult {
		p.Rows = nil
		p.TotalRows, p.OmittedRows, p.UnpositionedRows = 0, 0, 0
		p.Caveats = append(p.Caveats, reason)
		return p
	}
	if !resourceStackSingleSource(idx) {
		return fail("requires_complete_identity_mapped_single_source")
	}
	if idx.MeasureMalformed > 0 {
		return fail("malformed_measure_carrier")
	}
	if q.PID != 0 || q.Thread != "" || q.ThreadInput != "" || q.ThreadPIDInferred || q.TargetScope == TargetScopeProcess {
		return fail("raw_measurements_do_not_accept_owner_selectors")
	}
	if q.Pattern != "" || len(q.Patterns) > 0 || q.SpanName != "" || len(q.EventTypes) > 0 || len(q.EventNames) > 0 || len(q.EventFieldFilters) > 0 || len(q.TraceMarkActions) > 0 {
		return fail("requires_time_and_line_selectors_only")
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
		if ev.Type != EventMeasureInterval {
			continue
		}
		if ev.PluginFields == nil || ev.PluginFields.Measure == nil || !ev.PluginFields.Measure.Valid() {
			return fail("invalid_measure_record")
		}
		r := *ev.PluginFields.Measure
		if seen[r.RowID] {
			return fail("duplicate_measure_row_identity")
		}
		seen[r.RowID] = true
		if q.LineStart > 0 && ev.Line < q.LineStart || q.LineEnd > 0 && ev.Line > q.LineEnd {
			continue
		}
		if _, ok := r.StartNS.Integer(); !ok {
			p.UnpositionedRows++
			continue
		}
		selection, left, right := measurementSelection(r, start, end, q.timeEndBackfilled)
		if selection == "" {
			continue
		}
		p.TotalRows++
		if len(p.Rows) >= ViewCapacityFor(ViewMeasurements).ClampLimit(q.Limit) {
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
		p.Rows = append(p.Rows, MeasurementRow{source, ev.Line, local, r, left, right, selection, "unknown"})
	}
	if len(seen) == 0 {
		return fail("no_measure_observations")
	}
	p.Status = "available"
	if q.timeStartBackfilled || q.timeEndBackfilled {
		p.Caveats = append(p.Caveats, "Default bounds describe parsed carrier sorting coordinates, not capture completeness; negative/unknown timestamps use sorting coordinate zero. Explicit raw source times remain exact; duration endpoints do not enlarge default bounds.")
	}
	return p
}

func ValidMeasurements(p MeasurementsResult) bool {
	if p.SourcePath == "" || p.TotalRows < 0 || p.OmittedRows < 0 || p.UnpositionedRows < 0 || len(p.Rows) > MeasurementsLimit || p.TotalRows != len(p.Rows)+p.OmittedRows {
		return false
	}
	if p.Status == "unavailable" {
		return len(p.Rows) == 0 && p.TotalRows == 0 && p.OmittedRows == 0 && p.UnpositionedRows == 0
	}
	if p.Status != "available" {
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
		s, left, right := measurementSelection(row.Record, start, end, p.Window.EndInclusive)
		if s == "" || row.Selection != s || !equalProcessMeasurementPointer(left, row.ClippedStartNS) || !equalProcessMeasurementPointer(right, row.ClippedEndNS) {
			return false
		}
	}
	return true
}
