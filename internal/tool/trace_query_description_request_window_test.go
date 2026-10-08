package tool

import (
	"strings"
	"testing"
)

// Independently pin the only approved HMC §207 description addition. Older
// evolution tests reverse this exact suffix and keep their historical hashes;
// changing unrelated wording cannot be hidden by refreshing a golden hash.
func traceQueryDescriptionBeforeRequestWindowEvolution(t *testing.T, description string) string {
	t.Helper()
	description = traceQueryDescriptionBeforePreparationEvolution(t, description)
	const approved = "When both time bounds are omitted and there is no line scope or business_span_ref, trace_query defaults to the current request's one validated explicit time window for the same admitted capture. Explicit tool bounds (including a single endpoint) are never overwritten; multiple request windows are never combined or selected automatically. Without that request scope the existing whole-observed-source defaults remain unchanged."
	if traceQueryRequestWindowTeaching != approved || strings.Count(description, approved) != 1 || !strings.HasSuffix(description, " "+approved) {
		t.Fatal("request-window description delta must be the exact approved terminal addition")
	}
	return strings.TrimSuffix(description, " "+approved)
}

func TestTraceQueryDescriptionRequestWindowOnlyEvolution(t *testing.T) {
	prior := traceQueryDescriptionBeforeRequestWindowEvolution(t, (&TraceQuery{}).Description())
	if !strings.HasSuffix(prior, " "+traceQueryEventNameTeaching) {
		t.Fatal("request-window teaching changed prior terminal contract")
	}
}
