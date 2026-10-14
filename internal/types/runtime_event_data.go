package types

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// RuntimeEventData retains one native query's fields and membership before any
// answer selection. It is not a unique-event, thread, completion or causal map.
type RuntimeEventData struct {
	Version            int                               `json:"version"`
	QueryScopeID       string                            `json:"query_scope_id"`
	Query              TraceEventSearchInventoryQuery    `json:"query"`
	Coverage           TraceEventSearchInventoryCoverage `json:"coverage"`
	RowsComplete       bool                              `json:"rows_complete"`
	HandoffRowsOmitted int                               `json:"handoff_rows_omitted"`
	Fields             []RuntimeEventFieldDescriptor     `json:"fields"`
	Rows               []RuntimeEventRow                 `json:"rows"`
}

type RuntimeEventFieldDescriptor struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Type  string `json:"type"`
	Unit  string `json:"unit,omitempty"`
}

type RuntimeEventRow struct {
	ID     string                    `json:"id"`
	Fields []TraceEventSemanticField `json:"fields"`
}

// RuntimeEventSelection selects only published row/field identities. Counts
// use exactly the retained selected rows; no expression or inferred filter is
// accepted. Omitted row_ids means every retained member, never every source row.
type RuntimeEventSelection struct {
	RowIDs    []string `json:"row_ids,omitempty"`
	FieldKeys []string `json:"field_keys,omitempty"`
	GroupBy   []string `json:"group_by,omitempty"`
}

func (s *RuntimeEventSelection) Clone() *RuntimeEventSelection {
	if s == nil {
		return nil
	}
	out := *s
	out.RowIDs = append([]string(nil), s.RowIDs...)
	out.FieldKeys = append([]string(nil), s.FieldKeys...)
	out.GroupBy = append([]string(nil), s.GroupBy...)
	return &out
}

func runtimeEventSelectionKey(s *RuntimeEventSelection) string {
	raw, _ := json.Marshal(s)
	return string(raw)
}

func (d *RuntimeEventData) Clone() *RuntimeEventData {
	if d == nil {
		return nil
	}
	out := *d
	// Reuse the original inventory's deep-copy rules for query and coverage.
	inv := CloneTraceEventSearchInventory(&TraceEventSearchInventory{Query: d.Query, Coverage: d.Coverage})
	out.Query, out.Coverage = inv.Query, inv.Coverage
	out.Fields = append([]RuntimeEventFieldDescriptor(nil), d.Fields...)
	out.Rows = make([]RuntimeEventRow, len(d.Rows))
	for i, row := range d.Rows {
		out.Rows[i] = RuntimeEventRow{ID: row.ID, Fields: CloneTraceEventSemantics(&TraceEventSemantics{Fields: row.Fields}).Fields}
	}
	return &out
}

