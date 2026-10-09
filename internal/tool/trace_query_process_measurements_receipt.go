package tool

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/tracewire"
	"github.com/hanchaoqun/codrax/internal/types"
)

func processMeasurementScalar(s tracewire.ProcessMeasureScalar) string {
	switch s.Status {
	case "known":
		return s.Value
	case "null":
		return "未知（NULL）"
	case "unavailable":
		return "未提供"
	default:
		return fmt.Sprintf("%q（%s存储，非已知整数）", s.Value, s.StorageClass)
	}
}

func processMeasurementOwner(r tracewire.ProcessMeasureInterval) string {
	if r.OwnerStatus == "known" && r.PID != nil {
		return fmt.Sprintf("%s (PID %d)", r.ProcessName, *r.PID)
	}
	return "进程归属未确认（" + r.OwnerStatus + "）"
}

func processMeasurementEndpoint(v *int64) string {
	if v == nil {
		return "未知"
	}
	return strconv.FormatInt(*v, 10)
}

// Reuses the optional native-table selector. Values and timestamps are exact
// source strings; it grants display, never aggregate-completion or causality.
func traceQueryProcessMeasurementsReceipt(r types.ObservationRecord, p tracequery.ProcessMeasurementsResult) string {
	start, end, known := types.TraceObservationContinuousQueryWindow(r.SourceRef)
	if !tracequery.ValidProcessMeasurements(p) || !known || p.SourcePath != r.SourceRef.Path || p.Window.StartTs != start || p.Window.EndTs != end ||
		p.Status != r.Object || r.Span.StartTs != start || r.Span.EndTs != end || p.TargetPID != r.SourceRef.QueryTargetPID ||
		r.SourceRef.QueryTargetThread != "" || p.TargetScope != r.SourceRef.QueryTargetScope {
		return ""
	}
	right := ")"
	if p.Window.EndInclusive {
		right = "]"
	}
	base := []string{fmt.Sprintf("查询窗口 [%s,%s%s 秒。", traceQueryDisplaySeconds(start), traceQueryDisplaySeconds(end), right), "Source: " + p.SourcePath,
		"查询状态：" + p.Status + "；没有可展示记录不等于指标值为0。",
		"数值单位及存量/累计/增量含义未由源协议提供；不由指标名称推断。进程量测不证明线程执行、CPU活动、等待或响应根因。",
		"原始区间与窗口内交集分列；缺持续时间仅为时间点观测。NULL不等于0，重叠记录不合并、区间空洞不填满。记录数不证明采集完整。"}
	base = append(base, p.Caveats...)
	keep := len(p.Rows)
	for {
		notes := append([]string(nil), base...)
		notes = append(notes, fmt.Sprintf("查询匹配%d条；引擎保留%d条、省略%d条；本次表格保留%d条、额外省略%d条。另有%d条无法定位时间，不分配到查询窗口。完整查询载荷保留原数据。", p.TotalRows, len(p.Rows), p.OmittedRows, keep, len(p.Rows)-keep, p.UnpositionedRows))
		summary := processMeasurementSeriesTable(r.ID, p.Rows[:keep], notes)
		members := types.RuntimeMeasurementTable{ObservationID: r.ID, View: types.RuntimeMeasurementMembers, Label: "进程量测原始记录",
			Columns: []string{"进程", "采集内进程ID", "指标", "过滤器ID", "原始类型", "原始值（单位未提供）", "原起点(ns)", "原持续(ns)", "源记录/行"}, Notes: notes}
		timeline := types.RuntimeMeasurementTable{ObservationID: r.ID, View: types.RuntimeMeasurementTimeline, Label: "进程量测窗口内观测",
			Columns: []string{"进程", "采集内进程ID", "指标", "原始值（单位未提供）", "窗口内起点(ns)", "窗口内终点(ns)", "选取依据", "源记录/行"}, Notes: notes}
		for _, row := range p.Rows[:keep] {
			rec := row.Record
			name, kind := "未提供", "未提供"
			if rec.NameKnown {
				name = rec.Name
			}
			if rec.TypeKnown {
				kind = rec.MeasureType
			}
			ref := fmt.Sprintf("%d / %d", rec.RowID, row.SourceLine)
			members.Rows = append(members.Rows, []string{processMeasurementOwner(rec), processMeasurementScalar(rec.IPID), name, processMeasurementScalar(rec.FilterID), kind, processMeasurementScalar(rec.Value), processMeasurementScalar(rec.StartNS), processMeasurementScalar(rec.DurationNS), ref})
			selection := row.Selection
			switch selection {
			case "interval_overlap":
				selection = "源区间与查询窗交集"
			case "point":
				selection = "源时间点"
			case "unknown_duration":
				selection = "窗内时间点，持续未知"
			}
			timeline.Rows = append(timeline.Rows, []string{processMeasurementOwner(rec), processMeasurementScalar(rec.IPID), name, processMeasurementScalar(rec.Value), processMeasurementEndpoint(row.ClippedStartNS), processMeasurementEndpoint(row.ClippedEndNS), selection, ref})
		}
		publication := types.RuntimeMeasurementPublication{Version: 1, ObservationID: r.ID, Source: r.SourceRef, Tables: []types.RuntimeMeasurementTable{summary, members, timeline}}
		raw, err := json.Marshal(publication)
		if err != nil {
			return ""
		}
		if len(raw) <= 64<<10 {
			return types.TraceNoteKeyRuntimeMeasurement + "=" + string(raw)
		}
		if keep == 0 {
			return ""
		}
		keep--
	}
}

// Preview each retained owner/filter series once before the row-level views.
// This is a display regrouping, not a new statistic: no sum, endpoint delta,
// interpolation or complete-population claim is introduced.
func processMeasurementSeriesTable(id string, rows []tracequery.ProcessMeasurementRow, notes []string) types.RuntimeMeasurementTable {
	table := types.RuntimeMeasurementTable{ObservationID: id, View: types.RuntimeMeasurementSummary, Label: "各进程指标的保留观测序列",
		Columns: []string{"进程", "采集内进程ID", "指标", "过滤器ID", "保留记录数", "原起点(ns)→原值（单位未提供）"}, Notes: notes}
	positions := map[string]int{}
	var sequences [][]string
	for _, row := range rows {
		r := row.Record
		keyParts := []any{row.SourcePath, r.IPID, r.FilterID, r.NameKnown, r.Name, r.TypeKnown, r.MeasureType, r.PID, r.OwnerStatus}
		if _, known := r.FilterID.Integer(); !known || r.OwnerStatus != "known" {
			keyParts = append(keyParts, r.RowID)
		}
		key, _ := json.Marshal(keyParts)
		pos, exists := positions[string(key)]
		if !exists {
			pos = len(table.Rows)
			positions[string(key)] = pos
			name := "未提供"
			if r.NameKnown {
				name = r.Name
			}
			table.Rows = append(table.Rows, []string{processMeasurementOwner(r), processMeasurementScalar(r.IPID), name, processMeasurementScalar(r.FilterID), "", ""})
			sequences = append(sequences, nil)
		}
		sequences[pos] = append(sequences[pos], processMeasurementScalar(r.StartNS)+"→"+processMeasurementScalar(r.Value))
	}
	for i := range table.Rows {
		table.Rows[i][4] = strconv.Itoa(len(sequences[i]))
		table.Rows[i][5] = strings.Join(sequences[i], "; ")
	}
	return table
}
