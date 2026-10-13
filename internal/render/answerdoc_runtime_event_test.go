package render

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/types"
)

func eventRenderContract(t *testing.T, comm string) (*types.RuntimeMeasurementContract, string) {
	t.Helper()
	r := types.ObservationRecord{
		ID: "native-long-internal-query-identity#event_search_inventory", Producer: "trace_query", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
		Role: types.AnswerAggregateRoleSupportingCoverage, GroundingPolicy: types.ClaimGroundingHard,
		ProvenanceLane: types.ObservationProvenanceArtifactSpan, ClaimAuthority: types.ObservationClaimAuthorityDirectObservation,
		Predicate: types.TraceEventSearchInventoryPredicate, ObservedAt: "2026-10-13T12:00:00Z",
		SourceRef: types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, Path: "/capture.trace", PayloadRef: "/result.json", QueryScopeID: "native-query", QueryWindowKnown: true, QueryWindowStartTs: 1, QueryWindowEndTs: 1.5},
		EventSearchInventory: &types.TraceEventSearchInventory{
			SchemaVersion: 1, QueryScopeID: "native-query",
			Query:        types.TraceEventSearchInventoryQuery{View: "event_search", TimeStart: 1, TimeEnd: 1.5, TimeStartSet: true, TimeEndSet: true},
			Coverage:     types.TraceEventSearchInventoryCoverage{ScopeKind: "selected_window", ScopeComplete: true, MatchedTotal: 4, Emitted: 4, EnumerationComplete: true},
			RowsComplete: true, Caveats: []string{"source /capture.trace; selected [1, 1.5) s; payload appid is not emitter identity"},
		},
	}
	for n, tuple := range [][4]int64{{27599, 1020000000, 1040000000, 3}, {27599, 1100000000, 1160000000, 5}, {41002, 1180000000, 1240000000, 4}, {41002, 1320000000, 1420000000, 7}} {
		r.EventSearchInventory.Rows = append(r.EventSearchInventory.Rows, types.TraceEventSearchInventoryRow{
			Line: n + 1, SourcePath: "/capture.trace", LocalLine: n + 1, EventType: "trace_mark", EventName: "jank_event_sync", Comm: comm,
			TraceTimeSeconds: float64(tuple[1]) / 1e9, Raw: "source event",
			JankEvent: &types.TraceEventSearchJankEvent{TimeDomainStatus: types.TraceJankSourceClock, Values: &types.TraceEventSearchJankValues{
				AppID: tuple[0], StartTSNS: tuple[1], EndTSNS: tuple[2], JankFrames: tuple[3], ReportedDurationNS: tuple[2] - tuple[1],
			}},
		})
	}
	p, ok := types.BuildTraceEventRuntimeMeasurementPublication(r)
	if !ok {
		t.Fatal("native event publication rejected")
	}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	r.RichNotes = []string{types.TraceNoteKeyRuntimeMeasurement + "=" + string(raw)}
	decoded, ok := types.DecodeRuntimeMeasurementPublication(r)
	if !ok {
		t.Fatal("native event transport did not rebind")
	}
	return &types.RuntimeMeasurementContract{Tables: decoded.Tables}, r.ID
}

func eventRenderReceipt(t *testing.T, c *types.RuntimeMeasurementContract, id string, selection *types.RuntimeEventSelection) *types.AnswerRuntimeMeasurementReceipt {
	t.Helper()
	r := &types.AnswerRuntimeMeasurementReceipt{ObservationID: id, View: types.RuntimeMeasurementMembers, EventSelection: selection}
	if !types.BindRuntimeMeasurementReceipt(r, c) || !r.IsBound() {
		t.Fatal("event selection did not bind")
	}
	return r
}

func TestRuntimeEventSameFactsAcrossSummaryTableDiagram(t *testing.T) {
	c, id := eventRenderContract(t, "ui-worker")
	for name, selection := range map[string]*types.RuntimeEventSelection{
		"fields": {RowIDs: []string{"event-4", "event-3"}, FieldKeys: []string{"jank.appid", "jank.start_ts_ns", "jank.end_ts_ns", "jank.jank_frames"}},
		"groups": {GroupBy: []string{"jank.appid"}},
	} {
		t.Run(name, func(t *testing.T) {
			receipt := eventRenderReceipt(t, c, id, selection)
			want := [][]string{{"41002", "1180000000", "1240000000", "4"}, {"41002", "1320000000", "1420000000", "7"}}
			if name == "groups" {
				want = [][]string{{"27599", "2"}, {"41002", "2"}}
			}
			if !reflect.DeepEqual(receipt.BoundTable.Rows, want) {
				t.Fatalf("fixture's native selector returned wrong membership: got=%v want=%v", receipt.BoundTable.Rows, want)
			}
			before := receipt.Clone()
			for _, kind := range []types.AnswerBlockKind{types.BlockSummary, types.BlockTable, types.BlockDiagram} {
				block := types.AnswerBlock{ID: "facts", Kind: kind, RuntimeMeasurement: receipt}
				surfaces := RenderAnswerDocumentSurfaces(&types.AnswerDocumentV2{Blocks: []types.AnswerBlock{block}}, "en")
				for _, column := range receipt.BoundTable.Columns {
					if !strings.Contains(surfaces.Answer, measurementPlainText(column)) {
						t.Fatalf("%s lost column %q: %s", kind, column, surfaces.Answer)
					}
				}
				for _, row := range receipt.BoundTable.Rows {
					for _, value := range row {
						if !strings.Contains(surfaces.Primary, value) || !strings.Contains(surfaces.Answer, value) {
							t.Fatalf("%s lost value %q: %s", kind, value, surfaces.Answer)
						}
					}
				}
				for _, note := range receipt.BoundTable.Notes {
					if !strings.Contains(surfaces.Answer, measurementPlainText(note)) {
						t.Fatalf("%s lost source/window/coverage note %q", kind, note)
					}
				}
				if kind == types.BlockDiagram {
					body := eventRenderedDiagram(t, surfaces.Answer)
					if len(mermaidcompat.ParseEdges(body)) != 0 || strings.Contains(body, id) || strings.Contains(body, "event-4") {
						t.Fatalf("fact graph invented topology or exposed internal IDs: %s", body)
					}
					for n, row := range receipt.BoundTable.Rows {
						if !strings.Contains(body, fmt.Sprintf("N%d[\"", n+1)) {
							t.Fatalf("fact graph lost short row node: %s", body)
						}
						line := strings.Split(strings.Split(body, fmt.Sprintf("N%d[\"", n+1))[1], "\n")[0]
						for _, cell := range row {
							if !strings.Contains(line, cell) {
								t.Fatalf("appid/time tuple split across nodes: row=%v node=%s", row, line)
							}
						}
					}
					if rendered := RenderMermaidBlocks("```mermaid\n" + body + "\n```"); strings.Contains(rendered, "# ⚠") || strings.Contains(rendered, "# ·") || !strings.Contains(rendered, "```text") {
						t.Fatalf("generated graph did not render: %s", rendered)
					}
				}
			}
			if !reflect.DeepEqual(receipt, before) {
				t.Fatal("render mutated bound selection")
			}
		})
	}
}

