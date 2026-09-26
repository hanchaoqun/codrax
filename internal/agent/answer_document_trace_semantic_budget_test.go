package agent

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func semanticBudgetFixture() *types.TraceEventSemantics {
	name, number := strings.Repeat("业务<&>", 70), "9007199254740993"
	return &types.TraceEventSemantics{SchemaVersion: 1, Fields: []types.TraceEventSemanticField{
		{Key: "marker.name", Type: "text", Status: "known", Value: &name},
		{Key: "counter.value", Type: "decimal", Status: "known", Value: &number},
		{Key: "source.tid", Type: "int64", Status: "unavailable", IssueReason: "not_recorded"},
	}}
}

func TestTraceSemanticBudgetPreservesTypedUnknownsAndExactNumbers(t *testing.T) {
	semantics := semanticBudgetFixture()
	before, _ := json.Marshal(semantics)
	out := traceEventInventoryBoundedPromptObject(map[string]any{"line": 0, "semantics": semantics}, 1000)
	wire, _ := json.Marshal(out)
	var got struct {
		Semantics *types.TraceEventSemantics `json:"semantics"`
	}
	if err := json.Unmarshal(wire, &got); err != nil || !types.ValidateTraceEventSemantics(got.Semantics) || got.Semantics == nil {
		t.Fatalf("typed semantics lost: %v / %s", err, wire)
	}
	if len(wire) > 1000 || got.Semantics.Fields[0].Status != "omitted" || !reflect.DeepEqual(got.Semantics.Fields[1:], semantics.Fields[1:]) || out["line"] != json.Number("0") {
		t.Fatalf("budget fabricated/truncated a value or unknown state: %s", wire)
	}
	if got.Semantics.Fields[0].Omitted.SHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(*semantics.Fields[0].Value))) {
		t.Fatal("text omission is not bound to original bytes")
	}
	after, _ := json.Marshal(semantics)
	if string(before) != string(after) {
		t.Fatal("budget mutated accepted fields")
	}
}

func TestTraceSemanticBudgetCompactsRosterBeforeDroppingTypedFields(t *testing.T) {
	semantics := semanticBudgetFixture()
	semantics.Fields = semantics.Fields[1:]
	input := map[string]any{"semantics": semantics, "line": 0}
	for n := 0; n < 64; n++ {
		input[fmt.Sprintf("very_long_metadata_field_%02d", n)] = strings.Repeat("x", 2048)
	}
	out := traceEventInventoryBoundedPromptObject(input, 800)
	wire, _ := json.Marshal(out)
	if len(wire) > 800 || out["semantics"] == nil || !strings.Contains(string(wire), "9007199254740993") || !strings.Contains(string(wire), "not_recorded") {
		t.Fatalf("compact roster needlessly lost typed values: %s", wire)
	}
}

func TestTraceSemanticBudgetWholeOmissionBindsOriginalEnvelope(t *testing.T) {
	semantics := semanticBudgetFixture()
	out := traceEventInventoryBoundedPromptObject(map[string]any{"semantics": semantics, "line": 0}, 350)
	wire, _ := json.Marshal(out)
	original, _ := json.Marshal(map[string]any{"/semantics": semantics})
	omission := out["prompt_metadata_omission"].(map[string]any)
	if len(wire) > 350 || out["semantics"] != nil || omission["semantic_projection_omitted"] != true || omission["field_count"] != 1 || omission["sha256"] != fmt.Sprintf("%x", sha256.Sum256(original)) || omission["original_json_bytes"] != len(original) {
		t.Fatalf("whole omission is not exact/disclosed/bounded: %s", wire)
	}
}

func TestTraceSemanticBudgetThirtyTwoQueriesStayBounded(t *testing.T) {
	result := traceSemanticPublicResult(t, "app-201 (201) [001] .... 2.010000: tracing_mark_write: C|201|HeapSize|0\n")
	ledger := types.CompileObservationLedger(types.ObservationLedgerInput{ToolResults: []types.ToolResult{result}})
	var base types.ObservationRecord
	for _, record := range ledger.Records {
		if types.IsValidTraceEventSearchInventoryRecord(record) {
			base = record
			break
		}
	}
	if base.EventSearchInventory == nil {
		t.Fatal("missing actual query inventory")
	}
	ledger.Records = nil
	for n := 0; n < 32; n++ {
		record := traceEventInventoryDistinctRecord(base, n)
		record.EventSearchInventory.Rows[0].Semantics = semanticBudgetFixture()
		if !types.IsValidTraceEventSearchInventoryRecord(record) {
			t.Fatal("invalid stress fixture")
		}
		ledger.Records = append(ledger.Records, record)
	}
	before, _ := json.Marshal(ledger)
	prompt := renderAnswerDocTraceEventInventories(ledger)
	if len(prompt) > traceEventInventoryPromptByteLimit || len(traceEventInventoryPromptViews(t, prompt)) != 32 || strings.Count(prompt, "9007199254740993") != 32 {
		t.Fatalf("query roster or exact numeric fields lost: bytes=%d", len(prompt))
	}
	after, _ := json.Marshal(ledger)
	if string(before) != string(after) {
		t.Fatal("render mutated accepted ledger")
	}
}
