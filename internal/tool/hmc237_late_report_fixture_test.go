package tool

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

type lateReportOracle237 struct {
	Rows []struct {
		ReportTSNS int64 `json:"report_ts_ns"`
		AppID      int64 `json:"appid"`
		Frames     int64 `json:"jank_frames"`
		StartTSNS  int64 `json:"start_ts_ns"`
		EndTSNS    int64 `json:"end_ts_ns"`
		DurationMS int64 `json:"duration_ms"`
	} `json:"rows"`
	ApplicationCounts []struct {
		AppID int64 `json:"appid"`
		Count int64 `json:"count"`
	} `json:"application_counts"`
	PayloadStartFilteredCount int `json:"unrequested_payload_start_filter_count"`
}

func TestHMC237LateReportBinaryNativeFactReuse(t *testing.T) {
	dir, err := filepath.Abs("../../eval/fixtures/hmosperf_gzip_sqlite_late_reports")
	if err != nil {
		t.Fatal(err)
	}
	oracleBytes, err := os.ReadFile(filepath.Join(dir, "oracle.json"))
	if err != nil {
		t.Fatal(err)
	}
	var oracle lateReportOracle237
	if err := json.Unmarshal(oracleBytes, &oracle); err != nil {
		t.Fatal(err)
	}
	if len(oracle.Rows) != 5 || len(oracle.ApplicationCounts) != 2 || oracle.PayloadStartFilteredCount != 3 {
		t.Fatal("independent HMC237 oracle changed")
	}
	original, err := os.ReadFile(filepath.Join(dir, "capture.transport"))
	if err != nil {
		t.Fatal(err)
	}
	database, err := os.ReadFile(filepath.Join(dir, "capture.data"))
	if err != nil {
		t.Fatal(err)
	}
	z, err := gzip.NewReader(bytes.NewReader(original))
	if err != nil {
		t.Fatal(err)
	}
	unpacked, readErr := io.ReadAll(z)
	if closeErr := z.Close(); readErr != nil || closeErr != nil || !bytes.Equal(unpacked, database) {
		t.Fatalf("compressed input differs from independent closed DB: read=%v close=%v", readErr, closeErr)
	}
	// Exercise the real named-source preparer, not a converted text fixture.
	path := filepath.Join(t.TempDir(), "late-reports.capture")
	if err := os.WriteFile(path, original, 0o400); err != nil {
		t.Fatal(err)
	}
	bus, preparer, preparations := hmc17NamedPathContext(t)
	bus.RuntimeArtifactPreflight = types.NormalizeRuntimeArtifactPreflightProfile(types.RuntimeArtifactPreflightProfile{Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: path, Carrier: "request_path"}}})
	params := map[string]any{"source": "path", "path": path, "view": "event_search", "pattern": "jank_event_sync", "time_start": 1, "time_end": 1.5, "limit": 100,
		"event_field_filters": []map[string]string{{"field": "jank_frames", "op": "gte", "value": "3"}}}
	result := hmc17NamedQuery(t, bus, params)
	payload := hmc17NamedPayload(t, result)
	if len(payload.Events) != len(oracle.Rows)+1 || payload.TimeStart != 1 || payload.TimeEnd != 1.5 {
		t.Fatalf("inclusive native report-window enumeration changed: got %d events", len(payload.Events))
	}
	var record types.ObservationRecord
	for _, observation := range result.Observations {
		if observation.Predicate == types.TraceEventSearchInventoryPredicate {
			record = observation
			break
		}
	}
	inventory := record.EventSearchInventory
	if inventory == nil || len(inventory.Rows) != 6 || !inventory.RowsComplete || !inventory.Coverage.EnumerationComplete || inventory.Coverage.MatchedTotal != 6 {
		t.Fatalf("native inventory lost full original members: %+v", inventory)
	}
	for n, want := range oracle.Rows {
		row := inventory.Rows[n]
		if row.JankEvent == nil || row.JankEvent.Values == nil {
			t.Fatalf("row %d lost native payload", n)
		}
		v := row.JankEvent.Values
		if math.Round(row.TraceTimeSeconds*1e9) != float64(want.ReportTSNS) ||
			v.AppID != want.AppID || v.JankFrames != want.Frames || v.StartTSNS != want.StartTSNS || v.EndTSNS != want.EndTSNS || v.ReportedDurationNS != want.DurationMS*1e6 {
			t.Fatalf("row %d report/payload tuple mismatch: got=%+v/%+v want=%+v", n, row, v, want)
		}
	}
	if _, ok := types.DecodeRuntimeMeasurementPublication(record); !ok {
		t.Fatal("actual tool publication failed exact source/query/member validation")
	}
	// The lookup is closed; the user's requested display window is half-open.
	// Build the actual native contract so default field and group selections
	// consume that same request projection, not a manually picked oracle list.
	start, end := 1.0, 1.5
	rm := &types.RequestModel{RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "[1.0, 1.5)"}}
	displayBus := bus.ShallowClone()
	displayBus.AnalysisIR = &types.AnalysisIR{RequestModel: *rm}
	displayBus.ToolResults = []types.ToolResult{result}
	originalRecord, _ := json.Marshal(record)
	contract := types.BuildRuntimeMeasurementContract(types.ObservationLedgerInputFromBusContext(displayBus, 0))
	metadataOracle := hmc287CurrentMetadataOracle(t, displayBus, path, "")
	if contract == nil {
		t.Fatal("native request-scoped contract missing")
	}
	measured, metadata := 0, 0
	for _, table := range contract.Choices() {
		if metadataOracle.matches(t, table) {
			metadata++
			continue
		}
		measured++
		if table.ObservationID != record.ID || table.View != types.RuntimeMeasurementMembers || table.EventData == nil {
			t.Fatalf("native request-scoped event identity changed: %+v", table)
		}
	}
	if measured != 1 || metadata != 1 || len(contract.Tables) != measured+metadata {
		t.Fatalf("native event/metadata roster changed: event=%d metadata=%d", measured, metadata)
	}
	var wantedFields, wantedGroups [][]string
	for _, row := range oracle.Rows {
		wantedFields = append(wantedFields, []string{strconv.FormatFloat(float64(row.ReportTSNS)/1e9, 'f', -1, 64), fmt.Sprint(row.AppID), fmt.Sprint(row.Frames), fmt.Sprint(row.StartTSNS), fmt.Sprint(row.EndTSNS), fmt.Sprint(row.DurationMS)})
	}
	for _, group := range oracle.ApplicationCounts {
		wantedGroups = append(wantedGroups, []string{fmt.Sprint(group.AppID), fmt.Sprint(group.Count)})
	}
	for _, selection := range []struct {
		name string
		pick *types.RuntimeEventSelection
		want [][]string
	}{
		{"fields", &types.RuntimeEventSelection{FieldKeys: []string{"event.trace_time_seconds", "jank.appid", "jank.jank_frames", "jank.start_ts_ns", "jank.end_ts_ns", "jank.reported_duration_ms"}}, wantedFields},
		{"groups", &types.RuntimeEventSelection{GroupBy: []string{"jank.appid"}}, wantedGroups},
		{"late-members", &types.RuntimeEventSelection{RowIDs: []string{"event-2", "event-1"}, FieldKeys: []string{"event.trace_time_seconds", "jank.appid", "jank.jank_frames", "jank.start_ts_ns", "jank.end_ts_ns", "jank.reported_duration_ms"}}, wantedFields[:2]},
	} {
		t.Run(selection.name, func(t *testing.T) {
			receipt := &types.AnswerRuntimeMeasurementReceipt{ObservationID: record.ID, View: types.RuntimeMeasurementMembers, EventSelection: selection.pick}
			if !types.BindRuntimeMeasurementReceipt(receipt, contract) || !receipt.IsBound() || !reflect.DeepEqual(receipt.BoundTable.Rows, selection.want) {
				t.Fatalf("same-query native selection changed report/payload or grouping: %+v", receipt.BoundTable)
			}
			assertLateReportSurfaces237(t, receipt)
		})
	}
	afterRecord, _ := json.Marshal(record)
	if !bytes.Equal(originalRecord, afterRecord) || len(record.EventSearchInventory.Rows) != 6 || record.EventSearchInventory.Coverage.MatchedTotal != 6 {
		t.Fatal("request-scoped display mutated original source/query/coverage/members")
	}
	// This bus deliberately has no bounded-fact RequestModel: demonstrate that
	// the two time-coordinate filters really differ at the query layer without
	// bypassing a product provenance gate or pretending the extra filter is valid.
	params["event_field_filters"] = []map[string]string{{"field": "jank_frames", "op": "gte", "value": "3"}, {"field": "start_ts", "op": "gte", "value": "1000000000"}}
	filtered := hmc17NamedPayload(t, hmc17NamedQuery(t, bus, params))
	retained := 0
	for _, row := range filtered.Events {
		if row.Ts >= start && row.Ts < end {
			retained++
		}
	}
	if len(filtered.Events) != oracle.PayloadStartFilteredCount+1 || retained != oracle.PayloadStartFilteredCount {
		t.Fatalf("extra payload filter must expose closed-query 6 -> 4 / half-open request 5 -> 3 loss: native=%d request=%d", len(filtered.Events), retained)
	}
	if material, err := preparer.Prepare(t.Context(), path); err != nil || material == nil || preparations.Load() != 1 {
		t.Fatalf("real binary preparation was not shared: material=%v err=%v conversions=%d", material, err, preparations.Load())
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(original, after) {
		t.Fatal("query mutated original compressed input")
	}
}

