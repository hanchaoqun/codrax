package types

import (
	"strings"
	"time"
)

// WriteWorkflowRun is the outer write-controller persistence envelope. Batch 3
// introduces the schema so priority context packs have a durable home; Batch 4
// adds the store and controller that write this object.
type WriteWorkflowRun struct {
	RunID          string                   `json:"run_id"`
	Goal           string                   `json:"goal,omitempty"`
	Status         WriteWorkflowRunStatus   `json:"status,omitempty"`
	Completion     *WriteWorkflowCompletion `json:"completion,omitempty"`
	ActiveBatchID  string                   `json:"active_batch_id,omitempty"`
	CreatedAt      time.Time                `json:"created_at,omitempty"`
	UpdatedAt      time.Time                `json:"updated_at,omitempty"`
	Batches        []WriteWorkflowBatch     `json:"batches,omitempty"`
	Edges          []WriteWorkflowEdge      `json:"edges,omitempty"`
	ContextPacks   []WriteContextPack       `json:"context_packs,omitempty"`
	Budget         WriteWorkflowBudget      `json:"budget,omitempty"`
	ProgressLedger []WriteWorkflowProgress  `json:"progress_ledger,omitempty"`
}

type WriteWorkflowRunStatus string

const (
	WriteWorkflowRunPlanned    WriteWorkflowRunStatus = "planned"
	WriteWorkflowRunInProgress WriteWorkflowRunStatus = "in_progress"
	WriteWorkflowRunComplete   WriteWorkflowRunStatus = "complete"
	WriteWorkflowRunBlocked    WriteWorkflowRunStatus = "blocked"
)

type WriteWorkflowBatchStatus string

const (
	WriteWorkflowBatchNeedsExploration WriteWorkflowBatchStatus = "needs_exploration"
	WriteWorkflowBatchReadyToPlan      WriteWorkflowBatchStatus = "ready_to_plan"
	WriteWorkflowBatchPlanned          WriteWorkflowBatchStatus = "planned"
	WriteWorkflowBatchPendingApproval  WriteWorkflowBatchStatus = "pending_approval"
	WriteWorkflowBatchApplying         WriteWorkflowBatchStatus = "applying"
	WriteWorkflowBatchVerifying        WriteWorkflowBatchStatus = "verifying"
	WriteWorkflowBatchComplete         WriteWorkflowBatchStatus = "complete"
	WriteWorkflowBatchBlocked          WriteWorkflowBatchStatus = "blocked"
)

type WriteWorkflowCompletionVerdict string

const (
	WriteWorkflowCompletionVerified       WriteWorkflowCompletionVerdict = "verified"
	WriteWorkflowCompletionUnverified     WriteWorkflowCompletionVerdict = "unverified"
	WriteWorkflowCompletionAcceptedFailed WriteWorkflowCompletionVerdict = "accepted_failed"
)

// WriteWorkflowCompletion is the typed terminal verdict for a completed run or
// batch. Status="complete" means the controller has converged; Completion says
// whether that convergence was locally verified, unverified because the
// environment/test surface was unavailable, or explicitly accepted despite a
// failed verification. Consumers must read this field instead of inferring from
// prose or progress-ledger text.
type WriteWorkflowCompletion struct {
	Verdict    WriteWorkflowCompletionVerdict `json:"verdict,omitempty"`
	ReasonCode string                         `json:"reason_code,omitempty"`
	Source     string                         `json:"source,omitempty"`
	At         time.Time                      `json:"at,omitempty"`
}

type WriteWorkflowEdgeKind string

