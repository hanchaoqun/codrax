package types

import "time"

// PlanGroup is the multi-phase write-mode container introduced
// in stage II. A sequential PhaseProposal in WriteAnalysisIR
// triggers PlanGroup creation; the orchestrator's runPhaseGroup
// then drives each phase through plan→apply→verify in turn,
// gating advancement on per-phase acceptance check.
//
// Single-phase tasks (split=single OR no IR) bypass the group
// layer entirely and use the existing 3-node TaskGraph chain.
// PlanGroup persistence lives at
//
//	<runtimeAnchor>/plans/groups/<group-id>.json
//
// alongside the per-phase ChangePlan files in
//
//	<runtimeAnchor>/plans/<plan-id>.json
type PlanGroup struct {
	// ID is the canonical identifier for this group:
	//   group-<unix-nano>-<pid>
	ID string `json:"id"`

	// Goal carries WriteAnalysisIR.Request.RawRequest verbatim
	// so the on-disk group file is self-describing without
	// having to cross-reference the parent ChangePlans.
	Goal string `json:"goal"`

	// CreatedAt timestamps group creation; PhaseRecord carries
	// per-phase started/finished timestamps separately.
	CreatedAt time.Time `json:"created_at"`

	// Status is the group-level lifecycle position. See
	// PlanGroupStatus constants.
	Status PlanGroupStatus `json:"status"`

	// Phases is the ordered slice of per-phase records. Index
	// 0 is the first phase to execute. The slice never grows
	// after group creation — phase add/remove would reset
	// orchestrator scheduling state, which we explicitly
	// disallow within stage II's "linear" decision shape.
	Phases []PhaseRecord `json:"phases"`

	// ActiveIdx points at the phase currently in flight (or
	// next to enter, when ActiveIdx == len(Phases) the group
	// has exhausted its phases and Status flips to
	// PlanGroupCompleted). Persisted on every transition so
	// crash recovery resumes from the right phase.
	ActiveIdx int `json:"active_idx"`

	// Decision names the group's branching shape. Stage II only
	// supports "linear"; future stages may add "tree" / "fan-
	// out" but those are out of scope for this rollout. Storing
	// the decision lets a future loader detect an
	// unsupported-shape group on disk and refuse with a clear
	// error instead of silently misinterpreting it.
	Decision string `json:"decision"`
}

