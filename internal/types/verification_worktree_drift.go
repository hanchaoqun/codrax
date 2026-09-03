package types

import "strings"

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

// Closed outcome set of a VerificationLockedReverify record (F-run-tests
// fold-in of §40.36: the labels are accurate — a locked re-run is skipped
// as "report failed" only when the report actually failed, and as "suite
// infra downgraded" when the primary suite of the owning runner was cut
// short by a timeout / memory cap / CPU cap so re-running it under the same
// caps would only die again).
const (
	VerificationLockedReverifyPassed        = "passed"
	VerificationLockedReverifyFailed        = "failed"
	VerificationLockedReverifyDriftRecurred = "drift_recurred"
	VerificationLockedReverifyUnavailable   = "unavailable"
	// VerificationLockedReverifySkippedReportFailed: the report is not
	// Passed; the locked re-run would fail for the suite's own reasons.
	VerificationLockedReverifySkippedReportFailed = "skipped_report_failed"
	// VerificationLockedReverifySkippedSuiteInfraDowngraded: the owning
	// runner's primary suite was killed by an infrastructure cap; the
	// fixed point is left UNPROVEN and disclosed, never refused.
	VerificationLockedReverifySkippedSuiteInfraDowngraded = "skipped_suite_infra_downgraded"
)

// VerificationLockedReverify records one locked re-run of an executed runner
// after a dependency-lockfile refresh. Outcome is closed (constants above):
// passed | failed | drift_recurred | unavailable | skipped_report_failed |
// skipped_suite_infra_downgraded.
type VerificationLockedReverify struct {
	Runner       string   `json:"runner"`
	Framework    string   `json:"framework,omitempty"`
	WorkingDir   string   `json:"working_dir,omitempty"`
	Command      string   `json:"command,omitempty"`
	ExitCode     int      `json:"exit_code"`
	Outcome      string   `json:"outcome"`
	ReasonCode   string   `json:"reason_code,omitempty"`
	DriftedPaths []string `json:"drifted_paths,omitempty"`
	// SuiteOutcome is the launched-outcome label of the primary suite that
	// made the gate skip the locked re-run (timeout | oom | cpu_limit); set
	// only with Outcome=skipped_suite_infra_downgraded.
	SuiteOutcome string `json:"suite_outcome,omitempty"`
}

// VerificationLockfileFixedPoint is the typed witness state of one
// dependency_lockfile_refresh row: whether the locked re-run proved the
// refreshed lockfile is a fixed point. It is the single source every
// disclosure surface reads (verify note zh/en, run_tests summary, final
// report residual risks, context pack, controller prompt); no renderer
// re-derives it from LockedReverify records or free text.
type VerificationLockfileFixedPoint string

const (
	// VerificationLockfileFixedPointProven: the locked re-run exited 0 with
	// no further tracked drift.
	VerificationLockfileFixedPointProven VerificationLockfileFixedPoint = "proven"
	// VerificationLockfileFixedPointDisproven: the locked re-run failed,
	// drifted again or was unavailable; the row is refused.
	VerificationLockfileFixedPointDisproven VerificationLockfileFixedPoint = "disproven"
	// VerificationLockfileFixedPointUnprovenSuiteInfraDowngraded: the
	// owning runner's suite was cut short by an infrastructure cap, so the
	// locked re-run was not attempted; the row stays disclosed.
	VerificationLockfileFixedPointUnprovenSuiteInfraDowngraded VerificationLockfileFixedPoint = "unproven_suite_infra_downgraded"
	// VerificationLockfileFixedPointUnprovenReportFailed: the report failed
	// for the suite's own reasons, so the locked re-run was not attempted.
	VerificationLockfileFixedPointUnprovenReportFailed VerificationLockfileFixedPoint = "unproven_report_failed"
)

// AllVerificationLockfileFixedPoints is the closed set in stable order.
func AllVerificationLockfileFixedPoints() []VerificationLockfileFixedPoint {
	return []VerificationLockfileFixedPoint{
		VerificationLockfileFixedPointProven,
		VerificationLockfileFixedPointDisproven,
		VerificationLockfileFixedPointUnprovenSuiteInfraDowngraded,
		VerificationLockfileFixedPointUnprovenReportFailed,
	}
}

// VerificationLockfileFixedPointUnproven reports whether a DISCLOSED lockfile
// row lacks its fixed-point witness (the class definition promises one), so
// every disclosure surface must say so in plain words.
func VerificationLockfileFixedPointUnproven(fp VerificationLockfileFixedPoint) bool {
	switch fp {
	case VerificationLockfileFixedPointUnprovenSuiteInfraDowngraded, VerificationLockfileFixedPointUnprovenReportFailed:
		return true
	default:
		return false
	}
}

// VerificationLockfileFixedPointDisclosure is the customer-facing plain-words
// phrase for an unproven lockfile fixed point ("" for proven / disproven /
// not applicable). It never contains a comma so risk details that join rows
// with "," stay parseable.
func VerificationLockfileFixedPointDisclosure(fp VerificationLockfileFixedPoint, zh bool) string {
	switch fp {
	case VerificationLockfileFixedPointUnprovenSuiteInfraDowngraded:
		if zh {
			return "锁文件定点未证明：测试套件被超时或资源上限中止，未能执行锁定复验"
		}
		return "lockfile fixed point UNPROVEN: the test suite was cut short by a timeout or resource cap before a locked re-run could prove it"
	case VerificationLockfileFixedPointUnprovenReportFailed:
		if zh {
			return "锁文件定点未证明：测试套件失败，未执行锁定复验"
		}
		return "lockfile fixed point UNPROVEN: the test suite failed so no locked re-run was attempted"
	default:
		return ""
	}
}

// VerificationWorktreeUntrackedRetainedPaths is the one predicate every
// disclosure surface uses for the untracked lane: new untracked outputs the
// verification run left behind (retained, not committed, not auto-deleted).
// It reads only the effect rows, never the audit status, so a refused
// (tracked_drift) run discloses its untracked outputs exactly like a
// disclosed or clean-tracked run does. limit<=0 means unbounded.
func VerificationWorktreeUntrackedRetainedPaths(effects []VerificationWorktreeEffect, limit int) []string {
	var paths []string
	seen := map[string]bool{}
	for _, effect := range effects {
		if effect.Kind != VerificationWorktreeEffectUntrackedCreated {
			continue
		}
		path := strings.TrimSpace(effect.Path)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		paths = append(paths, path)
		if limit > 0 && len(paths) >= limit {
			break
		}
	}
	return paths
}

// UntrackedRetainedPaths is the audit-level form of
// VerificationWorktreeUntrackedRetainedPaths (nil-safe).
func (a *VerificationWorktreeAudit) UntrackedRetainedPaths(limit int) []string {
	if a == nil {
		return nil
	}
	return VerificationWorktreeUntrackedRetainedPaths(a.Effects, limit)
}

// HasUntrackedRetainedOutput reports whether the run left any untracked
// output behind, independent of the tracked lane's disposition.
func (a *VerificationWorktreeAudit) HasUntrackedRetainedOutput() bool {
	return a != nil && len(VerificationWorktreeUntrackedRetainedPaths(a.Effects, 1)) > 0
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