const (
	WriteWorkflowEdgeSeed     WriteWorkflowEdgeKind = "seed"
	WriteWorkflowEdgeExplore  WriteWorkflowEdgeKind = "explore"
	WriteWorkflowEdgePlan     WriteWorkflowEdgeKind = "plan"
	WriteWorkflowEdgeApply    WriteWorkflowEdgeKind = "apply"
	WriteWorkflowEdgeVerify   WriteWorkflowEdgeKind = "verify"
	WriteWorkflowEdgeSplit    WriteWorkflowEdgeKind = "split"
	WriteWorkflowEdgeFollowup WriteWorkflowEdgeKind = "followup"
	WriteWorkflowEdgeBlocked  WriteWorkflowEdgeKind = "blocked"
)

type WriteWorkflowSlice struct {
	ID              string                   `json:"id"`
	Status          ChangePlanSliceStatus    `json:"status,omitempty"`
	PlanID          string                   `json:"plan_id,omitempty"`
	ChangeIndexes   []int                    `json:"change_indexes,omitempty"`
	Paths           []string                 `json:"paths,omitempty"`
	DependsOnSlices []string                 `json:"depends_on_slices,omitempty"`
	ApplyRef        string                   `json:"apply_ref,omitempty"`
	ObserveRef      string                   `json:"observe_ref,omitempty"`
	VerifyRef       string                   `json:"verify_ref,omitempty"`
	ApprovalRef     string                   `json:"approval_ref,omitempty"`
	Checkpoint      *WriteWorkflowCheckpoint `json:"checkpoint,omitempty"`
	Restore         *WriteWorkflowRestore    `json:"restore,omitempty"`
	ContextPackIDs  []string                 `json:"context_pack_ids,omitempty"`
	Completion      *WriteWorkflowCompletion `json:"completion,omitempty"`
	Attempts        []WriteWorkflowAttempt   `json:"attempts,omitempty"`
	CreatedAt       time.Time                `json:"created_at,omitempty"`
	UpdatedAt       time.Time                `json:"updated_at,omitempty"`
}

// WriteWorkflowCheckpoint is durable slice-level restore metadata. It records
// the worktree commit/ref produced by an apply step so future rewind/fork
// flows can preserve verified slices without inferring state from prose or
// transient logs.
type WriteWorkflowCheckpoint struct {
	Ref          string    `json:"ref,omitempty"`
	CommitSHA    string    `json:"commit_sha,omitempty"`
	WorktreePath string    `json:"worktree_path,omitempty"`
	Source       string    `json:"source,omitempty"`
	ReasonCode   string    `json:"reason_code,omitempty"`
	At           time.Time `json:"at,omitempty"`
}

// WriteWorkflowRestore records that a slice resumed from a known checkpoint.
// It is metadata only; the actual reset/fork operation remains owned by the
// deterministic worktree/effect executor.
type WriteWorkflowRestore struct {
	CheckpointRef string    `json:"checkpoint_ref,omitempty"`
	CommitSHA     string    `json:"commit_sha,omitempty"`
	WorktreePath  string    `json:"worktree_path,omitempty"`
	Source        string    `json:"source,omitempty"`
	ReasonCode    string    `json:"reason_code,omitempty"`
	At            time.Time `json:"at,omitempty"`
}

type WriteWorkflowSliceEventKind string

const (
	WriteWorkflowSliceEventPlanned          WriteWorkflowSliceEventKind = "slice_planned"
	WriteWorkflowSliceEventApplyStarted     WriteWorkflowSliceEventKind = "slice_apply_started"
	WriteWorkflowSliceEventApplyCompleted   WriteWorkflowSliceEventKind = "slice_apply_completed"
	WriteWorkflowSliceEventObserveStarted   WriteWorkflowSliceEventKind = "slice_observe_started"
	WriteWorkflowSliceEventObserveCompleted WriteWorkflowSliceEventKind = "slice_observe_completed"
	WriteWorkflowSliceEventVerified         WriteWorkflowSliceEventKind = "slice_verified"
	WriteWorkflowSliceEventUnverified       WriteWorkflowSliceEventKind = "slice_unverified"
	WriteWorkflowSliceEventFailed           WriteWorkflowSliceEventKind = "slice_failed"
	WriteWorkflowSliceEventRestored         WriteWorkflowSliceEventKind = "slice_restored"
	WriteWorkflowSliceEventReplanRequested  WriteWorkflowSliceEventKind = "slice_replan_requested"
	WriteWorkflowSliceEventBlocked          WriteWorkflowSliceEventKind = "slice_blocked"
)

