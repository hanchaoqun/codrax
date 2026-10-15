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
	header, _ := json.Marshal(map[string]any{"selection_id": s.SelectionID, "options": s.Options, "coverage": s.Coverage})
	if b.Len()+len(header)+512 > budget {
		return ""
	}
	b.Write(header)
	b.WriteByte('\n')
	groups, members := 0, 0
	for _, g := range s.Clusters {
		row, _ := json.Marshal(map[string]any{"cluster_id": g.ID, "specification": g.Specification, "coverage": g.Coverage, "budget_skipped": g.BudgetSkipped})
		if b.Len()+len(row)+512 > budget {
			break
		}
		b.Write(row)
		b.WriteByte('\n')
		groups++
		for _, m := range g.Members {
			if !m.Selected {
				continue
			}
			row, _ := json.Marshal(map[string]any{"cluster_id": g.ID, "selected_member": m})
			if b.Len()+len(row)+512 > budget {
				continue
			}
			b.Write(row)
			b.WriteByte('\n')
			members++
		}
	}
	call, _ := json.Marshal(map[string]any{"action": "representative_status", "catalog_id": s.CatalogID, "offset": 0})
	fmt.Fprintf(&b, "comparison_groups_displayed=%d omitted=%d; selected_member_records_displayed=%d omitted=%d. Full selected/unselected/source-skipped membership and unmatched candidates: trace_catalog arguments %s. Navigation display omissions never mean runtime absence.\n", groups, len(s.Clusters)-groups, members, s.Coverage.Selected-members, call)
	return b.String()
}
