package dataworkflow

import "github.com/hanchaoqun/codrax/internal/dataquery"

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

type DeferredQueueAdvanceDecision struct {
	Action         string                         `json:"action,omitempty"`
	Status         DeferredDispatchStatus         `json:"status,omitempty"`
	Lifecycle      DeferredQueueLifecycleDecision `json:"lifecycle,omitempty"`
	DispatchedPlan dataquery.TaskPlan             `json:"dispatched_plan,omitempty"`
	RemainderPlan  dataquery.TaskPlan             `json:"remainder_plan,omitempty"`
	Queue          DeferredQueueState             `json:"queue,omitempty"`
	Reason         string                         `json:"reason,omitempty"`
}

type CandidatePlanAdmissionDecision struct {
	Admission      ActionDAGAdmissionDecision `json:"admission,omitempty"`
	Round          int                        `json:"round,omitempty"`
	Source         string                     `json:"source,omitempty"`
	Plan           dataquery.TaskPlan         `json:"plan,omitempty"`
	Accepted       bool                       `json:"accepted,omitempty"`
	Blocked        bool                       `json:"blocked,omitempty"`
	Rewritten      bool                       `json:"rewritten,omitempty"`
	AppendedRecord bool                       `json:"appended_record,omitempty"`
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
	return out
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
