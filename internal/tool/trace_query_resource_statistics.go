package tool

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

const resourceStatisticsTableByteBudget = 8 << 10

// Aggregate facts come from the complete query scan, not the retained event
// preview. Only whole source groups may be omitted from this display. The full
// query payload and the existing two resource tables remain unchanged.
func publishResourceEventStatistics(r types.ObservationRecord, p tracequery.ResourceStackResult, base []types.RuntimeMeasurementTable) types.ObservationRecord {
	legacy, legacyOK := types.PublishRuntimeResourceMeasurement(r, base, p.Window.EndInclusive)
	if p.EventStatistics == nil {
		if legacyOK {
			return legacy
		}
		return r
	}
	s := p.EventStatistics
	retained := len(s.Groups)
	omittedEvents := 0
	for {
		table := resourceEventStatisticsTable(r, p, retained, omittedEvents)
		raw, err := json.Marshal(table)
		if err == nil && len(raw) <= resourceStatisticsTableByteBudget {
			tables := append(append([]types.RuntimeMeasurementTable(nil), base...), table)
			if published, ok := types.PublishRuntimeResourceMeasurement(r, tables, p.Window.EndInclusive); ok {
				return published
			}
		}
		if retained == 0 {
			break
		}
		retained--
		omittedEvents += s.Groups[retained].EventCount
	}
	if legacyOK {
		return types.WithRuntimeResourceStatisticsDisplayOmission(legacy)
	}
	return r
}

func resourceEventStatisticsTable(r types.ObservationRecord, p tracequery.ResourceStackResult, retained, displayOmittedEvents int) types.RuntimeMeasurementTable {
	s := p.EventStatistics
	right := ")"
	if p.Window.EndInclusive {
		right = "]"
	}
	t := types.RuntimeMeasurementTable{ObservationID: r.ID, View: types.RuntimeMeasurementDistribution,
		Label:   "资源申请与释放 — 独立事件流量统计",
		Columns: []string{"观测主体", "资源族", "源操作 / 动作", "子类型编号", "子类型名称", "事件次数", "大小状态次数", "已知非负大小合计", "单位"},
		Notes: []string{
			fmt.Sprintf("审计来源：%s；原查询窗口 [%.9f, %.9f%s 秒；请求主体 scope=%s，编号=%d，名称=%q。按源事件起始时间选取；同编号跨来源不证明同一主体。", p.SourcePath, p.Window.StartTs, p.Window.EndTs, right, p.TargetScope, p.TargetPID, p.TargetThread),
			fmt.Sprintf("原查询全集事件=%d；独立分组=%d；交接省略组=%d（事件=%d）；本表另省略组=%d（事件=%d）；本表已展示组=%d；分组展示完整=%t。统计分母是原查询全集，不是保留的事件/栈帧预览；省略不等于零。", s.EventCount, s.TotalGroups, s.OmittedGroups, s.OmittedGroupEvents, len(s.Groups)-retained, displayOmittedEvents, retained, s.OmittedGroups == 0 && retained == len(s.Groups)),
			"这是事件流量，不是净存量、活跃资源、配对生命周期或泄漏。申请和释放独立计数，不由结束时间合成释放；不累加 native_hook_statistic 报告，也不合并不同主体、资源族、操作、子类型或单位。",
			"大小状态次数依次保留已知非负、空值、未提供、无效、负值；合计仅含已知非负值，没有已知值时未知，真实零保持0。bytes仅来自原生堆/映射协议，其他单位未声明。子类型编号/文本原样保留，未知和空文本不互换；不授CPU、执行/等待时长、因果关系或目标全集资格。",
		}}
	for _, g := range s.Groups[:retained] {
		owner := fmt.Sprintf("线程ID=%d；进程ID=%d；线程名=%s；进程名=%s；线程实例=%d；进程实例=%d", g.TID, g.PID, resourceFactName(g.Thread), resourceFactName(g.Process), g.ITID, g.IPID)
		total := g.KnownSizeTotal
		if total == "" {
			total = "未知（没有已知非负大小）"
		}
		unit := g.SizeUnit
		if unit == "unspecified" {
			unit = "未声明（unspecified）"
		}
		stateCounts := fmt.Sprintf("已知非负=%d；空值=%d；未提供=%d；无效=%d；负值=%d", g.KnownSizeEvents, g.NullSizeEvents, g.UnavailableSizeEvents, g.InvalidSizeEvents, g.NegativeSizeEvents)
		t.Rows = append(t.Rows, []string{owner, g.Family, g.Operation + " / " + g.Action, resourceFactScalar(g.Subtype.ID), resourceFactText(g.Subtype.Name), strconv.Itoa(g.EventCount), stateCounts, total, unit})
	}
	return t
}
