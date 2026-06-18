package types

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Plan status enum — legal values of ChangePlan.Status. The plan
// moves through these monotonically during a single apply Run;
// /plan list reads the string back to colour each row. Values are
// baked into the JSON schema so external tools (CI hooks that
// scan .codrax/plans/ for auditing) can filter deterministically.
const (
	// PlanStatusPending is the plan's initial state emitted by the
	// planner. Auto Pilot may apply it automatically when deterministic
	// risk policy returns allow; high-risk plans remain pending until an
	// approval record is written.
	PlanStatusPending = "pending_approval"
	// PlanStatusAppliedPendingVerify means apply landed every declared
	// change and the active workflow is waiting for the post-apply verifier
	// verdict. This keeps approval state ("pending_approval") separate from
	// execution state in controller prompts and durable workflow records.
	PlanStatusAppliedPendingVerify = "applied_pending_verify"
	// PlanStatusApplied means apply + verify both succeeded.
	PlanStatusApplied = "applied"
	// PlanStatusApplyFailed means apply_patch rejected a change
	// (W1 / W1b violation, git-apply refusal, stat error). Verify
	// never ran.
	PlanStatusApplyFailed = "applied_failed"
	// PlanStatusBlocked means a deterministic pre-apply policy gate
	// rejected the plan before any worktree write was attempted.
	// It is terminal and settled: the operator must create a new plan
	// rather than approve this one.
	PlanStatusBlocked = "blocked"
	// PlanStatusVerifyFailed means apply landed but verify's tests
	// failed. Distinct from applied_failed so operators can tell
	// "plan was broken" from "plan applied but surface regressed".
	PlanStatusVerifyFailed = "verify_failed"
	// PlanStatusUnverified means apply landed but local verification did
	// not produce a validating assertion. This covers environment/tooling
	// gaps such as FailureKindRunnerMissing (for example pytest missing in
	// a customer checkout) and zero-test discovery for runtime code. The
	// plan's bytes are on disk in the worktree, but no local test verdict
	// proved they work. Distinct from applied ("tests passed") and from
	// verify_failed ("tests/build ran and found a code failure").
	PlanStatusUnverified = "unverified"
	// PlanStatusPartiallyApplied means the apply stage hit a
	// rejection partway through plan.Changes — some files made it
	// to the worktree, some did not. Distinct from applied_failed
	// (apply produced zero successful units; the worktree is
	// untouched) and from applied (every TargetPath landed). The
	// state is unsettled: the operator decides whether to /reject
	// (discard the partial state along with the worktree) or
	// /approve --retry (re-plan from current AppliedSet to
	// finish the work). /merge is blocked on this status because
	// half-merging would land an incoherent diff onto main.
	PlanStatusPartiallyApplied = "partially_applied"
	// PlanStatusRejected is set by /reject — user reviewed the
	// plan and decided not to approve it.
	PlanStatusRejected = "rejected"
	// PlanStatusNoChangeRequired is an internal controller sentinel emitted
	// only during a replan round when the current applied worktree already
	// satisfies a typed planner probe. It is never applied as an empty patch:
	// the controller restores the prior applied plan and moves back to verify.
	PlanStatusNoChangeRequired = "no_change_required"
	// PlanStatusMerged is set by /merge — the applied plan's
	// commit(s) have been folded back into the main repo. Together
	// with PlanStatusRejected this completes the lifecycle: a plan
	// in either of these terminal states is "settled" and a new
	// plan can be created in the same project. Pre-2026-04-30 the
	// merged transition only flipped MergedAt timestamp without
	// changing Status; that left applied plans hanging as
	// "unsettled" forever from the lifecycle's point of view.
	PlanStatusMerged = "merged"
)

// IsUnsettledStatus reports whether a plan in `s` is still in the
// active lifecycle and would block creation of a NEW plan in the
// same project. The single-pending-plan invariant relies on this
// predicate at three layers:
//
//   - PlanStore.Save (data layer hard constraint)
//   - REPL /mode write switch (UX layer rejection)
//   - REPL banner (cold-start awareness)
//
// pending_approval / applied_pending_verify / applied / verify_failed / unverified /
// partially_applied are unsettled. merged / rejected /
// applied_failed / blocked are settled (terminal). applied_failed is
// terminal because apply itself crashed — nothing landed, the
// user moves on; treating it as unsettled would block forever on a
// dead plan. Unverified is unsettled because the user still needs
// to decide whether to add tests (verify the plan retroactively)
// or accept the work as-is (which transitions to merged via
// /merge). PartiallyApplied is unsettled because the worktree
// holds a partial diff — the operator must /reject to discard it
// or /approve --retry to finish the work.
func IsUnsettledStatus(s string) bool {
	switch s {
	case PlanStatusPending, PlanStatusAppliedPendingVerify, PlanStatusApplied, PlanStatusVerifyFailed, PlanStatusUnverified, PlanStatusPartiallyApplied:
		return true
	}
	return false
}

