package agent

// NW-05 软臂 (P3, 2026-07-24) — the composition prompt gains ONE causal-ceiling
// directive when a typed evidence authority published causal_conclusion=
// unproven. Precise trigger (typed authority field), soft effect (prompt hint
// only); absence of the authority keeps the prompt byte-identical.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func answerDocCausalCeilingTestContext(withUnproven bool) *types.AgentContext {
	mut := types.NewMutableState("分析鸿蒙 trace 丢帧")
	mut.SetPerfTrace(&types.PerfBundle{
		Meta: types.PerfMeta{Source: "hitrace"},
		Observations: []types.PerfObservation{{
			Kind:    "priority_semantics",
			Subject: "HarmonyOS priority semantics",
			Summary: "Harmony priority semantics: prio=120/ohos_rt observed in attached trace",
			Tags:    []string{"harmony_priority", "prio=120/ohos_rt"},
		}},
	})
	if withUnproven {
		mut.AppendDispatchToolResult(types.ToolResult{
			ToolName: "trace_query",
			Success:  true,
			TraceEvidenceAuthority: &types.TraceEvidenceAuthority{
				View:                "frame_root_cause_bundle",
				FrameEvidenceStatus: "absent",
				CausalConclusion:    "unproven",
			},
		})
	}
	return &types.AgentContext{
		Objective:             "分析鸿蒙 trace 丢帧",
		AttachedHitraceSource: "harmony_hitrace",
		Mutable:               mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario: types.ScenarioRootCause,
				Intent:   types.IntentRootCause,
			},
			AnswerContract: types.AnswerContract{},
		},
	}
}

func TestAnswerDocumentEvaluatorRendersCausalCeilingHintOnUnprovenAuthority(t *testing.T) {
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(answerDocCausalCeilingTestContext(true), nil)
	for _, want := range []string{
		"Runtime causal ceiling hint",
		"`causal_conclusion=unproven`",
		"bounded window facts and candidates",
		"`导致丢帧`/`caused the dropped frame`",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluatorOmitsCausalCeilingHintWithoutUnprovenAuthority(t *testing.T) {
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(answerDocCausalCeilingTestContext(false), nil)
	if strings.Contains(prompt, "Runtime causal ceiling hint") {
		t.Fatalf("hint must stay absent without the typed unproven authority:\n%s", prompt)
	}
}
