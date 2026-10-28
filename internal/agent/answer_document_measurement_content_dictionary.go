package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// A display-body key is not a preview/publication identity. The independent
// selectors and their full query receipts remain published beside the body.
// Keep all retained rows, units, notes and population metadata, not only the
// small preview that happens to fit. Scope, target and physical generation
// must also agree. No query authority is derived from equal display bytes.
func runtimeMeasurementContentKey(source, predicate string, p types.RuntimeMeasurementPublication, table types.RuntimeMeasurementTable) string {
	if source == "" || !types.RuntimeMeasurementPredicateIsRegistered(predicate) || !table.IsValid() || table.ObservationID != p.ObservationID || table.HasNativeSamplingFacts() || table.EventData != nil {
		return ""
	}
	copy := table.Clone()
	copy.ObservationID = ""
	if copy.DisplayPopulation != nil {
		if copy.DisplayPopulation.QueryScopeID != p.Source.QueryScopeID {
			return ""
		}
		copy.DisplayPopulation.QueryScopeID = ""
	}
	scope := p.Source
	scope.QueryScopeID, scope.RawRef, scope.PayloadRef = "", "", ""
	raw, err := json.Marshal([]any{source, predicate, p.Version, scope, copy})
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(raw)
	return "measurement-content:" + hex.EncodeToString(digest[:])
}

type runtimeMeasurementContentWitness struct {
	key   string
	query string
}

// Only the current private native source and the exact unmodified contract
// table can publish a body. Restored JSON, memo, stale generations, conflicting
// selectors and request-local projections cannot borrow this route.
func runtimeMeasurementContentWitnesses(ctx *types.AgentContext, tables []types.RuntimeMeasurementTable, previewKeys []string) []runtimeMeasurementContentWitness {
	out := make([]runtimeMeasurementContentWitness, len(tables))
	wanted := map[string]bool{}
	for _, table := range tables {
		if !table.HasNativeSamplingFacts() && table.EventData == nil {
			wanted[table.ObservationID] = true
		}
	}
	input := types.ObservationLedgerInputFromAgentContext(ctx, 0)
	type witness struct {
		runtimeMeasurementContentWitness
		table string
	}
	witnesses := map[samplingFactViewKey]witness{}
	conflicts := map[samplingFactViewKey]bool{}
	for _, results := range [][]types.ToolResult{input.ToolResults, input.SystemTraceSupplementResults} {
		for _, result := range results {
			for _, record := range result.Observations {
				if !wanted[record.ID] {
					continue
				}
				p, valid := types.DecodeRuntimeMeasurementPublication(record)
				if !valid {
					continue
				}
				source := types.RuntimeObservationPresentationSourceKey(input, result, record)
				query, _ := json.Marshal(p.Source)
				for _, table := range p.Tables {
					selector := samplingFactViewKey{table.ObservationID, table.View}
					raw, _ := json.Marshal(table)
					candidate := witness{runtimeMeasurementContentWitness{runtimeMeasurementContentKey(source, record.Predicate, p, table), string(query)}, string(raw)}
					if prior, found := witnesses[selector]; found && prior != candidate {
						conflicts[selector] = true
					}
					witnesses[selector] = candidate
				}
			}
		}
	}
	groups := map[string]map[string]bool{}
	for i, table := range tables {
		selector := samplingFactViewKey{table.ObservationID, table.View}
		w := witnesses[selector]
		raw, _ := json.Marshal(table)
		if conflicts[selector] || w.key == "" || w.table != string(raw) {
			continue
		}
		out[i] = w.runtimeMeasurementContentWitness
		if groups[w.key] == nil {
			groups[w.key] = map[string]bool{}
		}
		groups[w.key][previewKeys[i]] = true
	}
	for i, w := range out {
		// Existing same-query preview aliases keep their stricter contract.
		// Only a genuinely different publication identity needs this layer.
		if len(groups[w.key]) < 2 {
			out[i] = runtimeMeasurementContentWitness{}
		}
	}
	return out
}

type runtimeMeasurementContentPreview struct {
	ref  string
	rows int
}

