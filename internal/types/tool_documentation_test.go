package types

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func documentationTestResult(t *testing.T, view string, detail bool) ToolResult {
	t.Helper()
	doc, ok := NormalizeToolDocumentation(ToolDocumentation{Version: ToolDocumentationVersion,
		Schema: "test_catalog/v1", Selection: ToolDocumentationSelection{View: view, Detail: detail},
		Content: json.RawMessage(`{"static_only":true,"evidence":false,"requirements":["complete source identity"],"limitations":["not causal proof"]}`)})
	if !ok {
		t.Fatal("invalid fixture")
	}
	return AttachToolHandoffCarrier(ToolResult{ToolName: "test_catalog", Success: true, Summary: "presentation only",
		Handoff: &ToolHandoffCarrier{Version: ToolHandoffCarrierVersion, ToolName: "test_catalog", Documentation: &doc}})
}

func TestToolDocumentationNormalizeWholeDocument(t *testing.T) {
	base := *documentationTestResult(t, "waits", true).Handoff.Documentation
	for _, tc := range []struct {
		name string
		edit func(*ToolDocumentation)
		want bool
	}{
		{"complete", func(*ToolDocumentation) {}, true},
		{"initial_hash", func(d *ToolDocumentation) { d.ContentHash = "" }, true},
		{"unknown_version", func(d *ToolDocumentation) { d.Version++ }, false},
		{"missing_schema", func(d *ToolDocumentation) { d.Schema = "" }, false},
		{"truncated_json", func(d *ToolDocumentation) { d.Content = d.Content[:20]; d.ContentHash = "" }, false},
		{"scalar_json", func(d *ToolDocumentation) { d.Content = json.RawMessage(`"text"`); d.ContentHash = "" }, false},
		{"hash_mismatch", func(d *ToolDocumentation) { d.ContentHash = "stale" }, false},
		{"too_large", func(d *ToolDocumentation) {
			d.Content = json.RawMessage(`{"limitation":"` + strings.Repeat("x", ToolDocumentationMaxBytes) + `"}`)
			d.ContentHash = ""
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := *cloneToolDocumentation(&base)
			tc.edit(&in)
			got, ok := NormalizeToolDocumentation(in)
			if ok != tc.want {
				t.Fatalf("accepted=%v want=%v", ok, tc.want)
			}
			if ok {
				before := string(got.Content)
				in.Content[0] = 'X'
				if string(got.Content) != before || !json.Valid(got.Content) {
					t.Fatal("normalization aliases input or clips semantics")
				}
			}
		})
	}
}

func TestToolDocumentationTypedOnlyFailureAndAccounting(t *testing.T) {
	result := documentationTestResult(t, "waits", true)
	plain := ToolResult{ToolName: result.ToolName, Success: true, Summary: string(result.Handoff.Documentation.Content)}
	if AttachToolHandoffCarrier(plain).Handoff != nil {
		t.Fatal("summary promoted to documentation")
	}
	for _, mode := range []string{"failed", "wrong_producer", "missing_producer"} {
		t.Run(mode, func(t *testing.T) {
			bad := cloneTraceBusinessSpanToolResult(result)
			if mode == "failed" {
				bad.Success = false
			} else if mode == "missing_producer" {
				bad.ToolName, bad.Handoff.ToolName = "", ""
			} else {
				bad.ToolName = "other"
			}
			if toolResultHasDocumentation(bad) {
				t.Fatal("unqualified result entered completed-fork document publication")
			}
			if got := AttachToolHandoffCarrier(bad); got.Handoff != nil {
				t.Fatal("failed/unrelated result retained documentation")
			}
		})
	}
	withDoc := ToolHandoffCarrierBytes(*result.Handoff)
	noDoc := *result.Handoff
	noDoc.Documentation = nil
	if withDoc-ToolHandoffCarrierBytes(noDoc) < len(result.Handoff.Documentation.Content) {
		t.Fatal("document bypassed handoff byte accounting")
	}
	if pack := WriteContextPackFromPlannerToolResults("batch", "goal", []ToolResult{result}); len(pack.Items) != 0 {
		t.Fatal("static documentation became a write evidence item")
	}
	delta := EvidenceRoundDeltaFromReducerInput(EvidenceReducerInput{Class: EvidenceReducerInputTurnAHandoffSnapshot,
		HandoffCarriers: []ToolHandoffCarrier{*result.Handoff}}, "")
	if len(delta.AcceptedEvidence) != 0 {
		t.Fatal("documentation created accepted evidence")
	}
	if ledger := CompileObservationLedger(ObservationLedgerInput{ToolResults: []ToolResult{result}}); len(ledger.Records) != 0 {
		t.Fatal("documentation entered the observation ledger")
	}
	failedMixed := cloneTraceBusinessSpanToolResult(result)
	failedMixed.Success = false
	failedMixed.Handoff.RepairCode = "real_repair"
	failedMixed.Handoff.ReasonCode = "real_repair_reason"
	if got := AttachToolHandoffCarrier(failedMixed); got.Handoff == nil || got.Handoff.Documentation != nil || got.Handoff.RepairCode != "real_repair" {
		t.Fatal("failure stripping erased an independent typed repair")
	}
}

