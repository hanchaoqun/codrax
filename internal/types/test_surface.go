package types

import (
	"sort"
	"strings"
	"time"
)

// TestSurfaceCandidate is one runnable verification entry detected from typed
// repository signals: manifest files, test-file naming conventions, declared
// Makefile targets, configured native build directories, and python test
// infrastructure. It records WHAT could run, from WHERE, and whether typed
// detection saw actual test work behind it. No field is derived from user
// prose or model output.
type TestSurfaceCandidate struct {
	// ID is the stable label "<runner>[/<framework>]@<rel-dir>".
	ID string `json:"id"`

	// Runner is one of the run_tests whitelist runner identifiers.
	Runner string `json:"runner"`

	// Framework refines runner=python (pytest | unittest); empty otherwise.
	Framework string `json:"framework,omitempty"`

	// WorkingDir is the candidate root relative to the verification repo
	// root; "." means the root itself.
	WorkingDir string `json:"working_dir"`

	// Command is the canonical command template this candidate would run,
	// rendered for display and audit. Executors re-render at run time.
	Command string `json:"command,omitempty"`

	// Source is the repo-relative manifest path that justified this
	// candidate (e.g. "Makefile", "backend/pom.xml").
	Source string `json:"source,omitempty"`

	// MakeTarget is the detected Makefile test target for runner=make.
	MakeTarget string `json:"make_target,omitempty"`

	// HasTestSignal is the typed "this candidate has actual test work"
	// bit. Detection per runner family:
	//   - convention-scanning runners (go/python/node/ruby/swift/java):
	//     test-file conventions or python test infrastructure;
	//   - make: a declared check/test/tests target in the Makefile;
	//   - cmake/meson: a configured build directory;
	//   - runners without a convention scanner (rust/hvigor/cjpm): true,
	//     matching the executor's behaviour of always invoking them.
	HasTestSignal bool `json:"has_test_signal"`

	// Priority is the manifest priority from the detection table (lower
	// ranks earlier). Only a tiebreaker below HasTestSignal.
	Priority int `json:"priority"`
}

// TestSurface is the typed inventory of runnable verification candidates for
// one repository root. Candidates are ordered by the selection rule "runnable
// test work dominates manifest priority": HasTestSignal first, then manifest
// priority, then path depth, then path, then runner. SelectedID names the
// top-ranked candidate. The surface is computed deterministically from
// filesystem signals only and is safe input for hard gates.
type TestSurface struct {
	Candidates  []TestSurfaceCandidate `json:"candidates,omitempty"`
	SelectedID  string                 `json:"selected_id,omitempty"`
	GeneratedAt time.Time              `json:"generated_at"`
}

// ExecutedCommand records one verification command execution (or a typed
// non-execution outcome such as a synthetic no-tests pass) with provenance.
// Stored on ChangeReport so every attempt keeps durable command evidence for
// audit and replan handoff.
type ExecutedCommand struct {
	Runner     string `json:"runner"`
	Framework  string `json:"framework,omitempty"`
	WorkingDir string `json:"working_dir,omitempty"`
	Command    string `json:"command,omitempty"`
	ExitCode   int    `json:"exit_code"`
	DurationMS int64  `json:"duration_ms,omitempty"`

	// Source is the typed provenance of the execution decision:
	// llm_choice | auto_detect | no_tests_escalation |
	// runner_missing_escalation.
	Source string `json:"source,omitempty"`

	// Outcome is the typed coarse result of this entry:
	// executed | synthetic_no_tests | syntax_check_fallback |
	// runner_missing | timeout | oom | cpu_limit | parser_error |
	// not_configured.
	Outcome string `json:"outcome,omitempty"`
}

// SortTestSurfaceCandidates orders candidates in place by the typed selection
// rule: HasTestSignal desc, manifest priority asc, path depth asc, working
// dir asc, runner asc. Deterministic and stable.
func SortTestSurfaceCandidates(cands []TestSurfaceCandidate) {
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].HasTestSignal != cands[j].HasTestSignal {
			return cands[i].HasTestSignal
		}
		if cands[i].Priority != cands[j].Priority {
			return cands[i].Priority < cands[j].Priority
		}
		di, dj := testSurfacePathDepth(cands[i].WorkingDir), testSurfacePathDepth(cands[j].WorkingDir)
		if di != dj {
			return di < dj
		}
		if cands[i].WorkingDir != cands[j].WorkingDir {
			return cands[i].WorkingDir < cands[j].WorkingDir
		}
		return cands[i].Runner < cands[j].Runner
	})
}

// NormalizeTestSurface sorts candidates by the selection rule, stamps
// GeneratedAt when zero, and aligns SelectedID with the top-ranked candidate
// when unset or stale.
func NormalizeTestSurface(surface TestSurface) TestSurface {
	SortTestSurfaceCandidates(surface.Candidates)
	if surface.GeneratedAt.IsZero() {
		surface.GeneratedAt = time.Now()
	}
	if len(surface.Candidates) == 0 {
		surface.SelectedID = ""
		return surface
	}
	if surface.SelectedID != "" {
		for _, c := range surface.Candidates {
			if c.ID == surface.SelectedID {
				return surface
			}
		}
	}
	surface.SelectedID = surface.Candidates[0].ID
	return surface
}

func testSurfacePathDepth(rel string) int {
	rel = strings.TrimSpace(rel)
	if rel == "" || rel == "." {
		return -1
	}
	return strings.Count(rel, "/")
}