type WriteWorkflowSliceEvent struct {
	SliceID     string                      `json:"slice_id,omitempty"`
	Event       WriteWorkflowSliceEventKind `json:"event,omitempty"`
	Status      ChangePlanSliceStatus       `json:"status,omitempty"`
	ReasonCode  string                      `json:"reason_code,omitempty"`
	PlanID      string                      `json:"plan_id,omitempty"`
	ReportID    string                      `json:"report_id,omitempty"`
	ArtifactRef string                      `json:"artifact_ref,omitempty"`
	SurfaceRef  string                      `json:"surface_ref,omitempty"`
	At          time.Time                   `json:"at,omitempty"`
}

type WriteWorkflowBatch struct {
	ID              string                    `json:"id"`
	Goal            string                    `json:"goal,omitempty"`
	Purpose         string                    `json:"purpose,omitempty"`
	ExpectedPaths   []string                  `json:"expected_paths,omitempty"`
	SuccessCriteria []string                  `json:"success_criteria,omitempty"`
	Status          WriteWorkflowBatchStatus  `json:"status,omitempty"`
	Completion      *WriteWorkflowCompletion  `json:"completion,omitempty"`
	DependsOn       []string                  `json:"depends_on,omitempty"`
	PlanID          string                    `json:"plan_id,omitempty"`
	PlanRef         string                    `json:"plan_ref,omitempty"`
	ApplyRef        string                    `json:"apply_ref,omitempty"`
	VerifyRef       string                    `json:"verify_ref,omitempty"`
	ApprovalRef     string                    `json:"approval_ref,omitempty"`
	ContextPackIDs  []string                  `json:"context_pack_ids,omitempty"`
	Attempts        []WriteWorkflowAttempt    `json:"attempts,omitempty"`
	ActiveSliceID   string                    `json:"active_slice_id,omitempty"`
	Slices          []WriteWorkflowSlice      `json:"slices,omitempty"`
	SliceEvents     []WriteWorkflowSliceEvent `json:"slice_events,omitempty"`
	CreatedAt       time.Time                 `json:"created_at,omitempty"`
	UpdatedAt       time.Time                 `json:"updated_at,omitempty"`
}

type WriteWorkflowAttempt struct {
	ID                string    `json:"id,omitempty"`
	Kind              string    `json:"kind,omitempty"`
	Status            string    `json:"status,omitempty"`
	ReasonCode        string    `json:"reason_code,omitempty"`
	FailureReasonCode string    `json:"failure_reason_code,omitempty"`
	PlanID            string    `json:"plan_id,omitempty"`
	ReportID          string    `json:"report_id,omitempty"`
	ArtifactRef       string    `json:"artifact_ref,omitempty"`
	SurfaceRef        string    `json:"surface_ref,omitempty"`
	StartedAt         time.Time `json:"started_at,omitempty"`
	FinishedAt        time.Time `json:"finished_at,omitempty"`
}

type WriteWorkflowEdge struct {
	FromBatchID string                `json:"from_batch_id,omitempty"`
	ToBatchID   string                `json:"to_batch_id,omitempty"`
	Kind        WriteWorkflowEdgeKind `json:"kind"`
	ReasonCode  string                `json:"reason_code,omitempty"`
}

type WriteWorkflowBudget struct {
	MaxBatches            int `json:"max_batches,omitempty"`
	MaxExplorationRounds  int `json:"max_exploration_rounds,omitempty"`
	BatchesUsed           int `json:"batches_used,omitempty"`
	ExplorationRoundsUsed int `json:"exploration_rounds_used,omitempty"`
}

