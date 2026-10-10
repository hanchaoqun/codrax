package context

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

func formatAttachedLogContext(ac *types.AgentContext, availableTools map[string]bool) string {
	if ac.AttachedLogCatalog == nil {
		return formatAttachedLog(ac.AttachedLog, ac.WorkDir, attachedLogTriageState(ac), preStageDegradationSummaryFor(ac, types.StageLogTriage), attachedArtifactRenderOptions{ReadFileAvailable: availableTools["read_file"]})
	}
	var b strings.Builder
	b.WriteString("Attached logs retain separate complete sources. The text below is a bounded preview, not a complete capture; preview line numbers are not physical evidence coordinates. Missing preview rows do not establish absence. Source metadata describes preparation, not a query or a causal conclusion.\n")
	if availableTools["log_query"] {
		b.WriteString("Use log_query to inspect complete attached sources; its source IDs, physical line ranges, raw records and coverage are the evidence coordinates.\n")
	} else {
		b.WriteString("This stage has no log query tool. Use only visible observations; leave full-source collection to a later query-capable stage.\n")
	}
	b.WriteString("Keep source clocks separate: a missing year/timezone stays unknown, and recorded boot time is not an automatic trace clock mapping.\n")
	sources := ac.AttachedLogCatalog.Sources()
	const maxSources = 12
	const metadataBytes = 8 << 10
	shown, bytes := 0, 0
	for _, source := range sources {
		row, err := json.Marshal(source)
		if err != nil || shown >= maxSources || bytes+len(row) > metadataBytes {
			break
		}
		b.Write(row)
		b.WriteByte('\n')
		shown++
		bytes += len(row)
	}
	fmt.Fprintf(&b, "Source metadata shown: %d/%d; omitted sources remain available to log_query.\n", shown, len(sources))
	if shouldSuppressAttachedRuntimeLog(ac) {
		return b.String()
	}
	if strings.TrimSpace(ac.AttachedLog) != "" {
		b.WriteString("\nPreview only:\n")
		if len(ac.AttachedLog) <= attachedLogInlineCap {
			b.WriteString("```text\n" + ac.AttachedLog + "\n```\n")
		} else {
			preview := buildAttachedArtifactPreview(ac.AttachedLog)
			b.WriteString("```text\n" + preview.head + "\n... [preview omitted] ...\n" + preview.tail + "\n```\n")
		}
	}
	return b.String()
}
