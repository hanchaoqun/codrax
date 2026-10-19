package tracecatalog

import (
	"encoding/json"
	"sort"
)

// QueryCall is a navigation handle, not permission or evidence. The ordinary
// query dispatcher must still revalidate the current catalog, source and plan.
type QueryCall struct {
	Reference QueryReference `json:"catalog_query_ref"`
}

type QueryReference struct {
	CatalogID string `json:"catalog_id"`
	QueryID   string `json:"query_id"`
}

// CompactQueryNavigation keeps the current task coordinates and last attempt
// without replaying an unbounded retry transcript into every downstream stage.
// Full error messages and effective parameters remain in the original status
// page/saved index. Nothing in this projection restores a Catalog from JSON.
type CompactQueryNavigation struct {
	Source          string             `json:"source"`
	SourceStatus    string             `json:"source_status"`
	Origin          QueryOrigin        `json:"query_origin"`
	Query           QueryRecord        `json:"query"`
	Attempts        int                `json:"attempt_count"`
	AttemptOutcomes map[Outcome]int    `json:"attempt_outcomes"`
	LatestAttempt   *NavigationAttempt `json:"latest_attempt,omitempty"`
	QueryCall       *QueryCall         `json:"query_call,omitempty"`
}

type NavigationAttempt struct {
	ID             string  `json:"id"`
	Outcome        Outcome `json:"outcome"`
	SourceRevision string  `json:"source_revision,omitempty"`
	PayloadRef     string  `json:"payload_ref,omitempty"`
	RawRef         string  `json:"raw_ref,omitempty"`
	Truncated      bool    `json:"truncated,omitempty"`
	ErrorCode      string  `json:"error_code,omitempty"`
}

// NavigationQueries is a detached, complete current-query projection. Input
// order is stable; it does not choose a new representative or infer priority.
func (e ExecutionView) NavigationQueries() []CompactQueryNavigation {
	artifacts := map[string]Artifact{}
	for _, a := range e.Snapshot.Artifacts {
		artifacts[a.ID] = a
	}
	rows := make([]CompactQueryNavigation, 0, len(e.Snapshot.Queries))
	for _, q := range e.Snapshot.Queries {
		a := artifacts[q.ArtifactID]
		plan := q
		plan.Attempts = nil
		row := CompactQueryNavigation{Source: a.Path, SourceStatus: a.Status, Origin: e.QueryOrigins[q.ID], Query: cloneQuery(plan), Attempts: len(q.Attempts), AttemptOutcomes: map[Outcome]int{}}
		for _, attempt := range q.Attempts {
			row.AttemptOutcomes[attempt.Outcome]++
		}
		if len(q.Attempts) > 0 {
			last := q.Attempts[len(q.Attempts)-1]
			row.LatestAttempt = &NavigationAttempt{ID: last.ID, Outcome: last.Outcome, SourceRevision: last.SourceRevision, PayloadRef: last.PayloadRef, RawRef: last.RawRef, Truncated: last.Truncated}
			if last.Error != nil {
				row.LatestAttempt.ErrorCode = last.Error.Code
			}
		}
		if a.ID != "" && a.Status != "unavailable" && a.Status != "stale" {
			row.QueryCall = &QueryCall{Reference: QueryReference{CatalogID: e.Snapshot.ID, QueryID: q.ID}}
		}
		rows = append(rows, row)
	}
	return rows
}

// ExecutionGroup counts exact specifications, not events, captures or thread
// identities. Source-local members stay separate. Origin is part of the group
// so an observed subset can never become an expected-plan denominator.
type ExecutionGroup struct {
	ID            string             `json:"execution_group_id"`
	Specification QuerySpecification `json:"specification"`
	Origin        QueryOrigin        `json:"query_origin"`
	Queries       int                `json:"query_records"`
	Attempts      int                `json:"attempt_count"`
	Outcomes      map[Outcome]int    `json:"query_outcomes"`
}

func (e ExecutionView) SpecificationGroups() []ExecutionGroup {
	groups := map[string]*ExecutionGroup{}
	for _, q := range e.Snapshot.Queries {
		spec, err := querySpecification(q)
		if err != nil {
			continue
		} // Registration already validates canonical JSON.
		origin := e.QueryOrigins[q.ID]
		id := digest("execution_group_", struct {
			Specification QuerySpecification
			Origin        QueryOrigin
		}{spec, origin})
		if groups[id] == nil {
			groups[id] = &ExecutionGroup{ID: id, Specification: spec, Origin: origin, Outcomes: map[Outcome]int{}}
		}
		g := groups[id]
		g.Queries++
		g.Attempts += len(q.Attempts)
		g.Outcomes[q.Outcome]++
	}
	rows := make([]ExecutionGroup, 0, len(groups))
	for _, g := range groups {
		rows = append(rows, *g)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	return rows
}

// Shared with representative selection: only transport source/path are
// removed; target scope, every filter and exact window remain part of identity.
func querySpecification(q QueryRecord) (QuerySpecification, error) {
	args := map[string]json.RawMessage{}
	if err := json.Unmarshal(q.Parameters, &args); err != nil {
		return QuerySpecification{}, err
	}
	delete(args, "source")
	delete(args, "path")
	parameters, err := json.Marshal(args)
	if err != nil {
		return QuerySpecification{}, err
	}
	return cloneQuerySpecification(QuerySpecification{Object: q.Object, View: q.View, Window: q.Window, Parameters: parameters}), nil
}
