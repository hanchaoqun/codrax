package agent

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func businessTreeFinalizerContext(t *testing.T) *types.AgentContext {
	t.Helper()
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_marker_tree/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	ctx := &types.AgentContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
		Mutable: types.NewMutableState("Explain business nesting and self time"), AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Language: "zh", PerfTrace: &types.PerfBundle{}}}}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "time_start": 5, "time_end": 5.012})
	result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
	if err != nil || !result.Success {
		t.Fatalf("query: %v %+v", err, result)
	}
	ctx.Mutable.AppendDispatchToolResult(result)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
	return ctx
}

func TestBusinessTreePublicFinalizerKeepsInstancesSelfStatesAndNoCausalAuthority(t *testing.T) {
	ctx := businessTreeFinalizerContext(t)
	before, _ := json.Marshal(ctx.AnalysisIR.RequestModel)
	ledger := answerDocObservationLedger(ctx)
	ledgerBefore, _ := json.Marshal(ledger)
	for _, lang := range []string{"zh", "en"} {
		ctx.Language, ctx.AnalysisIR.RequestModel.Language = lang, lang
		prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
		for _, row := range ledger.Records {
			if row.Predicate != types.TraceBusinessTreePredicate {
				continue
			}
			fact, ok := tool.DecodeTraceBusinessTreeFact(row)
			if !ok {
				t.Fatal("lost typed tree fact")
			}
			want := tool.TraceBusinessTreeNodeText(fact.Node)
			if !strings.Contains(prompt, want) || !strings.Contains(prompt, "observation_id="+quotedTreeValue(row.ID)) {
				t.Errorf("%s context lost source-bound full tree account for %s", lang, row.Object)
			}
		}
		for _, boundary := range []string{"不是唤醒因果链", "不要从展示省略后的子项重算自身", "最终上下文独立展示 4 个实例、另省略 0", "状态未能计量，不按零处理"} {
			if !strings.Contains(prompt, boundary) {
				t.Errorf("lost boundary %q", boundary)
			}
		}
	}
	ctx.Language, ctx.AnalysisIR.RequestModel.Language = "zh", "zh"
	after, _ := json.Marshal(ctx.AnalysisIR.RequestModel)
	ledgerAfter, _ := json.Marshal(ledger)
	if string(before) != string(after) || string(ledgerBefore) != string(ledgerAfter) {
		t.Fatal("read-only handoff changed request or ledger")
	}
}

func quotedTreeValue(value string) string { b, _ := json.Marshal(value); return string(b) }

func TestBusinessTreeFinalContextDoesNotBorrowBroadWindow(t *testing.T) {
	ctx := businessTreeFinalizerContext(t)
	start, end := 5.002, 5.007
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "5.002..5.007"}
	if p := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil); strings.Contains(p, "### 已观测同步业务层级") {
		t.Fatal("broad query leaked through narrow user window")
	}
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_marker_tree/events.systrace")
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "time_start": start, "time_end": end})
	r, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
	if err != nil || !r.Success {
		t.Fatalf("narrow query failed: %v %+v", err, r)
	}
	ctx.Mutable.AppendDispatchToolResult(r)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "### 已观测同步业务层级") || !strings.Contains(prompt, "自身耗时{0 毫秒") {
		t.Fatal("exact clipped parent/self account did not reach final context")
	}
	if strings.Contains(prompt, "所选窗口=5.000000000..5.012000000 秒") {
		t.Fatal("narrow answer borrowed broad tree")
	}
}

func TestBusinessTreeHandoffHasIndependentBudgetAndKeepsQueryIdentity(t *testing.T) {
	ctx := businessTreeFinalizerContext(t)
	ledger := answerDocObservationLedger(ctx)
	var original types.ObservationRecord
	for _, r := range ledger.Records {
		if r.Predicate == types.TraceBusinessTreePredicate {
			original = r
			break
		}
	}
	if original.ID == "" {
		t.Fatal("no tree")
	}
	ledger.Records = nil
	for i := 0; i < 20; i++ {
		r := original
		r.ID += strings.Repeat("q", i)
		r.SourceRef.QueryScopeID += strings.Repeat("q", i)
		ledger.Records = append(ledger.Records, r)
	}
	text := renderAnswerDocBusinessTreeFacts(ctx, ledger)
	if strings.Count(text, "observation_id=") != 16 || !strings.Contains(text, "另省略 4 个已发布实例") {
		t.Fatal(text)
	}
	ledger.Records = []types.ObservationRecord{original, original}
	if strings.Count(renderAnswerDocBusinessTreeFacts(ctx, ledger), "observation_id=") != 1 {
		t.Fatal("exact duplicate not removed")
	}
}