func TestToolDocumentationMergeSeparatesSelectionsAndCopies(t *testing.T) {
	a, b := documentationTestResult(t, "", false), documentationTestResult(t, "waits", true)
	merged := ToolHandoffCarriersFromTurnAInputs([]ToolResult{a, a, b}, nil, nil)
	if len(merged) != 2 || !ToolHandoffCarrierIsDocumentationOnly(merged[0]) || !ToolHandoffCarrierIsDocumentationOnly(merged[1]) {
		t.Fatal("exact duplicates not merged or detail overwrote summary")
	}
	before := string(merged[0].Documentation.Content)
	a.Handoff.Documentation.Content[0] = 'X'
	if string(merged[0].Documentation.Content) != before {
		t.Fatal("merge aliases producer bytes")
	}
	// Independent typed repairs remain present and distinct beside documentation.
	first, second := merged[1], merged[1]
	first.RepairCode, first.ReasonCode = "first_repair", "first_reason"
	second.RepairCode, second.ReasonCode = "second_repair", "second_reason"
	if got := NormalizeToolHandoffCarriers([]ToolHandoffCarrier{first, second}); len(got) != 2 || ToolHandoffCarrierIsDocumentationOnly(got[0]) {
		t.Fatal("document identity merged away independent repair fields")
	}
}

func TestToolDocumentationMutableLifecycleAndReplay(t *testing.T) {
	result := documentationTestResult(t, "waits", true)
	want := string(result.Handoff.Documentation.Content)
	parent := NewMutableState("documentation handoff")
	parent.AppendDispatchToolResult(result)
	parent.StoreToolResultMemo("test_catalog", "waits", result)
	parent.SetTurnAArtifacts(TurnAArtifacts{ToolResults: []ToolResult{result}})
	result.Handoff.Documentation.Content[0] = 'X'
	assert := func(label string, got ToolResult) {
		t.Helper()
		if got.Handoff == nil || got.Handoff.Documentation == nil || string(got.Handoff.Documentation.Content) != want {
			t.Fatalf("%s lost/aliased documentation", label)
		}
		got.Handoff.Documentation.Content[0] = 'X'
	}
	for i := 0; i < 2; i++ {
		assert("dispatch", parent.DispatchToolResults()[0])
		memo, ok := parent.ToolResultMemo("test_catalog", "waits")
		if !ok {
			t.Fatal("memo missing")
		}
		assert("memo", memo)
		snapshot := parent.TurnAArtifacts()
		assert("snapshot tool", snapshot.ToolResults[0])
		if string(snapshot.HandoffCarriers[0].Documentation.Content) != want {
			t.Fatal("snapshot carrier aliases getter")
		}
		snapshot.HandoffCarriers[0].Documentation.Content[0] = 'X'
	}
	fork := parent.ForkForExploreDispatch()
	assert("fork snapshot", fork.TurnAArtifacts().ToolResults[0])
	next := documentationTestResult(t, "frequency", true)
	fork.AppendDispatchToolResult(next)
	if got := parent.MergeExploreForkPublishedTools(fork); len(got) != 1 || got[0].Handoff.Documentation.Selection.View != "frequency" {
		t.Fatal("completed sibling lost its published documentation")
	}
	if got := parent.MergeExploreForkPublishedTools(fork); len(got) != 0 {
		t.Fatal("repeated fork recovery duplicated document")
	}
	if len(parent.TurnAArtifacts().HandoffCarriers) != 2 {
		t.Fatal("merge lost selected documentation")
	}
	wire, err := json.Marshal(parent.TurnAArtifacts())
	if err != nil {
		t.Fatal(err)
	}
	var decoded TurnAArtifacts
	if err := json.Unmarshal(wire, &decoded); err != nil {
		t.Fatal(err)
	}
	restored := NewMutableState("restored")
	restored.SetTurnAArtifacts(decoded)
	if !reflect.DeepEqual(parent.TurnAArtifacts().HandoffCarriers, restored.TurnAArtifacts().HandoffCarriers) {
		t.Fatal("JSON replay changed document contracts")
	}
	if len(restored.EvidenceClosure().AcceptedEvidenceRefs()) != 0 || len(restored.EvidenceClosure().ReadSet()) != 0 {
		t.Fatal("replay promoted documentation to source evidence")
	}
	// The ordinary completed-worker merge follows the same snapshot channel.
	completed := restored.ForkForExploreDispatch()
	completeSnapshot := completed.TurnAArtifacts()
	completeSnapshot.ToolResults = append(completeSnapshot.ToolResults, documentationTestResult(t, "io", true))
	completed.SetTurnAArtifacts(*completeSnapshot)
	restored.MergeExploreFork(completed)
	if got := restored.TurnAArtifacts(); len(got.HandoffCarriers) != 3 || got.HandoffCarriers[2].Documentation.Selection.View != "io" {
		t.Fatal("ordinary fork merge lost completed documentation")
	}
}

