package tracequery

// PerfSamplingTreeUnsupportedSelectors names the exact typed selectors that
// cannot define the full sampling population. Callers must not remove them.
func PerfSamplingTreeUnsupportedSelectors(q Query) []string {
	var fields []string
	for _, field := range []struct {
		name    string
		present bool
	}{
		{"line_start", q.LineStart != 0}, {"line_end", q.LineEnd != 0},
		{"pattern", q.Pattern != ""}, {"patterns", len(q.Patterns) != 0},
		{"span_name", q.SpanName != ""}, {"event_types", len(q.EventTypes) != 0},
		{"event_names", len(q.EventNames) != 0}, {"event_field_filters", len(q.EventFieldFilters) != 0},
		{"trace_mark_actions", len(q.TraceMarkActions) != 0},
		{"target_scope", normalizedTargetScope(q.TargetScope) != TargetScopeThread},
	} {
		if field.present {
			fields = append(fields, field.name)
		}
	}
	return fields
}
