package types

import "time"

const (
	ReplanWorktreePathPresent    = "present"
	ReplanWorktreePathMissing    = "missing"
	ReplanWorktreePathUnreadable = "unreadable"
)

// ReplanAppliedEditReceipt is a bounded, exact line receipt from the applied
// PatchEffect. Text is carried as data for the planner; it is not interpreted
// as a routing or validation signal.
type ReplanAppliedEditReceipt struct {
	Kind         string `json:"kind,omitempty"`
	Line         int    `json:"line,omitempty"`
	Text         string `json:"text,omitempty"`
	TextBytes    int    `json:"text_bytes,omitempty"`
	TextSHA256   string `json:"text_sha256,omitempty"`
	TextComplete bool   `json:"text_complete"`
}

// ReplanCurrentPathState binds one previously applied path to the bytes that
// are actually present in the mutable worktree at the start of a replan.
// CurrentSHA256 is over the complete current file. AppliedEdits are exact
// PatchEffect lines and CurrentSourceSnapshots are fresh bounded windows read
// from that same file generation.
type ReplanCurrentPathState struct {
	Path                   string                     `json:"path,omitempty"`
	State                  string                     `json:"state,omitempty"`
	CurrentSHA256          string                     `json:"current_sha256,omitempty"`
	CurrentBytes           int                        `json:"current_bytes,omitempty"`
	AppliedEditTotal       int                        `json:"applied_edit_total,omitempty"`
	AppliedEditComplete    bool                       `json:"applied_edit_complete"`
	AppliedEdits           []ReplanAppliedEditReceipt `json:"applied_edits,omitempty"`
	CurrentSourceSnapshots []RepairSourceSnapshot     `json:"current_source_snapshots,omitempty"`
}

// ReplanCurrentWorktreeReceipt is the typed current-state handoff for a
// replan after an earlier plan has already mutated the isolated worktree. It
// deliberately separates original-repository context from current bytes: a
// planner can use the former to understand intent, but must construct the next
// patch against the latter. The scheduler creates this receipt only from a
// ChangePlan PatchEffect, durable workflow attempts, and filesystem bytes.
type ReplanCurrentWorktreeReceipt struct {
	BatchID             string                   `json:"batch_id,omitempty"`
	SourcePlanID        string                   `json:"source_plan_id,omitempty"`
	ApplyGeneration     int                      `json:"apply_generation,omitempty"`
	PatchEffectRecordID string                   `json:"patch_effect_record_id,omitempty"`
	DiffFingerprint     string                   `json:"diff_fingerprint,omitempty"`
	TriggerReasonCode   string                   `json:"trigger_reason_code,omitempty"`
	Paths               []ReplanCurrentPathState `json:"paths,omitempty"`
	GeneratedAt         time.Time                `json:"generated_at,omitempty"`
}
