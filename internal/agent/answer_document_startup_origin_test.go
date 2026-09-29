package agent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestStartupNameOriginSQLiteToActualFinalizer(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_referenced_dictionary/capture.data")
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m, err := traceinput.Prepare(context.Background(), traceinput.Options{InputPath: path, RuntimeAnchor: t.TempDir(), PreviewBytes: 512})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	bus := &types.BusContext{RepoRoot: dir, WorkDir: dir, AttachedTraceMaterial: m, AttachedHitrace: m.Preview(), AttachedHitraceSource: path}
	result, err := (&tool.TraceQuery{}).Execute(bus, json.RawMessage(`{"source":"attached_trace","view":"event_search","event_types":["trace_mark"],"time_start":1.0,"time_end":1.08,"limit":40}`))
	if err != nil || !result.Success {
		t.Fatalf("query: %v %s", err, result.Summary)
	}
	ctx := traceEventInventoryPublicContext([]types.ToolResult{result})
	ledger, _ := json.Marshal(answerDocObservationLedger(ctx))
	views := traceEventInventoryPromptViews(t, traceEventInventoryActualFinalizerPrompt(t, ctx))
	if len(views) != 1 || len(views[0].Inventory.Rows) != 8 || !views[0].Inventory.RowsComplete {
		t.Fatalf("lost 4 complete stage endpoint pairs: %+v", views)
	}
	wants := []struct{ value, status, reason string }{{"BOOTSTRAP", "known", ""}, {"", "unavailable", "null_reference"}, {"", "unavailable", "unresolved_reference"}, {"LoadPreferences", "known", ""}}
	for i, row := range views[0].Inventory.Rows {
		if row.Semantics == nil || !types.ValidateTraceEventSemantics(row.Semantics) {
			t.Fatalf("row %d lost valid semantics", i)
		}
		fields := map[string]types.TraceEventSemanticField{}
		for _, f := range row.Semantics.Fields {
			fields[f.Key] = f
		}
		want := wants[i/2]
		f := fields["marker.business_name"]
		if f.Status != want.status || f.IssueReason != want.reason || (f.Value != nil && *f.Value != want.value) || (f.Value == nil && want.status == "known") {
			t.Fatalf("row %d wrong business identity: %+v", i, f)
		}
		for key, value := range map[string]string{"source.table": "app_startup", "source.representation": "sql_app_startup", "marker.label_origin": "synthesized_sql_label"} {
			if fields[key].Value == nil || *fields[key].Value != value {
				t.Fatalf("row %d missing provenance %s", i, key)
			}
		}
	}
	after, _ := json.Marshal(answerDocObservationLedger(ctx))
	current, err := os.ReadFile(path)
	if err != nil || sha256.Sum256(current) != sha256.Sum256(before) || string(ledger) != string(after) {
		t.Fatal("read-only source/handoff changed")
	}
}
