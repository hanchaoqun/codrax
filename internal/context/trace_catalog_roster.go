package context

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/hanchaoqun/codrax/internal/tracecatalog"
)

// Display-only dictionary encoding. Neither this type nor its local indices can
// restore catalog authority. Share exact source tuples/specifications, never
// source-local query identities or measured values. The old self-contained rows
// remain preferred whenever they already fit; this whole-roster fallback only
// removes repetition when that prevents an otherwise avoidable omission.
type representativeRosterSource struct {
	ArtifactID       string `json:"artifact_id"`
	Path             string `json:"path"`
	ArtifactRevision string `json:"artifact_revision"`
	SourceRevision   string `json:"source_revision,omitempty"`
	SourceStatus     string `json:"source_status"`
}

type representativeRosterGroup struct {
	ID            string                                    `json:"cluster_id"`
	Specification tracecatalog.QuerySpecification           `json:"specification"`
	Coverage      tracecatalog.RepresentativeMemberCoverage `json:"coverage"`
	BudgetSkipped bool                                      `json:"budget_skipped"`
}

type representativeRosterMember struct {
	GroupIndex      int                          `json:"group_index"`
	SourceIndex     int                          `json:"source_index"`
	QueryID         string                       `json:"query_id"`
	Selected        bool                         `json:"selected"`
	SelectionReason string                       `json:"selection_reason"`
	Outcome         tracecatalog.Outcome         `json:"outcome"`
	Attempts        int                          `json:"attempt_count"`
	LatestAttempt   *tracecatalog.Attempt        `json:"latest_attempt,omitempty"`
	Origin          tracecatalog.QueryOrigin     `json:"query_origin"`
	AttemptOutcomes map[tracecatalog.Outcome]int `json:"attempt_outcomes"`
}

func formatCompleteRepresentativeRoster(s tracecatalog.RepresentativeSnapshot, navigation map[string]tracecatalog.RepresentativeQueryNavigation, intro string, budget int) string {
	roster := struct {
		Sources []representativeRosterSource `json:"sources"`
		Groups  []representativeRosterGroup  `json:"groups"`
		Members []representativeRosterMember `json:"members"`
	}{}
	sources, queries := map[representativeRosterSource]int{}, map[string]bool{}
	selected, unselected := 0, 0
	for gi, g := range s.Clusters {
		roster.Groups = append(roster.Groups, representativeRosterGroup{g.ID, g.Specification, g.Coverage, g.BudgetSkipped})
		for _, m := range g.Members {
			n, ok := navigation[m.QueryID]
			// Reuse the exact live-generation join, not label/path similarity.
			// An inconsistent detached view must not acquire a completeness claim.
			if !ok || queries[m.QueryID] || !reflect.DeepEqual(n.Specification, g.Specification) {
				return ""
			}
			queries[m.QueryID] = true
			source := representativeRosterSource{m.ArtifactID, m.Path, m.ArtifactRevision, m.SourceRevision, m.SourceStatus}
			si, exists := sources[source]
			if !exists {
				si = len(roster.Sources)
				sources[source] = si
				roster.Sources = append(roster.Sources, source)
			}
			roster.Members = append(roster.Members, representativeRosterMember{gi, si, m.QueryID, m.Selected, m.SelectionReason, m.Outcome, m.Attempts, m.LatestAttempt, n.Origin, n.AttemptOutcomes})
			if m.Selected {
				selected++
			} else {
				unselected++
			}
		}
	}
	if len(roster.Members) != s.Coverage.Members || selected != s.Coverage.Selected {
		return ""
	}
	row, err := json.Marshal(struct {
		CatalogID string `json:"catalog_id"`
		Roster    any    `json:"representative_roster"`
	}{s.CatalogID, roster})
	if err != nil {
		return ""
	}
	call, _ := json.Marshal(map[string]any{"action": "representative_status", "catalog_id": s.CatalogID, "offset": 0})
	teaching := "Complete selection-member roster below uses zero-based group_index/source_index into its groups/sources dictionaries; attach those exact fields to each member. This is complete only for this fixed selection's member population, not unmatched captures, later/out-of-scope queries, measurements or diagnosis. query_id plus the catalog_id is an ordinary trace_query catalog_query_ref, subject to normal admission; stale/unavailable sources remain ineligible.\n"
	footer := fmt.Sprintf("unselected_member_records_displayed=%d omitted=0; unselected is not unexecuted, empty or normal.\ncomparison_groups_displayed=%d omitted=0; selected_member_records_displayed=%d omitted=0. Full source-skipped membership and unmatched candidates: trace_catalog arguments %s.\n", unselected, len(roster.Groups), selected, call)
	if len(intro)+len(teaching)+len(row)+1+len(footer) > budget {
		return ""
	}
	return intro + teaching + string(row) + "\n" + footer
}
