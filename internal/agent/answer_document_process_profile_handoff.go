package agent

import (
	"fmt"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
	"strings"
)

func renderAnswerDocProcessProfiles(ctx *types.AgentContext, ledger types.ObservationLedger) string {
	var b strings.Builder
	seen := map[string]bool{}
	accepted, shown := 0, 0
	for _, r := range ledger.Records {
		p, ok := tool.DecodeTraceProcessProfile(r)
		if !ok {
			continue
		}
		if ctx != nil && ctx.AnalysisIR != nil {
			profile := ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile
			if profile != nil && profile.HasExplicitTimeWindows() && !profile.ContainsExplicitTimeWindow(p.Window.StartTs, p.Window.EndTs) {
				continue
			}
		}
		key := r.SourceRef.QueryScopeID + "\x00" + r.SourceRef.PayloadRef
		if seen[key] {
			continue
		}
		seen[key] = true
		accepted++
		if shown == 4 {
			continue
		}
		if shown == 0 {
			b.WriteString("### 已观测进程概览 / Observed process profiles\n\n")
		}
		fmt.Fprintf(&b, "observation_id=%q；source=%q；query_scope=%q；payload=%q\n", r.ID, r.SourceRef.Path, r.SourceRef.QueryScopeID, r.SourceRef.PayloadRef)
		b.WriteString(tool.TraceProcessProfileText(p, 12))
		shown++
	}
	if shown > 0 {
		fmt.Fprintf(&b, "另省略查询概览=%d；不同来源/窗口不合并，不给概览分配根因排名。\n\n", accepted-shown)
	}
	return b.String()
}
