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
	Basis            string          `json:"coverage_basis"`
	DeclaredPlans    int             `json:"declared_plans"`
	ResumedPlans     int             `json:"resumed_plans"`
	ObservedQueries  int             `json:"observed_queries"`
	Attempts         int             `json:"attempts"`
	PlannedOutcomes  map[Outcome]int `json:"planned_outcomes"`
	ObservedOutcomes map[Outcome]int `json:"observed_outcomes"`
}

// ExecutionView is an atomic detached snapshot for current-run consumers.
// Origins confer no read, window, measurement, completion or causal permission.
type ExecutionView struct {
	Snapshot     Snapshot
	Coverage     ExecutionCoverage
	QueryOrigins map[string]QueryOrigin
}

func (c *Catalog) CurrentExecution() ExecutionView {
	out := ExecutionView{
		Coverage:     ExecutionCoverage{Basis: "observed_queries_only", PlannedOutcomes: map[Outcome]int{}, ObservedOutcomes: map[Outcome]int{}},
		QueryOrigins: map[string]QueryOrigin{},
	}
	if c == nil {
		out.Snapshot = c.Snapshot()
		return out
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out.Snapshot = c.snapshotLocked()
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

const ExecutionCoverageBoundary = "Current registered plans and observed-only direct queries are separate sets. When no current plans are registered, expected coverage is unknown; observed calls are only a queried subset. Registered plans describe this catalog, not proof of the complete user-requested population. Attempts and final execution outcomes do not establish native availability or analysis completion. Historical attempts are not counted as current work."
