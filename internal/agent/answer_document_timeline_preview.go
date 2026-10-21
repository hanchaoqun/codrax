package agent

import "github.com/hanchaoqun/codrax/internal/types"

// measurementHandoffPreview changes only which retained rows fit an existing
// preview allocation. The trusted producer's typed timeline enables this
// policy independently of default output layout; no cell text infers it.
// Indices remain in producer order, including tables grouped by CPU/subject.
type measurementHandoffPreview struct {
	Columns                []string   `json:"columns"`
	Rows                   [][]string `json:"rows"`
	SourceRowIndices       []int      `json:"source_row_indices,omitempty"`
	RowIndexBase           *int       `json:"row_index_base,omitempty"`
	Selection              string     `json:"selection,omitempty"`
	SeriesBoundaryCoverage string     `json:"series_boundary_coverage,omitempty"`
}

func runtimeMeasurementHandoffPreview(table types.RuntimeMeasurementTable, count int) measurementHandoffPreview {
	if count < 0 {
		count = 0
	}
	if count > len(table.Rows) {
		count = len(table.Rows)
	}
	p := measurementHandoffPreview{Columns: table.Columns, Rows: table.Rows[:count]}
	if table.View != types.RuntimeMeasurementTimeline {
		return p
	}
	zero := 0
	p.RowIndexBase = &zero
	p.Selection = "all_retained_rows"
	if count < len(table.Rows) {
		p.Selection = "table_boundary_sample"
		if count == 0 {
			p.Selection = "no_rows"
		} else if count == 1 {
			p.Selection = "first_retained_row"
		}
		// Whole-table endpoints are not the endpoints of each independent
		// series. Do not infer per-subject completeness from this preview.
		p.SeriesBoundaryCoverage = "not_claimed"
	}
	p.Rows = make([][]string, 0, count)
	for i := 0; i < count; i++ {
		index := i
		if count > 1 && count < len(table.Rows) && i == count-1 {
			index = len(table.Rows) - 1
		}
		p.SourceRowIndices = append(p.SourceRowIndices, index)
		p.Rows = append(p.Rows, table.Rows[index])
	}
	return p
}
