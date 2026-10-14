package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Real native query -> the actual finalizer must not replay an unbound copy of
// the accepted conclusion after every requested native member is published.
// Independent notes and the durable accepted reason are not display duplicates.
func TestEventPublicationHMC240ActualFinalizerKeepsIndependentNotes(t *testing.T) {
	ctx, _ := eventMemberScopeHMC237Context(t)
	const synthesis = "HMC240_STALE_CURRENT_SYNTHESIS"
	const independent = "HMC240_INDEPENDENT_INVESTIGATION_BOUNDARY"
	ta := *ctx.Mutable.TurnAArtifacts()
	ta.InvestigationNotes = []string{synthesis, independent}
	ta.AcceptedClosureReason, ta.AcceptedResultKind = synthesis, "resolved"
	ctx.Mutable.SetTurnAArtifacts(ta)
	ctx.Mutable.SetInvestigationComplete(synthesis)
	ctx.Mutable.SetInvestigationResultKind("resolved")
	// The shared fixture already compiled a plan while collecting fact IDs;
	// a real finalizer dispatch receives a fresh per-dispatch context cache.
	ctx = ctx.ShallowClone()
	before := traceInventoryScopeAuthority(t, ctx)
	prompt := traceEventInventoryActualFinalizerPrompt(t, ctx)
	if strings.Contains(prompt, synthesis) {
		t.Error("complete native publication still replays the exact current synthesis")
		plan := answerSurfacePlan(ctx)
		t.Logf("stable_kind=%s diagnostic=%t source_origin=%t question=%+v", plan.StableInvestigationResultKind, plan.CurrentStatusDiagnosticRequired, plan.CurrentSourceEvidenceOrigin, ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile)
	}
	if !strings.Contains(prompt, independent) {
		t.Error("independent investigation boundary disappeared")
	}
	if ctx.Mutable.StableInvestigationCompleteReason() != synthesis || len(ctx.Mutable.TurnAArtifacts().InvestigationNotes) != 2 {
		t.Fatal("prompt deduplication mutated accepted investigation artifacts")
	}
	if after := traceInventoryScopeAuthority(t, ctx); before != after {
		t.Fatal("publication changed source evidence or causal authority")
	}
}

func eventPublicationHMC240Context(t *testing.T) (*types.AgentContext, types.ObservationRecord) {
	t.Helper()
	result := traceInventoryScopePublicQuery(t,
		"writer-101 (101) [000] .... 1.100000: tracing_mark_write: B|101|jank_event_sync: start_ts=1000000000, end_ts=1100000000, jank_frames=3, appid=10\n"+
			"writer-101 (101) [000] .... 1.900000: tracing_mark_write: B|101|jank_event_sync: start_ts=1800000000, end_ts=1900000000, jank_frames=4, appid=10\n"+
			"writer-101 (101) [000] .... 2.000000: tracing_mark_write: B|101|jank_event_sync: start_ts=1900000000, end_ts=2000000000, jank_frames=5, appid=10\n", 1, 2, nil)
	ctx := traceInventoryScopeContext(result, 1, 2)
	ctx.AnalysisIR.RequestModel.RuntimeTargets = nil
	return ctx, traceInventoryScopeReceipt(t, ctx)
}

