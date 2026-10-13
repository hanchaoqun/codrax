package agent

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestNativeEventPublicationPromptDedupPreservesLedgerHMC236(t *testing.T) {
	results := traceEventInventoryPublicResults(t, 30, "2")
	ctx := traceEventInventoryPublicContext(results)
	before, _ := json.Marshal(answerDocObservationLedger(ctx))
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if index := strings.Index(prompt, types.TraceNoteKeyRuntimeMeasurement+"="); index >= 0 {
		start := max(0, index-1600)
		end := min(len(prompt), index+200)
		t.Fatalf("raw event publication leaked at byte %d / %d:\n%s", index, len(prompt), prompt[start:end])
	}
	after, _ := json.Marshal(answerDocObservationLedger(ctx))
	if string(before) != string(after) {
		t.Fatal("display dedup changed source ledger")
	}
}

func TestNativeEventPublicationNoteDedupRequiresExactPayloadHMC236(t *testing.T) {
	ctx := traceEventInventoryPublicContext(traceEventInventoryPublicResults(t, 30, "2"))
	var original types.ObservationRecord
	for _, record := range answerDocObservationLedger(ctx).Records {
		if types.IsValidTraceEventSearchInventoryRecord(record) {
			original = record
			break
		}
	}
	if original.EventSearchInventory == nil || original.SourceRef.ToolCallID == "" {
		t.Fatal("fixture did not reach a ledger-decorated native inventory")
	}
	original.RichNotes = append(append([]string(nil), original.RichNotes...), "requested_window=6.000000..6.010000", "matching_window=6.000000..6.010500", "user-data containing runtime_measurement= is not a system note")
	// Producer scope notes are part of the strict publication fingerprint.
	// Re-publish this fixture with those notes before ledger decoration.
	original.RichNotes = original.RichNotes[len(original.RichNotes)-3:]
	producer := original
	producer.SourceRef.ToolCallID = ""
	publication, valid := types.BuildTraceEventRuntimeMeasurementPublication(producer)
	if !valid {
		t.Fatal("scope-note fixture could not be published")
	}
	raw, _ := json.Marshal(publication)
	original.RichNotes = append([]string{types.TraceNoteKeyRuntimeMeasurement + "=" + string(raw)}, original.RichNotes...)
	before, _ := json.Marshal(original)
	if got := traceEventInventoryPromptNotes(original); !reflect.DeepEqual(got, original.RichNotes[len(original.RichNotes)-3:]) {
		t.Fatalf("source notes were lost while removing native duplicate: %v", got)
	}
	for name, change := range map[string]func(*types.ObservationRecord){
		"different source":      func(r *types.ObservationRecord) { r.SourceRef.Path += ".other" },
		"different window":      func(r *types.ObservationRecord) { r.SourceRef.QueryWindowEndTs += 1 },
		"different query":       func(r *types.ObservationRecord) { r.EventSearchInventory.Query.Limit++ },
		"different member":      func(r *types.ObservationRecord) { r.EventSearchInventory.Rows[0].JankEvent.Values.AppID++ },
		"different scope note":  func(r *types.ObservationRecord) { r.RichNotes[1] += "changed" },
		"duplicate publication": func(r *types.ObservationRecord) { r.RichNotes = append(r.RichNotes, r.RichNotes[0]) },
		"model origin":          func(r *types.ObservationRecord) { r.Origin = types.AnswerEvidenceOriginSystemInference },
	} {
		t.Run(name, func(t *testing.T) {
			r := original
			r.EventSearchInventory = types.CloneTraceEventSearchInventory(original.EventSearchInventory)
			r.RichNotes = append([]string(nil), original.RichNotes...)
			change(&r)
			if got := traceEventInventoryPromptNotes(r); !reflect.DeepEqual(got, r.RichNotes) {
				t.Fatal("unverified publication or caller data was silently removed")
			}
		})
	}
	after, _ := json.Marshal(original)
	if string(before) != string(after) {
		t.Fatal("display-only note validation modified the original receipt")
	}
}

