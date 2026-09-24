package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The selector roster comes from the same fresh contract as emit/patch. A
// bounded preview guides analysis, but never changes the selected output rows.
func renderAnswerDocRuntimeMeasurementChoices(ctx *types.AgentContext) string {
	view := types.BuildAnswerSemanticViewForAgentContext(ctx)
	if view == nil || !view.RuntimeMeasurementContract.Active() {
		return ""
	}
	const maxGroups = 8
	const maxRosterGroups = 32
	const previewRows = 4
	var b strings.Builder
	b.WriteString("### 已核对的测量表 / Verified measurement tables\n\n")
	b.WriteString("- For measured summaries, members, distributions or changes over time, prefer the optional runtime_measurement selector instead of retyping numbers. It renders all retained producer rows, including source/window, units, unknown values and omissions. Choose only views needed for the question. Keep interpretation in adjacent prose; this is not a dependency or root-cause proof. No source evidence_items are needed for these runtime tables.\n")
	b.WriteString("- JSON shape: {\"kind\":\"table\",\"runtime_measurement\":{\"observation_id\":\"<published ID>\",\"view\":\"summary\"}}. Choose only published pairs: members lists accepted paired endpoints; distribution groups measured sizes; timeline shows the producer's labeled quantity over its declared intervals/buckets, respecting its endpoint policy (request depth or endpoint event/byte rates are different measures). Omit text/items/columns/diagram/runtime_work_relation on that table; these are alternative payloads, not extra required fields. The system fills data. Never reconstruct members from thread-name resemblance or depth alone.\n")
	seen := map[string]bool{}
	omitted := map[string]bool{}
	previewed := map[string]bool{}
	for _, table := range view.RuntimeMeasurementContract.Choices() {
		if !seen[table.ObservationID] && len(seen) == maxRosterGroups {
			omitted[table.ObservationID] = true
			continue
		}
		if !seen[table.ObservationID] && len(previewed) < maxGroups {
			previewed[table.ObservationID] = true
		}
		seen[table.ObservationID] = true
		rows := table.Rows
		if !previewed[table.ObservationID] {
			rows = nil // Retain selector identity and ruler, not another data preview.
		}
		if len(rows) > previewRows {
			rows = rows[:previewRows]
		}
		preview, _ := json.Marshal(struct {
			Columns []string   `json:"columns"`
			Rows    [][]string `json:"rows"`
		}{table.Columns, rows})
		fmt.Fprintf(&b, "- observation_id=%q view=%q label=%q; output_rows=%d; preview_omitted_rows=%d; preview=%s\n",
			table.ObservationID, table.View, table.Label, len(table.Rows), len(table.Rows)-len(rows), preview)
		// Keep the ruler beside every selector; opaque IDs are not sufficient
		// teaching for multiple captures/windows with the same device label.
		if len(table.Notes) >= 2 {
			fmt.Fprintf(&b, "  scope: %s; %s\n", table.Notes[0], table.Notes[1])
		}
		if previewed[table.ObservationID] && table.View == types.RuntimeMeasurementSummary && len(table.Notes) > 2 {
			for _, note := range table.Notes[2:] {
				fmt.Fprintf(&b, "  boundary: %s\n", note)
			}
		} else if previewed[table.ObservationID] && len(table.Notes) >= 2 {
			for _, note := range table.Notes[len(table.Notes)-2:] {
				fmt.Fprintf(&b, "  boundary: %s\n", note)
			}
		}
	}
	fmt.Fprintf(&b, "- Preview groups=%d; additional selectable groups not previewed=%d; selector roster groups omitted=%d. Preview omissions never mean absent events, missing output rows, or complete capture. Full source-bound output is selected by ID/view.\n\n", len(previewed), len(seen)-len(previewed)+len(omitted), len(omitted))
	return b.String()
}
