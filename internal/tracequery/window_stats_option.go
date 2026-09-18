package tracequery

// WithWindowStats explicitly selects the optional statistics attached to a
// wakeup_chain result. An unset option keeps the historical default (enabled).
// Other views still compute and publish their required statistics regardless
// of this option. Like WithRunContext, this returns a copy of the query.
func (q Query) WithWindowStats(include bool) Query {
	q.IncludeWindowStats = include
	q.windowStatsSpecified = true
	return q
}
