package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceQueryEventNamesPublicAndLegacyCategories(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_io_activity/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("event names")}
	for _, tc := range []struct {
		name     string
		params   map[string]any
		want     int
		expected []string
	}{
		{"mmc", map[string]any{"event_names": []string{"mmc_request_start", "mmc_request_done"}}, 2, []string{"mmc_request_start", "mmc_request_done"}},
		{"f2fs", map[string]any{"event_names": []string{"f2fs_sync_file_enter", "f2fs_sync_file_exit"}}, 2, []string{"f2fs_sync_file_enter", "f2fs_sync_file_exit"}},
		{"bio", map[string]any{"event_names": []string{"block_bio_queue", "block_bio_complete"}}, 2, []string{"block_bio_queue", "block_bio_complete"}},
		{"legacy_bio_alias", map[string]any{"event_types": []string{"block_bio_queue", "block_bio_complete"}}, 18, nil},
		{"case_sensitive", map[string]any{"event_names": []string{"MMC_REQUEST_START"}}, 0, []string{"MMC_REQUEST_START"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.params["path"], tc.params["view"], tc.params["limit"] = path, "event_search", 40
			r := businessRefTestQuery(t, ctx, tc.params)
			record := requireEventSearchInventory(t, r)
			i := record.EventSearchInventory
			if i.Coverage.MatchedTotal != tc.want || !reflect.DeepEqual(i.Query.EventNames, tc.expected) {
				t.Fatalf("query/name/count mismatch: %+v", i)
			}
			for _, row := range i.Rows {
				if row.EventName == "" {
					t.Fatal("publication lost parser-retained event name")
				}
			}
			if len(i.Query.EventNames) > 0 && len(i.Rows) > 0 {
				copy := types.CloneTraceEventSearchInventory(i)
				copy.Query.EventNames[0] = "different_event"
				copy.Rows[0].EventName = "outside_filter"
				forged := record
				forged.EventSearchInventory = copy
				if types.IsValidTraceEventSearchInventoryRecord(forged) || i.Query.EventNames[0] != tc.expected[0] || i.Rows[0].EventName == "outside_filter" {
					t.Fatal("clone aliased original or contradictory exact-name receipt gained authority")
				}
			}
			if !strings.Contains(r.Summary, "event_filter_contract") {
				t.Fatal("missing nearby category/name distinction")
			}
			if len(i.Rows) > 0 && !strings.Contains(r.Summary, "io_query_navigation role=navigation_only") {
				t.Fatal("typed IO rows lack measurement navigation")
			}
		})
	}
}

func TestTraceQueryIOActivityNavigationPreservesOriginalScope(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_io_activity/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	for _, lineOnly := range []bool{false, true} {
		dir := t.TempDir()
		ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("statistics")}
		params := map[string]any{"source": "path", "path": path, "view": "event_search", "time_start": 2, "time_end": "2.25s", "event_names": []string{"block_rq_issue"}, "pattern": "16384", "bucket_ms": 50}
		if lineOnly {
			params["line_start"], params["line_end"] = 20, 20
		}
		r := businessRefTestQuery(t, ctx, params)
		payload := businessSpanSchedulerPublicPayload(t, r)
		if payload.WindowStats != nil {
			t.Fatal("advisory automatically executed measurement")
		}
		var next map[string]any
		for _, line := range strings.Split(r.Summary, "\n") {
			if _, encoded, ok := strings.Cut(line, "optional_next_trace_query="); ok {
				if err := json.Unmarshal([]byte(encoded), &next); err != nil {
					t.Fatal(err)
				}
			}
		}
		if next["view"] != "window_stats" || next["path"] != path || next["time_start"] != "2" || next["time_end"] != "2.25s" || next["bucket_ms"] != float64(50) {
			t.Fatalf("original bounds lost to lookup tolerance: %+v", next)
		}
		for _, field := range []string{"pattern", "event_names", "event_types", "pid", "thread"} {
			if _, exists := next[field]; exists {
				t.Fatalf("measurement inherited discovery/target filter %s", field)
			}
		}
		if lineOnly && (next["line_start"] != float64(20) || next["line_end"] != float64(20)) {
			t.Fatal("lost authoritative line range")
		}
		for _, note := range []string{"all issuers", "absent sizes are unknown", "No extra call is required", "not target-thread waiting", "lookup tolerance"} {
			if !strings.Contains(r.Summary, note) {
				t.Errorf("missing navigation boundary %s", note)
			}
		}
		follow := businessRefTestQuery(t, ctx, next)
		stats := businessSpanSchedulerPublicPayload(t, follow).WindowStats
		if stats == nil || stats.IOActivity == nil {
			t.Fatal("advertised route did not produce native measurement")
		}
		if !lineOnly && (stats.IOActivity.Window.StartTs != 2 || stats.IOActivity.Window.EndTs != 2.25) {
			t.Fatal("metric window inherited event lookup tolerance")
		}
	}
}

func TestTraceQueryIONavigationRequiresTypedEndpoint(t *testing.T) {
	for name, body := range map[string]string{
		"marker_payload":         "tracing_mark_write: B|7|mmc_request_start",
		"malformed_mmc":          "mmc_request_start: mmc0 blocks=2",
		"unsupported_filesystem": "ext4_sync_file_enter: dev 8,0 ino 9 parent 1 datasync 1",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "events.systrace")
			if err := os.WriteFile(path, []byte("worker-7 (7) [001] .... 1.000000: "+body+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("IO bandwidth")}
			r := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "event_search"})
			if !r.Success || strings.Contains(r.Summary, "io_query_navigation") {
				t.Fatalf("non-admitted IO acquired route: %s", r.Summary)
			}
		})
	}
}

func TestTraceQueryEventNameSchemaAndRejectBeforeSource(t *testing.T) {
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal((&TraceQuery{}).Parameters(), &schema); err != nil {
		t.Fatal(err)
	}
	var field struct {
		MaxItems    int    `json:"maxItems"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(schema.Properties["event_names"], &field); err != nil {
		t.Fatal(err)
	}
	if field.MaxItems != tracequery.EventSearchNameLimit || !strings.Contains(field.Description, "case-sensitive") || !strings.Contains((&TraceQuery{}).Description(), traceQueryEventNameTeaching) {
		t.Fatal("schema/description drift")
	}
	for _, raw := range []string{
		`{"view":"window_stats","event_names":["mmc_request_start"],"path":"/missing"}`,
		`{"view":"event_search","event_names":[" "],"path":"/missing"}`,
		`{"view":"event_search","event_names":["a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n", "o", "p", "q"],"path":"/missing"}`,
	} {
		r, err := (&TraceQuery{}).Execute(&types.BusContext{}, json.RawMessage(raw))
		if err != nil || r.Success || !strings.Contains(r.Summary, "event_names") {
			t.Fatalf("must reject exact parameter shape before IO: %v %+v", err, r)
		}
	}
}
