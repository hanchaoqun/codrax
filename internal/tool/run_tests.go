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
//     (requires the pytest-json-report plugin; falls back
//     to fail-loud error on missing plugin so the operator
//     knows to install it)
//   - Rust:   `cargo test` (stable text output; parser extracts
//     pass/fail counts + failed test names via regex)
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
// operators reading logs know the scope without reading code.
func (t *RunTests) Description() string {
	return "Run the project test suite inside the active worktree and emit a structured ChangeReport. " +
		"Supports Go (go test -json), Node (jest/vitest --json), Python (pytest --json-report plugin required), Rust (cargo test text), Java (Maven/Gradle JUnit XML), Kotlin (via the Java Gradle path — build.gradle.kts recognised), Ruby (RSpec --format json), CMake (ctest --output-junit; requires pre-configured build dir), Meson (meson test --xunit-file), raw Makefile (make check/test; pass/fail from exit code), HarmonyOS ArkTS via hvigor (hvigorw test → JUnit XML), and HarmonyOS Cangjie via cjpm (cjpm test → cargo-style text)."
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
				" (looked for go.mod / package.json / pyproject.toml / pytest.ini / setup.py / Cargo.toml / pom.xml / build.gradle[.kts] / Gemfile / CMakeLists.txt / meson.build / Makefile)"), nil
	}
	logging.Info("[run_tests] detected runner=%s in %s", runner, ctx.RepoRoot)

	// CMake and Meson require a pre-configured build directory;
	// run_tests refuses early with actionable guidance rather than
	// running a broken command. Checked here so Suite parsing /
	// timeout setup don't waste cycles.
	if runner == "cmake" || runner == "meson" {
		if detectNativeBuildDir(ctx.RepoRoot) == "" {
			return errResult(t.Name(),
				fmt.Sprintf("run_tests: %s project detected but no configured build directory found "+
					"(looked for build/, Build/, builddir/, out/, cmake-build-debug/, cmake-build-release/). "+
					"Configure the project first (e.g. `cmake -S . -B build` or `meson setup builddir`) and build it, "+
					"then re-run verify.", runner)), nil
		}
	}

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
	// For runners whose parser infers pass/fail from output markers
	// (go / node / python / rust / java XML), the exit code is
	// redundant and we preserve the prior behaviour of ignoring it.
	// The make runner has no structured output, so the parser reads
	// the exit error directly via parseRunnerOutput's new runErr
	// parameter.
	runErr := cmd.Run()
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

	// Step 2b: post-exec hooks for runners whose test reports live
	// in a separate file/dir rather than on stdout.
	//
	//   - Java (Maven/Gradle): JUnit XML dir at target/surefire-reports
	//     or build/test-results/test; locateJUnitReportDir picks.
	//   - CMake: ctest --output-junit writes a single XML file we
	//     passed as extraFile. If that file didn't appear, ctest
	//     never reached the test phase (configure/build broken).
	//   - Meson: meson test --xunit-file writes a single XML file.
	//
	// When the expected report artifact is absent we synthesise a
	// build-failure ChangeReport so the retry hint + verifier prompt
	// carry the actual error text instead of an opaque pointer to the
	// stored blob. Mirrors the Rust cargo-build fallback in
	// parseCargoOutput.
	if runner == "java" {
		reportDir := locateJUnitReportDir(ctx.RepoRoot)
		if reportDir == "" {
			return synthesizeBuildFailureReport(ctx, t.Name(), runner, "Java", output)
		}
		extraFile = reportDir
	}
	if runner == "hvigor" {
		// HarmonyOS hvigor emits JUnit XML into the same dir shapes
		// as Gradle; the Java helper works as-is because it scans
		// for `surefire-reports/` / `test-results/test/` inside any
		// module sub-tree. A missing report means hvigor's build
		// failed before the test phase — surface as build failure.
		reportDir := locateJUnitReportDir(ctx.RepoRoot)
		if reportDir == "" {
			return synthesizeBuildFailureReport(ctx, t.Name(), runner, "hvigor", output)
		}
		extraFile = reportDir
	}
	if runner == "cmake" || runner == "meson" {
		// extraFile is the tmpfile path; check it materialised and is
		// non-empty. A zero-byte file means ctest/meson exited before
		// writing any suite info (usually a configure/build regression
		// surfaced as a stderr dump in output).
		produced := false
		if extraFile != "" {
			if info, err := os.Stat(extraFile); err == nil && info.Size() > 0 {
				produced = true
			}
		}
		if !produced {
			label := "CMake"
			if runner == "meson" {
				label = "Meson"
			}
			return synthesizeBuildFailureReport(ctx, t.Name(), runner, label, output)
		}
	}

	// Step 3: parse output. runErr is typically non-nil when any
	// test failed — the parser handles that case by returning a
	// report with Passed=false + per-test details. Parse failures
	// are a different class and surface as tool errors. runErr is
	// forwarded so the make parser (which has no structured output)
	// can derive pass/fail from exit status.
	report, err := parseRunnerOutput(runner, output, extraFile, cmdStr, runErr)
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
//  1. go.mod → "go"        (Go preferred for dogfooding)
//  2. package.json → "node" (check scripts.test to infer jest/vitest/mocha)
//  3. pyproject.toml / pytest.ini / setup.py → "python"
//  4. Cargo.toml → "rust"
//  5. pom.xml / build.gradle / build.gradle.kts → "java"
//  6. Gemfile → "ruby"
//
// Operators with a non-standard layout can hand-edit the
// test-execute-skill prompt to force a specific runner, but the
// default sniff should cover 95%+ of single-language repos.
//
// Polyglot caveat: Maven is preferred over Gradle when both pom.xml
// and build.gradle coexist (industry norm; a polyglot repo with
// mixed build systems typically scripts its own test runner anyway).
func detectRunner(repoRoot string) string {
	checks := []struct {
		file   string
		runner string
	}{
		{"go.mod", "go"},
		// HarmonyOS ArkTS + Cangjie manifests must be probed before
		// package.json / build.gradle so a mixed HarmonyOS project
		// (Stage Model + Cangjie native) is routed to the right
		// runner. oh-package.json5 (ArkTS) and hvigorfile.ts mark
		// the project as hvigor; cjpm.toml marks a Cangjie module.
		{"oh-package.json5", "hvigor"},
		{"build-profile.json5", "hvigor"},
		{"hvigorfile.ts", "hvigor"},
		{"cjpm.toml", "cjpm"},
		{"package.json", "node"},
		{"pyproject.toml", "python"},
		{"pytest.ini", "python"},
		{"setup.py", "python"},
		{"Cargo.toml", "rust"},
		// Swift / Apple ecosystems via Swift Package Manager. Older
		// XCode-only projects with .xcodeproj are not auto-detected
		// here — they typically need `xcodebuild test -scheme X` and
		// the user wires that via a Makefile target. Package.swift
		// is the cross-platform path SPM uses.
		{"Package.swift", "swift"},
		{"pom.xml", "java"},
		{"build.gradle", "java"},
		{"build.gradle.kts", "java"},
		{"Gemfile", "ruby"},
		// C / C++ / other native-build stacks. Ordered by specificity:
		// a CMake project is more informative than a raw Makefile; a
		// meson project typically also has a Makefile in its build
		// dir but the top-level manifest is meson.build. Raw Makefile
		// is the last fallback for bare Autotools / hand-rolled builds.
		{"CMakeLists.txt", "cmake"},
		{"meson.build", "meson"},
		{"Makefile", "make"},
		{"makefile", "make"},
		{"GNUmakefile", "make"},
	}
	for _, c := range checks {
		if _, err := os.Stat(filepath.Join(repoRoot, c.file)); err == nil {
			return c.runner
		}
	}
	return ""
}

