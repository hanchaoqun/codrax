package orchestrator

import (
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

// This file holds the write controller's read-only exploration subflow:
// the explore_code action dispatches a read-pipeline exploration pass and
// projects its TurnA artifacts into the typed WriteExplorationHandoff +
// WriteContextPack that the planner consumes. It was extracted from the
// retired stage-II phase scheduler (PlanGroup lane) — these functions are
// the live remainder; sequential multi-phase work now runs as controller
// batches seeded from WriteAnalysisIR.PhaseProposal.

// extractPhaseGoalFromPrefix peels the "## Phase X of Y: " header
// off the seeded planning-hint prefix and returns the phase
// goal text. Used by dispatchStage(StagePlan) to bias the
// stage-3 pitfall ranking toward this phase's specific work
// (commit 40 P2). Empty input or non-matching shape returns
// empty so the caller can degrade silently.
func extractPhaseGoalFromPrefix(prefix string) string {
	if prefix == "" {
		return ""
	}
	const marker = "## Phase "
	idx := strings.Index(prefix, marker)
	if idx < 0 {
		return ""
	}
	tail := prefix[idx+len(marker):]
	// Skip past the "X of Y: " run by finding the first colon.
	colon := strings.Index(tail, ":")
	if colon < 0 {
		return ""
	}
	goal := strings.TrimSpace(tail[colon+1:])
	// Drop trailing newlines/whitespace; the seeded prefix
	// includes "\n\n" but the goal itself is the first line.
	if nl := strings.IndexByte(goal, '\n'); nl >= 0 {
		goal = strings.TrimSpace(goal[:nl])
	}
	return goal
}

// runWriteExplorationSubflow runs one read-only exploration dispatch for
// the active WriteExplorationRequest and projects the result into the
// typed handoff. It is the body of defaultReadExplorationRunner — the
// controller's explore_code action.
func (o *Orchestrator) runWriteExplorationSubflow() (int, error) {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return 0, nil
	}
	if o.agents == nil {
		return 0, nil
	}
	req := o.busCtx.Mutable.WriteExplorationRequest()
	if req == nil {
		return 0, nil
	}
	if !shouldRunWriteExplorationSubflow(*req) {
		return 0, nil
	}
	if existing := o.busCtx.Mutable.WriteExplorationHandoff(); existing != nil {
		return 0, nil
	}
	out, err := o.dispatchStage(types.StageExplore)
	o.projectWriteExplorationHandoffFromTurnA()
	if err != nil {
		return 1, err
	}
	if out == nil {
		return 0, nil
	}
	return 1, nil
}

func shouldRunWriteExplorationSubflow(req types.WriteExplorationRequest) bool {
	req = types.NormalizeWriteExplorationRequest(req)
	if req.Goal == "" && len(req.ExplorationQuestions) == 0 && len(req.CandidatePaths) == 0 {
		return false
	}
	batchID := req.BatchID
	if batchID == "" {
		batchID = "batch-1"
	}
	goal := req.Goal
	if goal == "" && len(req.ExplorationQuestions) > 0 {
		goal = req.ExplorationQuestions[0]
	}
	batch := writeflow.NormalizeBatchPlan(writeflow.WriteBatchPlan{
		ID:                   batchID,
		Goal:                 goal,
		Status:               writeflow.BatchNeedsExploration,
		NeedsCodeExploration: true,
		ExploreTargets:       append([]string(nil), req.CandidatePaths...),
		SuccessCriteria:      append([]string(nil), req.EvidenceRequirements...),
	})
	evaluation := writeflow.EvaluateWriteWorkflow(writeflow.EvaluationInput{
		Workflow: writeflow.WriteWorkflowPlan{
			Goal:            goal,
			Status:          writeflow.WorkflowInProgress,
			SuccessCriteria: append([]string(nil), req.EvidenceRequirements...),
			NextBatch:       &batch,
		},
		Batch:            &batch,
		RiskAssessment:   writeflow.RiskAssessment{Level: writeflow.RiskLow},
		ApprovalDecision: writeflow.ApprovalDecision{Action: writeflow.ApprovalActionAutoExecute, Policy: writeflow.ApprovalPolicyAutoSafe},
	})
	return evaluation.Status == writeflow.EvalContinueExplore
}

