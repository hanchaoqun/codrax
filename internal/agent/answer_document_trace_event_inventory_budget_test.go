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

func traceEventInventoryBudgetRecord(t *testing.T) types.ObservationRecord {
	t.Helper()
	ledger := types.CompileObservationLedger(types.ObservationLedgerInput{ToolResults: traceEventInventoryPublicResults(t, 20, "2")})
	for _, record := range ledger.Records {
		if types.IsValidTraceEventSearchInventoryRecord(record) {
			return record
		}
	}
	t.Fatal("native inventory missing")
	return types.ObservationRecord{}
}

func traceEventInventoryDistinctRecord(base types.ObservationRecord, index int) types.ObservationRecord {
	out := base
	out.ID = fmt.Sprintf("query-%02d", index)
	out.EventSearchInventory = types.CloneTraceEventSearchInventory(base.EventSearchInventory)
	out.SourceRef.QueryScopeID = out.ID
	out.EventSearchInventory.QueryScopeID = out.ID
	return out
}

func TestTraceEventInventoryQueryRosterKeepsEarliestIdentityAndExactOmissions(t *testing.T) {
	base := traceEventInventoryBudgetRecord(t)
	var ledger types.ObservationLedger
	for n := 0; n < 33; n++ {
		ledger.Records = append(ledger.Records, traceEventInventoryDistinctRecord(base, n))
	}
	// Neither repeated publication nor malformed scope consumes a roster slot.
	ledger.Records = append(ledger.Records, ledger.Records[0])
	invalid := traceEventInventoryDistinctRecord(base, 99)
	invalid.SourceRef.QueryScopeID = "mismatched"
	ledger.Records = append(ledger.Records, invalid)
	prompt := renderAnswerDocTraceEventInventories(ledger)
	views := traceEventInventoryPromptViews(t, prompt)
	if len(views) != 32 || !strings.Contains(prompt, "query_receipts_omitted=1;") || !strings.Contains(prompt, "prompt_member_rows=32/32") {
		t.Fatalf("roster/member budget is not exact: views=%d", len(views))
	}
	for n, view := range views {
		original := ledger.Records[n]
		if view.ObservationID != original.ID || view.Inventory.QueryScopeID != original.SourceRef.QueryScopeID ||
			!reflect.DeepEqual(view.Source, original.SourceRef) || !reflect.DeepEqual(view.Inventory.Query, original.EventSearchInventory.Query) ||
			view.Inventory.Coverage != original.EventSearchInventory.Coverage || view.PromptRowsShown != 1 || view.PromptRowsOmitted != 6 || view.Inventory.RowsComplete {
			t.Fatalf("query %d lost its identity, filter, ruler or exact row omission: %+v", n, view)
		}
	}
}

func TestTraceEventInventoryMembersUseSharedBudgetWithoutEightRowCap(t *testing.T) {
	base := traceEventInventoryBudgetRecord(t)
	for _, counts := range [][]int{{40}, {0, 0, 0}, {28, 1, 0, 1, 1, 1}, {12, 12, 12, 12, 12, 12}} {
		var records []types.ObservationRecord
		for n, count := range counts {
			r := traceEventInventoryDistinctRecord(base, n)
			i := r.EventSearchInventory
			row := i.Rows[0]
			i.Rows = []types.TraceEventSearchInventoryRow{}
			for m := 0; m < count; m++ {
				row.Line = m + 1
				i.Rows = append(i.Rows, row)
			}
			i.Coverage.MatchedTotal, i.Coverage.Emitted = count, count
			i.RowsComplete = true
			records = append(records, r)
		}
		got := traceEventInventoryPromptRowCounts(records)
		var want []int
		switch len(counts) {
		case 1:
			want = []int{32}
		case 3:
			want = []int{0, 0, 0}
		case 6:
			if counts[0] == 28 {
				want = []int{28, 1, 0, 1, 1, 1}
			} else {
				want = []int{6, 6, 5, 5, 5, 5}
			}
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("rows=%v got=%v want=%v", counts, got, want)
		}
		views := traceEventInventoryPromptViews(t, renderAnswerDocTraceEventInventories(types.ObservationLedger{Records: records}))
		if len(views) != len(counts) {
			t.Fatal("zero-row query summary disappeared")
		}
		for n, view := range views {
			if view.PromptRowsShown != want[n] || view.Inventory.Coverage.MatchedTotal != counts[n] ||
				view.Inventory.RowsComplete != (counts[n] == want[n]) {
				t.Fatalf("empty/partial row completeness changed: %+v", view)
			}
		}
	}
}

