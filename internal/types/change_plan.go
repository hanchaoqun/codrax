package types

import (
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
	// planner. Awaiting /approve (REPL) or --auto-apply (single-shot).
	PlanStatusPending = "pending_approval"
	// PlanStatusApplied means apply + verify both succeeded.
	PlanStatusApplied = "applied"
	// PlanStatusApplyFailed means apply_patch rejected a change
	// (W1 / W1b violation, git-apply refusal, stat error). Verify
	// never ran.
	PlanStatusApplyFailed = "applied_failed"
	// PlanStatusVerifyFailed means apply landed but verify's tests
	// failed. Distinct from applied_failed so operators can tell
	// "plan was broken" from "plan applied but surface regressed".
	PlanStatusVerifyFailed = "verify_failed"
	// PlanStatusRejected is set by /reject — user reviewed the
	// plan and decided not to approve it.
	PlanStatusRejected = "rejected"
)

// ChangePlan is the Plan stage's on-disk artifact — a structured
// description of the code change the planner intends to apply. Stored
// as .codrax/plans/<id>.json by emit_change_plan.Execute, consumed by
// the apply stage when the user opts in via --mode=apply --plan-file.
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
//  2. User approves (REPL /approve or --auto-apply): Status="approved"
//     and a separate Run() with --mode=apply consumes the file.
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

	// AcceptanceTests names the test assertions the verify stage
	// must cover before this plan is considered successful.
	// Natural-language in B0 skeleton; B1 can promote to Criterion
	// IR (open question #1). Empty slice is legal — plan passes
	// verify trivially.
	AcceptanceTests []string `json:"acceptance_tests,omitempty"`

	// TargetPaths is the declared write scope — the set of files
	// the apply stage is allowed to modify. Populated from Changes
	// but also stored independently so the apply-stage pre-flight
	// (W1 enforcer) can refuse to run if a ChangeUnit's patch
	// reaches outside this list.
	TargetPaths []string `json:"target_paths"`

	// Status transitions — see the PlanStatus* constants below.
	// Legal values: pending_approval, approved, applied,
	// applied_failed (apply tool failed), verify_failed (tests
	// failed post-apply), rejected.
	Status string `json:"status"`

	// AppliedCommitSHA is the git commit SHA produced inside the
	// worktree after all ChangeUnits applied successfully. Empty
	// while Status != "applied". B0 worktree-as-dry-run design:
	// this SHA lives inside the worktree, never cherry-picked to
	// main repo automatically — the user is responsible for merging.
	AppliedCommitSHA string `json:"applied_commit_sha,omitempty"`

	// WorktreePath is the absolute path of the git worktree the
	// apply stage ran in. Left on disk for user inspection after
	// --mode=apply finishes; cleaned up by the next
	// worktree.PruneDeadSessions startup reap or explicit REPL
	// /discard.
	WorktreePath string `json:"worktree_path,omitempty"`

	// CreatedAt is the plan emission timestamp (wall clock).
	// Serves as the tie-breaker when PlanStore.List sorts plans.
	CreatedAt time.Time `json:"created_at"`

	// AppliedAt is set when the apply stage commits successfully.
	// Empty while Status != "applied".
	AppliedAt *time.Time `json:"applied_at,omitempty"`
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
	//   - "patch":  B2 addition — apply unified diff in Patch field
	Kind string `json:"kind"`

	// NewContent is the full file body for create/modify. Empty for
	// delete. Unset for patch (B2).
	NewContent string `json:"new_content,omitempty"`

	// Patch is the unified-diff payload for kind="patch". B2
	// addition; empty in B0 skeleton.
	Patch string `json:"patch,omitempty"`

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
}

// ChangeReport is the Verify stage's output — a structured summary
// of what passed, what failed, and any performance impact. Written
// alongside the plan JSON as .codrax/plans/<plan-id>.report.json so
// the verify outcome is recoverable across process restarts.
//
// Consumed by ShapeChangeReport's answer rendering path: the
// finalize stage reads this file and renders a user-visible
// AnswerDocument describing the change and its test outcome.
type ChangeReport struct {
	// PlanID names the ChangePlan this report corresponds to.
	// One-to-one: a single plan produces zero-or-one report.
	PlanID string `json:"plan_id"`

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

	// FailureKind classifies the terminal state when Passed=false so
	// the verify→plan retry hint can surface resource exhaustion
	// distinctly from a normal red test ("tests_failed"). Empty +
	// Passed=false defaults to "tests_failed". Set by run_tests when
	// the SupervisedRun exit kind is timeout / OOM / cpu_limit, and
	// preserved through merge/qualify so the heuristic hint reads it
	// before the planner re-dispatches.
	//
	// Provenance: the OOM-killed pytest event (RSS 2.47 GiB after 9 h
	// of CPU) demonstrated that a generic "tests failed" hint sends
	// the planner re-deriving the same bug from the same noisy
	// stderr; surfacing the resource-exhaustion signal explicitly
	// gives it a corrective direction the symptoms alone don't.
	FailureKind FailureKind `json:"failure_kind,omitempty"`

	// GeneratedAt is the verify-stage completion timestamp.
	GeneratedAt time.Time `json:"generated_at"`
}

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
)

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
	File    string `json:"file"`              // repo-relative or absolute as printed by the tool
	Line    int    `json:"line,omitempty"`    // 1-based; 0 = unknown
	Column  int    `json:"column,omitempty"`  // 1-based; 0 = unknown
	Symbol  string `json:"symbol,omitempty"`  // optional — e.g. "TS2304" / "AssertionError"
	Message string `json:"message"`           // the compiler's error text (single line)
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
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("WriteChangeReportToFile: marshal: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("WriteChangeReportToFile: write %s: %w", path, err)
	}
	return nil
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
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return fmt.Errorf("UpdatePlanStatusOnDisk marshal: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("UpdatePlanStatusOnDisk write %s: %w", path, err)
	}
	return nil
}

// LoadChangePlanFromFile reads a ChangePlan JSON from disk and
// returns a deserialised *ChangePlan. Called by the orchestrator's
// the apply stage hook when --mode=apply --plan-file=X needs to install
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
