package tool

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Recreate HMC311's eight independent query publications through the real
// query and answer entry points. No old payload or private receipt is imported.
func TestHMC312PublicRepeatedTimelineDisplay(t *testing.T) {
	body, err := os.ReadFile("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	bus, path := businessRefTestContext(t, string(body))
	rm := nativeFactDisplayRequest(types.RequestedAnswerDimensionObservedValue)
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "zh"}}
	bus.Mutable.SetRequestModel(rm)
	variants := []map[string]any{
		{"view": "window_stats"},
		{"view": "wakeup_chain"},
		{"view": "root_cause_rank"},
		{"view": "window_stats", "target_scope": "thread", "platform": "harmony", "trace_flavor": "harmony_hitrace"},
		{"view": "thread_timeline", "target_scope": "thread", "platform": "harmony", "trace_flavor": "harmony_hitrace"},
		{"view": "wakeup_chain", "target_scope": "thread", "platform": "harmony"},
		{"view": "critical_blocking_calls", "target_scope": "thread", "platform": "harmony", "trace_flavor": "harmony_hitrace"},
		{"view": "root_cause_rank", "target_scope": "thread", "platform": "harmony"},
	}
	payloads := map[string][]byte{}
	for _, params := range variants {
		params["path"], params["pid"], params["time_start"], params["time_end"] = path, 100, .999, 1.051
		result := businessRefTestQuery(t, bus, params)
		if !result.Success {
			t.Fatal(result.Summary)
		}
		for _, row := range result.Observations {
			if row.Predicate != TraceThreadTimelinePredicate {
				continue
			}
			raw, err := os.ReadFile(row.SourceRef.PayloadRef)
			if err != nil {
				t.Fatal(err)
			}
			payloads[row.SourceRef.PayloadRef] = raw
			t.Logf("query=%v observation=%s source=%+v", params, row.ID, row.SourceRef)
		}
	}
	input := types.ObservationLedgerInputFromBusContext(bus, 0)
	contract := types.BuildRuntimeMeasurementContract(input)
	var timelines []types.RuntimeMeasurementTable
	for _, table := range contract.Choices() {
		if table.View == types.RuntimeMeasurementTimeline && strings.HasSuffix(table.ObservationID, "#"+TraceThreadTimelinePredicate) {
			timelines = append(timelines, table)
		}
	}
	if len(timelines) != 8 || len(payloads) != 8 {
		t.Fatalf("must keep eight original publications: tables=%d payloads=%d", len(timelines), len(payloads))
	}
	for _, table := range timelines {
		if !reflect.DeepEqual(table.Rows, timelines[0].Rows) || !reflect.DeepEqual(table.Notes, timelines[0].Notes) || len(table.Rows) != 4 {
			t.Fatal("fixture no longer describes the same complete native partition")
		}
	}
	ledgerBefore, _ := json.Marshal(types.CompileObservationLedger(input))
	for _, mode := range []string{"automatic", "explicit_selector", "planned_aliases"} {
		t.Run(mode, func(t *testing.T) {
			if mode == "planned_aliases" {
				selectors := []types.AnswerRuntimeMeasurementReceipt{{ObservationID: timelines[2].ObservationID, View: types.RuntimeMeasurementTimeline}, {ObservationID: timelines[7].ObservationID, View: types.RuntimeMeasurementTimeline}}
				handoff := &types.InvestigationNativeFactHandoff{RuntimeMeasurements: selectors, AnswerDisplay: selectors}
				if !types.SealInvestigationNativeAnswerDisplay(handoff, &rm, contract) {
					t.Fatal("current independent layout selectors not accepted")
				}
				bus.Mutable.SetInvestigationComplete("native query receipt display")
				bus.Mutable.AcceptInvestigationNativeFactHandoff(handoff)
				if selected, unavailable := types.ResolveInvestigationNativeAnswerDisplay(bus.Mutable.AcceptedInvestigationNativeFactHandoff(), &rm, contract); len(selected) != 2 || unavailable != 0 {
					t.Fatalf("layout selectors not actually retained: %d/%d", len(selected), unavailable)
				}
			}
			blocks := []any{map[string]any{"id": "lead", "kind": "summary", "text": "Independent interpretations and query receipts remain unchanged."}}
			if mode == "explicit_selector" {
				blocks = append(blocks, map[string]any{"id": "selected", "kind": "table", "runtime_measurement": map[string]any{"observation_id": timelines[7].ObservationID, "view": "timeline"}})
			}
			result := b1659bExecuteAnswer(t, bus, map[string]any{"blocks": blocks}, false)
			if !result.Success {
				t.Fatal(result.Summary)
			}
			doc := bus.Mutable.AnswerDocumentV2()
			count := 0
			for _, block := range doc.Blocks {
				if block.RuntimeMeasurement == nil || !strings.HasSuffix(block.RuntimeMeasurement.ObservationID, "#"+TraceThreadTimelinePredicate) {
					continue
				}
				count++
				if !block.RuntimeMeasurement.IsBound() || !reflect.DeepEqual(block.RuntimeMeasurement.BoundTable.Rows, timelines[0].Rows) {
					t.Fatal("selected complete table changed")
				}
			}
			visible := render.RenderAnswerDocument(doc, "zh")
			t.Logf("accepted render (%d timeline tables):\n%s", count, visible)
			if count != 1 {
				t.Fatalf("same source/generation/window/partition displayed %d times, want once without deleting any query", count)
			}
		})
	}
	ledgerAfter, _ := json.Marshal(types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, 0)))
	if !bytes.Equal(ledgerBefore, ledgerAfter) || len(types.BuildRuntimeMeasurementContract(types.ObservationLedgerInputFromBusContext(bus, 0)).Choices()) != len(contract.Choices()) {
		t.Fatal("display reuse changed original observations or selectors")
	}
	for path, original := range payloads {
		current, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(original, current) {
			t.Fatalf("independent payload changed: %s %v", path, err)
		}
	}
}

