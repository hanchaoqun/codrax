package agent

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

func buildWriteContextPackPromptSection(ctx *types.AgentContext, consumer types.WriteContextConsumer, title string, limit int) string {
	if ctx == nil || ctx.Mutable == nil {
		return ""
	}
	pack := ctx.Mutable.WriteContextPack()
	if pack == nil {
		return ""
	}
	view := pack.View(consumer, limit)
	if len(view.Items) == 0 {
		return ""
	}
	if strings.TrimSpace(title) == "" {
		title = "Priority write context pack"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## %s\n\n", title)
	b.WriteString("Prioritized typed handoff for this write batch. Treat it as planning and verification context only; hard gates still read ChangePlan, ChangeReport, approval records, and other typed artifacts directly.\n")
	if pack.BatchID != "" {
		fmt.Fprintf(&b, "- batch_id: %s\n", pack.BatchID)
	}
	if pack.Goal != "" {
		fmt.Fprintf(&b, "- goal: %s\n", pack.Goal)
	}
	b.WriteString("- items:\n")
	for _, item := range view.Items {
		source := item.SourceStage
		if item.SourceID != "" {
			if source != "" {
				source += "/" + item.SourceID
			} else {
				source = item.SourceID
			}
		}
		label := fmt.Sprintf("%s %s", item.Priority, item.Kind)
		if source != "" {
			label += " [" + source + "]"
		}
		fmt.Fprintf(&b, "  - %s: %s\n", label, item.Text)
	}
	if limit > 0 {
		all := pack.View(consumer, 0)
		if len(all.Items) > len(view.Items) {
			fmt.Fprintf(&b, "  - ... +%d more context item(s)\n", len(all.Items)-len(view.Items))
		}
	}
	return strings.TrimSpace(b.String())
}
