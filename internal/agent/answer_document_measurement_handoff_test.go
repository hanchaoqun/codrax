package agent

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRuntimeMeasurementActualFinalizerHandoff(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, scope := range []types.RuntimeQuestionScope{types.RuntimeQuestionScopeBoundedFactSet, types.RuntimeQuestionScopeCausalDiagnosis} {
			t.Run(lang+"/"+string(scope), func(t *testing.T) {
				ctx, result, native := ioInFlightPublicContext(t, lang, scope, "")
				before, _ := json.Marshal(result)
				view := types.BuildAnswerSemanticViewForAgentContext(ctx)
				choices := view.RuntimeMeasurementContract.Choices()
				if len(native.Groups) != 4 || len(choices) != 27 {
					t.Fatalf("actual producer must retain 12 paired tables and add 15 endpoint tables: groups=%d tables=%d", len(native.Groups), len(choices))
				}
				assertRuntimeMeasurementFixturePopulations(t, result, choices)
				prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
				for _, table := range choices {
					if !strings.Contains(prompt, "observation_id=\""+table.ObservationID+"\" view=\""+string(table.View)+"\"") {
						t.Fatalf("selector missing from actual finalizer instruction: %s/%s", table.ObservationID, table.View)
					}
				}
				for _, token := range []string{"Peak concurrent requests", "Starts inside query", "Actual start (s)", "0.998000", "1.012000", "preview_omitted_rows=", "No source evidence_items are needed", "Endpoint events", "Known bytes/s", "Size lower bound inclusive (B)", "additional selectable groups not previewed=1; selector roster groups omitted=0"} {
					if !strings.Contains(prompt, token) {
						t.Errorf("same-population selector teaching/preview lost %q", token)
					}
				}
				if strings.Contains(prompt, types.TraceNoteKeyRuntimeMeasurement+"={") {
					t.Error("raw publication JSON leaked into the model prompt")
				}
				after, _ := json.Marshal(result)
				if string(before) != string(after) || ctx.Mutable.TraceRootCauseReport() != nil {
					t.Fatal("display selector altered native result or acquired causal authority")
				}
			})
		}
	}
}

// Use producer predicates, not ID spelling or table titles, to distinguish
// the old four complete-pair groups from five new endpoint identity groups.
func assertRuntimeMeasurementFixturePopulations(t *testing.T, result types.ToolResult, choices []types.RuntimeMeasurementTable) {
	t.Helper()
	groups := map[string]int{}
	publications := map[string]types.RuntimeMeasurementTable{}
	for _, row := range result.Observations {
		p, ok := types.DecodeRuntimeMeasurementPublication(row)
		if !ok {
			continue
		}
		wantViews := []types.RuntimeMeasurementView{types.RuntimeMeasurementSummary, types.RuntimeMeasurementTimeline}
		switch row.Predicate {
		case "io_inflight":
			wantViews = append(wantViews, types.RuntimeMeasurementMembers)
		case "io_activity":
			wantViews = append(wantViews, types.RuntimeMeasurementDistribution)
		default:
			t.Fatalf("unexpected selectable producer %q", row.Predicate)
		}
		if row.Role != types.AnswerAggregateRoleSupportingCoverage || len(p.Tables) != 3 {
			t.Fatalf("measurement changed authority or view count: %+v", row)
		}
		groups[row.Predicate]++
		views := map[types.RuntimeMeasurementView]bool{}
		for _, table := range p.Tables {
			key := table.ObservationID + "/" + string(table.View)
			if _, duplicate := publications[key]; duplicate || views[table.View] {
				t.Fatalf("duplicate measurement selector %s", key)
			}
			publications[key], views[table.View] = table, true
		}
		for _, want := range wantViews {
			if !views[want] {
				t.Fatalf("%s lost its %s projection", row.Predicate, want)
			}
		}
	}
	if !reflect.DeepEqual(groups, map[string]int{"io_inflight": 4, "io_activity": 5}) || len(publications) != 27 || len(choices) != 27 {
		t.Fatalf("12 paired + 15 activity tables changed: groups=%v publications=%d choices=%d", groups, len(publications), len(choices))
	}
	for _, table := range choices {
		key := table.ObservationID + "/" + string(table.View)
		if !reflect.DeepEqual(table, publications[key]) {
			t.Fatalf("handoff altered source-bound values/units/population for %s", key)
		}
	}
}
