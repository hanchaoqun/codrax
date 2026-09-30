package agent

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceConversionQualityReachesActualFinalizer(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_hisys_scalars/capture.data")
	if err != nil {
		t.Fatal(err)
	}
	m, err := traceinput.Prepare(context.Background(), traceinput.Options{InputPath: path, RuntimeAnchor: t.TempDir(), PreviewBytes: 16384})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	bus := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("system events"), AttachedTraceMaterial: m, AttachedHitrace: m.Preview(), AttachedHitraceSource: path}
	result, err := (&tool.TraceQuery{}).Execute(bus, json.RawMessage(`{"source":"attached_trace","view":"event_search","event_types":["hi_sysevent"],"time_start":2.0,"time_end":2.08,"limit":40}`))
	if err != nil || !result.Success {
		t.Fatalf("prepared query: %v %+v", err, result)
	}
	ctx := traceEventInventoryPublicContext([]types.ToolResult{result})
	views := traceEventInventoryPromptViews(t, traceEventInventoryActualFinalizerPrompt(t, ctx))
	if len(views) != 1 || len(views[0].Inventory.Rows) != 8 {
		t.Fatalf("valid events lost: %+v", views)
	}
	notes := strings.Join(views[0].Inventory.Caveats, "\n")
	for _, want := range []string{"table=hisys_all_event", "rows_read=10", "rows_emitted=8", "invalid_timestamp=2", "invalid_source_tid=5", "capture_state=unknown", "counts_scope=source_table_before_query_filters", "untimed_rows_have_no_query_window_assignment"} {
		if !strings.Contains(notes, want) {
			t.Errorf("actual finalizer missing %q: %s", want, notes)
		}
	}
	result, err = (&tool.EmitPerfTrace{}).Execute(bus, json.RawMessage(`{"meta":{"source":"hitrace","summary":"system events"},"observations":[{"kind":"line_anchor","subject":"system events","summary":"observed system event rows","evidence":"attached converted rows","confidence":1}]}`))
	if err != nil || !result.Success {
		t.Fatalf("pre-stage: %v %+v", err, result)
	}
	found := false
	for _, row := range bus.Mutable.PerfTrace().Observations {
		if row.Kind == "time_semantics" {
			found = true
			if row.EndTsMs < 2070 || row.EndTsMs > 2071 {
				t.Fatalf("timed comment events excluded from attachment extent: %+v", row)
			}
		}
	}
	if !found {
		t.Fatal("no attachment extent")
	}
}
