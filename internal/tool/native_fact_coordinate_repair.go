package tool

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

const nativeFactCoordinateRepairByteLimit = 4 << 10

// Coordinates are diagnostic navigation, never aliases accepted by the native
// source binder. Keep whole submitted/current pairs and disclose budget loss.
type nativeFactCoordinateRepair struct {
	Field             string                                  `json:"field"`
	Code              string                                  `json:"code"`
	Submitted         types.AnswerRuntimeMeasurementReceipt   `json:"submitted"`
	MatchKind         string                                  `json:"match_kind"`
	CurrentCandidates []types.AnswerRuntimeMeasurementReceipt `json:"current_candidates"`
}

type nativeFactCoordinateRepairs struct {
	Entries    []nativeFactCoordinateRepair `json:"entries"`
	Omitted    int                          `json:"omitted"`
	Unresolved int                          `json:"unresolved"`
}

func nativeFactHandoffCoordinateRepairs(selected []types.AnswerRuntimeMeasurementReceipt, issues []nativeFactHandoffViolation, choices []types.AnswerRuntimeMeasurementReceipt) nativeFactCoordinateRepairs {
	out := nativeFactCoordinateRepairs{Entries: []nativeFactCoordinateRepair{}}
	for _, issue := range issues {
		index := -1
		if issue.Code == "unknown_id" || issue.Code == "wrong_view" {
			for i := range selected {
				if issue.Field == fmt.Sprintf("native_fact_handoff.runtime_measurements[%d]", i) {
					index = i
					break
				}
			}
		}
		if index < 0 {
			out.Unresolved++
			continue
		}
		submitted := selected[index]
		entry := nativeFactCoordinateRepair{Field: issue.Field, Code: issue.Code,
			Submitted: types.AnswerRuntimeMeasurementReceipt{ObservationID: submitted.ObservationID, View: submitted.View},
			MatchKind: "none", CurrentCandidates: []types.AnswerRuntimeMeasurementReceipt{}}
		for _, choice := range choices {
			if choice.ObservationID == submitted.ObservationID {
				entry.MatchKind = "exact_observation_id"
				entry.CurrentCandidates = append(entry.CurrentCandidates, choice)
			}
		}
		if len(entry.CurrentCandidates) == 0 && submitted.ObservationID != "" {
			for _, choice := range choices {
				// A whole namespace boundary is only a soft repair lead. It
				// neither changes submitted coordinates nor proves identity.
				if strings.HasSuffix(choice.ObservationID, ":"+submitted.ObservationID) {
					entry.MatchKind = "exact_namespace_suffix"
					entry.CurrentCandidates = append(entry.CurrentCandidates, choice)
				}
			}
		}
		ids := map[string]bool{}
		for _, choice := range entry.CurrentCandidates {
			ids[choice.ObservationID] = true
		}
		if len(ids) != 1 {
			out.Unresolved++
		}
		trial := out
		trial.Entries = append(append([]nativeFactCoordinateRepair(nil), out.Entries...), entry)
		wire, _ := json.Marshal(trial)
		// Leave room for the final counters, never shorten an ID or drop a
		// candidate from inside an otherwise retained field-local unit.
		if len(out.Entries) == 16 || len(wire) > nativeFactCoordinateRepairByteLimit-128 {
			out.Omitted++
			continue
		}
		out.Entries = trial.Entries
	}
	return out
}

func (r nativeFactCoordinateRepairs) focused() bool {
	return len(r.Entries) > 0 && r.Omitted == 0 && r.Unresolved == 0
}
