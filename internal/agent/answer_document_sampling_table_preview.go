package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

type samplingTablePreview struct {
	table types.RuntimeMeasurementTable
	rows  int
}

func renderSamplingTablePreview(table types.RuntimeMeasurementTable, rows int) string {
	var b strings.Builder
	preview, _ := json.Marshal(struct {
		Columns []string   `json:"columns"`
		Rows    [][]string `json:"rows"`
	}{table.Columns, table.Rows[:rows]})
	fmt.Fprintf(&b, "- observation_id=%q view=%q label=%q; output_rows=%d; preview_omitted_rows=%d; preview=%s\n",
		table.ObservationID, table.View, table.Label, len(table.Rows), len(table.Rows)-rows, preview)
	// These notes define units, filters, population and unknown-state limits.
	// They are an atomic part of the native preview, never a clipped suffix.
	for _, note := range table.Notes {
		fmt.Fprintf(&b, "  boundary: %s\n", note)
	}
	return b.String()
}

func publishSamplingTablePreview(table types.RuntimeMeasurementTable, rows int, facts *samplingFactRosterPublication, receipt *answerDocNativePresentationReceipt) {
	facts.table(table, rows)
	for _, note := range table.Notes {
		facts.note(table, note)
	}
	receipt.observeMeasurementView(table, rows == len(table.Rows))
}
