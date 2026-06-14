package types

import (
	"strings"
	"time"
)

// WriteWorkflowRun is the outer write-controller persistence envelope. Batch 3
// introduces the schema so priority context packs have a durable home; Batch 4
// adds the store and controller that write this object.
type WriteWorkflowRun struct {
	RunID          string                  `json:"run_id"`
	Goal           string                  `json:"goal,omitempty"`
	Status         WriteWorkflowRunStatus  `json:"status,omitempty"`
	ActiveBatchID  string                  `json:"active_batch_id,omitempty"`
	CreatedAt      time.Time               `json:"created_at,omitempty"`
	UpdatedAt      time.Time               `json:"updated_at,omitempty"`
	Batches        []WriteWorkflowBatch    `json:"batches,omitempty"`
	Edges          []WriteWorkflowEdge     `json:"edges,omitempty"`
	ContextPacks   []WriteContextPack      `json:"context_packs,omitempty"`
	Budget         WriteWorkflowBudget     `json:"budget,omitempty"`
	ProgressLedger []WriteWorkflowProgress `json:"progress_ledger,omitempty"`
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

type WriteWorkflowBatch struct {
	ID             string                   `json:"id"`
	Goal           string                   `json:"goal,omitempty"`
	Status         WriteWorkflowBatchStatus `json:"status,omitempty"`
	DependsOn      []string                 `json:"depends_on,omitempty"`
	PlanID         string                   `json:"plan_id,omitempty"`
	PlanRef        string                   `json:"plan_ref,omitempty"`
	ApplyRef       string                   `json:"apply_ref,omitempty"`
	VerifyRef      string                   `json:"verify_ref,omitempty"`
	ApprovalRef    string                   `json:"approval_ref,omitempty"`
	ContextPackIDs []string                 `json:"context_pack_ids,omitempty"`
	Attempts       []WriteWorkflowAttempt   `json:"attempts,omitempty"`
	CreatedAt      time.Time                `json:"created_at,omitempty"`
	UpdatedAt      time.Time                `json:"updated_at,omitempty"`
}

type WriteWorkflowAttempt struct {
	ID          string    `json:"id,omitempty"`
	Kind        string    `json:"kind,omitempty"`
	Status      string    `json:"status,omitempty"`
	ReasonCode  string    `json:"reason_code,omitempty"`
	PlanID      string    `json:"plan_id,omitempty"`
	ReportID    string    `json:"report_id,omitempty"`
	ArtifactRef string    `json:"artifact_ref,omitempty"`
	SurfaceRef  string    `json:"surface_ref,omitempty"`
	StartedAt   time.Time `json:"started_at,omitempty"`
	FinishedAt  time.Time `json:"finished_at,omitempty"`
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
	At         time.Time `json:"at,omitempty"`
}

func NormalizeWriteWorkflowRun(in WriteWorkflowRun) WriteWorkflowRun {
	in.RunID = trimWriteWorkflowRunText(in.RunID)
	in.Goal = trimWriteWorkflowRunText(in.Goal)
	in.ActiveBatchID = trimWriteWorkflowRunText(in.ActiveBatchID)
	in.Status = normalizeWriteWorkflowRunStatus(in.Status)
	in.Batches = normalizeWriteWorkflowBatches(in.Batches)
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
		batch.DependsOn = dedupTrimWriteWorkflowRunStrings(batch.DependsOn)
		batch.PlanID = trimWriteWorkflowRunText(batch.PlanID)
		batch.PlanRef = trimWriteWorkflowRunText(batch.PlanRef)
		batch.ApplyRef = trimWriteWorkflowRunText(batch.ApplyRef)
		batch.VerifyRef = trimWriteWorkflowRunText(batch.VerifyRef)
		batch.ApprovalRef = trimWriteWorkflowRunText(batch.ApprovalRef)
		batch.Status = normalizeWriteWorkflowBatchStatus(batch.Status)
		batch.ContextPackIDs = dedupTrimWriteWorkflowRunStrings(batch.ContextPackIDs)
		batch.Attempts = normalizeWriteWorkflowAttempts(batch.Attempts)
		out = append(out, batch)
	}
	return out
}

func normalizeWriteWorkflowAttempts(in []WriteWorkflowAttempt) []WriteWorkflowAttempt {
	out := make([]WriteWorkflowAttempt, 0, len(in))
	for _, attempt := range in {
		attempt.ID = trimWriteWorkflowRunText(attempt.ID)
		attempt.Kind = trimWriteWorkflowRunText(attempt.Kind)
		attempt.Status = trimWriteWorkflowRunText(attempt.Status)
		attempt.ReasonCode = trimWriteWorkflowRunText(attempt.ReasonCode)
		attempt.PlanID = trimWriteWorkflowRunText(attempt.PlanID)
		attempt.ReportID = trimWriteWorkflowRunText(attempt.ReportID)
		attempt.ArtifactRef = trimWriteWorkflowRunText(attempt.ArtifactRef)
		attempt.SurfaceRef = trimWriteWorkflowRunText(attempt.SurfaceRef)
		if attempt.ID == "" && attempt.Kind == "" && attempt.Status == "" && attempt.ReasonCode == "" &&
			attempt.PlanID == "" && attempt.ReportID == "" && attempt.ArtifactRef == "" && attempt.SurfaceRef == "" &&
			attempt.StartedAt.IsZero() && attempt.FinishedAt.IsZero() {
			continue
		}
		out = append(out, attempt)
	}
	return out
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
		if item.BatchID == "" && item.Stage == "" && item.Status == "" && item.ReasonCode == "" && item.Message == "" && item.At.IsZero() {
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
