package context

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Status is deliberately separate from runtime observations. Persisted JSON
// cannot reach this path, and paths/labels remain quoted untrusted data.
func formatTraceCatalogs(ac *types.AgentContext) string {
	if ac == nil || ac.Mutable == nil || ac.Stage.IsWrite() || ac.Stage == types.StageAnalyze {
		return ""
	}
	var b strings.Builder
	b.WriteString(formatTraceCatalogWorkRequest(ac))
	catalogHeaderWritten := false
	for _, c := range ac.Mutable.TraceCatalogs() {
		if b.Len() >= (24<<10)-512 {
			b.WriteString("Additional catalog navigation omitted by the 24 KiB context budget; use trace_catalog status for the current catalogs.\n")
			break
		}
		execution := c.CurrentExecution()
		s := execution.Snapshot
		if !catalogHeaderWritten {
			b.WriteString("Capture/object query directory — navigation and last recorded execution status only. Window numbers are argument bounds; endpoint inclusion is unknown unless explicitly recorded, so use the actual query's boundary contract. Revalidate sources before reuse. It does not prove measurements, capture completeness, cross-capture clock alignment or causality. Use the actual query evidence for findings; do not count failed, stale or unexecuted work as zero.\n")
			b.WriteString(tracecatalog.ExecutionCoverageBoundary + "\n")
			catalogHeaderWritten = true
		}
		counts := map[string]int{}
		for _, q := range s.Queries {
			counts[string(q.Outcome)]++
		}
		countJSON, _ := json.Marshal(counts)
		coverageJSON, _ := json.Marshal(execution.Coverage)
		header := fmt.Sprintf("catalog_id=%q root=%q candidates=%d registered_query_records=%d discovery_complete=%t query_status_counts=%s\ncurrent_execution=%s\nrepresentative_selection_state=%q\n", s.ID, s.Root, len(s.Artifacts), len(s.Queries), s.Discovery.Complete, countJSON, coverageJSON, execution.SelectionState())
		if b.Len()+len(header) > (24<<10)-512 {
			b.WriteString("Additional catalog navigation omitted by the 24 KiB context budget; use trace_catalog status for the current catalogs.\n")
			break
		}
		b.WriteString(header)
		issueRows, issuesShown := 0, 0
		for _, a := range s.Artifacts {
			if len(a.Issues) > 0 {
				issueRows++
				row, _ := json.Marshal(map[string]any{"source": a.Path, "input_status": a.Status, "issues": a.Issues})
				if b.Len()+len(row)+1024 < 24<<10 {
					b.Write(row)
					b.WriteByte('\n')
					issuesShown++
				}
			}
		}
		if issueRows > issuesShown {
			fmt.Fprintf(&b, "input_issue_records_displayed=%d omitted=%d; see full catalog status.\n", issuesShown, issueRows-issuesShown)
		}
		if execution.Selection != nil {
			selection := *execution.Selection
			remaining := (24 << 10) - b.Len() - 256
			reserve := 0
			if len(selection.NewQueryIDs)+len(selection.ExcludedQueryIDs) > 0 {
				reserve = min(8<<10, remaining/3)
			}
			if section := formatTraceCatalogRepresentatives(execution, remaining-reserve); section != "" {
				b.WriteString(section)
			} else {
				fmt.Fprintf(&b, "Representative comparison omitted by context budget; use trace_catalog with action=representative_status and catalog_id=%q.\n", s.ID)
			}
			if b.Len() > 24<<10 {
				break
			}
			if len(selection.ExcludedQueryIDs) > 0 {
				ids := map[string]bool{}
				for _, id := range selection.ExcludedQueryIDs {
					ids[id] = true
				}
				budget := (24 << 10) - b.Len() - 256
				if len(selection.NewQueryIDs) > 0 {
					budget /= 2
				}
				b.WriteString(formatTraceCatalogQueryNavigation(execution, ids, budget, "queries_outside_selection_scope"))
			}
			if len(selection.NewQueryIDs) > 0 {
				ids := map[string]bool{}
				for _, id := range selection.NewQueryIDs {
					ids[id] = true
				}
				b.WriteString(formatTraceCatalogQueryNavigation(execution, ids, (24<<10)-b.Len()-256, "outside_selection_queries"))
			}
			continue
		}
		b.WriteString("No representative selection has occurred in this run. The exact specification groups below describe registered work, not selected representatives; discovery and direct queries do not perform selection.\n")
		b.WriteString(formatTraceCatalogExecutionGroups(execution, min(8<<10, ((24<<10)-b.Len()-256)/3)))
		b.WriteString(formatTraceCatalogQueryNavigation(execution, nil, (24<<10)-b.Len()-256, "query_records"))
		if len(s.Queries) == 0 {
			b.WriteString("No expected object queries were registered; discovery alone does not mean any candidate was analyzed.\n")
		}
		if b.Len() > 24<<10 {
			break
		}
	}
	return b.String()
}

