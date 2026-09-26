package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func traceResourceContractPublicResults(t *testing.T) []types.ToolResult {
	t.Helper()
	dir := t.TempDir()
	source := strings.Join([]string{
		"worker-23 (23) [002] .... 4.030000: tracing_mark_write: I|23|NativeHook:AllocEvent source_heap_size=9007199254741001 source_callchain_id=9007199254741003 resource_end_ts_ns=9223372036854775807 source_addr_i64=9007199254741001 source_addr_bits_hex=0x0020000000000009 source_sub_type_id=7",
		"worker-23 (23) [002] .... 4.030000: tracing_mark_write: C|23|HeapSize|16384",
		"worker-23 (23) [002] .... 4.040000: tracing_mark_write: I|23|NativeHook:FreeEvent source_heap_size=0 source_callchain_id=-1 resource_end_ts_ns=0 source_addr_i64=0 source_addr_bits_hex=0x0000000000000000 source_sub_type_id=0 source_sub_type_name=\"\"",
		"worker-23 (23) [002] .... 4.040000: tracing_mark_write: C|23|HeapSize|8192",
		"worker-23 (23) [002] .... 4.050000: tracing_mark_write: I|23|NativeHook:MmapEvent source_heap_size=null source_callchain_id=null resource_end_ts_ns=null source_addr_i64=-9 source_addr_bits_hex=0xfffffffffffffff7 source_sub_type_id=8 source_sub_type_name=null",
		"worker-23 (23) [002] .... 4.050000: tracing_mark_write: C|23|MmapSize|2048",
	}, "\n") + "\n"
	path := filepath.Join(dir, "resource.ftrace")
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	var results []types.ToolResult
	for _, pattern := range []string{"", "NativeHook:", "HeapSize", "MmapSize"} {
		params, err := json.Marshal(map[string]any{
			"source": "path", "path": path, "view": "event_search", "event_types": []string{"trace_mark"},
			"time_start": 4.02, "time_end": 4.08, "pattern": pattern,
		})
		if err != nil {
			t.Fatal(err)
		}
		result, err := (&tool.TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
		if err != nil || !result.Success {
			t.Fatalf("resource query failed: %v %s", err, result.Summary)
		}
		results = append(results, result)
	}
	return results
}

func TestTraceResourceContractPublicIdentityInitialContext(t *testing.T) {
	results := traceResourceContractPublicResults(t)
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			seed := traceEventInventoryPublicContext(results)
			seed.AnalysisIR.RequestModel.Language = lang
			seed.AnalysisIR.AnswerContract.Language = lang
			ctx := ctxbuilder.BuildAgentContext(&types.BusContext{
				Language: lang, Mutable: seed.Mutable, AnalysisIR: seed.AnalysisIR,
			}, types.AgentFinalizer, types.StageFinalize)
			before, err := json.Marshal([]any{ctx.AnalysisIR, ctx.Mutable.TurnAArtifacts(), answerDocObservationLedger(ctx)})
			if err != nil {
				t.Fatal(err)
			}
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			for _, want := range []string{
				"source_addr_i64 and source_addr_bits_hex are signed-decimal and hexadecimal views of the same 64-bit pattern",
				"sign, zero, or all-one bits alone do not establish a valid or invalid address, or operation success or failure",
				"source_sub_type_id is an opaque reference within the same capture",
				"JSON-string source_sub_type_name (including an empty string), explicit null, and an unpublished field",
				"do not guess why a name is absent",
			} {
				if !strings.Contains(prompt, want) {
					t.Errorf("initial resource guidance missing %q", want)
				}
			}
			if strings.Count(prompt, skill.TraceResourceObservationContract) != 1 || len(traceEventInventoryPromptViews(t, prompt)) != 4 {
				t.Fatal("shared teaching or independent query receipts changed")
			}
			after, _ := json.Marshal([]any{ctx.AnalysisIR, ctx.Mutable.TurnAArtifacts(), answerDocObservationLedger(ctx)})
			if string(before) != string(after) {
				t.Fatal("initial guidance mutated request, query results, or observation authority")
			}
		})
	}
}

func TestTraceResourceContractPublicQueryReachesFinalizer(t *testing.T) {
	ctx := traceEventInventoryPublicContext(traceResourceContractPublicResults(t))
	ledger := answerDocObservationLedger(ctx)
	before, _ := json.Marshal([]any{ledger, types.CompileTraceCausalProjectionSet(ledger)})
	expected := make(map[string]*types.TraceEventSearchInventory)
	for _, record := range ledger.Records {
		if types.IsValidTraceEventSearchInventoryRecord(record) {
			expected[record.SourceRef.QueryScopeID] = record.EventSearchInventory
		}
	}
	for name, prompt := range map[string]string{
		"ledger":  renderAnswerDocObservationLedger(ctx),
		"initial": (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil),
	} {
		if count := strings.Count(prompt, skill.TraceResourceObservationContract); count != 1 {
			t.Errorf("%s resource contract count=%d, want once", name, count)
		}
		views := traceEventInventoryPromptViews(t, prompt)
		if len(views) != 4 {
			t.Fatalf("%s lost independent resource query scopes: %d", name, len(views))
		}
		wantCounts := map[string]int{"": 6, "NativeHook:": 3, "HeapSize": 2, "MmapSize": 1}
		for _, view := range views {
			inventory := view.Inventory
			original := expected[view.Source.QueryScopeID]
			if original == nil || inventory.QueryScopeID != view.Source.QueryScopeID {
				t.Fatal("resource teaching lost query-bound identity")
			}
			want, known := wantCounts[inventory.Query.Pattern]
			if !known || inventory.Coverage.MatchedTotal != want || len(inventory.Rows) != want || !inventory.RowsComplete {
				t.Fatalf("resource and counter scopes were merged or altered: %+v", inventory)
			}
			delete(wantCounts, inventory.Query.Pattern)
			gotJSON, _ := json.Marshal(inventory)
			wantJSON, _ := json.Marshal(original)
			if string(gotJSON) != string(wantJSON) {
				t.Fatal("teaching changed query filters, exact raw rows, coverage, or source fields")
			}
		}
	}
	after, _ := json.Marshal([]any{answerDocObservationLedger(ctx), types.CompileTraceCausalProjectionSet(answerDocObservationLedger(ctx))})
	if string(before) != string(after) {
		t.Fatal("teaching mutated accepted source observations or causal projection")
	}
}

