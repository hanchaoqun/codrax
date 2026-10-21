package agent

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

// This is a constructed SQLite capture, not a device capture. Unlike the
// exported-text tests, the public tool must first prepare the original file.
// No expected answer or resource interpretation is supplied to that tool.
func hmc274ResourceCapture(t *testing.T, label string) (string, []byte) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "capture.data")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	statements := []string{
		"CREATE TABLE trace_range (start_ts INT)", "INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)", "INSERT INTO process VALUES (1, 100, 'app')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (1, 100, 1, 'app-main', 0, 1, 1)",
		"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
		"INSERT INTO thread_state VALUES (1, 0, 9000000, 1, 'Running')",
		// Empty authoritative families are distinct from absent tables. The
		// shared lifecycle admission must be exercised, not bypassed by tests.
		"CREATE TABLE instant (ts INT, name TEXT, ref INT, wakeup_from INT, ref_type TEXT)",
		"CREATE TABLE sched_slice (ts INT, dur INT, cpu INT, itid INT, end_state TEXT, priority INT)",
		"CREATE TABLE callstack (id INT, ts INT, dur INT, itid INT, callid INT, name TEXT, flag TEXT, cookie INT, chainId TEXT, depth INT)",
		"CREATE TABLE syscall (ts INT, dur INT, syscall_number INT, itid INT)",
		"CREATE TABLE frame_slice (id INT, type INT, ts INT, itid INT)",
		"CREATE TABLE app_startup (start_time INT, end_time INT, start_name INT, ipid INT)",
		"CREATE TABLE static_initalize (start_time INT, end_time INT, so_name TEXT, ipid INT, tid INT)",
		"CREATE TABLE native_hook (id INT, start_ts INT, end_ts INT, event_type TEXT, all_heap_size INT, itid INT, ipid INT, heap_size, callchain_id, addr, sub_type_id)",
		"INSERT INTO native_hook VALUES (1,1000000,9223372036854775807,'AllocEvent',8192,1,1,9007199254740993,9007199254740995,9007199254740993,7)",
		"INSERT INTO native_hook VALUES (2,2000000,0,'FreeEvent',4096,1,1,0,-1,0,0)",
		"INSERT INTO native_hook VALUES (3,3000000,NULL,'MmapEvent',4096,1,1,4096,NULL,-1,8)",
		"INSERT INTO native_hook VALUES (4,4000000,NULL,'AllocEvent',4128,1,1,32,12,NULL,99)",
		"INSERT INTO native_hook VALUES (5,5000000,NULL,'MmapEvent',8192,1,1,4096,13,-9223372036854775808,NULL)",
		"INSERT INTO native_hook VALUES (6,5250000,NULL,'MmapEvent',8256,1,1,64,14,1,9)",
		"INSERT INTO native_hook VALUES (7,6000000,9000000,'FD_Open_Event',1,1,1,1,15,17,10)",
		"CREATE TABLE data_dict (id, data)",
		"INSERT INTO data_dict VALUES (0,'')", "INSERT INTO data_dict VALUES (8,NULL)",
		"INSERT INTO data_dict VALUES (9,'first')", "INSERT INTO data_dict VALUES (9,'second')",
		"INSERT INTO data_dict VALUES (10,'outside')",
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("fixture SQL: %v", err)
		}
	}
	if _, err := db.Exec("INSERT INTO data_dict VALUES (7,?)", label); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return path, data
}

func hmc274ResourceQuery(t *testing.T, ctx *types.AgentContext, path, pattern string, start, end float64) types.ToolResult {
	t.Helper()
	args, err := json.Marshal(map[string]any{"source": "path", "path": path, "view": "event_search", "event_types": []string{"trace_mark"}, "pattern": pattern, "time_start": start, "time_end": end, "limit": 40})
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
	if err != nil || !result.Success {
		t.Fatalf("public original-file query: %v %s", err, result.Summary)
	}
	return result
}

func hmc274ResourceContext(t *testing.T) *types.AgentContext {
	t.Helper()
	ctx := traceEventInventoryPublicContext(nil)
	ctx.RepoRoot, ctx.WorkDir = t.TempDir(), t.TempDir()
	ctx.Stage, ctx.AgentName, ctx.Language = types.StageFinalize, types.AgentFinalizer, "zh"
	ctx.TraceInputPreparer = traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(ctx.WorkDir, ".codrax"), PreviewBytes: 16384})
	start, end := .0005, .0055
	ctx.AnalysisIR.RequestModel.Intent = types.IntentExplain
	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet}
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "0.0005 to 0.0055 seconds"}
	return ctx
}