type WriteWorkflowProgress struct {
	BatchID    string    `json:"batch_id,omitempty"`
	Stage      string    `json:"stage,omitempty"`
	Status     string    `json:"status,omitempty"`
	ReasonCode string    `json:"reason_code,omitempty"`
	Message    string    `json:"message,omitempty"`
	FactKeys   []string  `json:"fact_keys,omitempty"`
	At         time.Time `json:"at,omitempty"`
}

func NormalizeWriteWorkflowRun(in WriteWorkflowRun) WriteWorkflowRun {
	in.RunID = trimWriteWorkflowRunText(in.RunID)
	in.Goal = trimWriteWorkflowRunText(in.Goal)
	in.ActiveBatchID = trimWriteWorkflowRunText(in.ActiveBatchID)
	in.Status = normalizeWriteWorkflowRunStatus(in.Status)
	in.Completion = normalizeWriteWorkflowCompletionPtr(in.Completion)
	if in.Status != WriteWorkflowRunComplete {
		in.Completion = nil
	}
	in.Batches = normalizeWriteWorkflowBatches(in.Batches)
	if in.Status == WriteWorkflowRunComplete && in.Completion == nil {
		in.Completion = inferWriteWorkflowRunCompletionFromBatches(in.Batches)
	}
	in.Edges = normalizeWriteWorkflowEdges(in.Edges)
	in.ContextPacks = normalizeWriteWorkflowContextPacks(in.ContextPacks)
	in.ProgressLedger = normalizeWriteWorkflowProgress(in.ProgressLedger)
	if in.Budget.MaxBatches < 0 {
		in.Budget.MaxBatches = 0
	}
	if in.Budget.MaxExplorationRounds < 0 {
		in.Budget.MaxExplorationRounds = 0
	}
	if in.Budget.BatchesUsed < 0 {
		in.Budget.BatchesUsed = 0
	}
	if in.Budget.ExplorationRoundsUsed < 0 {
		in.Budget.ExplorationRoundsUsed = 0
	}
	return in
}

func CloneWriteWorkflowRun(in WriteWorkflowRun) WriteWorkflowRun {
	return NormalizeWriteWorkflowRun(in)
}

func normalizeWriteWorkflowRunStatus(in WriteWorkflowRunStatus) WriteWorkflowRunStatus {
	switch in {
	case WriteWorkflowRunPlanned, WriteWorkflowRunInProgress, WriteWorkflowRunComplete, WriteWorkflowRunBlocked:
		return in
	default:
		return ""
	}
}

func normalizeWriteWorkflowBatchStatus(in WriteWorkflowBatchStatus) WriteWorkflowBatchStatus {
	switch in {
	case WriteWorkflowBatchNeedsExploration, WriteWorkflowBatchReadyToPlan, WriteWorkflowBatchPlanned,
		WriteWorkflowBatchPendingApproval, WriteWorkflowBatchApplying, WriteWorkflowBatchVerifying,
		WriteWorkflowBatchComplete, WriteWorkflowBatchBlocked:
		return in
	default:
		return ""
	}
}

func normalizeWriteWorkflowCompletionPtr(in *WriteWorkflowCompletion) *WriteWorkflowCompletion {
	if in == nil {
		return nil
	}
	out := normalizeWriteWorkflowCompletion(*in)
	if out.Verdict == "" && out.ReasonCode == "" && out.Source == "" && out.At.IsZero() {
		return nil
	}
	return &out
}

func normalizeWriteWorkflowCompletion(in WriteWorkflowCompletion) WriteWorkflowCompletion {
	in.Verdict = normalizeWriteWorkflowCompletionVerdict(in.Verdict)
	in.ReasonCode = trimWriteWorkflowRunText(in.ReasonCode)
	in.Source = trimWriteWorkflowRunText(in.Source)
	if in.Verdict == "" && in.ReasonCode == "" && in.Source == "" && in.At.IsZero() {
		return WriteWorkflowCompletion{}
	}
	return in
}

