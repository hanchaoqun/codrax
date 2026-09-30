package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Uses the existing target/window-filtered ledger, before the generic Top-N.
// This does not create a runtime-work relation, accepted focus, or causal seat.
func renderAnswerDocBusinessTreeFacts(ctx *types.AgentContext, ledger types.ObservationLedger) string {
	const limit = 16
	var b strings.Builder
	seen := map[string]bool{}
	accepted, shown := 0, 0
	for _, r := range ledger.Records {
		fact, ok := tool.DecodeTraceBusinessTreeFact(r)
		if !ok {
			continue
		}
		if ctx != nil && ctx.AnalysisIR != nil {
			profile := ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile
			if profile != nil && profile.HasExplicitTimeWindows() && (fact.WindowUnavailableReason != "" || !profile.ContainsExplicitTimeWindow(fact.Window.StartTs, fact.Window.EndTs)) {
				continue
			}
		}
		key, _ := json.Marshal(r)
		if seen[string(key)] {
			continue
		}
		seen[string(key)] = true
		accepted++
		if shown == limit {
			continue
		}
		if shown == 0 {
			b.WriteString("### 已观测同步业务层级 / Observed synchronous work nesting\n\n")
			b.WriteString("- 以下为真实同源同线程同步打点的父子关系，不是唤醒因果链。原生为全线程观察库存按起始行展示，不是目标过滤或耗时排名；按用户所问线程解释，其它线程只作背景。总耗时包含子层，自身耗时扣除完整直接子区间并集，按各条记录的查询范围与已配对区间计量；不要把父子总耗时相加，也不要从展示省略后的子项重算自身。实例身份不能用名称替代；跨线程或邻近业务不能猜父子。\n")
			b.WriteString("- 状态按每个业务总区间/自身离散区间分别求交；未知不是零，睡眠中IO等待为睡眠包含项。业务耗时不是CPU执行时间、可消除量或根因贡献。父子图须区分缺失父/未闭合/展示省略，链上根因仍需独立唤醒与等待证据。Use the requested answer language and reader-facing business labels, not internal IDs/status codes; retain IDs only for unambiguous evidence references.\n")
		}
		fmt.Fprintf(&b, "- %s；%s；该查询观察实例=%d、原生未展示=%d；observation_id=%q；physical_source=%q；query_index=%q；query_scope=%q；payload=%q；support=%q\n",
			tool.TraceBusinessTreeNodeText(fact.Node), tool.TraceBusinessTreeWindowText(fact), fact.NodeCount, fact.OmittedNodes,
			r.ID, fact.Node.SourcePath, r.SourceRef.Path, r.SourceRef.QueryScopeID, r.SourceRef.PayloadRef, strings.Join(r.SupportRefs, "; "))
		shown++
	}
	if shown > 0 {
		fmt.Fprintf(&b, "- 最终上下文独立展示 %d 个实例、另省略 %d 个已发布实例；原生与上下文省略不可混为无打点或完整覆盖。未闭合行仅保存已观测身份，不证明跨窗仍活动。\n\n", shown, accepted-shown)
	}
	return b.String()
}
