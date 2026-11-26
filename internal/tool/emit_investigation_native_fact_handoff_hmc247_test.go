package tool

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/loginput"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestPreparedFrameNativeFactHandoffBindsCitedRows(t *testing.T) {
	ctx := nativeMeasurementHandoffContext(t)
	var err error
	ctx.AttachedTraceMaterial, err = traceinput.Prepare(context.Background(), traceinput.Options{
		InputPath:     "../../eval/fixtures/hmosperf_io_request_latency_distribution/events.systrace",
		RuntimeAnchor: t.TempDir(), PreviewBytes: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	const frame = `{"record_kind":"frame","side":"target","metric_kind":"jank","count":1,"thread_id":42,"start_ns":1000000000,"end_ns":14000000000,"observation_id":"o","source_id":"s","frame_metrics":{"duration_ns":13000000000}}`
	ctx.AttachedLogCatalog, err = loginput.Prepare(context.Background(), []loginput.Input{{Name: "saved fact", Data: []byte(frame)}}, loginput.Options{})
	if err != nil || !UsesCompactNativeFactHandoff(ctx) {
		t.Fatalf("missing prepared frame capability: %v", err)
	}
	choices := NativeFactHandoffChoices(ctx)
	if len(choices) < 2 {
		t.Fatalf("want two native measurements, got %+v", choices)
	}
	for _, tc := range []struct {
		label  string
		repair bool
	}{
		{"event_search:10976.243331..10976.550198", true},
		{NativeFactMeasurementRef(choices[0]), false},
	} {
		raw, _ := json.Marshal(map[string]any{
			"reason": "complete", "confidence": "high", "result_kind": "resolved",
			"native_fact_handoff": map[string]any{
				"measurement_refs": []string{NativeFactMeasurementRef(choices[0])},
				"interpretation":   "Keep the measured scope.",
				"support_refs":     []string{tc.label},
			},
		})
		p, _, err := decodeEmitInvestigationCompleteParamsStrict(ctx, "emit_investigation_complete", raw, (&EmitInvestigationComplete{}).Parameters())
		if (err == nil) != tc.repair {
			t.Fatalf("support ref %q decode result: %v", tc.label, err)
		}
		if tc.repair && (len(p.NativeFactHandoff.MeasurementRefs) != 1 || p.NativeFactHandoff.MeasurementRefs[0] != NativeFactMeasurementRef(choices[0])) {
			t.Fatalf("repair changed the selected measurement: %+v", p.NativeFactHandoff)
		}
	}
	_, err = completionNativeFactHandoffSelection(ctx, &completionNativeFactHandoff{
		Interpretation: json.RawMessage(`"Keep the measured scope."`),
		AnswerDisplay:  choices[:1],
	})
	if err == nil || !strings.Contains(err.Error(), "native_fact_handoff.measurement_refs: select_current_refs_or_native_rows") ||
		!strings.Contains(err.Error(), NativeFactMeasurementRef(choices[0])) {
		t.Fatalf("compact handoff should diagnose missing selection using current reference strings: %v", err)
	}
	raw := json.RawMessage(`{"reason":"complete","confidence":"high","result_kind":"resolved","aggregate_facts":[{"kind":"scalar_value","label":"frame duration","value":"306.867","unit":"ms","load_bearing_summary":true}]}`)
	decoded, failure, err := decodeEmitInvestigationCompleteParamsStrict(ctx, "emit_investigation_complete", raw, (&EmitInvestigationComplete{}).Parameters())
	if err != nil || failure != nil || len(decoded.AggregateFacts) != 1 || decoded.AggregateFacts[0].Value != "306.867" {
		t.Fatalf("foreign boolean annotation must preserve the source fact: %+v %v %+v", decoded.AggregateFacts, err, failure)
	}
	for _, tt := range []struct {
		name  string
		whole bool
		row   int
		valid bool
	}{
		{"row only", false, 0, true},
		{"row plus separately selected whole table", true, 0, true},
		{"fabricated row address", true, 100000, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			choice := choices[0]
			params := map[string]any{
				"reason": "complete", "confidence": "high", "result_kind": "resolved",
				"native_fact_handoff": map[string]any{
					"interpretation_units": []map[string]any{{
						"text": "Keep the measured scope.",
						"native_refs": []types.InvestigationNativeRowReference{{
							ObservationID: choice.ObservationID, View: choice.View, RowIndices: []int{tt.row},
						}},
					}},
				},
			}
			if tt.whole {
				params["measurement_refs"] = []string{NativeFactMeasurementRef(choices[1])}
			}
			raw, _ := json.Marshal(params)
			decoded, failure, err := decodeEmitInvestigationCompleteParamsStrict(ctx, "emit_investigation_complete", raw, (&EmitInvestigationComplete{}).Parameters())
			if err != nil || failure != nil {
				t.Fatalf("decode: %v %+v", err, failure)
			}
			handoff, err := completionNativeFactHandoffSelection(ctx, decoded.NativeFactHandoff)
			if (err == nil) != tt.valid {
				t.Fatalf("handoff valid=%v, want %v: %v", err == nil, tt.valid, err)
			}
			if tt.valid {
				want := 1
				if tt.whole {
					want++
				}
				if len(handoff.RuntimeMeasurements) != want || handoff.RuntimeMeasurements[want-1].ObservationID != choice.ObservationID {
					t.Fatalf("wrong assembled selection: %+v", handoff.RuntimeMeasurements)
				}
			}
		})
	}
}