type runtimeMeasurementContentDictionary struct {
	bodies  map[string]runtimeMeasurementContentPreview
	queries map[string]string
}

// Write definitions before references, with the complete selector navigation
// surviving as one record. A failed budget reservation never publishes a body
// receipt or makes a later query look complete. Independent query metadata is
// not borrowed from the first table even when every display byte agrees.
func (d *runtimeMeasurementContentDictionary) write(b *strings.Builder, w runtimeMeasurementContentWitness, table types.RuntimeMeasurementTable, count int, sourceTitle, dimensions string, notes runtimeMeasurementNoteNavigation, publication *answerDocNativePresentationReceipt) (bool, int, int) {
	prior, exists := d.bodies[w.key]
	if exists {
		count = prior.rows
	}
	contentRef := prior.ref
	var body string
	var pendingNotes runtimeMeasurementNoteNavigation
	if !exists {
		contentRef = fmt.Sprintf("measurement-content-%d", len(d.bodies)+1)
		// The full note list is part of the body key, but identical note bytes
		// across its summary/members/timeline views need only one definition.
		// Work on a tentative dictionary until all writes fit their budgets.
		pendingNotes = runtimeMeasurementNoteNavigation{}
		for note, ref := range notes {
			pendingNotes[note] = ref
		}
		var definitions strings.Builder
		noteRefs := pendingNotes.references(&definitions, table.Notes)
		payload, _ := json.Marshal(struct {
			Preview  measurementHandoffPreview `json:"preview"`
			NoteRefs json.RawMessage           `json:"note_refs"`
		}{runtimeMeasurementHandoffPreview(table, count), json.RawMessage(noteRefs)})
		body = definitions.String() + fmt.Sprintf("- native_measurement_content %s=%s\n", contentRef, payload)
	}
	queryRef := d.queries[w.query]
	var definition string
	if queryRef == "" {
		queryRef = fmt.Sprintf("measurement-query-%d", len(d.queries)+1)
		definition = fmt.Sprintf("- native_measurement_query %s=%s\n", queryRef, w.query)
	}
	navigation := fmt.Sprintf("- observation_id=%q view=%q label=%q%s; query_receipt=%s; content_ref=%s; output_rows=%d; preview_omitted_rows=%d\n", table.ObservationID, table.View, table.Label, sourceTitle, queryRef, contentRef, len(table.Rows), len(table.Rows)-count)
	if table.MemberSet != nil {
		raw, _ := json.Marshal(table.MemberSet)
		navigation += "  member_set=" + string(raw) + "\n"
	}
	if table.DisplayPopulation != nil {
		raw, _ := json.Marshal(table.DisplayPopulation)
		navigation += "  display_population=" + string(raw) + "\n"
	}
	navigation += dimensions
	// Navigation is reserved before any actual write; the body then uses its
	// independent detail budget. Orphan source titles are harmless metadata.
	if publication != nil && len(definition)+len(navigation) > nativePresentationNavigationByteLimit-publication.navigationBytes {
		publication.navigationOmitted++
		publication.observeMeasurementView(table, false)
		return false, 0, 0
	}
	if !publication.takePresentationBytes(len(body)) {
		publication.writePresentationNavigation(b, fmt.Sprintf("- observation_id=%q view=%q; output_rows=%d; preview_omitted_rows=%d; preview_not_shown=shared_native_detail_budget%s\n", table.ObservationID, table.View, len(table.Rows), len(table.Rows), sourceTitle))
		publication.observeMeasurementView(table, false)
		return false, 0, 0
	}
	b.WriteString(body)
	publication.writePresentationNavigation(b, definition+navigation)
	if d.bodies == nil {
		d.bodies = map[string]runtimeMeasurementContentPreview{}
		d.queries = map[string]string{}
	}
	d.bodies[w.key] = runtimeMeasurementContentPreview{contentRef, count}
	d.queries[w.query] = queryRef
	for note, ref := range pendingNotes {
		notes[note] = ref
	}
	publication.observeMeasurementViewDetails(table, count == len(table.Rows), true)
	newRows := 0
	if !exists {
		newRows = count
	}
	return true, count, newRows
}
