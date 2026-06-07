package dataworkflow

import "github.com/hanchaoqun/codrax/internal/dataquery"

// WorkflowRuntime is the storage-neutral handle for live data workflow state.
// It deliberately stores only workflow/dataquery IR, not REPL or CLI records.
type WorkflowRuntime struct {
	currentPlan     dataquery.TaskPlan
	planTransitions []PlanTransitionEvent
	deferredQueue   DeferredQueueState
	admission       ActionDAGAdmissionDecision
	dataRounds      int
	repairRounds    int
}

type PlanTransitionEvent struct {
	Source          string `json:"source,omitempty"`
	Round           int    `json:"round,omitempty"`
	Actions         int    `json:"actions,omitempty"`
	FirstActionID   string `json:"first_action_id,omitempty"`
	FirstActionKind string `json:"first_action_kind,omitempty"`
	Reason          string `json:"reason,omitempty"`
}

func NewWorkflowRuntime(current dataquery.TaskPlan) *WorkflowRuntime {
	rt := &WorkflowRuntime{}
	rt.SetCurrentPlan(current)
	return rt
}

func (rt *WorkflowRuntime) SetCurrentPlan(plan dataquery.TaskPlan) {
	if rt == nil {
		return
	}
	rt.currentPlan = cloneTaskPlanValue(plan)
}

func (rt *WorkflowRuntime) SwitchCurrentPlan(round int, source string, plan dataquery.TaskPlan, reason string) dataquery.TaskPlan {
	if rt == nil {
		return cloneTaskPlanValue(plan)
	}
	rt.currentPlan = cloneTaskPlanValue(plan)
	rt.planTransitions = appendPlanTransitionEvent(rt.planTransitions, buildPlanTransitionEvent(round, source, rt.currentPlan, reason))
	return rt.CurrentPlan()
}

func (rt *WorkflowRuntime) CurrentPlan() dataquery.TaskPlan {
	if rt == nil {
		return dataquery.TaskPlan{}
	}
	return cloneTaskPlanValue(rt.currentPlan)
}

func (rt *WorkflowRuntime) PlanTransitions() []PlanTransitionEvent {
	if rt == nil {
		return nil
	}
	return append([]PlanTransitionEvent(nil), rt.planTransitions...)
}

func (rt *WorkflowRuntime) SetDeferredQueue(queue DeferredQueueState) {
	if rt == nil {
		return
	}
	rt.deferredQueue = cloneDeferredQueue(queue)
}

func (rt *WorkflowRuntime) DeferredQueue() DeferredQueueState {
	if rt == nil {
		return DeferredQueueState{}
	}
	return cloneDeferredQueue(rt.deferredQueue)
}

func (rt *WorkflowRuntime) DeferredPlan() dataquery.TaskPlan {
	if rt == nil {
		return dataquery.TaskPlan{}
	}
	return DeferredQueuePlan(rt.deferredQueue)
}

func (rt *WorkflowRuntime) EnqueueDeferred(round int, plan dataquery.TaskPlan, reason string) {
	if rt == nil {
		return
	}
	rt.deferredQueue = EnqueueDeferredQueue(rt.deferredQueue, round, plan, reason)
}

func (rt *WorkflowRuntime) DispatchDeferred(round int, dispatched, remainder dataquery.TaskPlan, status DeferredDispatchStatus, reason string) {
	if rt == nil {
		return
	}
	rt.deferredQueue = DispatchDeferredQueue(rt.deferredQueue, round, dispatched, remainder, status, reason)
}

func (rt *WorkflowRuntime) RetainDeferred(round int, status DeferredDispatchStatus) {
	if rt == nil {
		return
	}
	rt.deferredQueue = RetainDeferredQueue(rt.deferredQueue, round, status)
}

func (rt *WorkflowRuntime) DiscardDeferred(round int, status DeferredDispatchStatus, reason string) {
	if rt == nil {
		return
	}
	rt.deferredQueue = DiscardDeferredQueue(rt.deferredQueue, round, status, reason)
}

func (rt *WorkflowRuntime) ClearDeferred(round int, reason string) {
	if rt == nil {
		return
	}
	rt.deferredQueue = ClearDeferredQueue(rt.deferredQueue, round, reason)
}

func (rt *WorkflowRuntime) SetAdmission(admission ActionDAGAdmissionDecision) {
	if rt == nil {
		return
	}
	rt.admission = cloneAdmissionDecision(admission)
}

func (rt *WorkflowRuntime) Admission() ActionDAGAdmissionDecision {
	if rt == nil {
		return ActionDAGAdmissionDecision{}
	}
	return cloneAdmissionDecision(rt.admission)
}

func (rt *WorkflowRuntime) SetRounds(dataRounds, repairRounds int) {
	if rt == nil {
		return
	}
	rt.dataRounds = dataRounds
	rt.repairRounds = repairRounds
}

