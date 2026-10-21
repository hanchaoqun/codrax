package agent

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC272TimelinePreviewBudgetAndProducerOrder(t *testing.T) {
	// Deliberately grouped by CPU, not globally sorted by time. The helper
	// cannot pretend whole-table endpoints cover each series' boundaries.
	table := types.RuntimeMeasurementTable{View: types.RuntimeMeasurementTimeline, DefaultPresentation: true,
		Columns: []string{"subject", "time"}, Rows: [][]string{{"CPU0", "1"}, {"CPU0", "2"}, {"CPU1", "0"}, {"CPU1", "3"}}}
	before, _ := json.Marshal(table)
	for _, tc := range []struct {
		count int
		want  []int
	}{
		{-1, nil}, {0, nil}, {1, []int{0}}, {2, []int{0, 3}}, {3, []int{0, 1, 3}}, {4, []int{0, 1, 2, 3}}, {8, []int{0, 1, 2, 3}},
	} {
		p := runtimeMeasurementHandoffPreview(table, tc.count)
		if !reflect.DeepEqual(p.SourceRowIndices, tc.want) || len(p.Rows) != len(tc.want) || p.RowIndexBase == nil || *p.RowIndexBase != 0 {
			t.Fatalf("budget %d: indices=%v rows=%v", tc.count, p.SourceRowIndices, p.Rows)
		}
		for i, index := range p.SourceRowIndices {
			if !reflect.DeepEqual(p.Rows[i], table.Rows[index]) || i > 0 && p.SourceRowIndices[i-1] >= index {
				t.Fatalf("budget %d reordered or reinterpreted source: %+v", tc.count, p)
			}
		}
		if len(p.Rows) < len(table.Rows) {
			if p.Selection != "table_boundary_sample" || p.SeriesBoundaryCoverage != "not_claimed" {
				t.Fatalf("partial multi-series preview claims coverage: %+v", p)
			}
		} else if p.Selection != "all_retained_rows" || p.SeriesBoundaryCoverage != "" {
			t.Fatalf("complete small retained table misrepresented: %+v", p)
		}
	}
	after, _ := json.Marshal(table)
	if string(before) != string(after) {
		t.Fatal("preview changed the original table")
	}
	for _, empty := range [][][]string{nil, {}} {
		table.Rows = empty
		p := runtimeMeasurementHandoffPreview(table, 2)
		if len(p.Rows) != 0 || len(p.SourceRowIndices) != 0 || p.Selection != "all_retained_rows" {
			t.Fatalf("empty source gained values: %+v", p)
		}
	}
}

func TestHMC272TimelinePreviewRequiresTypedDisplayIntent(t *testing.T) {
	for _, tc := range []struct {
		view           types.RuntimeMeasurementView
		defaultDisplay bool
	}{
		{types.RuntimeMeasurementTimeline, false},
		{types.RuntimeMeasurementSummary, true},
		{types.RuntimeMeasurementMembers, true},
	} {
		table := types.RuntimeMeasurementTable{ObservationID: "spoof#thread_timeline_observation", View: tc.view, DefaultPresentation: tc.defaultDisplay,
			Columns: []string{"实际终点（秒）"}, Rows: [][]string{{"head"}, {"middle"}, {"tail"}}}
		p := runtimeMeasurementHandoffPreview(table, 2)
		if !reflect.DeepEqual(p.Rows, table.Rows[:2]) || p.Selection != "" || len(p.SourceRowIndices) != 0 {
			t.Fatal("ID/column spelling or non-timeline intent enabled boundary selection")
		}
	}
}
