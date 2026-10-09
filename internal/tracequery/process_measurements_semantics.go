package tracequery

import (
	"github.com/hanchaoqun/codrax/internal/tracewire"
	"strconv"
)

// ProcessMeasurementSourceTimestamp returns the signed source coordinate,
// not the nonnegative publication/sort coordinate in Event.Ts.
func ProcessMeasurementSourceTimestamp(event Event) (int64, bool) {
	if event.Type != EventProcessMeasureInterval || event.PluginFields == nil || event.PluginFields.ProcessMeasure == nil {
		return 0, false
	}
	return event.PluginFields.ProcessMeasure.StartNS.Integer()
}

func processMeasurementEventInTimeWindow(event Event, q Query) bool {
	startSet := !q.timeStartBackfilled && queryExplicitTimeStart(q)
	endSet := !q.timeEndBackfilled && queryExplicitTimeEnd(q)
	if !startSet && !endSet {
		return true
	}
	ts, known := ProcessMeasurementSourceTimestamp(event)
	if !known {
		return false
	}
	if startSet {
		start, ok := processMeasurementNS(q.TimeStart)
		if !ok || ts < start {
			return false
		}
	}
	if endSet {
		end, ok := processMeasurementNS(q.TimeEnd)
		if !ok || ts >= end {
			return false
		}
	}
	return true
}

func (p *traceEventSemanticProjector) processMeasurement(r tracewire.ProcessMeasureInterval) {
	p.known("source.representation", "sql_process_measure_interval")
	p.known("source.table", "process_measure")
	p.known("source.row_id", strconv.FormatInt(r.RowID, 10))
	p.known("source.subject_role", "process_measurement_not_thread_execution")
	p.known("source.filter_table", "process_measure_filter")
	p.known("source.filter_storage_class", r.FilterID.StorageClass)
	filterEncoding := ""
	if r.FilterID.StorageClass == "blob" {
		filterEncoding = "base64"
	}
	p.known("source.filter_encoding", filterEncoding)
	if r.FilterID.StorageClass == "null" || r.FilterID.StorageClass == "absent" {
		p.unknown("source.filter_id", "unavailable", r.FilterID.Status)
	} else {
		p.known("source.filter_id", r.FilterID.Value)
	}
	p.known("plugin.value_storage_class", r.Value.StorageClass)
	encoding := ""
	if r.Value.StorageClass == "blob" {
		encoding = "base64"
	}
	p.known("plugin.value_encoding", encoding)
	if r.NameKnown {
		p.known("plugin.metric", r.Name)
	} else {
		p.unknown("plugin.metric", "unavailable", "filter_name_unavailable")
	}
	if r.TypeKnown {
		p.known("plugin.category", r.MeasureType)
	}
	for _, field := range []struct {
		key    string
		scalar tracewire.ProcessMeasureScalar
	}{{"source.start_ns", r.StartNS}, {"source.duration_ns", r.DurationNS}, {"source.owner_ipid", r.IPID}} {
		if _, ok := field.scalar.Integer(); ok {
			p.known(field.key, field.scalar.Value)
		} else {
			p.unknown(field.key, "unavailable", field.scalar.Status+"_"+field.scalar.StorageClass)
		}
	}
	if end, ok := r.EndNS(); ok {
		p.known("source.end_ns", strconv.FormatInt(end, 10))
	}
	if r.PID != nil {
		p.known("source.owner_pid", strconv.Itoa(*r.PID))
	} else {
		p.unknown("source.owner_pid", "unavailable", r.OwnerStatus)
	}
	if _, ok := r.Value.Integer(); ok {
		p.known("plugin.value", r.Value.Value)
	} else {
		p.unknown("plugin.value", "unavailable", r.Value.Status+"_"+r.Value.StorageClass)
		if r.Value.Status == "invalid_storage" {
			p.known("counter.raw_value", r.Value.Value)
		}
	}
	p.known("counter.aggregation_status", "not_aggregated_unit_and_quantity_semantics_unknown")
}
