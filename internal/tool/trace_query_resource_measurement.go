package tool

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/tracewire"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Fixed native tables share an exact executed query, never a model-written
// sequence of frame names. The old rich resource display remains independent.
func traceQueryResourceMeasurementPublications(records []types.ObservationRecord) []types.ObservationRecord {
	for i, r := range records {
		if p, ok := DecodeTraceResourceStack(r); ok {
			records[i] = traceQueryResourceMeasurement(r, p)
		}
	}
	return records
}

func traceQueryResourceMeasurement(r types.ObservationRecord, p tracequery.ResourceStackResult) types.ObservationRecord {
	if p.Status != "available" || !tracequery.ValidResourceStack(p) {
		return r
	}
	right := ")"
	if p.Window.EndInclusive {
		right = "]"
	}
	notes := []string{
		"审计来源：" + p.SourcePath,
		fmt.Sprintf("原查询窗口 [%.9f, %.9f%s 秒；请求主体种类=%s，编号=%d，名称=%q。请求名称不是观测名称；同编号跨来源不证明同一主体。", p.Window.StartTs, p.Window.EndTs, right, p.TargetScope, p.TargetPID, p.TargetThread),
		fmt.Sprintf("原查询匹配资源事件=%d；保留事件=%d；原交接省略事件=%d。未知不是零；无匹配记录不证明目标存在或完整采集没有活动。", p.MatchedEvents, len(p.Events), p.OmittedEvents),
		"源事件将帧绑定到本采集主体；帧深度是原字段，不是函数执行顺序。深度空洞、重复、未知符号和展示省略独立；不可把同深度帧串成调用边。完整源帧不证明完整展开。CPU与执行/等待时长未知，不授泄漏或响应根因。",
	}
	ownerKind := "线程"
	if p.TargetScope == "process" {
		ownerKind = "进程"
	}
	label := fmt.Sprintf("资源事件与栈记录（请求%s %d）", ownerKind, p.TargetPID)
	if p.TargetPID == 0 {
		label = "资源事实 — 请求线程名 " + p.TargetThread
	}
	summary := types.RuntimeMeasurementTable{ObservationID: r.ID, View: types.RuntimeMeasurementSummary, Label: label, DefaultPresentation: true,
		Columns: []string{"源事件", "事件时刻（ns）", "观测主体", "资源操作", "栈记录质量", "深度形态"}, Notes: notes}
	members := types.RuntimeMeasurementTable{ObservationID: r.ID, View: types.RuntimeMeasurementMembers, Label: label + " — 源帧记录（不是有序调用链）",
		Columns: []string{"源事件", "事件时刻（ns）", "观测主体", "源帧", "原深度", "符号", "库", "帧地址信息"}, Notes: append([]string(nil), notes...)}
	for _, e := range p.Events {
		event, ts := strconv.FormatInt(e.RowID, 10), strconv.FormatInt(e.TimestampNS, 10)
		owner := fmt.Sprintf("线程ID=%d；进程ID=%d；线程名=%s", e.Source.TID, e.Source.PID, resourceFactName(e.Source.Thread))
		operation := fmt.Sprintf("%s；%s；地址（signed）=%s", e.Source.Operation, resourceQuantityText(e.Source.Size), resourceFactScalar(e.Source.Address))
		quality := fmt.Sprintf("源帧=%d；缺深度=%d；重复深度=%d；无效深度=%d；未知符号=%d；展示省略帧=%d", e.Source.FrameCount, e.MissingDepths, e.DuplicateDepths, e.InvalidDepths, e.UnknownSymbols, e.OmittedFrames)
		quality += fmt.Sprintf("；源帧载体完整=%t；栈记录=%s；真实展开=%s", e.SourceFramesComplete, resourceFactState(e.Source.StackStatus), resourceFactState(e.UnwindCompleteness))
		summary.Rows = append(summary.Rows, []string{event, ts, owner, operation, quality, resourceDepthState(e.DepthStatus)})
		members.Notes = append(members.Notes, "源事件 "+event+"："+quality+"；"+resourceDepthState(e.DepthStatus)+"。")
		for _, f := range e.Frames {
			frame := fmt.Sprintf("物理行=%d；原ID=%s；证据行=%d", f.RowID, resourceFactScalar(f.SourceID), f.Line)
			address := fmt.Sprintf("IP（signed）=%s；offset=%s；symbol offset=%s；vaddr=%s", resourceFactScalar(f.IP), resourceFactScalar(f.Offset), resourceFactScalar(f.SymbolOffset), resourceFactText(f.VAddr))
			members.Rows = append(members.Rows, []string{event, ts, owner, frame, resourceFactScalar(f.Depth), resourceFactText(f.Symbol), resourceFactText(f.Library), address})
		}
	}
	if p.MatchedEvents == 0 {
		summary.Rows = [][]string{{"无匹配记录", "未知", "未知", "未知", "无观测", "无观测"}}
	}
	if published, ok := types.PublishRuntimeResourceMeasurement(r, []types.RuntimeMeasurementTable{summary, members}, p.Window.EndInclusive); ok {
		return published
	}
	return r
}

func resourceFactScalar(v tracewire.ResourceScalar) string {
	if v.Status == "known" {
		return v.Value
	}
	return "未知（" + resourceFactState(v.Status) + "）"
}

func resourceFactText(v tracewire.ResourceText) string {
	if v.Status == "known" {
		if v.Value == "" {
			return "空文本"
		}
		return v.Value // Rendering escapes once; never encode a new source name here.
	}
	return "未知（" + resourceFactState(v.Status) + "）"
}

func resourceFactState(state string) string {
	switch state {
	case "known":
		return "已知"
	case "null":
		return "空值"
	case "unavailable":
		return "未提供"
	case "invalid":
		return "无效"
	case "ambiguous":
		return "有歧义"
	case "observed":
		return "已记录"
	case "unknown":
		return "未知"
	case "no_frames":
		return "未记录帧"
	case "frame_table_unavailable":
		return "帧表不可用"
	case "unresolved_callchain":
		return "栈标识未解析"
	default:
		return state
	}
}

// DepthStatus and ResourceText.Status are distinct typed fields: an ambiguous
// dictionary lookup does not imply a duplicate or invalid frame depth.
func resourceDepthState(state string) string {
	switch state {
	case "incomplete":
		return "存在缺失或无效深度"
	case "ambiguous":
		return "存在重复深度"
	case "contiguous_observed":
		return "已记录深度连续"
	default:
		return resourceFactState(state)
	}
}

// The wire carries an exact scalar, not a unit. Both public displays share
// this boundary without inferring bytes from the resource operation name.
func resourceQuantityText(size tracewire.ResourceScalar) string {
	return "源数量/大小=" + resourceFactScalar(size) + "（单位未在该载体声明）"
}

func resourceFactName(name string) string {
	if strings.TrimSpace(name) == "" {
		return "未知"
	}
	return name
}
