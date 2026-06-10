package writeflow

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// WorkflowAction is the model-facing next action for the write workflow
// controller. It is typed and schema-validated; prose explanations never drive
// hard routing.
type WorkflowAction string

const (
	ActionExploreCode WorkflowAction = "explore_code"
	ActionPlanBatch   WorkflowAction = "plan_batch"
	ActionApplyPlan   WorkflowAction = "apply_plan"
	ActionVerifyBatch WorkflowAction = "verify_batch"
	ActionAppendBatch WorkflowAction = "append_batch"
	ActionSplitBatch  WorkflowAction = "split_batch"
	ActionReplanBatch WorkflowAction = "replan_batch"
	ActionAskUser     WorkflowAction = "ask_user"
	ActionFinish      WorkflowAction = "finish"
	ActionBlock       WorkflowAction = "block"

	// Deprecated controller action names accepted as schema aliases during the
	// controller-first migration. NormalizeWriteWorkflowDecision maps these to
	// the canonical action names before orchestration switches on them.
	ActionPlanChangeBatch WorkflowAction = "plan_change_batch"
	ActionApplyReadyPlan  WorkflowAction = "apply_ready_plan"
	ActionVerify          WorkflowAction = "verify"
)

// WriteWorkflowDecision is the typed output shape for a future workflow
// controller dispatch. It can request targeted read-only exploration before a
// bounded ChangePlan, or continue to apply/verify/finish when existing typed
// artifacts are sufficient.
type WriteWorkflowDecision struct {
	Action             WorkflowAction                 `json:"action"`
	ReasonCode         string                         `json:"reason_code,omitempty"`
	Reason             string                         `json:"reason,omitempty"`
	Workflow           WriteWorkflowPlan              `json:"workflow,omitempty"`
	Batch              *WriteBatchPlan                `json:"batch,omitempty"`
	ExplorationRequest *types.WriteExplorationRequest `json:"exploration_request,omitempty"`
	QuestionsForUser   []string                       `json:"questions_for_user,omitempty"`
	SafetyNotes        []string                       `json:"safety_notes,omitempty"`
}

// NormalizeWriteWorkflowDecision trims and bounds a workflow decision. It
// preserves the model's typed action while repairing only shape-level noise.
func NormalizeWriteWorkflowDecision(decision WriteWorkflowDecision) WriteWorkflowDecision {
	decision.Action = normalizeWorkflowAction(decision.Action)
	decision.ReasonCode = strings.TrimSpace(decision.ReasonCode)
	decision.Reason = strings.TrimSpace(decision.Reason)
	decision.Workflow = NormalizeWorkflowPlan(decision.Workflow)
	if decision.Batch != nil {
		batch := NormalizeBatchPlan(*decision.Batch)
		if batch.Goal == "" {
			decision.Batch = nil
		} else {
			decision.Batch = &batch
		}
	}
	if decision.ExplorationRequest != nil {
		req := types.NormalizeWriteExplorationRequest(*decision.ExplorationRequest)
		if req.Goal == "" && len(req.ExplorationQuestions) == 0 && len(req.CandidatePaths) == 0 {
			decision.ExplorationRequest = nil
		} else {
			decision.ExplorationRequest = &req
		}
	}
	decision.QuestionsForUser = dedupTrimWorkflowStrings(decision.QuestionsForUser)
	decision.SafetyNotes = dedupTrimWorkflowStrings(decision.SafetyNotes)
	return decision
}

// ValidateWriteWorkflowDecision returns structural repair hints. It does not
// parse prose or infer user intent from keywords.
func ValidateWriteWorkflowDecision(decision WriteWorkflowDecision) []string {
	decision = NormalizeWriteWorkflowDecision(decision)
	var errs []string
	if decision.Action == "" {
		errs = append(errs, "action is required")
	}
	switch decision.Action {
	case ActionExploreCode:
		if decision.ExplorationRequest == nil {
			errs = append(errs, "explore_code requires exploration_request")
		}
	case ActionPlanBatch, ActionAppendBatch, ActionSplitBatch, ActionReplanBatch:
		if decision.Batch == nil {
			errs = append(errs, fmt.Sprintf("%s requires batch", decision.Action))
		}
	case ActionAskUser:
		if len(decision.QuestionsForUser) == 0 {
			errs = append(errs, "ask_user requires questions_for_user")
		}
	case ActionBlock:
		if decision.ReasonCode == "" && decision.Reason == "" {
			errs = append(errs, "block requires reason_code or reason")
		}
	}
	return errs
}

// WriteWorkflowDecisionSchema returns the model-facing JSON schema for the
// workflow controller. It uses the same x-codrax split-string hints as existing
// writeflow schemas so the unified JSON compatibility layer can repair common
// small-model shape drift before validation.
func WriteWorkflowDecisionSchema() json.RawMessage {
	raw, err := json.Marshal(map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"action": enumStringProp([]string{
				string(ActionExploreCode),
				string(ActionPlanBatch),
				string(ActionApplyPlan),
				string(ActionVerifyBatch),
				string(ActionAppendBatch),
				string(ActionSplitBatch),
				string(ActionReplanBatch),
				string(ActionAskUser),
				string(ActionFinish),
				string(ActionBlock),
			}),
			"reason_code":         stringProp("Short typed reason code for the selected action."),
			"reason":              stringProp("Brief explanation of the selected action."),
			"workflow":            workflowPlanSchema(),
			"batch":               batchPlanSchema(),
			"exploration_request": explorationRequestSchema(),
			"questions_for_user":  stringArrayProp("Questions to ask only when required user-owned information is missing."),
			"safety_notes":        stringArrayProp("Typed safety notes that should affect approval or verification."),
		},
		"required": []string{"action"},
	})
	if err != nil {
		return json.RawMessage(fmt.Sprintf(`{"type":"object","description":"write workflow decision schema build failed: %s"}`, err))
	}
	return raw
}

func workflowPlanSchema() map[string]any {
	return map[string]any{
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
	}
}

func explorationRequestSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"batch_id":              stringProp("The write workflow batch this exploration supports."),
			"goal":                  stringProp("The read-only source question to answer before planning code changes."),
			"exploration_questions": stringArrayProp("Specific source questions to answer before planning."),
			"candidate_paths":       stringArrayProp("Likely files or directories to inspect."),
			"constraints":           stringArrayProp("Constraints source exploration must preserve as context."),
			"max_rounds":            map[string]any{"type": "integer", "minimum": 0},
			"evidence_requirements": stringArrayProp("Evidence expectations for the handoff, such as file:line anchors or tests to identify."),
		},
	}
}

func normalizeWorkflowAction(action WorkflowAction) WorkflowAction {
	switch action {
	case ActionPlanChangeBatch:
		return ActionPlanBatch
	case ActionApplyReadyPlan:
		return ActionApplyPlan
	case ActionVerify:
		return ActionVerifyBatch
	case ActionExploreCode, ActionPlanBatch, ActionApplyPlan, ActionVerifyBatch,
		ActionAppendBatch, ActionSplitBatch, ActionReplanBatch, ActionAskUser,
		ActionFinish, ActionBlock:
		return action
	default:
		return ""
	}
}
