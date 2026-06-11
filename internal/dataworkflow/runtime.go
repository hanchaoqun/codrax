package dataworkflow

import (
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

// WorkflowRuntime is the storage-neutral handle for live data workflow state.
// It deliberately stores only workflow/dataquery IR, not REPL or CLI types.
type WorkflowRuntime struct {
	currentPlan     dataquery.TaskPlan
	planTransitions []PlanTransitionEvent
	records         []WorkflowRecord
	processEvents   []WorkflowJournalEvent
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

type PlanSwitchDecision struct {
	Source        string                 `json:"source,omitempty"`
	Round         int                    `json:"round,omitempty"`
	Reason        string                 `json:"reason,omitempty"`
	Plan          dataquery.TaskPlan     `json:"plan,omitempty"`
	ProcessEvents []WorkflowJournalEvent `json:"process_events,omitempty"`
	Switched      bool                   `json:"switched,omitempty"`
}

type DeferredQueueAdvanceDecision struct {
	Action         string                         `json:"action,omitempty"`
	Status         DeferredDispatchStatus         `json:"status,omitempty"`
	Lifecycle      DeferredQueueLifecycleDecision `json:"lifecycle,omitempty"`
	QueuedPlan     dataquery.TaskPlan             `json:"queued_plan,omitempty"`
	DispatchedPlan dataquery.TaskPlan             `json:"dispatched_plan,omitempty"`
	RemainderPlan  dataquery.TaskPlan             `json:"remainder_plan,omitempty"`
	Queue          DeferredQueueState             `json:"queue,omitempty"`
	Reason         string                         `json:"reason,omitempty"`
}

type PostResultRuntimeDecision struct {
	PostResult PostResultDecision           `json:"post_result,omitempty"`
	Advance    DeferredQueueAdvanceDecision `json:"advance,omitempty"`
	Applied    bool                         `json:"applied,omitempty"`
}

type GuardRecoveryRuntimeDecision struct {
	Recovery       GuardRecoveryDecision        `json:"recovery,omitempty"`
	Switch         PlanSwitchDecision           `json:"switch,omitempty"`
	Deferred       DeferredQueueAdvanceDecision `json:"deferred,omitempty"`
	Plan           dataquery.TaskPlan           `json:"plan,omitempty"`
	Remainder      dataquery.TaskPlan           `json:"remainder,omitempty"`
	Reason         string                       `json:"reason,omitempty"`
	DeferredQueued bool                         `json:"deferred_queued,omitempty"`
	Applied        bool                         `json:"applied,omitempty"`
}

type FailureRecoveryRuntimeDecision struct {
	Recovery FailureRecoveryDecision `json:"recovery,omitempty"`
	Switch   PlanSwitchDecision      `json:"switch,omitempty"`
	Plan     dataquery.TaskPlan      `json:"plan,omitempty"`
	Source   string                  `json:"source,omitempty"`
	Reason   string                  `json:"reason,omitempty"`
	Applied  bool                    `json:"applied,omitempty"`
}

type CandidatePlanAdmissionDecision struct {
	Admission      ActionDAGAdmissionDecision `json:"admission,omitempty"`
	Round          int                        `json:"round,omitempty"`
	Source         string                     `json:"source,omitempty"`
	Plan           dataquery.TaskPlan         `json:"plan,omitempty"`
	ProcessEvents  []WorkflowJournalEvent     `json:"process_events,omitempty"`
	DeferredPlan   dataquery.TaskPlan         `json:"deferred_plan,omitempty"`
	DeferredQueue  DeferredQueueState         `json:"deferred_queue,omitempty"`
	DeferredReason string                     `json:"deferred_reason,omitempty"`
	Accepted       bool                       `json:"accepted,omitempty"`
	Blocked        bool                       `json:"blocked,omitempty"`
	Rewritten      bool                       `json:"rewritten,omitempty"`
	AppendedRecord bool                       `json:"appended_record,omitempty"`
	DeferredQueued bool                       `json:"deferred_queued,omitempty"`
}

type WorkflowRuntimeSnapshot struct {
	CurrentPlan     dataquery.TaskPlan         `json:"current_plan,omitempty"`
	DeferredQueue   DeferredQueueState         `json:"deferred_queue,omitempty"`
	DeferredPlan    dataquery.TaskPlan         `json:"deferred_plan,omitempty"`
	Records         []WorkflowRecord           `json:"records,omitempty"`
	ProcessEvents   []WorkflowJournalEvent     `json:"process_events,omitempty"`
	PlanTransitions []PlanTransitionEvent      `json:"plan_transitions,omitempty"`
	Admission       ActionDAGAdmissionDecision `json:"admission,omitempty"`
	DataRounds      int                        `json:"data_rounds,omitempty"`
	RepairRounds    int                        `json:"repair_rounds,omitempty"`
}

type WorkflowRuntimeView struct {
	Records       []WorkflowRecord
	CurrentPlan   dataquery.TaskPlan
	DeferredQueue DeferredQueueState
	DeferredPlan  dataquery.TaskPlan
	DataRounds    int
	RepairRounds  int
}

type WorkflowRuntimeViewInput struct {
	Runtime              *WorkflowRuntime
	FallbackRecords      []WorkflowRecord
	FallbackCurrent      dataquery.TaskPlan
	FallbackDeferred     dataquery.TaskPlan
	FallbackDataRounds   int
	FallbackRepairRounds int
}

type WorkflowIterationDecision struct {
	Phase         string             `json:"phase,omitempty"`
	CurrentPlan   dataquery.TaskPlan `json:"current_plan,omitempty"`
	DeferredQueue DeferredQueueState `json:"deferred_queue,omitempty"`
	DeferredPlan  dataquery.TaskPlan `json:"deferred_plan,omitempty"`
	Records       []WorkflowRecord   `json:"records,omitempty"`
	DataRounds    int                `json:"data_rounds,omitempty"`
	RepairRounds  int                `json:"repair_rounds,omitempty"`
}

// BuildWorkflowRuntimeView is the reducer-owned runtime boundary used by CLI
// and REPL adapters when they need prompt, evaluator, repair, or checkpoint
// state. Fallback values support legacy/no-runtime tests, while a live runtime
// snapshot is authoritative whenever it carries concrete state.
func BuildWorkflowRuntimeView(input WorkflowRuntimeViewInput) WorkflowRuntimeView {
	out := WorkflowRuntimeView{
		Records:      cloneWorkflowRecords(input.FallbackRecords),
		CurrentPlan:  cloneTaskPlanValue(input.FallbackCurrent),
		DeferredPlan: cloneTaskPlanValue(input.FallbackDeferred),
		DataRounds:   maxRuntimeInt(input.FallbackDataRounds, 0),
		RepairRounds: maxRuntimeInt(input.FallbackRepairRounds, 0),
	}
	if len(input.FallbackDeferred.Actions) > 0 {
		out.DeferredQueue = NewDeferredQueue(input.FallbackDeferred)
	}
	if input.Runtime == nil {
		return out
	}
	snapshot := input.Runtime.Snapshot()
	if len(snapshot.Records) > 0 {
		out.Records = cloneWorkflowRecords(snapshot.Records)
	}
	if TaskPlanHasRuntimeShape(snapshot.CurrentPlan) {
		out.CurrentPlan = cloneTaskPlanValue(snapshot.CurrentPlan)
	}
	if len(snapshot.DeferredQueue.Plan.Actions) > 0 || len(snapshot.DeferredPlan.Actions) > 0 {
		out.DeferredQueue = cloneDeferredQueue(snapshot.DeferredQueue)
		out.DeferredPlan = cloneTaskPlanValue(snapshot.DeferredPlan)
	}
	if snapshot.DataRounds > 0 || snapshot.RepairRounds > 0 {
		out.DataRounds = snapshot.DataRounds
		out.RepairRounds = snapshot.RepairRounds
	}
	return out
}

func TaskPlanHasRuntimeShape(plan dataquery.TaskPlan) bool {
	return strings.TrimSpace(plan.Status) != "" ||
		strings.TrimSpace(plan.Goal) != "" ||
		strings.TrimSpace(plan.Script) != "" ||
		len(plan.Actions) > 0 ||
		len(plan.InputPaths) > 0 ||
		len(plan.CoverageContract.RequiredMaterials) > 0 ||
		len(plan.CoverageContract.OptionalMaterials) > 0
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
	transition := buildPlanTransitionEvent(round, source, rt.currentPlan, reason)
	rt.planTransitions = appendPlanTransitionEvent(rt.planTransitions, transition)
	rt.processEvents = appendWorkflowJournalEvents(rt.processEvents, BuildPlanTransitionProcessEvent(transition, rt.currentPlan))
	return rt.CurrentPlan()
}

func (rt *WorkflowRuntime) SwitchCurrentPlanWithEvents(round int, source string, plan dataquery.TaskPlan, reason string) PlanSwitchDecision {
	out := PlanSwitchDecision{
		Source: trimRuntimeText(source),
		Round:  round,
		Reason: trimRuntimeText(reason),
		Plan:   cloneTaskPlanValue(plan),
	}
	if rt == nil {
		return out
	}
	processEventStart := len(rt.processEvents)
	out.Plan = rt.SwitchCurrentPlan(round, source, plan, reason)
	out.ProcessEvents = rt.processEventsSince(processEventStart)
	out.Switched = true
	return out
}

func (rt *WorkflowRuntime) CurrentPlan() dataquery.TaskPlan {
	if rt == nil {
		return dataquery.TaskPlan{}
	}
	return cloneTaskPlanValue(rt.currentPlan)
}

func (rt *WorkflowRuntime) Snapshot() WorkflowRuntimeSnapshot {
	if rt == nil {
		return WorkflowRuntimeSnapshot{}
	}
	deferredQueue := rt.DeferredQueue()
	return WorkflowRuntimeSnapshot{
		CurrentPlan:     rt.CurrentPlan(),
		DeferredQueue:   deferredQueue,
		DeferredPlan:    DeferredQueuePlan(deferredQueue),
		Records:         rt.Records(),
		ProcessEvents:   rt.ProcessEvents(),
		PlanTransitions: rt.PlanTransitions(),
		Admission:       rt.Admission(),
		DataRounds:      rt.DataRounds(),
		RepairRounds:    rt.RepairRounds(),
	}
}

func (rt *WorkflowRuntime) PlanTransitions() []PlanTransitionEvent {
	if rt == nil {
		return nil
	}
	return append([]PlanTransitionEvent(nil), rt.planTransitions...)
}

func (rt *WorkflowRuntime) SetRecords(records []WorkflowRecord) {
	if rt == nil {
		return
	}
	rt.records = cloneWorkflowRecords(records)
	rt.processEvents = BuildWorkflowJournalEvents(rt.records)
}

func (rt *WorkflowRuntime) Records() []WorkflowRecord {
	if rt == nil {
		return nil
	}
	return cloneWorkflowRecords(rt.records)
}

func (rt *WorkflowRuntime) AppendRecord(record WorkflowRecord) []WorkflowRecord {
	if rt == nil {
		return []WorkflowRecord{cloneWorkflowRecord(record)}
	}
	nextRound := len(rt.records) + 1
	copied := cloneWorkflowRecord(record)
	rt.records = append(rt.records, copied)
	rt.processEvents = appendWorkflowJournalEvents(rt.processEvents, BuildWorkflowJournalEventsForRecord(nextRound, copied)...)
	return rt.Records()
}

func (rt *WorkflowRuntime) RecordsWith(record WorkflowRecord) []WorkflowRecord {
	if rt == nil {
		return []WorkflowRecord{cloneWorkflowRecord(record)}
	}
	out := rt.Records()
	out = append(out, cloneWorkflowRecord(record))
	return out
}

func blockedIdempotencyKeysFromRecords(records []WorkflowRecord) []string {
	seen := map[string]bool{}
	addAction := func(action dataquery.DataAction) {
		if key := strings.TrimSpace(ActionIdempotencyKey(action)); key != "" {
			seen[key] = true
		}
	}
	addPlan := func(plan dataquery.TaskPlan) {
		for _, action := range plan.Actions {
			addAction(action)
		}
	}
	for _, record := range records {
		blocked := strings.TrimSpace(record.Err) != "" || len(record.Violations) > 0
		if record.Admission != nil && strings.TrimSpace(record.Admission.FinalGuardErr) != "" {
			blocked = true
		}
		if !blocked {
			continue
		}
		// Action-attributed failures poison only the named actions.
		// The previous behaviour poisoned EVERY action key of a
		// blocked plan, so a correct sibling action resubmitted in a
		// repair plan was rejected as "repeats a previously blocked
		// or failed workflow edge" — observed live: a valid decimal
		// multiply custom_transform was refused solely because it
		// shared a plan with one rejected action, forcing the planner
		// into hardcoded-constant workarounds. Violations carry a
		// typed ActionID; when at least one names an action present
		// in the plan, only those actions' keys are poisoned. Records
		// with no action attribution (plan-shape rejections) keep the
		// conservative whole-plan poisoning.
		attributed := map[string]bool{}
		for _, violation := range record.Violations {
			if id := strings.TrimSpace(violation.ActionID); id != "" {
				attributed[id] = true
			}
		}
		if len(attributed) > 0 {
			matched := false
			addAttributed := func(plan dataquery.TaskPlan) {
				for _, action := range plan.Actions {
					if attributed[strings.TrimSpace(action.ID)] {
						addAction(action)
						matched = true
					}
				}
			}
			addAttributed(record.Plan)
			if record.Admission != nil {
				addAttributed(record.Admission.Plan)
				addAttributed(record.Admission.Original)
			}
			if matched {
				continue
			}
			// Attribution named no action in the plan (stale or
			// foreign ID) — fall through to whole-plan poisoning.
		}
		addPlan(record.Plan)
		if record.Admission != nil {
			addPlan(record.Admission.Plan)
			addPlan(record.Admission.Original)
		}
	}
	if len(seen) == 0 {
		return nil
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func mergeIdempotencyKeys(first, second []string) []string {
	if len(first) == 0 && len(second) == 0 {
		return nil
	}
	seen := map[string]bool{}
	for _, key := range first {
		key = strings.TrimSpace(key)
		if key != "" {
			seen[key] = true
		}
	}
	for _, key := range second {
		key = strings.TrimSpace(key)
		if key != "" {
			seen[key] = true
		}
	}
	out := make([]string, 0, len(seen))
	for key := range seen {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func (rt *WorkflowRuntime) AppendProcessEvent(event WorkflowJournalEvent) []WorkflowJournalEvent {
	if rt == nil {
		return []WorkflowJournalEvent{cloneWorkflowJournalEvent(event)}
	}
	rt.processEvents = appendWorkflowJournalEvents(rt.processEvents, event)
	return rt.ProcessEvents()
}

func (rt *WorkflowRuntime) AppendProcessEventFromInput(input WorkflowProcessEventInput) []WorkflowJournalEvent {
	return rt.AppendProcessEvent(BuildWorkflowProcessEvent(input))
}

func (rt *WorkflowRuntime) AppendGuardEvent(kind string, round int, plan dataquery.TaskPlan, guard GuardResult, auditDetails ...string) WorkflowJournalEvent {
	event := BuildGuardProcessEvent(kind, round, plan, guard, auditDetails...)
	if event.Guard == nil || event.Guard.Empty() {
		return WorkflowJournalEvent{}
	}
	rt.AppendProcessEvent(event)
	return event
}

func (rt *WorkflowRuntime) ProcessEvents() []WorkflowJournalEvent {
	if rt == nil {
		return nil
	}
	return cloneWorkflowJournalEvents(rt.processEvents)
}

func (rt *WorkflowRuntime) AttachLastEvaluation(eval dataquery.Evaluation) []WorkflowRecord {
	if rt == nil {
		return nil
	}
	if len(rt.records) == 0 {
		return rt.Records()
	}
	copied := eval
	copied.MissingInputs = append([]string(nil), eval.MissingInputs...)
	rt.records[len(rt.records)-1].Evaluation = &copied
	return rt.Records()
}

func (rt *WorkflowRuntime) AttachLastError(errText string) []WorkflowRecord {
	if rt == nil {
		return nil
	}
	if len(rt.records) == 0 {
		return rt.Records()
	}
	rt.records[len(rt.records)-1].Err = errText
	return rt.Records()
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

func (rt *WorkflowRuntime) QueueDeferred(round int, plan dataquery.TaskPlan, reason string) DeferredQueueAdvanceDecision {
	out := DeferredQueueAdvanceDecision{
		Action:     DeferredQueueTransitionEnqueue,
		QueuedPlan: cloneTaskPlanValue(plan),
		Reason:     trimRuntimeText(reason),
	}
	if rt == nil {
		return out
	}
	rt.EnqueueDeferred(round, plan, reason)
	out.Queue = rt.DeferredQueue()
	return out
}

func (rt *WorkflowRuntime) DispatchDeferred(round int, dispatched, remainder dataquery.TaskPlan, status DeferredDispatchStatus, reason string) DeferredQueueState {
	if rt == nil {
		return DeferredQueueState{}
	}
	rt.deferredQueue = DispatchDeferredQueue(rt.deferredQueue, round, dispatched, remainder, status, reason)
	return rt.DeferredQueue()
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

func (rt *WorkflowRuntime) AdvanceDeferredQueue(round int, dispatched, remainder dataquery.TaskPlan, status DeferredDispatchStatus, dispatchReady bool, dispatchReason, discardReason string) DeferredQueueAdvanceDecision {
	out := DeferredQueueAdvanceDecision{
		Status:         status,
		DispatchedPlan: cloneTaskPlanValue(dispatched),
		RemainderPlan:  cloneTaskPlanValue(remainder),
	}
	if dispatchReady {
		out.Action = DeferredQueueTransitionDispatch
		out.Reason = trimRuntimeText(dispatchReason)
		out.Queue = rt.DispatchDeferred(round, dispatched, remainder, status, out.Reason)
		return out
	}
	deferredPlan := rt.DeferredPlan()
	if len(deferredPlan.Actions) == 0 {
		out.Action = DeferredQueueLifecycleNone
		out.Lifecycle = DeferredQueueLifecycleDecision{Action: DeferredQueueLifecycleNone, ReasonCode: status.ReasonCode, Reason: status.Reason}
		rt.ClearDeferred(round, status.Reason)
		out.Queue = rt.DeferredQueue()
		return out
	}
	lifecycle := DecideDeferredQueueLifecycle(status)
	out.Lifecycle = lifecycle
	switch lifecycle.Action {
	case DeferredQueueLifecycleRetain:
		out.Action = DeferredQueueTransitionRetain
		rt.RetainDeferred(round, status)
	case DeferredQueueLifecycleDiscard:
		out.Action = DeferredQueueTransitionDiscard
		out.Reason = firstNonEmpty(discardReason, lifecycle.Reason, status.Reason)
		rt.DiscardDeferred(round, status, out.Reason)
	default:
		out.Action = DeferredQueueTransitionClear
		out.Reason = lifecycle.Reason
		rt.ClearDeferred(round, out.Reason)
	}
	out.Queue = rt.DeferredQueue()
	return out
}

func (rt *WorkflowRuntime) ApplyPostResultDecision(dataRound int, decision PostResultDecision) PostResultRuntimeDecision {
	out := PostResultRuntimeDecision{PostResult: clonePostResultDecision(decision)}
	switch decision.Action {
	case PostResultDispatchDeferred:
		out.Advance = rt.AdvanceDeferredQueue(dataRound+1, decision.Plan, decision.Remainder, decision.DeferredStatus, true, "dispatch ready deferred typed data action rank", "")
		out.Applied = true
	case PostResultUpdateDeferred:
		out.Advance = rt.AdvanceDeferredQueue(dataRound, dataquery.TaskPlan{}, dataquery.TaskPlan{}, decision.DeferredStatus, false, "", "")
		out.Applied = true
	case PostResultEvaluate:
		out.Advance = rt.AdvanceDeferredQueue(dataRound, dataquery.TaskPlan{}, dataquery.TaskPlan{}, DeferredDispatchStatus{}, false, "", "")
		out.Applied = true
	}
	return out
}

func (rt *WorkflowRuntime) ApplyGuardRecoveryDecision(round int, decision GuardRecoveryDecision, protect func(dataquery.TaskPlan) dataquery.TaskPlan) GuardRecoveryRuntimeDecision {
	out := GuardRecoveryRuntimeDecision{
		Recovery: cloneGuardRecoveryDecision(decision),
		Reason:   trimRuntimeText(decision.Reason),
	}
	if decision.Action != GuardRecoveryFallbackPlan || !decision.HasPlan() {
		return out
	}
	if protect == nil {
		protect = func(plan dataquery.TaskPlan) dataquery.TaskPlan { return plan }
	}
	fallback := protect(decision.Plan)
	out.Plan = cloneTaskPlanValue(fallback)
	out.Switch = rt.SwitchCurrentPlanWithEvents(round, "continue", fallback, out.Reason)
	if TaskPlanHasRuntimeShape(out.Switch.Plan) {
		out.Plan = cloneTaskPlanValue(out.Switch.Plan)
	}
	if decision.HasRemainder() {
		out.Remainder = cloneTaskPlanValue(decision.Remainder)
		out.Deferred = rt.QueueDeferred(round, decision.Remainder, out.Reason)
		out.DeferredQueued = len(out.Deferred.QueuedPlan.Actions) > 0
	}
	out.Applied = true
	return out
}

func (rt *WorkflowRuntime) ApplyFailureRecoveryDecision(round int, source string, decision FailureRecoveryDecision, protect func(dataquery.TaskPlan) dataquery.TaskPlan) FailureRecoveryRuntimeDecision {
	out := FailureRecoveryRuntimeDecision{
		Recovery: cloneFailureRecoveryDecision(decision),
		Source:   trimRuntimeText(source),
		Reason:   trimRuntimeText(decision.Reason),
	}
	if out.Source == "" {
		out.Source = "continue"
	}
	if decision.Action != FailureRecoveryFallbackPlan || !decision.HasPlan() {
		return out
	}
	if protect == nil {
		protect = func(plan dataquery.TaskPlan) dataquery.TaskPlan { return plan }
	}
	fallback := protect(decision.Plan)
	out.Plan = cloneTaskPlanValue(fallback)
	out.Switch = rt.SwitchCurrentPlanWithEvents(round, out.Source, fallback, out.Reason)
	if TaskPlanHasRuntimeShape(out.Switch.Plan) {
		out.Plan = cloneTaskPlanValue(out.Switch.Plan)
	}
	out.Applied = true
	return out
}

func clonePostResultDecision(decision PostResultDecision) PostResultDecision {
	out := decision
	out.Plan = cloneTaskPlanValue(decision.Plan)
	out.Remainder = cloneTaskPlanValue(decision.Remainder)
	out.DeferredPlan = cloneTaskPlanValue(decision.DeferredPlan)
	return out
}

func cloneGuardRecoveryDecision(decision GuardRecoveryDecision) GuardRecoveryDecision {
	out := decision
	out.Plan = cloneTaskPlanValue(decision.Plan)
	out.Remainder = cloneTaskPlanValue(decision.Remainder)
	out.Guard = cloneGuardResult(decision.Guard)
	return out
}

func cloneFailureRecoveryDecision(decision FailureRecoveryDecision) FailureRecoveryDecision {
	out := decision
	out.Plan = cloneTaskPlanValue(decision.Plan)
	out.Guard = cloneGuardResult(decision.Guard)
	return out
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

func (rt *WorkflowRuntime) AdmitCandidatePlan(round int, source string, input ActionDAGAdmissionInput) CandidatePlanAdmissionDecision {
	processEventStart := 0
	if rt != nil {
		processEventStart = len(rt.processEvents)
		input.BlockedIdempotencyKeys = mergeIdempotencyKeys(input.BlockedIdempotencyKeys, blockedIdempotencyKeysFromRecords(rt.records))
	}
	admission := AdmitActionDAGPlan(input)
	out := CandidatePlanAdmissionDecision{
		Admission: cloneAdmissionDecision(admission),
		Round:     round,
		Source:    trimRuntimeText(source),
		Plan:      cloneTaskPlanValue(admission.Plan),
		Blocked:   trimRuntimeText(admission.FinalGuardErr) != "",
		Rewritten: admission.Rewritten,
	}
	if rt == nil {
		return out
	}
	rt.SetAdmission(admission)
	transitionSource := out.Source
	if admission.Rewritten {
		transitionSource = "continue"
		rt.AppendRecord(WorkflowRecord{
			Plan:      admission.Original,
			Err:       admission.GuardErr,
			Admission: &admission,
		})
		out.AppendedRecord = true
	}
	out.Source = transitionSource
	if out.Blocked {
		rt.SetCurrentPlan(admission.Plan)
		out.Plan = rt.CurrentPlan()
	} else {
		out.Plan = rt.SwitchCurrentPlan(round, transitionSource, admission.Plan, admission.Reason)
		out.Accepted = true
	}
	out.Admission = rt.Admission()
	out.ProcessEvents = rt.processEventsSince(processEventStart)
	return out
}

func (rt *WorkflowRuntime) AdmitCandidatePlanAndQueueRemainder(round int, source string, input ActionDAGAdmissionInput) CandidatePlanAdmissionDecision {
	out := rt.AdmitCandidatePlan(round, source, input)
	if rt == nil || !out.Rewritten || len(out.Admission.Remainder.Actions) == 0 {
		return out
	}
	reason := firstNonEmpty(out.Admission.Reason, "deferred remainder from admitted candidate plan")
	queued := rt.QueueDeferred(round, out.Admission.Remainder, reason)
	out.DeferredQueued = true
	out.DeferredPlan = cloneTaskPlanValue(queued.QueuedPlan)
	out.DeferredQueue = queued.Queue
	out.DeferredReason = queued.Reason
	return out
}

func (rt *WorkflowRuntime) processEventsSince(start int) []WorkflowJournalEvent {
	if rt == nil {
		return nil
	}
	events := rt.ProcessEvents()
	if start < 0 || start > len(events) {
		start = 0
	}
	return cloneWorkflowJournalEvents(events[start:])
}

func (rt *WorkflowRuntime) SetRounds(dataRounds, repairRounds int) {
	if rt == nil {
		return
	}
	if dataRounds < 0 {
		dataRounds = 0
	}
	if repairRounds < 0 {
		repairRounds = 0
	}
	rt.dataRounds = dataRounds
	rt.repairRounds = repairRounds
}

func (rt *WorkflowRuntime) IterationDecision(phase string) WorkflowIterationDecision {
	out := WorkflowIterationDecision{Phase: trimRuntimeText(phase)}
	if rt == nil {
		return out
	}
	snapshot := rt.Snapshot()
	out.CurrentPlan = snapshot.CurrentPlan
	out.DeferredQueue = snapshot.DeferredQueue
	out.DeferredPlan = snapshot.DeferredPlan
	out.Records = snapshot.Records
	out.DataRounds = snapshot.DataRounds
	out.RepairRounds = snapshot.RepairRounds
	return out
}

func (rt *WorkflowRuntime) BeginDataIteration() WorkflowIterationDecision {
	if rt == nil {
		return WorkflowIterationDecision{Phase: "data"}
	}
	rt.IncrementDataRound()
	return rt.IterationDecision("data")
}

func (rt *WorkflowRuntime) BeginRepairIteration() WorkflowIterationDecision {
	if rt == nil {
		return WorkflowIterationDecision{Phase: "repair"}
	}
	rt.IncrementRepairRound()
	return rt.IterationDecision("repair")
}

func (rt *WorkflowRuntime) IncrementDataRound() int {
	if rt == nil {
		return 0
	}
	rt.dataRounds++
	return rt.dataRounds
}

func (rt *WorkflowRuntime) DataRounds() int {
	if rt == nil {
		return 0
	}
	return rt.dataRounds
}

func (rt *WorkflowRuntime) IncrementRepairRound() int {
	if rt == nil {
		return 0
	}
	rt.repairRounds++
	return rt.repairRounds
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

func runtimeProcessEventsForRecords(rt *WorkflowRuntime, records []WorkflowRecord) []WorkflowJournalEvent {
	if rt == nil {
		return BuildWorkflowJournalEvents(records)
	}
	events := rt.ProcessEvents()
	if len(events) == 0 {
		return BuildWorkflowJournalEvents(records)
	}
	runtimeRecords := rt.Records()
	if len(records) > len(runtimeRecords) {
		for i := len(runtimeRecords); i < len(records); i++ {
			events = appendWorkflowJournalEvents(events, BuildWorkflowJournalEventsForRecord(i+1, records[i])...)
		}
	}
	return events
}

func appendWorkflowJournalEvents(events []WorkflowJournalEvent, next ...WorkflowJournalEvent) []WorkflowJournalEvent {
	out := cloneWorkflowJournalEvents(events)
	for _, event := range next {
		out = append(out, cloneWorkflowJournalEvent(event))
	}
	const maxWorkflowProcessEvents = 128
	if len(out) > maxWorkflowProcessEvents {
		out = out[len(out)-maxWorkflowProcessEvents:]
	}
	return out
}

func cloneWorkflowRecords(in []WorkflowRecord) []WorkflowRecord {
	out := make([]WorkflowRecord, 0, len(in))
	for _, record := range in {
		out = append(out, cloneWorkflowRecord(record))
	}
	return out
}

func cloneWorkflowRecord(in WorkflowRecord) WorkflowRecord {
	out := in
	out.Plan = cloneTaskPlanValue(in.Plan)
	if in.Result != nil {
		copied := cloneResultValue(*in.Result)
		out.Result = &copied
	}
	out.Violations = cloneDataTaskViolations(in.Violations)
	if in.Evaluation != nil {
		copied := *in.Evaluation
		copied.MissingInputs = append([]string(nil), in.Evaluation.MissingInputs...)
		out.Evaluation = &copied
	}
	if in.Admission != nil {
		copied := cloneAdmissionDecision(*in.Admission)
		out.Admission = &copied
	}
	return out
}

func cloneDataTaskViolations(in []dataquery.DataTaskViolation) []dataquery.DataTaskViolation {
	out := make([]dataquery.DataTaskViolation, 0, len(in))
	for _, violation := range in {
		copied := violation
		copied.InputAliases = append([]string(nil), violation.InputAliases...)
		copied.MissingFields = append([]string(nil), violation.MissingFields...)
		copied.AvailableFieldSample = append([]string(nil), violation.AvailableFieldSample...)
		out = append(out, copied)
	}
	return out
}

func cloneResultValue(in dataquery.Result) dataquery.Result {
	out := in
	out.Artifacts = cloneDataArtifacts(in.Artifacts)
	out.Rows = cloneRowDecisions(in.Rows)
	out.RuleCoverage = cloneRuleCoverage(in.RuleCoverage)
	out.Contributions = cloneContributions(in.Contributions)
	out.EntityResolutions = cloneEntityResolutions(in.EntityResolutions)
	if in.Reconcile != nil {
		copied := cloneReconcileReport(*in.Reconcile)
		out.Reconcile = &copied
	}
	out.Metrics = append([]dataquery.Metric(nil), in.Metrics...)
	out.ConsumedPaths = append([]string(nil), in.ConsumedPaths...)
	out.ContractWarnings = append([]string(nil), in.ContractWarnings...)
	out.ResultPatches = cloneResultPatches(in.ResultPatches)
	return out
}

func cloneDataArtifacts(in []dataquery.DataArtifact) []dataquery.DataArtifact {
	out := make([]dataquery.DataArtifact, 0, len(in))
	for _, artifact := range in {
		copied := artifact
		copied.SourcePaths = append([]string(nil), artifact.SourcePaths...)
		copied.SourceRecordPaths = append([]string(nil), artifact.SourceRecordPaths...)
		copied.ReferencePaths = append([]string(nil), artifact.ReferencePaths...)
		copied.EvidencePaths = append([]string(nil), artifact.EvidencePaths...)
		if artifact.Fields != nil {
			copied.Fields = make(map[string]string, len(artifact.Fields))
			for key, value := range artifact.Fields {
				copied.Fields[key] = value
			}
		}
		copied.Headers = append([]string(nil), artifact.Headers...)
		copied.Sample = append([]string(nil), artifact.Sample...)
		copied.Children = cloneDataArtifacts(artifact.Children)
		out = append(out, copied)
	}
	return out
}

func cloneRowDecisions(in []dataquery.RowDecision) []dataquery.RowDecision {
	out := make([]dataquery.RowDecision, 0, len(in))
	for _, row := range in {
		copied := row
		if row.NormalizedFields != nil {
			copied.NormalizedFields = make(map[string]string, len(row.NormalizedFields))
			for key, value := range row.NormalizedFields {
				copied.NormalizedFields[key] = value
			}
		}
		copied.EvidenceRef = append([]string(nil), row.EvidenceRef...)
		copied.RuleRefs = append([]string(nil), row.RuleRefs...)
		out = append(out, copied)
	}
	return out
}

func cloneRuleCoverage(in []dataquery.RuleCoverageRecord) []dataquery.RuleCoverageRecord {
	out := make([]dataquery.RuleCoverageRecord, 0, len(in))
	for _, rule := range in {
		copied := rule
		copied.EvidenceRefs = append([]string(nil), rule.EvidenceRefs...)
		out = append(out, copied)
	}
	return out
}

func cloneContributions(in []dataquery.ContributionRecord) []dataquery.ContributionRecord {
	out := make([]dataquery.ContributionRecord, 0, len(in))
	for _, contribution := range in {
		copied := contribution
		copied.EvidenceRefs = append([]string(nil), contribution.EvidenceRefs...)
		copied.RuleRefs = append([]string(nil), contribution.RuleRefs...)
		out = append(out, copied)
	}
	return out
}

func cloneEntityResolutions(in []dataquery.EntityResolutionRecord) []dataquery.EntityResolutionRecord {
	out := make([]dataquery.EntityResolutionRecord, 0, len(in))
	for _, resolution := range in {
		copied := resolution
		copied.Candidates = append([]dataquery.EntityCandidate(nil), resolution.Candidates...)
		copied.EvidenceRefs = append([]string(nil), resolution.EvidenceRefs...)
		copied.RuleRefs = append([]string(nil), resolution.RuleRefs...)
		out = append(out, copied)
	}
	return out
}

func cloneReconcileReport(in dataquery.ReconcileReport) dataquery.ReconcileReport {
	out := in
	out.Differences = append([]string(nil), in.Differences...)
	out.Groups = append([]dataquery.ReconcileGroup(nil), in.Groups...)
	return out
}

func cloneResultPatches(in []dataquery.DataResultPatch) []dataquery.DataResultPatch {
	out := make([]dataquery.DataResultPatch, 0, len(in))
	for _, patch := range in {
		copied := patch
		copied.Value = append([]byte(nil), patch.Value...)
		out = append(out, copied)
	}
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

func maxRuntimeInt(a, b int) int {
	if a > b {
		return a
	}
	return b
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
