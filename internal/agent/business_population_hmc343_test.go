package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC343BusinessPopulationFinalizerKeepsSeparateQueryCensus(t *testing.T) {
	ctx, path := hmc320BusinessFinalizer(t)
	// This is deliberately a target-free request. The original named-target
	// helper otherwise auto-fills PID=700 into the supposedly broad query.
	ctx.AnalysisIR.RequestModel.RuntimeTargets = nil
	ctx.AnalysisIR.RequestModel.RuntimeTargetProfile = nil
	ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
	hmc320BusinessQuery(t, ctx, path, nil)
	hmc320BusinessQuery(t, ctx, path, map[string]any{"pid": 700})
	ledger := answerDocObservationLedger(ctx)
	before, _ := json.Marshal(ledger)
	causalBefore, _ := json.Marshal(types.CompileTraceCausalProjectionSet(ledger))
	prompt := renderAnswerDocBusinessTreeFacts(ctx, ledger)
	for _, text := range []string{"全线程观察库存=4", "目标线程 TID=700 的观察库存=3", "未指定唯一物理线程，不提供目标线程总数", "不等于窗内已闭合片段数", "最终上下文独立展示 8 条查询发布记录"} {
		if !strings.Contains(prompt, text) {
			t.Errorf("lost applicable census %q", text)
		}
	}
	if strings.Count(prompt, "- business_population ") != 2 || strings.Count(prompt, "population_ref=") != 8 {
		t.Fatal("per-query census was repeated per row, or independent query identities merged")
	}
	for _, r := range ledger.Records {
		if r.Predicate == types.TraceBusinessTreePredicate && r.SourceRef.QueryTargetPID == 700 && r.Subject == "business-700" && !strings.Contains(prompt, quotedTreeValue(r.SourceRef.QueryScopeID)) {
			t.Fatal("census dictionary erased its own query coordinate")
		}
	}
	afterLedger := answerDocObservationLedger(ctx)
	after, _ := json.Marshal(afterLedger)
	causalAfter, _ := json.Marshal(types.CompileTraceCausalProjectionSet(afterLedger))
	if string(before) != string(after) || string(causalBefore) != string(causalAfter) {
		t.Fatal("display census changed facts or causal qualification")
	}
}
