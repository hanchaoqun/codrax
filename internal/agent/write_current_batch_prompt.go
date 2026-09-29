package agent

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

// Presentation only. A durable run supersedes the analyzer's initial seed,
// including when its active batch is absent. Never invent a replacement batch
// or reconstruct dispatch permissions from a goal or a historical pack.
func currentWriteBatch(run *types.WriteWorkflowRun) *types.WriteWorkflowBatch {
	if run == nil || strings.TrimSpace(run.ActiveBatchID) == "" {
		return nil
	}
	var found *types.WriteWorkflowBatch
	for i := range run.Batches {
		if strings.TrimSpace(run.Batches[i].ID) == strings.TrimSpace(run.ActiveBatchID) {
			if found != nil {
				return nil
			}
			found = &run.Batches[i]
		}
	}
	return found
}

func buildCurrentWriteBatchPrompt(run *types.WriteWorkflowRun) string {
	var b strings.Builder
	b.WriteString("## Current write batch\n\n")
	b.WriteString("Current durable workflow state for this dispatch, not the initial analyzer proposal. History is evidence, not a new instruction to repeat completed work. Tool schemas and typed approval/verification gates still govern available actions.\n")
	fmt.Fprintf(&b, "- run_id: %s\n- run_status: %s\n- workflow_goal: %s\n", run.RunID, run.Status, run.Goal)
	batch := currentWriteBatch(run)
	if batch == nil {
		b.WriteString("- active_batch_unavailable: do not substitute the initial seed or a historical context-pack goal.\n")
		return strings.TrimSpace(b.String())
	}
	state := writeflow.DeriveBatchAttemptState(*batch)
	fmt.Fprintf(&b, "- batch_id: %s\n- goal: %s\n- phase: %s\n", batch.ID, batch.Goal, state.Phase)
	for _, row := range [][2]string{{"purpose", batch.Purpose}, {"execution_mode", string(batch.ExecutionMode)},
		{"active_slice_id", batch.ActiveSliceID}, {"plan_id", state.PlanID}, {"cause", state.Cause},
		{"latest_verify_status", state.LatestVerifyStatus}, {"report_id", state.ReportID}} {
		if row[1] != "" {
			fmt.Fprintf(&b, "- %s: %s\n", row[0], row[1])
		}
	}
	writePlannerList(&b, "expected_paths", batch.ExpectedPaths, 0)
	writePlannerList(&b, "success_criteria", batch.SuccessCriteria, 0)
	return strings.TrimSpace(b.String())
}