func TestToolDocumentationBudgetRetainsCompleteContractsAndHigherAuthority(t *testing.T) {
	var docs []ToolHandoffCarrier
	for i := 0; i < toolHandoffMaxCarriers+1; i++ {
		docs = append(docs, *documentationTestResult(t, fmt.Sprintf("view_%d", i), true).Handoff)
	}
	if got := NormalizeToolHandoffCarriers(docs); len(got) != toolHandoffMaxCarriers {
		t.Fatal("static documentation bypassed inherited carrier count budget")
	}
	repair := ToolHandoffCarrier{ToolName: "edit", Repair: &ToolRepair{Code: "valid_repair", Hint: "retain exact typed repair"}}
	observation := ToolHandoffCarrier{ToolName: "trace_query", ObservationRefs: []ToolObservationRef{{ID: "actual_observation", Producer: "trace_query", Source: "actual.trace"}}}
	all := append(docs, repair, observation)
	got := NormalizeToolHandoffCarriers(all)
	seenRepair, seenObservation := false, false
	for _, carrier := range got {
		seenRepair = seenRepair || carrier.ToolName == "edit"
		seenObservation = seenObservation || len(carrier.ObservationRefs) != 0
		if carrier.Documentation != nil && !json.Valid(carrier.Documentation.Content) {
			t.Fatal("budget excerpted a semantic document")
		}
	}
	if !seenRepair || !seenObservation {
		t.Fatal("documents evicted a higher-priority repair or real observation")
	}
	result := documentationTestResult(t, "whole", true)
	result.Handoff.Documentation.Content = json.RawMessage(`{"prerequisites":"` + strings.Repeat("x", 45000) + `","limitation":"never causal proof"}`)
	result.Handoff.Documentation.ContentHash = ""
	result = AttachToolHandoffCarrier(result)
	kept, truncation := BoundTurnAToolResultsWithTruncation([]ToolResult{result}, 1, TurnAToolResultBytes(result)-1, nil)
	if len(kept) != 0 || truncation == nil || truncation.Dropped != 1 {
		t.Fatal("tool-result byte budget failed to account for complete document")
	}
	kept, truncation = BoundTurnAToolResultsWithTruncation([]ToolResult{result}, 1, TurnAToolResultBytes(result), nil)
	if len(kept) != 1 || truncation != nil || string(kept[0].Handoff.Documentation.Content) != string(result.Handoff.Documentation.Content) {
		t.Fatal("budget changed complete retained document")
	}
}
