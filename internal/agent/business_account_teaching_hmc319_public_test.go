package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC319BusinessAccountTeachingActualFinalizerAndEmit(t *testing.T) {
	for _, text := range []string{
		"三个已证互斥自身分区为 5、4、1 毫秒，合计 10 毫秒；其运行分区为 5、3.5、1 毫秒，合计 9.5 毫秒。包含子层的总量不重复累计，这些计算不证明响应根因。",
		"同步层级和各自身分段见原生事实；未知状态不能当零，包含关系不证明响应根因。",
	} {
		t.Run(text, func(t *testing.T) {
			ctx := businessTreeFinalizerContext(t)
			rm := &ctx.AnalysisIR.RequestModel
			rm.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactCountOrDuration}}
			ctx.Mutable.SetRequestModel(*rm)
			bus := types.ToolBusContext(ctx, types.AgentFinalizer)
			artifactsBefore, _ := json.Marshal(ctx.Mutable.TurnAArtifacts())
			ledger := answerDocObservationLedger(ctx)
			projectionBefore, _ := json.Marshal(types.CompileTraceCausalProjectionSet(ledger))
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			for _, want := range []string{"同源、同线程、同一查询范围及单位", "互斥自身分段及其同状态交集可合计", "省略或未知不能补零或外推全集", "不要把父子总耗时相加"} {
				if !strings.Contains(prompt, want) {
					t.Errorf("actual finalizer INIT lost scoped arithmetic teaching %q", want)
				}
			}
			for _, r := range ledger.Records {
				if fact, ok := tool.DecodeTraceBusinessTreeFact(r); ok && !strings.Contains(prompt, tool.TraceBusinessTreeNodeText(fact.Node)) {
					t.Errorf("teaching displaced native complete account for %s", fact.Node.Name)
				}
			}
			params, _ := json.Marshal(map[string]any{"blocks": []any{map[string]any{"id": "lead", "kind": "summary", "text": text}}})
			out, err := (&tool.EmitAnswerDocument{}).Execute(bus, params)
			if err != nil || !out.Success {
				t.Fatalf("legal arithmetic or omission of optional arithmetic rejected: %v %+v", err, out)
			}
			visible := render.RenderAnswerDocument(ctx.Mutable.AnswerDocumentV2(), "zh")
			if !strings.Contains(visible, text) || !strings.Contains(visible, "本表为含子层的总区间量测") {
				t.Fatal("actual emit/render lost authored text or scoped native note")
			}
			artifactsAfter, _ := json.Marshal(ctx.Mutable.TurnAArtifacts())
			projectionAfter, _ := json.Marshal(types.CompileTraceCausalProjectionSet(answerDocObservationLedger(ctx)))
			if string(artifactsBefore) != string(artifactsAfter) || string(projectionBefore) != string(projectionAfter) {
				t.Fatal("teaching or display changed native artifacts or causal projection")
			}
		})
	}
}

func TestHMC319BusinessAccountTeachingDoesNotRecastSampling(t *testing.T) {
	ctx, result := samplingBundleContextHMC241(t)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{"total_weight=35", "total_weight=100", "total_weight=7", "采样权重不是毫秒", "Self weight is the last recorded frame, not proof of complete unwinding."} {
		if !strings.Contains(prompt, want) {
			t.Errorf("sampling own unit/interpretation boundary lost %q", want)
		}
	}
	if strings.Contains(prompt, "互斥自身分段及其同状态交集可合计") || strings.Contains(prompt, "本表为含子层的总区间量测") {
		t.Fatal("business elapsed-time guidance was applied to sample weights")
	}
}
