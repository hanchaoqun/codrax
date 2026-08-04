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
	profile := ctx.AnalysisIR.RequestModel.CurrentSourceExplanationProfile
	if profile == nil || !profile.Active() {
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

func renderCommandMeasurementEvidencePathAuthority(count int, lang string) string {
	if count <= 0 {
		return ""
	}
	var b strings.Builder
	if lang == "zh" {
		b.WriteString("## 命令测量证据路径\n\n")
		fmt.Fprintf(&b, "- typed command measurement carrier：%d 个。\n", count)
		b.WriteString("- 当前实现的数据路径是：`execCommandMeasurement` 从原始命令输出生成 `ToolCommandMeasurement` → `ToolResult.CommandMeasurement` 携带 typed 值 → `ObservationLedgerInputFromAgentContext` 汇集已接受工具结果 → `CompileObservationLedger` 的单一编译咽喉调用 `compileToolResultObservations` → `observationRecordForCommandMeasurement` 生成供成文阶段使用的 observation record。\n")
		b.WriteString("- `EmitAnalysis`、`RequiredFiles` 和 `SourceInventoryProfile` 在 exploration 前建立分类与导航约束；它们不承载之后才产生的 command measurement。解释当前代码机制时，请沿上面的真实 carrier/compile 路径，并用实际已读源码证明相邻调用关系。\n")
		b.WriteString("- 若画图，数据载体边请标成 carrier/compile/consume；只有 typed call evidence 支持时才标成 call。不要绘制 `command measurement → EmitAnalysis`。这是精确上下文提示，不替模型下结论。\n\n")
	} else {
		b.WriteString("## Command Measurement Evidence Path\n\n")
		fmt.Fprintf(&b, "- Typed command-measurement carriers: %d.\n", count)
		b.WriteString("- The current implementation path is: `execCommandMeasurement` derives `ToolCommandMeasurement` from raw command output → `ToolResult.CommandMeasurement` carries the typed value → `ObservationLedgerInputFromAgentContext` gathers accepted tool results → the `CompileObservationLedger` single compile throat calls `compileToolResultObservations` → `observationRecordForCommandMeasurement` creates the observation record consumed during answer composition.\n")
		b.WriteString("- `EmitAnalysis`, `RequiredFiles`, and `SourceInventoryProfile` establish classification and navigation constraints before exploration; they do not transport a command measurement produced later. Explain the current-source mechanism along the real carrier/compile path above, and use actually read source to prove adjacent call relations.\n")
		b.WriteString("- In diagrams, label data movement as carrier/compile/consume; use call only where typed call evidence supports it. Do not draw `command measurement → EmitAnalysis`. This is precise context guidance, not a system-authored conclusion.\n\n")
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