func normalizeWriteWorkflowCompletionVerdict(in WriteWorkflowCompletionVerdict) WriteWorkflowCompletionVerdict {
	switch in {
	case WriteWorkflowCompletionVerified, WriteWorkflowCompletionUnverified, WriteWorkflowCompletionAcceptedFailed:
		return in
	default:
		return ""
	}
}

func normalizeWriteWorkflowEdgeKind(in WriteWorkflowEdgeKind) WriteWorkflowEdgeKind {
	switch in {
	case WriteWorkflowEdgeSeed, WriteWorkflowEdgeExplore, WriteWorkflowEdgePlan, WriteWorkflowEdgeApply,
		WriteWorkflowEdgeVerify, WriteWorkflowEdgeSplit, WriteWorkflowEdgeFollowup, WriteWorkflowEdgeBlocked:
		return in
	default:
		return ""
	}
}

func normalizeWriteWorkflowBatches(in []WriteWorkflowBatch) []WriteWorkflowBatch {
	out := make([]WriteWorkflowBatch, 0, len(in))
	for _, batch := range in {
		batch.ID = trimWriteWorkflowRunText(batch.ID)
		if batch.ID == "" {
			continue
		}
		batch.Goal = trimWriteWorkflowRunText(batch.Goal)
		batch.Purpose = trimWriteWorkflowRunText(batch.Purpose)
		batch.ExpectedPaths = dedupTrimWriteWorkflowRunStrings(batch.ExpectedPaths)
		batch.SuccessCriteria = dedupTrimWriteWorkflowRunStrings(batch.SuccessCriteria)
		batch.DependsOn = dedupTrimWriteWorkflowRunStrings(batch.DependsOn)
		batch.PlanID = trimWriteWorkflowRunText(batch.PlanID)
		batch.PlanRef = trimWriteWorkflowRunText(batch.PlanRef)
		batch.ApplyRef = trimWriteWorkflowRunText(batch.ApplyRef)
		batch.VerifyRef = trimWriteWorkflowRunText(batch.VerifyRef)
		batch.ApprovalRef = trimWriteWorkflowRunText(batch.ApprovalRef)
		batch.Status = normalizeWriteWorkflowBatchStatus(batch.Status)
		batch.Completion = normalizeWriteWorkflowCompletionPtr(batch.Completion)
		if batch.Status != WriteWorkflowBatchComplete {
			batch.Completion = nil
		}
		if batch.Status == WriteWorkflowBatchComplete && batch.Completion == nil {
			batch.Completion = inferWriteWorkflowCompletionFromAttempts(batch.Attempts)
		}
		batch.ContextPackIDs = dedupTrimWriteWorkflowRunStrings(batch.ContextPackIDs)
		batch.Attempts = normalizeWriteWorkflowAttempts(batch.Attempts)
		batch.ActiveSliceID = trimWriteWorkflowRunText(batch.ActiveSliceID)
		batch.Slices = normalizeWriteWorkflowSlices(batch.Slices)
		if batch.ActiveSliceID == "" && len(batch.Slices) > 0 {
			batch.ActiveSliceID = batch.Slices[0].ID
		}
		batch.SliceEvents = normalizeWriteWorkflowSliceEvents(batch.SliceEvents)
		out = append(out, batch)
	}
	return out
}

func inferWriteWorkflowCompletionFromAttempts(attempts []WriteWorkflowAttempt) *WriteWorkflowCompletion {
	for i := len(attempts) - 1; i >= 0; i-- {
		attempt := attempts[i]
		if trimWriteWorkflowRunText(attempt.Kind) != "verify" {
			continue
		}
		reason := trimWriteWorkflowRunText(attempt.ReasonCode)
		if reason == "" {
			reason = trimWriteWorkflowRunText(attempt.Status)
		}
		out := WriteWorkflowCompletion{
			ReasonCode: reason,
			Source:     "attempts_backfill",
			At:         attempt.FinishedAt,
		}
		switch trimWriteWorkflowRunText(attempt.Status) {
		case "passed":
			out.Verdict = WriteWorkflowCompletionVerified
		case "unverified":
			out.Verdict = WriteWorkflowCompletionUnverified
		case "failed":
			out.Verdict = WriteWorkflowCompletionAcceptedFailed
		default:
			return nil
		}
		return &out
	}
	return nil
}

