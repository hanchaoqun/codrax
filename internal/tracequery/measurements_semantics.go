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
	p.known("source.filter_table", "measure_filter")
	p.known("source.filter_status", r.FilterStatus)
	p.measurementScalar("source.filter_id", "source.filter_storage_class", "source.filter_encoding", r.FilterID)
	if r.Filter != nil && r.Filter.Name.StorageClass == "text" && r.Filter.Name.Encoding == "" {
		p.known("plugin.metric", r.Filter.Name.Value)
	} else {
		p.unknown("plugin.metric", "unavailable", r.FilterStatus)
	}
	p.measurementScalar("plugin.category", "plugin.category_storage_class", "plugin.category_encoding", r.MeasureType)
	if r.Filter != nil {
		p.measurementScalar("plugin.filter_type", "plugin.filter_type_storage_class", "plugin.filter_type_encoding", r.Filter.Type)
		p.measurementScalar("source.arg_set_id", "source.arg_set_storage_class", "source.arg_set_encoding", r.Filter.SourceArgSetID)
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
	p.measurementScalar("plugin.value", "plugin.value_storage_class", "plugin.value_encoding", r.Value)
	p.known("counter.aggregation_status", "not_aggregated_unit_and_quantity_semantics_unknown")
}

func (p *traceEventSemanticProjector) measurementScalar(key, storageKey, encodingKey string, value tracewire.MeasureScalar) {
	p.known(storageKey, value.StorageClass)
	encoding := value.Encoding
	if value.StorageClass == "blob" {
		encoding = "base64"
	}
	p.known(encodingKey, encoding)
	if value.StorageClass == "null" || value.StorageClass == "absent" {
		p.unknown(key, "unavailable", value.StorageClass)
	} else {
		p.known(key, value.Value)
	}
}
