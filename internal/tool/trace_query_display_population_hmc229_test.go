package tool

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/tracewire"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC229NativeDisplayProducerPopulation(t *testing.T) {
	for _, view := range []string{"measurements", "process_measurements"} {
		t.Run(view, func(t *testing.T) {
			dir := "hmosperf_measurements"
			wantRows := 13
			if view == "process_measurements" {
				dir, wantRows = "hmosperf_process_measurements", 11
			}
			path, _ := filepath.Abs("../../eval/fixtures/" + dir + "/capture.data")
			bus, _, _ := hmc17NamedPathContext(t)
			query := func(limit int) types.ToolResult {
				args := map[string]any{"source": "path", "path": path, "view": view, "time_start": 1, "time_end": 2}
				if limit > 0 {
					args["limit"] = limit
				}
				out := hmc17NamedQuery(t, bus, args)
				if !out.Success {
					t.Fatal(out.Summary)
				}
				return out
			}
			full := query(0)
			rawPayload := hmc17NamedPayload(t, full)
			var origin types.ObservationRecord
			for _, r := range full.Observations {
				if r.Predicate == view+"_observation" {
					origin = r
				}
			}
			if origin.ID == "" {
				t.Fatal("producer missing")
			}
			for _, mode := range []string{"complete", "engine_limit", "byte_budget", "empty", "duplicate_row_identity"} {
				t.Run(mode, func(t *testing.T) {
					r := origin
					if mode == "engine_limit" {
						for _, candidate := range query(2).Observations {
							if candidate.Predicate == origin.Predicate {
								r = candidate
							}
						}
					} else if mode != "complete" {
						// Keep the real source/window and exact query data. The
						// mutations exercise only producer retention boundaries.
						raw, _ := json.Marshal(rawPayload)
						var p tracequery.Result
						if err := json.Unmarshal(raw, &p); err != nil {
							t.Fatal(err)
						}
						var note string
						if view == "measurements" {
							m := p.Measurements
							switch mode {
							case "byte_budget":
								for i := range m.Rows {
									m.Rows[i].Record.Value = tracewire.MeasureScalar{StorageClass: "text", Value: strings.Repeat("precise-value", 700)}
								}
							case "empty":
								m.Rows, m.TotalRows, m.OmittedRows = nil, 0, 0
							case "duplicate_row_identity":
								m.Rows[1].Record.RowID = m.Rows[0].Record.RowID
							}
							note = traceQueryMeasurementsReceipt(r, *m)
						} else {
							m := p.ProcessMeasurements
							switch mode {
							case "byte_budget":
								for i := range m.Rows {
									m.Rows[i].Record.NameKnown, m.Rows[i].Record.Name = true, strings.Repeat("precise-metric", 700)
								}
							case "empty":
								m.Rows, m.TotalRows, m.OmittedRows = nil, 0, 0
							case "duplicate_row_identity":
								m.Rows[1].Record.RowID = m.Rows[0].Record.RowID
							}
							note = traceQueryProcessMeasurementsReceipt(r, *m)
						}
						if mode == "duplicate_row_identity" {
							if note != "" {
								t.Fatal("ambiguous identities were published")
							}
							return
						}
						if note == "" {
							t.Fatal("display table was lost")
						}
						if len(note) > (64<<10)+len(types.TraceNoteKeyRuntimeMeasurement)+1 {
							t.Fatal("publication exceeded existing budget")
						}
						r.RichNotes = []string{note}
					}
					pub, ok := types.DecodeRuntimeMeasurementPublication(r)
					if !ok {
						t.Fatal("valid display publication rejected")
					}
					for _, table := range pub.Tables {
						want := mode == "complete" && table.View == types.RuntimeMeasurementMembers
						if table.HasCompleteNativeDisplayPopulation() != want || table.MemberSet != nil {
							t.Fatalf("mode=%s view=%s display=%v member=%v", mode, table.View, table.HasCompleteNativeDisplayPopulation(), table.MemberSet)
						}
						if table.View != types.RuntimeMeasurementMembers {
							continue
						}
						if mode == "complete" && len(table.Rows) != wantRows {
							t.Fatalf("native rows=%d want=%d", len(table.Rows), wantRows)
						}
						if mode == "byte_budget" && (len(table.Rows) == 0 || len(table.Rows) >= wantRows) {
							t.Fatal("budget must retain whole rows and withhold completeness")
						}
						if mode == "byte_budget" {
							cell := 7
							needle := strings.Repeat("precise-value", 700)
							if view == "process_measurements" {
								cell, needle = 2, strings.Repeat("precise-metric", 700)
							}
							if !strings.Contains(table.Rows[0][cell], needle) {
								t.Fatal("budget truncated a field rather than a whole row")
							}
						}
					}
				})
			}
			// Receipt creation is display-only and does not mutate the native
			// result, including the separately audited unknown-time population.
			again := hmc17NamedPayload(t, full)
			if !reflect.DeepEqual(rawPayload, again) {
				t.Fatal("native query values were modified")
			}
		})
	}
}

func TestHMC229UnpositionedTableCannotAcquireTimedDisplayPopulation(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_dual_measurements/current.data")
	result, _ := unpositionedMeasurementQuery(t, path, "event_search")
	r := unpositionedMeasurementRecord(t, result)
	pub, ok := types.DecodeRuntimeMeasurementPublication(r)
	if !ok || len(pub.Tables) != 1 {
		t.Fatal("unknown-time native table absent")
	}
	table := &pub.Tables[0]
	table.DisplayPopulation = &types.RuntimeMeasurementDisplayPopulation{QueryScopeID: r.SourceRef.QueryScopeID, RowIDs: []string{"unknown-9"}, MatchedRows: 1}
	raw, _ := json.Marshal(pub)
	r.RichNotes = []string{types.TraceNoteKeyRuntimeMeasurement + "=" + string(raw)}
	decoded, ok := types.DecodeRuntimeMeasurementPublication(r)
	if !ok || decoded.Tables[0].HasCompleteNativeDisplayPopulation() || decoded.Tables[0].Rows[0][7] != "55 (integer)" {
		t.Fatal("scan coordinates became record-time population or raw value was lost")
	}
}