// PhaseRecord is one phase's persisted state. Each phase emits
// its own ChangePlan (PlanID below ties back to that file in
// the per-plan store). The acceptance check verdict gates
// advancement to the next phase; a rejected phase transitions
// the entire group to PlanGroupFailed.
type PhaseRecord struct {
	// Index is the 0-based position of this phase within the
	// parent PlanGroup.Phases slice. Stored explicitly so a
	// phase can be inspected without holding a reference to
	// its parent.
	Index int `json:"index"`

	// Goal is the per-phase objective string the LLM emitted
	// in PhaseProposal.Phases[i].Goal — what this phase is
	// supposed to accomplish.
	Goal string `json:"goal"`

	// RoughTargetPaths carries the LLM's per-phase suggested
	// files (from PhaseProposal.Phases[i].RoughTargetPaths).
	// Seeded into Mutable.PlanningHint when the phase enters
	// plan stage so the planner has a starting point. Empty
	// after the phase's planner emits — the real authoritative
	// list lives on ChangePlan.TargetPaths.
	RoughTargetPaths []string `json:"rough_target_paths,omitempty"`

	// Status is the phase-level lifecycle position. See
	// PhaseStatus constants.
	Status PhaseStatus `json:"status"`

	// PlanID is the ChangePlan.ID this phase emitted (after
	// the planner runs). Empty until the phase reaches plan
	// stage. Lets /phase show cross-link to the per-plan file.
	PlanID string `json:"plan_id,omitempty"`

	// AppliedSHA is the worktree git commit SHA after this
	// phase's apply succeeded. Used by /phase rollback to
	// reset the worktree to a prior phase's state, and by the
	// next phase's apply baseline (the next phase begins from
	// this commit).
	AppliedSHA string `json:"applied_sha,omitempty"`

	// AcceptanceCheck records the LLM verdict on whether this
	// phase satisfied its Goal. Populated after verify passes
	// and the acceptance check dispatch returns. Nil when the
	// phase has not yet reached the acceptance step OR when
	// the AcceptanceChecker is not configured (degraded path
	// — phase auto-advances).
	AcceptanceCheck *AcceptanceCheck `json:"acceptance_check,omitempty"`

	// StartedAt / FinishedAt timestamps. Pointer-typed because
	// "phase has not started" / "phase still in flight" are
	// distinguishable states.
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`

	// RetryAttempts counts the number of verify→plan retry
	// cycles this phase consumed before reaching its current
	// status. Zero means "passed on first try"; 3 means
	// "needed 3 retries". Useful post-mortem signal when
	// reading /phase show — distinguishes "this phase was
	// hard" from "this phase passed cleanly". Mutated by
	// clearForReplan when a verify SC failure requeues the
	// plan stage.
	RetryAttempts int `json:"retry_attempts,omitempty"`

	// OwnerPID is the orchestrator process id that set this
	// phase to in_progress. Used by FindActiveGroup's orphan
	// reaper: when a process crashes mid-phase (SIGKILL / panic
	// / OOM) the phase stays at in_progress on disk forever, so
	// the next REPL session needs a way to detect "the
	// orchestrator that owned this phase is dead" and surface
	// a recoverable state to the operator instead of silently
	// resuming a half-finished group. Zero on phases set by
	// the operator (`/phase next` / `/phase skip`) — those
	// don't belong to a running orchestrator.
	OwnerPID int `json:"owner_pid,omitempty"`

	// WorkflowEvaluation is the deterministic write-workflow verdict recorded
	// after this phase's inner plan/apply/verify batch settles. It is a typed
	// scheduler fact, not LLM prose: downstream REPL/history views can inspect
	// whether the batch looked complete, needed another plan, needed approval,
	// or hit a danger-deny condition without re-parsing summaries.
	WorkflowEvaluation *WriteWorkflowEvaluationSnapshot `json:"workflow_evaluation,omitempty"`
}

// WriteWorkflowEvaluationSnapshot is the persisted phase-level view of
// writeflow.WriteEvaluation. Kept in types (instead of importing writeflow) so
// PlanGroup remains a low-level persistence object.
type WriteWorkflowEvaluationSnapshot struct {
	Status         string `json:"status"`
	ReasonCode     string `json:"reason_code,omitempty"`
	Reason         string `json:"reason,omitempty"`
	NextBatchID    string `json:"next_batch_id,omitempty"`
	RiskLevel      string `json:"risk_level,omitempty"`
	ApprovalAction string `json:"approval_action,omitempty"`
}

// AcceptanceCheck is the per-phase "did this phase actually
// satisfy its Goal" judgement. Distinct from CritTestsPass:
// tests passing means "no test regressed", not "the phase
// achieved its stated objective". A rejected check is
// load-bearing — it transitions the group to PlanGroupFailed
// rather than silently advancing to a phase whose precondition
// (the previous phase's work) wasn't actually met.
//
// One LLM call per phase (cheap-model routing recommended; the
// input is bounded). Failure paths in the dispatch (Chat error,
// no tool call, malformed JSON) are degraded to accept: don't
// block phase advance just because the check itself
// misbehaved.
type AcceptanceCheck struct {
	// Passed is the boolean verdict.
	Passed bool `json:"passed"`

	// Reasoning is the LLM's 1-2 sentence justification,
	// citing evidence from the test report or plan summary.
	// Rendered verbatim in /phase show.
	Reasoning string `json:"reasoning"`

	// NextHint is an optional 1-sentence carry-over the
	// reviewer can leave for the NEXT phase's planner — what
	// context the just-finished phase established. Empty when
	// the reviewer didn't think a hand-off note was useful.
	NextHint string `json:"next_hint,omitempty"`
}

// PlanGroupStatus is the closed set of group-level lifecycle
// positions. The orchestrator persists transitions atomically
// via PlanGroupStore.
type PlanGroupStatus string

const (
	// PlanGroupPlanning: group exists but the first phase has
	// not yet entered the apply stage. Initial state after
	// buildPlanGroupFromProposal.
	PlanGroupPlanning PlanGroupStatus = "planning"

	// PlanGroupInFlight: at least one phase has reached
	// PhaseAccepted; phase scheduler is iterating through
	// remaining phases.
	PlanGroupInFlight PlanGroupStatus = "in_flight"

	// PlanGroupCompleted: every phase reached PhaseAccepted.
	// Terminal — group is settled.
	PlanGroupCompleted PlanGroupStatus = "completed"

	// PlanGroupFailed: a phase exhausted retries OR its
	// acceptance check rejected. Terminal — operator action
	// required (typically /phase rollback or /reject of the
	// active phase's plan).
	PlanGroupFailed PlanGroupStatus = "failed"

	// PlanGroupRolledBack: operator-driven rollback via
	// /phase rollback walked the group back past phase 0.
	// Terminal — worktree is at pre-group state.
	PlanGroupRolledBack PlanGroupStatus = "rolled_back"
)

// IsTerminalGroupStatus reports whether the group has settled
// and won't change without explicit operator action.
func IsTerminalGroupStatus(s PlanGroupStatus) bool {
	switch s {
	case PlanGroupCompleted, PlanGroupFailed, PlanGroupRolledBack:
		return true
	}
	return false
}

// PhaseLivenessChecker abstracts the OS pid-liveness probe so
// callers can stub it in tests. Production wiring uses
// internal/logging.IsPidAlive (cross-platform); tests inject
// a deterministic answer. Zero-pid inputs MUST return true so
// operator-driven phase transitions (no OwnerPID stamp) don't
// trip the orphan reaper.
type PhaseLivenessChecker func(pid int) bool

// IsOrphanedActivePhase reports whether the named phase has a
// stale OwnerPID — set to in_progress by a process that is no
// longer alive. Callers route through PhaseLivenessChecker so
// tests can deterministically answer "pid alive / dead". When
// OwnerPID is zero the phase didn't claim ownership (operator
// /phase next / /phase skip set the status) and the function
// returns false unconditionally.
func IsOrphanedActivePhase(phase *PhaseRecord, alive PhaseLivenessChecker) bool {
	if phase == nil || alive == nil {
		return false
	}
	if phase.Status != PhaseInProgress {
		return false
	}
	if phase.OwnerPID == 0 {
		return false
	}
	return !alive(phase.OwnerPID)
}

// PhaseStatus is the closed set of per-phase lifecycle
// positions.
type PhaseStatus string

const (
	// PhasePending: phase exists in the group but the
	// scheduler hasn't entered it yet.
	PhasePending PhaseStatus = "pending"

	// PhaseInProgress: scheduler entered the phase; one of
	// plan / apply / verify is dispatching.
	PhaseInProgress PhaseStatus = "in_progress"

	// PhaseApplied: apply landed bytes for this phase. Verify
	// has not yet completed. Transient state.
	PhaseApplied PhaseStatus = "applied"

	// PhaseVerified: verify passed for this phase. Acceptance
	// check has not yet completed. Transient state.
	PhaseVerified PhaseStatus = "verified"

	// PhaseAccepted: acceptance check passed. Terminal for
	// this phase; scheduler advances ActiveIdx.
	PhaseAccepted PhaseStatus = "accepted"

	// PhaseRolledBack: phase failed (retries exhausted OR
	// acceptance rejected) AND the group decided to undo it.
	// Worktree is at the previous phase's AppliedSHA.
	// Terminal.
	PhaseRolledBack PhaseStatus = "rolled_back"

	// PhaseSkipped: operator marked this phase via
	// /phase skip — the work isn't needed for whatever
	// reason. Terminal; scheduler advances past it.
	PhaseSkipped PhaseStatus = "skipped"

	// PhaseAcceptanceUnverified: tests passed and verify ran
	// cleanly, but the acceptance-check LLM call errored (no
	// adapter configured, network blip, timeout, malformed
	// emit). Distinct from PhaseAccepted so the operator sees
	// the gap in /phase show — fail-open auto-accept would
	// silently mask infra failures and let the group complete
	// while a phase was effectively un-judged. Treated as
	// terminal so the scheduler advances rather than
	// deadlocking on infrastructure outside the phase's own
	// correctness; the group still reaches Completed status,
	// but /phase show prints "acceptance: unverified" alongside
	// the recorded error message so operators can intervene.
	PhaseAcceptanceUnverified PhaseStatus = "acceptance_unverified"
)

// IsTerminalPhaseStatus reports terminal phase states (won't
// change without explicit operator action). The scheduler skips
// terminal phases when iterating ActiveIdx forward.
func IsTerminalPhaseStatus(s PhaseStatus) bool {
	switch s {
	case PhaseAccepted, PhaseRolledBack, PhaseSkipped, PhaseAcceptanceUnverified:
		return true
	}
	return false
}

// MaxPhasesPerGroup is the default cap on phase count.
// emit_write_analysis schema enforces this at LLM-emit time so
// a misbehaving LLM cannot create a 50-phase group that would
// burn LLM budget across pointless micro-divisions. The number
// is empirical: real-world multi-phase tasks (schema migration
// + code change, library extraction + adoption, deprecate +
// remove) are usually 2-3 phases; 5 covers the long tail; >5
// almost always means the LLM mis-classified what should be a
// single phase as multiple. Operators can override via
// `codrax.yaml :: pipeline_max_phases_per_run` (clamped to
// MaxPhasesPerGroupCeil) when their task structure genuinely
// needs more.
const MaxPhasesPerGroup = 5

// MaxPhasesPerGroupCeil is the absolute ceiling on the
// pipeline_max_phases_per_run yaml override. Sized so a
// runaway LLM can't burn through a 30-phase fan-out even when
// the operator deliberately raised the soft cap. SetMaxPhases
// enforces this clamp.
const MaxPhasesPerGroupCeil = 12

// resolvedMaxPhases is the per-Run cap; defaults to
// MaxPhasesPerGroup but may be raised by SetMaxPhases up to
// MaxPhasesPerGroupCeil. Read by emit_write_analysis truncation.
var resolvedMaxPhases = MaxPhasesPerGroup

// ResolvedMaxPhases returns the active cap. Mirror of the
// other "default with yaml override" patterns in the codebase
// so emit_write_analysis can read a single source of truth
// without depending on the orchestrator package.
func ResolvedMaxPhases() int {
	if resolvedMaxPhases <= 0 {
		return MaxPhasesPerGroup
	}
	return resolvedMaxPhases
}

// SetMaxPhases installs the operator's pipeline_max_phases_per_run
// override. Zero / negative inputs reset to the default.
// Values above MaxPhasesPerGroupCeil are clamped to the ceiling
// (defense against a misconfigured yaml that would let a
// runaway LLM proposal through).
func SetMaxPhases(n int) {
	if n <= 0 {
		resolvedMaxPhases = MaxPhasesPerGroup
		return
	}
	if n > MaxPhasesPerGroupCeil {
		n = MaxPhasesPerGroupCeil
	}
	resolvedMaxPhases = n
}
