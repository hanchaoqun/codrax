package types

import (
	"context"
	"errors"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracecatalog"
)

func TestHMC225AcquisitionStatusDoesNotInferDataFromMissingPublications(t *testing.T) {
	a := tracecatalog.Artifact{ID: "a"}
	for _, test := range []struct {
		name    string
		outcome tracecatalog.Outcome
		err     error
		issues  []tracecatalog.Issue
		want    string
	}{
		{"not_executed", tracecatalog.OutcomeNotExecuted, nil, nil, "not_executed"},
		{"unqualified_empty", tracecatalog.OutcomeEmpty, nil, nil, "unavailable"},
		{"success_without_native", tracecatalog.OutcomeSuccess, nil, nil, "unavailable"},
		{"failed", tracecatalog.OutcomeFailure, nil, nil, "query_failed"},
		{"cancelled", tracecatalog.OutcomeCanceled, nil, nil, "cancelled"},
		{"stale", tracecatalog.OutcomeSuccess, tracecatalog.ErrStale, nil, "stale"},
		{"missing", tracecatalog.OutcomeNotExecuted, tracecatalog.ErrUnavailable, nil, "unavailable"},
		{"prep_failed", tracecatalog.OutcomeFailure, nil, []tracecatalog.Issue{{Code: "preparation_failed", Message: "no values available"}}, "prep_failed"},
		{"global_cancel", tracecatalog.OutcomeSuccess, context.Canceled, nil, "cancelled"},
		{"error_prose_not_semantics", tracecatalog.OutcomeFailure, errors.New("no data, successful empty, cancelled"), nil, "query_failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			a.Issues = test.issues
			q := tracecatalog.QueryRecord{Plan: tracecatalog.Plan{ArtifactID: "a"}, Attempts: []tracecatalog.Attempt{{Completion: tracecatalog.Completion{Outcome: test.outcome}}}}
			state, refs, _ := runtimeMeasurementAcquisitionSide(a, test.err, []tracecatalog.QueryRecord{q}, nil, nil)
			if state != test.want || len(refs) > 0 {
				t.Fatalf("state=%s want=%s refs=%v", state, test.want, refs)
			}
		})
	}
}

func TestHMC225AcquisitionStatusDoesNotPromoteSupplementaryWindow(t *testing.T) {
	for _, producer := range []string{"available", "empty"} {
		q := tracecatalog.QueryRecord{Plan: tracecatalog.Plan{ArtifactID: "a"}, Attempts: []tracecatalog.Attempt{{Completion: tracecatalog.Completion{Outcome: tracecatalog.OutcomeSuccess, PayloadRef: "native.json"}}}}
		p := RuntimeMeasurementPublication{ObservationID: "native", Source: ObservationSourceRef{PayloadRef: "native.json"}, Tables: []RuntimeMeasurementTable{{DefaultPresentation: false}}}
		state, refs, _ := runtimeMeasurementAcquisitionSide(tracecatalog.Artifact{ID: "a"}, nil, []tracecatalog.QueryRecord{q}, []RuntimeMeasurementPublication{p}, map[string]string{"native": producer})
		if state != "unavailable" || len(refs) != 0 {
			t.Fatalf("supplementary %s promoted to requested-window state=%s refs=%v", producer, state, refs)
		}
	}
}