// detectNativeBuildDir locates a pre-configured build directory for
// a CMake or Meson project. Both tools require an out-of-tree build
// dir with their generator artifacts (Ninja files / Makefiles /
// CTestTestfile.cmake for CMake; meson-info/ for Meson); run_tests
// does NOT run the configure step itself (too slow, too many knobs).
// Returns the first match from the candidates list, or "" if the
// project hasn't been configured yet.
//
// Candidates cover the common in-repo layouts:
//
//   - "build" / "Build"        — cmake -S . -B build convention
//   - "builddir"               — meson default
//   - "out"                    — Google/Android-style
//   - "cmake-build-debug" and
//     "cmake-build-release"    — CLion IDE defaults
//
// Operators using a non-standard layout (e.g. sibling build dirs
// outside the repo root) can't be auto-detected; they must invoke
// tests via a project-specific script and fall through to the
// make runner.
func detectNativeBuildDir(repoRoot string) string {
	candidates := []string{
		"build", "Build",
		"builddir",
		"out",
		"cmake-build-debug",
		"cmake-build-release",
	}
	for _, name := range candidates {
		abs := filepath.Join(repoRoot, name)
		info, err := os.Stat(abs)
		if err != nil || !info.IsDir() {
			continue
		}
		// Peek for a generator sentinel so an empty dir left over from
		// a failed configure doesn't count. CMake drops
		// CMakeCache.txt; Meson drops meson-info/. We accept either —
		// the parser step tolerates whichever output shape the command
		// produces.
		if _, err := os.Stat(filepath.Join(abs, "CMakeCache.txt")); err == nil {
			return abs
		}
		if _, err := os.Stat(filepath.Join(abs, "meson-info")); err == nil {
			return abs
		}
	}
	return ""
}

