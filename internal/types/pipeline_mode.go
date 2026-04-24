package types

// PipelineMode classifies a single Orchestrator.Run() invocation into
// read-only analysis vs one of the write-phase modes (plan / apply /
// verify). The zero value is ModeRead so every legacy caller that
// never touches PipelineMode gets the historical read-only behavior
// — this is load-bearing for the L1 red line (pre-B0 behavior
// byte-identical under --mode=read and when flag is omitted).
//
// The enum is set once at Run() entry from the CLI/yaml layer and
// never mutates mid-Run. Phase-dispatch switches in the orchestrator
// key off BusContext.Mode; the tool layer never reads it directly
// (tool-vs-mode gating lives in skill.ToolSuggestions, populated by
// the chosen agent/skill triple for the current Stage).
//
// The four values are mutually exclusive:
//
//   - ModeRead: analyze → explore → extract → finalize. No worktree
//     is created, no write-phase stages run. Default.
//
//   - ModePlan: analyze → explore → extract → plan → finalize.
//     Produces a ChangePlan artifact on disk (typically
//     .codrax/plans/<id>.json) and returns. Main repo HEAD untouched.
//     A worktree may be created for read-side probing but is
//     discarded at finalize time.
//
//   - ModeApply: requires --plan-file. Creates a git worktree
//     (detached HEAD), applies the plan inside, runs the apply
//     stage, then runs verify. No merge back to main repo — the
//     user is responsible for cherry-picking from the worktree if
//     they approve the resulting commit.
//
//   - ModeVerify: requires --plan-file and an already-applied
//     worktree (re-runs just the verify stage on existing state,
//     e.g. to retry flaky tests without redoing apply).
//
// PipelineMode is a string type so yaml serialization, REPL /mode
// commands, and log messages all share a single canonical form.
type PipelineMode string

const (
	// ModeRead is the default; read-only analysis pipeline.
	// Legacy callers that zero-value the field hit this path.
	ModeRead PipelineMode = "read"

	// ModePlan runs the full read pipeline plus a single plan
	// stage; emits a ChangePlan to disk and finalizes. No write
	// stages fire.
	ModePlan PipelineMode = "plan"

	// ModeApply consumes an existing ChangePlan (from --plan-file
	// or REPL /plan state) and runs plan + apply + verify stages
	// inside a detached-HEAD worktree.
	ModeApply PipelineMode = "apply"

	// ModeVerify re-runs only the verify stage against an existing
	// worktree + applied plan. Used to retry flaky tests or
	// re-check after a user tweaks the worktree by hand.
	ModeVerify PipelineMode = "verify"
)

// IsValid returns true when m is one of the four defined modes
// (including ModeRead, the zero value). The empty string is
// tolerated as ModeRead so pre-B0 tests and callers that never set
// the field continue to work — the orchestrator normalizes "" to
// ModeRead at Run() entry.
func (m PipelineMode) IsValid() bool {
	switch m {
	case "", ModeRead, ModePlan, ModeApply, ModeVerify:
		return true
	}
	return false
}

// IsWrite reports whether m activates any write-phase stage. Used
// by the orchestrator to gate worktree creation, by the CLI layer
// to require --auto-apply in single-shot mode, and by the skill
// registry to pick the write-mode skill set when wiring agents.
//
// ModeRead and the empty zero value both return false — only ModePlan
// is borderline: it produces a ChangePlan artifact but does not
// modify the repository. It still returns true here because the
// orchestrator needs to stand up a worktree for some plan-stage
// probing and the user-visible semantics (plan.json on disk) is a
// write-phase deliverable even if the repo bytes stay clean.
func (m PipelineMode) IsWrite() bool {
	switch m {
	case ModePlan, ModeApply, ModeVerify:
		return true
	}
	return false
}

// Normalize coerces an empty string to ModeRead and passes every
// other value through unchanged. Used at the single Orchestrator
// entry point to establish the invariant that BusContext.Mode is
// always one of the four named constants (never "") from Run()
// onward — downstream code can switch on exact equality.
func (m PipelineMode) Normalize() PipelineMode {
	if m == "" {
		return ModeRead
	}
	return m
}
