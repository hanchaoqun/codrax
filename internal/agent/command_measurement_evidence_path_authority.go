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
		b.WriteString("- producer 有两个互斥分支：`internal/tool/builtin.go` 的 history-count 分支只生成 `Origin=VCSMetadata, History=true`；普通确定性整数分支生成 `Origin=CommandMeasurement, History=false`。返回处把同一个 measurement 分别交给 payload renderer 与 `ToolResult.CommandMeasurement` 字段；二者是 sibling consumer，不是 payload renderer 再写 ToolResult。\n")
		b.WriteString("- 当前 ledger 数据路径按源码所有权分为四条相邻边：`internal/tool/builtin.go` 的 `execCommandMeasurement` 生成 `ToolCommandMeasurement` 并赋给 `ToolResult.CommandMeasurement`；`internal/types/observation_ledger_context.go` 的 `ObservationLedgerInputFromAgentContext` 把已接受的 `ToolResult` 收集进 `ObservationLedgerInput.ToolResults`；`internal/types/observation_ledger.go` 的 `CompileObservationLedger` 调用 `compileToolResultObservations`；同文件的 carrier compiler 再调用 `observationRecordForCommandMeasurement`，record 随后进入 compiled `ObservationLedger` 和成文上下文。调用方向必须按这四条边解释，不要反向描述 adapter/record builder。\n")
		b.WriteString("- `AnswerAggregateFact` 是 Explorer 在 completion 中另行提交的 model structured handoff，不是 `observationRecordForCommandMeasurement` 的返回物或下一条转换边。count 请求下，`reconcileCompletionAggregateFactsWithDeterministicCount` 可读取 typed tool measurement 校准一个兼容的 aggregate value；这是 observation/aggregate 两条并行载体间的单向核对，不是 record→aggregate 调用链。\n")
		b.WriteString("- `internal/tool/emit_investigation_complete.go` 是独立的 Explorer 闭环控制路径：它写 stop flag，ParseOutput 据此设置 `HasEnoughFacts`。它不调用 `compileToolResultObservations`，也不生成 command-measurement observation record；不要把这条控制路径接到上面的 ledger 数据路径。\n")
		b.WriteString("- `EmitAnalysis`、`RequiredFiles` 和 `SourceInventoryProfile` 在 exploration 前建立分类与导航约束；它们不承载之后才产生的 command measurement。解释当前代码机制时，请沿上面的真实 carrier/compile 路径，并用实际已读源码证明相邻调用关系。\n")
		b.WriteString("- 若画图，数据载体边请标成 carrier/compile/consume；只有 typed call evidence 支持时才标成 call。不要绘制 `command measurement → EmitAnalysis`。这是精确上下文提示，不替模型下结论。\n\n")
	} else {
		b.WriteString("## Command Measurement Evidence Path\n\n")
		fmt.Fprintf(&b, "- Typed command-measurement carriers: %d.\n", count)
		b.WriteString("- The producer has two mutually exclusive branches: the history-count branch in `internal/tool/builtin.go` creates only `Origin=VCSMetadata, History=true`; the ordinary deterministic-integer branch creates `Origin=CommandMeasurement, History=false`. The return path gives the same measurement to the payload renderer and to `ToolResult.CommandMeasurement` as sibling consumers; the payload renderer does not write the ToolResult field.\n")
		b.WriteString("- The ledger data path has four source-owned adjacent edges: `execCommandMeasurement` in `internal/tool/builtin.go` creates `ToolCommandMeasurement` and assigns `ToolResult.CommandMeasurement`; `ObservationLedgerInputFromAgentContext` in `internal/types/observation_ledger_context.go` gathers accepted `ToolResult` values into `ObservationLedgerInput.ToolResults`; `CompileObservationLedger` in `internal/types/observation_ledger.go` calls `compileToolResultObservations`; the carrier compiler in that same file then calls `observationRecordForCommandMeasurement`, whose record enters the compiled `ObservationLedger` and answer context. Explain adapter/record-builder calls only in these directions.\n")
		b.WriteString("- `AnswerAggregateFact` is a separate model-structured handoff submitted by the Explorer at completion; it is not returned by `observationRecordForCommandMeasurement` and is not the next conversion edge. For count requests, `reconcileCompletionAggregateFactsWithDeterministicCount` may read the typed tool measurement to correct one compatible aggregate value. That is a one-way reconciliation between parallel observation and aggregate carriers, not a record→aggregate call chain.\n")
		b.WriteString("- `internal/tool/emit_investigation_complete.go` is a disjoint Explorer-closure control path: it writes a stop flag and ParseOutput uses that flag to set `HasEnoughFacts`. It does not call `compileToolResultObservations` or create a command-measurement observation record; do not connect that control path to the ledger data path above.\n")
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