func TestEventPublicationHMC240ExactBudgetAndScopeReceipt(t *testing.T) {
	for _, name := range []string{"complete", "half_open_projection", "duplicate_publication", "raw_preview", "producer_preview", "producer_incomplete", "producer_semantic_omission", "row_budget", "query_budget", "query_metadata_budget", "row_metadata_budget", "different_payload", "different_query", "different_clock", "conflicting_same_query"} {
		t.Run(name, func(t *testing.T) {
			ctx, base := eventPublicationHMC240Context(t)
			original, _ := json.Marshal(base)
			r := base
			r.EventSearchInventory = types.CloneTraceEventSearchInventory(base.EventSearchInventory)
			var before []byte
			ledger := types.ObservationLedger{Records: []types.ObservationRecord{r}}
			want := false
			switch name {
			case "complete", "half_open_projection":
				want = true
			case "duplicate_publication":
				copy := r
				copy.ID += ":duplicate-publication"
				ledger.Records = append(ledger.Records, copy)
				want = true
			case "raw_preview":
				r.EventSearchInventory.Rows[0].Raw = strings.Repeat("raw-preview-", 100)
				want = true
			case "producer_preview":
				i := r.EventSearchInventory
				i.Rows = i.Rows[:1]
				i.HandoffRowsOmitted = i.Coverage.Emitted - len(i.Rows)
				i.RowsComplete = false
			case "producer_incomplete":
				r.EventSearchInventory.Coverage.ScopeComplete = false
				r.EventSearchInventory.RowsComplete = false
			case "producer_semantic_omission":
				r.EventSearchInventory.Rows[0].Semantics = &types.TraceEventSemantics{SchemaVersion: types.TraceEventSemanticsVersion, Fields: []types.TraceEventSemanticField{{Key: "marker.name", Type: "text", Status: "omitted", IssueReason: "value_byte_limit", Omitted: &types.TraceEventSemanticOmission{OriginalBytes: 2048, SHA256: strings.Repeat("a", 64)}}}}
			case "row_budget":
				i := r.EventSearchInventory
				row := i.Rows[0]
				i.Rows = nil
				for n := 0; n < 40; n++ {
					row.Line, row.LocalLine = n+1, n+1
					i.Rows = append(i.Rows, row)
				}
				i.Coverage.MatchedTotal, i.Coverage.Emitted = 40, 40
			case "query_budget":
				ledger.Records = nil
				for n := 0; n < traceEventInventoryPromptQueryLimit; n++ {
					ledger.Records = append(ledger.Records, traceEventInventoryDistinctRecord(r, n))
				}
				ledger.Records = append(ledger.Records, r)
			case "query_metadata_budget":
				r.EventSearchInventory.Query.Pattern = strings.Repeat("long-query-", 16000)
			case "row_metadata_budget":
				r.EventSearchInventory.Rows[0].Comm = strings.Repeat("long-name-", 16000)
			case "different_payload":
				ledger.Records[0].SourceRef.PayloadRef += ":other"
			case "different_query":
				ledger.Records[0].SourceRef.QueryScopeID += ":other"
				ledger.Records[0].EventSearchInventory.QueryScopeID = ledger.Records[0].SourceRef.QueryScopeID
			case "different_clock":
				ledger.Records[0].SourceRef.CanonicalTimeDomain += ":other"
			case "conflicting_same_query":
				conflict := r
				conflict.ID += ":conflict"
				conflict.EventSearchInventory = types.CloneTraceEventSearchInventory(r.EventSearchInventory)
				conflict.EventSearchInventory.Rows[0].Comm = "other-thread-name"
				ledger.Records = append(ledger.Records, conflict)
			}
			for _, record := range ledger.Records {
				if !types.IsValidTraceEventSearchInventoryRecord(record) {
					t.Fatal("boundary fixture must reach the valid native inventory renderer")
				}
			}
			before, _ = json.Marshal(ledger)
			var publication answerDocNativePresentationReceipt
			var request *types.RequestModel
			if name != "complete" {
				request = &ctx.AnalysisIR.RequestModel
			}
			prompt, _ := renderAnswerDocTraceEventInventoryPublicationViewWithPublication(ledger, nil, &publication, request)
			if got := publication.completeFor(ctx); got != want {
				for _, observed := range answerDocObservationLedger(ctx).Records {
					t.Logf("predicate=%s producer=%s publication_family=%s", observed.Predicate, observed.Producer, nativePresentationFamily(observed))
				}
				t.Fatalf("completeFor=%t want=%t", got, want)
			}
			if name == "half_open_projection" {
				views := traceEventInventoryPromptViews(t, prompt)
				if len(views) != 1 || len(views[0].Inventory.Rows) != 2 || views[0].Inventory.Coverage.MatchedTotal != 3 {
					t.Fatal("requested projection rewrote the original query count or kept the right boundary")
				}
			}
			if name == "raw_preview" && !strings.Contains(prompt, `"raw_truncated":true`) {
				t.Fatal("raw preview truncation must be explicit beside retained typed fields")
			}
			after, _ := json.Marshal(ledger)
			if string(before) != string(after) {
				t.Fatal("rendering mutated accepted inventory")
			}
			after, _ = json.Marshal(base)
			if string(original) != string(after) {
				t.Fatal("boundary fixtures mutated their original native receipt")
			}
		})
	}
}

func TestEventPublicationHMC240IndependentQueriesRequireOwnPublication(t *testing.T) {
	const source = "writer-101 (101) [000] .... 1.100000: tracing_mark_write: I|101|one\n" +
		"writer-101 (101) [000] .... 1.200000: tracing_mark_write: I|101|two\n"
	results := []types.ToolResult{
		traceInventoryScopePublicQuery(t, source, 1, 2, nil),
		traceInventoryScopePublicQuery(t, source, 1, 2, map[string]any{"pattern": "one"}),
	}
	ctx := traceEventInventoryPublicContext(results)
	ledger := answerDocObservationLedger(ctx)
	var inventories []types.ObservationRecord
	for _, record := range ledger.Records {
		if types.IsValidTraceEventSearchInventoryRecord(record) {
			inventories = append(inventories, record)
		}
	}
	if len(inventories) != 2 {
		t.Fatal("fixture must have independent broad and filtered native queries")
	}
	for _, count := range []int{1, 2} {
		t.Run(fmt.Sprintf("queries_%d", count), func(t *testing.T) {
			var publication answerDocNativePresentationReceipt
			renderAnswerDocTraceEventInventoryPublicationViewWithPublication(types.ObservationLedger{Records: inventories[:count]}, nil, &publication)
			if publication.completeFor(ctx) != (count == 2) {
				t.Fatal("one query borrowed another query's complete publication")
			}
		})
	}
}

func TestEventPublicationHMC240InventoryCannotPublishSeparateArtifactCoverage(t *testing.T) {
	ctx := traceEventInventoryPublicContext(traceEventInventoryPublicResults(t, 20, "2"))
	ledger := answerDocObservationLedger(ctx)
	var publication answerDocNativePresentationReceipt
	renderAnswerDocTraceEventInventoryPublicationViewWithPublication(ledger, nil, &publication)
	// These unwindowed queries carry a separate whole-artifact scope receipt.
	// Displaying an event inventory does not publish that separate boundary.
	if publication.completeFor(ctx) {
		t.Fatal("native rows lent publication coverage to a separate artifact-scope receipt")
	}
}