func TestTraceEventInventoryByteBudgetDeclaresOversizedMetadataWithoutChangingFacts(t *testing.T) {
	base := traceEventInventoryBudgetRecord(t)
	large := strings.Repeat("长范围<&>\"", 500)
	var ledger types.ObservationLedger
	for n := 0; n < 32; n++ {
		r := traceEventInventoryDistinctRecord(base, n)
		// The old producer contract bounds raw/member counts, but not these
		// metadata strings. Stress every free-form source/query string together.
		for _, value := range []reflect.Value{reflect.ValueOf(&r.SourceRef).Elem(), reflect.ValueOf(&r.EventSearchInventory.Query).Elem()} {
			for f := 0; f < value.NumField(); f++ {
				field := value.Field(f)
				name := value.Type().Field(f).Name
				if field.Kind() == reflect.String && name != "Kind" && name != "View" && name != "QueryScopeID" {
					field.SetString(large)
				}
			}
		}
		i := r.EventSearchInventory
		i.Query.Patterns = []string{large, large}
		i.Caveats = []string{large, large}
		for row := range i.Rows {
			i.Rows[row].SourcePath = large
			i.Rows[row].Comm = large
			i.Rows[row].Raw = strings.Repeat("证", 1000)
		}
		if !types.IsValidTraceEventSearchInventoryRecord(r) {
			t.Fatal("oversized fixture did not reach the existing accepted contract")
		}
		ledger.Records = append(ledger.Records, r)
	}
	before, _ := json.Marshal(ledger)
	prompt := renderAnswerDocTraceEventInventories(ledger)
	if len(prompt) > traceEventInventoryPromptByteLimit || strings.Contains(prompt, large) || !strings.Contains(prompt, "prompt_metadata_omission") {
		t.Fatalf("unbounded or undisclosed metadata: bytes=%d", len(prompt))
	}
	views := traceEventInventoryPromptViews(t, prompt)
	if len(views) != 32 {
		t.Fatal("oversized metadata discarded an entire query summary")
	}
	for n, view := range views {
		original := ledger.Records[n]
		if view.Inventory.Coverage != original.EventSearchInventory.Coverage || view.Inventory.Query.TimeStart != original.EventSearchInventory.Query.TimeStart ||
			view.Inventory.Query.TimeEnd != original.EventSearchInventory.Query.TimeEnd || view.PromptRowsShown != 1 || view.PromptRowsOmitted != 6 ||
			view.Inventory.Rows[0].JankEvent.Values.StartTSNS != original.EventSearchInventory.Rows[0].JankEvent.Values.StartTSNS {
			t.Fatal("display budget rewrote a count, query window or exact timestamp")
		}
	}
	after, _ := json.Marshal(ledger)
	if string(before) != string(after) {
		t.Fatal("display budget rewrote accepted source evidence")
	}
}

func TestTraceEventInventoryMetadataOmissionBindsExactOriginalFields(t *testing.T) {
	path := strings.Repeat("来源<&>\"/", 600)
	input := map[string]any{"source": map[string]any{"path": path, "query_scope_id": "query-one"}, "time_start": 0, "matched_total": 0}
	out := traceEventInventoryBoundedPromptObject(input, 512)
	data, _ := json.Marshal(out)
	if len(data) > 512 || out["source"].(map[string]any)["path"] != nil || out["matched_total"] != json.Number("0") || out["time_start"] != json.Number("0") {
		t.Fatalf("path was fabricated or known zero disappeared: %s", data)
	}
	omitted := out["prompt_metadata_omission"].(map[string]any)
	original, _ := json.Marshal(map[string]any{"/source/path": path})
	if !reflect.DeepEqual(omitted["fields"], []string{"/source/path"}) || omitted["sha256"] != fmt.Sprintf("%x", sha256.Sum256(original)) || omitted["original_json_bytes"] != len(original) {
		t.Fatalf("omission is not tied to exact original metadata: %+v", omitted)
	}
	if input["source"].(map[string]any)["path"] != path {
		t.Fatal("projection mutated source")
	}
}

func TestTraceEventInventoryMetadataOmissionRosterIsAlsoBounded(t *testing.T) {
	input := map[string]any{"matched_total": 0}
	originalFields := map[string]any{}
	for n := 0; n < 64; n++ {
		key := fmt.Sprintf("long_free_form_metadata_field_%02d", n)
		input[key] = strings.Repeat("x", 2048)
		originalFields["/"+key] = input[key]
	}
	out := traceEventInventoryBoundedPromptObject(input, 512)
	encoded, _ := json.Marshal(out)
	if len(encoded) > 512 || out["matched_total"] != json.Number("0") {
		t.Fatalf("omission list itself grew beyond its budget: bytes=%d", len(encoded))
	}
	omission := out["prompt_metadata_omission"].(map[string]any)
	original, _ := json.Marshal(originalFields)
	if omission["all_free_form_string_and_array_fields"] != true || omission["field_count"] != 64 ||
		omission["sha256"] != fmt.Sprintf("%x", sha256.Sum256(original)) || omission["original_json_bytes"] != len(original) {
		t.Fatalf("bounded omission lost its exact field-set meaning/digest: %+v", omission)
	}
}
