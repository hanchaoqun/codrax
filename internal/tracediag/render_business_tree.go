package tracediag

import (
	"fmt"
	"reflect"
	"strconv"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// This independent business-work face stays after older detail families. The
// exact DTO renderer preserves zero-valued measurements and nanosecond-sized
// intervals; it never promotes nesting to causal/root authority.
func renderBusinessTreeDetail(value any, path string, emit func(string), depth int, policy *detailRenderPolicy) {
	switch item := value.(type) {
	case tracequery.TraceMarkerTreeStats:
		emit(fmt.Sprintf("- %s: 业务层级 node_count=%d omitted_nodes=%d；仅描述已观察到的同步嵌套，不是唤醒根因树。", path, item.NodeCount, item.OmittedNodes))
		coverage := "层级覆盖情况未知"
		if item.Coverage == "observed_stream" {
			coverage = "已观察到的片段层级，不保证完整采集或完整调用图"
		} else if item.Coverage == "partial_topology" {
			coverage = "层级仅部分覆盖，缺失祖先或起止的片段不可当作完整调用"
		}
		emit("  " + coverage)
		if item.WindowUnavailableReason != "" {
			window := "连续时间窗未确定"
			if item.WindowUnavailableReason == "line_selected_time_window_undetermined" {
				window = "仅按行选择，连续时间窗未确定"
			}
			emit("  " + path + ".window: " + window + "，不能把占位端点当作真实时间；节点仍保留各自的物理区间。")
		} else {
			emit(fmt.Sprintf("  %s.window: start_ts=%s end_ts=%s", path, businessTreeNumber(item.Window.StartTs), businessTreeNumber(item.Window.EndTs)))
		}
		walkDetailWithPolicy(reflect.ValueOf(item.Caveats), path+".caveats", emit, depth+1, policy)
		walkDetailWithPolicy(reflect.ValueOf(item.Nodes), path+".nodes", emit, depth+1, policy)
	case tracequery.TraceMarkerTreeNode:
		parent := "前序层级未知"
		switch item.ParentStatus {
		case "observed_root":
			parent = "已观察到的最外层，不代表程序入口"
		case "observed_parent":
			parent = "已观察到父片段 parent_id=" + clampToken(item.ParentID)
		}
		closure := "尚未闭合，不能计量完整耗时"
		if item.Closure == "closed" {
			closure = "起止已闭合"
		} else if item.Closure == "invalidated" {
			closure = "配对已失效，不能计量完整耗时"
		}
		emit(fmt.Sprintf("- %s: %s id=%s 来源=%s 线程=%s start_line=%d direct_child_count=%d；%s；%s",
			path, clampToken(item.Name), clampToken(item.ID), sourcePathDisplayToken(item.SourcePath), formatInlineStruct(reflect.ValueOf(item.Thread)), item.StartLine, item.DirectChildCount, parent, closure))
		end := "未知"
		if item.ActualEndTs != nil {
			end = businessTreeNumber(*item.ActualEndTs)
		}
		emit(fmt.Sprintf("  actual_start_ts=%s actual_end_ts=%s end_line=%d", businessTreeNumber(item.ActualStartTs), end, item.EndLine))
		walkDetailWithPolicy(reflect.ValueOf(item.Inclusive), path+".inclusive", emit, depth+1, policy)
		walkDetailWithPolicy(reflect.ValueOf(item.Self), path+".self", emit, depth+1, policy)
	case tracequery.TraceMarkerTreeAccount:
		emit(fmt.Sprintf("- %s: duration_ms=%s omitted_segments=%d；查询范围内的墙上时间，自身耗时可能由多个不连续区间组成。", path, businessTreeNumber(item.DurationMs), item.OmittedSegments))
		for i, segment := range item.Segments {
			emit(fmt.Sprintf("  %s.segments[%d]: start_ts=%s end_ts=%s", path, i, businessTreeNumber(segment.StartTs), businessTreeNumber(segment.EndTs)))
		}
		walkDetailWithPolicy(reflect.ValueOf(item.States), path+".states", emit, depth+1, policy)
	case tracequery.TraceMarkerTreeStates:
		coverage := "线程状态未能计量，不能按零处理"
		measured := item.Values != nil && (item.Coverage == "complete" || item.Coverage == "partial")
		if measured {
			coverage = "线程状态覆盖完整"
			if item.Coverage == "partial" {
				coverage = "线程状态仅部分覆盖，缺测或未分类时间不能按零处理"
			}
		}
		emit(fmt.Sprintf("- %s: %s unknown_ms=%s", path, coverage, businessTreeNumber(item.UnknownMs)))
		if measured {
			walkDetailWithPolicy(reflect.ValueOf(item.Values), path+".values", emit, depth+1, policy)
		}
		walkDetailWithPolicy(reflect.ValueOf(item.Reasons), path+".reasons", emit, depth+1, policy)
	case tracequery.TraceMarkerTreeStateValues:
		emit(fmt.Sprintf("- %s: running_ms=%s runnable_ms=%s sleep_ms=%s d_state_ms=%s io_wait_ms=%s stopped_ms=%s dead_ms=%s accounted_ms=%s sleep_io_wait_ms=%s（睡眠内包含项，不另加）",
			path, businessTreeNumber(item.RunningMs), businessTreeNumber(item.RunnableMs), businessTreeNumber(item.SleepMs), businessTreeNumber(item.DStateMs), businessTreeNumber(item.IOWaitMs), businessTreeNumber(item.StoppedMs), businessTreeNumber(item.DeadMs), businessTreeNumber(item.AccountedMs), businessTreeNumber(item.SleepIOWaitMs)))
	}
}

func businessTreeNumber(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}
