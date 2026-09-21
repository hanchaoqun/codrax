package tracediag

import (
	"fmt"
	"reflect"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// The optional marker-local account stays under its original business span,
// not in the root-rank/key-first lane. Required zero state values are measured
// facts only when the producer reports a constructed complete/partial account.
// Unknown coverage never becomes a zero-duration claim. No attribution or
// measurement is derived here, and source paths retain the privacy policy.
func renderBusinessSpanSchedulerDetail(s tracequery.TraceSpanSchedulerStates, path string, emit func(string), depth int, policy *detailRenderPolicy) {
	emit(fmt.Sprintf("- %s: 业务片段线程状态 来源=%s 线程=%s 区间=%s",
		path, sourcePathDisplayToken(s.SourcePath), formatInlineStruct(reflect.ValueOf(s.Thread)), formatInlineStruct(reflect.ValueOf(s.Window))))
	switch s.Coverage {
	case "complete", "partial":
		coverage := "覆盖完整"
		if s.Coverage == "partial" {
			coverage = "仅部分覆盖，缺测或未分类时间不能按零处理"
		}
		emit(fmt.Sprintf("  %s；运行=%.3f ms 等待调度=%.3f ms 睡眠=%.3f ms 不可中断等待=%.3f ms 调度标记IO等待=%.3f ms 已计量=%.3f ms",
			coverage, s.RunningMs, s.RunnableMs, s.SleepMs, s.DStateMs, s.IOWaitMs, s.AccountedMs))
		emit(fmt.Sprintf("  睡眠内调度标记IO等待=%.3f ms（包含项，不另加）；只描述本片段状态，不证明等待机理或链上根因。", s.SleepIOWaitMs))
	default:
		emit("  本片段线程状态未能计量，不能按零处理，也不能借用更宽窗口的总量。")
	}
	if s.MeasurementDomain != nil {
		renderSchedulerMeasurementDomainDetail(*s.MeasurementDomain, path+".measurement_domain", emit)
	}
	if s.HeadState != nil {
		walkDetailWithPolicy(reflect.ValueOf(s.HeadState), path+".head_state", emit, depth+1, policy)
	}
	if s.IntegrityFailure != "" {
		emit("  integrity_failure=" + clampToken(s.IntegrityFailure))
	}
	walkDetailWithPolicy(reflect.ValueOf(s.Caveats), path+".caveats", emit, depth+1, policy)
}
