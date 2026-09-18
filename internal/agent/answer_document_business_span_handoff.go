package agent

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// This read-only handoff uses the explicit-window-filtered ledger before the
// generic priority budget. Merely observing a named B/E interval neither
// selects a root cause nor proves that its entire duration was CPU work.
func renderAnswerDocBusinessSpanFacts(ctx *types.AgentContext, ledger types.ObservationLedger) string {
	var requestModel *types.RequestModel
	if ctx != nil && ctx.AnalysisIR != nil {
		requestModel = &ctx.AnalysisIR.RequestModel
	}
	facts := types.TraceBusinessSpanFacts(ledger, requestModel)
	if len(facts) == 0 {
		return ""
	}
	zh := !strings.HasPrefix(strings.ToLower(extractAnswerDocLang(ctx)), "en")
	var b strings.Builder
	if zh {
		b.WriteString("### 已观测业务区间\n\n- 以下名称、线程、时间范围和耗时来自已配对的业务打点，不要求名称属于预定义优化类型。区间耗时不是 CPU 执行时间、可消除量或因果贡献；时间重叠也不证明目标依赖该工作。链上根因仍须使用独立的链与等待凭证。\n")
	} else {
		b.WriteString("### Observed business intervals\n\n- These names, threads, bounds and elapsed measurements come from paired work markers, without requiring a predefined optimization category. Elapsed interval time is not CPU execution, removable time or causal contribution; overlap does not prove the target depends on the work. Root causes still require independent chain and wait evidence.\n")
	}
	for i, record := range facts {
		if i >= types.TraceBusinessSpanFactLimit {
			break
		}
		start, end, _ := types.TraceCausalProjectionSelectedWindowNote(record.RichNotes)
		if zh {
			fmt.Fprintf(&b, "- 业务 %q；线程 %q；所选窗口 %.6f–%.6f 秒；窗口内区间 %.6f–%.6f 秒，耗时 %s 毫秒", record.Object, record.Subject, start, end, record.Span.StartTs, record.Span.EndTs, record.Value)
		} else {
			fmt.Fprintf(&b, "- Work %q; thread %q; selected window %.6f–%.6f s; in-window interval %.6f–%.6f s, elapsed %s ms", record.Object, record.Subject, start, end, record.Span.StartTs, record.Span.EndTs, record.Value)
		}
		actual := traceQueryObservationSupplementNoteValue(record, types.TraceNoteKeyActualWindow)
		actualMS := traceQueryObservationSupplementNoteValue(record, types.TraceNoteKeyActualImpactMS)
		if actualStart, actualEnd, ok := types.TraceCausalProjectionParseWindowValue(actual); ok && actualMS != "" {
			if zh {
				fmt.Fprintf(&b, "；原始配对范围 %.6f–%.6f 秒，完整耗时 %s 毫秒（不是窗口内耗时）", actualStart, actualEnd, actualMS)
			} else {
				fmt.Fprintf(&b, "; physical paired range %.6f–%.6f s, full elapsed %s ms (not the in-window duration)", actualStart, actualEnd, actualMS)
			}
		}
		fmt.Fprintf(&b, "; observation_id=%q; source=%q; artifact=%q; query_scope=%q; support=%q\n", record.ID, record.SourceRef.Path, record.SourceRef.ArtifactID, record.SourceRef.QueryScopeID, strings.Join(record.SupportRefs, "; "))
	}
	if len(facts) > types.TraceBusinessSpanFactLimit {
		if zh {
			fmt.Fprintf(&b, "- 此处只展示已发布的 %d 条业务区间中的前 %d 条，另有 %d 条未展示；完整记录保留在查询结果中。\n", len(facts), types.TraceBusinessSpanFactLimit, len(facts)-types.TraceBusinessSpanFactLimit)
		} else {
			fmt.Fprintf(&b, "- Showing %d of %d published business intervals; %d additional rows are omitted here and remain in query results.\n", types.TraceBusinessSpanFactLimit, len(facts), len(facts)-types.TraceBusinessSpanFactLimit)
		}
	}
	if zh {
		b.WriteString("- 这些是已返回的观测行，不是全 Trace 业务清单或完整覆盖声明。业务区间与嵌套工作、调度状态、IO 请求可能重叠，不能直接相加。保持上述窗口口径，工作与目标的关系由模型依据已发布证据解释；不得将关系未证说成没有业务打点。\n\n")
	} else {
		b.WriteString("- These are returned observations, not an all-trace business inventory or a completeness claim. Work intervals, nested work, scheduler states and IO requests can overlap and must not simply be added. Preserve the stated window basis; the model explains work-to-target relationships from evidence. An unproven relation does not mean the capture lacks business markers.\n\n")
	}
	return b.String()
}
