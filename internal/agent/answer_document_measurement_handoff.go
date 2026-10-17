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
func renderAnswerDocRuntimeMeasurementChoices(ctx *types.AgentContext, publications ...*answerDocNativePresentationReceipt) string {
	var publication *answerDocNativePresentationReceipt
	if len(publications) == 1 {
		publication = publications[0]
	}
	var samplingFacts samplingFactRosterPublication
	view := types.BuildAnswerSemanticViewForAgentContext(ctx)
	if view == nil {
		return ""
	}
	if !view.RuntimeMeasurementContract.Active() {
		return renderAcceptedNativeAnswerDisplay(ctx, view)
	}
	const maxRosterGroups = 32
	choices, omittedGroups := runtimeMeasurementHandoffRoster(view.RuntimeMeasurementContract.Choices(), maxRosterGroups)
	previewCounts := runtimeMeasurementHandoffPreviewRows(choices)
	var b strings.Builder
	if ctx.AnalysisIR != nil && len(types.RuntimeNativeFactDisplaySelections(&ctx.AnalysisIR.RequestModel, view.RuntimeMeasurementContract)) > 0 {
		b.WriteString(types.RuntimeNativeFactDisplayTeaching + "\n")
	}
	b.WriteString(renderAcceptedNativeAnswerDisplay(ctx, view))
	b.WriteString("### 已核对的测量表 / Verified measurement tables\n\n")
	if indices := view.RuntimeMeasurementContract.NativeDisplayDimensionChoices(); len(indices) > 0 {
		fmt.Fprintf(&b, "- Complete native record tables can carry requested member-set display dimensions %v. Bind only the indices actually answered with block-level requested_dimension_indices beside runtime_measurement. One table can carry several dimensions (for example the event roster and its per-row identities); do not retype rows. For an automatically shown table, add an exact model selector with those indices; it replaces the duplicate default display. This is display ownership only, not source-code, causal or investigation-completion evidence.\n", indices)
	}
	if ctx.Mutable != nil && ctx.AnalysisIR != nil && types.RuntimeMeasurementMemberSetDomain(&ctx.AnalysisIR.RequestModel, types.BuildRuntimeSourceAnswerAuthoritySnapshotForAgentContext(ctx, types.ObservationLedger{})) {
		if selected, ok := types.RuntimeMeasurementMemberSetSelections(ctx.Mutable.InvestigationMeasurementMemberSets(), view.RuntimeMeasurementContract, &ctx.AnalysisIR.RequestModel); ok {
			for _, receipt := range selected {
				candidate := types.AnswerBlock{Kind: types.BlockTable, RuntimeMeasurement: &receipt, RequestedDimensionIndices: view.RuntimeMeasurementContract.NativeDisplayDimensionChoices()}
				if _, valid := types.BoundNativeDisplayDimensions(candidate, view.RuntimeMeasurementContract); valid {
					fmt.Fprintf(&b, "- Selected during investigation for a requested member set: observation_id=%q view=%q. Reuse with requested_dimension_indices for the dimensions it actually answers; do not retype its members.\n", receipt.ObservationID, receipt.View)
				} else {
					fmt.Fprintf(&b, "- Selected during investigation for a requested member set: observation_id=%q view=%q. Reuse this table with facet_ids:[\"member_set\"] if it still answers that dimension; do not retype its members.\n", receipt.ObservationID, receipt.View)
				}
			}
		}
	}
	b.WriteString("- For measured summaries, members, distributions or changes over time, prefer the optional runtime_measurement selector instead of retyping numbers. It renders all retained producer rows, including source/window, units, unknown values and omissions. Choose only views needed for the question. Reuse those same source-bound event points and state intervals in prose and diagrams; a wakeup point is not a switch-in, and a window endpoint is not a proven event. Preserve every relevant frequency interval and its published unit rather than reconstructing a timeline from totals or the final sample. Keep interpretation in adjacent prose; this is not a dependency or root-cause proof. No source evidence_items are needed for these runtime tables.\n")
	b.WriteString("- JSON shape: {\"kind\":\"table\",\"runtime_measurement\":{\"observation_id\":\"<published ID>\",\"view\":\"summary\"}}. Choose only published observation_id/view combinations: members lists the producer's accepted interval or endpoint witnesses; distribution uses its labeled measure and denominator (event sizes and time spent at a thread count are different populations); timeline shows its labeled quantity over declared intervals/buckets, respecting its endpoint policy. Bucket peaks, duration-weighted means and endpoint rates are distinct. Omit text/items/columns/diagram/runtime_work_relation on that table; these are alternative payloads, not extra required fields. The system fills data. Never reconstruct members from thread-name resemblance or depth alone.\n")
	for _, table := range choices {
		if table.HasNativeSamplingFacts() {
			b.WriteString("- Native sampling fact choices also supply summary/table/diagram with the same runtime_measurement:{observation_id,view}; omit competing text/items/columns/diagram. summary retains separate event/producer/unit rows with their own denominators; distribution shows query-population quality, not hotspot-subset quality, counted in sample records rather than stack frames. Do not add cross-cohort weights or infer calls, elapsed time or causes. Keep interpretation separate; fact diagrams have no relationship edges.\n")
			break
		}
	}
	for _, table := range choices {
		if table.HasNativeResourceFacts() {
			b.WriteString("- Native resource fact choices supply the summary/table/diagram kinds allowed by the current schema through the same published runtime_measurement pair. summary preserves source events and observed owners; members preserves physical frame rows, original depths and symbol status. Reuse the pair instead of copying names or constructing arrows. Empty queries retain requested identity separately from unknown observed names. Missing depths, repeated depths and unknown symbols stay distinct; a fact diagram has no relationship edges. Example: {\"kind\":\"table\",\"runtime_measurement\":{\"observation_id\":\"<published resource ID>\",\"view\":\"members\"}}. Omit text/items/columns/diagram; keep interpretation separate.\n")
			break
		}
	}
	for _, table := range choices {
		if table.EventData != nil {
			b.WriteString("- Native event facts additionally accept event_selection on an available summary/table/diagram. Example: {\"kind\":\"summary\",\"runtime_measurement\":{\"observation_id\":\"<published event ID>\",\"view\":\"members\",\"event_selection\":{\"row_ids\":[\"<published row ID>\"],\"field_keys\":[\"<published field key>\"]}}}. Reuse this selector for each needed surface instead of copying app/process identities or numbers. For counts, replace field_keys with group_by; the system counts exactly those retained rows. Omitted row_ids means all retained rows, not all matches beyond a display limit. Empty selection uses all retained rows and fields. Choose fields that answer the business question. Omit text/items/columns/diagram and causal metadata; if a principal summary requires trace_causal_claim_caliber, use only no_causal_conclusion. A fact diagram has no causal edges and cannot replace a required relationship diagram. Keep interpretation in a separate block.\n")
			b.WriteString("- Event selectors reuse their own query's selector_row_ids aligned with row_refs in Trace Event Search Inventories; display_rows supplies the native fields. IDs are local to observation_id, not event identity across queries. Omit row_ids for every retained row; never reconstruct unseen values. Each scope_ref resolves an exact note below and applies only to the table referencing it; shared text never merges sources, windows or populations.\n")
			break
		}
	}
	seen := map[string]bool{}
	previewed := map[string]bool{}
	eventCatalogs := map[string]string{}
	eventNotes := map[string]string{}
	previewRows := 0
	for i, table := range choices {
		writtenNotes := map[string]bool{}
		count := previewCounts[i]
		if count > 0 {
			previewed[table.ObservationID] = true
		}
		seen[table.ObservationID] = true
		if table.EventData != nil {
			fields, _ := json.Marshal(table.EventData.Fields)
			catalog := eventCatalogs[string(fields)]
			if catalog == "" {
				catalog = fmt.Sprintf("event-fields-%d", len(eventCatalogs)+1)
				eventCatalogs[string(fields)] = catalog
				fmt.Fprintf(&b, "- native_event_fields %s=%s\n", catalog, fields)
			}
			var scopeRefs []string
			for _, note := range table.Notes {
				ref := eventNotes[note]
				if ref == "" {
					ref = fmt.Sprintf("event-scope-%d", len(eventNotes)+1)
					eventNotes[note] = ref
					fmt.Fprintf(&b, "- %s: %s\n", ref, note)
				}
				scopeRefs = append(scopeRefs, ref)
			}
			fmt.Fprintf(&b, "- observation_id=%q view=%q; field_catalog=%s; retained_rows=%d; scope_refs=%s\n", table.ObservationID, table.View, catalog, len(table.Rows), strings.Join(scopeRefs, ","))
			continue
		}
		rows := table.Rows[:count]
		previewRows += count
		preview, _ := json.Marshal(struct {
			Columns []string   `json:"columns"`
			Rows    [][]string `json:"rows"`
		}{table.Columns, rows})
		fmt.Fprintf(&b, "- observation_id=%q view=%q label=%q; output_rows=%d; preview_omitted_rows=%d; preview=%s\n",
			table.ObservationID, table.View, table.Label, len(table.Rows), len(table.Rows)-len(rows), preview)
		samplingFacts.table(table, len(rows))
		candidate := types.AnswerBlock{Kind: types.BlockTable, RuntimeMeasurement: &types.AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View}, RequestedDimensionIndices: view.RuntimeMeasurementContract.NativeDisplayDimensionChoices()}
		if indices, valid := types.BoundNativeDisplayDimensions(candidate, view.RuntimeMeasurementContract); valid {
			fmt.Fprintf(&b, "  eligible requested_dimension_indices=%v (choose only dimensions this table answers).\n", indices)
		}
		if table.MemberSet != nil {
			fmt.Fprintf(&b, "  population=%q; member_rows=%d/%d; complete_population=%t (not capture completeness).\n", table.MemberSet.PopulationID, len(table.MemberSet.RowIDs), table.MemberSet.TotalRows, table.MemberSet.Complete)
		}
		// Keep the ruler beside every selector; opaque IDs are not sufficient
		// teaching for multiple captures/windows with the same device label.
		if len(table.Notes) >= 2 {
			fmt.Fprintf(&b, "  scope: %s; %s\n", table.Notes[0], table.Notes[1])
			samplingFacts.note(table, table.Notes[0])
			samplingFacts.note(table, table.Notes[1])
			writtenNotes[table.Notes[0]], writtenNotes[table.Notes[1]] = true, true
		}
		// Empty summaries still carry the producer's unavailable/empty status.
		// No preview rows is not evidence of a measured zero.
		if table.View == types.RuntimeMeasurementSummary && (count > 0 || len(table.Rows) == 0) && len(table.Notes) > 2 {
			for _, note := range table.Notes[2:] {
				fmt.Fprintf(&b, "  boundary: %s\n", note)
				samplingFacts.note(table, note)
				writtenNotes[note] = true
			}
		} else if count > 0 && len(table.Notes) >= 2 {
			for _, note := range table.Notes[len(table.Notes)-2:] {
				fmt.Fprintf(&b, "  boundary: %s\n", note)
				samplingFacts.note(table, note)
				writtenNotes[note] = true
			}
		}
		complete := count == len(table.Rows)
		for _, note := range table.Notes {
			complete = complete && writtenNotes[note]
		}
		publication.observeMeasurementView(table, complete)
	}
	fmt.Fprintf(&b, "- Preview groups=%d; additional selectable groups not previewed=%d; selector roster groups omitted=%d; preview rows=%d/128. Source/query groups and their views share the row budget; summaries have priority for remaining rows. Whole small tables can fit without a per-table cap. Preview omissions never mean absent events, missing output rows, or complete capture. Select the full source-bound table by ID/view for omitted distributions, members or time buckets; do not reconstruct unseen rows.\n\n", len(previewed), len(seen)-len(previewed), omittedGroups, previewRows)
	samplingFacts.publish(ctx, publication)
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

// Reuse the fact presentation allocator, with exact producer observation IDs
// as source/query groups. This only selects previews, never output membership.
func runtimeMeasurementHandoffPreviewRows(tables []types.RuntimeMeasurementTable) []int {
	groups := make([]types.PresentationRowGroup, len(tables))
	for i, table := range tables {
		priority := 1
		if table.View == types.RuntimeMeasurementSummary {
			priority = 0
		}
		groups[i] = types.PresentationRowGroup{Key: table.ObservationID + ":" + string(table.View), ParentKey: table.ObservationID, Rows: len(table.Rows), Priority: priority}
		if table.EventData != nil {
			groups[i].Rows = 0 // Already shared through the inventory's row_refs.
		}
	}
	return types.AllocatePresentationRows(groups, 128)
}