// detectJavaBuildSystem returns "maven" when pom.xml exists, else
// "gradle" when build.gradle[.kts] exists. Called by
// buildRunCommand when runner=="java" so the command and the
// post-exec report locator both know which layout to target.
func detectJavaBuildSystem(repoRoot string) string {
	if _, err := os.Stat(filepath.Join(repoRoot, "pom.xml")); err == nil {
		return "maven"
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "build.gradle")); err == nil {
		return "gradle"
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "build.gradle.kts")); err == nil {
		return "gradle"
	}
	return ""
}

// locateJUnitReportDir walks well-known JUnit XML output locations
// and returns the FIRST non-empty directory it finds. Returns ""
// when no candidate directory carries XML (build failed before tests
// ran; caller surfaces a clear error).
//
// Priority order:
//
//  1. Maven's target/surefire-reports (Maven projects with a stale
//     Gradle build/ dir from an earlier toolchain switch shouldn't
//     accidentally point us there).
//  2. Gradle's build/test-results/test.
//  3. HarmonyOS hvigor commonly drops JUnit XML under one of:
//     `entry/build/default/intermediates/test/test-results/`
//     `<module>/build/intermediates/test/test-results/`
//     The candidate list covers the documented Stage Model layouts
//     plus a generic fallback at .codrax/test-results/.
//  4. Final fallback: bounded walk of the worktree looking for any
//     directory whose name ends in `test-results` / `test-result` /
//     `surefire-reports` and contains `.xml` files. Walk depth is
//     capped at 6 levels so we don't traverse node_modules / .git.
//
// Multi-module caveat: polyglot Java/Gradle repos may have multiple
// surefire-reports directories under submodules. The parser's XML
// walk handles that — we return the highest non-empty match and let
// parseJUnitXMLDir recurse.
func locateJUnitReportDir(repoRoot string) string {
	candidates := []string{
		filepath.Join(repoRoot, "target", "surefire-reports"),
		filepath.Join(repoRoot, "build", "test-results", "test"),
		// HarmonyOS hvigor — Stage Model entry module.
		filepath.Join(repoRoot, "entry", "build", "default", "intermediates", "test", "test-results"),
		filepath.Join(repoRoot, "entry", "build", "intermediates", "test", "test-results"),
		filepath.Join(repoRoot, "build", "intermediates", "test", "test-results"),
		filepath.Join(repoRoot, ".codrax", "test-results"),
	}
	for _, dir := range candidates {
		if dirHasXML(dir) {
			return dir
		}
	}
	// Bounded fallback walk. Catches non-standard hvigor layouts and
	// custom test-runner configurations without hardcoding every
	// possible output dir. Depth-cap 6 keeps the walk cheap on large
	// monorepos; skip-list excludes common large/unrelated dirs.
	if found := walkJUnitDir(repoRoot, 6); found != "" {
		return found
	}
	return ""
}

