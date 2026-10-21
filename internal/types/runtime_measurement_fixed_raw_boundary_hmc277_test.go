package types

import (
	"encoding/json"
	"testing"
)

// A current physical-source read is not a replacement for the raw producer's
// independent semantic seal. This contract-boundary test keeps that real
// current read/replay receipt while damaging only the raw observation.
func TestHMC277GenericFixedFactsCannotResealInvalidRawProducer(t *testing.T) {
	for _, predicate := range []string{"measurements_observation", "process_measurements_observation"} {
		for _, mode := range []string{"valid", "missing_seal", "json_record", "semantic_unit", "changed_row", "changed_notes"} {
			t.Run(predicate+"/"+mode, func(t *testing.T) {
				input := hmc277CurrentMeasurementInput(t)
				result := &input.ToolResults[0]
				r := &result.Observations[0]
				r.Predicate, r.Role = predicate, AnswerAggregateRoleSupportingCoverage
				sealed, ok := PublishRuntimeRawMeasurement(*r)
				if !ok || !runtimeRawMeasurementValid(sealed) {
					t.Fatal("fixture must start with the real raw producer seal")
				}
				*r = sealed
				switch mode {
				case "missing_seal":
					r.runtimeRawMeasurementPublication = nil
				case "json_record":
					raw, err := json.Marshal(r)
					if err != nil {
						t.Fatal(err)
					}
					var restored ObservationRecord
					if err := json.Unmarshal(raw, &restored); err != nil {
						t.Fatal(err)
					}
					*r = restored
				case "semantic_unit":
					// The tables and actual source remain byte-identical. Only
					// an unverified physical interpretation has been introduced.
					r.Unit = "invented elapsed milliseconds"
				case "changed_row", "changed_notes":
					publication, valid := DecodeRuntimeMeasurementPublication(*r)
					if !valid {
						t.Fatal("valid raw fixture failed decode before mutation")
					}
					if mode == "changed_row" {
						publication.Tables[0].Rows[0][0] = "invented value"
					} else {
						publication.Tables[0].Notes = append(publication.Tables[0].Notes, "invented hardware state")
					}
					hmc277SetMeasurementPublication(t, r, publication)
				}
				wantRaw := mode == "valid"
				if runtimeRawMeasurementValid(*r) != wantRaw {
					t.Fatal("fixture did not independently invalidate the producer seal")
				}
				if source, _, current := input.runtimeMeasurementPairConsumer.ResolveTraceQueryWindowReplay(result.TraceQueryWindowReplay); !current || source != r.SourceRef.Path {
					t.Fatal("negative control accidentally lost its current physical-source receipt")
				}
				contract := BuildRuntimeMeasurementContract(input)
				if len(contract.Choices()) != 2 || contract.answerDisplayIdentities[r.ID] == "" {
					t.Fatal("legacy table/source eligibility disappeared, so this does not exercise generic fallback")
				}
				for _, table := range contract.Choices() {
					if table.HasNativeMeasurementFacts() {
						t.Fatal("raw producer domain borrowed generic fixed-fact authority")
					}
					if table.HasNativeRawFacts() != wantRaw || table.HasNativeFixedFacts() != wantRaw {
						t.Fatal("raw-specific fixed display did not retain its independent validation boundary")
					}
					if !table.IsValid() {
						t.Fatal("generic surface fix must not destroy the pre-existing ordinary table lane")
					}
				}
			})
		}
	}
}
