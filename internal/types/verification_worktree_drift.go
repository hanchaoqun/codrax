package types

// verification_worktree_drift.go — V5-2 (colleague_merge_audit §40.11): the
// tracked-drift gate gains a typed side-effect OWNER dimension. A tracked
// path changed by the verification run is classified into a closed set from
// typed facts only — the executed-runner roster, the closed per-runner
// side-effect manifest, the plan's typed output_path contracts, and a
// formatter fixed-point check — never from path prose or model rationale.
// Three classes are disclosed (Passed kept, advisory record); only
// unclassified drift keeps the hard failure.

// VerificationWorktreeDriftClass is the closed owner set of a tracked drift.
type VerificationWorktreeDriftClass string

const (
	// VerificationWorktreeDriftDependencyLockfileRefresh: a lockfile the
	// executed runner declares as its own (Cargo.lock, go.sum, …) was
	// rewritten by that toolchain and the locked re-verify proved the
	// refreshed file is a fixed point.
	VerificationWorktreeDriftDependencyLockfileRefresh VerificationWorktreeDriftClass = "dependency_lockfile_refresh"
	// VerificationWorktreeDriftFormatterNoSemanticDiff: formatting the
	// pre-run content with the runner language's canonical formatter yields
	// exactly the post-run bytes (reversibility witness).
	VerificationWorktreeDriftFormatterNoSemanticDiff VerificationWorktreeDriftClass = "formatter_no_semantic_diff"
	// VerificationWorktreeDriftDeclaredGeneratedOutput: the plan declared the
	// path as a generated output through a typed output_path behavior
	// contract.
	VerificationWorktreeDriftDeclaredGeneratedOutput VerificationWorktreeDriftClass = "declared_generated_output"
	// VerificationWorktreeDriftUnclassified: none of the typed lanes own the
	// path; the verification verdict is refused as before.
	VerificationWorktreeDriftUnclassified VerificationWorktreeDriftClass = "unclassified_tracked_drift"
)

// AllVerificationWorktreeDriftClasses is the closed set in stable order.
func AllVerificationWorktreeDriftClasses() []VerificationWorktreeDriftClass {
	return []VerificationWorktreeDriftClass{
		VerificationWorktreeDriftDependencyLockfileRefresh,
		VerificationWorktreeDriftFormatterNoSemanticDiff,
		VerificationWorktreeDriftDeclaredGeneratedOutput,
		VerificationWorktreeDriftUnclassified,
	}
}

// VerificationWorktreeDriftDisclosed reports whether the class is one the
// gate discloses instead of refusing.
func VerificationWorktreeDriftDisclosed(class VerificationWorktreeDriftClass) bool {
	switch class {
	case VerificationWorktreeDriftDependencyLockfileRefresh,
		VerificationWorktreeDriftFormatterNoSemanticDiff,
		VerificationWorktreeDriftDeclaredGeneratedOutput:
		return true
	default:
		return false
	}
}

// VerificationWorktreeEffectDisposition says what the gate did with one row.
type VerificationWorktreeEffectDisposition string

const (
	VerificationWorktreeEffectRefused   VerificationWorktreeEffectDisposition = "refused"
	VerificationWorktreeEffectDisclosed VerificationWorktreeEffectDisposition = "disclosed"
)

// Reason codes of the disclosed lane and its locked re-verify.
const (
	// VerificationTrackedSideEffectDisclosedReason: every tracked drift was
	// owned by a disclosed class; the verification verdict stands.
	VerificationTrackedSideEffectDisclosedReason = "verification_tracked_side_effect_disclosed"
	// VerificationLockedReverifyFailedReason: the runner's locked re-run
	// failed or drifted again, so the lockfile refresh is not a fixed point.
	VerificationLockedReverifyFailedReason = "lockfile_locked_reverify_failed"
)

// VerificationLockedReverify records one locked re-run of an executed runner
// after a dependency-lockfile refresh. Outcome is closed:
// passed | failed | drift_recurred | unavailable.
type VerificationLockedReverify struct {
	Runner       string   `json:"runner"`
	Framework    string   `json:"framework,omitempty"`
	WorkingDir   string   `json:"working_dir,omitempty"`
	Command      string   `json:"command,omitempty"`
	ExitCode     int      `json:"exit_code"`
	Outcome      string   `json:"outcome"`
	ReasonCode   string   `json:"reason_code,omitempty"`
	DriftedPaths []string `json:"drifted_paths,omitempty"`
}

// FailureKindReplanEscapeLane names the typed escape a replan-routed failure
// kind carries (§1.6 / §40.11 item 4). Every declared FailureKind must be
// registered here; the census in change_plan_failure_kind_census_test.go
// goes red for an unregistered kind.
type FailureKindReplanEscapeLane string

const (
	// FailureKindEscapeReplanOnly: code failure evidence; replan is the only
	// way forward (no disposition bypass).
	FailureKindEscapeReplanOnly FailureKindReplanEscapeLane = "replan_only"
	// FailureKindEscapeAcceptUnverified: verifier availability failure;
	// finish_disposition=accept_unverified may bypass.
	FailureKindEscapeAcceptUnverified FailureKindReplanEscapeLane = "accept_unverified"
	// FailureKindEscapeDriftOwnerLane: refused only for unclassified drift;
	// disclosed classes never enter the failure kind at all.
	FailureKindEscapeDriftOwnerLane FailureKindReplanEscapeLane = "drift_owner_lane"
)

// FailureKindReplanEscapeLanes is the registration table.
var FailureKindReplanEscapeLanes = map[FailureKind]FailureKindReplanEscapeLane{
	FailureKindTestsFailed:             FailureKindEscapeReplanOnly,
	FailureKindBuildFailure:            FailureKindEscapeReplanOnly,
	FailureKindTimeout:                 FailureKindEscapeReplanOnly,
	FailureKindOOM:                     FailureKindEscapeReplanOnly,
	FailureKindCPULimit:                FailureKindEscapeReplanOnly,
	FailureKindCrash:                   FailureKindEscapeReplanOnly,
	FailureKindVerificationSideEffect:  FailureKindEscapeDriftOwnerLane,
	FailureKindPreexistingBuildFailure: FailureKindEscapeAcceptUnverified,
	FailureKindRunnerMissing:           FailureKindEscapeAcceptUnverified,
	FailureKindParserError:             FailureKindEscapeAcceptUnverified,
	FailureKindVerificationIncomplete:  FailureKindEscapeAcceptUnverified,
	FailureKindNoTests:                 FailureKindEscapeAcceptUnverified,
}
