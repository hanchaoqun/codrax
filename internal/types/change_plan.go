package types

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
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

	// Status transitions: "pending_approval" → "approved" → "applied"
	// (on success) or "rejected" / "applied_failed" on failure.
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

	// FailureSummary is a human-readable explanation when
	// Passed=false. Rendered in the user-visible AnswerDocument.
	// Empty when Passed=true.
	FailureSummary string `json:"failure_summary,omitempty"`

	// GeneratedAt is the verify-stage completion timestamp.
	GeneratedAt time.Time `json:"generated_at"`
}

// TestResult is one row in ChangeReport.TestResults. Mirrors the
// shape common test harnesses (go test -json, pytest json-report,
// mocha --reporter json) expose so the B3 parser surface is
// minimal.
type TestResult struct {
	// AssertionID is the canonical identifier used for matching
	// against ChangePlan.AcceptanceTests. Format is framework-
	// specific ("TestFoo", "test_module.TestClass.test_method").
	AssertionID string `json:"assertion_id"`

	// Suite groups related assertions (Go package path, pytest
	// module, etc.). Same suite on all tests from one
	// run_tests invocation.
	Suite string `json:"suite"`

	// Passed is true when the framework reported success.
	Passed bool `json:"passed"`

	// Duration is observed wall-clock time.
	Duration time.Duration `json:"duration"`

	// FailureDetail is framework stdout/stderr excerpt when the
	// test failed; empty on pass. Bounded to ~2 KB per test so a
	// ChangeReport stays readable.
	FailureDetail string `json:"failure_detail,omitempty"`
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
// B1.3 runVerifyPhase calls this after the run_tests parser
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

// LoadChangePlanFromFile reads a ChangePlan JSON from disk and
// returns a deserialised *ChangePlan. Called by the orchestrator's
// runApplyPhase when --mode=apply --plan-file=X needs to install
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
