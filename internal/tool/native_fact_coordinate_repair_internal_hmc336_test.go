package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC336CoordinateDiagnosticsDoNotGuessIdentity(t *testing.T) {
	pair := func(id, view string) types.AnswerRuntimeMeasurementReceipt {
		return types.AnswerRuntimeMeasurementReceipt{ObservationID: id, View: types.RuntimeMeasurementView(view)}
	}
	for _, tc := range []struct {
		name       string
		submitted  types.AnswerRuntimeMeasurementReceipt
		choices    []types.AnswerRuntimeMeasurementReceipt
		code, kind string
		candidates int
		unresolved int
	}{
		{"exact_before_suffix", pair("source:one", "other"), []types.AnswerRuntimeMeasurementReceipt{pair("source:one", "summary"), pair("other:source:one", "summary")}, "wrong_view", "exact_observation_id", 1, 0},
		{"same_id_multiple_views", pair("source:one", "other"), []types.AnswerRuntimeMeasurementReceipt{pair("source:one", "summary"), pair("source:one", "timeline")}, "wrong_view", "exact_observation_id", 2, 0},
		{"ambiguous_namespace", pair("one", "summary"), []types.AnswerRuntimeMeasurementReceipt{pair("sourceA:one", "summary"), pair("sourceB:one", "summary")}, "unknown_id", "exact_namespace_suffix", 2, 1},
		{"not_a_namespace_boundary", pair("one", "summary"), []types.AnswerRuntimeMeasurementReceipt{pair("source:someone", "summary")}, "unknown_id", "none", 0, 1},
		{"not_a_prefix_or_edit_match", pair("source:one!", "summary"), []types.AnswerRuntimeMeasurementReceipt{pair("source:one", "summary")}, "unknown_id", "none", 0, 1},
		{"empty_id", pair("", "summary"), []types.AnswerRuntimeMeasurementReceipt{pair("source:one", "summary")}, "unknown_id", "none", 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			selected := []types.AnswerRuntimeMeasurementReceipt{tc.submitted}
			before, _ := json.Marshal([]any{selected, tc.choices})
			issue := nativeFactHandoffViolation{"native_fact_handoff.runtime_measurements[0]", tc.code}
			r := nativeFactHandoffCoordinateRepairs(selected, []nativeFactHandoffViolation{issue}, tc.choices)
			if len(r.Entries) != 1 || r.Omitted != 0 || r.Unresolved != tc.unresolved || r.focused() != (tc.unresolved == 0) {
				t.Fatalf("wrong diagnostic coverage: %+v", r)
			}
			entry := r.Entries[0]
			if entry.MatchKind != tc.kind || len(entry.CurrentCandidates) != tc.candidates || !reflect.DeepEqual(entry.Submitted, tc.submitted) {
				t.Fatalf("guessed or changed coordinates: %+v", entry)
			}
			for _, candidate := range entry.CurrentCandidates {
				found := false
				for _, current := range tc.choices {
					found = found || reflect.DeepEqual(current, candidate)
				}
				if !found {
					t.Fatal("invented a candidate not in the bounded live roster")
				}
			}
			hint := (managedDeliveryNativeHandoffError{violations: []nativeFactHandoffViolation{issue}, choices: tc.choices, coordinates: r}).hint()
			if strings.Contains(hint, "Copy selected whole objects from these current pairs") != (tc.unresolved != 0) {
				t.Fatal("ambiguous/unresolved coordinates lost the full-roster fallback")
			}
			after, _ := json.Marshal([]any{selected, tc.choices})
			if string(before) != string(after) {
				t.Fatal("diagnosis changed the submitted or current objects")
			}
		})
	}
}

func TestHMC336CoordinateRepairWholeUnitBudget(t *testing.T) {
	var selected, choices []types.AnswerRuntimeMeasurementReceipt
	var issues []nativeFactHandoffViolation
	for i := 0; i < 16; i++ {
		id := fmt.Sprintf("entry-%d-", i) + strings.Repeat("字", 85)
		if i == 0 {
			id = strings.Repeat("字", 1600) // Cannot fit even as a single whole unit.
		}
		selected = append(selected, types.AnswerRuntimeMeasurementReceipt{ObservationID: id, View: "summary"})
		choices = append(choices, types.AnswerRuntimeMeasurementReceipt{ObservationID: "source:" + id, View: "summary"})
		issues = append(issues, nativeFactHandoffViolation{fmt.Sprintf("native_fact_handoff.runtime_measurements[%d]", i), "unknown_id"})
	}
	before, _ := json.Marshal([]any{selected, choices, issues})
	r := nativeFactHandoffCoordinateRepairs(selected, issues, choices)
	wire, err := json.Marshal(r)
	if err != nil || len(wire) > nativeFactCoordinateRepairByteLimit || len(r.Entries) == 0 || r.Omitted == 0 || r.focused() || r.Unresolved != 0 || len(r.Entries)+r.Omitted != 16 {
		t.Fatalf("whole diagnostic budget lost coverage accounting: %d bytes, %+v, %v", len(wire), r, err)
	}
	for _, entry := range r.Entries {
		if len(entry.CurrentCandidates) != 1 || entry.CurrentCandidates[0].ObservationID != "source:"+entry.Submitted.ObservationID {
			t.Fatalf("candidate list or identity was truncated: %+v", entry)
		}
	}
	after, _ := json.Marshal([]any{selected, choices, issues})
	if string(before) != string(after) {
		t.Fatal("budget modified original coordinates")
	}
	r = nativeFactHandoffCoordinateRepairs(selected[:1], []nativeFactHandoffViolation{{"native_fact_handoff.runtime_measurements", "selection_limit_16"}, {"native_fact_handoff.interpretation_units[0]", "invalid_unit_shape"}}, choices)
	if r.focused() || len(r.Entries) != 0 || r.Unresolved != 2 {
		t.Fatalf("unrelated structured errors were relabeled as coordinate repairs: %+v", r)
	}
}