func hmc274ResourceFields(t *testing.T, row types.TraceEventSearchInventoryRow) map[string]types.TraceEventSemanticField {
	t.Helper()
	if row.Semantics == nil {
		t.Fatalf("missing native semantics for %s", row.Raw)
	}
	fields := map[string]types.TraceEventSemanticField{}
	for _, field := range row.Semantics.Fields {
		fields[field.Key] = field
	}
	return fields
}

func hmc274ResourceValue(t *testing.T, fields map[string]types.TraceEventSemanticField, key, value, status, issue string) {
	t.Helper()
	f, ok := fields[key]
	if !ok || f.Status != status || f.IssueReason != issue || status == "known" && (f.Value == nil || *f.Value != value) || status != "known" && f.Value != nil {
		t.Fatalf("field %s: got %+v; want %q/%s/%s", key, f, value, status, issue)
	}
}

func TestHMC274ResourceIdentityOriginalSQLiteToActualFinalizer(t *testing.T) {
	path, original := hmc274ResourceCapture(t, "缓存|纹理")
	ctx := hmc274ResourceContext(t)
	var results []types.ToolResult
	for _, pattern := range []string{"", "NativeHook:", "HeapSize", "MmapSize"} {
		results = append(results, hmc274ResourceQuery(t, ctx, path, pattern, .0005, .0055))
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	ledger := answerDocObservationLedger(ctx)
	before, _ := json.Marshal([]any{ledger, types.CompileTraceCausalProjectionSet(ledger)})
	prompt := traceEventInventoryActualFinalizerPrompt(t, ctx)
	views := traceEventInventoryPromptViews(t, prompt)
	if len(views) != 4 {
		t.Fatalf("independent query receipts lost: %d", len(views))
	}
	counts := map[string]int{"": 12, "NativeHook:": 6, "HeapSize": 3, "MmapSize": 3}
	for _, view := range views {
		i := view.Inventory
		want, ok := counts[i.Query.Pattern]
		if !ok || i.Coverage.MatchedTotal != want || len(i.Rows) != want || !i.RowsComplete || view.PromptRowsOmitted != 0 || i.QueryScopeID != view.Source.QueryScopeID || view.Source.QueryScopeID == "" {
			t.Fatalf("original query population/identity changed: pattern=%q matched=%d rows=%d complete=%t omitted=%d scope=%q", i.Query.Pattern, i.Coverage.MatchedTotal, len(i.Rows), i.RowsComplete, view.PromptRowsOmitted, view.Source.QueryScopeID)
		}
		delete(counts, i.Query.Pattern)
		if view.Source.Path == path || !strings.Contains(view.Source.Path, ".tracebundle.json") {
			t.Fatalf("test did not traverse complete original-file preparation: %s", view.Source.Path)
		}
		for _, row := range i.Rows {
			if row.TraceTimeSeconds >= .006 {
				t.Fatal("outside resource/counter entered the bounded query")
			}
		}
		if i.Query.Pattern != "NativeHook:" {
			continue
		}
		for n, row := range i.Rows {
			if row.MarkerPID != 100 || row.EmitterTID != 100 || row.Semantics == nil {
				t.Fatalf("resource owner or independent native field set lost: %+v", row)
			}
			fields := hmc274ResourceFields(t, row)
			resourceFields := 0
			for key := range fields {
				if strings.HasPrefix(key, "resource.") {
					resourceFields++
				}
			}
			if resourceFields != 8 {
				t.Fatalf("native resource field set lost: %+v", fields)
			}
			if fields["resource.size"].Unit != "" {
				t.Fatal("untyped source size was relabeled as bytes")
			}
			for key, value := range []map[string]string{
				{"resource.address_i64": "9007199254740993", "resource.address_bits_hex": "0x0020000000000001", "resource.sub_type_id": "7", "resource.sub_type_name": "缓存|纹理"},
				{"resource.address_i64": "0", "resource.address_bits_hex": "0x0000000000000000", "resource.sub_type_id": "0", "resource.sub_type_name": ""},
				{"resource.address_i64": "-1", "resource.address_bits_hex": "0xffffffffffffffff", "resource.sub_type_id": "8"},
				{"resource.sub_type_id": "99"},
				{"resource.address_i64": "-9223372036854775808", "resource.address_bits_hex": "0x8000000000000000"},
				{"resource.address_i64": "1", "resource.address_bits_hex": "0x0000000000000001", "resource.sub_type_id": "9"},
			}[n] {
				hmc274ResourceValue(t, fields, key, value, "known", "")
			}
			switch n {
			case 2:
				hmc274ResourceValue(t, fields, "resource.sub_type_name", "", "unavailable", "source_null")
			case 3:
				hmc274ResourceValue(t, fields, "resource.address_i64", "", "unavailable", "source_null")
				hmc274ResourceValue(t, fields, "resource.sub_type_name", "", "unavailable", "not_published")
			case 4:
				hmc274ResourceValue(t, fields, "resource.sub_type_id", "", "unavailable", "source_null")
				hmc274ResourceValue(t, fields, "resource.sub_type_name", "", "unavailable", "not_published")
			case 5:
				hmc274ResourceValue(t, fields, "resource.sub_type_name", "", "unavailable", "not_published")
			}
		}
	}
	for _, p := range types.CompileTraceCausalProjectionSet(ledger).Projections {
		if p.PrimaryRootCause != nil || len(p.PrimaryRootCauses) != 0 || len(p.OnChainCauses) != 0 {
			t.Fatal("resource metadata acquired chain/root-cause authority")
		}
	}
	afterLedger := answerDocObservationLedger(ctx)
	after, _ := json.Marshal([]any{afterLedger, types.CompileTraceCausalProjectionSet(afterLedger)})
	if !bytes.Equal(before, after) {
		t.Fatal("actual finalizer changed source facts or causal eligibility")
	}
	current, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(current, original) {
		t.Fatal("read-only original-file query changed source bytes")
	}
	files, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(files) != 1 || files[0].Name() != filepath.Base(path) {
		t.Fatal("read-only original-file query introduced source sidecars")
	}
}

func TestHMC274ResourceIdentityCaptureAndQueryScopesStayIndependent(t *testing.T) {
	ctx := hmc274ResourceContext(t)
	var results []types.ToolResult
	labels := []string{"capture-A", "capture-B"}
	for _, label := range labels {
		path, _ := hmc274ResourceCapture(t, label)
		results = append(results, hmc274ResourceQuery(t, ctx, path, "NativeHook:", .0005, .0055))
		results = append(results, hmc274ResourceQuery(t, ctx, path, "NativeHook:", .0005, .0015))
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	ledger := answerDocObservationLedger(ctx)
	want := map[string]types.ObservationRecord{}
	for _, row := range ledger.Records {
		if types.IsValidTraceEventSearchInventoryRecord(row) {
			want[row.SourceRef.QueryScopeID] = row
		}
	}
	views := traceEventInventoryPromptViews(t, traceEventInventoryActualFinalizerPrompt(t, ctx))
	if len(views) != 4 || len(want) != 4 {
		t.Fatalf("capture or query identity collapsed: views=%d original=%d", len(views), len(want))
	}
	seen := map[string]int{}
	for _, view := range views {
		r, found := want[view.Source.QueryScopeID]
		if !found || !reflect.DeepEqual(view.Source, r.SourceRef) || !reflect.DeepEqual(view.Inventory, r.EventSearchInventory) {
			t.Fatal("actual consumer changed independently scoped original facts")
		}
		if len(view.Inventory.Rows) == 0 {
			t.Fatal("fixture must publish original resource events")
		}
		fields := hmc274ResourceFields(t, view.Inventory.Rows[0])
		name := fields["resource.sub_type_name"]
		if name.Value == nil || (*name.Value != labels[0] && *name.Value != labels[1]) {
			t.Fatalf("same dictionary ID lost its own capture: %+v", name)
		}
		seen[fmt.Sprintf("%s/%d", *name.Value, view.Inventory.Coverage.MatchedTotal)]++
	}
	for _, label := range labels {
		for _, count := range []int{1, 6} {
			if seen[fmt.Sprintf("%s/%d", label, count)] != 1 {
				t.Fatalf("same-source narrow lookup borrowed broad denominator or foreign name: %v", seen)
			}
		}
	}
}
