package writeflow

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// WorkflowStatus is the coarse lifecycle of a write workflow.
type WorkflowStatus string

const (
	WorkflowPlanned            WorkflowStatus = "planned"
	WorkflowInProgress         WorkflowStatus = "in_progress"
	WorkflowNeedsClarification WorkflowStatus = "needs_clarification"
	WorkflowBlocked            WorkflowStatus = "blocked"
	WorkflowComplete           WorkflowStatus = "complete"
)

// BatchStatus is the lifecycle of one rolling batch inside a workflow.
type BatchStatus string

const (
	BatchPending            BatchStatus = "pending"
	BatchNeedsExploration   BatchStatus = "needs_exploration"
	BatchReadyForChangePlan BatchStatus = "ready_for_change_plan"
	BatchApplied            BatchStatus = "applied"
	BatchNeedsRepair        BatchStatus = "needs_repair"
	BatchBlocked            BatchStatus = "blocked"
	BatchComplete           BatchStatus = "complete"
)

// WriteWorkflowPlan is the high-level, rolling workflow envelope for write
// mode. It does not replace ChangePlan; it decides what the next bounded batch
// should learn or change before the existing planner/coder/verifier machinery
// does its normal work.
type WriteWorkflowPlan struct {
	Goal                string           `json:"goal"`
	KnownConstraints    []string         `json:"known_constraints,omitempty"`
	MissingObservations []string         `json:"missing_observations,omitempty"`
	SuccessCriteria     []string         `json:"success_criteria,omitempty"`
	Status              WorkflowStatus   `json:"status,omitempty"`
	Strategy            string           `json:"strategy,omitempty"`
	MaxBatches          int              `json:"max_batches,omitempty"`
	CompletedBatches    []string         `json:"completed_batches,omitempty"`
	NextBatch           *WriteBatchPlan  `json:"next_batch,omitempty"`
	PlannedBatches      []WriteBatchPlan `json:"planned_batches,omitempty"`
}

// WriteBatchPlan describes one bounded exploration/change/verification batch.
// The model should unfold only the next useful batch, not an exhaustive
// one-shot implementation plan for the whole user request.
type WriteBatchPlan struct {
	ID                   string      `json:"id,omitempty"`
	Goal                 string      `json:"goal"`
	Purpose              string      `json:"purpose,omitempty"`
	Status               BatchStatus `json:"status,omitempty"`
	WhyThisBatch         string      `json:"why_this_batch,omitempty"`
	NeedsCodeExploration bool        `json:"needs_code_exploration,omitempty"`
	ExploreTargets       []string    `json:"explore_targets,omitempty"`
	ExpectedChangeKinds  []string    `json:"expected_change_kinds,omitempty"`
	ExpectedPaths        []string    `json:"expected_paths,omitempty"`
	SuccessCriteria      []string    `json:"success_criteria,omitempty"`
	DependsOn            []string    `json:"depends_on,omitempty"`
}

// NormalizeWorkflowPlan trims and defaults a workflow plan after schema-level
// JSON compatibility repair. It never invents goal content or success criteria.
func NormalizeWorkflowPlan(plan WriteWorkflowPlan) WriteWorkflowPlan {
	plan.Goal = strings.TrimSpace(plan.Goal)
	plan.Status = normalizeWorkflowStatus(plan.Status)
	plan.Strategy = strings.TrimSpace(plan.Strategy)
	if plan.MaxBatches < 0 {
		plan.MaxBatches = 0
	}
	plan.KnownConstraints = dedupTrimWorkflowStrings(plan.KnownConstraints)
	plan.MissingObservations = dedupTrimWorkflowStrings(plan.MissingObservations)
	plan.SuccessCriteria = dedupTrimWorkflowStrings(plan.SuccessCriteria)
	plan.CompletedBatches = dedupTrimWorkflowStrings(plan.CompletedBatches)
	if plan.NextBatch != nil {
		batch := NormalizeBatchPlan(*plan.NextBatch)
		if batch.Goal == "" {
			plan.NextBatch = nil
		} else {
			plan.NextBatch = &batch
		}
	}
	out := make([]WriteBatchPlan, 0, len(plan.PlannedBatches))
	for _, batch := range plan.PlannedBatches {
		batch = NormalizeBatchPlan(batch)
		if batch.Goal != "" {
			out = append(out, batch)
		}
	}
	plan.PlannedBatches = out
	return plan
}

