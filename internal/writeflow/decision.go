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

	// FinishDisposition is the typed escape lane for the finish hard
	// gate. The scheduler rejects finish while a batch's latest
	// post-apply verify attempt failed UNLESS the model declares
	// accept_unverified here. Only meaningful with action=finish.
	FinishDisposition string `json:"finish_disposition,omitempty"`
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
	decision.FinishDisposition = strings.TrimSpace(decision.FinishDisposition)
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
	switch decision.FinishDisposition {
	case "", FinishDispositionAllVerified, FinishDispositionAcceptUnverified:
	default:
		errs = append(errs, fmt.Sprintf("finish_disposition %q is not one of: %s, %s",
			decision.FinishDisposition, FinishDispositionAllVerified, FinishDispositionAcceptUnverified))
	}
	if decision.FinishDisposition != "" && decision.Action != ActionFinish {
		errs = append(errs, "finish_disposition is only valid with action=finish")
	}
	return errs
}

// AllWorkflowActions is the full canonical action set in schema order.
func AllWorkflowActions() []WorkflowAction {
	return []WorkflowAction{
		ActionExploreCode,
		ActionPlanBatch,
		ActionApplyPlan,
		ActionVerifyBatch,
		ActionAppendBatch,
		ActionSplitBatch,
		ActionReplanBatch,
		ActionAskUser,
		ActionFinish,
		ActionBlock,
	}
}

// WorkflowActionsForMode returns the action set the controller may emit in
// the given typed mode. ModePlan is a planning-only lane: apply_plan and
// verify_batch are removed from the schema BEFORE the model sees it, so the
// model never has to learn the restriction from a runtime rejection. Every
// other mode keeps the full set.
func WorkflowActionsForMode(mode types.PipelineMode) []WorkflowAction {
	switch mode {
	case types.ModePlan:
		out := make([]WorkflowAction, 0, 8)
		for _, action := range AllWorkflowActions() {
			if action == ActionApplyPlan || action == ActionVerifyBatch {
				continue
			}
			out = append(out, action)
		}
		return out
	case types.ModeVerify:
		// Verify-only lane: the run exists to give an already-applied (or
		// imported) plan its verdict. Planning, applying, and exploration
		// are structurally absent from the schema.
		return []WorkflowAction{ActionVerifyBatch, ActionAskUser, ActionFinish, ActionBlock}
	default:
		return AllWorkflowActions()
	}
}

// WorkflowActionAllowedInMode reports whether the typed action is in the
// mode's schema set. Shared by the emit tool's runtime validation and the
// scheduler guard so all three layers read one source of truth.
func WorkflowActionAllowedInMode(action WorkflowAction, mode types.PipelineMode) bool {
	for _, allowed := range WorkflowActionsForMode(mode) {
		if action == allowed {
			return true
		}
	}
	return false
}

// WriteWorkflowDecisionSchema returns the model-facing JSON schema for the
// workflow controller. It uses the same x-codrax split-string hints as existing
// writeflow schemas so the unified JSON compatibility layer can repair common
// small-model shape drift before validation.
func WriteWorkflowDecisionSchema() json.RawMessage {
	return WriteWorkflowDecisionSchemaForActions(AllWorkflowActions())
}

// WriteWorkflowDecisionSchemaForActions builds the decision schema with the
// action enum restricted to the given set. Mode-scoped tool views project
// the schema per dispatch so masked actions are structurally unavailable.
func WriteWorkflowDecisionSchemaForActions(actions []WorkflowAction) json.RawMessage {
	enum := make([]string, 0, len(actions))
	for _, a := range actions {
		enum = append(enum, string(a))
	}
	raw, err := json.Marshal(map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"action":              enumStringProp(enum),
			"reason_code":         stringProp("Short typed reason code for the selected action."),
			"reason":              stringProp("Brief explanation of the selected action."),
			"workflow":            workflowPlanSchema(),
			"batch":               batchPlanSchema(),
			"exploration_request": explorationRequestSchema(),
			"questions_for_user":  stringArrayProp("Questions to ask only when required user-owned information is missing."),
			"safety_notes":        stringArrayProp("Typed safety notes that should affect approval or verification."),
			"finish_disposition": map[string]any{
				"type": "string",
				"enum": []string{FinishDispositionAllVerified, FinishDispositionAcceptUnverified},
				"description": "Only with action=finish. all_verified: every applied batch passed its latest verification. " +
					"accept_unverified: you explicitly accept finishing although the latest verification of some batch failed; " +
					"the run completes with that caveat recorded. Without accept_unverified, finish is rejected while a " +
					"batch's latest verification failed.",
			},
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