func (o *Orchestrator) projectWriteExplorationHandoffFromTurnA() {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return
	}
	req := o.busCtx.Mutable.WriteExplorationRequest()
	if req == nil {
		return
	}
	ta, ok := o.writeExplorationProjectionArtifacts()
	if !ok {
		return
	}
	handoff := types.WriteExplorationHandoffFromTurnA(*req, ta)
	if handoff.Goal == "" &&
		len(handoff.ExplorationQuestions) == 0 &&
		len(handoff.TargetFiles) == 0 &&
		len(handoff.RelevantSymbols) == 0 &&
		len(handoff.ExistingPatterns) == 0 &&
		len(handoff.Invariants) == 0 &&
		len(handoff.TestSurface) == 0 &&
		len(handoff.RiskNotes) == 0 &&
		len(handoff.Unknowns) == 0 &&
		len(handoff.EvidenceRefs) == 0 {
		return
	}
	o.busCtx.Mutable.SetWriteExplorationHandoff(&handoff)
	o.setWriteContextPackForBatch(req, &handoff)
}

func (o *Orchestrator) writeExplorationProjectionArtifacts() (types.TurnAArtifacts, bool) {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return types.TurnAArtifacts{}, false
	}
	var out types.TurnAArtifacts
	if ta := o.busCtx.Mutable.TurnAArtifacts(); ta != nil {
		out = *ta
	}
	if len(out.ReadFiles) == 0 {
		if closure := o.busCtx.Mutable.EvidenceClosure(); closure != nil {
			out.ReadFiles = append(out.ReadFiles, closure.CanonicalReadFiles()...)
		}
	}
	out.EvidenceItems = mergeWriteExplorationProjectionEvidence(out.EvidenceItems, o.busCtx.EvidenceItems, o.busCtx.Mutable.EmittedEvidence())
	if len(out.ReadFiles) == 0 &&
		len(out.EvidenceItems) == 0 &&
		len(out.FlowFindings) == 0 &&
		len(out.AcceptedAggregateFacts) == 0 &&
		len(out.ValidationBoundaryNotes) == 0 &&
		strings.TrimSpace(out.AcceptedClosureReason) == "" {
		return types.TurnAArtifacts{}, false
	}
	o.busCtx.Mutable.SetTurnAArtifacts(out)
	return out, true
}

func mergeWriteExplorationProjectionEvidence(groups ...[]types.EvidenceItem) []types.EvidenceItem {
	seen := map[string]bool{}
	var out []types.EvidenceItem
	for _, group := range groups {
		for _, item := range group {
			key := strings.TrimSpace(item.ID)
			if key == "" {
				key = strings.TrimSpace(item.Source) + "|" +
					strings.TrimSpace(item.Subject) + "|" +
					strings.TrimSpace(item.AnchorSymbol) + "|" +
					strings.TrimSpace(item.Summary)
				if item.LineStart > 0 {
					key += "|" + strconv.Itoa(item.LineStart)
				}
			}
			if strings.TrimSpace(key) == "" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, item)
		}
	}
	return out
}

func (o *Orchestrator) setWriteContextPackForBatch(req *types.WriteExplorationRequest, handoff *types.WriteExplorationHandoff) {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return
	}
	var packs []types.WriteContextPack
	if ir := o.busCtx.Mutable.WriteAnalysisIR(); ir != nil {
		packs = append(packs, types.WriteContextPackFromWriteAnalysisIR(ir))
	}
	if handoff != nil {
		packs = append(packs, types.WriteContextPackFromExplorationHandoff(*handoff))
	}
	if len(packs) == 0 {
		return
	}
	batchID := ""
	goal := ""
	if req != nil {
		batchID = req.BatchID
		goal = req.Goal
	}
	if handoff != nil {
		if batchID == "" {
			batchID = handoff.BatchID
		}
		if goal == "" {
			goal = handoff.Goal
		}
	}
	pack := types.MergeWriteContextPacks(batchID, goal, packs...)
	if len(pack.Items) == 0 {
		return
	}
	o.busCtx.Mutable.SetWriteContextPack(&pack)
}