func TestHMC312PublicFrequencyDisplayAndSourceBoundaries(t *testing.T) {
	fixture, err := os.ReadFile("../../eval/fixtures/hmosperf_dual_source_windows/baseline.systrace")
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"exact", "other_source", "other_window", "other_target", "generation", "memo", "json", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			bus, path := businessRefTestContext(t, string(fixture))
			rm := nativeFactDisplayRequest(types.RequestedAnswerDimensionObservedValue)
			bus.AnalysisIR = &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "zh"}}
			bus.Mutable.SetRequestModel(rm)
			first := businessRefTestQuery(t, bus, map[string]any{"path": path, "view": "window_stats", "pid": 41, "time_start": 1, "time_end": 1.01})
			secondPath, end, pid := path, 1.01, 41
			if mode == "other_source" {
				secondPath = filepath.Join(bus.RepoRoot, "independent-capture.systrace")
				if err := os.WriteFile(secondPath, fixture, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "other_window" {
				end = 1.009
			}
			if mode == "other_target" {
				pid = 7
			}
			if mode == "generation" {
				replacement := filepath.Join(bus.RepoRoot, "replacement.systrace")
				if err := os.WriteFile(replacement, fixture, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(replacement, path); err != nil {
					t.Fatal(err)
				}
			}
			second := businessRefTestQuery(t, bus, map[string]any{"path": secondPath, "view": "wakeup_chain", "pid": pid, "time_start": 1, "time_end": end, "include_window_stats": true})
			if !first.Success || !second.Success {
				t.Fatalf("query failed: %s / %s", first.Summary, second.Summary)
			}
			if mode == "memo" {
				second.ReusedFromRunMemo = true
			}
			if mode == "json" {
				data, _ := json.Marshal(second)
				var restored types.ToolResult
				if err := json.Unmarshal(data, &restored); err != nil {
					t.Fatal(err)
				}
				second = restored
			}
			if mode == "canceled" {
				second.TraceViewCancellation = &types.TraceViewCancellation{}
			}
			// Use exact retained results, without the original dispatch copies
			// masking a missing or stale witness in the negative controls.
			input := types.ObservationLedgerInputFromBusContext(bus, 0)
			input.ToolResults = []types.ToolResult{first, second}
			contract := types.BuildRuntimeMeasurementContract(input)
			var receipts []types.AnswerRuntimeMeasurementReceipt
			for _, table := range contract.Choices() {
				if strings.HasSuffix(table.ObservationID, "#"+TraceCPUFrequencyTimelinePredicate) {
					if table.MemberSet != nil || len(table.Rows) != 2 || len(table.Rows[0]) != 7 {
						t.Fatal("frequency detail or authority changed")
					}
					receipts = append(receipts, types.AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View})
				}
			}
			if len(receipts) != 2 {
				t.Fatalf("independent selectors lost: %d", len(receipts))
			}
			same := types.RuntimeMeasurementDisplayKey(receipts[0], contract) == types.RuntimeMeasurementDisplayKey(receipts[1], contract)
			if same != (mode == "exact") {
				t.Fatalf("source/query boundary %s improperly merged=%t", mode, same)
			}
			if mode != "exact" {
				return
			}
			if out := b1659bExecuteAnswer(t, bus, map[string]any{"blocks": []any{map[string]any{"id": "lead", "kind": "summary", "text": "Frequency intervals remain complete native observations."}}}, false); !out.Success {
				t.Fatal(out.Summary)
			}
			count := 0
			for _, block := range bus.Mutable.AnswerDocumentV2().Blocks {
				if block.RuntimeMeasurement != nil && strings.HasSuffix(block.RuntimeMeasurement.ObservationID, "#"+TraceCPUFrequencyTimelinePredicate) {
					count++
					if len(block.RuntimeMeasurement.BoundTable.Rows) != 2 {
						t.Fatal("full frequency intervals replaced by preview")
					}
				}
			}
			if count != 1 {
				t.Fatalf("frequency full projection displayed %d times", count)
			}
		})
	}
}
