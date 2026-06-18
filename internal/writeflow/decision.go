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
	MissingFacts       []MissingUserFact              `json:"missing_facts,omitempty"`
	SafetyNotes        []string                       `json:"safety_notes,omitempty"`

	// FinishDisposition is the typed escape lane for the finish hard
	// gate. The scheduler rejects finish while a batch's latest
	// post-apply verify attempt failed UNLESS the model declares
	// accept_unverified here. Only meaningful with action=finish.
	FinishDisposition string `json:"finish_disposition,omitempty"`
}

// MissingUserFact is the typed reason an ask_user decision is allowed
// to interrupt Auto Pilot. The controller may ask prose questions, but
// the hard permission to pause comes from this structured fact.
type MissingUserFact struct {
	Kind        string `json:"kind"`
	Description string `json:"description"`
	EvidenceRef string `json:"evidence_ref,omitempty"`
	Consumer    string `json:"consumer"`
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
		if batchPlanIsEmpty(batch) {
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
	decision.MissingFacts = normalizeMissingUserFacts(decision.MissingFacts)
	decision.SafetyNotes = dedupTrimWorkflowStrings(decision.SafetyNotes)
	decision.FinishDisposition = strings.TrimSpace(decision.FinishDisposition)
	return decision
}

func batchPlanIsEmpty(batch WriteBatchPlan) bool {
	return batch.ID == "" &&
		batch.Goal == "" &&
		batch.Purpose == "" &&
		batch.Status == BatchPending &&
		batch.WhyThisBatch == "" &&
		!batch.NeedsCodeExploration &&
		len(batch.ExploreTargets) == 0 &&
		len(batch.ExpectedChangeKinds) == 0 &&
		len(batch.ExpectedPaths) == 0 &&
		len(batch.SuccessCriteria) == 0 &&
		len(batch.DependsOn) == 0
}

// HydrateWriteWorkflowDecisionFromRun fills small typed omissions from the
// durable workflow envelope. It reads only run/batch fields, never prose: when
// the controller emits a batch object with an ID/status but omits goal, the
// active run's matching batch goal is the canonical source of that value.
func HydrateWriteWorkflowDecisionFromRun(decision WriteWorkflowDecision, run types.WriteWorkflowRun) WriteWorkflowDecision {
	decision = NormalizeWriteWorkflowDecision(decision)
	if decision.Batch == nil {
		return decision
	}
	run = types.NormalizeWriteWorkflowRun(run)
	batch, ok := workflowRunBatch(run, decision.Batch.ID)
	if !ok && strings.TrimSpace(decision.Batch.ID) == "" {
		batch, ok = workflowRunBatch(run, run.ActiveBatchID)
	}
	if ok {
		if strings.TrimSpace(decision.Batch.Goal) == "" {
			decision.Batch.Goal = strings.TrimSpace(batch.Goal)
		}
		if strings.TrimSpace(decision.Batch.Purpose) == "" {
			decision.Batch.Purpose = strings.TrimSpace(batch.Purpose)
		}
		if len(decision.Batch.ExpectedPaths) == 0 {
			decision.Batch.ExpectedPaths = append([]string(nil), batch.ExpectedPaths...)
		}
		if len(decision.Batch.SuccessCriteria) == 0 {
			decision.Batch.SuccessCriteria = append([]string(nil), batch.SuccessCriteria...)
		}
	}
	if strings.TrimSpace(decision.Batch.Goal) == "" &&
		(strings.TrimSpace(decision.Batch.ID) == "" || strings.TrimSpace(decision.Batch.ID) == strings.TrimSpace(run.ActiveBatchID)) {
		decision.Batch.Goal = strings.TrimSpace(run.Goal)
	}
	return NormalizeWriteWorkflowDecision(decision)
}

func workflowRunBatchGoal(run types.WriteWorkflowRun, batchID string) string {
	batch, ok := workflowRunBatch(run, batchID)
	if !ok {
		return ""
	}
	return strings.TrimSpace(batch.Goal)
}

func workflowRunBatch(run types.WriteWorkflowRun, batchID string) (types.WriteWorkflowBatch, bool) {
	batchID = strings.TrimSpace(batchID)
	if batchID == "" {
		return types.WriteWorkflowBatch{}, false
	}
	for _, batch := range run.Batches {
		if strings.TrimSpace(batch.ID) == batchID {
			return batch, true
		}
	}
	return types.WriteWorkflowBatch{}, false
}

func normalizeMissingUserFacts(in []MissingUserFact) []MissingUserFact {
	if len(in) == 0 {
		return nil
	}
	out := make([]MissingUserFact, 0, len(in))
	seen := map[string]bool{}
	for _, fact := range in {
		fact.Kind = strings.ToLower(strings.TrimSpace(fact.Kind))
		fact.Description = strings.TrimSpace(fact.Description)
		fact.EvidenceRef = strings.TrimSpace(fact.EvidenceRef)
		fact.Consumer = strings.ToLower(strings.TrimSpace(fact.Consumer))
		if fact.Kind == "" && fact.Description == "" && fact.Consumer == "" && fact.EvidenceRef == "" {
			continue
		}
		key := fact.Kind + "\x00" + fact.Description + "\x00" + fact.EvidenceRef + "\x00" + fact.Consumer
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, fact)
	}
	return out
}

