package tool

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/tracewire"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceQueryEventInventoryCoordinatesProducerSingleAuthority(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "trace_query_event_search_inventory.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	projected := 0
	ast.Inspect(file, func(node ast.Node) bool {
		if sel, ok := node.(*ast.SelectorExpr); ok {
			owner, _ := sel.X.(*ast.Ident)
			if owner != nil && owner.Name == "event" && (sel.Sel.Name == "CPU" || sel.Sel.Name == "PID" || sel.Sel.Name == "TGID") {
				t.Errorf("inventory publication bypassed typed coordinate projection: %s", sel.Sel.Name)
			}
		}
		if call, ok := node.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "ProjectTraceEventInventoryCoordinates" {
				projected++
			}
		}
		return true
	})
	if projected != 1 {
		t.Fatalf("coordinate authority calls=%d, want one per publication row", projected)
	}
}

func TestTraceQueryEventSearchInventoryPublicKnownUnknownCoordinates(t *testing.T) {
	marker, err := tracequery.FormatCPUUnavailableTraceMark(tracequery.CPUUnavailableTraceMark{TimestampNS: 1001000000, TID: 11, TGID: 10, SpanPID: 20, Action: "B", Comm: "app", Name: "initialize lib.so", Reason: tracequery.TraceMarkCPUReasonUnknownStart})
	if err != nil {
		t.Fatal(err)
	}
	sourceTID := int64(11)
	hisys, err := tracewire.FormatHiSysEventObservation(tracewire.HiSysEvent{TimestampNS: 1002000000, SourceTID: &sourceTID, Domain: tracewire.HiSysEventName{Status: "null_reference"}, Event: tracewire.HiSysEventName{Status: "null_reference"}, Contents: tracewire.HiSysEventContents{StorageClass: "null"}})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	body := "swapper-0 (0) [000] .... 1.000000: print: B|0|idle marker\n" + marker + "\n" + hisys + "\n"
	if err := os.WriteFile(filepath.Join(dir, "events.systrace"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	r, err := (&TraceQuery{}).Execute(ctx, json.RawMessage(`{"path":"events.systrace","view":"event_search","time_start":1,"time_end":1.01}`))
	if err != nil || !r.Success {
		t.Fatalf("public query %v %+v", err, r)
	}
	i := requireEventSearchInventory(t, r).EventSearchInventory
	if len(i.Rows) != 3 {
		t.Fatalf("rows %+v", i)
	}
	a, b, c := i.Rows[0], i.Rows[1], i.Rows[2]
	if a.CPUKnown == nil || !*a.CPUKnown || a.CPU != 0 || a.EmitterTIDKnown == nil || !*a.EmitterTIDKnown || a.EmitterTID != 0 {
		t.Fatalf("true zeros lost %+v", a)
	}
	if b.CPUKnown == nil || *b.CPUKnown || b.CPU != -1 || b.CPUUnknownReason != tracequery.TraceMarkCPUReasonUnknownStart || b.Raw != marker || b.EmitterTID != 11 || !*b.EmitterTIDKnown || b.MarkerPID != 20 {
		t.Fatalf("CPU placeholder crossed publication %+v", b)
	}
	if c.EmitterTIDKnown == nil || *c.EmitterTIDKnown || c.EmitterTID != -1 || *c.CPUKnown || c.CPU != -1 || c.Raw != hisys {
		t.Fatalf("SQL source TID became emitter %+v", c)
	}
	found := false
	for _, f := range b.Semantics.Fields {
		if f.Key == "marker.name" && f.Value != nil && *f.Value == "initialize lib.so" {
			found = true
		}
	}
	if !found {
		t.Fatal("decoded business name lost")
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInput{ToolResults: []types.ToolResult{r}})
	found = false
	for _, o := range ledger.Records {
		if o.EventSearchInventory != nil {
			found = true
			if o.EventSearchInventory.Rows[1].CPUKnown == nil || *o.EventSearchInventory.Rows[1].CPUKnown {
				t.Fatal("ledger lost explicit unknown")
			}
		}
	}
	if !found {
		t.Fatal("inventory missing from ledger")
	}
}

func TestTraceQueryEventSearchInventoryStaticDefaultPreparationCoordinates(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_static_initialize/capture.data")
	if err != nil {
		t.Fatal(err)
	}
	ctx, _, _ := hmc17NamedPathContext(t)
	r := hmc17NamedQuery(t, ctx, map[string]any{"source": "path", "path": path, "view": "event_search", "pid": 101, "time_start": 10, "time_end": 10.05, "event_types": []string{"trace_mark"}, "trace_mark_actions": []string{"B"}})
	i := requireEventSearchInventory(t, r).EventSearchInventory
	if len(i.Rows) != 2 {
		t.Fatalf("wrong target inventory %+v", i)
	}
	for n, row := range i.Rows {
		if row.EmitterTIDKnown == nil || !*row.EmitterTIDKnown || row.EmitterTID != 101 || row.CPUKnown == nil {
			t.Fatalf("lost target %+v", row)
		}
		if n == 0 && (!*row.CPUKnown || row.CPU != 0) {
			t.Fatalf("known CPU0 lost %+v", row)
		}
		if n == 1 && (*row.CPUKnown || row.CPU != -1 || row.CPUUnknownReason == "") {
			t.Fatalf("static unknown CPU promoted %+v", row)
		}
	}
}