// NormalizeBatchPlan trims and defaults one batch.
func NormalizeBatchPlan(batch WriteBatchPlan) WriteBatchPlan {
	batch.ID = strings.TrimSpace(batch.ID)
	batch.Goal = strings.TrimSpace(batch.Goal)
	batch.Purpose = strings.TrimSpace(batch.Purpose)
	batch.Status = normalizeBatchStatus(batch.Status)
	batch.WhyThisBatch = strings.TrimSpace(batch.WhyThisBatch)
	batch.ExploreTargets = dedupTrimWorkflowStrings(batch.ExploreTargets)
	batch.ExpectedChangeKinds = dedupTrimWorkflowStrings(batch.ExpectedChangeKinds)
	batch.ExpectedPaths = dedupTrimWorkflowStrings(batch.ExpectedPaths)
	batch.SuccessCriteria = dedupTrimWorkflowStrings(batch.SuccessCriteria)
	batch.DependsOn = dedupTrimWorkflowStrings(batch.DependsOn)
	return batch
}

// ValidateWorkflowPlan returns deterministic structural errors. It is intended
// for typed tool rejection/hints once the workflow emitter is introduced.
func ValidateWorkflowPlan(plan WriteWorkflowPlan) []string {
	var errs []string
	plan = NormalizeWorkflowPlan(plan)
	if plan.Goal == "" {
		errs = append(errs, "goal is required")
	}
	if len(plan.SuccessCriteria) == 0 {
		errs = append(errs, "success_criteria must contain at least one concrete criterion")
	}
	if plan.Status == WorkflowNeedsClarification && len(plan.MissingObservations) == 0 {
		errs = append(errs, "needs_clarification requires missing_observations")
	}
	if plan.Status != WorkflowComplete && plan.Status != WorkflowBlocked && plan.Status != WorkflowNeedsClarification && plan.NextBatch == nil {
		errs = append(errs, "next_batch is required unless the workflow is complete, blocked, or needs clarification")
	}
	if plan.NextBatch != nil && strings.TrimSpace(plan.NextBatch.Goal) == "" {
		errs = append(errs, "next_batch.goal is required")
	}
	return errs
}

// WorkflowSeedFromWriteAnalysis projects the existing write analyzer output
// into a rolling workflow seed. It is a compatibility bridge: current write
// mode already emits PhaseProposal, but the future workflow controller needs a
// next-batch envelope instead of a static all-at-once phase list.
func WorkflowSeedFromWriteAnalysis(ir *types.WriteAnalysisIR) WriteWorkflowPlan {
	if ir == nil {
		return WriteWorkflowPlan{}
	}
	goal := strings.TrimSpace(ir.Request.Task.Summary)
	if goal == "" {
		goal = strings.TrimSpace(ir.Request.RawRequest)
	}
	constraints := make([]string, 0, len(ir.Request.Constraints))
	for _, c := range ir.Request.Constraints {
		constraints = append(constraints, renderConstraintSeed(c))
	}
	plan := WriteWorkflowPlan{
		Goal:             goal,
		KnownConstraints: constraints,
		SuccessCriteria:  dedupTrimWorkflowStrings(ir.Request.ExpectedOutcomes),
		Status:           WorkflowPlanned,
		Strategy:         "rolling_batches",
	}
	if len(ir.PhaseProposal.Phases) > 0 && (ir.PhaseProposal.Split == "sequential" || ir.PhaseProposal.Split == "parallel") {
		plan.MaxBatches = len(ir.PhaseProposal.Phases)
		plan.PlannedBatches = make([]WriteBatchPlan, 0, len(ir.PhaseProposal.Phases))
		for i, phase := range ir.PhaseProposal.Phases {
			batch := WriteBatchPlan{
				ID:                   fmt.Sprintf("batch-%d", i+1),
				Goal:                 strings.TrimSpace(phase.Goal),
				Status:               BatchNeedsExploration,
				NeedsCodeExploration: true,
				ExploreTargets:       dedupTrimWorkflowStrings(phase.RoughTargetPaths),
				ExpectedPaths:        dedupTrimWorkflowStrings(phase.RoughTargetPaths),
				SuccessCriteria:      plan.SuccessCriteria,
			}
			batch = NormalizeBatchPlan(batch)
			if batch.Goal == "" {
				continue
			}
			if plan.NextBatch == nil {
				next := batch
				plan.NextBatch = &next
			}
			plan.PlannedBatches = append(plan.PlannedBatches, batch)
		}
		return NormalizeWorkflowPlan(plan)
	}
	plan.MaxBatches = 1
	next := WriteBatchPlan{
		ID:                   "batch-1",
		Goal:                 goal,
		Status:               BatchNeedsExploration,
		NeedsCodeExploration: true,
		ExploreTargets:       dedupTrimWorkflowStrings(ir.Request.ScopeAnchors),
		ExpectedPaths:        dedupTrimWorkflowStrings(ir.Request.ScopeAnchors),
		SuccessCriteria:      plan.SuccessCriteria,
	}
	// Typed short path for micro-scope tasks with known anchors: the
	// analyzer already classified the change as contained (Scope=micro,
	// typed enum) and extracted concrete scope anchors, so the batch seeds
	// ready-for-plan instead of needs-exploration. This optimizes round
	// count over the SAME controller DAG — explore_code stays available in
	// the action schema and the controller may still choose it; only the
	// typed starting state changes.
	if ir.Request.Task.Scope == types.ScopeMicro && len(next.ExpectedPaths) > 0 {
		next.Status = BatchReadyForChangePlan
		next.NeedsCodeExploration = false
	}
	plan.NextBatch = &next
	return NormalizeWorkflowPlan(plan)
}