// Accepted work is navigation intent, not a claim that any existing catalog
// already covers this task. It stays visible before discovery, while the
// existing current_execution/selection fields remain the sole status source.
func formatTraceCatalogWorkRequest(ac *types.AgentContext) string {
	rm := ac.Mutable.RequestModel()
	if rm == nil && ac.AnalysisIR != nil {
		rm = &ac.AnalysisIR.RequestModel
	}
	if rm == nil || !rm.RuntimeArtifactWorkProfile.Valid() {
		return ""
	}
	work := rm.RuntimeArtifactWorkProfile
	row, _ := json.Marshal(struct {
		Mode                  types.RuntimeArtifactWorkMode `json:"mode"`
		PerSpecificationLimit int                           `json:"per_specification_limit,omitempty"`
	}{work.Mode, work.PerSpecificationLimit})
	return "runtime_artifact_work_request=" + string(row) + "\n" +
		"This is accepted task intent, not plans, selection, completed queries or answer size. To plan directory work, explicitly choose an ordinary query view and call trace_catalog discover with request_plan:{view:...} and an authorized root. It reuses accepted user targets and a single common window without guessing them; missing inputs remain pending. Do not mix request_plan with explicit queries or representatives. Execute returned query_calls through trace_query and inspect actual status; ordinary exploratory queries remain allowed.\n"
}

func formatTraceCatalogExecutionGroups(e tracecatalog.ExecutionView, budget int) string {
	if budget < 512 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Exact specification groups count query records and attempts, not runtime events or completed captures; source identity remains local to every member.\n")
	groups, shown := e.SpecificationGroups(), 0
	for _, group := range groups {
		row, _ := json.Marshal(group)
		if shown >= 16 || b.Len()+len(row)+256 > budget {
			continue
		}
		b.Write(row)
		b.WriteByte('\n')
		shown++
	}
	fmt.Fprintf(&b, "execution_groups_displayed=%d omitted=%d; full current counts above are not reduced by this preview.\n", shown, len(groups)-shown)
	return b.String()
}

func formatTraceCatalogQueryNavigation(e tracecatalog.ExecutionView, ids map[string]bool, budget int, label string) string {
	if budget < 512 {
		if budget < 128 {
			return ""
		}
		return "Query navigation omitted by context budget; use trace_catalog status for exact current query calls.\n"
	}
	var b strings.Builder
	b.WriteString("query_call is unchanged trace_query arguments for normal admission, not automatic execution. Attempt history is summarized; full errors/effective parameters remain in trace_catalog status and the saved index.\n")
	total, shown := 0, 0
	for _, query := range e.NavigationQueries() {
		if ids != nil && !ids[query.Query.ID] {
			continue
		}
		total++
		row, _ := json.Marshal(query)
		// An oversized complete record must not evict later independent work.
		if shown >= 16 || b.Len()+len(row)+384 > budget {
			continue
		}
		b.Write(row)
		b.WriteByte('\n')
		shown++
	}
	fmt.Fprintf(&b, "%s_displayed=%d omitted=%d; trace_catalog(action=status,catalog_id=%q) pages the full directory.\n", label, shown, total-shown, e.Snapshot.ID)
	return b.String()
}
