package types

// PipelineMode classifies a single Orchestrator.Run() invocation into
// read-only analysis vs one of the write-phase modes (plan / apply /
// verify / audit). The zero value is ModeRead so every legacy caller that
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
// The values are mutually exclusive:
//
//   - ModeRead: analyze → explore → extract → finalize. No worktree
//     is created, no write-phase stages run. Default.
//
//   - ModePlan: analyze → write_analyze → write_controller.
//     The controller may run read-only exploration and emit a
//     bounded ChangePlan artifact on disk (typically
//     .codrax/plans/<id>.json), then stops before mutation. Main
//     repo HEAD remains untouched.
//
//   - ModeApply: analyze → write_analyze → write_controller.
//     The controller can explore, plan, apply, verify, replan,
//     split, append, finish, or block through typed actions. Any
//     supplied --plan-file is imported as a single-batch workflow
//     seed and still passes through the final risk/approval gate.
//
//   - ModeVerify: verifies an active workflow batch, a saved run,
//     or an imported --plan-file seed through the same durable
//     controller path.
//
//   - ModeAudit: loads existing typed write artifacts and prints an
//     offline replay/audit summary. It does not enter the orchestrator,
//     create a worktree, call tools, or call LLMs.
//
// PipelineMode is a string type so yaml serialization, REPL /mode
// commands, and log messages all share a single canonical form.
type PipelineMode string

const (
	// ModeRead is the default; read-only analysis pipeline.
	// Legacy callers that zero-value the field hit this path.
	ModeRead PipelineMode = "read"

	// ModePlan runs the write controller far enough to explore and
	// emit a bounded ChangePlan. No apply mutation fires.
	ModePlan PipelineMode = "plan"

	// ModeApply runs the write controller end-to-end. It may start
	// from a fresh request, an active workflow run, or an imported
	// --plan-file seed.
	ModeApply PipelineMode = "apply"

	// ModeVerify asks the write controller to verify an active batch,
	// saved workflow run, or imported --plan-file seed.
	ModeVerify PipelineMode = "verify"

	// ModeAudit is an advanced read-only write-artifact audit lane.
	ModeAudit PipelineMode = "audit"
)

// IsValid returns true when m is one of the defined modes
// (including ModeRead, the zero value). The empty string is
// tolerated as ModeRead so pre-B0 tests and callers that never set
// the field continue to work — the orchestrator normalizes "" to
// ModeRead at Run() entry.
func (m PipelineMode) IsValid() bool {
	switch m {
	case "", ModeRead, ModePlan, ModeApply, ModeVerify, ModeAudit:
		return true
	}
	return false
}

// IsWrite reports whether m activates any write-phase stage. Used
// by the orchestrator to gate worktree creation, by the CLI layer
// to route explicit write invocations, and by the skill registry to
// pick the write-mode skill set when wiring agents.
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
// always one of the named constants (never "") from Run()
// onward — downstream code can switch on exact equality.
func (m PipelineMode) Normalize() PipelineMode {
	if m == "" {
		return ModeRead
	}
	return m
}
