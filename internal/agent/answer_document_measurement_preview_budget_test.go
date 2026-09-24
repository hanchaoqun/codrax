package agent

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRuntimeMeasurementPreviewBudgetProtectsEverySummary(t *testing.T) {
	var tables []types.RuntimeMeasurementTable
	for group := 0; group < 33; group++ {
		// Deliberately put large detail tables before each summary. The precise
		// view, not producer order or the spelling of an ID, controls priority.
		for _, view := range []types.RuntimeMeasurementView{types.RuntimeMeasurementTimeline, types.RuntimeMeasurementDistribution, types.RuntimeMeasurementSummary} {
			rows := [][]string{{"0"}, {"known"}, {"unknown"}, {"ratio"}, {"extra detail"}}
			tables = append(tables, types.RuntimeMeasurementTable{ObservationID: fmt.Sprintf("group-%d", group), View: view, Rows: rows})
		}
	}
	roster, omitted := runtimeMeasurementHandoffRoster(tables, 32)
	if len(roster) != 96 || omitted != 1 {
		t.Fatalf("roster budget changed: %d tables, %d omitted groups", len(roster), omitted)
	}
	counts := runtimeMeasurementHandoffPreviewRows(roster)
	total := 0
	for i, table := range roster {
		want := 0
		if table.View == types.RuntimeMeasurementSummary {
			want = 4
		}
		if counts[i] != want || len(table.Rows) != 5 {
			t.Fatalf("summary starvation or output mutation: %s/%s count=%d", table.ObservationID, table.View, counts[i])
		}
		total += counts[i]
	}
	if total != 128 {
		t.Fatalf("global row budget=%d, want 128", total)
	}
}

func TestRuntimeMeasurementPreviewBudgetSharesRemainingRows(t *testing.T) {
	var tables []types.RuntimeMeasurementTable
	for group := 0; group < 32; group++ {
		for _, view := range []types.RuntimeMeasurementView{types.RuntimeMeasurementTimeline, types.RuntimeMeasurementMembers, types.RuntimeMeasurementSummary} {
			rows := [][]string{{"first"}, {"second"}, {"third"}, {"fourth"}, {"fifth"}}
			if view == types.RuntimeMeasurementSummary {
				rows = rows[:1]
			}
			tables = append(tables, types.RuntimeMeasurementTable{ObservationID: fmt.Sprintf("group-%d", group), View: view, Rows: rows})
		}
	}
	counts := runtimeMeasurementHandoffPreviewRows(tables)
	total := 0
	for i, table := range tables {
		if counts[i] < 1 || counts[i] > 4 {
			t.Fatalf("an early detail table monopolized rows: index=%d count=%d", i, counts[i])
		}
		if table.View == types.RuntimeMeasurementSummary && counts[i] != 1 {
			t.Fatalf("summary changed: %d", counts[i])
		}
		total += counts[i]
	}
	if total != 128 || !reflect.DeepEqual(counts, runtimeMeasurementHandoffPreviewRows(tables)) {
		t.Fatalf("budget is not bounded/deterministic: total=%d", total)
	}
	if len(runtimeMeasurementHandoffPreviewRows(nil)) != 0 {
		t.Fatal("empty supply gained data")
	}
}