func TestNativeEventPublicationDedupBeforeScopedMemberProjectionHMC236(t *testing.T) {
	result := traceInventoryScopePublicQuery(t,
		"writer-101 (101) [000] .... 10.000000: tracing_mark_write: I|101|inside\n"+
			"writer-202 (101) [000] .... 10.002000: tracing_mark_write: I|101|other-thread\n"+
			"writer-101 (101) [000] .... 10.005000: tracing_mark_write: I|101|outside\n", 10, 10.01, nil)
	ctx := traceInventoryScopeContext(result, 10, 10.004)
	ledger := answerDocObservationLedger(ctx)
	if _, complete := renderAnswerDocScopedTraceEventInventoryView(ctx, ledger, ledger); len(complete) != 0 {
		t.Fatal("scope-excluded members incorrectly shadowed generic metadata")
	}
	before := traceInventoryScopeAuthority(t, ctx)
	prompt := traceEventInventoryActualFinalizerPrompt(t, ctx)
	views := traceEventInventoryPromptViews(t, prompt)
	if len(views) != 1 || len(views[0].Inventory.Rows) != 1 || views[0].Inventory.Coverage.MatchedTotal != 3 || views[0].Inventory.RowsComplete {
		t.Fatal("display projection changed native count or kept out-of-scope members")
	}
	if strings.Contains(prompt, types.TraceNoteKeyRuntimeMeasurement+"=") || !strings.Contains(prompt, "requested_window=10.000000..10.010000") || !strings.Contains(prompt, "matching_window_policy=lookup_only_boundary_tolerance") {
		t.Fatal("scoped member projection resurrected publication or removed original window notes")
	}
	if after := traceInventoryScopeAuthority(t, ctx); before != after {
		t.Fatal("native inventory or causal authority changed during scoped display")
	}
}

func TestNativeEventPublicationMetadataDedupRequiresCompleteDisplayHMC236(t *testing.T) {
	var source strings.Builder
	for n := 0; n < 35; n++ {
		fmt.Fprintf(&source, "writer-101 (101) [000] .... 10.%03d000: tracing_mark_write: I|101|event-%d\n", n, n)
	}
	result := traceInventoryScopePublicQuery(t, source.String(), 10, 10.1, nil)
	ctx := traceEventInventoryPublicContext([]types.ToolResult{result})
	record := traceInventoryScopeReceipt(t, ctx)
	ledger := types.ObservationLedger{Records: []types.ObservationRecord{record}}
	prompt, complete := renderAnswerDocTraceEventInventoryView(ledger)
	if !strings.Contains(prompt, "prompt_member_rows=32/32") || len(complete) != 0 {
		t.Fatal("member budget omission incorrectly shadowed generic metadata")
	}
	// Reduce this valid producer fixture to one retained native member, then
	// compare complete publication with an over-budget metadata envelope.
	record.RichNotes = traceEventInventoryPromptNotes(record)
	record.EventSearchInventory = types.CloneTraceEventSearchInventory(record.EventSearchInventory)
	record.EventSearchInventory.Rows = record.EventSearchInventory.Rows[:1]
	record.EventSearchInventory.HandoffRowsOmitted = 34
	record.EventSearchInventory.RowsComplete = false
	p, ok := types.BuildTraceEventRuntimeMeasurementPublication(record)
	if !ok {
		t.Fatal("small retained fixture not publishable")
	}
	raw, _ := json.Marshal(p)
	record.RichNotes = append(record.RichNotes, types.TraceNoteKeyRuntimeMeasurement+"="+string(raw))
	ledger.Records = []types.ObservationRecord{record}
	_, complete = renderAnswerDocTraceEventInventoryView(ledger)
	if !complete[record.ID] {
		t.Fatal("full retained metadata did not shadow the exact generic envelope")
	}
	record.RichNotes = append(record.RichNotes, "producer_annotation="+strings.Repeat("x", 256*1024))
	ledger.Records = []types.ObservationRecord{record}
	_, complete = renderAnswerDocTraceEventInventoryView(ledger)
	if len(complete) != 0 {
		t.Fatal("metadata omission incorrectly shadowed generic metadata")
	}
}
