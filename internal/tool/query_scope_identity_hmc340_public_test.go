package tool

import (
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Equal content-addressed result bytes are not equal query identities. A
// legitimate second selector must not make the first query's native tables
// disappear as if one observation had conflicting source receipts.
func TestHMC340PublicEqualPayloadQueriesRetainNativeChoices(t *testing.T) {
	ctx, first := businessNativeSelfQuery339(t, 5, 5.012)
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_marker_tree/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	second := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "window_stats", "pid": 700, "time_start": 5, "time_end": 5.012, "limit": 201})
	if !second.Success {
		t.Fatal(second.Summary)
	}
	a, _, _ := businessNativeRows339(t, first, "business-700", "OpenDocument")
	b, _, _ := businessNativeRows339(t, second, "business-700", "OpenDocument")
	if a.SourceRef.PayloadRef != b.SourceRef.PayloadRef || a.SourceRef.QueryScopeID == b.SourceRef.QueryScopeID {
		t.Fatalf("fixture did not exercise different real queries sharing complete result bytes: %+v %+v", a.SourceRef, b.SourceRef)
	}
	if a.ID == b.ID {
		t.Errorf("different query scopes collided at native observation ID %s", a.ID)
	}
	ids := make(map[string]bool)
	predicates := make(map[string]bool)
	for _, r := range first.Observations {
		ids[r.ID] = true
		predicates[r.Predicate] = true
	}
	if len(predicates) < 3 {
		t.Fatal("public query did not cover independent typed observation families")
	}
	for _, r := range second.Observations {
		if ids[r.ID] {
			t.Errorf("query collision also reached predicate %s: %s", r.Predicate, r.ID)
		}
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{first, second}})
	contract := types.BuildAnswerSemanticViewForBusContext(ctx).RuntimeMeasurementContract
	for _, record := range []types.ObservationRecord{a, b} {
		r := types.AnswerRuntimeMeasurementReceipt{ObservationID: record.ID, View: types.RuntimeMeasurementSummary}
		if !types.BindRuntimeMeasurementReceipt(&r, contract) || len(r.BoundTable.Rows) != 13 {
			t.Errorf("a lawful later query hid an existing whole native account: %s", record.ID)
		}
	}
	// A true repeated query still has stable coordinates. This is not a
	// random execution suffix, nor permission to merge different queries.
	repeat := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "window_stats", "pid": 700, "time_start": 5, "time_end": 5.012, "limit": 201})
	c, _, _ := businessNativeRows339(t, repeat, "business-700", "OpenDocument")
	if c.ID != b.ID || c.SourceRef.QueryScopeID != b.SourceRef.QueryScopeID {
		t.Fatal("an exact repeated query acquired a different observation identity")
	}
}