// MissingUserFactKeys returns durable de-dupe keys for ask_user facts.
// The key intentionally uses only typed lanes and evidence refs; question
// prose and fact descriptions remain user-visible context, not hard logic.
func MissingUserFactKeys(facts []MissingUserFact) []string {
	facts = normalizeMissingUserFacts(facts)
	out := make([]string, 0, len(facts))
	seen := map[string]bool{}
	for _, fact := range facts {
		key := strings.Join([]string{
			fact.Kind,
			fact.Consumer,
			fact.EvidenceRef,
		}, "|")
		if key == "||" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	return out
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
		} else if strings.TrimSpace(decision.Batch.Goal) == "" {
			errs = append(errs, fmt.Sprintf("%s requires batch.goal", decision.Action))
		}
	case ActionApplyPlan, ActionVerifyBatch, ActionFinish:
	case ActionAskUser:
		if len(decision.QuestionsForUser) == 0 {
			errs = append(errs, "ask_user requires questions_for_user")
		}
		if len(decision.MissingFacts) == 0 {
			errs = append(errs, "ask_user requires missing_facts")
		}
		for i, fact := range decision.MissingFacts {
			if !validMissingFactKind(fact.Kind) {
				errs = append(errs, fmt.Sprintf("missing_facts[%d].kind %q is invalid", i, fact.Kind))
			}
			if fact.Description == "" {
				errs = append(errs, fmt.Sprintf("missing_facts[%d].description is required", i))
			}
			if !validMissingFactConsumer(fact.Consumer) {
				errs = append(errs, fmt.Sprintf("missing_facts[%d].consumer %q is invalid", i, fact.Consumer))
			}
		}
	case ActionBlock:
		if decision.ReasonCode == "" && decision.Reason == "" {
			errs = append(errs, "block requires reason_code or reason")
		}
	case "":
		// Already reported above.
	default:
		errs = append(errs, fmt.Sprintf("action %q is not one of: %s", decision.Action, strings.Join(workflowActionStrings(AllWorkflowActions()), ", ")))
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

func validMissingFactKind(kind string) bool {
	switch kind {
	case "scope_boundary", "user_choice", "external_fact", "approval_boundary", "runtime_constraint", "credential", "unknown":
		return true
	default:
		return false
	}
}

func validMissingFactConsumer(consumer string) bool {
	switch consumer {
	case "controller", "planner", "verifier", "coder":
		return true
	default:
		return false
	}
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
	enum := workflowActionStrings(actions)
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
			"missing_facts":       missingFactsSchema(),
			"safety_notes":        stringArrayProp("Typed safety notes that should affect approval or verification."),
			"finish_disposition": map[string]any{
				"type": "string",
				"enum": []string{FinishDispositionAllVerified, FinishDispositionAcceptUnverified},
				"description": "Only with action=finish. all_verified: every applied batch passed its latest verification. " +
					"accept_unverified: finish when local verification is unavailable (no_tests, runner_missing, parser_error) " +
					"and no typed code-failure evidence remains. It is not valid for tests_failed, build_failed, or " +
					"verification_probe failures. Without an allowed disposition, finish is rejected while a batch's " +
					"latest verification failed.",
			},
		},
		"required": []string{"action"},
	})
	if err != nil {
		return json.RawMessage(fmt.Sprintf(`{"type":"object","description":"write workflow decision schema build failed: %s"}`, err))
	}
	return raw
}

func workflowActionStrings(actions []WorkflowAction) []string {
	out := make([]string, 0, len(actions))
	seen := map[WorkflowAction]bool{}
	for _, action := range actions {
		action = normalizeWorkflowAction(action)
		if !isCanonicalWorkflowAction(action) || seen[action] {
			continue
		}
		seen[action] = true
		out = append(out, string(action))
	}
	return out
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

func missingFactsSchema() map[string]any {
	return map[string]any{
		"type":        "array",
		"description": "Typed missing facts that justify ask_user. Required with action=ask_user; questions alone are not enough to pause Auto Pilot.",
		"items": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"kind": enumStringProp([]string{
					"scope_boundary",
					"user_choice",
					"external_fact",
					"approval_boundary",
					"runtime_constraint",
					"credential",
					"unknown",
				}),
				"description":  stringProp("Concrete missing fact owned by the user or external environment."),
				"evidence_ref": stringProp("Optional artifact, path, report, batch, or context-pack reference showing why this is missing."),
				"consumer": enumStringProp([]string{
					"controller",
					"planner",
					"verifier",
					"coder",
				}),
			},
			"required": []string{"kind", "description", "consumer"},
		},
	}
}

func normalizeWorkflowAction(action WorkflowAction) WorkflowAction {
	action = WorkflowAction(strings.TrimSpace(string(action)))
	if action == "" {
		return ""
	}
	return action
}

func isCanonicalWorkflowAction(action WorkflowAction) bool {
	switch action {
	case ActionExploreCode, ActionPlanBatch, ActionApplyPlan, ActionVerifyBatch,
		ActionAppendBatch, ActionSplitBatch, ActionReplanBatch, ActionAskUser,
		ActionFinish, ActionBlock:
		return true
	default:
		return false
	}
}
