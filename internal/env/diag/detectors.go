package diag

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// runnerPrimaryBinary mirrors run_tests.go::runnerPrimaryBinary
// without importing the tool package (avoid layering reversal).
// 12 runners catalog; new runners need an entry here AND in the
// per-language Recommender.
var runnerPrimaryBinary = map[string]string{
	"go":     "go",
	"python": "pytest",
	"rust":   "cargo",
	"java":   "mvn",
	"ruby":   "bundle",
	"swift":  "swift",
	"cmake":  "ctest",
	"meson":  "meson",
	"make":   "make",
	"hvigor": "hvigorw",
	"cjpm":   "cjpm",
	"node":   "npm",
}

// detectRunnerMissing replicates run_tests.go::detectRunnerMissing's
// signal set in the dedicated package. New layered code reuses this
// instead of importing tool.
//
// Triggers on:
//   1. exit_code_127 (shell command-not-found)
//   2. shell pattern "<bin>: command not found" / "not found" /
//      "executable file not found" / Windows "is not recognized"
//   3. python-specific: "No module named pytest" (the python -m
//      pytest case where interpreter resolves but module absent)
func detectRunnerMissing(stderr, runner string, _ *types.EnvFacts) (types.Diagnosis, bool) {
	bin := runnerPrimaryBinary[runner]
	if bin == "" {
		return types.Diagnosis{}, false
	}
	lower := strings.ToLower(stderr)
	patterns := []string{
		bin + ": command not found",
		bin + ": not found",
		bin + " not found",
		"command not found: " + bin,
		"executable file not found",
		"is not recognized as an internal or external command",
		"no such file or directory: '" + bin + "'",
	}
	if runner == "python" {
		patterns = append(patterns,
			"python: command not found",
			"python: not found",
			"python3: command not found",
			"python3: not found",
			"no module named pytest",
			"no module named 'pytest'",
		)
	}
	for _, p := range patterns {
		if strings.Contains(lower, strings.ToLower(p)) {
			return types.Diagnosis{
				Kind:       types.DiagRunnerMissing,
				Runner:     runner,
				Tool:       bin,
				Source:     "pattern: " + p,
				Excerpt:    excerpt(stderr, 200),
				Confidence: 1.0,
			}, true
		}
	}
	return types.Diagnosis{}, false
}

// detectDepsMissing fires for known "deps not installed" shapes:
// node's "Cannot find module 'foo'", Ruby's "Could not find foo
// in any source", Python's import error for a third-party module
// (we distinguish from "No module named pytest" handled above —
// pytest is the runner itself, not a dep). Cargo / Go report
// these via their build pipelines, so usually surface as
// DiagBuildFailure not deps_missing.
func detectDepsMissing(stderr, runner string, _ *types.EnvFacts) (types.Diagnosis, bool) {
	lower := strings.ToLower(stderr)
	switch runner {
	case "node":
		if strings.Contains(lower, "cannot find module") {
			return types.Diagnosis{
				Kind:       types.DiagDepsMissing,
				Runner:     runner,
				Tool:       "node_modules",
				Source:     "pattern: cannot find module",
				Excerpt:    excerpt(stderr, 200),
				Confidence: 0.95,
			}, true
		}
	case "ruby":
		if strings.Contains(lower, "could not find") &&
			strings.Contains(lower, "in any source") {
			return types.Diagnosis{
				Kind:       types.DiagDepsMissing,
				Runner:     runner,
				Tool:       "Gemfile.deps",
				Source:     "pattern: could not find <gem> in any source",
				Excerpt:    excerpt(stderr, 200),
				Confidence: 0.95,
			}, true
		}
	case "python":
		// Python: "ModuleNotFoundError: No module named 'X'" where X
		// is NOT pytest (pytest case caught by detectRunnerMissing).
		if strings.Contains(lower, "modulenotfounderror") &&
			!strings.Contains(lower, "pytest") {
			return types.Diagnosis{
				Kind:       types.DiagDepsMissing,
				Runner:     runner,
				Tool:       "site-packages",
				Source:     "pattern: ModuleNotFoundError",
				Excerpt:    excerpt(stderr, 200),
				Confidence: 0.85,
			}, true
		}
	}
	return types.Diagnosis{}, false
}

