package agent

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Only real successful producer results supply pressure. Neither summaries,
// native rows, receipts nor production caps are fabricated to cause eviction.
// Repeated independent tool executions are valid history even when their
// source/query population is identical and needs only one factual copy.
func hmc325PublicBusinessQuery(t *testing.T, ctx *types.AgentContext, path string, start, end float64) types.ToolResult {
	t.Helper()
	params, err := json.Marshal(map[string]any{
		"source": "path", "path": path, "view": "window_stats", "pid": 700,
		"time_start": start, "time_end": end,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
	if err != nil || !result.Success {
		t.Fatalf("public query failed: %s: %v %+v", params, err, result)
	}
	ctx.Mutable.AppendDispatchToolResult(result)
	return result
}

func hmc325ResultBytes(t *testing.T, label string, results []types.ToolResult) int {
	t.Helper()
	total := 0
	for i, result := range results {
		rawRows, err := json.Marshal(result.Observations)
		if err != nil {
			t.Fatal(err)
		}
		n := types.TurnAToolResultBytes(result)
		total += n
		t.Logf("%s result=%d total_budget_bytes=%d observations_json_bytes=%d summary_bytes=%d observation_count=%d raw_ref=%q",
			label, i, n, len(rawRows), len(result.Summary), len(result.Observations), result.RawRef)
	}
	t.Logf("%s count=%d total_budget_bytes=%d window_cap=%d", label, len(results), total, turnAToolResultsWindowByteCap)
	return total
}

func hmc325TreePopulation(ledger types.ObservationLedger, source types.ObservationSourceRef) []types.ObservationRecord {
	var rows []types.ObservationRecord
	for _, record := range ledger.Records {
		if record.Predicate == types.TraceBusinessTreePredicate && record.SourceRef.Path == source.Path &&
			record.SourceRef.QueryScopeID == source.QueryScopeID && record.SourceRef.PayloadRef == source.PayloadRef {
			rows = append(rows, record)
		}
	}
	return rows
}

func hmc325ExactAccount(t *testing.T, ledger types.ObservationLedger, before types.ObservationRecord, expected tool.TraceBusinessTreeFact) bool {
	t.Helper()
	for _, record := range hmc325TreePopulation(ledger, before.SourceRef) {
		if record.ID != before.ID {
			continue
		}
		fact, ok := tool.DecodeTraceBusinessTreeFact(record)
		oldFact, _ := json.Marshal(expected)
		newFact, _ := json.Marshal(fact)
		if !ok || string(oldFact) != string(newFact) || record.Origin != before.Origin || record.Producer != before.Producer ||
			record.Role != before.Role || record.GroundingPolicy != before.GroundingPolicy || record.ProvenanceLane != before.ProvenanceLane {
			t.Errorf("native account changed fields or authority across history handoff: before=%s after=%s", oldFact, newFact)
			return false
		}
		rows := hmc325TreePopulation(ledger, record.SourceRef)
		if !ledger.CurrentRuntimeQueryPopulation(record.SourceRef, types.TraceBusinessTreePredicate, rows) {
			t.Errorf("retained account no longer has its original complete private query census: %s", record.ID)
			return false
		}
		return true
	}
	return false
}

// Public producer -> dispatch -> ParseOutput's real window budget -> fork
// merge -> new-dispatch finalizer. The old 16/128 presentation budgets are not
// involved in the first failing assertion: the original complete native
// account has disappeared from the shared ledger input itself.
func TestHMC325NativeBusinessAccountsSurvivePublicWindowHandoff(t *testing.T) {
	for _, pressure := range []bool{false, true} {
		t.Run(fmt.Sprintf("native_query_pressure_%t", pressure), func(t *testing.T) {
			parent, path := hmc323BusinessBudgetContext(t)
			child := parent.ShallowClone()
			child.Mutable = parent.Mutable.ForkForExploreDispatch()
			child.Stage, child.AgentName = types.StageExplore, types.AgentExplorer
			fullResult := hmc325PublicBusinessQuery(t, child, path, 5.2, 5.312)
			clippedResult := hmc325PublicBusinessQuery(t, child, path, 5.201, 5.2069)
			results := []types.ToolResult{fullResult, clippedResult}
			bytes := types.TurnAToolResultBytes(fullResult) + types.TurnAToolResultBytes(clippedResult)
			if bytes >= turnAToolResultsWindowByteCap {
				t.Fatalf("control already exceeds unchanged window budget: bytes=%d", bytes)
			}
			if pressure {
				// Enough newer copies to occupy the real byte budget without
				// reaching the independent count cap. The full old query and
				// recent narrow query remain different factual populations.
				newerBytes := types.TurnAToolResultBytes(clippedResult)
				for newerBytes <= turnAToolResultsWindowByteCap+types.TurnAToolResultBytes(fullResult) && len(results) < turnAToolResultsWindowCountCap {
					r := hmc325PublicBusinessQuery(t, child, path, 5.201, 5.2069)
					results = append(results, r)
					newerBytes += types.TurnAToolResultBytes(r)
				}
				if len(results) >= turnAToolResultsWindowCountCap {
					t.Fatal("fixture hit count cap rather than isolating the real byte budget")
				}
			}
			beforeBytes := hmc325ResultBytes(t, "before_capture", results)
			before := answerDocObservationLedger(child)
			full, fullFact := hmc323FindBusinessAccount(t, before, 5.2, 5.2)
			clipped, clippedFact := hmc323FindBusinessAccount(t, before, 5.201, 5.2)
			if fullFact.Node.ID != clippedFact.Node.ID || full.SourceRef.QueryScopeID == clipped.SourceRef.QueryScopeID ||
				fullFact.Node.Self == nil || clippedFact.Node.Self == nil || math.Abs(fullFact.Node.Self.DurationMs-3.938) > 1e-8 || clippedFact.Node.Self.DurationMs != 0 {
				t.Fatalf("complete/clipped public prerequisites: full=%+v clipped=%+v", fullFact, clippedFact)
			}
			for _, row := range []types.ObservationRecord{full, clipped} {
				t.Logf("before_capture exact_record_key=%s", traceEventPublicationRecordKey(row))
				if !before.CurrentRuntimeQueryPopulation(row.SourceRef, types.TraceBusinessTreePredicate, hmc325TreePopulation(before, row.SourceRef)) {
					t.Fatalf("producer census unavailable before any bound: %s", row.ID)
				}
			}
			beforeCauses, _ := json.Marshal(types.CompileTraceCausalProjectionSet(before))
			requestBefore, _ := json.Marshal(parent.AnalysisIR.RequestModel)
			if child.Mutable.TurnAArtifacts() != nil {
				t.Fatal("fixture must not preload an unbounded prior handoff that hides the capture loss")
			}
			e := &explorerEvaluator{repoRoot: child.RepoRoot, userQuestion: "Explain the observed business accounts within the selected window", mutable: child.Mutable, analysisIR: child.AnalysisIR}
			e.ensureHeuristics()
			out, err := e.ParseOutput(child, nil, results, nil)
			if err != nil || out == nil {
				t.Fatalf("public explorer ParseOutput: %v", err)
			}
			captured := child.Mutable.TurnAArtifacts()
			if captured == nil {
				t.Fatal("public explorer did not create its snapshot")
			}
			afterBytes := hmc325ResultBytes(t, "after_capture", captured.ToolResults)
			t.Logf("window_truncation=%+v before_bytes=%d after_bytes=%d", captured.ToolResultTruncation, beforeBytes, afterBytes)
			if pressure && (!captured.ToolResultTruncation.Active() || afterBytes > turnAToolResultsWindowByteCap || len(captured.ToolResults) >= len(results)) {
				t.Fatalf("unchanged history budget must really drop results without growing: %+v", captured.ToolResultTruncation)
			}
			if !pressure && (captured.ToolResultTruncation.Active() || len(captured.ToolResults) != len(results)) {
				t.Fatal("unpressured control unexpectedly lost tool history")
			}
			child.Mutable.ResetDispatchToolResults()
			parent.Mutable.MergeExploreFork(child.Mutable)
			parent.Mutable.ResetDispatchToolResults()
			input := types.ObservationLedgerInputFromAgentContext(parent, 64)
			hmc325ResultBytes(t, "final_shared_input", input.ToolResults)
			after := answerDocObservationLedger(parent)
			for _, account := range []struct {
				record types.ObservationRecord
				fact   tool.TraceBusinessTreeFact
			}{{full, fullFact}, {clipped, clippedFact}} {
				if !hmc325ExactAccount(t, after, account.record, account.fact) {
					t.Errorf("native account lost before final presentation after real history bound: source=%q query=%q payload=%q physical=%q measuring_window=%+v self_ms=%g",
						account.record.SourceRef.Path, account.record.SourceRef.QueryScopeID, account.record.SourceRef.PayloadRef, account.fact.Node.ID, account.fact.Window, account.fact.Node.Self.DurationMs)
				}
			}
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(parent, nil)
			t.Logf("complete finalizer handoff after public budget/merge:\n%s", prompt)
			for _, account := range []struct {
				record types.ObservationRecord
				fact   tool.TraceBusinessTreeFact
			}{{full, fullFact}, {clipped, clippedFact}} {
				if !strings.Contains(prompt, "observation_id="+quotedTreeValue(account.record.ID)) || !strings.Contains(prompt, tool.TraceBusinessTreeNodeText(account.fact.Node)) {
					t.Errorf("complete atomic account not delivered to finalizer: %s self=%g", account.record.ID, account.fact.Node.Self.DurationMs)
				}
			}
			afterCauses, _ := json.Marshal(types.CompileTraceCausalProjectionSet(after))
			requestAfter, _ := json.Marshal(parent.AnalysisIR.RequestModel)
			if string(beforeCauses) != string(afterCauses) || string(requestBefore) != string(requestAfter) {
				t.Fatal("history handoff changed causal identity or original request scope")
			}
		})
	}
}
