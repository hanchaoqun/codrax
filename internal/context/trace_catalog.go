package context

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Status is deliberately separate from runtime observations. Persisted JSON
// cannot reach this path, and paths/labels remain quoted untrusted data.
func formatTraceCatalogs(ac *types.AgentContext) string {
	if ac == nil || ac.Mutable == nil || ac.Stage.IsWrite() || ac.Stage == types.StageAnalyze {
		return ""
	}
	var b strings.Builder
	for _, c := range ac.Mutable.TraceCatalogs() {
		s := c.Snapshot()
		if b.Len() == 0 {
			b.WriteString("Capture/object query directory — navigation and last recorded execution status only. Window numbers are argument bounds; endpoint inclusion is unknown unless explicitly recorded, so use the actual query's boundary contract. Revalidate sources before reuse. It does not prove measurements, capture completeness, cross-capture clock alignment or causality. Use the actual query evidence for findings; do not count failed, stale or unexecuted work as zero.\n")
		}
		counts := map[string]int{}
		for _, q := range s.Queries {
			counts[string(q.Outcome)]++
		}
		countJSON, _ := json.Marshal(counts)
		fmt.Fprintf(&b, "catalog_id=%q root=%q candidates=%d planned_queries=%d discovery_complete=%t query_status_counts=%s\n", s.ID, s.Root, len(s.Artifacts), len(s.Queries), s.Discovery.Complete, countJSON)
		paths := map[string]string{}
		for _, a := range s.Artifacts {
			paths[a.ID] = a.Path
		}
		shown := 0
		for _, q := range s.Queries {
			if shown >= 16 {
				break
			}
			row, _ := json.Marshal(map[string]any{"source": paths[q.ArtifactID], "query": q})
			if b.Len()+len(row) > 24<<10 {
				break
			}
			b.Write(row)
			b.WriteByte('\n')
			shown++
		}
		fmt.Fprintf(&b, "query_records_displayed=%d omitted=%d; trace_catalog(action=status,catalog_id=%q) pages the full directory.\n", shown, len(s.Queries)-shown, s.ID)
		if len(s.Queries) == 0 {
			b.WriteString("No expected object queries were registered; discovery alone does not mean any candidate was analyzed.\n")
		}
		if b.Len() > 24<<10 {
			break
		}
	}
	return b.String()
}
