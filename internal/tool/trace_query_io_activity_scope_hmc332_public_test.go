package tool

import (
	"encoding/json"
	"html"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

func activityTablesHMC332(t *testing.T, bus *types.BusContext, result types.ToolResult) []types.RuntimeMeasurementTable {
	t.Helper()
	input := types.ObservationLedgerInputFromBusContext(bus, types.ObservationPromptRecordLimit)
	input.ToolResults = []types.ToolResult{result}
	contract := types.BuildRuntimeMeasurementContract(input)
	var out []types.RuntimeMeasurementTable
	for _, table := range contract.Choices() {
		if strings.Contains(table.ObservationID, "#io_activity:") {
			out = append(out, table)
		}
	}
	if len(out) != 3 {
		t.Fatalf("public native query must retain all three activity views: %d", len(out))
	}
	return out
}

func TestHMC332IOActivityPublicEndpointMatchesNativeWindow(t *testing.T) {
	for _, mode := range []string{"explicit", "line_precedence", "implicit_capture"} {
		t.Run(mode, func(t *testing.T) {
			params := map[string]any{"time_start": 2, "time_end": 2.25, "pid": 40, "bucket_ms": 100}
			switch mode {
			case "line_precedence":
				params["line_start"], params["line_end"] = 2, 2
			case "implicit_capture":
				params = map[string]any{}
			}
			bus, result, stats := ioActivityPublicQuery(t, ioActivityPublicTrace, params)
			tables := activityTablesHMC332(t, bus, result)
			if mode == "explicit" && (stats.Window == nil || stats.Window.EndInclusive || stats.Groups[0].Values.EventCount != 3 || stats.Groups[0].Rates.EventsPerSecond != 12) {
				t.Fatal("fixture lost the exact explicit half-open native population")
			}
			if mode == "line_precedence" && (stats.Window != nil || stats.Groups[0].Values.EventCount != 1 || stats.Groups[0].Rates != nil) {
				t.Fatal("line precedence acquired a time denominator")
			}
			if mode == "implicit_capture" && (stats.Window == nil || !stats.Window.EndInclusive || stats.Groups[0].Values.EventCount != 3) {
				t.Fatal("capture-inclusive population lost its final endpoint")
			}
			for _, table := range tables {
				if table.HasNativeRawFacts() || table.HasNativeResourceFacts() || table.HasNativeSamplingFacts() {
					t.Fatal("display endpoint changed the authority lane")
				}
				basis, ok := table.NativeSourceTitleBasis()
				if mode != "explicit" {
					// Legacy line/capture populations do not have an explicit
					// continuous query ruler. Do not demand or manufacture one.
					if table.HasNativeMeasurementFacts() || (ok && basis.QueryWindow != "") {
						t.Errorf("%s acquired explicit query display authority: %+v", mode, basis)
					}
					continue
				}
				if !table.HasNativeMeasurementFacts() || !ok {
					t.Fatal("real current native source lost its display identity")
				}
				if mode == "explicit" && basis.QueryWindow != "[2,2.25) s" {
					t.Errorf("%s title lost the native half-open endpoint: %q", table.View, basis.QueryWindow)
				}
				if mode != "explicit" && basis.QueryWindow != "" {
					t.Errorf("%s title minted an explicit query window: %q", mode, basis.QueryWindow)
				}
				for _, language := range []string{"zh", "en"} {
					_, sourceNotes, valid := table.NativeSourcePresentation(language)
					if !valid {
						t.Fatal("current source presentation missing")
					}
					if mode == "explicit" && (!strings.Contains(strings.Join(sourceNotes, " "), "[2,2.25) s") || strings.Contains(strings.Join(sourceNotes, " "), "端点包含性未知") || strings.Contains(strings.Join(sourceNotes, " "), "endpoint inclusion unavailable")) {
						t.Errorf("%s/%s source notes contradict the native half-open ruler: %v", table.View, language, sourceNotes)
					}
				}
				if table.View == types.RuntimeMeasurementSummary && mode == "explicit" && !reflect.DeepEqual(table.Rows[0], []string{"all operations", "3", "3", "20480", "6826.66666667", "12", "81920"}) {
					t.Fatalf("endpoint display changed native scalar values: %v", table.Rows)
				}
			}
			if mode == "explicit" {
				view := types.BuildAnswerSemanticViewForBusContext(bus)
				doc := &types.AnswerDocumentV2{DocumentModel: "v2"}
				for _, table := range tables {
					wire, _ := json.Marshal(map[string]any{"id": string(table.View), "kind": "table", "runtime_measurement": map[string]any{"observation_id": table.ObservationID, "view": table.View}})
					var raw emitAnswerBlockV2
					if err := json.Unmarshal(wire, &raw); err != nil {
						t.Fatal(err)
					}
					block, err := NormalizeEmitAnswerBlock(raw, "blocks[0]")
					if err != nil {
						t.Fatal(err)
					}
					doc.Blocks = append(doc.Blocks, block)
				}
				if err := bindRuntimeMeasurementReceipts(doc, view); err != nil {
					t.Fatal(err)
				}
				for _, language := range []string{"zh", "en"} {
					actual := html.UnescapeString(render.RenderAnswerDocument(doc, language))
					if !strings.Contains(actual, "[2,2.25) s") || strings.Contains(actual, "端点包含性未知") || strings.Contains(actual, "endpoint inclusion unavailable") {
						t.Errorf("%s real native answer presentation lost endpoint ownership:\n%s", language, actual)
					}
				}
			}
		})
	}
}

func rewriteActivityPublicationHMC332(t *testing.T, row types.ObservationRecord, change func(*types.ObservationRecord, *types.RuntimeMeasurementPublication)) types.ObservationRecord {
	t.Helper()
	row.RichNotes = append([]string(nil), row.RichNotes...)
	for i, note := range row.RichNotes {
		if raw, ok := strings.CutPrefix(note, types.TraceNoteKeyRuntimeMeasurement+"="); ok {
			var publication types.RuntimeMeasurementPublication
			if err := json.Unmarshal([]byte(raw), &publication); err != nil {
				t.Fatal(err)
			}
			change(&row, &publication)
			publication.Source = row.SourceRef
			encoded, _ := json.Marshal(publication)
			row.RichNotes[i] = types.TraceNoteKeyRuntimeMeasurement + "=" + string(encoded)
			return row
		}
	}
	t.Fatal("public query did not emit its native activity publication")
	return row
}

func TestHMC332IOActivityPublicEndpointCannotBeRecreatedOrReassigned(t *testing.T) {
	bus, result, _ := ioActivityPublicQuery(t, ioActivityPublicTrace, map[string]any{"time_start": 2, "time_end": 2.25})
	index := -1
	for i, row := range result.Observations {
		if row.Predicate == "io_activity" {
			index = i
			break
		}
	}
	if index < 0 {
		t.Fatal("public activity observation missing")
	}
	original := result.Observations[index]
	originalWire, _ := json.Marshal(original)
	for _, mode := range []string{"notes_deleted", "json", "row_changed", "scope_changed", "note_replaced"} {
		t.Run(mode, func(t *testing.T) {
			row := original
			if mode == "json" {
				row = types.ObservationRecord{}
				if err := json.Unmarshal(originalWire, &row); err != nil {
					t.Fatal(err)
				}
			} else {
				row = rewriteActivityPublicationHMC332(t, row, func(r *types.ObservationRecord, p *types.RuntimeMeasurementPublication) {
					switch mode {
					case "notes_deleted":
						for i := range p.Tables {
							p.Tables[i].Notes = nil
						}
					case "row_changed":
						p.Tables[0].Rows[0][1] = "999"
					case "scope_changed":
						r.SourceRef.QueryWindowEndTs = 2.5
					case "note_replaced":
						p.Tables[0].Notes = []string{"Window [2, 2.25) seconds; invented evidence"}
					}
				})
			}
			row = types.FinalizeRuntimeRawMeasurement(row)
			if mode != "json" {
				// An existing witness is immutable: a consumer cannot re-sign a
				// changed payload or flip the native endpoint.
				row = types.PublishRuntimeMeasurementQueryWindow(row, 2, 2.25, true)
			}
			changed := result
			changed.Observations = append([]types.ObservationRecord(nil), result.Observations...)
			changed.Observations[index] = row
			input := types.ObservationLedgerInputFromBusContext(bus, types.ObservationPromptRecordLimit)
			input.ToolResults = []types.ToolResult{changed}
			contract := types.BuildRuntimeMeasurementContract(input)
			retained := 0
			for _, table := range contract.Choices() {
				if table.ObservationID != original.ID {
					continue
				}
				retained++
				basis, _ := table.NativeSourceTitleBasis()
				if mode == "notes_deleted" {
					if basis.QueryWindow != "[2,2.25) s" || len(table.Notes) != 0 {
						t.Fatalf("legal whole-note deletion lost exact endpoint: %q, notes=%v", basis.QueryWindow, table.Notes)
					}
				} else if basis.QueryWindow == "[2,2.25) s" || basis.QueryWindow == "[2,2.25] s" {
					t.Fatal("wire/mutated facts reacquired the native query endpoint")
				}
				if table.HasNativeRawFacts() {
					t.Fatal("endpoint witness minted raw-fact authority")
				}
			}
			if mode == "notes_deleted" && retained != 3 {
				t.Fatalf("legal note deletion dropped native views: %d", retained)
			}
		})
	}
	after, _ := json.Marshal(original)
	if string(originalWire) != string(after) {
		t.Fatal("copy-out or note deletion mutated original native evidence")
	}
}

func TestHMC332IOActivityPublicTeachingAllowsQualifiedSubtotals(t *testing.T) {
	bus, result, _ := ioActivityPublicQuery(t, ioActivityPublicTrace, map[string]any{"time_start": 2, "time_end": 2.25})
	var surfaces []string
	for _, row := range result.Observations {
		if row.Predicate == "io_activity" {
			for _, note := range row.RichNotes {
				if text, ok := strings.CutPrefix(note, types.TraceNoteKeyIOActivityScope+"="); ok {
					surfaces = append(surfaces, text)
				}
			}
		}
	}
	for _, table := range activityTablesHMC332(t, bus, result) {
		surfaces = append(surfaces, strings.Join(table.Notes, " "))
	}
	if len(surfaces) != 4 {
		t.Fatalf("must audit typed observation and all three native views: %d", len(surfaces))
	}
	for i, text := range surfaces {
		for _, want := range []string{"Disjoint device/operation groups", "same source, endpoint family, phase, byte caliber and query scope", "qualified subtotal", "not unique logical requests", "does not prove target waiting, response impact, hardware bandwidth or root cause", "Capture completeness is unknown"} {
			if !strings.Contains(text, want) {
				t.Errorf("surface %d omitted scoped arithmetic or authority boundary %q", i, want)
			}
		}
		for _, forbidden := range []string{"Do not add sources, layers, phases or byte calibers", "source/layer/phase/byte calibers are not additive"} {
			if strings.Contains(text, forbidden) {
				t.Errorf("surface %d retains the blanket arithmetic prohibition", i)
			}
		}
	}
}
