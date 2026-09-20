package tracequery

// queryViewInvalidResult follows the engine's other invalid-query results:
// retain the requested operation/window for diagnosis, but publish no rows,
// enumeration coverage, or evidence for an operation that never executed.
func queryViewInvalidResult(idx *Index, q Query, err error) Result {
	result := Result{
		View:      CanonicalViewName(q.View),
		TimeUnit:  "seconds",
		TimeStart: q.TimeStart,
		TimeEnd:   q.TimeEnd,
		Caveats:   []string{"view_invalid=true; no trace rows were evaluated; " + err.Error()},
	}
	if idx != nil {
		result.SourcePath = idx.Path
	}
	return result
}
