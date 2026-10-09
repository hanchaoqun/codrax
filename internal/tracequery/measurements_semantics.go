package tracequery

import (
	"github.com/hanchaoqun/codrax/internal/tracewire"
	"strconv"
)

func (p *traceEventSemanticProjector) measurement(r tracewire.MeasureInterval) {
	p.known("source.representation", "sql_measure_interval")
	p.known("source.table", "measure")
	p.known("source.row_id", strconv.FormatInt(r.RowID, 10))
	p.known("source.subject_role", "raw_measurement_unknown_resource")
	if r.Filter != nil && r.Filter.Name.StorageClass == "text" && r.Filter.Name.Encoding == "" {
		p.known("plugin.metric", r.Filter.Name.Value)
	} else {
		p.unknown("plugin.metric", "unavailable", r.FilterStatus)
	}
	if r.MeasureType.StorageClass == "text" && r.MeasureType.Encoding == "" {
		p.known("plugin.category", r.MeasureType.Value)
	}
	for _, f := range []struct {
		key string
		s   tracewire.MeasureScalar
	}{{"source.start_ns", r.StartNS}, {"source.duration_ns", r.DurationNS}} {
		if _, ok := f.s.Integer(); ok {
			p.known(f.key, f.s.Value)
		} else {
			p.unknown(f.key, "unavailable", f.s.StorageClass)
		}
	}
	if end, ok := r.EndNS(); ok {
		p.known("source.end_ns", strconv.FormatInt(end, 10))
	}
	if r.Value.StorageClass == "null" || r.Value.StorageClass == "absent" || r.Value.Encoding != "" {
		p.unknown("plugin.value", "unavailable", r.Value.StorageClass)
	} else {
		p.known("plugin.value", r.Value.Value)
	}
	p.known("counter.aggregation_status", "not_aggregated_unit_and_quantity_semantics_unknown")
}