func TestTraceResourceContractSelectionUsesParsedSourceMarkersOnly(t *testing.T) {
	ledger := types.CompileObservationLedger(types.ObservationLedgerInput{ToolResults: traceResourceContractPublicResults(t)})
	var base types.ObservationRecord
	for _, record := range ledger.Records {
		if types.IsValidTraceEventSearchInventoryRecord(record) && record.EventSearchInventory.Query.Pattern == "MmapSize" {
			base = record
		}
	}
	if base.EventSearchInventory == nil {
		t.Fatal("public resource query did not publish a receipt")
	}
	for _, tc := range []struct {
		name, eventType, raw string
		truncated, want      bool
	}{
		{"resource_instant", "trace_mark", "worker-23 (23) [002] .... 4.030000: tracing_mark_write: I|23|NativeHook:AllocEvent", false, true},
		{"heap_counter", "trace_mark", "worker-23 (23) [002] .... 4.030000: tracing_mark_write: C|23|HeapSize|16384", false, true},
		{"mapping_counter", "trace_mark", "worker-23 (23) [002] .... 4.030000: tracing_mark_write: C|23|MmapSize|2048", false, true},
		{"unrelated_instant", "trace_mark", "worker-23 (23) [002] .... 4.030000: tracing_mark_write: I|23|Render NativeHook:AllocEvent", false, false},
		{"unrelated_counter", "trace_mark", "worker-23 (23) [002] .... 4.030000: tracing_mark_write: C|23|NotHeapSize|16384", false, false},
		{"execution_span_not_resource_instant", "trace_mark", "worker-23 (23) [002] .... 4.030000: tracing_mark_write: B|23|NativeHook:AllocEvent", false, false},
		{"wrong_typed_family", "sched_switch", "worker-23 (23) [002] .... 4.030000: tracing_mark_write: I|23|NativeHook:AllocEvent", false, false},
		{"unparsed_text", "trace_mark", "NativeHook:AllocEvent source_heap_size=9", false, false},
		{"truncated_resource", "trace_mark", "worker-23 (23) [002] .... 4.030000: tracing_mark_write: I|23|NativeHook:AllocEvent", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := base
			r.EventSearchInventory = types.CloneTraceEventSearchInventory(base.EventSearchInventory)
			// Query keywords alone do not activate resource guidance.
			r.EventSearchInventory.Query.Pattern = "NativeHook"
			row := &r.EventSearchInventory.Rows[0]
			// Exercise legacy receipts; the public-query tests separately cover
			// producer semantics, which intentionally take priority over Raw.
			row.Semantics = nil
			row.EventType, row.Raw, row.RawTruncated = tc.eventType, tc.raw, tc.truncated
			if !types.IsValidTraceEventSearchInventoryRecord(r) {
				t.Fatal("test receipt must remain structurally valid")
			}
			prompt := renderAnswerDocTraceEventInventories(types.ObservationLedger{Records: []types.ObservationRecord{r}})
			if got := strings.Contains(prompt, skill.TraceResourceObservationContract); got != tc.want {
				t.Fatalf("resource guidance selected=%t, want %t", got, tc.want)
			}
		})
	}
	if strings.Contains(renderAnswerDocTraceEventInventories(types.ObservationLedger{}), skill.TraceResourceObservationContract) {
		t.Fatal("absent typed receipt received resource guidance")
	}
}

func TestTraceResourceContractProducerSemanticsBeyondRawPreview(t *testing.T) {
	for _, tc := range []struct {
		name, action, prefix string
		want                 bool
	}{
		{"resource", "I", "NativeHook:AllocEvent", true},
		{"unrelated_name", "I", "Render NativeHook:AllocEvent", false},
		{"execution_span", "B", "NativeHook:AllocEvent", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := traceSemanticPublicResult(t, "worker-23 (23) [002] .... 2.030000: tracing_mark_write: "+
				tc.action+"|23|"+tc.prefix+strings.Repeat("detail", 100)+"-last\n")
			ctx := traceEventInventoryPublicContext([]types.ToolResult{result})
			prompt := traceEventInventoryActualFinalizerPrompt(t, ctx)
			views := traceEventInventoryPromptViews(t, prompt)
			if len(views) != 1 || len(views[0].Inventory.Rows) != 1 {
				t.Fatal("public source row missing in actual finalizer")
			}
			row := views[0].Inventory.Rows[0]
			if row.Semantics == nil || !row.RawTruncated || strings.Contains(row.Raw, "-last") {
				t.Fatal("test must exercise retained semantics beyond the raw preview")
			}
			if got := strings.Contains(prompt, skill.TraceResourceObservationContract); got != tc.want {
				t.Fatalf("producer resource guidance selected=%t, want %t", got, tc.want)
			}
		})
	}
}
