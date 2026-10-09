package agent

import (
	"encoding/json"
	"fmt"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
	"strings"
)

// Protocol observations have a dedicated, non-causal writing lens. Never add
// these records to the wakeup graph, member certificates or root-cause ranks.
func renderAnswerDocTransactionHandoffs(ctx *types.AgentContext, ledger types.ObservationLedger) string {
	type identity struct{ source, scope, payload string }
	keyOf := func(r types.ObservationRecord) identity {
		return identity{r.SourceRef.Path, r.SourceRef.QueryScopeID, r.SourceRef.PayloadRef}
	}
	contents, conflicts := map[identity]string{}, map[identity]bool{}
	for _, r := range ledger.Records {
		if p, ok := tool.DecodeTraceTransactionHandoffs(r); ok {
			key := keyOf(r)
			body, _ := json.Marshal(p)
			if old, found := contents[key]; found && old != string(body) {
				conflicts[key] = true
			}
			contents[key] = string(body)
		}
	}
	seen := map[identity]bool{}
	var b strings.Builder
	accepted, shown := 0, 0
	for _, r := range ledger.Records {
		p, ok := tool.DecodeTraceTransactionHandoffs(r)
		if !ok {
			continue
		}
		key := keyOf(r)
		if seen[key] || conflicts[key] {
			continue
		}
		seen[key] = true
		if ctx != nil && ctx.AnalysisIR != nil {
			rm := &ctx.AnalysisIR.RequestModel
			profile := rm.RuntimeArtifactScopeProfile
			if profile != nil && profile.HasExplicitTimeWindows() && !resourceStackInsideRequestedWindow(profile, tracequery.ResourceStackWindow{StartTs: p.Window.StartTs, EndTs: p.Window.EndTs, EndInclusive: p.Window.EndInclusive}) {
				continue
			}
			if answerDocHasUserRuntimeTarget(rm) {
				kept := make([]tracequery.TransactionHandoff, 0, len(p.Handoffs))
				for _, h := range p.Handoffs {
					matches := false
					for _, es := range [][]tracequery.TransactionEndpoint{h.Submissions, h.Consumptions} {
						for _, e := range es {
							tidKnown, tgidKnown := e.TID > 0, e.TGID > 0
							row := types.TraceEventSearchInventoryRow{EmitterTID: e.TID, EmitterTGID: e.TGID, EmitterTIDKnown: &tidKnown, EmitterTGIDKnown: &tgidKnown, Comm: e.Thread}
							matches = matches || traceEventInventoryRowMatchesTarget(row, rm)
						}
					}
					if matches {
						kept = append(kept, h)
					}
				}
				if len(kept) == 0 {
					continue
				}
				p.OmittedKeys += len(p.Handoffs) - len(kept)
				p.Handoffs = kept
			}
		}
		accepted++
		if shown == 4 {
			continue
		}
		text := fmt.Sprintf("查询记录=%q；来源=%q；查询标识=%q；完整数据=%q\n", r.ID, r.SourceRef.Path, r.SourceRef.QueryScopeID, r.SourceRef.PayloadRef) + tool.TraceTransactionHandoffsText(p)
		if b.Len()+len(text) > (64<<10)-1024 {
			continue
		}
		if shown == 0 {
			b.WriteString("### 应用提交与渲染服务消费（事务协议观察）\n\n")
		}
		b.WriteString(text)
		shown++
	}
	if accepted > 0 {
		fmt.Fprintf(&b, "另省略查询=%d；不同来源/窗口不合并。保留独立提交到同一消费的分支，不用时间相邻补造关系。\n\n", accepted-shown)
	}
	return b.String()
}
