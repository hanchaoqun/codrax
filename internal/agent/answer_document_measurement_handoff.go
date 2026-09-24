package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The selector roster comes from the same fresh contract as emit/patch. A
// bounded preview guides analysis, but never changes the selected output rows.
// Summaries get a fair share before details: an earlier measurement family
// must not consume every numeric preview of a later family.
func renderAnswerDocRuntimeMeasurementChoices(ctx *types.AgentContext) string {
	view := types.BuildAnswerSemanticViewForAgentContext(ctx)
	if view == nil || !view.RuntimeMeasurementContract.Active() {
		return ""
	}
	const maxRosterGroups = 32
	choices, omittedGroups := runtimeMeasurementHandoffRoster(view.RuntimeMeasurementContract.Choices(), maxRosterGroups)
	previewCounts := runtimeMeasurementHandoffPreviewRows(choices)
	var b strings.Builder
	b.WriteString("### 已核对的测量表 / Verified measurement tables\n\n")
	b.WriteString("- For measured summaries, members, distributions or changes over time, prefer the optional runtime_measurement selector instead of retyping numbers. It renders all retained producer rows, including source/window, units, unknown values and omissions. Choose only views needed for the question. Keep interpretation in adjacent prose; this is not a dependency or root-cause proof. No source evidence_items are needed for these runtime tables.\n")
	b.WriteString("- JSON shape: {\"kind\":\"table\",\"runtime_measurement\":{\"observation_id\":\"<published ID>\",\"view\":\"summary\"}}. Choose only published observation_id/view combinations: members lists accepted paired endpoints; distribution groups measured sizes; timeline shows the producer's labeled quantity over its declared intervals/buckets, respecting its endpoint policy (request depth or endpoint event/byte rates are different measures). Omit text/items/columns/diagram/runtime_work_relation on that table; these are alternative payloads, not extra required fields. The system fills data. Never reconstruct members from thread-name resemblance or depth alone.\n")
	seen := map[string]bool{}
	previewed := map[string]bool{}
	previewRows := 0
	for i, table := range choices {
		count := previewCounts[i]
		if count > 0 {
			previewed[table.ObservationID] = true
		}
		seen[table.ObservationID] = true
		rows := table.Rows[:count]
		previewRows += count
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
		if count > 0 && table.View == types.RuntimeMeasurementSummary && len(table.Notes) > 2 {
			for _, note := range table.Notes[2:] {
				fmt.Fprintf(&b, "  boundary: %s\n", note)
			}
		} else if count > 0 && len(table.Notes) >= 2 {
			for _, note := range table.Notes[len(table.Notes)-2:] {
				fmt.Fprintf(&b, "  boundary: %s\n", note)
			}
		}
	}
	fmt.Fprintf(&b, "- Preview groups=%d; additional selectable groups not previewed=%d; selector roster groups omitted=%d; preview rows=%d/128. Summaries receive rows before detail views; each table preview has at most 4 rows. Preview omissions never mean absent events, missing output rows, or complete capture. Select the full source-bound table by ID/view for omitted distributions, members or time buckets; do not reconstruct unseen rows.\n\n", len(previewed), len(seen)-len(previewed), omittedGroups, previewRows)
	return b.String()
}

func runtimeMeasurementHandoffRoster(tables []types.RuntimeMeasurementTable, limit int) ([]types.RuntimeMeasurementTable, int) {
	seen, omitted := map[string]bool{}, map[string]bool{}
	var out []types.RuntimeMeasurementTable
	for _, table := range tables {
		if !seen[table.ObservationID] && len(seen) == limit {
			omitted[table.ObservationID] = true
			continue
		}
		seen[table.ObservationID] = true
		out = append(out, table)
	}
	return out, len(omitted)
}

// Round-robin within each priority tier. Only the schema-validated view chooses
// the tier; neither question words, IDs, family names nor noisy ranks do so.
// The roster limit guarantees up to four summary rows for every listed group.
func runtimeMeasurementHandoffPreviewRows(tables []types.RuntimeMeasurementTable) []int {
	counts := make([]int, len(tables))
	remaining := 128
	for _, summary := range []bool{true, false} {
		for row := 0; row < 4; row++ {
			for i, table := range tables {
				if (table.View == types.RuntimeMeasurementSummary) != summary || len(table.Rows) <= row {
					continue
				}
				if remaining == 0 {
					return counts
				}
				counts[i]++
				remaining--
			}
		}
	}
	return counts
}
