package types

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// selectRuntimeEventTable is the sole facts/COUNT projection for all answer
// surfaces. Neither the selection order nor display limits change membership
// order, source scope, unknown values or the original query's census.
func selectRuntimeEventTable(original RuntimeMeasurementTable, selection *RuntimeEventSelection) (RuntimeMeasurementTable, bool) {
	if original.EventData == nil || !original.nativeEventData || original.View != RuntimeMeasurementMembers {
		return RuntimeMeasurementTable{}, false
	}
	d := original.EventData
	if d.Version != 1 || len(d.Rows) > TraceEventSearchInventoryRowLimit || len(d.Fields) == 0 || len(d.Fields) > 128 {
		return RuntimeMeasurementTable{}, false
	}
	fields := map[string]RuntimeEventFieldDescriptor{}
	fieldKeys, rowKeys := map[string]bool{}, map[string]bool{}
	for _, field := range d.Fields {
		fields[field.Key], fieldKeys[field.Key] = field, true
	}
	for _, row := range d.Rows {
		rowKeys[row.ID] = true
	}
	s := RuntimeEventSelection{}
	if selection != nil {
		s = *selection
	}
	if len(s.FieldKeys) > 0 && len(s.GroupBy) > 0 {
		return RuntimeMeasurementTable{}, false
	}
	if !runtimeEventChoiceKeys(s.RowIDs, rowKeys, TraceEventSearchInventoryRowLimit) ||
		!runtimeEventChoiceKeys(s.FieldKeys, fieldKeys, 128) || !runtimeEventChoiceKeys(s.GroupBy, fieldKeys, 4) {
		return RuntimeMeasurementTable{}, false
	}
	selectedIDs := map[string]bool{}
	for _, id := range s.RowIDs {
		selectedIDs[id] = true
	}
	var selected []RuntimeEventRow
	for _, row := range d.Rows {
		if len(selectedIDs) == 0 || selectedIDs[row.ID] {
			selected = append(selected, row)
		}
	}
	keys := s.FieldKeys
	if len(keys) == 0 {
		for _, field := range d.Fields {
			keys = append(keys, field.Key)
		}
	}
	if len(s.GroupBy) > 0 {
		keys = s.GroupBy
	}
	out := original.Clone()
	out.Columns, out.Rows = nil, nil
	out.MemberSet, out.coverageScope, out.DisplayPopulation = nil, nil, nil
	out.nativeTraceDisplayPopulation = false
	out.nativeFactIdentity = ""
	out.nativeEventSelectionKey = runtimeEventSelectionKey(selection)
	for _, key := range keys {
		out.Columns = append(out.Columns, runtimeEventColumn(fields[key]))
	}
	if len(s.GroupBy) > 0 {
		out.Label = "原生事件分组计数（同一查询的保留选集）"
		out.Columns = append(out.Columns, "记录次数")
		positions := map[string]int{}
		for _, row := range selected {
			var identity []any
			var cells []string
			for _, key := range keys {
				field, present := runtimeEventField(row, key)
				identity = append(identity, struct {
					Key     string
					Present bool
					Field   TraceEventSemanticField
				}{key, present, field})
				cells = append(cells, runtimeEventFieldDisplay(field, present))
			}
			raw, err := json.Marshal(identity)
			if err != nil {
				return RuntimeMeasurementTable{}, false
			}
			if n, found := positions[string(raw)]; found {
				count, _ := strconv.Atoi(out.Rows[n][len(keys)])
				out.Rows[n][len(keys)] = strconv.Itoa(count + 1)
			} else {
				positions[string(raw)] = len(out.Rows)
				out.Rows = append(out.Rows, append(cells, "1"))
			}
		}
	} else {
		out.Label = "原生事件字段（同一查询的保留选集）"
		for _, row := range selected {
			var cells []string
			for _, key := range keys {
				field, present := runtimeEventField(row, key)
				cells = append(cells, runtimeEventFieldDisplay(field, present))
			}
			out.Rows = append(out.Rows, cells)
		}
	}
	// Remove only the producer-owned prior scope note when rebinding the
	// original table. No prose is scanned or treated as an authority signal.
	if n := original.nativeEventScopeNoteIndex; original.nativeEventSelectionKey != "" && n >= 0 && n < len(out.Notes) {
		out.Notes = append(out.Notes[:n], out.Notes[n+1:]...)
	}
	population := "查询记录完整"
	if !d.RowsComplete {
		population = "查询记录不完整；分组仅覆盖保留记录，不能代表完整查询的分组总数"
	}
	if len(s.RowIDs) > 0 {
		population += "；显式选择的保留记录，选择本身不证明满足业务筛选或完整枚举"
	} else {
		population += "；本视图覆盖交接保留的全部记录"
	}
	out.nativeEventScopeNoteIndex = len(out.Notes)
	out.Notes = append(out.Notes, fmt.Sprintf("本视图：%d 条；库存保留：%d 条；查询匹配：%d 条；查询已返回：%d 条；交接省略：%d 条。%s。各查询独立，次数不代表唯一请求数或根因。", len(selected), len(d.Rows), d.Coverage.MatchedTotal, d.Coverage.Emitted, d.HandoffRowsOmitted, population))
	return out, true
}
