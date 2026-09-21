package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracequery"
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
		b.WriteString(answerDocBusinessSpanSchedulerMeaning(record, zh))
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

func answerDocBusinessSpanSchedulerMeaning(record types.ObservationRecord, zh bool) string {
	if traceQueryObservationSupplementNoteValue(record, types.TraceNoteKeySpanKind) != "sync" {
		return ""
	}
	var states tracequery.TraceSpanSchedulerStates
	if err := json.Unmarshal([]byte(traceQueryObservationSupplementNoteValue(record, types.TraceNoteKeyBusinessSpanSchedulerStates)), &states); err != nil ||
		!states.Matches(record.SourceRef.Path, record.Subject, record.Span.StartTs, record.Span.EndTs) {
		return ""
	}
	if states.Coverage == "unavailable" {
		if zh {
			return "；本业务区间的线程状态未能计量，不能按零处理，也不能借用更宽查询窗的状态总量"
		}
		return "; marker-local scheduler states unavailable, not zero; do not substitute a wider query's state totals"
	}
	if zh {
		coverage := "覆盖完整"
		if states.Coverage == "partial" {
			coverage = "仅部分覆盖，未观测或未分类的时间不能按零处理"
		}
		return fmt.Sprintf("；本业务区间的线程状态：运行 %.3f 毫秒、等待调度 %.3f 毫秒、睡眠 %.3f 毫秒、%s %.3f 毫秒、调度标记 IO 等待 %.3f 毫秒；已计量 %.3f 毫秒（%s；睡眠中调度标记 IO 等待 %.3f 毫秒为包含项，不另加；仅说明本区间状态，不证明等待原因，不能替代为更宽查询窗的总量）",
			states.RunningMs, states.RunnableMs, states.SleepMs, tool.TraceStateNonIODStateWord(true), states.DStateMs, states.IOWaitMs, states.AccountedMs, coverage, states.SleepIOWaitMs)
	}
	coverage := "complete coverage"
	if states.Coverage == "partial" {
		coverage = "partial coverage; unobserved or unclassified time is not zero"
	}
	return fmt.Sprintf("; marker-local scheduler states: running %.3f ms, runnable %.3f ms, sleep %.3f ms, %s %.3f ms, scheduler-marked IO wait %.3f ms; accounted %.3f ms (%s; scheduler-marked IO within sleep %.3f ms is an included overlay, not an addend; states do not prove a wait mechanism and must not be replaced by wider-query totals)",
		states.RunningMs, states.RunnableMs, states.SleepMs, tool.TraceStateNonIODStateWord(false), states.DStateMs, states.IOWaitMs, states.AccountedMs, coverage, states.SleepIOWaitMs)
}