func TestNativeFactHandoffKeepsRawWakeupObservationAsLocator(t *testing.T) {
	ctx, _ := hmc228CausalDiagramContext(t, &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis})
	var edge string
	for _, row := range ctx.Mutable.DispatchToolResults()[0].Observations {
		if row.Predicate == "wakeup_chain_edge" {
			edge = row.ID
			break
		}
	}
	if edge == "" {
		t.Fatal("fixture did not publish a wakeup edge")
	}
	wakeup := types.InvestigationNativeRowReference{ObservationID: edge, View: types.RuntimeMeasurementSummary, RowIndices: []int{0}}
	unpublished := types.InvestigationNativeRowReference{ObservationID: edge + "-missing", View: types.RuntimeMeasurementSummary, RowIndices: []int{0}}
	wrongRow := types.InvestigationNativeRowReference{ObservationID: edge, View: types.RuntimeMeasurementSummary, RowIndices: []int{1}}
	units := []types.InvestigationInterpretationUnit{{
		Text:       "The trace identifies a wakeup edge. The cause of the wait is unproven.",
		NativeRefs: []types.InvestigationNativeRowReference{wakeup, unpublished, wrongRow},
	}}
	normalizeTraceObservationRefs(ctx, units, nil)
	if !reflect.DeepEqual(units[0].SupportRefs, []string{edge}) ||
		!reflect.DeepEqual(units[0].NativeRefs, []types.InvestigationNativeRowReference{unpublished, wrongRow}) {
		t.Fatalf("raw source normalization changed unresolved addresses or lost the published edge: %+v", units[0])
	}
}

