package context

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracecatalog"
)

// This comparison is navigation/status, never measured values. Keep each
// source-local member and its exact original result references separate.
func formatTraceCatalogRepresentatives(s tracecatalog.RepresentativeSnapshot, budget int) string {
	if budget < 1024 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Representative comparison — fixed query-plan selection, soft navigation priority only. Equal specifications do not establish common physical identity, clock, device or cause. Unselected or skipped members remain unassessed; empty, failure and pending are not interchangeable. Read actual query evidence for measurements.\n")
	header, _ := json.Marshal(map[string]any{"selection_id": s.SelectionID, "selection_scope": s.SelectionScope, "options": s.Options, "coverage": s.Coverage})
	if b.Len()+len(header)+1024 > budget {
		return ""
	}
	b.Write(header)
	b.WriteByte('\n')
	if len(s.ExcludedQueryIDs) > 0 {
		fmt.Fprintf(&b, "queries_outside_selection_scope=%d were already registered but excluded by the explicit selection scope; they remain executable and their outcomes are unchanged, not unselected members or newly discovered work.\n", len(s.ExcludedQueryIDs))
	}
	groups, members := 0, 0
	for _, g := range s.Clusters {
		row, _ := json.Marshal(map[string]any{"cluster_id": g.ID, "specification": g.Specification, "coverage": g.Coverage, "budget_skipped": g.BudgetSkipped})
		if b.Len()+len(row)+1024 > budget {
			continue
		}
		b.Write(row)
		b.WriteByte('\n')
		groups++
		for _, m := range g.Members {
			if !m.Selected {
				continue
			}
			rowData := map[string]any{"cluster_id": g.ID, "selected_member": m}
			if m.SourceStatus != "stale" && m.SourceStatus != "unavailable" {
				rowData["query_call"] = tracecatalog.QueryCall{Reference: tracecatalog.QueryReference{CatalogID: s.CatalogID, QueryID: m.QueryID}}
			}
			row, _ := json.Marshal(rowData)
			if b.Len()+len(row)+1024 > budget {
				continue
			}
			b.Write(row)
			b.WriteByte('\n')
			members++
		}
	}
	// Keep unselected members visible as pending/failure/etc., never silently
	// turn the fixed representative subset into the whole execution roster.
	unselected, unselectedShown := 0, 0
	for _, g := range s.Clusters {
		for _, m := range g.Members {
			if m.Selected {
				continue
			}
			unselected++
			row, _ := json.Marshal(map[string]any{"cluster_id": g.ID, "unselected_member": map[string]any{"query_id": m.QueryID, "path": m.Path, "source_status": m.SourceStatus, "selection_reason": m.SelectionReason, "outcome": m.Outcome, "attempt_count": m.Attempts}})
			if unselectedShown >= 16 || b.Len()+len(row)+1024 > budget {
				continue
			}
			b.Write(row)
			b.WriteByte('\n')
			unselectedShown++
		}
	}
	fmt.Fprintf(&b, "unselected_member_records_displayed=%d omitted=%d; unselected is not unexecuted, empty or normal.\n", unselectedShown, unselected-unselectedShown)
	call, _ := json.Marshal(map[string]any{"action": "representative_status", "catalog_id": s.CatalogID, "offset": 0})
	fmt.Fprintf(&b, "comparison_groups_displayed=%d omitted=%d; selected_member_records_displayed=%d omitted=%d. Full selected/unselected/source-skipped membership and unmatched candidates: trace_catalog arguments %s. Navigation display omissions never mean runtime absence.\n", groups, len(s.Clusters)-groups, members, s.Coverage.Selected-members, call)
	return b.String()
}
