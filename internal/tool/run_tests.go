package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// RunTests executes the project's test suite inside the active
// worktree and stores a structured ChangeReport on Mutable. The
// B1.3 implementation covers four runners deterministically:
//
//   - Go:     `go test -json ./...` — native JSONL output
//   - Node:   `npm test -- --json` (jest) / `vitest --reporter=json`
//   - Python: `pytest --json-report --json-report-file=...`
//              (requires the pytest-json-report plugin; falls back
//              to fail-loud error on missing plugin so the operator
//              knows to install it)
//   - Rust:   `cargo test` (stable text output; parser extracts
//              pass/fail counts + failed test names via regex)
//
// The runner is detected by sniffing the worktree root for
// language-tagged manifest files (go.mod / package.json /
// pyproject.toml or pytest.ini / Cargo.toml). Detection is purely
// filesystem-based — no LLM judgment, no git blame.
//
// L3 red line: Execute MUST NOT invoke ground.BuildContext or
// ground.GroundItem. Test outputs are structured pass/fail data,
// not citations. Enforced by write_mode_red_lines_test.go.
//
// Classified ReadOnly + NonEvidenceTool. Tests read the repo and
// produce verdicts but do not mutate the worktree — the file I/O
// tests do happen through run_tests, but only under test-framework
// control (e.g. tmpdir fixtures) which the runner owns, not codrax.
type RunTests struct {
	ReadOnly
	NonEvidenceTool
}

// runTestsParams is the wire-level payload. All fields optional.
type runTestsParams struct {
	// Suite is a language-specific selector. Empty = all tests.
	// Semantics:
	//   Go:     package pattern (default "./...")
	//   Node:   test name / path pattern
	//   Python: pytest node-id or file path
	//   Rust:   test name filter passed after --
	Suite string `json:"suite,omitempty"`

	// TimeoutSeconds overrides the default per-suite timeout
	// (default 300 = 5 minutes). Operators bump for large suites.
	TimeoutSeconds int `json:"timeout_seconds,omitempty"`
}

// Name returns the stable tool identifier.
func (t *RunTests) Name() string { return "run_tests" }

// Description is one sentence + the supported-runner list so
// operators reading logs know the B1.3 scope without reading code.
func (t *RunTests) Description() string {
	return "Run the project test suite inside the active worktree and emit a structured ChangeReport. " +
		"B1.3 supports Go (go test -json), Node (jest/vitest --json), Python (pytest --json-report plugin required), and Rust (cargo test text)."
}