func assertLateReportSurfaces237(t *testing.T, receipt *types.AnswerRuntimeMeasurementReceipt) {
	t.Helper()
	before := receipt.Clone()
	for _, kind := range []types.AnswerBlockKind{types.BlockSummary, types.BlockTable, types.BlockDiagram} {
		doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "facts", Kind: kind, RuntimeMeasurement: receipt}}}
		out := render.RenderAnswerDocument(doc, "zh")
		for _, row := range receipt.BoundTable.Rows {
			found := false
			for _, line := range strings.Split(out, "\n") {
				allCells := true
				for _, value := range row {
					allCells = allCells && strings.Contains(line, value)
				}
				found = found || allCells
			}
			if !found {
				t.Fatalf("%s split or changed an app/report/payload tuple %v: %s", kind, row, out)
			}
		}
		for _, note := range receipt.BoundTable.Notes {
			if !strings.Contains(html.UnescapeString(out), note) {
				t.Fatalf("%s lost source/window/completeness note %q", kind, note)
			}
		}
		if kind == types.BlockDiagram {
			parts := strings.Split(out, "```mermaid\n")
			if len(parts) != 2 {
				t.Fatalf("native record diagram missing: %s", out)
			}
			body := strings.Split(parts[1], "\n```")[0]
			if len(mermaidcompat.ParseEdges(body)) != 0 || strings.Contains(body, receipt.ObservationID) || strings.Contains(body, "event-1") || strings.Count(body, "[\"") != len(receipt.BoundTable.Rows) {
				t.Fatalf("record overview introduced relations, long IDs or lost nodes: %s", body)
			}
			if rendered := render.RenderMermaidBlocks("```mermaid\n" + body + "\n```"); strings.Contains(rendered, "# ⚠") || strings.Contains(rendered, "# ·") {
				t.Fatalf("native fact diagram failed actual render: %s", rendered)
			}
		}
	}
	if !reflect.DeepEqual(before, receipt) {
		t.Fatal("rendering mutated same-source native values")
	}
}