// RenderIterationLedger builds the "## Iteration history" prompt
// section for the planner from the per-Run ledger. Each record is
// rendered verbatim (no truncation, no system pre-classification);
// when a FailureSummary is too large to inline cleanly, the section
// surfaces the inline text PLUS a blob ref pointer so the planner
// can call read_file with offset/limit to page through the rest.
//
// Empty result when the ledger is empty (FIRST plan attempt has no
// prior history) so the planner falls through to its workflow.
//
// This is a pure rendering function — exported so both the planner
// (internal/agent) and any future test harness can call it without
// cross-package dependencies.
func RenderIterationLedger(ledger []IterationRecord) string {
	if len(ledger) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Iteration history\n\n")
	b.WriteString("Each entry below is a completed verify→plan attempt within THIS Run, in append order. Read the entries fully — including raw failure summaries — to decide what to try next. The data is verbatim; no system pre-classification.\n\n")
	for _, rec := range ledger {
		fmt.Fprintf(&b, "### Attempt %d", rec.Attempt)
		if rec.PlanID != "" {
			fmt.Fprintf(&b, " (plan %s)", rec.PlanID)
		}
		b.WriteString("\n\n")
		if rec.PassedCount+rec.FailedCount > 0 {
			fmt.Fprintf(&b, "Tests: %d passed, %d failed.\n\n", rec.PassedCount, rec.FailedCount)
		}
		if rec.PlanSummary != "" {
			b.WriteString("Plan summary (verbatim):\n")
			b.WriteString(rec.PlanSummary)
			b.WriteString("\n\n")
		}
		if len(rec.ChangedFiles) > 0 {
			b.WriteString("Files this attempt's plan changed:\n")
			for _, p := range rec.ChangedFiles {
				fmt.Fprintf(&b, "  - %s\n", p)
			}
			b.WriteString("\n")
		}
		if rec.FailureSummary != "" {
			b.WriteString("Failure summary (verbatim runner output):\n")
			b.WriteString(rec.FailureSummary)
			b.WriteString("\n")
			if rec.FailureSummaryBlobRef != "" {
				fmt.Fprintf(&b, "(Full stderr available at %s — call read_file with offset/limit to page through.)\n", rec.FailureSummaryBlobRef)
			}
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// IterationRecord is one row of the per-Run iteration ledger
// (Module C). The orchestrator's clearForReplan appends one record
// per completed verify→plan retry attempt BEFORE resetting Mutable
// state, so the planner on the next retry reads the FULL history.
//
// Every field is raw data (no system pre-classification): the
// planner reads the actual failure summary, the actual changed
// files, the actual pass/fail counts. When the failure summary is
// too large to embed inline, FailureSummaryBlobRef points at the
// blob path so the planner can read_file the full content.
//
// This is the Reflexion (Shinn et al. 2023) episodic-memory shape
// adapted for write mode: each retry is a memory entry; the planner
// is the actor that reads them and decides what to try next.
type IterationRecord struct {
	// Attempt is 1-indexed (first retry produces Attempt=1, second
	// produces Attempt=2, …).
	Attempt int `json:"attempt"`

	// PlanID identifies which plan this attempt's apply tried to
	// land. Empty when the attempt failed before a plan was emitted.
	PlanID string `json:"plan_id,omitempty"`

	// PlanSummary is the verbatim summary text the planner wrote
	// for this attempt's plan. Not truncated.
	PlanSummary string `json:"plan_summary,omitempty"`

	// ChangedFiles lists the repo-relative paths the plan declared
	// in TargetPaths.
	ChangedFiles []string `json:"changed_files,omitempty"`

	// PassedCount + FailedCount are the verify-stage test counts.
	PassedCount int `json:"passed_count"`
	FailedCount int `json:"failed_count"`

	// FailureSummary is the runner's verbatim summary. NOT truncated.
	// When too large it gets blobbed and the ref goes into
	// FailureSummaryBlobRef below.
	FailureSummary string `json:"failure_summary,omitempty"`

	// FailureSummaryBlobRef is the blob path produced by run_tests
	// (or qualify) when the full stderr exceeds the inline cap.
	// Empty when the summary fit inline. The planner can call
	// read_file on this path with offset/limit to page through.
	FailureSummaryBlobRef string `json:"failure_summary_blob_ref,omitempty"`

	// Timestamp records when this attempt's verify finished.
	Timestamp time.Time `json:"timestamp"`
}

// WriteApprovalReason mirrors writeflow.RiskReason without importing writeflow
// into the low-level types package. It is persisted on ChangePlan so an
// approval/denial can be audited after the original Run exits.
type WriteApprovalReason struct {
	Code   string `json:"code"`
	Detail string `json:"detail,omitempty"`
	Path   string `json:"path,omitempty"`
	Level  string `json:"level"`
}

// WriteApprovalRecord is the persisted approval decision for the exact
// ChangePlan fingerprint that reached the apply boundary. Approval is separate
// from PlanStatus: Status tracks execution lifecycle, while this record tracks
// policy, risk, decision source, and whether a human explicitly approved a
// manual-risk plan.
type WriteApprovalRecord struct {
	Policy          string                `json:"policy,omitempty"`
	RiskLevel       string                `json:"risk_level,omitempty"`
	Action          string                `json:"action,omitempty"`
	UserDecision    string                `json:"user_decision,omitempty"`
	ReasonCode      string                `json:"reason_code,omitempty"`
	Reason          string                `json:"reason,omitempty"`
	Reasons         []WriteApprovalReason `json:"reasons,omitempty"`
	Source          string                `json:"source,omitempty"`
	PlanFingerprint string                `json:"plan_fingerprint,omitempty"`
	DecidedAt       *time.Time            `json:"decided_at,omitempty"`

	// Operator identifies who the decision was recorded for (the main
	// repo's git identity, falling back to the OS user). Deterministic
	// capture at record time; no flag.
	Operator string `json:"operator,omitempty"`

	// RecordFingerprint is the record's own tamper-evidence hash over
	// the canonical decision fields (see ApprovalRecordFingerprint).
	// Post-hoc edits to user_decision / decided_at / operator no
	// longer go unnoticed: apply re-verifies it next to the plan
	// fingerprint.
	RecordFingerprint string `json:"record_fingerprint,omitempty"`
}

// ApprovalRecordFingerprint computes the canonical tamper-evidence
// hash of the decision fields. Reasons[] is excluded (advisory detail,
// frequently re-rendered); everything that changes WHAT was decided,
// BY WHOM, and FOR WHICH plan is included.
func ApprovalRecordFingerprint(r *WriteApprovalRecord) string {
	if r == nil {
		return ""
	}
	decidedUnix := int64(0)
	if r.DecidedAt != nil {
		decidedUnix = r.DecidedAt.Unix()
	}
	canonical := strings.Join([]string{
		r.Policy, r.RiskLevel, r.Action, r.UserDecision, r.ReasonCode,
		r.Source, r.PlanFingerprint, fmt.Sprintf("%d", decidedUnix), r.Operator,
	}, "|")
	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:])
}

// ApprovalRecordIntegrityOK reports whether the persisted record still
// matches its own fingerprint. Records written before fingerprinting
// (empty RecordFingerprint) are treated as legacy-valid.
func (p *ChangePlan) ApprovalRecordIntegrityOK() bool {
	if p == nil || p.Approval == nil {
		return true
	}
	if p.Approval.RecordFingerprint == "" {
		return true
	}
	return p.Approval.RecordFingerprint == ApprovalRecordFingerprint(p.Approval)
}

// ApprovalMatchesCurrentFingerprint reports whether the persisted approval was
// made against the plan bytes currently being considered for apply.
func (p *ChangePlan) ApprovalMatchesCurrentFingerprint() bool {
	if p == nil || p.Approval == nil {
		return false
	}
	return p.Approval.PlanFingerprint != "" && p.Approval.PlanFingerprint == PlanFingerprint(p)
}

// ChangePlan is the Plan stage's on-disk artifact — a structured
// description of the code change the planner intends to apply. Stored
// as .codrax/plans/<id>.json by emit_change_plan.Execute, consumed by
// the apply stage when the user opts in via --mode=write --write-phase=apply --plan-file.
//
// Schema is intentionally minimal for B0 skeleton. B1 will sharpen
// ChangeUnit (per open question #1) with fields like risk grading,
// explicit depends_on, acceptance_tests IR. Anything the B0 schema
// omits today gets added as a new optional field so old plan JSONs
// stay parseable.
//
// Lifecycle:
//  1. Plan stage: emit_change_plan.Execute writes <id>.json,
//     WriteClosure.PendingApplies is populated, Status="pending_approval"
//  2. User approves (REPL /approve) or policy auto-allows: Approval records
//     the decision and a separate Run() with --mode=write --write-phase=apply
//     consumes the file.
//  3. Apply stage reads <id>.json, iterates ChangeUnits through
//     apply_patch tool; sets applied_commit_sha on success.
//  4. Verify stage runs tests; outcome stored on ChangeReport (below).
type ChangePlan struct {
	// ID identifies this plan; format: plan-<unix-nano>-<pid>.
	// Same shape as memory.Turn.ID so PlanStore can reuse
	// timestamp-based ordering.
	ID string `json:"id"`

	// TriggerTurnID is the REPL turn ID that produced this plan.
	// Empty in single-shot mode. Used by REPL /undo (when session
	// memory has a Kind=plan IndexEntry with Refs=[this]) to walk
	// back to the user question that triggered the plan.
	TriggerTurnID string `json:"trigger_turn_id,omitempty"`

	// SessionID is the REPL session that produced this plan. Empty
	// in single-shot. Matches memory.Turn.SessionID naming so the
	// same-session boost in memory.BuildContext applies to plan
	// entries.
	SessionID string `json:"session_id,omitempty"`

	// Request is the user's natural-language request that produced
	// this plan. Stored verbatim so the approval UI / future code-
	// review tooling can re-present intent alongside the diff.
	Request string `json:"request"`

	// Summary is the planner's human-readable prose explanation of
	// what the plan does and why. Rendered as the user-visible
	// description in approval UI. 3-10 sentences typical.
	Summary string `json:"summary"`

	// Changes is the ordered list of file-level modifications the
	// apply stage will execute, one entry per file. Order is
	// significant — apply_patch processes them sequentially.
	Changes []FileChange `json:"changes"`

	// Slices are the deterministic online-convergence view over Changes.
	// They are authored by the harness or derived from Changes/DependsOn, not
	// required from the planner. The controller consumes this typed structure
	// to apply and observe small bounded slices without parsing planner prose.
	Slices []ChangePlanSlice `json:"slices,omitempty"`

	// AcceptanceTests names the test assertions the verify stage
	// must cover before this plan is considered successful.
	// Natural-language in B0 skeleton; B1 can promote to Criterion
	// IR (open question #1). Empty slice is legal — plan passes
	// verify trivially.
	AcceptanceTests []string `json:"acceptance_tests,omitempty"`

	// VerificationProbes are optional, typed, bounded behavioural
	// checks the verify stage may execute when the project runner is
	// unavailable or its output cannot be parsed. They are not parsed
	// from acceptance_tests prose: the planner must emit this structured
	// lane explicitly, and run_tests consumes only the fields below.
	VerificationProbes []VerificationProbe `json:"verification_probes,omitempty"`

	// BehaviorContracts is the write_analyzer contract snapshot visible when
	// this plan was emitted. It lets verify/probe coverage reference stable
	// atom IDs instead of re-reading mutable task prose after retries.
	BehaviorContracts []WriteBehaviorContract `json:"behavior_contracts,omitempty"`

	// TargetPaths is the declared write scope — the set of files
	// the apply stage is allowed to modify. Populated from Changes
	// but also stored independently so the apply-stage pre-flight
	// (W1 enforcer) can refuse to run if a ChangeUnit's patch
	// reaches outside this list.
	TargetPaths []string `json:"target_paths"`

	// AppliedPaths is the subset of TargetPaths that the coder
	// actually landed in the worktree (commit 42 P0). Persisted
	// at apply-post-hook time so /plan show on a partially_applied
	// plan can render WHICH files landed and which didn't —
	// pre-commit-42 only Status=partially_applied was visible
	// and the operator had no way to tell the work distribution
	// from a glance. AppliedPaths == TargetPaths on a fully-
	// applied plan; AppliedPaths is a proper subset on
	// partially_applied; empty on applied_failed (zero units
	// landed).
	AppliedPaths []string `json:"applied_paths,omitempty"`

	// Status transitions — see the PlanStatus* constants above.
	// Legal values: pending_approval, applied_pending_verify, applied,
	// applied_failed, verify_failed, unverified, partially_applied,
	// blocked, no_change_required, rejected, merged. Approval itself is
	// tracked by Approval, not by a separate status string.
	Status string `json:"status"`

	// AppliedCommitSHA is the git commit SHA produced inside the
	// worktree after all ChangeUnits applied successfully. Empty
	// while Status != "applied". B0 worktree-as-dry-run design:
	// this SHA lives inside the worktree, never cherry-picked to
	// main repo automatically — the user is responsible for merging.
	AppliedCommitSHA string `json:"applied_commit_sha,omitempty"`

	// WorktreePath is the absolute path of the git worktree the
	// apply stage ran in. Left on disk for user inspection after
	// --mode=write --write-phase=apply finishes; cleaned up by the next
	// worktree.PruneDeadSessions startup reap or explicit REPL
	// /discard.
	WorktreePath string `json:"worktree_path,omitempty"`

	// CreatedAt is the plan emission timestamp (wall clock).
	// Serves as the tie-breaker when PlanStore.List sorts plans.
	CreatedAt time.Time `json:"created_at"`

	// AppliedAt is set when the apply stage commits successfully.
	// Empty while Status != "applied".
	AppliedAt *time.Time `json:"applied_at,omitempty"`

	// MergedAt is set when /merge folds the worktree commit(s)
	// back into the main repo. Empty while Status != "merged".
	// Stamped together with Status = PlanStatusMerged in
	// PlanStore.Settle so the lifecycle stays explicit.
	MergedAt *time.Time `json:"merged_at,omitempty"`

	// RejectedAt is set when /reject (or PlanStore.Settle with
	// PlanStatusRejected) closes the plan as discarded. Empty
	// while Status != "rejected".
	RejectedAt *time.Time `json:"rejected_at,omitempty"`

	// RejectionReason carries the user's free-form note from
	// /reject [reason]. Empty when no reason was supplied or
	// Status != "rejected".
	RejectionReason string `json:"rejection_reason,omitempty"`

	// Approval records the latest deterministic write approval decision for
	// this exact plan fingerprint. A stale fingerprint never authorizes apply.
	Approval *WriteApprovalRecord `json:"approval,omitempty"`

	// PlanCritique is the optional pre-apply review text produced
	// by the plan_critic agent (commit 4 P1-F). Persisted onto
	// the plan so /plan show can render the critique even after a
	// REPL restart (the in-memory Mutable.PlanCritique slot is
	// per-Run; without persistence /plan show would lose the
	// review on every new session). Empty when the critic was
	// disabled or produced no risks.
	PlanCritique string `json:"plan_critique,omitempty"`

	// PatchReview is the post-apply typed review of the actual applied effect
	// against the active plan/slice scope. It is lifecycle audit metadata and
	// is deliberately excluded from PlanFingerprint.
	PatchReview *PatchReviewRecord `json:"patch_review,omitempty"`

	// PatchEffect is the typed factual view of the actual applied diff captured
	// from the worktree/applied commit. It is lifecycle/evidence metadata and
	// is deliberately excluded from PlanFingerprint.
	PatchEffect *PatchEffectRecord `json:"patch_effect,omitempty"`

	// LocalizationReview is the typed owner-boundary support review for the
	// plan's production source paths against prior localization context. It is
	// lifecycle/handoff metadata and is deliberately excluded from
	// PlanFingerprint.
	LocalizationReview *SourceLocalizationReview `json:"localization_review,omitempty"`

	// ImpactObligations are the typed downstream obligations derived from the
	// plan's declared changes, dependencies, contracts, symbols, and probes.
	// They are advisory handoff/confidence inputs, not hard approval authority,
	// and are excluded from PlanFingerprint.
	ImpactObligations *ImpactObligationSet `json:"impact_obligations,omitempty"`

	// ImpactAnalysis is the durable first-class impact result derived from the
	// actual patch effect, plan contracts/probes, and repository graph. It is
	// the source for controller/verifier scheduling and Patch Critic semantic
	// coverage. ImpactObligations remains its compact compatibility projection.
	// Both are lifecycle metadata and are excluded from PlanFingerprint.
	ImpactAnalysis *ImpactAnalysisResult `json:"impact_analysis,omitempty"`

	// UnvalidatedReasons lists pre-apply static-check stages that
	// were skipped because their toolchain was unavailable (e.g.
	// "rust:cargo not in PATH", "java/maven:mvn not in PATH").
	// Populated by emit_change_plan's dry-build helpers when a
	// language-specific helper bails out on toolchain absence.
	// /plan show renders these as a "· unvalidated stages" warning
	// so the operator knows the plan reached apply with one or
	// more languages skipped — distinct from "validated and
	// passed" which would have an empty list. nil/empty means
	// every applicable stage ran.
	UnvalidatedReasons []string `json:"unvalidated_reasons,omitempty"`

	// WriteAnalysisIR is the pinned snapshot of the write-mode
	// analyzer's output that drove this plan (commit 9 #3).
	// Persisted on the plan so a subsequent Run (e.g. /approve
	// invoking apply-mode against this plan, or /approve --retry
	// after a partial apply) reuses the SAME IR instead of
	// re-dispatching write_analyzer and producing a (possibly
	// drifted) second classification. The orchestrator seeds
	// Mutable.WriteAnalysisIR from this slot at apply-stage entry
	// when present, falling through to a fresh write_analyzer
	// dispatch only when the slot is empty (legacy plans / pure
	// /mode write flow). Pointer rather than embed so plans
	// emitted before commit 9 stay JSON-compatible (omitempty
	// elides the field on disk).
	WriteAnalysisIR *WriteAnalysisIR `json:"write_analysis_ir,omitempty"`

	// PhaseGroupID, when non-empty, ties this plan to a parent
	// PlanGroup created by stage II's runPhaseGroup. Empty for
	// single-phase plans (back-compat — every plan emitted
	// before commit 18 has this empty). Lets /phase show
	// cross-link to the per-plan file; lets /merge / /reject
	// recognise that a plan is part of a multi-phase group and
	// route differently.
	PhaseGroupID string `json:"phase_group_id,omitempty"`

	// PhaseIndex is the 0-based position of this plan within
	// the parent PlanGroup.Phases slice. Meaningless when
	// PhaseGroupID is empty (zero value matches the absence
	// signal at PhaseGroupID=="").
	PhaseIndex int `json:"phase_index,omitempty"`
}

// PlanFingerprint returns a deterministic hash of the apply-relevant plan
// fields. It deliberately excludes lifecycle/handoff fields (Status, AppliedAt,
// WorktreePath, Approval, critique, patch review/effect, impact analysis)
// so persisting status/approval/context metadata does not invalidate the
// approved payload, while any change to requested file operations or test
// obligations does.
func PlanFingerprint(plan *ChangePlan) string {
	if plan == nil {
		return ""
	}
	payload := struct {
		ID                 string              `json:"id"`
		Request            string              `json:"request,omitempty"`
		Summary            string              `json:"summary,omitempty"`
		Changes            []FileChange        `json:"changes"`
		AcceptanceTests    []string            `json:"acceptance_tests,omitempty"`
		VerificationProbes []VerificationProbe `json:"verification_probes,omitempty"`
		TargetPaths        []string            `json:"target_paths"`
		PhaseGroupID       string              `json:"phase_group_id,omitempty"`
		PhaseIndex         int                 `json:"phase_index,omitempty"`
	}{
		ID:                 plan.ID,
		Request:            plan.Request,
		Summary:            plan.Summary,
		Changes:            append([]FileChange(nil), plan.Changes...),
		AcceptanceTests:    append([]string(nil), plan.AcceptanceTests...),
		VerificationProbes: append([]VerificationProbe(nil), plan.VerificationProbes...),
		TargetPaths:        append([]string(nil), plan.TargetPaths...),
		PhaseGroupID:       plan.PhaseGroupID,
		PhaseIndex:         plan.PhaseIndex,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// VerificationProbe is a bounded, structured behavioural check attached to a
// ChangePlan. The executor is language-tagged and provider-backed so small
// local behaviour checks can run without parsing natural-language
// acceptance_tests or opening an arbitrary shell-command escape hatch.
type VerificationProbe struct {
	ID                     string   `json:"id,omitempty"`
	Language               string   `json:"language"`
	WorkingDir             string   `json:"working_dir,omitempty"`
	Code                   string   `json:"code"`
	TimeoutSeconds         int      `json:"timeout_seconds,omitempty"`
	ExpectedStdout         []string `json:"expected_stdout,omitempty"`
	ContractRefs           []string `json:"contract_refs,omitempty"`
	ChangedSymbolRefs      []string `json:"changed_symbol_refs,omitempty"`
	ExpectsBaselineFailure bool     `json:"expects_baseline_failure,omitempty"`
}

// FileChange describes one file-level modification the apply stage
// will execute. ChangePlan.Changes is an ordered list; apply_patch
// consumes one FileChange per Execute invocation.
//
// B0 skeleton uses "write_file" / "create" / "delete" semantics.
// B2 extends with unified-diff / line-range patch (open question #2:
// coder emit-once vs per-unit).
type FileChange struct {
	// Path is repo-relative; must be a prefix match of a
	// ChangePlan.TargetPaths entry or apply rejects (W1 enforcer).
	Path string `json:"path"`

	// Kind is the change type. Legal values:
	//   - "create": new file; NewContent is the full file body
	//   - "modify": overwrite file; NewContent is the full file body
	//   - "delete": remove the file; NewContent is ignored
	//   - "patch":  apply unified diff in Patch field
	//   - "rename": move file from Path to NewPath; NewContent ignored
	//               (a follow-on modify entry can edit the file at its
	//                new location). Git auto-detects the rename based
	//                on content similarity, preserving blame / history.
	Kind string `json:"kind"`

	// NewContent is the full file body for create/modify. Empty for
	// delete / rename. Unset for patch.
	NewContent string `json:"new_content,omitempty"`

	// Patch is the unified-diff payload for kind="patch".
	Patch string `json:"patch,omitempty"`

	// Edits is the structured-edit alternative for kind="patch".
	// Planner may use it for localized line operations instead of
	// hand-writing a unified diff. The tool layer deterministically
	// compiles Edits against current file bytes into Patch before
	// validation/apply. Raw Patch remains available for complex diffs.
	Edits []StructuredEdit `json:"edits,omitempty"`

	// NewPath is the destination path for kind="rename". Repo-
	// relative, must not collide with any existing file in the
	// worktree or with another change's Path / NewPath. Empty for
	// every other kind. Validation (emit_change_plan):
	//  - rename requires NewPath non-empty
	//  - the old Path must exist in the repo
	//  - the new NewPath must not yet exist
	//  - cross-change collision: NewPath cannot equal another
	//    entry's Path OR NewPath
	NewPath string `json:"new_path,omitempty"`

	// Rationale is the planner's prose explanation for WHY this
	// specific file needs this specific change. Rendered in the
	// approval UI per-file. 1-3 sentences typical.
	Rationale string `json:"rationale"`

	// DependsOn is the list of repo-relative paths of OTHER changes
	// in the same plan that MUST apply before this one. Empty slice
	// means "no explicit ordering" — the apply stage walks plan.
	// Changes in declaration order. Typical shape: a create(X) that
	// must land before modify(Y) where Y imports X.
	//
	// Validation (B1, emit_change_plan.Execute):
	//  - every entry must appear as another Changes[].Path
	//  - no cycles (DFS rejects circular depends_on graph)
	//  - one-change-per-file constraint means each DependsOn entry
	//    unambiguously identifies its target ChangeUnit
	//
	// Apply-stage uses DependsOn to drive a topological sort so
	// LLM-emitted per-unit apply_patch calls satisfy ordering
	// constraints even when the LLM picks a different emission
	// order. The W1 invariant (apply_patch.Execute) double-checks
	// at runtime that every DependsOn target is already in
	// WriteClosure.AppliedSet before the tool accepts the unit.
	DependsOn []string `json:"depends_on,omitempty"`

	// Apply records the actual apply engine/outcome for this change.
	// It is populated by apply_patch after the plan is approved and
	// written back to the plan artifact for audit. Planner-authored
	// plans leave this empty.
	Apply *FileChangeApplyRecord `json:"apply,omitempty"`
}

// FileChangeApplyRecord is the per-change apply audit trail. Source
// describes the plan payload shape; Engine describes the executor that
// actually touched bytes in the worktree.
type FileChangeApplyRecord struct {
	Status    string    `json:"status"`
	Source    string    `json:"source,omitempty"`
	Engine    string    `json:"engine,omitempty"`
	Reason    string    `json:"reason,omitempty"`
	AppliedAt time.Time `json:"applied_at,omitempty"`
}

// StructuredEdit is a structured edit inside a single existing file.
// Line numbers are 1-based and refer to the file bytes read by the validator
// before any edit in this list is applied. Multiple edits in one FileChange are
// composed deterministically after overlap checks.
type StructuredEdit struct {
	// Kind is one of replace, delete, insert_before, insert_after,
	// insert_at_eof, insert_before_final_brace.
	Kind string `json:"kind"`

	// StartLine is required for line-addressed kinds. For replace/delete it
	// is the first line in the inclusive range. For insert_before/insert_after
	// it is the anchor line. It is ignored for insert_at_eof and
	// insert_before_final_brace.
	StartLine int `json:"start_line"`

	// EndLine is the inclusive 1-based last line for replace/delete;
	// omitted (0) it defaults to StartLine (single-line edit). Ignored
	// for insert kinds.
	EndLine int `json:"end_line,omitempty"`

	// Content is inserted or used as replacement text for insert/replace.
	// It may contain multiple lines and is preserved byte-for-byte.
	Content string `json:"content,omitempty"`

	// OldText optionally carries the exact original bytes expected at the
	// target range or insertion anchor. When set, the builder rejects stale
	// context instead of relying on line numbers alone.
	OldText string `json:"old_text,omitempty"`
}

// ChangeReport is the Verify stage's output — a structured summary
// of what passed, what failed, and any performance impact. Written
// alongside the plan JSON as .codrax/plans/<plan-id>.report.json so
// the verify outcome is recoverable across process restarts.
//
// Consumed by the write-mode ChangeReport rendering path: the
// finalize stage reads this file and renders a user-visible
// AnswerDocument describing the change and its test outcome.
type ChangeReport struct {
	// PlanID names the ChangePlan this report corresponds to.
	// One-to-one: a single plan produces zero-or-one report.
	PlanID string `json:"plan_id"`

	// Channel records which scheduler lane produced the report. Hard workflow
	// decisions must only treat post_apply_verify reports as authoritative.
	// Planner probes are useful context, but they describe the pre-apply state
	// and must never drive finish/replan routing for the active batch.
	Channel ChangeReportChannel `json:"channel,omitempty"`

	// PhaseGroupID + PhaseIndex carry the multi-phase coordinates
	// when this report belongs to a stage II phase plan (commit
	// 32 schema gap fix). Both empty/zero on single-phase reports.
	// /history can group consecutive reports by PhaseGroupID
	// instead of rendering them as flat unrelated rows; without
	// these the multi-phase clustering signal lives only on
	// ChangePlan and reports look like 5 unrelated tasks.
	PhaseGroupID string `json:"phase_group_id,omitempty"`
	PhaseIndex   int    `json:"phase_index,omitempty"`

	// TestResults is the verify stage's assertion-level outcomes,
	// one entry per test case / assertion. Derived from run_tests
	// output by the language-specific parser (B3 open question #4).
	TestResults []TestResult `json:"test_results"`

	// MetricDeltas records before-vs-after measurements the verify
	// stage captured (benchmark output, lint counts, coverage
	// changes). Key naming is free-form; consumers grep for
	// specific metric names they care about.
	MetricDeltas map[string]MetricDelta `json:"metric_deltas,omitempty"`

	// Passed is the overall verdict: true iff every TestResult
	// entry has Passed=true and no MetricDelta regressed past its
	// configured threshold. Read by CritNoRegression evaluator.
	Passed bool `json:"passed"`

	// VerificationStatus is the scheduler-facing verdict for the verify
	// attempt. Passed remains the assertion aggregate for compatibility; this
	// field distinguishes "all executed assertions passed" from "no
	// authoritative local verification was available" (missing runner, parser
	// failure, or zero executed tests).
	VerificationStatus VerificationStatus `json:"verification_status,omitempty"`

	// BuildFailed is true when the test harness short-circuited
	// because the build/compile step failed before any test could
	// run. evalTestsPass uses this as a top-level fast-fail so
	// "tests_pass: 1 of 1 tests failed: java-build" doesn't show up
	// as the rejection reason — operators get a clear "build failed
	// before tests ran" message instead. Always true for
	// TestResults that contain a TestResultKindBuildError entry.
	BuildFailed bool `json:"build_failed,omitempty"`

	// FailureSummary is a human-readable explanation when
	// Passed=false. Rendered in the user-visible AnswerDocument.
	// Empty when Passed=true.
	FailureSummary string `json:"failure_summary,omitempty"`

	// FailureSummaryBlobRef is the blob path produced by run_tests
	// when the runner's full stderr exceeds the inline cap and got
	// offloaded via tool.StoreBlob. Empty when the summary fit
	// inline OR when no blob was created. The planner can call
	// read_file with this path to page the complete content via
	// offset/limit — Module D's "feed complete error to model"
	// guarantee. Never used as the FailureSummary itself; the
	// inline FailureSummary stays the readable preview, the blob
	// ref is the full original.
	FailureSummaryBlobRef string `json:"failure_summary_blob_ref,omitempty"`

	// FailureReasonCode is a bounded, typed subreason for unavailable or
	// failed verification states. It refines FailureKind for controller /
	// replan handoff without requiring any consumer to parse FailureSummary
	// prose. Examples: pytest_import_startup_error,
	// pytest_json_report_missing, pytest_collection_error_no_cases.
	FailureReasonCode string `json:"failure_reason_code,omitempty"`

	// VerificationDiagnostics is the normalized diagnostic lane for verify
	// evidence that should survive across controller/planner/verifier
	// handoff, even when it is not the report's primary FailureReasonCode.
	// Examples include a bounded verification_probe authoring error followed
	// by a project-suite parser error, or a missing runner before a fallback
	// candidate runs. Items are derived from typed command/probe outcomes,
	// never from model prose.
	VerificationDiagnostics []VerificationDiagnostic `json:"verification_diagnostics,omitempty"`

	// VerificationConfidence records why a local verdict should or should not
	// be treated as strong evidence. This lane is separate from pass/fail:
	// missing pytest can leave delivery unverified without being a code
	// failure, while a passing probe can still be low-confidence when it does
	// not cover required behavior contracts or changed symbols. Records are
	// derived from ChangePlan probes/contracts, command outcomes, and typed
	// diagnostics; they are never inferred from model prose.
	VerificationConfidence []VerificationConfidenceRecord `json:"verification_confidence,omitempty"`

	// RegressionAssertions lists test AssertionIDs the verifier LLM
	// classified as "passed in baseline, fails now — caused by this
	// plan". Authoritative for evalNoRegression — populated by
	// emit_test_results when the verifier explicitly classifies
	// failures, falling back to a baseline-vs-current diff in
	// run_tests_parsers when the LLM doesn't. Empty when
	// BaselineReport is absent (regression check skipped).
	RegressionAssertions []string `json:"regression_assertions,omitempty"`

	// PreexistingAssertions lists test AssertionIDs that failed
	// in BOTH baseline AND current — unrelated to this plan.
	// Surfaced in the answer document so the operator knows which
	// failures predate the change. Populated alongside
	// RegressionAssertions by emit_test_results / run_tests_parsers.
	PreexistingAssertions []string `json:"preexisting_assertions,omitempty"`

	// FixedAssertions lists test AssertionIDs that failed in
	// baseline but pass now — a side-benefit of the plan. Bonus
	// signal; not used by any criterion but rendered to the user.
	FixedAssertions []string `json:"fixed_assertions,omitempty"`

	// NoTestsRunners records the runner names that completed cleanly
	// but discovered zero test cases (e.g. pytest exit code 5, jest
	// "no tests found", go test ./... with no _test.go files). The
	// list is intentionally separate from FailureSummary so the
	// verifier evaluator can keep Passed=true while still surfacing
	// "verification ran but had no tests to execute" to the LLM /
	// operator. Empty when every runner that ran found at least one
	// test, which is the normal case.
	//
	// Provenance: a Python-script-in-a-Go-repo apply triggered the
	// verify→plan retry loop because parsePytestJSONReport mapped exit
	// 5 ("no tests collected") to Passed=false; the planner then
	// hallucinated a test_*.py file to "fix" the absent suite. Making
	// "zero tests discovered" a first-class signal across all 12
	// runners stops the parser from inventing failures and lets the
	// verifier prompt drive a syntax-only fallback when appropriate.
	NoTestsRunners []string `json:"no_tests_runners,omitempty"`

	// FailureKind classifies terminal failed and unavailable states so
	// the verify→plan retry hint can surface resource exhaustion,
	// missing runners, parser dead-ends, and no-test outcomes distinctly
	// from a normal red test ("tests_failed"). Empty + Passed=false
	// defaults to "tests_failed". Set by run_tests when the
	// SupervisedRun exit kind is timeout / OOM / cpu_limit, and preserved
	// through merge/qualify so the heuristic hint reads it before the
	// planner re-dispatches.
	//
	// Provenance: the OOM-killed pytest event (RSS 2.47 GiB after 9 h
	// of CPU) demonstrated that a generic "tests failed" hint sends
	// the planner re-deriving the same bug from the same noisy
	// stderr; surfacing the resource-exhaustion signal explicitly
	// gives it a corrective direction the symptoms alone don't.
	FailureKind FailureKind `json:"failure_kind,omitempty"`

	// ExecutedCommands records each verification command this report's
	// run executed — or decided not to execute — with typed provenance
	// and exit data. Durable command evidence for audit and for the
	// verify-failure replan handoff; never derived from prose.
	ExecutedCommands []ExecutedCommand `json:"executed_commands,omitempty"`

	// TestSurface is the typed runnable-candidate inventory computed
	// before command selection for this run. Persisted with the report
	// so a failed attempt keeps the full decision context (what else
	// could have run), and so replan can suggest the next candidate.
	TestSurface *TestSurface `json:"test_surface,omitempty"`

	// GeneratedAt is the verify-stage completion timestamp.
	GeneratedAt time.Time `json:"generated_at"`
}

type ChangeReportChannel string

const (
	ChangeReportChannelPlannerProbe    ChangeReportChannel = "planner_probe"
	ChangeReportChannelPostApplyVerify ChangeReportChannel = "post_apply_verify"
)

type VerificationStatus string

const (
	VerificationStatusPassed      VerificationStatus = "passed"
	VerificationStatusFailed      VerificationStatus = "failed"
	VerificationStatusUnavailable VerificationStatus = "unavailable"
)

// FailureKind tags ChangeReport.FailureSummary so consumers can route
// the retry-hint narrative on the cause rather than the symptom.
type FailureKind string

const (
	// FailureKindTestsFailed — at least one test reported red but
	// the run completed within resource budget. Default when
	// Passed=false and no other kind applies.
	FailureKindTestsFailed FailureKind = "tests_failed"

	// FailureKindBuildFailure — the build/compile step failed before
	// any test could run. Same case BuildFailed=true tracks; this
	// kind exists so Passed=false can be classified by a single field.
	FailureKindBuildFailure FailureKind = "build_failure"

	// FailureKindPreexistingBuildFailure — the build/compile step failed, but
	// every structured file:line build diagnostic was proven to be outside the
	// current ChangePlan changed-line surface. This is unavailable local
	// verification, not evidence that the current patch is broken.
	FailureKindPreexistingBuildFailure FailureKind = "preexisting_build_failure"

	// FailureKindTimeout — wall-clock timeout fired before the test
	// run completed. The supervisor SIGKILLed the entire process
	// tree.
	FailureKindTimeout FailureKind = "timeout"

	// FailureKindOOM — kernel OOM-killed a process in the tree
	// (Unix exit 137 / signal SIGKILL classified by the supervisor)
	// or a JobObject memory limit fired (Windows). Most common cause
	// is unbounded allocation in test code; planner retry must
	// revise the test approach, not the production code.
	FailureKindOOM FailureKind = "oom"

	// FailureKindCPULimit — RLIMIT_CPU (Unix SIGXCPU) or
	// PerJobUserTimeLimit (Windows) fired. Distinct from Timeout
	// because the wall-clock budget may not have been reached;
	// CPU-time is what burned out. Common cause: infinite loop
	// without sleep/yield.
	FailureKindCPULimit FailureKind = "cpu_limit"

	// FailureKindCrash — process exited unexpectedly (segfault,
	// internal panic) without producing a parseable test report.
	// Reserved; not yet emitted by run_tests.
	FailureKindCrash FailureKind = "crash"

	// FailureKindRunnerMissing — the test runner binary itself is
	// not installed in the execution environment (e.g. `pytest:
	// command not found`, exit 127). Distinct from BuildFailure
	// (the runner ran but the project failed to build) and from
	// TestsFailed (the runner ran tests that reported red). Set
	// when the supervised process exits 127 with stderr containing
	// "command not found" / "executable file not found" / "No such
	// file or directory" patterns from sh / bash / Windows cmd.
	//
	// Retry policy: the verify→plan loop SKIPS retry on this kind.
	// Re-running the planner cannot fix a missing tool — only the
	// operator's environment can. The user-facing message names the
	// missing binary + a per-runner install hint; downstream
	// consumers (REPL, CLI exit code) treat this as a terminal
	// environment error rather than a plan defect.
	FailureKindRunnerMissing FailureKind = "runner_missing"

	// FailureKindParserError — the runner executed but Codrax could not
	// obtain the structured report required for an authoritative verdict
	// (for example pytest aborted during collection before
	// pytest-json-report wrote its file). This is a verifier/tooling or
	// environment compatibility gap, not evidence that the code patch is
	// wrong. Treat it as terminal unverified for the local run so the
	// planner does not chase infrastructure noise.
	FailureKindParserError FailureKind = "parser_error"

	// FailureKindNoTests — the selected runner completed or was skipped
	// cleanly but produced no executable tests for this plan. This is an
	// unavailable local verification signal, not proof of code failure.
	// The report may keep Passed=true for legacy no-test parser
	// compatibility; consumers must rely on VerificationStatus and this
	// typed reason rather than the legacy boolean alone.
	FailureKindNoTests FailureKind = "no_tests"
)

// VerificationDiagnostic is one typed verification diagnostic projected from
// command/probe/test-surface execution evidence. It is intentionally compact:
// hard routing still consumes ChangeReport fields directly, while this lane
// keeps secondary but important evidence available to controller/planner/
// verifier views without asking them to parse summaries.
type VerificationDiagnostic struct {
	Source     string `json:"source,omitempty"`
	Category   string `json:"category,omitempty"`
	Severity   string `json:"severity,omitempty"`
	ReasonCode string `json:"reason_code,omitempty"`
	Runner     string `json:"runner,omitempty"`
	Framework  string `json:"framework,omitempty"`
	WorkingDir string `json:"working_dir,omitempty"`
	Command    string `json:"command,omitempty"`
	Outcome    string `json:"outcome,omitempty"`
	ExitCode   int    `json:"exit_code,omitempty"`
	Detail     string `json:"detail,omitempty"`
}

// VerificationConfidenceRecord is a typed confidence signal for a local verify
// attempt. It intentionally does not replace VerificationStatus. Consumers use
// it to decide how much to trust a local pass/unavailable result without
// parsing natural-language summaries.
type VerificationConfidenceRecord struct {
	Source            string   `json:"source,omitempty"`
	Category          string   `json:"category,omitempty"`
	Status            string   `json:"status,omitempty"`
	Severity          string   `json:"severity,omitempty"`
	ReasonCode        string   `json:"reason_code,omitempty"`
	ContractRefs      []string `json:"contract_refs,omitempty"`
	ChangedSymbolRefs []string `json:"changed_symbol_refs,omitempty"`
	Detail            string   `json:"detail,omitempty"`
}

// Score returns the (passed, total) test counts for this report,
// counting only TestResultKindUnit entries (build_error rows are
// classifications not assertions and would distort comparisons).
// Used by the verify→plan retry loop's best-known-good latch
// (clearForReplan) to detect retry regressions.
//
// (-1, -1) when the receiver is nil so callers can detect "no report
// captured yet" without a separate nil check at every call site.
func (r *ChangeReport) Score() (passed, total int) {
	if r == nil {
		return -1, -1
	}
	for _, t := range r.TestResults {
		kind := t.Kind
		if kind == "" {
			kind = TestResultKindUnit
		}
		if kind != TestResultKindUnit {
			continue
		}
		total++
		if t.Passed {
			passed++
		}
	}
	return passed, total
}

func (r *ChangeReport) NormalizeVerificationStatus() VerificationStatus {
	if r == nil {
		return ""
	}
	if r.FailureKind == FailureKindBuildFailure && FailureReasonCodeIndicatesVerificationUnavailable(r.FailureReasonCode) {
		return VerificationStatusUnavailable
	}
	switch r.FailureKind {
	case FailureKindRunnerMissing, FailureKindParserError, FailureKindNoTests, FailureKindPreexistingBuildFailure:
		return VerificationStatusUnavailable
	}
	if len(r.NoTestsRunners) > 0 && len(r.TestResults) == 0 {
		return VerificationStatusUnavailable
	}
	if !r.Passed {
		return VerificationStatusFailed
	}
	return VerificationStatusPassed
}

func (r *ChangeReport) EnsureVerificationStatus() {
	if r == nil {
		return
	}
	if len(r.NoTestsRunners) > 0 && len(r.TestResults) == 0 && r.FailureKind == "" {
		r.FailureKind = FailureKindNoTests
	}
	r.VerificationStatus = r.NormalizeVerificationStatus()
}

// FailureReasonCodeIndicatesVerificationUnavailable returns true when every
// typed reason code describes local verifier/tooling/environment unavailability
// rather than evidence that the production patch is behaviorally wrong.
func FailureReasonCodeIndicatesVerificationUnavailable(raw string) bool {
	parts := strings.Split(raw, ",")
	seen := false
	for _, part := range parts {
		code := strings.TrimSpace(part)
		if code == "" {
			continue
		}
		seen = true
		switch code {
		case string(FailureKindNoTests),
			string(FailureKindRunnerMissing),
			string(FailureKindParserError),
			string(FailureKindPreexistingBuildFailure),
			"skip_verify",
			"accepted_without_local_verify",
			"make_python_module_missing",
			"make_target_missing",
			"pytest_import_startup_error",
			"unittest_loader_import_error",
			"verification_probe_import_error",
			"verification_probe_module_not_found",
			"verification_probe_name_error",
			"verification_probe_syntax_error":
			continue
		default:
			return false
		}
	}
	return seen
}

// IsBetterThan returns true when this report has strictly more
// passing tests than other (or, on a tie, this one has more total
// tests — defensive against a retry that converges by deleting tests
// rather than fixing them). A nil receiver loses to any non-nil
// report; nil-vs-nil returns false.
//
// "Strictly more" matters: ties on (passed, total) keep the existing
// best so the system never thrashes between equivalent plans.
func (r *ChangeReport) IsBetterThan(other *ChangeReport) bool {
	if r == nil {
		return false
	}
	if other == nil {
		return true
	}
	rp, rt := r.Score()
	op, ot := other.Score()
	if rp != op {
		return rp > op
	}
	return rt > ot
}

// TestResultKind tags a TestResult so downstream consumers can tell
// "compile error before tests ran" apart from "unit test failed".
// Replaces the earlier string convention (AssertionID="java-build" /
// "cargo-build" / "hvigor-build" / etc.) with a structured field.
type TestResultKind string

const (
	// TestResultKindUnit is a normal test case outcome from a
	// language-specific test runner. Default value when not
	// explicitly set.
	TestResultKindUnit TestResultKind = "unit"

	// TestResultKindBuildError is a synthetic TestResult emitted
	// when the build/compile step failed before any test ran. The
	// BuildErrors[] field carries structured file:line:msg entries
	// extracted from build-tool stdout; FailureDetail still carries
	// a free-form excerpt for human reading.
	TestResultKindBuildError TestResultKind = "build_error"
)

// BuildError is one structured compile-error entry parsed from
// build-tool stdout. Populated only on TestResultKindBuildError
// entries. Consumers (verifier prompt, /history rendering) walk
// BuildErrors[] to surface concrete file:line targets the LLM /
// operator can act on.
type BuildError struct {
	File    string `json:"file"`             // repo-relative or absolute as printed by the tool
	Line    int    `json:"line,omitempty"`   // 1-based; 0 = unknown
	Column  int    `json:"column,omitempty"` // 1-based; 0 = unknown
	Symbol  string `json:"symbol,omitempty"` // optional — e.g. "TS2304" / "AssertionError"
	Message string `json:"message"`          // the compiler's error text (single line)
}

// TestResult is one row in ChangeReport.TestResults. Mirrors the
// shape common test harnesses (go test -json, pytest json-report,
// mocha --reporter json) expose so the B3 parser surface is
// minimal.
type TestResult struct {
	// Kind classifies this TestResult. TestResultKindUnit for
	// normal test outcomes; TestResultKindBuildError for synthetic
	// rows describing a pre-test build failure.
	Kind TestResultKind `json:"kind,omitempty"`

	// AssertionID is the canonical identifier used for matching
	// against ChangePlan.AcceptanceTests. Format is framework-
	// specific ("TestFoo", "test_module.TestClass.test_method").
	// For build-error entries, conventionally "<file>:<line>" of
	// the first error or empty when unparseable.
	AssertionID string `json:"assertion_id"`

	// Suite groups related assertions (Go package path, pytest
	// module, etc.). Same suite on all tests from one
	// run_tests invocation. "build" for build-error entries.
	Suite string `json:"suite"`

	// Passed is true when the framework reported success. Always
	// false for build-error entries.
	Passed bool `json:"passed"`

	// Duration is observed wall-clock time.
	Duration time.Duration `json:"duration"`

	// FailureDetail is framework stdout/stderr excerpt when the
	// test failed; empty on pass. Bounded to ~2 KB per test so a
	// ChangeReport stays readable.
	FailureDetail string `json:"failure_detail,omitempty"`

	// BuildErrors is populated only when Kind == TestResultKindBuildError.
	// Each entry is a parsed file:line:msg compile error from build-tool
	// stdout. Empty when the parser couldn't extract structured info —
	// FailureDetail is then the only signal.
	BuildErrors []BuildError `json:"build_errors,omitempty"`
}

// MetricDelta records one pre-vs-post change measurement the verify
// stage captured. Baseline is the value observed on the pre-apply
// worktree HEAD; Current is the value observed after apply. Unit is
// free-form ("ns/op", "MB", "lines", "%").
//
// CritNoRegression compares (Current - Baseline) against a
// user-specified or default threshold to decide whether the change
// regresses this metric.
type MetricDelta struct {
	Baseline  float64 `json:"baseline"`
	Current   float64 `json:"current"`
	Unit      string  `json:"unit"`
	Threshold float64 `json:"threshold,omitempty"` // max allowed regression
}

// WriteChangeReportToFile serialises a ChangeReport as indented
// JSON to path. Parent directories are created if missing.
// B1.3 the verify stage hook calls this after the run_tests parser
// populates the report so operators can inspect outcome post-Run
// even though the worktree itself is discarded.
//
// The companion plan file under .codrax/plans/<plan-id>.json
// already exists (written by B0 Day-6 emit_change_plan →
// PlanStore.Save or cmd/root.go writePlanFile); the report
// conventionally lives at the same directory with a .report.json
// suffix so the plan+report pair stays grep-able.
//
// Failure is non-fatal for verify stage completion — the caller
// logs a warning and continues. The ChangeReport still lives on
// Mutable, which is what the REPL renderer / single-shot stdout
// summary actually consumes for user display.
// LoadChangeReportFromFile reads a persisted ChangeReport JSON. Used by
// workflow resume to rebuild typed verify-failure context from the durable
// artifact chain.
func LoadChangeReportFromFile(path string) (*ChangeReport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var report ChangeReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("parse change report %s: %w", path, err)
	}
	report.EnsureVerificationStatus()
	return &report, nil
}

func WriteChangeReportToFile(report *ChangeReport, path string) error {
	if report == nil {
		return fmt.Errorf("WriteChangeReportToFile: nil report")
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("WriteChangeReportToFile: empty path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("WriteChangeReportToFile: mkdir %s: %w", filepath.Dir(path), err)
	}
	report.EnsureVerificationStatus()
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("WriteChangeReportToFile: marshal: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("WriteChangeReportToFile: write %s: %w", path, err)
	}
	return nil
}

// WritePlanToFile serialises a ChangePlan as indented JSON and
// writes it to path atomically (tmp + rename). Used by the
// orchestrator after planPostHook fills late-bound fields like
// PlanCritique that the planner did not have at emit time. Plan
// files written by the REPL's PlanStore.Save go through their
// own atomic write — this helper is the fallback path the
// orchestrator uses to persist updates.
//
// Failure is non-fatal at the call site: callers log a warning
// and continue with the in-memory plan. Empty path returns an
// error so the caller can detect "this plan was never persisted"
// (typical for ModePlan dispatches before /approve).
func WritePlanToFile(plan *ChangePlan, path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("WritePlanToFile: empty path")
	}
	if plan == nil {
		return fmt.Errorf("WritePlanToFile: nil plan")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("WritePlanToFile: mkdir %s: %w", filepath.Dir(path), err)
	}
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return fmt.Errorf("WritePlanToFile: marshal: %w", err)
	}
	if err := AtomicWriteFileSync(path, data, 0o644); err != nil {
		return fmt.Errorf("WritePlanToFile: write %s: %w", path, err)
	}
	return nil
}

// bestPlanReportPair is the on-disk companion shape for the
// best-known-good (plan, report) latch. Stored as a single JSON
// object beside the plan file so a process crash mid-retry doesn't
// drop the iteration history that drove the latch.
type bestPlanReportPair struct {
	Plan   *ChangePlan   `json:"plan"`
	Report *ChangeReport `json:"report"`
}

// WriteBestPlanReportPair serialises the (plan, report) pair to
// <plan-dir>/<plan-id>.best.json. Used by clearForReplan after the
// best-known-good latch updates so that a process restart mid-
// retry can recover the high-water mark via LoadBestPlanReportPair.
//
// Failure is non-fatal — the caller logs and continues; the
// in-memory latch is still authoritative for the rest of the Run.
// Persistence is "nice to have on crash recovery", not a hard
// invariant.
func WriteBestPlanReportPair(plan *ChangePlan, report *ChangeReport, planFilePath string) error {
	planFilePath = strings.TrimSpace(planFilePath)
	if planFilePath == "" {
		return fmt.Errorf("WriteBestPlanReportPair: empty planFilePath")
	}
	if plan == nil || report == nil {
		return fmt.Errorf("WriteBestPlanReportPair: nil plan or report")
	}
	planID := strings.TrimSpace(plan.ID)
	if planID == "" {
		return fmt.Errorf("WriteBestPlanReportPair: plan.ID empty")
	}
	dir := filepath.Dir(planFilePath)
	bestPath := filepath.Join(dir, planID+".best.json")
	pair := bestPlanReportPair{Plan: plan, Report: report}
	data, err := json.MarshalIndent(pair, "", "  ")
	if err != nil {
		return fmt.Errorf("WriteBestPlanReportPair: marshal: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("WriteBestPlanReportPair: mkdir %s: %w", dir, err)
	}
	if err := AtomicWriteFileSync(bestPath, data, 0o644); err != nil {
		return fmt.Errorf("WriteBestPlanReportPair: write %s: %w", bestPath, err)
	}
	return nil
}

// LoadBestPlanReportPair reads the (plan, report) pair from
// <plan-dir>/<plan-id>.best.json, returning (nil, nil, nil) when
// the file does not exist (the typical case — no prior best
// recorded). Returns a non-nil error only when the file exists
// but cannot be parsed.
//
// Used at Run entry to seed Mutable.SetBestPlanReport on a
// resumed retry cycle: a process killed between iteration N's
// best-update and iteration N+1's apply would otherwise lose the
// high-water mark and surface iteration N+1's worse plan as the
// final answer (the eval Batch K forth-py regression that
// motivated the latch in the first place).
func LoadBestPlanReportPair(planFilePath string) (*ChangePlan, *ChangeReport, error) {
	planFilePath = strings.TrimSpace(planFilePath)
	if planFilePath == "" {
		return nil, nil, nil
	}
	planID := strings.TrimSuffix(filepath.Base(planFilePath), ".json")
	if planID == "" {
		return nil, nil, nil
	}
	bestPath := filepath.Join(filepath.Dir(planFilePath), planID+".best.json")
	data, err := os.ReadFile(bestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("LoadBestPlanReportPair: read %s: %w", bestPath, err)
	}
	var pair bestPlanReportPair
	if err := json.Unmarshal(data, &pair); err != nil {
		return nil, nil, fmt.Errorf("LoadBestPlanReportPair: unmarshal %s: %w", bestPath, err)
	}
	return pair.Plan, pair.Report, nil
}

// UpdatePlanStatusOnDisk reads the plan JSON at path, replaces
// its Status (and optionally AppliedAt + WorktreePath), and
// rewrites the file. Used by the apply stage hook / the verify stage hook to
// record transitions (pending → applied / applied_failed /
// verify_failed) so /plan list in the REPL shows current lifecycle
// state; worktreePath is populated when Fix 4's keep-on-success
// preserves the worktree so /worktree list can later correlate
// preserved trees back to their originating plans.
//
// Preserves all other fields byte-for-byte via unmarshal →
// mutate → marshal; callers rely on this so AppliedCommitSHA /
// TriggerTurnID survive the update. An empty worktreePath means
// "don't touch" so callers that don't care (verify_failed, reject,
// etc.) can pass "" without clobbering a previously persisted path.
//
// Non-fatal: when path is empty or the file is missing, returns
// an error that the caller typically logs and swallows (failing
// to persist status must not break apply/verify itself).
func UpdatePlanStatusOnDisk(path, status string, appliedAt *time.Time, worktreePath string) error {
	return UpdatePlanStatusOnDiskWithApplied(path, status, appliedAt, worktreePath, nil)
}

// UpdatePlanStatusOnDiskWithApplied is the commit-42 variant
// that ALSO persists the AppliedPaths subset. Callers that
// know the apply outcome (apply post-hook for partially_applied)
// pass a non-nil slice so /plan show on the persisted plan
// can render WHICH files landed. Callers that don't (verify
// post-hook for applied / verify_failed) pass nil to leave
// the slot untouched.
func UpdatePlanStatusOnDiskWithApplied(path, status string, appliedAt *time.Time, worktreePath string, appliedPaths []string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("UpdatePlanStatusOnDisk: empty path")
	}
	plan, err := LoadChangePlanFromFile(path)
	if err != nil {
		return fmt.Errorf("UpdatePlanStatusOnDisk load: %w", err)
	}
	plan.Status = status
	if appliedAt != nil {
		plan.AppliedAt = appliedAt
	}
	if worktreePath != "" {
		plan.WorktreePath = worktreePath
	}
	if appliedPaths != nil {
		plan.AppliedPaths = appliedPaths
	}
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return fmt.Errorf("UpdatePlanStatusOnDisk marshal: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("UpdatePlanStatusOnDisk write %s: %w", path, err)
	}
	return nil
}

// PersistAppliedRecoveryOnDisk records the apply commit SHA on the
// plan's JSON file so /merge / /history / manual recovery can find
// it after the worktree is destroyed. The companion ref name is
// derived in the worktree package (worktree.AppliedRef(planID)) and
// is NOT persisted here — it's recomputable from the plan ID.
//
// Call this AFTER worktree.TagAppliedCommit succeeds so the
// invariant is "if the disk has AppliedCommitSHA, then either the
// worktree at WorktreePath or the ref refs/codrax/applied/<id>
// resolves it". Best-effort: an error here means the operator can't
// auto-recover via /merge, but the bytes are still in main_repo/.git
// /objects until git gc runs — manual recovery from log output is
// still possible.
func PersistAppliedRecoveryOnDisk(path, sha string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("PersistAppliedRecoveryOnDisk: empty path")
	}
	if strings.TrimSpace(sha) == "" {
		return fmt.Errorf("PersistAppliedRecoveryOnDisk: empty sha")
	}
	plan, err := LoadChangePlanFromFile(path)
	if err != nil {
		return fmt.Errorf("PersistAppliedRecoveryOnDisk load: %w", err)
	}
	plan.AppliedCommitSHA = sha
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return fmt.Errorf("PersistAppliedRecoveryOnDisk marshal: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("PersistAppliedRecoveryOnDisk write %s: %w", path, err)
	}
	return nil
}

