package agent

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTransactionHandoffsActualFinalizerPublicBoundary(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_transaction_handoffs/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	start, end := 1.0, 1.05
	ctx := &types.AgentContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
		Mutable: types.NewMutableState("应用提交与消费"), AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Language: "zh", PerfTrace: &types.PerfBundle{},
			RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1到1.05秒"}}}}
	args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "transaction_handoffs", "time_start": start, "time_end": end})
	r, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
	if err != nil || !r.Success {
		t.Fatalf("public query failed: %v %s", err, r.Summary)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{r}})
	before, _ := json.Marshal(answerDocObservationLedger(ctx))
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{"### 应用提交与渲染服务消费", "事务[tid=101, seq=7]", "事务[tid=102, seq=8]", "有歧义，不选首个", "身份关联未确认", "窗外关联背景", "不等于整帧完成、线程等待、GPU执行或响应根因", "不得把同批多事务画成互相调用"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("actual finalizer lost %q", want)
		}
	}
	after, _ := json.Marshal(answerDocObservationLedger(ctx))
	if string(before) != string(after) {
		t.Fatal("display mutated original ledger")
	}
	ledger := answerDocObservationLedger(ctx)
	start, end = 2, 2.05
	if got := renderAnswerDocTransactionHandoffs(ctx, ledger); got != "" {
		t.Fatalf("neighboring window became primary query: %s", got)
	}
	start, end = 1, 1.05
	for _, record := range r.Observations {
		if record.Predicate != tool.TraceTransactionHandoffsPredicate {
			continue
		}
		p, ok := tool.DecodeTraceTransactionHandoffs(record)
		if !ok {
			t.Fatal("native typed result rejected")
		}
		one := types.ObservationLedger{Records: []types.ObservationRecord{record, record}}
		if strings.Count(renderAnswerDocTransactionHandoffs(ctx, one), "查询记录=") != 1 {
			t.Fatal("identical query was repeated")
		}
		p.Caveats = append(append([]string(nil), p.Caveats...), "conflicting payload")
		data, _ := json.Marshal(p)
		one.Records[1].RichNotes = []string{types.TraceNoteKeyTransactionHandoffs + "=" + string(data)}
		if got := renderAnswerDocTransactionHandoffs(ctx, one); got != "" {
			t.Fatal("conflicting payload chosen by input order")
		}
		for _, mode := range []string{"producer", "source", "window", "role", "duplicate JSON note"} {
			forged := record
			switch mode {
			case "producer":
				forged.Producer = "emit_answer"
			case "source":
				forged.SourceRef.Path += ".another"
			case "window":
				forged.SourceRef.QueryWindowStartTs = 2
			case "role":
				forged.Role = types.AnswerAggregateRole("root_cause")
			case "duplicate JSON note":
				forged.RichNotes = append(append([]string(nil), record.RichNotes...), record.RichNotes...)
			}
			if got := renderAnswerDocTransactionHandoffs(ctx, types.ObservationLedger{Records: []types.ObservationRecord{forged}}); got != "" {
				t.Errorf("%s forged publication rendered", mode)
			}
		}
	}
}
