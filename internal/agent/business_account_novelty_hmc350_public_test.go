package agent

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Two legitimate queries can publish the same source body while keeping
// independent query receipts. Give both owners their first complete account,
// then spend the unchanged 16 record slots on additional measured information.
func TestHMC350PublicBusinessBudgetKeepsQueriesAndAddsDistinctAccounts(t *testing.T) {
	ctx, path := hmc323BusinessBudgetContext(t)
	for _, limit := range []int{40, 41} {
		hmc320BusinessQuery(t, ctx, path, map[string]any{"pid": 700, "time_start": 5.0, "time_end": 5.111487, "limit": limit})
	}
	ledger := answerDocObservationLedger(ctx)
	before, _ := json.Marshal(ledger)
	// The per-stage dispatch outbox is reset by BaseAgent.Execute. Original
	// exploration results live in the immutable Turn A handoff, not that outbox.
	resultsBefore, _ := json.Marshal(ctx.Mutable.TurnAArtifacts().ToolResults)
	causalBefore, _ := json.Marshal(types.CompileTraceCausalProjectionSet(ledger))
	accounts := hmc325FullAccounts(ledger)
	queries, payloads := map[string]bool{}, map[string]bool{}
	for _, a := range accounts {
		queries[a.record.SourceRef.QueryScopeID] = true
		payloads[a.record.SourceRef.PayloadRef] = true
	}
	if len(queries) != 2 || len(payloads) != 1 || len(accounts) <= answerDocBusinessAccountLimit {
		t.Fatalf("public fixture must retain independent same-body queries: queries=%d payloads=%d accounts=%d", len(queries), len(payloads), len(accounts))
	}
	plan := planAnswerDocBusinessTreeFacts(ctx, ledger)
	if len(plan.accounts) != 16 || plan.groups != 2 || plan.shownGroups != 2 || len(plan.published) != 16 {
		t.Fatal("16 complete record budget or independent query fairness changed")
	}
	physical, owners := map[string]bool{}, map[string]bool{}
	for _, a := range plan.accounts {
		physical[a.fact.Node.ID] = true
		if a.fact.Node.ActualStartTs == 5.0 && a.fact.Node.ActualEndTs != nil && *a.fact.Node.ActualEndTs == 5.111487 {
			owners[a.record.SourceRef.QueryScopeID] = true
		}
	}
	if len(physical) < 15 {
		t.Fatalf("duplicate query accounts consume later slots: only %d distinct measured accounts in 16 slots", len(physical))
	}
	if len(owners) != 2 {
		t.Fatal("another query borrowed the exact owner instead of retaining its own complete account")
	}
	prompt := renderAnswerDocBusinessAccountPlan(plan)
	_, actual := finalizerReaderFieldMessages(t, ctx)
	for _, a := range plan.accounts {
		for _, text := range []string{tool.TraceBusinessTreeNodeText(a.fact.Node), "observation_id=" + quotedTreeValue(a.record.ID), "query_scope=" + quotedTreeValue(a.record.SourceRef.QueryScopeID)} {
			if !strings.Contains(prompt, text) || !strings.Contains(actual, text) {
				t.Fatal("complete per-query account or actual finalizer navigation was lost")
			}
		}
	}
	// Reorder the actual ledger, retaining the same independently validated
	// source receipts, rather than manufacturing private keys in the test.
	for i, j := 0, len(ledger.Records)-1; i < j; i, j = i+1, j-1 {
		ledger.Records[i], ledger.Records[j] = ledger.Records[j], ledger.Records[i]
	}
	if reversed := renderAnswerDocBusinessAccountPlan(planAnswerDocBusinessTreeFacts(ctx, ledger)); reversed != prompt {
		t.Fatal("display preference depends on query arrival order")
	}
	afterLedger := answerDocObservationLedger(ctx)
	after, _ := json.Marshal(afterLedger)
	resultsAfter, _ := json.Marshal(ctx.Mutable.TurnAArtifacts().ToolResults)
	causalAfter, _ := json.Marshal(types.CompileTraceCausalProjectionSet(afterLedger))
	if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(resultsBefore, resultsAfter) || !reflect.DeepEqual(causalBefore, causalAfter) {
		t.Fatal("soft display preference changed original facts or causal projection")
	}
}

func TestHMC350BusinessNoveltySourceCannotBeRestoredOrBorrowedByID(t *testing.T) {
	ctx, path := hmc323BusinessBudgetContext(t)
	hmc320BusinessQuery(t, ctx, path, map[string]any{"pid": 700, "time_start": 5.0, "time_end": 5.111487})
	original := ctx.Mutable.TurnAArtifacts().ToolResults
	if len(answerDocBusinessAccountSources(ctx)) == 0 {
		t.Fatal("native source witness prerequisite absent")
	}
	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var restored []types.ToolResult
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	ctx.Mutable.ResetDispatchToolResults()
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: restored})
	if len(answerDocBusinessAccountSources(ctx)) != 0 {
		t.Fatal("readable JSON restored a private physical-generation witness")
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: append(original, restored...)})
	if len(answerDocBusinessAccountSources(ctx)) != 0 {
		t.Fatal("unqualified repeated IDs borrowed an original sibling's private source")
	}
}
