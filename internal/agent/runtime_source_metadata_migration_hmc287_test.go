package agent

import (
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The old measurement oracles still own their exact rows and permissions.
// Separately account for each new source-declaration display, never silently
// ignore an extra selector or omit its rows from a shared preview budget.
func splitSourceMetadataHMC287(t *testing.T, all []types.RuntimeMeasurementTable, ranges map[string][3]string) ([]types.RuntimeMeasurementTable, []types.RuntimeMeasurementTable) {
	t.Helper()
	canonical := func(path string) string {
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			t.Fatal(err)
		}
		return resolved
	}
	want := make(map[string][3]string, len(ranges))
	for path, bounds := range ranges {
		want[canonical(path)] = bounds
	}
	seen := map[string]bool{}
	var measurements, metadata []types.RuntimeMeasurementTable
	for _, table := range all {
		if !strings.HasPrefix(table.ObservationID, "trace_source_metadata:") {
			measurements = append(measurements, table)
			continue
		}
		if table.View != types.RuntimeMeasurementSummary || !table.DefaultPresentation || table.MemberSet != nil || table.DisplayPopulation != nil || table.HasCompleteNativeDisplayPopulation() || table.CoversMemberSet(nil) {
			t.Fatalf("metadata changed its independent display-only contract: %+v", table)
		}
		if !reflect.DeepEqual(table.Columns, []string{"原始采集", "记录位置", "字段类别", "字段", "状态", "原值", "说明"}) || len(table.Rows) != 7 || len(table.Notes) != 1 {
			t.Fatalf("unexpected fixture metadata shape: %+v", table)
		}
		if len(table.Rows[0]) != 7 {
			t.Fatal("metadata lacks original-source column")
		}
		path := canonical(table.Rows[0][0])
		bounds, ok := want[path]
		if !ok || seen[path] {
			t.Fatalf("unexpected or duplicated original metadata source %s", path)
		}
		seen[path] = true
		missing := []struct{ origin, kind, field string }{
			{"device_info", "device", "physical_width"},
			{"device_info", "device", "physical_height"},
			{"device_info", "device", "physical_frame_rate"},
			{"meta", "parser", "table"},
		}
		for i, field := range missing {
			row := table.Rows[i]
			if len(row) != 7 || canonical(row[0]) != path || !reflect.DeepEqual(row[1:], []string{field.origin, field.kind, field.field, "missing", `""`, ""}) {
				t.Fatalf("existing missing metadata field changed: %v", row)
			}
		}
		for i, name := range []string{"start_ts", "end_ts", "duration_ns"} {
			row := table.Rows[4+i]
			state := "observed"
			if bounds[i] == "" {
				state = "missing"
			}
			if len(row) != 7 || canonical(row[0]) != path || !reflect.DeepEqual(row[1:6], []string{"trace_range", "source_range", name, state, strconv.Quote(bounds[i])}) || !strings.Contains(row[6], "单位 ns") || !strings.Contains(row[6], "不是") {
				t.Fatalf("source range declaration changed or borrowed query window: %v want=%v", row, bounds)
			}
		}
		metadata = append(metadata, table)
	}
	if len(seen) != len(want) {
		t.Fatalf("independent metadata source count=%d want=%d", len(seen), len(want))
	}
	return measurements, metadata
}