// Parameters returns the JSON schema.
func (t *RunTests) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "suite": {
      "type": "string",
      "description": "Language-specific test selector. Go: package pattern (default ./...). Node: test name/path pattern. Python: pytest node-id. Rust: filter string. Empty = run all tests."
    },
    "timeout_seconds": {
      "type": "integer",
      "description": "Per-suite timeout override (default 300)."
    }
  }
}`)
}

// Execute runs the 3-step flow: detect runner → run command →
// parse output → install Mutable.ChangeReport. On any step-level
// failure, returns a descriptive ToolResult.Summary so the verify
// stage surfaces the reason cleanly.
func (t *RunTests) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	if ctx == nil || ctx.Mutable == nil {
		return errResult(t.Name(), "run_tests requires BusContext.Mutable"), nil
	}
	if ctx.RepoRoot == "" {
		return errResult(t.Name(),
			"run_tests requires ctx.RepoRoot — orchestrator must have swapped to a worktree before verify"), nil
	}

	var p runTestsParams
	if len(params) > 0 {
		if err := json.Unmarshal(params, &p); err != nil {
			return errResult(t.Name(), fmt.Sprintf("invalid params: %v", err)), err
		}
	}

	// Step 1: detect runner.
	runner := detectRunner(ctx.RepoRoot)
	if runner == "" {
		return errResult(t.Name(),
			"run_tests: no supported test runner detected in "+ctx.RepoRoot+
				" (looked for go.mod / package.json / pyproject.toml / pytest.ini / Cargo.toml)"), nil
	}
	logging.Info("[run_tests] detected runner=%s in %s", runner, ctx.RepoRoot)

	timeout := 300 * time.Second
	if p.TimeoutSeconds > 0 {
		timeout = time.Duration(p.TimeoutSeconds) * time.Second
	}

	// Step 2: run command + capture output.
	cmdStr, extraFile := buildRunCommand(runner, p.Suite, ctx.RepoRoot)
	if extraFile != "" {
		defer os.Remove(extraFile)
	}

	execCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := NewShellCommandContext(execCtx, cmdStr)
	cmd.Dir = ctx.RepoRoot
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	// Non-zero exit is normal when tests fail — parser handles
	// that case by populating per-test failures. We explicitly
	// discard the command error; the parser is the source of
	// truth for pass/fail.
	_ = cmd.Run()
	output := buf.String()

	if execCtx.Err() == context.DeadlineExceeded {
		// Store full output for post-mortem; report includes the
		// blob ref so the operator can inspect via /history or
		// the blob directory.
		_, ref := StoreBlob(ctx, t.Name()+"-timeout", output)
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   fmt.Sprintf("[run_tests: runner=%s] command timed out after %v (set timeout_seconds to bump)", runner, timeout),
			RawRef:    ref,
			Timestamp: time.Now(),
		}, nil
	}

	// Step 3: parse output. runErr is typically non-nil when any
	// test failed — the parser handles that case by returning a
	// report with Passed=false + per-test details. Parse failures
	// are a different class and surface as tool errors.
	report, err := parseRunnerOutput(runner, output, extraFile, cmdStr)
	if err != nil {
		logging.Warning("[run_tests] parser error for runner=%s: %v", runner, err)
		_, ref := StoreBlob(ctx, t.Name()+"-unparsed", output)
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   fmt.Sprintf("[run_tests: runner=%s] output parser failed: %v — raw output stored for inspection", runner, err),
			RawRef:    ref,
			Timestamp: time.Now(),
		}, nil
	}

	// Anchor the report to the current plan if one is installed.
	// The verify-stage e2e flow always has a plan; standalone
	// test runs (hypothetical future) may not.
	if plan := ctx.Mutable.ChangePlan(); plan != nil {
		report.PlanID = plan.ID
	}
	report.GeneratedAt = time.Now()
	ctx.Mutable.SetChangeReport(report)

	// Build a summary for the tool-result return. Runner output
	// can be multi-megabyte; we show only the aggregate counts +
	// per-failure names so the LLM / REPL consumer doesn't drown.
	summary := renderTestSummary(runner, report)
	// Always blob the raw output so the operator can drill down if
	// the summary is ambiguous.
	_, ref := StoreBlob(ctx, t.Name(), output)

	success := report.Passed
	logging.Info("[run_tests] runner=%s passed=%v total=%d failed=%d",
		runner, report.Passed, len(report.TestResults), countFailed(report.TestResults))

	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   success,
		Summary:   summary,
		RawRef:    ref,
		Timestamp: time.Now(),
	}, nil
}

// detectRunner sniffs the worktree root for language-tagged
// manifest files and returns a short tag naming the runner. Empty
// string means "no supported runner found". Priority order when
// multiple manifests coexist (polyglot repo):
//
//   1. go.mod → "go"        (Go preferred for dogfooding)
//   2. package.json → "node" (check scripts.test to infer jest/vitest/mocha)
//   3. pyproject.toml / pytest.ini / setup.py → "python"
//   4. Cargo.toml → "rust"
//
// Operators with a non-standard layout can hand-edit the
// test-execute-skill prompt to force a specific runner, but the
// default sniff should cover 95%+ of single-language repos.
func detectRunner(repoRoot string) string {
	checks := []struct {
		file   string
		runner string
	}{
		{"go.mod", "go"},
		{"package.json", "node"},
		{"pyproject.toml", "python"},
		{"pytest.ini", "python"},
		{"setup.py", "python"},
		{"Cargo.toml", "rust"},
	}
	for _, c := range checks {
		if _, err := os.Stat(filepath.Join(repoRoot, c.file)); err == nil {
			return c.runner
		}
	}
	return ""
}

// buildRunCommand assembles the shell command string for a runner.
// Returns (command, extraFile) — extraFile is a temp file path
// the runner writes its JSON output to (pytest-json-report uses
// a file argument; other runners print to stdout). Empty extraFile
// means parse from stdout.
func buildRunCommand(runner, suite, repoRoot string) (string, string) {
	switch runner {
	case "go":
		pkg := strings.TrimSpace(suite)
		if pkg == "" {
			pkg = "./..."
		}
		return fmt.Sprintf("go test -json %s", pkg), ""
	case "node":
		// Prefer project's npm script when it exists (respects
		// monorepo / workspace config). Add --json to whatever
		// the script runs; jest and vitest both accept it.
		filter := strings.TrimSpace(suite)
		if filter == "" {
			return "npm test -- --json --silent", ""
		}
		return fmt.Sprintf("npm test -- --json --silent -t %q", filter), ""
	case "python":
		// pytest-json-report writes to a file specified by
		// --json-report-file. We use a temp file in the repo's
		// worktree so the report doesn't escape.
		tmpFile := filepath.Join(repoRoot, ".codrax-pytest-report.json")
		filter := strings.TrimSpace(suite)
		if filter == "" {
			return fmt.Sprintf("pytest --json-report --json-report-file=%q", tmpFile), tmpFile
		}
		return fmt.Sprintf("pytest %q --json-report --json-report-file=%q", filter, tmpFile), tmpFile
	case "rust":
		filter := strings.TrimSpace(suite)
		if filter == "" {
			return "cargo test", ""
		}
		return fmt.Sprintf("cargo test %q", filter), ""
	}
	return "", ""
}

// renderTestSummary builds the one-paragraph string for the
// tool-result Summary. Shows aggregate counts + failed test names
// (if any, cap 10 so the Summary stays under a reasonable length).
// Consumed by the verifier agent's LLM turn as context; full
// per-test detail lives in Mutable.ChangeReport.TestResults.
func renderTestSummary(runner string, report *types.ChangeReport) string {
	total := len(report.TestResults)
	failed := countFailed(report.TestResults)
	passed := total - failed
	verdict := "PASSED"
	if !report.Passed {
		verdict = "FAILED"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[run_tests: runner=%s verdict=%s] %d total, %d passed, %d failed",
		runner, verdict, total, passed, failed)
	if failed > 0 {
		b.WriteString("\n\nFailed tests:")
		shown := 0
		for _, r := range report.TestResults {
			if r.Passed {
				continue
			}
			shown++
			if shown > 10 {
				fmt.Fprintf(&b, "\n  ... (+%d more)", failed-10)
				break
			}
			fmt.Fprintf(&b, "\n  - %s (%s)", r.AssertionID, r.Suite)
			if r.FailureDetail != "" {
				// Clip failure detail to first line + ≤160 chars.
				line := strings.SplitN(r.FailureDetail, "\n", 2)[0]
				if len(line) > 160 {
					line = line[:160] + "…"
				}
				fmt.Fprintf(&b, "\n    %s", line)
			}
		}
	}
	return b.String()
}

// countFailed returns how many TestResult entries have Passed=false.
func countFailed(results []types.TestResult) int {
	n := 0
	for _, r := range results {
		if !r.Passed {
			n++
		}
	}
	return n
}