// LoadChangePlanFromFile reads a ChangePlan JSON from disk and
// returns a deserialised *ChangePlan. Called by the orchestrator's
// the apply stage hook when --mode=write --write-phase=apply --plan-file=X needs to install
// the plan on Mutable before the coder agent dispatches.
//
// Validation is minimal (non-empty ID, non-nil Changes, reasonable
// Status); the tool-side emit_change_plan.Execute already enforced
// the strict graph invariants (dup paths / cycles / unknown
// depends_on) when the plan was originally produced, so loading a
// file that fell out of an earlier Run can trust those properties.
// Hand-edited plan files are a supported but edge-case workflow;
// they get the same structural check as a tool-emitted plan.
//
// Returns (nil, err) on any filesystem / JSON / validation failure
// so the caller can surface a clean error rather than proceeding
// with a corrupt plan.
func LoadChangePlanFromFile(path string) (*ChangePlan, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("LoadChangePlanFromFile: empty path")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("LoadChangePlanFromFile: read %s: %w", path, err)
	}
	var plan ChangePlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, fmt.Errorf("LoadChangePlanFromFile: unmarshal %s: %w", path, err)
	}
	if strings.TrimSpace(plan.ID) == "" {
		return nil, fmt.Errorf("LoadChangePlanFromFile: %s has empty plan ID", path)
	}
	if len(plan.Changes) == 0 {
		return nil, fmt.Errorf("LoadChangePlanFromFile: %s has no changes[] — refusing to install an empty plan", path)
	}
	// Recompute TargetPaths from Changes so hand-edited plans whose
	// TargetPaths drifted from Changes (the emit tool keeps them in
	// sync but nothing guarantees hand-edits do) converge to a
	// consistent W1-enforcing snapshot.
	seen := make(map[string]struct{}, len(plan.Changes))
	canonical := make([]string, 0, len(plan.Changes))
	for _, c := range plan.Changes {
		if _, dup := seen[c.Path]; dup {
			return nil, fmt.Errorf("LoadChangePlanFromFile: %s has duplicate change for path %q (one-change-per-file constraint)", path, c.Path)
		}
		seen[c.Path] = struct{}{}
		canonical = append(canonical, c.Path)
	}
	plan.TargetPaths = canonical
	return &plan, nil
}
