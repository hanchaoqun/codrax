package types

import "strconv"

var runtimeEventCoreDescriptors = []RuntimeEventFieldDescriptor{
	{"event.trace_time_seconds", "Trace 时刻", "decimal", "s"},
	{"event.type", "事件类型", "text", ""},
	{"event.name", "事件名", "text", ""},
	{"event.comm", "记录线程名", "text", ""},
	{"source.path", "事件来源", "text", ""},
	{"source.local_line", "来源行号", "int64", ""},
	{"source.trace_time_seconds", "源 Trace 时刻", "decimal", "s"},
	{"emitter.tid", "发射线程 ID", "int64", ""},
	{"emitter.tgid", "发射进程 ID", "int64", ""},
	{"event.cpu", "记录 CPU", "int64", ""},
}

var runtimeEventJankDescriptors = []RuntimeEventFieldDescriptor{
	{"jank.appid", "appid（标记载荷）", "int64", ""},
	{"jank.start_ts_ns", "卡顿起点", "int64", "ns"},
	{"jank.end_ts_ns", "卡顿终点", "int64", "ns"},
	{"jank.jank_frames", "卡顿帧数", "int64", "frames"},
	{"jank.reported_duration_ns", "标记报告时长", "int64", "ns"},
	{"jank.time_domain_status", "载荷时间坐标状态", "text", ""},
}

// All extraction is typed. The raw preview and model-authored summaries are
// deliberately absent; a missing coordinate is never rendered as zero.
func runtimeEventNativeFields(row TraceEventSearchInventoryRow) ([]TraceEventSemanticField, []RuntimeEventFieldDescriptor) {
	descriptors := append([]RuntimeEventFieldDescriptor(nil), runtimeEventCoreDescriptors...)
	var fields []TraceEventSemanticField
	add := func(d RuntimeEventFieldDescriptor, value string, known bool, reason string) {
		f := TraceEventSemanticField{Key: d.Key, Type: d.Type, Unit: d.Unit, Status: "known", Value: &value}
		if !known {
			f.Status, f.Value, f.IssueReason = "unavailable", nil, reason
		}
		fields = append(fields, f)
	}
	traceTimeKnown := true
	if row.EventType == "measure_interval" || row.EventType == "process_measure_interval" {
		// These producers use a nonnegative ordering coordinate even when the
		// source timestamp is absent or negative. Only the separately verified
		// source coordinate can establish that the ordering value is observed.
		traceTimeKnown = row.SourceTimeKnown && row.TraceTimeSeconds == row.SourceTimeSeconds
	}
	add(descriptors[0], strconv.FormatFloat(row.TraceTimeSeconds, 'f', -1, 64), traceTimeKnown, "trace_time_unpositioned")
	add(descriptors[1], row.EventType, true, "")
	add(descriptors[2], row.EventName, row.EventName != "", "event_name_not_provided")
	add(descriptors[3], row.Comm, row.Comm != "", "comm_not_provided")
	add(descriptors[4], row.SourcePath, row.SourcePath != "", "source_path_not_provided")
	add(descriptors[5], strconv.Itoa(row.LocalLine), row.LocalLine > 0, "source_line_not_provided")
	add(descriptors[6], strconv.FormatFloat(row.SourceTimeSeconds, 'f', -1, 64), row.SourceTimeKnown, "source_time_unknown")
	add(descriptors[7], strconv.Itoa(row.EmitterTID), row.EmitterTIDKnown != nil && *row.EmitterTIDKnown, "emitter_tid_unverified")
	add(descriptors[8], strconv.Itoa(row.EmitterTGID), row.EmitterTGIDKnown != nil && *row.EmitterTGIDKnown, "emitter_tgid_unverified")
	add(descriptors[9], strconv.Itoa(row.CPU), row.CPUKnown != nil && *row.CPUKnown, "cpu_unverified")
	if row.Semantics != nil {
		semantic := CloneTraceEventSemantics(row.Semantics)
		for _, field := range semantic.Fields {
			d, found := LookupTraceEventSemanticDescriptor(field.Key)
			if !found {
				continue
			}
			descriptors = append(descriptors, RuntimeEventFieldDescriptor{Key: d.Key, Label: d.Label, Type: d.Type, Unit: d.Unit})
			fields = append(fields, field)
		}
	}
	if j := row.JankEvent; j != nil {
		descriptors = append(descriptors, runtimeEventJankDescriptors...)
		var values [5]int64
		if j.Values != nil {
			values = [5]int64{j.Values.AppID, j.Values.StartTSNS, j.Values.EndTSNS, j.Values.JankFrames, j.Values.ReportedDurationNS}
		}
		for n, value := range values {
			add(runtimeEventJankDescriptors[n], strconv.FormatInt(value, 10), j.Values != nil, j.IssueReason)
			if j.Values == nil && j.IssueReason != "" {
				fields[len(fields)-1].Status = "invalid"
			}
		}
		add(runtimeEventJankDescriptors[5], j.TimeDomainStatus, true, "")
	}
	return appendRuntimeEventMillisecondFields(fields, descriptors)
}
