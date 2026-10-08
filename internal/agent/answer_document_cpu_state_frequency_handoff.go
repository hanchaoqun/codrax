package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func renderAnswerDocCPUStateFrequency(ctx *types.AgentContext, ledger types.ObservationLedger) string {
	var b strings.Builder
	seen := map[string]bool{}
	contents := map[string]string{}
	conflicts := map[string]bool{}
	for _, r := range ledger.Records {
		if p, ok := tool.DecodeTraceCPUStateFrequency(r); ok {
			key := r.SourceRef.QueryScopeID + "\x00" + r.SourceRef.PayloadRef
			body, _ := json.Marshal(p)
			if prior, exists := contents[key]; exists && prior != string(body) {
				conflicts[key] = true
			}
			contents[key] = string(body)
		}
	}
	accepted, shown := 0, 0
	for _, r := range ledger.Records {
		p, ok := tool.DecodeTraceCPUStateFrequency(r)
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
		if seen[key] || conflicts[key] {
			continue
		}
		seen[key] = true
		accepted++
		if shown == 4 {
			continue
		}
		if shown == 0 {
			b.WriteString("### 已观测CPU状态与频率联合区间\n\n")
		}
		fmt.Fprintf(&b, "observation_id=%q；source=%q；query_scope=%q；payload=%q\n", r.ID, r.SourceRef.Path, r.SourceRef.QueryScopeID, r.SourceRef.PayloadRef)
		b.WriteString(tool.TraceCPUStateFrequencyText(p, 8))
		shown++
	}
	if shown > 0 {
		fmt.Fprintf(&b, "另省略查询=%d；不同来源/窗口不合并，此统计不分配根因排名。\n\n", accepted-shown)
	}
	return b.String()
}
