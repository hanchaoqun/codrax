package agent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func traceSemanticPublicResult(t *testing.T, source string) types.ToolResult {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "business-events.trace")
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "event_search", "time_start": 2.0, "time_end": 2.08, "limit": 40})
	result, err := (&tool.TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
	if err != nil || !result.Success {
		t.Fatalf("public semantic query: %v / %s", err, result.Summary)
	}
	return result
}

func TestTraceEventSemanticsSQLitePreparationToActualFinalizer(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_hisys_semantics/capture.data")
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m, err := traceinput.Prepare(context.Background(), traceinput.Options{InputPath: path, RuntimeAnchor: t.TempDir(), PreviewBytes: 512})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	bus := &types.BusContext{RepoRoot: dir, WorkDir: dir, AttachedTraceMaterial: m, AttachedHitrace: m.Preview(), AttachedHitraceSource: path}
	result, err := (&tool.TraceQuery{}).Execute(bus, json.RawMessage(`{"source":"attached_trace","view":"event_search","event_types":["hi_sysevent"],"time_start":2.0,"time_end":2.08,"limit":40}`))
	if err != nil || !result.Success {
		t.Fatalf("actual prepared query: %v / %s", err, result.Summary)
	}
	ctx := traceEventInventoryPublicContext([]types.ToolResult{result})
	before, _ := json.Marshal(answerDocObservationLedger(ctx))
	views := traceEventInventoryPromptViews(t, traceEventInventoryActualFinalizerPrompt(t, ctx))
	if len(views) != 1 || len(views[0].Inventory.Rows) != 6 || views[0].Inventory.Coverage.MatchedTotal != 6 || !views[0].Inventory.RowsComplete {
		t.Fatalf("SQL preparation/handoff lost population: %+v", views)
	}
	for n, row := range views[0].Inventory.Rows {
		if row.Semantics == nil || !types.ValidateTraceEventSemantics(row.Semantics) {
			t.Fatalf("row %d has no typed semantics", n)
		}
		fields := map[string]types.TraceEventSemanticField{}
		for _, f := range row.Semantics.Fields {
			fields[f.Key] = f
		}
		for key, want := range map[string]string{"plugin.domain": "CAMERA_PIPELINE", "plugin.event_name": "CAPTURE_DONE"} {
			if n != 1 && n != 5 {
				continue
			}
			f := fields[key]
			if f.Status != "known" || f.Value == nil || *f.Value != want {
				t.Fatalf("row %d %s=%+v", n, key, f)
			}
		}
		if n == 2 && fields["plugin.domain"].Status != "unavailable" || n == 3 && fields["plugin.event_name"].Status != "unavailable" {
			t.Fatalf("unknown reference manufactured an identity: row %d %+v", n, fields)
		}
		if n == 4 {
			if fields["source.contents"].Value == nil || *fields["source.contents"].Value != "file=\"片段甲.mp4\"\nstate=ready" || fields["plugin.domain"].Value == nil || *fields["plugin.domain"].Value != "Media/管线" {
				t.Fatalf("special content/name changed: %+v", fields)
			}
		}
		if n == 5 {
			f := fields["plugin.contents"]
			if f.Value == nil || !strings.HasSuffix(*f.Value, " result=complete") || !row.RawTruncated || strings.Contains(row.Raw, "result=complete") {
				t.Fatalf("content beyond preview not available to finalizer: %+v", row)
			}
		}
	}
	after, _ := json.Marshal(answerDocObservationLedger(ctx))
	current, err := os.ReadFile(path)
	if err != nil || sha256.Sum256(current) != sha256.Sum256(original) || string(before) != string(after) {
		t.Fatal("read-only handoff changed source or accepted evidence")
	}
}

func TestTraceEventSemanticsActualFinalizerPreservesParsedBusinessFields(t *testing.T) {
	longName := "upload-" + strings.Repeat("segment", 90) + "-last"
	result := traceSemanticPublicResult(t,
		"writer-101 (101) [001] .... 1.999000: tracing_mark_write: I|201|outside\n"+
			"<hisysevent>-101 (101) [001] .... 2.005000: print: CAMERA_PIPELINE/FRAME_READY: session=17\n"+
			"writer-101 (101) [001] .... 2.010000: tracing_mark_write: C|201|HeapSize|0\n"+
			"writer-101 (101) [001] .... 2.020000: tracing_mark_write: B|201|"+longName+"\n"+
			"writer-101 (101) [001] .... 2.021000: tracing_mark_write: E|201\n"+
			"writer-101 (101) [001] .... 2.081000: tracing_mark_write: I|201|outside\n")
	ctx := traceEventInventoryPublicContext([]types.ToolResult{result})
	var original *types.TraceEventSearchInventory
	for _, record := range answerDocObservationLedger(ctx).Records {
		if types.IsValidTraceEventSearchInventoryRecord(record) {
			original = record.EventSearchInventory
			break
		}
	}
	if original == nil {
		t.Fatal("public tool did not publish an accepted inventory")
	}
	before, _ := json.Marshal(answerDocObservationLedger(ctx))
	prompt := traceEventInventoryActualFinalizerPrompt(t, ctx)
	views := traceEventInventoryPromptViews(t, prompt)
	if len(views) != 1 || len(views[0].Inventory.Rows) != 4 || views[0].Inventory.Coverage.MatchedTotal != 4 {
		t.Fatalf("semantic handoff changed membership: %+v", views)
	}
	for n, expected := range []string{"CAMERA_PIPELINE", "HeapSize", longName, "E"} {
		row := views[0].Inventory.Rows[n]
		wire, _ := json.Marshal(row)
		var object map[string]json.RawMessage
		if err := json.Unmarshal(wire, &object); err != nil {
			t.Fatal(err)
		}
		semantics := string(object["semantics"])
		if !strings.Contains(semantics, expected) {
			t.Errorf("row %d lost parsed business field %q: %s", n, expected, semantics)
		}
		if n == 0 && (row.EventName != "print" || row.Comm != "<hisysevent>" || row.EventType != "hi_sysevent" || !strings.Contains(semantics, "FRAME_READY")) {
			t.Errorf("business identity replaced physical identity: %+v", row)
		}
		if n == 2 && (!row.RawTruncated || strings.Contains(row.Raw, "-last")) {
			t.Error("long parsed marker was not tested beyond raw preview")
		}
	}
	if !reflect.DeepEqual(views[0].Inventory.Query, original.Query) || views[0].Inventory.Coverage != original.Coverage || !views[0].Inventory.RowsComplete {
		t.Fatalf("parsed fields changed query scope/completeness: query=%+v coverage=%+v rows_complete=%v", views[0].Inventory.Query, views[0].Inventory.Coverage, views[0].Inventory.RowsComplete)
	}
	after, _ := json.Marshal(answerDocObservationLedger(ctx))
	if string(before) != string(after) {
		t.Fatal("finalizer altered accepted semantics")
	}
}
