package agent

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

func buildWriteFailureObservationSection(ctx *types.AgentContext, consumer types.WriteContextConsumer) string {
	if ctx == nil || ctx.Mutable == nil || !ctx.Mode.IsWrite() {
		return ""
	}
	if ctx.Mutable.ChangeReport() != nil {
		// Reuse the established plan/channel selection. A present current
		// report with no observations supersedes the archival context; an
		// unrelated report must not trigger a fallback to historical text.
		report := authoritativeWriteControllerReport(ctx)
		if report == nil {
			return ""
		}
		body := types.RenderVerificationFailureObservations(types.CurrentReportFailureObservations(report), false)
		if body == "" {
			return ""
		}
		return fmt.Sprintf("## Supplementary check observations\n\nReport plan: %q. These observations do not change the project suite verdict.\n%s", report.PlanID, body)
	}
	pack := ctx.Mutable.WriteContextPack()
	if pack == nil {
		return ""
	}
	batchID, sliceID := activeWriteContextScope(ctx)
	planID := ""
	if plan := ctx.Mutable.ChangePlan(); plan != nil {
		planID = strings.TrimSpace(plan.ID)
	} else if run := ctx.Mutable.WriteWorkflowRun(); run != nil {
		for _, batch := range run.Batches {
			if batch.ID == batchID {
				planID = strings.TrimSpace(batch.PlanID)
				break
			}
		}
	}
	if planID == "" {
		return ""
	}
	view := pack.ViewForScope(consumer, 0, batchID, sliceID)
	for _, item := range view.Items {
		if item.Kind == "verification_failure_observation" && item.SourceStage == "verify" && strings.TrimSpace(item.SourceID) == planID {
			return fmt.Sprintf("## Supplementary check observations\n\nRetained historical observations from plan %q; later verification may have superseded them. They do not establish the current plan's failure or require a particular action.\n%s", planID, item.Text)
		}
	}
	return ""
}