// detectToolchainMissing fires when even the language's primary
// runtime (python3 / node / cargo / rustc / javac) is absent.
// Distinct from runner_missing: e.g. running pytest without
// python3 itself in PATH is toolchain_missing, not runner_missing.
func detectToolchainMissing(stderr, runner string, env *types.EnvFacts) (types.Diagnosis, bool) {
	if env == nil {
		return types.Diagnosis{}, false
	}
	switch runner {
	case "python":
		if !env.HasPython() {
			return types.Diagnosis{
				Kind: types.DiagBuildToolchainMissing, Runner: runner, Tool: "python3",
				Source: "env: no python found", Confidence: 1.0,
			}, true
		}
	case "node":
		if !env.HasNode() {
			return types.Diagnosis{
				Kind: types.DiagBuildToolchainMissing, Runner: runner, Tool: "node",
				Source: "env: no node found", Confidence: 1.0,
			}, true
		}
	case "rust":
		if len(env.Rusts) == 0 {
			return types.Diagnosis{
				Kind: types.DiagBuildToolchainMissing, Runner: runner, Tool: "cargo",
				Source: "env: no rust toolchain", Confidence: 1.0,
			}, true
		}
	case "java":
		if len(env.Javas) == 0 || env.Javas[0].JavacPath == "" {
			return types.Diagnosis{
				Kind: types.DiagBuildToolchainMissing, Runner: runner, Tool: "javac",
				Source: "env: no JDK", Confidence: 1.0,
			}, true
		}
	case "ruby":
		if len(env.Rubies) == 0 {
			return types.Diagnosis{
				Kind: types.DiagBuildToolchainMissing, Runner: runner, Tool: "ruby",
				Source: "env: no ruby", Confidence: 1.0,
			}, true
		}
	case "go":
		if !env.HasPkgManager("go") {
			return types.Diagnosis{
				Kind: types.DiagBuildToolchainMissing, Runner: runner, Tool: "go",
				Source: "env: no go", Confidence: 1.0,
			}, true
		}
	}
	return types.Diagnosis{}, false
}

// detectSystemLibMissing scans for the canonical "library not
// found" / "header not found" shapes from common toolchains. The
// pattern table here is conservative — long-tail libs route to
// LLM tab. Returns the lib NAME (not the package, which is
// distro-specific and resolved by Recommender).
func detectSystemLibMissing(stderr, runner string, _ *types.EnvFacts) (types.Diagnosis, bool) {
	lower := strings.ToLower(stderr)
	// Linker-style: ld: library not found for -l<name>
	if i := strings.Index(lower, "library not found for -l"); i >= 0 {
		name := extractToken(lower[i+len("library not found for -l"):])
		if name != "" {
			return types.Diagnosis{
				Kind:       types.DiagSystemLibMissing,
				Runner:     runner,
				Tool:       name,
				Source:     "ld: library not found for -l" + name,
				Excerpt:    excerpt(stderr, 200),
				Confidence: 0.95,
			}, true
		}
	}
	// Linker-style: cannot find -l<name>
	if i := strings.Index(lower, "cannot find -l"); i >= 0 {
		name := extractToken(lower[i+len("cannot find -l"):])
		if name != "" {
			return types.Diagnosis{
				Kind:    types.DiagSystemLibMissing, Runner: runner,
				Tool: name, Source: "ld: cannot find -l" + name,
				Excerpt: excerpt(stderr, 200), Confidence: 0.9,
			}, true
		}
	}
	// Compile-time: <header>.h: No such file or directory
	if strings.Contains(lower, ": no such file or directory") &&
		strings.Contains(lower, ".h:") {
		// Best-effort header extraction
		for _, line := range strings.Split(lower, "\n") {
			if !strings.Contains(line, ".h:") {
				continue
			}
			// "fatal error: openssl/ssl.h: no such file..."
			parts := strings.Split(line, ":")
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if strings.HasSuffix(p, ".h") {
					return types.Diagnosis{
						Kind:    types.DiagSystemLibMissing, Runner: runner,
						Tool: p, Source: "compile: header not found",
						Excerpt: excerpt(stderr, 200), Confidence: 0.85,
					}, true
				}
			}
		}
	}
	// Runtime: error while loading shared libraries: lib<name>.so
	if i := strings.Index(lower, "error while loading shared libraries:"); i >= 0 {
		tail := lower[i+len("error while loading shared libraries:"):]
		name := extractToken(strings.TrimSpace(tail))
		if name != "" {
			return types.Diagnosis{
				Kind:    types.DiagSystemLibMissing, Runner: runner,
				Tool: name, Source: "runtime: missing .so",
				Excerpt: excerpt(stderr, 200), Confidence: 0.9,
			}, true
		}
	}
	return types.Diagnosis{}, false
}