func inferWriteWorkflowRunCompletionFromBatches(batches []WriteWorkflowBatch) *WriteWorkflowCompletion {
	sawComplete := false
	out := WriteWorkflowCompletion{
		Verdict:    WriteWorkflowCompletionVerified,
		ReasonCode: "all_batches_verified",
		Source:     "batch_completion_backfill",
	}
	for _, batch := range batches {
		if batch.Status != WriteWorkflowBatchComplete {
			continue
		}
		sawComplete = true
		if batch.Completion == nil {
			out.Verdict = WriteWorkflowCompletionUnverified
			out.ReasonCode = "missing_terminal_verify_verdict"
			continue
		}
		switch batch.Completion.Verdict {
		case WriteWorkflowCompletionAcceptedFailed:
			return &WriteWorkflowCompletion{
				Verdict:    WriteWorkflowCompletionAcceptedFailed,
				ReasonCode: batch.Completion.ReasonCode,
				Source:     "batch_completion_backfill",
				At:         batch.Completion.At,
			}
		case WriteWorkflowCompletionUnverified:
			if out.Verdict != WriteWorkflowCompletionAcceptedFailed {
				out.Verdict = WriteWorkflowCompletionUnverified
				out.ReasonCode = batch.Completion.ReasonCode
				out.At = batch.Completion.At
			}
		case WriteWorkflowCompletionVerified:
			if out.At.IsZero() {
				out.At = batch.Completion.At
			}
		}
	}
	if !sawComplete {
		return nil
	}
	return &out
}

func normalizeWriteWorkflowAttempts(in []WriteWorkflowAttempt) []WriteWorkflowAttempt {
	out := make([]WriteWorkflowAttempt, 0, len(in))
	for _, attempt := range in {
		attempt.ID = trimWriteWorkflowRunText(attempt.ID)
		attempt.Kind = trimWriteWorkflowRunText(attempt.Kind)
		attempt.Status = trimWriteWorkflowRunText(attempt.Status)
		attempt.ReasonCode = trimWriteWorkflowRunText(attempt.ReasonCode)
		attempt.FailureReasonCode = trimWriteWorkflowRunText(attempt.FailureReasonCode)
		attempt.PlanID = trimWriteWorkflowRunText(attempt.PlanID)
		attempt.ReportID = trimWriteWorkflowRunText(attempt.ReportID)
		attempt.ArtifactRef = trimWriteWorkflowRunText(attempt.ArtifactRef)
		attempt.SurfaceRef = trimWriteWorkflowRunText(attempt.SurfaceRef)
		if attempt.ID == "" && attempt.Kind == "" && attempt.Status == "" && attempt.ReasonCode == "" &&
			attempt.FailureReasonCode == "" && attempt.PlanID == "" && attempt.ReportID == "" && attempt.ArtifactRef == "" && attempt.SurfaceRef == "" &&
			attempt.StartedAt.IsZero() && attempt.FinishedAt.IsZero() {
			continue
		}
		out = append(out, attempt)
	}
	return out
}

