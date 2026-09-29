package agent

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceResourceSemanticsAndScanScopeActualFinalizer(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_native_resource_identity/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "event_search", "time_start": .0005, "time_end": .0055, "limit": 40})
	result, err := (&tool.TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, args)
	if err != nil || !result.Success {
		t.Fatalf("query: %v %s", err, result.Summary)
	}
	ctx := traceEventInventoryPublicContext([]types.ToolResult{result})
	before, _ := json.Marshal(answerDocObservationLedger(ctx))
	prompt := traceEventInventoryActualFinalizerPrompt(t, ctx)
	views := traceEventInventoryPromptViews(t, prompt)
	if len(views) != 1 || len(views[0].Inventory.Rows) != 10 {
		t.Fatalf("inventory: %+v", views)
	}
	c := views[0].Inventory.Coverage
	if c.ScanScope == nil || math.Abs(c.ScanScope.TimeStart-.00045) > 1e-12 || math.Abs(c.ScanScope.TimeEnd-.00555) > 1e-12 || c.ScanScope.ObservedCount != 10 || c.ScopeTimeStart != .001 || c.ScopeTimeEnd != .005 {
		t.Fatalf("scan confused with observations: %+v / %+v", c, c.ScanScope)
	}
	if !strings.Contains(prompt, "requested_window=0.000500..0.005500") || !strings.Contains(prompt, "matching_window=0.000450..0.005550") {
		t.Fatal("lookup expansion lost original requested window disclosure")
	}
	if strings.Contains(prompt, `"scope_time_start"`) || !strings.Contains(prompt, `"observed_time_start"`) {
		t.Fatal("ambiguous legacy keys leaked into finalizer")
	}
	for n, want := range []map[string]string{
		{"resource.address_i64": "9007199254740993", "resource.address_bits_hex": "0x0020000000000001", "resource.sub_type_id": "7", "resource.sub_type_name": "缓存|纹理"},
		{"resource.address_i64": "0", "resource.sub_type_name": ""},
		{"resource.address_i64": "-1", "resource.address_bits_hex": "0xffffffffffffffff"},
		{"resource.sub_type_id": "99"},
		{"resource.address_i64": "-9223372036854775808", "resource.address_bits_hex": "0x8000000000000000"},
	} {
		row := views[0].Inventory.Rows[n*2]
		fields := map[string]types.TraceEventSemanticField{}
		if row.Semantics == nil {
			t.Fatal("missing parsed resource fields")
		}
		for _, f := range row.Semantics.Fields {
			fields[f.Key] = f
		}
		for key, value := range want {
			f := fields[key]
			if f.Status != "known" || f.Value == nil || *f.Value != value {
				t.Fatalf("row %d %s: %+v", n, key, f)
			}
		}
		if n == 2 && fields["resource.sub_type_name"].IssueReason != "source_null" || n == 3 && fields["resource.sub_type_name"].IssueReason != "not_published" {
			t.Fatal("null/missing conflated")
		}
		if fields["resource.size"].Unit != "" {
			t.Fatal("resource quantity assumed bytes")
		}
	}
	after, _ := json.Marshal(answerDocObservationLedger(ctx))
	current, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) || !reflect.DeepEqual(current, original) {
		t.Fatal("display mutated source/ledger")
	}
}
