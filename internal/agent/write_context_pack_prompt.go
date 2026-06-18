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
	batchID, sliceID := activeWriteContextScope(ctx)
	view := pack.ViewForScope(consumer, limit, batchID, sliceID)
	if len(view.Items) == 0 {
		return ""
	}
	if strings.TrimSpace(title) == "" {
		title = "Priority write context pack"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## %s\n\n", title)
	b.WriteString("Prioritized typed handoff for this write batch. Treat it as planning and verification context only; hard gates still read ChangePlan, ChangeReport, approval records, and other typed artifacts directly.\n")
	if note := writeContextPackConsumerGuidance(view); note != "" {
		b.WriteString(note)
		b.WriteByte('\n')
	}
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
	if view.DroppedItems > 0 {
		fmt.Fprintf(&b, "  - ... +%d more context item(s)\n", view.DroppedItems)
	}
	return strings.TrimSpace(b.String())
}

func activeWriteContextScope(ctx *types.AgentContext) (string, string) {
	if ctx == nil || ctx.Mutable == nil {
		return "", ""
	}
	run := ctx.Mutable.WriteWorkflowRun()
	if run == nil {
		return "", ""
	}
	batchID := strings.TrimSpace(run.ActiveBatchID)
	if batchID == "" {
		return "", ""
	}
	for _, batch := range run.Batches {
		if strings.TrimSpace(batch.ID) != batchID {
			continue
		}
		return batchID, strings.TrimSpace(batch.ActiveSliceID)
	}
	return batchID, ""
}

func writeContextPackConsumerGuidance(view types.WriteContextView) string {
	switch view.Consumer {
	case types.WriteConsumerPlanner:
		hasCoverageItem := false
		for _, item := range view.Items {
			if item.Priority != types.WriteContextP0 && item.Priority != types.WriteContextP1 {
				continue
			}
			switch item.Kind {
			case "constraint", "risk_note", "scope_anchor", "localization_anchor", "source_localization_review", "source_localization_missing_path", "target_file", "evidence_ref", "invariant", "success_criterion":
				hasCoverageItem = true
			}
			if hasCoverageItem {
				break
			}
		}
		if !hasCoverageItem {
			return ""
		}
		return "Planner coverage guidance (soft): keep P0 constraints and P1 scope/target/evidence items visible while choosing the bounded ChangePlan. The plan may modify only the smallest correct subset, but any unmodified production surface named by those typed rows should still be covered by verification_probes/test_surface or carried forward as an explicit uncertainty. Do not turn this guidance into a hard gate; validation, approval, and verifier verdicts remain typed-artifact decisions."
	default:
		return ""
	}
}