func normalizeWriteWorkflowSlices(in []WriteWorkflowSlice) []WriteWorkflowSlice {
	out := make([]WriteWorkflowSlice, 0, len(in))
	for _, slice := range in {
		slice.ID = trimWriteWorkflowRunText(slice.ID)
		if slice.ID == "" {
			continue
		}
		slice.Status = normalizeChangePlanSliceStatus(slice.Status)
		if slice.Status == "" {
			slice.Status = ChangePlanSlicePending
		}
		slice.PlanID = trimWriteWorkflowRunText(slice.PlanID)
		slice.ChangeIndexes = normalizeWorkflowSliceIndexes(slice.ChangeIndexes)
		slice.Paths = dedupTrimWriteWorkflowRunStrings(slice.Paths)
		slice.DependsOnSlices = dedupTrimWriteWorkflowRunStrings(slice.DependsOnSlices)
		slice.ApplyRef = trimWriteWorkflowRunText(slice.ApplyRef)
		slice.ObserveRef = trimWriteWorkflowRunText(slice.ObserveRef)
		slice.VerifyRef = trimWriteWorkflowRunText(slice.VerifyRef)
		slice.ApprovalRef = trimWriteWorkflowRunText(slice.ApprovalRef)
		slice.Checkpoint = normalizeWriteWorkflowCheckpointPtr(slice.Checkpoint)
		slice.Restore = normalizeWriteWorkflowRestorePtr(slice.Restore)
		slice.ContextPackIDs = dedupTrimWriteWorkflowRunStrings(slice.ContextPackIDs)
		slice.Completion = normalizeWriteWorkflowCompletionPtr(slice.Completion)
		if slice.Status != ChangePlanSliceVerified &&
			slice.Status != ChangePlanSliceUnverified &&
			slice.Status != ChangePlanSliceFailed &&
			slice.Status != ChangePlanSliceBlocked {
			slice.Completion = nil
		}
		slice.Attempts = normalizeWriteWorkflowAttempts(slice.Attempts)
		out = append(out, slice)
	}
	return out
}

func normalizeWriteWorkflowCheckpointPtr(in *WriteWorkflowCheckpoint) *WriteWorkflowCheckpoint {
	if in == nil {
		return nil
	}
	out := WriteWorkflowCheckpoint{
		Ref:          trimWriteWorkflowRunText(in.Ref),
		CommitSHA:    trimWriteWorkflowRunText(in.CommitSHA),
		WorktreePath: trimWriteWorkflowRunText(in.WorktreePath),
		Source:       trimWriteWorkflowRunText(in.Source),
		ReasonCode:   trimWriteWorkflowRunText(in.ReasonCode),
		At:           in.At,
	}
	if out.Ref == "" && out.CommitSHA == "" && out.WorktreePath == "" && out.Source == "" && out.ReasonCode == "" && out.At.IsZero() {
		return nil
	}
	return &out
}

func normalizeWriteWorkflowRestorePtr(in *WriteWorkflowRestore) *WriteWorkflowRestore {
	if in == nil {
		return nil
	}
	out := WriteWorkflowRestore{
		CheckpointRef: trimWriteWorkflowRunText(in.CheckpointRef),
		CommitSHA:     trimWriteWorkflowRunText(in.CommitSHA),
		WorktreePath:  trimWriteWorkflowRunText(in.WorktreePath),
		Source:        trimWriteWorkflowRunText(in.Source),
		ReasonCode:    trimWriteWorkflowRunText(in.ReasonCode),
		At:            in.At,
	}
	if out.CheckpointRef == "" && out.CommitSHA == "" && out.WorktreePath == "" && out.Source == "" && out.ReasonCode == "" && out.At.IsZero() {
		return nil
	}
	return &out
}

func normalizeWorkflowSliceIndexes(in []int) []int {
	seen := map[int]struct{}{}
	out := make([]int, 0, len(in))
	for _, idx := range in {
		if idx < 0 {
			continue
		}
		if _, ok := seen[idx]; ok {
			continue
		}
		seen[idx] = struct{}{}
		out = append(out, idx)
	}
	return out
}

