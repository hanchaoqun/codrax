package types

import (
	"encoding/json"
	"os"
	"testing"
)

func TestHMC277PairFixedFactsRequireIndependentCurrentSource(t *testing.T) {
	for _, mode := range []string{"current", "system_supplement", "memo_only", "cancelled", "failed", "foreign_run", "reset_run", "stale_source", "json_replay", "no_replay", "unknown_window", "containing_window", "outside_window", "ambiguous_producer"} {
		t.Run(mode, func(t *testing.T) {
			input := hmc277CurrentMeasurementInput(t)
			input.RequestModel.RuntimeArtifactScopeProfile.SourceQuote = "Display the requested time interval."
			owner := input.runtimeMeasurementPairConsumer
			result := input.ToolResults[0]
			switch mode {
			case "memo_only":
				result.ReusedFromRunMemo = true
			case "cancelled":
				result.TraceViewCancellation = &TraceViewCancellation{}
			case "failed":
				result.Success = false
			case "no_replay":
				result.TraceQueryWindowReplay = TraceQueryWindowReplayRef{}
			case "unknown_window":
				r := &result.Observations[0]
				p, _ := DecodeRuntimeMeasurementPublication(*r)
				r.SourceRef.QueryWindowKnown = false
				hmc277SetMeasurementPublication(t, r, p)
			case "ambiguous_producer":
				result.Observations = append(result.Observations, result.Observations[0])
			}
			pair := NewNativeMeasurementPair(owner, [2]NativeMeasurementPairSide{{Request: json.RawMessage(`{"source":"path","view":"window_stats","time_start":0,"time_end":1}`), Result: result}})
			wrapper := ToolResult{ToolName: "trace_query", Success: true, RuntimeMeasurementPair: pair}
			switch mode {
			case "foreign_run":
				input.runtimeMeasurementPairConsumer = NewMutableState("different run")
			case "reset_run":
				owner.ResetTurnAArtifacts()
			case "stale_source":
				if err := os.WriteFile(result.Observations[0].SourceRef.Path, []byte("replacement physical generation\n"), 0600); err != nil {
					t.Fatal(err)
				}
			case "json_replay":
				data, _ := json.Marshal(wrapper)
				var restored ToolResult
				if err := json.Unmarshal(data, &restored); err != nil {
					t.Fatal(err)
				}
				wrapper = restored
			case "containing_window":
				*input.RequestModel.RuntimeArtifactScopeProfile.TimeStart = 0
				*input.RequestModel.RuntimeArtifactScopeProfile.TimeEnd = 2
			case "outside_window":
				*input.RequestModel.RuntimeArtifactScopeProfile.TimeStart = 2
				*input.RequestModel.RuntimeArtifactScopeProfile.TimeEnd = 3
			}
			if !input.RequestModel.RuntimeArtifactScopeProfile.HasExplicitTimeWindows() {
				t.Fatal("fixture requires a valid typed request window")
			}
			input.ToolResults = []ToolResult{wrapper}
			if mode == "system_supplement" {
				input.SystemTraceSupplementResults, input.ToolResults = input.ToolResults, nil
			}
			fixed := 0
			for _, table := range BuildRuntimeMeasurementContract(input).Choices() {
				if table.HasNativeMeasurementFacts() {
					fixed++
				}
			}
			want := 0
			if mode == "current" || mode == "system_supplement" {
				want = 2
			}
			if fixed != want {
				t.Fatalf("%s qualified %d fixed tables, want %d", mode, fixed, want)
			}
		})
	}
}

func TestHMC277SharedDisplayAdmissionPreservesDedicatedProducerBoundaries(t *testing.T) {
	for _, predicate := range []string{"io_inflight", "measurements_observation", "process_measurements_observation", "unpositioned_measurements_observation", RuntimeSamplingMeasurementPredicate, RuntimeResourceMeasurementPredicate, TraceEventSearchInventoryPredicate, "unregistered_producer"} {
		t.Run(predicate, func(t *testing.T) {
			input := hmc277CurrentMeasurementInput(t)
			result := input.ToolResults[0]
			p, _ := DecodeRuntimeMeasurementPublication(result.Observations[0])
			// Test admission separately: a current physical source and ordinary
			// table shape do not replace any dedicated producer's semantic seal.
			qualifyCurrentNativeMeasurementDisplay(&p, predicate, p.Source.Path, result.TraceQueryWindowReplay, input.RequestModel.RuntimeArtifactScopeProfile, false)
			if p.nativeFixedDisplay != (predicate == "io_inflight") {
				t.Fatal("shared admission borrowed or lost producer-specific display authority")
			}
		})
	}
}

func TestHMC277PairRawWithoutSemanticSealCannotBorrowGenericDisplay(t *testing.T) {
	for _, predicate := range []string{"measurements_observation", "process_measurements_observation"} {
		t.Run(predicate, func(t *testing.T) {
			input := hmc277CurrentMeasurementInput(t)
			result := input.ToolResults[0]
			result.Observations[0].Predicate = predicate
			pair := NewNativeMeasurementPair(input.runtimeMeasurementPairConsumer, [2]NativeMeasurementPairSide{{Request: json.RawMessage(`{"source":"path","view":"measurements"}`), Result: result}})
			input.ToolResults = []ToolResult{{ToolName: "trace_query", Success: true, RuntimeMeasurementPair: pair}}
			contract := BuildRuntimeMeasurementContract(input)
			if len(contract.Choices()) != 3 {
				t.Fatal("raw negative control never reached the source-bound pair path")
			}
			if contract.answerDisplayIdentities[result.Observations[0].ID] != "" {
				t.Fatal("generic pair fix expanded dedicated producer automatic display")
			}
			for _, table := range contract.Choices() {
				if table.ObservationID == result.Observations[0].ID && table.HasNativeFixedFacts() {
					t.Fatal("current pair invented raw semantics through generic fallback")
				}
			}
		})
	}
}