// BuildTraceEventRuntimeMeasurementPublication is called only at native tool
// publication. The decoder repeats this projection from the unchanged typed
// inventory, so serialized tables cannot replace the original field values.
func BuildTraceEventRuntimeMeasurementPublication(r ObservationRecord) (RuntimeMeasurementPublication, bool) {
	if r.Negative || !IsValidTraceEventSearchInventoryRecord(r) {
		return RuntimeMeasurementPublication{}, false
	}
	i := CloneTraceEventSearchInventory(r.EventSearchInventory)
	d := &RuntimeEventData{Version: 1, QueryScopeID: i.QueryScopeID, Query: i.Query, Coverage: i.Coverage,
		RowsComplete: i.RowsComplete, HandoffRowsOmitted: i.HandoffRowsOmitted}
	seen := map[string]bool{}
	for n, row := range i.Rows {
		fields, descriptors := runtimeEventNativeFields(row)
		for _, descriptor := range descriptors {
			if !seen[descriptor.Key] {
				seen[descriptor.Key] = true
				d.Fields = append(d.Fields, descriptor)
			}
		}
		d.Rows = append(d.Rows, RuntimeEventRow{ID: fmt.Sprintf("event-%d", n+1), Fields: fields})
	}
	// A real zero population retains a known, useful structural column set.
	if len(d.Fields) == 0 {
		d.Fields = append([]RuntimeEventFieldDescriptor(nil), runtimeEventCoreDescriptors[:3]...)
	}
	if len(d.Fields) > 128 {
		return RuntimeMeasurementPublication{}, false
	}
	query, err := json.Marshal(i.Query)
	if err != nil {
		return RuntimeMeasurementPublication{}, false
	}
	t := RuntimeMeasurementTable{ObservationID: r.ID, View: RuntimeMeasurementMembers,
		Label: "Trace 原生事件记录（同一查询）", EventData: d, nativeEventData: true,
		nativeEventScopeNoteIndex: -1,
		Notes:                     []string{"来源：" + r.SourceRef.Path, "查询条件：" + string(query), "字段只表示原始事件观测，不证明调度身份、配对关系、等待或根因。"}}
	if r.SourceRef.PayloadRef != "" {
		t.Notes = append(t.Notes, "完整查询结果："+r.SourceRef.PayloadRef)
	}
	for _, field := range d.Fields {
		if descriptor, ok := LookupTraceEventSemanticDescriptor(field.Key); ok && descriptor.Family == "scheduler" {
			t.Notes = append(t.Notes, TraceSchedulerIdentityTeaching)
			break
		}
	}
	t.Notes = append(t.Notes, i.Caveats...)
	// Exact producer-owned lookup notes distinguish the requested business
	// window from a wider matching window. They are display context only.
	for _, note := range r.RichNotes {
		key, _, _ := strings.Cut(note, "=")
		switch key {
		case "requested_window", "matching_window", "matching_window_policy", "metric_denominator_window":
			t.Notes = append(t.Notes, note)
		}
	}
	bound, ok := selectRuntimeEventTable(t, nil)
	if !ok {
		return RuntimeMeasurementPublication{}, false
	}
	return RuntimeMeasurementPublication{Version: 1, ObservationID: r.ID, Source: r.SourceRef, Tables: []RuntimeMeasurementTable{bound}}, true
}

func runtimeEventPublicationMatches(r ObservationRecord, p RuntimeMeasurementPublication) bool {
	expected, ok := BuildTraceEventRuntimeMeasurementPublication(r)
	if !ok {
		return false
	}
	// Private trust/selection state is never compared with public transport.
	want, err := json.Marshal(expected)
	got, otherErr := json.Marshal(p)
	return err == nil && otherErr == nil && reflect.DeepEqual(want, got)
}

func runtimeEventFieldDisplay(field TraceEventSemanticField, present bool) string {
	if !present {
		return "未提供"
	}
	if field.Status == "known" && field.Value != nil {
		if *field.Value == "" {
			return "\"\"（已知空值）"
		}
		if field.Type == "text" {
			return strconv.Quote(*field.Value)
		}
		return *field.Value
	}
	status := map[string]string{"unavailable": "未知", "invalid": "无效", "omitted": "已省略"}[field.Status]
	if status == "" {
		status = "未验证"
	}
	if field.IssueReason != "" {
		status += "（" + field.IssueReason + "）"
	}
	if field.Omitted != nil {
		status += " 原值" + strconv.Itoa(field.Omitted.OriginalBytes) + "字节 SHA256=" + field.Omitted.SHA256
	}
	return status
}

func runtimeEventColumn(d RuntimeEventFieldDescriptor) string {
	if d.Unit != "" {
		return d.Label + " (" + d.Unit + ")"
	}
	return d.Label
}

func runtimeEventField(row RuntimeEventRow, key string) (TraceEventSemanticField, bool) {
	for _, field := range row.Fields {
		if field.Key == key {
			return field, true
		}
	}
	return TraceEventSemanticField{}, false
}

func runtimeEventChoiceKeys(keys []string, available map[string]bool, max int) bool {
	if len(keys) > max {
		return false
	}
	seen := map[string]bool{}
	for _, key := range keys {
		if strings.TrimSpace(key) == "" || !available[key] || seen[key] {
			return false
		}
		seen[key] = true
	}
	return true
}
