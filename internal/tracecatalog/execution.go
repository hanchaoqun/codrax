package tracecatalog

// QueryOrigin describes current-run registration only. It is deliberately not
// part of persisted QueryRecord: loading JSON cannot restore private markers.
type QueryOrigin string

const (
	QueryOriginDeclared QueryOrigin = "declared_current_plan"
	QueryOriginResumed  QueryOrigin = "resumed_current_plan"
	QueryOriginObserved QueryOrigin = "observed_query_only"
)

// ExecutionCoverage counts recorded work, never native availability, evidence
// completeness or completion of the whole user request. Ordinary direct calls
// are not an expected population just because registration precedes execution.
type ExecutionCoverage struct {
	Basis                  string          `json:"coverage_basis"`
	DeclaredPlans          int             `json:"declared_plans"`
	ResumedPlans           int             `json:"resumed_plans"`
	ObservedQueries        int             `json:"observed_queries"`
	Attempts               int             `json:"attempts"`
	QueriesWithAttempts    int             `json:"query_records_with_attempts"`
	QueriesWithoutAttempts int             `json:"query_records_without_attempts"`
	AttemptOutcomes        map[Outcome]int `json:"attempt_outcomes"`
	PlannedOutcomes        map[Outcome]int `json:"planned_outcomes"`
	ObservedOutcomes       map[Outcome]int `json:"observed_outcomes"`
}

// ExecutionView is an atomic detached snapshot for current-run consumers.
// Origins confer no read, window, measurement, completion or causal permission.
type ExecutionView struct {
	Snapshot     Snapshot
	Coverage     ExecutionCoverage
	QueryOrigins map[string]QueryOrigin
	Selection    *RepresentativeSnapshot
}

// SelectionState describes a real, current-run selection, never a conclusion
// inferred from discovery, equal specifications, or successful native queries.
func (e ExecutionView) SelectionState() string {
	if e.Selection == nil {
		return "not_selected"
	}
	return "selected"
}

func (c *Catalog) CurrentExecution() ExecutionView {
	out := ExecutionView{
		Coverage:     ExecutionCoverage{Basis: "observed_queries_only", PlannedOutcomes: map[Outcome]int{}, ObservedOutcomes: map[Outcome]int{}, AttemptOutcomes: map[Outcome]int{}},
		QueryOrigins: map[string]QueryOrigin{},
	}
	if c == nil {
		out.Snapshot = c.Snapshot()
		return out
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out.Snapshot = c.snapshotLocked()
	if c.representatives != nil {
		selection := c.representativeSnapshotLocked()
		out.Selection = &selection
	}
	for _, q := range out.Snapshot.Queries {
		origin := QueryOriginObserved
		switch {
		case c.declared[q.ID]:
			origin = QueryOriginDeclared
			out.Coverage.DeclaredPlans++
		case c.resumedPlans[q.ID]:
			origin = QueryOriginResumed
			out.Coverage.ResumedPlans++
		default:
			out.Coverage.ObservedQueries++
		}
		out.QueryOrigins[q.ID] = origin
		out.Coverage.Attempts += len(q.Attempts)
		if len(q.Attempts) == 0 {
			out.Coverage.QueriesWithoutAttempts++
		} else {
			out.Coverage.QueriesWithAttempts++
		}
		for _, attempt := range q.Attempts {
			out.Coverage.AttemptOutcomes[attempt.Outcome]++
		}
		if origin == QueryOriginObserved {
			out.Coverage.ObservedOutcomes[q.Outcome]++
		} else {
			out.Coverage.PlannedOutcomes[q.Outcome]++
		}
	}
	if out.Coverage.DeclaredPlans+out.Coverage.ResumedPlans > 0 {
		out.Coverage.Basis = "registered_current_plans"
	}
	return out
}

const ExecutionCoverageBoundary = "Registration origins (declared_plans, resumed_plans, observed_queries) partition query records: observed_queries means additional direct-query records, NOT all executed work. query_records_with_attempts counts distinct records actually attempted, including failure/empty; attempts counts all attempts including retries. Query outcomes describe each record's latest state, while attempt_outcomes retains every attempt. No current plans means expected coverage is unknown; registered plans describe only this catalog, not the complete user request. These counts establish neither native availability nor analysis completion. Historical attempts are not current work."