func eventRenderedDiagram(t *testing.T, answer string) string {
	t.Helper()
	parts := strings.Split(answer, "```mermaid\n")
	if len(parts) != 2 {
		t.Fatalf("expected one Mermaid diagram: %s", answer)
	}
	return strings.Split(parts[1], "\n```")[0]
}

func TestRuntimeEventRendererLiteralLabelsAndFailClosed(t *testing.T) {
	const value = "worker\"] --> injected[\"bad\n```mermaid\nA --> B\n``` <script> &quot; C:\\tmp\\"
	c, id := eventRenderContract(t, value)
	r := eventRenderReceipt(t, c, id, &types.RuntimeEventSelection{RowIDs: []string{"event-4"}, FieldKeys: []string{"event.comm", "jank.appid"}})
	for _, kind := range []types.AnswerBlockKind{types.BlockSummary, types.BlockDiagram} {
		block := types.AnswerBlock{ID: "fact", Kind: kind, RuntimeMeasurement: r, Text: "UNTRUSTED NARRATIVE", Diagram: &types.AnswerDiagramBlock{Body: "flowchart LR\nBAD --> WRONG"}}
		out, handled := RenderRuntimeEventMeasurementBlock(block, "en")
		if !handled || strings.Contains(out, "UNTRUSTED") || strings.Contains(out, "BAD --> WRONG") || strings.Contains(out, "<script>") {
			t.Fatalf("native renderer consumed freeform body: %s", out)
		}
		if kind == types.BlockDiagram {
			body := eventRenderedDiagram(t, out)
			if len(mermaidcompat.ParseEdges(body)) != 0 || strings.Count(body, "\n") != 1 {
				t.Fatalf("data escaped its node label: %s", body)
			}
			if !strings.Contains(body, "&lt;script&gt;") || !strings.Contains(body, "&amp;quot;") || !strings.Contains(body, "&#96;") || !strings.Contains(body, "&quot;") {
				t.Fatalf("literal encoding lost: %s", body)
			}
			if rendered := RenderMermaidBlocks("```mermaid\n" + body + "\n```"); strings.Contains(rendered, "# ⚠") || strings.Contains(rendered, "# ·") {
				t.Fatalf("literal label could not be rendered safely: %s", rendered)
			}
		}
		stale := r.Clone()
		stale.EventSelection.RowIDs = []string{"event-3"}
		block.RuntimeMeasurement = stale
		out, handled = RenderRuntimeEventMeasurementBlock(block, "en")
		if !handled || !strings.Contains(out, "not bound") || strings.Contains(out, "41002") || strings.Contains(out, "UNTRUSTED") {
			t.Fatalf("stale native selection fell back to values or prose: %s", out)
		}
	}
}

func TestRuntimeEventDiagramEncodingFailureKeepsAllFactsAsWarnedText(t *testing.T) {
	c, id := eventRenderContract(t, "worker")
	r := eventRenderReceipt(t, c, id, &types.RuntimeEventSelection{FieldKeys: []string{"event.comm", "jank.appid", "jank.start_ts_ns"}})
	// Native projection quotes control bytes. Exercise the final display guard
	// independently for an unexpected unsafe cell without changing that rule.
	r.BoundTable.Rows[0][0] = "worker\x00literal"
	var b strings.Builder
	renderRuntimeEventFactDiagram(&b, r.BoundTable, answerDocLangEN)
	out := b.String()
	if !strings.Contains(out, "```text\n# ⚠ ") || strings.Contains(out, "```mermaid") {
		t.Fatalf("encoding failure did not follow warned text fallback: %s", out)
	}
	for _, row := range r.BoundTable.Rows {
		for _, value := range row {
			if !strings.Contains(out, value) {
				t.Fatalf("encoding failure silently dropped %q", value)
			}
		}
	}
}
