package tracecatalog

// RepresentativeQueryNavigation makes each member self-contained without
// changing the saved selection or replaying its unbounded attempt transcript.
// Outcomes count every actual attempt; ordered attempts and original payloads
// remain in the same-generation saved index. This is navigation, not authority.
type RepresentativeQueryNavigation struct {
	Specification   QuerySpecification `json:"specification"`
	Origin          QueryOrigin        `json:"query_origin"`
	AttemptOutcomes map[Outcome]int    `json:"attempt_outcomes"`
}

// RepresentativeNavigation joins only exact query identities from this one
// detached execution view. Similar specifications, paths or object labels are
// never enough to borrow another source's execution history. Inconsistent
// detached inputs are omitted rather than silently joining different epochs.
func (e ExecutionView) RepresentativeNavigation() map[string]RepresentativeQueryNavigation {
	rows := map[string]RepresentativeQueryNavigation{}
	if e.Selection == nil || e.Selection.CatalogID != e.Snapshot.ID {
		return rows
	}
	queries := map[string]CompactQueryNavigation{}
	for _, q := range e.NavigationQueries() {
		queries[q.Query.ID] = q
	}
	artifacts := map[string]Artifact{}
	for _, a := range e.Snapshot.Artifacts {
		artifacts[a.ID] = a
	}
	for _, g := range e.Selection.Clusters {
		for _, m := range g.Members {
			q, ok := queries[m.QueryID]
			a := artifacts[m.ArtifactID]
			if !ok || a.ID == "" || q.Query.ArtifactID != m.ArtifactID || q.Query.ArtifactRevision != m.ArtifactRevision || q.Source != m.Path || a.SourceRevision != m.SourceRevision || q.SourceStatus != m.SourceStatus || q.Query.Outcome != m.Outcome || q.Attempts != m.Attempts {
				continue
			}
			if (q.LatestAttempt == nil) != (m.LatestAttempt == nil) || (q.LatestAttempt != nil && q.LatestAttempt.ID != m.LatestAttempt.ID) {
				continue
			}
			spec, err := querySpecification(q.Query)
			if err != nil || digest("", spec) != digest("", g.Specification) {
				continue
			}
			rows[m.QueryID] = RepresentativeQueryNavigation{Specification: spec, Origin: q.Origin, AttemptOutcomes: q.AttemptOutcomes}
		}
	}
	return rows
}
