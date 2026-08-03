package hitraceconv

import (
	"reflect"
	"testing"
)

func TestTraceDBCoverageDiagnosticWitnessKeysUsesProducerNamingContract(t *testing.T) {
	metadata := map[string]string{
		"unknown_comm_witnesses":                                "a",
		"longest_accepted_span_witness":                         "b",
		"official_viewer_typed_only_sync_witnesses_cpu_unknown": "c",
		"raw_marker_local_validation_witnesses_emitted":         "1",
		"format_record_witnesses_omitted":                       "2",
		"incomplete_format_witness_cap":                         "3",
		"ordinary_state":                                        "complete",
		"Unsafe_Witnesses":                                      "no",
		"unsafe_witnesses\nforged":                              "no",
	}
	want := []string{
		"longest_accepted_span_witness",
		"official_viewer_typed_only_sync_witnesses_cpu_unknown",
		"unknown_comm_witnesses",
	}
	if got := TraceDBCoverageDiagnosticWitnessKeys(metadata); !reflect.DeepEqual(got, want) {
		t.Fatalf("diagnostic witness keys = %+v, want %+v", got, want)
	}
}
