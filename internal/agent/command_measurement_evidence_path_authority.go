package agent

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// commandMeasurementEvidencePathActive deliberately joins two typed signals:
// the analyzer's soft current-source explanation lane and a producer-authored
// command-measurement carrier. It never inspects request or model prose. The
// result drives prompt guidance only; it is not an answer or evidence gate.
func commandMeasurementEvidencePathActive(ctx *types.AgentContext, results []types.ToolResult) (int, bool) {
	if ctx == nil || ctx.AnalysisIR == nil {
		return 0, false
	}
	if !commandMeasurementEvidencePathRequested(ctx) {
		return 0, false
	}
	count := 0
	for _, result := range results {
		if result.Success && result.CommandMeasurement != nil {
			count++
		}
	}
	return count, count > 0
}

// commandMeasurementEvidencePathRequested keeps the analyzer profile as the
// primary soft carrier, while preserving a precise router obligation when the
// analyzer omits that optional profile. The fallback joins schema fields only:
// required mixed current-source routing, a scalar count obligation, and an
// explain/mechanism answer shape. It never inspects the route rationale,
// request/model prose, aggregate labels, or final answer text, and it activates
// prompt guidance only (not evidence, completion, or answer gates).
func commandMeasurementEvidencePathRequested(ctx *types.AgentContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	if profile := rm.CurrentSourceExplanationProfile; profile != nil && profile.Active() {
		return true
	}
	hint := ctx.TurnRouteHint
	if types.NormalizeTurnRouteCurrentSourceEvidenceMode(string(hint.CurrentSourceEvidenceMode)) != types.TurnRouteCurrentSourceEvidenceRequired ||
		!hint.NeedsRepoAccess || hint.NeedsOperationAccess || hint.ConcreteOperation ||
		strings.TrimSpace(hint.Source) != "mixed" {
		return false
	}
	if !rm.Predicates.IsCountQuestion || !rm.Predicates.IsScalarAnswer || rm.Intent != types.IntentExplain {
		return false
	}
	return rm.Scenario == types.ScenarioArchitectureExplain ||
		types.NormalizeRequirementKind(rm.AnalyzerHints.Kind) == types.ReqMechanism
}

func renderCommandMeasurementEvidencePathAuthority(count int, lang string) string {
	if count <= 0 {
		return ""
	}
	var b strings.Builder
	if lang == "zh" {
		b.WriteString("## 当前源码测量上下文\n\n")
		fmt.Fprintf(&b, "- typed command measurement carrier：%d 个。\n", count)
		b.WriteString("- 确定性命令测量与模型生成的汇总/解释是相互独立的证据载体。兼容的计数可以用测量值做单向核对，但两者并置本身不证明客户仓中的调用边、数据流边或所有权关系。\n")
		b.WriteString("- 回答当前源码机制时，只从本轮在当前仓实际读取并引用的源码推导 producer、consumer、相邻调用和方向；运行分析工具自身的内部测量管线不属于客户仓源码证据，不要把它写成客户仓架构。\n")
		b.WriteString("- 若现有源码证据不足以证明相邻边，请保留边界或继续读取相关文件，不要用测量载体的存在补造调用链。若画图，只有 typed call evidence 支持时才标成 call。\n")
		b.WriteString("- 这是关于证据权限与来源边界的软提示，不替模型选择机制结论，也不修改最终答案。\n\n")
	} else {
		b.WriteString("## Current-source measurement context\n\n")
		fmt.Fprintf(&b, "- Typed command-measurement carriers: %d.\n", count)
		b.WriteString("- Deterministic command measurements and model-authored summaries or explanations are independent evidence carriers. A compatible count may be checked one way against the measurement, but their coexistence does not prove a call edge, data-flow edge, or ownership relation in the customer repository.\n")
		b.WriteString("- For a current-source mechanism answer, derive producers, consumers, adjacent calls, and direction only from source actually read and cited from the current repository. The analysis tool's own measurement plumbing is outside the customer-repository evidence boundary and must not be presented as customer architecture.\n")
		b.WriteString("- If current source evidence does not prove an adjacent edge, preserve that boundary or read the relevant source instead of inventing a call chain from the presence of a measurement carrier. In diagrams, use call only where typed call evidence supports it.\n")
		b.WriteString("- This is soft guidance about evidence authority and provenance. It does not choose the model's mechanism conclusion or rewrite the final answer.\n\n")
	}
	return b.String()
}

func renderAnswerDocCommandMeasurementEvidencePathAuthority(ctx *types.AgentContext) string {
	input := types.ObservationLedgerInputFromAgentContext(ctx, 1)
	count, active := commandMeasurementEvidencePathActive(ctx, input.ToolResults)
	if !active {
		return ""
	}
	return renderCommandMeasurementEvidencePathAuthority(count, extractAnswerDocLang(ctx))
}

func (e *explorerEvaluator) postCommandMeasurementEvidencePathSignal(ctx *types.AgentContext, obs LoopObservation) LoopSignal {
	if e == nil || e.midLoopCommandMeasurementPathSent {
		return LoopSignal{}
	}
	count, active := commandMeasurementEvidencePathActive(ctx, obs.AllToolResults)
	if !active {
		return LoopSignal{}
	}
	e.midLoopCommandMeasurementPathSent = true
	return LoopSignal{
		HintRequested:  true,
		Progress:       true,
		HintKey:        "explorer.mid-loop.command-measurement-evidence-path",
		Hint:           renderCommandMeasurementEvidencePathAuthority(count, "en"),
		BypassThrottle: true,
	}
}