// dirHasXML reports whether dir exists and contains at least one
// `*.xml` file at its top level (we don't recurse — a directory with
// only nested .xml is a hint we're at the wrong level and should
// keep walking).
func dirHasXML(dir string) bool {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".xml") {
			return true
		}
	}
	return false
}

// walkJUnitDir descends from root looking for a directory whose
// basename matches the JUnit-output convention (`test-results`,
// `surefire-reports`, `test-report`) AND that contains at least one
// .xml file. Returns the first match, or "" on no match.
//
// Skip list excludes common large dirs that won't carry test
// reports (node_modules, .git, vendor, dist, etc.) so the walk stays
// cheap even on monorepos.
func walkJUnitDir(root string, maxDepth int) string {
	skip := map[string]struct{}{
		"node_modules": {},
		".git":         {},
		"vendor":       {},
		"dist":         {},
		"out":          {},
		".gradle":      {},
		".idea":        {},
		".vscode":      {},
	}
	matchNames := []string{"test-results", "surefire-reports", "test-report"}
	var found string
	var walk func(dir string, depth int) bool
	walk = func(dir string, depth int) bool {
		if depth > maxDepth || found != "" {
			return false
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return false
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			if _, skipped := skip[e.Name()]; skipped {
				continue
			}
			full := filepath.Join(dir, e.Name())
			for _, m := range matchNames {
				if strings.HasSuffix(e.Name(), m) {
					if dirHasXML(full) {
						found = full
						return true
					}
					// Sometimes the XML is one level deeper (Gradle
					// puts files under <suite>/ TEST-*.xml). Peek a
					// single child level for any .xml.
					if firstXMLDescendant(full, 2) {
						found = full
						return true
					}
				}
			}
			if walk(full, depth+1) {
				return true
			}
		}
		return false
	}
	walk(root, 0)
	return found
}