func TestNativeFactHandoffMeasurementRefsMatchLegacyReceipts(t *testing.T) {
	ctx := nativeMeasurementHandoffContext(t)
	choice := NativeFactHandoffChoices(ctx)[0]
	submit := func(handoff map[string]any) types.ToolResult {
		t.Helper()
		handoff["interpretation"] = "Independent uncertainty is retained."
		raw, err := json.Marshal(map[string]any{"reason": "lifecycle", "confidence": "high", "result_kind": "resolved", "native_fact_handoff": handoff})
		if err != nil {
			t.Fatal(err)
		}
		result, err := (&EmitInvestigationComplete{}).Execute(ctx, raw)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	if result := submit(map[string]any{"runtime_measurements": []types.AnswerRuntimeMeasurementReceipt{choice}}); !result.Success {
		t.Fatal(result.Summary)
	}
	legacy := ctx.Mutable.AcceptedInvestigationNativeFactHandoff()
	ref := NativeFactMeasurementRef(choice)
	if result := submit(map[string]any{"measurement_refs": []string{ref}}); !result.Success {
		t.Fatal(result.Summary)
	}
	if got := ctx.Mutable.AcceptedInvestigationNativeFactHandoff(); !reflect.DeepEqual(got, legacy) {
		t.Fatalf("reference form changed the accepted handoff: %#v != %#v", got, legacy)
	}
	for _, invalid := range [][]string{{}, {ref, ref}, {"unknown|summary"}} {
		if result := submit(map[string]any{"measurement_refs": invalid}); result.Success ||
			!reflect.DeepEqual(ctx.Mutable.AcceptedInvestigationNativeFactHandoff(), legacy) {
			t.Fatalf("invalid selection %v altered the accepted handoff: %s", invalid, result.Summary)
		}
	}
}

func TestHMC247NativeFactHandoffPublicValidation(t *testing.T) {
	for _, mode := range []string{"valid", "empty_interpretation", "missing_interpretation", "unknown_id", "wrong_view", "duplicate", "event_selection", "copied_values", "stale_source", "no_contract"} {
		t.Run(mode, func(t *testing.T) {
			ctx := nativeMeasurementHandoffContext(t)
			c := types.BuildAnswerSemanticViewForBusContext(ctx).RuntimeMeasurementContract
			if len(c.Choices()) == 0 {
				t.Fatal("no public native table")
			}
			table := c.Choices()[0]
			r := map[string]any{"observation_id": table.ObservationID, "view": table.View}
			h := map[string]any{"runtime_measurements": []any{r}, "interpretation": "Independent hypothesis remains unproven."}
			switch mode {
			case "empty_interpretation":
				h["interpretation"] = ""
			case "missing_interpretation":
				delete(h, "interpretation")
			case "unknown_id":
				r["observation_id"] = "unpublished"
			case "wrong_view":
				r["view"] = "unknown"
			case "duplicate":
				h["runtime_measurements"] = []any{r, r}
			case "event_selection":
				r["event_selection"] = map[string]any{}
			case "copied_values":
				r["value"] = 999
			case "stale_source", "no_contract":
				results := ctx.Mutable.DispatchToolResults()
				ctx.Mutable = types.NewMutableState("new consumer")
				if mode == "stale_source" {
					wire, _ := json.Marshal(results)
					json.Unmarshal(wire, &results)
					for i := range results {
						for j := range results[i].Observations {
							results[i].Observations[j].SourceRef.Path += "-different"
						}
					}
					ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
				}
			}
			ctx.Mutable.SetInvestigationComplete("prior")
			raw, _ := json.Marshal(map[string]any{"reason": "lifecycle", "confidence": "high", "result_kind": "resolved", "native_fact_handoff": h})
			result, err := (&EmitInvestigationComplete{}).Execute(ctx, raw)
			valid := mode == "valid" || mode == "empty_interpretation"
			if valid {
				if err != nil || !result.Success || ctx.Mutable.AcceptedInvestigationNativeFactHandoff() == nil {
					t.Fatalf("valid native handoff rejected: %v %s", err, result.Summary)
				}
			} else if result.Success || ctx.Mutable.StableInvestigationCompleteReason() != "prior" || ctx.Mutable.AcceptedInvestigationNativeFactHandoff() != nil {
				t.Fatalf("invalid handoff altered accepted state: %v %s", err, result.Summary)
			}
		})
	}
}

func TestHMC247NativeFactHandoffSchemaBoundary(t *testing.T) {
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal((&EmitInvestigationComplete{}).Parameters(), &schema); err != nil {
		t.Fatal(err)
	}
	h := string(schema.Properties["native_fact_handoff"])
	for _, word := range []string{"observation_id", "interpretation", "current sources", "disclosed separately", "does not waive", "negative finding", "source explanation"} {
		if !strings.Contains(h, word) {
			t.Errorf("missing schema contract %q", word)
		}
	}
	if strings.Contains((&EmitInvestigationComplete{}).Description(), "native_fact_handoff") || !strings.Contains(h, "reason is lifecycle rationale only") {
		t.Fatal("conditional handoff teaching must stay with its available schema field")
	}
	if strings.Contains(h, "otherwise the legacy reason remains") || !strings.Contains(h, "interpretation alone owns the current analysis narrative") {
		t.Fatal("presentation completeness contradicted current narrative ownership")
	}
}
