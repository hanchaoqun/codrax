package tool

import (
	"sort"
	"strings"
)

// run_tests_side_effect_manifest.go — V5-2 (colleague_merge_audit §40.11):
// the closed per-runner side-effect manifest. Each runner declares (a) the
// dependency lockfiles its toolchain owns and may rewrite, together with the
// locked-mode arguments/environment that make a re-run refuse further
// rewrites (the fixed-point witness), and (b) the canonical stdin→stdout
// formatter of its language family used for the formatter reversibility
// check. Runners without a lane keep every tracked drift unclassified. The
// table is keyed exactly by allowedRunners (census-pinned) and is consulted
// only through the typed executed-runner roster — a path name alone never
// selects a runner.

type runnerSideEffectManifest struct {
	Runner string
	// LockfileBasenames are exact basenames (no globs) the runner's toolchain
	// owns. A drifted tracked path whose basename is listed AND whose
	// directory is the executed working dir or one of its ancestors inside
	// the repo (workspace-root rule) is a dependency_lockfile_refresh
	// candidate.
	LockfileBasenames []string
	// LockedArgs are inserted right after the runner's test subcommand so the
	// re-run refuses to rewrite the lockfile.
	LockedArgs []string
	// LockedEnv are KEY=VALUE pairs appended to the re-run environment.
	LockedEnv []string
	// Formatter is the canonical stdin→stdout formatter argv of the language
	// family (formatter_no_semantic_diff); nil = no formatter lane.
	Formatter []string
	// FormatterExts are the exact file extensions the formatter owns.
	FormatterExts []string
}

// hasLockedLane reports whether the manifest can prove a lockfile fixed point.
func (m runnerSideEffectManifest) hasLockedLane() bool {
	return len(m.LockedArgs) > 0 || len(m.LockedEnv) > 0
}

func runnerSideEffectManifests() map[string]runnerSideEffectManifest {
	return map[string]runnerSideEffectManifest{
		"rust": {Runner: "rust", LockfileBasenames: []string{"Cargo.lock"}, LockedArgs: []string{"--locked"},
			Formatter: []string{"rustfmt", "--emit", "stdout"}, FormatterExts: []string{".rs"}},
		// go.mod is the dependency MANIFEST (the Cargo.toml analogue): `go test`
		// never rewrites it and -mod=readonly cannot witness a semantics change
		// in it, so only go.sum is a toolchain-owned lockfile.
		"go": {Runner: "go", LockfileBasenames: []string{"go.sum"}, LockedArgs: []string{"-mod=readonly"},
			Formatter: []string{"gofmt"}, FormatterExts: []string{".go"}},
		"swift":  {Runner: "swift", LockfileBasenames: []string{"Package.resolved"}, LockedArgs: []string{"--force-resolved-versions"}},
		"ruby":   {Runner: "ruby", LockfileBasenames: []string{"Gemfile.lock"}, LockedEnv: []string{"BUNDLE_FROZEN=true"}},
		"python": {Runner: "python", Formatter: []string{"black", "-q", "-"}, FormatterExts: []string{".py"}},
		"node":   {Runner: "node"},
		"java":   {Runner: "java"},
		"cmake":  {Runner: "cmake"},
		"meson":  {Runner: "meson"},
		"make":   {Runner: "make"},
		"hvigor": {Runner: "hvigor"},
		"cjpm":   {Runner: "cjpm"},
	}
}

// runnerTestSubcommandPrefix is the exact token pair after which LockedArgs
// are inserted. Runners whose base command does not start with it have no
// locked form (ok=false).
var runnerTestSubcommandPrefix = map[string]string{
	"rust":  "cargo test",
	"go":    "go test",
	"swift": "swift test",
}

// buildLockedRunCommand renders the runner's ordinary command in locked mode:
// LockedArgs right after the test subcommand, LockedEnv returned separately.
// ok=false when the runner has no locked lane or the base command does not
// carry the expected subcommand (framework variants).
func buildLockedRunCommand(runner, framework, suite, repoRoot, mainRoot string) (cmd string, env []string, ok bool) {
	manifest, known := runnerSideEffectManifests()[runner]
	if !known || !manifest.hasLockedLane() {
		return "", nil, false
	}
	base, _ := buildRunCommandWithFramework(runner, framework, suite, repoRoot, mainRoot)
	base = strings.TrimSpace(base)
	if base == "" {
		return "", nil, false
	}
	if len(manifest.LockedArgs) == 0 {
		return base, append([]string(nil), manifest.LockedEnv...), true
	}
	prefix := runnerTestSubcommandPrefix[runner]
	if prefix == "" || !(base == prefix || strings.HasPrefix(base, prefix+" ")) {
		return "", nil, false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(base, prefix))
	cmd = prefix + " " + strings.Join(manifest.LockedArgs, " ")
	if rest != "" {
		cmd += " " + rest
	}
	return cmd, append([]string(nil), manifest.LockedEnv...), true
}

// runnerSideEffectLockfileOwners lists (runner, basename) pairs for the
// census: a basename may belong to at most one runner.
func runnerSideEffectLockfileOwners() map[string][]string {
	out := map[string][]string{}
	for runner, manifest := range runnerSideEffectManifests() {
		for _, base := range manifest.LockfileBasenames {
			out[base] = append(out[base], runner)
		}
	}
	for base := range out {
		sort.Strings(out[base])
	}
	return out
}