// firstXMLDescendant returns true if any .xml file lives within
// the given dir, walking up to maxDepth levels (1 = direct children
// only). Used by walkJUnitDir to detect Gradle-style nested layouts
// where the test-results dir contains a `test/` subfolder before the
// XML files.
func firstXMLDescendant(dir string, maxDepth int) bool {
	if maxDepth < 0 {
		return false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() {
			if strings.HasSuffix(strings.ToLower(e.Name()), ".xml") {
				return true
			}
			continue
		}
		if firstXMLDescendant(filepath.Join(dir, e.Name()), maxDepth-1) {
			return true
		}
	}
	return false
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
	case "swift":
		// Swift Package Manager. `swift test` emits text output that
		// the cargo-style parser handles cleanly because both follow
		// the `test result: ok. N passed; N failed` footer convention.
		// `--parallel` would break parsing; intentionally omitted.
		filter := strings.TrimSpace(suite)
		if filter == "" {
			return "swift test", ""
		}
		return fmt.Sprintf("swift test --filter %q", filter), ""
	case "java":
		// Maven + Gradle both write JUnit XML to a fixed
		// subdirectory; the parser's post-exec hook walks those
		// dirs regardless of which tool ran. The "extraFile"
		// channel isn't used here because the XML files are
		// multi-file in a directory, not one named file. The
		// command itself just needs to be `mvn test` / `gradle
		// test` so the reports get produced.
		filter := strings.TrimSpace(suite)
		build := detectJavaBuildSystem(repoRoot)
		switch build {
		case "maven":
			if filter == "" {
				// -Dsurefire.useFile=true is the Maven default;
				// we rely on target/surefire-reports/*.xml.
				return "mvn -B -q test", ""
			}
			return fmt.Sprintf("mvn -B -q -Dtest=%q test", filter), ""
		case "gradle":
			if filter == "" {
				return "./gradlew --no-daemon --console=plain test", ""
			}
			return fmt.Sprintf("./gradlew --no-daemon --console=plain test --tests %q", filter), ""
		}
		// Unknown Java layout — let command fail and the parser
		// surface a clear error. Shouldn't happen because
		// detectRunner matched one of pom.xml / build.gradle[.kts].
		return "mvn -B -q test", ""
	case "ruby":
		// RSpec with --format json prints a single top-level
		// JSON object to stdout. `bundle exec` ensures the
		// project's Gemfile-pinned rspec version is used.
		filter := strings.TrimSpace(suite)
		if filter == "" {
			return "bundle exec rspec --format json", ""
		}
		return fmt.Sprintf("bundle exec rspec --format json %q", filter), ""
	case "cmake":
		// ctest emits JUnit XML via --output-junit (CMake >= 3.21).
		// The build dir must already be configured + built; Execute
		// falls back to a clear error when detectNativeBuildDir
		// returns "". Filter maps to -R <regex> which ctest treats as
		// a POSIX ERE applied to the test name.
		buildDir := detectNativeBuildDir(repoRoot)
		if buildDir == "" {
			// Command will not be built; Execute refuses before
			// reaching this path. Keep a harmless no-op here so a
			// caller bypassing Execute sees a clear failure.
			return "", ""
		}
		// Tmpfile must end in .xml so parseJUnitXMLDir's suffix gate
		// accepts it when filepath.Walk passes it through.
		tmpFile := filepath.Join(repoRoot, ".codrax-ctest-report.xml")
		filter := strings.TrimSpace(suite)
		if filter == "" {
			return fmt.Sprintf("ctest --test-dir %q --output-junit %q --output-on-failure", buildDir, tmpFile), tmpFile
		}
		return fmt.Sprintf("ctest --test-dir %q --output-junit %q --output-on-failure -R %q",
			buildDir, tmpFile, filter), tmpFile
	case "meson":
		// meson test writes JUnit XML when --xunit-file is set. The
		// command runs from the build dir via -C. Filter maps to a
		// trailing positional arg (test name substring).
		buildDir := detectNativeBuildDir(repoRoot)
		if buildDir == "" {
			return "", ""
		}
		tmpFile := filepath.Join(repoRoot, ".codrax-meson-report.xml")
		filter := strings.TrimSpace(suite)
		if filter == "" {
			return fmt.Sprintf("meson test -C %q --xunit-file %q --print-errorlogs", buildDir, tmpFile), tmpFile
		}
		return fmt.Sprintf("meson test -C %q --xunit-file %q --print-errorlogs %q",
			buildDir, tmpFile, filter), tmpFile
	case "make":
		// Raw Makefile projects have no structured test output.
		// Prefer `make check` (GNU convention) over `make test` by
		// peeking at the Makefile for the target name; if neither is
		// obvious, default to `check` since it's the autotools /
		// GNU-ism most mature Makefile test suites follow. Parser
		// uses exit code for pass/fail and stdout for failure detail.
		target := detectMakeTestTarget(repoRoot)
		filter := strings.TrimSpace(suite)
		if filter != "" {
			// User supplied a specific target name.
			target = filter
		}
		return fmt.Sprintf("make %s", target), ""
	case "hvigor":
		// HarmonyOS ArkTS projects use hvigorw (wrapper for hvigor,
		// analogous to ./gradlew). `test` task runs unit + component
		// tests and emits JUnit XML into test results dirs that the
		// Java parser's post-exec JUnit walker already picks up.
		//
		// Unlike Java's pom.xml/build.gradle, hvigor sometimes only
		// ships the wrapper at repo root; a missing hvigorw falls
		// through to `hvigor` directly (PATH). Both are accepted; a
		// test environment without either still reaches the tool via
		// exit-code-only failure in Execute.
		filter := strings.TrimSpace(suite)
		wrapper := "hvigorw"
		if _, err := os.Stat(filepath.Join(repoRoot, "hvigorw")); err != nil {
			wrapper = "hvigor"
		}
		if filter == "" {
			return fmt.Sprintf("%s --no-daemon test", wrapper), ""
		}
		return fmt.Sprintf("%s --no-daemon test --tests %q", wrapper, filter), ""
	case "cjpm":
		// Cangjie's package manager. cjpm 1.0.0 LTS does not document
		// a stable `--json` reporter flag, so the runner uses the
		// default text output; parseCjpmText parses the `test result:
		// ok. N passed; N failed` footer (reused from cargo parser).
		// A future cjpm release that adds `--json` can be probed by
		// parsing `cjpm test --help` at runtime; for now the text
		// path is the single source of truth.
		filter := strings.TrimSpace(suite)
		if filter == "" {
			return "cjpm test", ""
		}
		return fmt.Sprintf("cjpm test --filter %q", filter), ""
	}
	return "", ""
}

