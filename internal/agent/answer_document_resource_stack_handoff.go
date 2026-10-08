package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// This is a factual display of source-bound resource stacks, not a source-code
// call graph, an execution trace, or a new path to causal-root admission.
func renderAnswerDocResourceStacks(ctx *types.AgentContext, ledger types.ObservationLedger) string {
	const displayBytes = 64 << 10
	const footerReserve = 1024
	type identity struct{ source, scope, payload string }
	keyOf := func(r types.ObservationRecord) identity {
		return identity{r.SourceRef.Path, r.SourceRef.QueryScopeID, r.SourceRef.PayloadRef}
	}
	contents := map[identity]string{}
	conflicts := map[identity]bool{}
	for _, r := range ledger.Records {
		if p, ok := tool.DecodeTraceResourceStack(r); ok {
			key := keyOf(r)
			body, _ := json.Marshal(p)
			if prior, exists := contents[key]; exists && prior != string(body) {
				conflicts[key] = true
			}
			contents[key] = string(body)
		}
	}
	var b strings.Builder
	seen := map[identity]bool{}
	accepted, shown := 0, 0
	for _, r := range ledger.Records {
		p, ok := tool.DecodeTraceResourceStack(r)
		if !ok {
			continue
		}
		if ctx != nil && ctx.AnalysisIR != nil {
			profile := ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile
			if profile != nil && profile.HasExplicitTimeWindows() && !resourceStackInsideRequestedWindow(profile, p.Window) {
				continue
			}
		}
		key := keyOf(r)
		if seen[key] || conflicts[key] {
			continue
		}
		seen[key] = true
		accepted++
		if shown == 4 {
			continue
		}
		var query strings.Builder
		if shown == 0 {
			query.WriteString("### 已观测资源事件与调用栈 / Observed resource event stacks\n\n")
		}
		fmt.Fprintf(&query, "observation_id=%q；source=%q；query_scope=%q；payload=%q\n", r.ID, r.SourceRef.Path, r.SourceRef.QueryScopeID, r.SourceRef.PayloadRef)
		query.WriteString(tool.TraceResourceStackText(p, 12))
		// Keep complete source-bound query displays. A byte budget must never
		// turn a truncated symbol into an apparently complete function name.
		if b.Len()+query.Len() > displayBytes-footerReserve {
			continue
		}
		b.WriteString(query.String())
		shown++
	}
	if accepted > 0 {
		fmt.Fprintf(&b, "资源栈展示受查询数与64KiB预算约束，另省略查询=%d；完整数据仍在原始查询载荷。不同来源/窗口不合并。栈帧只支持资源事件的调用位置线索，不证明函数执行时长、等待或响应根因；不固定某个深度为业务叶子。\n\n", accepted-shown)
	}
	return b.String()
}

func resourceStackInsideRequestedWindow(profile *types.RuntimeArtifactScopeProfile, w tracequery.ResourceStackWindow) bool {
	// Resource envelopes can include the last observed point, including a
	// single-point capture. Do not extend a user's right-open time window by
	// applying the continuous-duration comparison tolerance to that point.
	if !w.EndInclusive {
		return profile.ContainsExplicitTimeWindow(w.StartTs, w.EndTs)
	}
	for _, requested := range profile.ExplicitTimeWindows() {
		if w.StartTs >= *requested.TimeStart && w.EndTs < *requested.TimeEnd {
			return true
		}
	}
	return false
}