// WriteWorkflowPlanSchema returns the model-facing JSON schema for the future
// workflow emitter. It is kept near the Go structs so schema-compatible repair
// tests can cover new fields before the tool exists.
func WriteWorkflowPlanSchema() json.RawMessage {
	raw, err := json.Marshal(map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"goal":                 stringProp("The user's code-change goal, restated as an outcome."),
			"known_constraints":    stringArrayProp("User constraints and already established technical constraints."),
			"missing_observations": stringArrayProp("Information still needed before continuing, if any."),
			"success_criteria":     stringArrayProp("Concrete criteria that tell the workflow it is done."),
			"status": enumStringProp([]string{
				string(WorkflowPlanned),
				string(WorkflowInProgress),
				string(WorkflowNeedsClarification),
				string(WorkflowBlocked),
				string(WorkflowComplete),
			}),
			"strategy":          stringProp("Short explanation of the rolling workflow strategy."),
			"max_batches":       map[string]any{"type": "integer", "minimum": 0},
			"completed_batches": stringArrayProp("IDs of batches already completed in this workflow."),
			"next_batch":        batchPlanSchema(),
			"planned_batches": map[string]any{
				"type":        "array",
				"description": "Optional coarse milestones. Do not over-plan every implementation step.",
				"items":       batchPlanSchema(),
			},
		},
		"required": []string{"goal", "success_criteria"},
	})
	if err != nil {
		return json.RawMessage(fmt.Sprintf(`{"type":"object","description":"write workflow schema build failed: %s"}`, err))
	}
	return raw
}

func batchPlanSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"id":                     stringProp("Stable batch id, such as batch-1."),
			"goal":                   stringProp("The bounded goal for this next batch."),
			"purpose":                stringProp("What this batch is trying to learn or change."),
			"status":                 enumStringProp([]string{string(BatchPending), string(BatchNeedsExploration), string(BatchReadyForChangePlan), string(BatchApplied), string(BatchNeedsRepair), string(BatchBlocked), string(BatchComplete)}),
			"why_this_batch":         stringProp("Why this batch is the right next step."),
			"needs_code_exploration": map[string]any{"type": "boolean"},
			"explore_targets":        stringArrayProp("Files, packages, symbols, or test surfaces to inspect in this batch."),
			"expected_change_kinds":  stringArrayProp("Expected change kinds, e.g. modify tests, patch implementation, update docs."),
			"expected_paths":         stringArrayProp("Likely repo-relative paths. Leave empty if not known yet."),
			"success_criteria":       stringArrayProp("Batch-local success criteria."),
			"depends_on":             stringArrayProp("IDs of prior batches this batch depends on."),
		},
		"required": []string{"goal"},
	}
}

func renderConstraintSeed(c types.WriteConstraint) string {
	parts := []string{strings.TrimSpace(c.Kind)}
	if target := strings.TrimSpace(c.Target); target != "" {
		parts = append(parts, target)
	}
	if note := strings.TrimSpace(c.Note); note != "" {
		parts = append(parts, note)
	}
	return strings.Join(dedupTrimWorkflowStrings(parts), ": ")
}

func stringProp(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func stringArrayProp(description string) map[string]any {
	return map[string]any{
		"type":                        "array",
		"description":                 description,
		"x-codrax-split-string-array": true,
		"items":                       map[string]any{"type": "string"},
	}
}

func enumStringProp(values []string) map[string]any {
	return map[string]any{"type": "string", "enum": values}
}

func normalizeWorkflowStatus(status WorkflowStatus) WorkflowStatus {
	switch status {
	case WorkflowPlanned, WorkflowInProgress, WorkflowNeedsClarification, WorkflowBlocked, WorkflowComplete:
		return status
	default:
		return WorkflowPlanned
	}
}

func normalizeBatchStatus(status BatchStatus) BatchStatus {
	switch status {
	case BatchPending, BatchNeedsExploration, BatchReadyForChangePlan, BatchApplied, BatchNeedsRepair, BatchBlocked, BatchComplete:
		return status
	default:
		return BatchPending
	}
}

func dedupTrimWorkflowStrings(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, value := range in {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