// detectConfigMissing fires when the runner is set but the
// expected manifest is absent. The probe layer captured every
// known manifest in EnvFacts.ProjectFiles.
func detectConfigMissing(_ string, runner string, env *types.EnvFacts) (types.Diagnosis, bool) {
	if env == nil {
		return types.Diagnosis{}, false
	}
	expected := map[string][]string{
		"python": {"pyproject.toml", "setup.py", "setup.cfg", "Pipfile", "requirements.txt"},
		"node":   {"package.json"},
		"rust":   {"Cargo.toml"},
		"go":     {"go.mod"},
		"java":   {"pom.xml", "build.gradle", "build.gradle.kts"},
		"ruby":   {"Gemfile"},
		"swift":  {"Package.swift"},
		"cmake":  {"CMakeLists.txt"},
		"meson":  {"meson.build"},
		"make":   {"Makefile", "GNUmakefile"},
		"hvigor": {"oh-package.json5", "hvigorfile.ts"},
		"cjpm":   {"cjpm.toml"},
	}
	files, ok := expected[runner]
	if !ok {
		return types.Diagnosis{}, false
	}
	for _, f := range files {
		if env.HasProjectFile(f) {
			return types.Diagnosis{}, false
		}
	}
	return types.Diagnosis{
		Kind: types.DiagConfigMissing, Runner: runner,
		Tool: files[0], Source: "no manifest in project",
		Confidence: 1.0,
	}, true
}

// detectGitState reads facts.GitRepoState (probed already).
// stderr is ignored — git state is determined by filesystem, not
// failure messages. Only fires when state != ready.
func detectGitState(_ string, _ string, env *types.EnvFacts) (types.Diagnosis, bool) {
	if env == nil {
		return types.Diagnosis{}, false
	}
	switch env.GitRepoState {
	case "git_missing":
		return types.Diagnosis{
			Kind: types.DiagGitNotInstalled, Tool: "git",
			Source: "env: git binary not on PATH", Confidence: 1.0,
		}, true
	case "not_initialized":
		return types.Diagnosis{
			Kind: types.DiagGitRepoNotInitialized, Tool: "git",
			Source: "env: .git/ absent", Confidence: 1.0,
		}, true
	case "no_commits":
		return types.Diagnosis{
			Kind: types.DiagGitRepoNoCommits, Tool: "git",
			Source: "env: HEAD has no commits", Confidence: 1.0,
		}, true
	}
	return types.Diagnosis{}, false
}

// extractToken reads the leading identifier from s — alphanum +
// dot/dash/underscore. Returns empty on no-match. Used for
// pulling lib names out of "library not found for -lssl".
func extractToken(s string) string {
	i := 0
	for i < len(s) {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			c == '.' || c == '-' || c == '_' || c == '+' {
			i++
			continue
		}
		break
	}
	return s[:i]
}