func (rt *WorkflowRuntime) DataRounds() int {
	if rt == nil {
		return 0
	}
	return rt.dataRounds
}

func (rt *WorkflowRuntime) RepairRounds() int {
	if rt == nil {
		return 0
	}
	return rt.repairRounds
}

func cloneAdmissionDecision(in ActionDAGAdmissionDecision) ActionDAGAdmissionDecision {
	out := in
	out.Plan = cloneTaskPlanValue(in.Plan)
	out.Original = cloneTaskPlanValue(in.Original)
	out.Remainder = cloneTaskPlanValue(in.Remainder)
	out.Guard = cloneGuardResult(in.Guard)
	out.FinalGuard = cloneGuardResult(in.FinalGuard)
	return out
}

func appendPlanTransitionEvent(events []PlanTransitionEvent, event PlanTransitionEvent) []PlanTransitionEvent {
	if event.Source == "" && event.Actions == 0 && event.Reason == "" {
		return events
	}
	const maxPlanTransitionEvents = 64
	out := append(append([]PlanTransitionEvent(nil), events...), event)
	if len(out) > maxPlanTransitionEvents {
		out = out[len(out)-maxPlanTransitionEvents:]
	}
	return out
}

func buildPlanTransitionEvent(round int, source string, plan dataquery.TaskPlan, reason string) PlanTransitionEvent {
	event := PlanTransitionEvent{
		Source:  trimRuntimeText(source),
		Round:   round,
		Actions: len(plan.Actions),
		Reason:  trimRuntimeText(reason),
	}
	if len(plan.Actions) > 0 {
		first := plan.Actions[0]
		event.FirstActionID = trimRuntimeText(first.ID)
		event.FirstActionKind = string(NormalizeActionKind(first.Kind))
	}
	return event
}

func trimRuntimeText(text string) string {
	return cleanRuntimeText(text)
}

func cleanRuntimeText(text string) string {
	for len(text) > 0 {
		switch text[0] {
		case ' ', '\t', '\n', '\r':
			text = text[1:]
		default:
			goto trimRight
		}
	}
trimRight:
	for len(text) > 0 {
		switch text[len(text)-1] {
		case ' ', '\t', '\n', '\r':
			text = text[:len(text)-1]
		default:
			return text
		}
	}
	return text
}

func cloneTaskPlanValue(plan dataquery.TaskPlan) dataquery.TaskPlan {
	out := plan
	out.InputPaths = append([]string(nil), plan.InputPaths...)
	out.Questions = append([]dataquery.Question(nil), plan.Questions...)
	out.KnownConstraints = append([]string(nil), plan.KnownConstraints...)
	out.MissingObservations = append([]string(nil), plan.MissingObservations...)
	out.SuccessCriteria = append([]string(nil), plan.SuccessCriteria...)
	out.CoverageContract.RequiredMaterials = cloneCoverageMaterials(plan.CoverageContract.RequiredMaterials)
	out.CoverageContract.OptionalMaterials = cloneCoverageMaterials(plan.CoverageContract.OptionalMaterials)
	out.CoverageContract.ValidationRules = append([]string(nil), plan.CoverageContract.ValidationRules...)
	out.Actions = make([]dataquery.DataAction, 0, len(plan.Actions))
	for _, action := range plan.Actions {
		copied := action
		copied.InputPaths = append([]string(nil), action.InputPaths...)
		copied.SuccessCriteria = append([]string(nil), action.SuccessCriteria...)
		if action.Params != nil {
			copied.Params = make(map[string]string, len(action.Params))
			for key, value := range action.Params {
				copied.Params[key] = value
			}
		}
		out.Actions = append(out.Actions, copied)
	}
	return out
}

func cloneCoverageMaterials(in []dataquery.CoverageMaterial) []dataquery.CoverageMaterial {
	out := make([]dataquery.CoverageMaterial, 0, len(in))
	for _, material := range in {
		copied := material
		copied.DistilledNotes = append([]string(nil), material.DistilledNotes...)
		out = append(out, copied)
	}
	return out
}

func cloneGuardResult(in GuardResult) GuardResult {
	out := in
	out.Violations = make([]WorkflowViolation, 0, len(in.Violations))
	for _, violation := range in.Violations {
		copied := violation
		copied.MissingFields = append([]string(nil), violation.MissingFields...)
		copied.AvailableFieldSample = append([]string(nil), violation.AvailableFieldSample...)
		copied.InputAliases = append([]string(nil), violation.InputAliases...)
		copied.RepairActionHints = append([]string(nil), violation.RepairActionHints...)
		copied.CandidateArtifacts = append([]string(nil), violation.CandidateArtifacts...)
		out.Violations = append(out.Violations, copied)
	}
	return out
}