func normalizeWriteWorkflowSliceEvents(in []WriteWorkflowSliceEvent) []WriteWorkflowSliceEvent {
	out := make([]WriteWorkflowSliceEvent, 0, len(in))
	for _, event := range in {
		event.SliceID = trimWriteWorkflowRunText(event.SliceID)
		event.Event = normalizeWriteWorkflowSliceEventKind(event.Event)
		event.Status = normalizeChangePlanSliceStatus(event.Status)
		event.ReasonCode = trimWriteWorkflowRunText(event.ReasonCode)
		event.PlanID = trimWriteWorkflowRunText(event.PlanID)
		event.ReportID = trimWriteWorkflowRunText(event.ReportID)
		event.ArtifactRef = trimWriteWorkflowRunText(event.ArtifactRef)
		event.SurfaceRef = trimWriteWorkflowRunText(event.SurfaceRef)
		if event.SliceID == "" && event.Event == "" && event.Status == "" && event.ReasonCode == "" &&
			event.PlanID == "" && event.ReportID == "" && event.ArtifactRef == "" && event.SurfaceRef == "" &&
			event.At.IsZero() {
			continue
		}
		out = append(out, event)
	}
	return out
}

func normalizeWriteWorkflowSliceEventKind(in WriteWorkflowSliceEventKind) WriteWorkflowSliceEventKind {
	switch in {
	case WriteWorkflowSliceEventPlanned, WriteWorkflowSliceEventApplyStarted,
		WriteWorkflowSliceEventApplyCompleted, WriteWorkflowSliceEventObserveStarted,
		WriteWorkflowSliceEventObserveCompleted, WriteWorkflowSliceEventVerified,
		WriteWorkflowSliceEventUnverified, WriteWorkflowSliceEventFailed, WriteWorkflowSliceEventRestored,
		WriteWorkflowSliceEventReplanRequested, WriteWorkflowSliceEventBlocked:
		return in
	default:
		return ""
	}
}

func normalizeWriteWorkflowEdges(in []WriteWorkflowEdge) []WriteWorkflowEdge {
	out := make([]WriteWorkflowEdge, 0, len(in))
	for _, edge := range in {
		edge.FromBatchID = trimWriteWorkflowRunText(edge.FromBatchID)
		edge.ToBatchID = trimWriteWorkflowRunText(edge.ToBatchID)
		edge.Kind = normalizeWriteWorkflowEdgeKind(edge.Kind)
		edge.ReasonCode = trimWriteWorkflowRunText(edge.ReasonCode)
		if edge.Kind == "" {
			continue
		}
		out = append(out, edge)
	}
	return out
}

func normalizeWriteWorkflowContextPacks(in []WriteContextPack) []WriteContextPack {
	out := make([]WriteContextPack, 0, len(in))
	for _, pack := range in {
		pack = NormalizeWriteContextPack(pack)
		if pack.PackID == "" && len(pack.Items) == 0 {
			continue
		}
		out = append(out, pack)
	}
	return out
}

func normalizeWriteWorkflowProgress(in []WriteWorkflowProgress) []WriteWorkflowProgress {
	out := make([]WriteWorkflowProgress, 0, len(in))
	for _, item := range in {
		item.BatchID = trimWriteWorkflowRunText(item.BatchID)
		item.Stage = trimWriteWorkflowRunText(item.Stage)
		item.Status = trimWriteWorkflowRunText(item.Status)
		item.ReasonCode = trimWriteWorkflowRunText(item.ReasonCode)
		item.Message = trimWriteWorkflowRunText(item.Message)
		item.FactKeys = dedupTrimWriteWorkflowRunStrings(item.FactKeys)
		if item.BatchID == "" && item.Stage == "" && item.Status == "" && item.ReasonCode == "" && item.Message == "" && len(item.FactKeys) == 0 && item.At.IsZero() {
			continue
		}
		out = append(out, item)
	}
	return out
}

func trimWriteWorkflowRunText(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	return strings.Join(strings.Fields(raw), " ")
}

func dedupTrimWriteWorkflowRunStrings(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, value := range in {
		value = trimWriteWorkflowRunText(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}
