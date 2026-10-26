package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

const toolHistoryCurrentNavigationMaxBytes = 3 * 1024

// currentToolNavigation is a locator, not a result or a replayable permission.
// Producers may contribute only from current private tool state. In particular,
// neither persisted catalogs nor historical ToolResult JSON feed this carrier.
type currentToolNavigation struct {
	Tool      string            `json:"tool"`
	Arguments map[string]string `json:"arguments"`
}

func renderToolHistoryCurrentNavigationCheckpoint(ctx *types.AgentContext, budget int) string {
	if ctx == nil || ctx.Stage != types.StageExplore || ctx.Mutable == nil || budget <= 0 {
		return ""
	}
	var rows []currentToolNavigation
	// Keep the payload independent of selection, attempts, source contents and
	// publication paths. An ordinary status call retrieves those when needed;
	// the normal query path still owns source admission and execution authority.
	for _, catalog := range ctx.Mutable.TraceCatalogs() {
		if id := catalog.ID(); id != "" {
			rows = append(rows, currentToolNavigation{Tool: "trace_catalog", Arguments: map[string]string{"action": "status", "catalog_id": id}})
		}
	}
	if len(rows) == 0 {
		return ""
	}
	const header = "\nTool navigation handles retained at this checkpoint (navigation only, not current results or permissions). Reuse an applicable call instead of guessing identifiers; ordinary tool admission and source-generation checks still apply. No query or selection has been performed by this checkpoint.\n"
	footer := func(shown int) string {
		return fmt.Sprintf("tool_navigation_displayed=%d omitted=%d; omitted handles remain in live state, not reconstructed from history.\n", shown, len(rows)-shown)
	}
	if len(header)+len(footer(0)) > budget {
		const omitted = "Tool navigation omitted by checkpoint budget.\n"
		if len(omitted) <= budget {
			return omitted
		}
		return ""
	}
	var b strings.Builder
	b.WriteString(header)
	shown := 0
	for _, row := range rows {
		data, err := json.Marshal(row)
		if err != nil || b.Len()+len(data)+1+len(footer(shown+1)) > budget {
			continue
		}
		b.Write(data)
		b.WriteByte('\n')
		shown++
	}
	b.WriteString(footer(shown))
	return b.String()
}