// detectMakeTestTarget reads the Makefile head and returns the test
// target name the project uses. Preference order: "check" (GNU /
// autotools convention) > "test" (CMake-generated / npm-style) >
// "tests" (occasional plural). Defaults to "check" when neither
// target is present — `make check` is the widely-accepted standard
// and a missing target surfaces a clean "No rule to make target
// 'check'" error the parser can relay.
//
// Note: this is a best-effort grep for `^<target>:`. A Makefile that
// builds its test target dynamically (via included fragments or
// $(eval)) may hide the target from this scan; those projects can
// override via the Suite parameter.
func detectMakeTestTarget(repoRoot string) string {
	candidates := []string{"Makefile", "makefile", "GNUmakefile"}
	var makefilePath string
	for _, name := range candidates {
		p := filepath.Join(repoRoot, name)
		if _, err := os.Stat(p); err == nil {
			makefilePath = p
			break
		}
	}
	if makefilePath == "" {
		return "check"
	}
	data, err := os.ReadFile(makefilePath)
	if err != nil {
		return "check"
	}
	// Scan for the first target named "check", "test", or "tests"
	// that appears as a top-level rule line (starts at column 0).
	preference := []string{"check", "test", "tests"}
	lines := strings.Split(string(data), "\n")
	found := map[string]bool{}
	for _, ln := range lines {
		if len(ln) == 0 || ln[0] == ' ' || ln[0] == '\t' || ln[0] == '#' {
			continue
		}
		for _, target := range preference {
			// "<target>:" at line start — Makefile target definition.
			if strings.HasPrefix(ln, target+":") || strings.HasPrefix(ln, target+" :") {
				found[target] = true
			}
		}
	}
	for _, target := range preference {
		if found[target] {
			return target
		}
	}
	return "check"
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

// synthesizeBuildFailureReport installs a single-TestResult
// ChangeReport describing a build/configure failure that short-
// circuited test execution. Shared by java / hvigor / cmake / meson
// branches when their expected XML artifact never materialised.
//
// The TestResult carries Kind=TestResultKindBuildError and structured
// BuildErrors[] parsed from stdout via parseBuildErrors (handles
// Maven javac, Kotlin, generic javac, TypeScript, Cangjie). When
// no patterns match, BuildErrors is nil but FailureDetail still
// holds a human-readable excerpt via narrativeBuildErrorExcerpt.
// ChangeReport.BuildFailed is set so evalTestsPass surfaces a clean
// "build failed before tests ran" message instead of "1 of 1 tests
// failed: <synthetic-id>".
//
// label is the human-readable toolchain name ("Java", "hvigor",
// "CMake", "Meson") for the user-facing failure summary. The
// returned ToolResult is what run_tests.Execute returns directly;
// caller does not call parseRunnerOutput when we short out here.
func synthesizeBuildFailureReport(
	ctx *types.BusContext,
	toolName, runner, label, output string,
) (types.ToolResult, error) {
	excerpt := narrativeBuildErrorExcerpt(output)
	buildErrs := parseBuildErrors(output)
	_, ref := StoreBlob(ctx, toolName+"-no-reports", output)
	failSummary := renderBuildFailureSummary(label, buildErrs,
		strings.SplitN(excerpt, "\n", 2)[0])
	report := &types.ChangeReport{
		TestResults: []types.TestResult{{
			Kind:          types.TestResultKindBuildError,
			AssertionID:   firstBuildErrorAssertionID(buildErrs),
			Suite:         "build",
			Passed:        false,
			FailureDetail: excerpt,
			BuildErrors:   buildErrs,
		}},
		Passed:         false,
		BuildFailed:    true,
		FailureSummary: failSummary,
		GeneratedAt:    time.Now(),
	}
	if plan := ctx.Mutable.ChangePlan(); plan != nil {
		report.PlanID = plan.ID
	}
	ctx.Mutable.SetChangeReport(report)
	return types.ToolResult{
		ToolName:  toolName,
		Success:   false,
		Summary:   fmt.Sprintf("[run_tests: runner=%s] %s", runner, failSummary),
		RawRef:    ref,
		Timestamp: time.Now(),
	}, nil
}
