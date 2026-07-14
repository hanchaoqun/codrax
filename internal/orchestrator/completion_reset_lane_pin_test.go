package orchestrator

// completion_reset_lane_pin_test.go — PIN-1 B3 (§29.65 回归口, 2026-07-13):
// structural enum pin for the §29.60/.1 completion-gate Reset narrowing.
//
// The §29.60.2 as-built keeps accepted completion TERMINAL except for exactly
// THREE typed lanes that may clear it on the next explore-window dispatch
// (via the pendingCompletionReset latch):
//
//	1. template-SC requeues  — post-completion reachable only under strict
//	   policy (two set sites: the non-validate requeue and the validate
//	   fingerprint-miss requeue);
//	2. the fatal-class zero-witness retry (§29.60: no successful
//	   read/search/evidence result — the carried completion is invalid
//	   regardless of who is right);
//	3. FallbackBackToExplore contract backtracks (the answer slate was
//	   cleared; fresh evidence will reshape the closure).
//
// Quality-class floor detections DISCLOSE and never set the latch. Before
// this pin the closed set lived only in comments + scenario tests: a new
// soft-lane `pendingCompletionReset = true` (a 5th set site) did not
// necessarily bite red. This test enumerates the set sites structurally and
// requires each to sit in a documented lane; adding a set site now reddens
// until the lane list here AND the declaration comment are extended
// deliberately (§29.60 ruling review required — 系统的判定要尊重模型).
//
// It also pins the single consume throat: ResetInvestigationComplete() has
// exactly ONE production call site in the whole repo, guarded by the latch —
// a direct reset call anywhere else would bypass the lane discipline.

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// completionResetLaneMarkers is the closed set: each set site's preceding
// comment block must name its lane with one of these markers (the same
// wording the pendingCompletionReset declaration comment declares).
var completionResetLaneMarkers = []string{
	"strict policy",      // template-SC requeue lanes (both sites)
	"zero-witness retry", // fatal-class structurally-empty retry (§29.60)
	"Contract backtrack", // FallbackBackToExplore
}

func TestCompletionResetSetSitesStayInTypedLanes(t *testing.T) {
	src, err := os.ReadFile("orchestrator.go")
	if err != nil {
		t.Fatalf("read orchestrator.go: %v", err)
	}
	lines := strings.Split(string(src), "\n")

	// The declaration comment is the single documented source of the lane
	// list — hold its wording so the closed set cannot silently regrow there.
	declaration := string(src)
	for _, want := range []string{
		"pendingCompletionReset (§29.60)",
		"Set only by the",
		"zero-witness retry",
		"FallbackBackToExplore contract backtracks",
		"template-SC requeues",
		"quality-class floor detections disclose and never set it",
	} {
		if !strings.Contains(declaration, want) {
			t.Fatalf("pendingCompletionReset declaration comment lost its lane list (%q missing)", want)
		}
	}

	var sites []int
	for i, line := range lines {
		if strings.TrimSpace(line) == "pendingCompletionReset = true" {
			sites = append(sites, i)
		}
	}
	// Exactly two template-SC sites + one zero-witness site + one contract
	// backtrack site. A new set site is a NEW completion-reset lane: extend
	// completionResetLaneMarkers, the declaration comment and this count in
	// the same commit, with a §29.60 lane justification (typed carrier,
	// fatal-class argument) — never silently.
	if len(sites) != 4 {
		t.Fatalf("pendingCompletionReset set-site count drifted: want the 4 adjudicated lane sites, got %d at lines %v",
			len(sites), sitesToHuman(sites))
	}
	seen := map[string]bool{}
	for _, site := range sites {
		marker := ""
		for back := site - 1; back >= 0 && back >= site-8; back-- {
			text := strings.TrimSpace(lines[back])
			if !strings.HasPrefix(text, "//") {
				break
			}
			for _, m := range completionResetLaneMarkers {
				if strings.Contains(text, m) {
					marker = m
				}
			}
		}
		if marker == "" {
			t.Fatalf("pendingCompletionReset set site at orchestrator.go:%d carries no lane marker comment (closed set: %v)",
				site+1, completionResetLaneMarkers)
		}
		seen[marker] = true
	}
	for _, m := range completionResetLaneMarkers {
		if !seen[m] {
			t.Fatalf("adjudicated completion-reset lane %q lost its set site", m)
		}
	}

	// No other production file in the package may grow its own latch writes.
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || name == "orchestrator.go" || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		body, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "pendingCompletionReset = true") {
			t.Fatalf("%s: pendingCompletionReset set site outside orchestrator.go — the lane pin must move with it", name)
		}
	}
}

func TestResetInvestigationCompleteSingleGuardedThroat(t *testing.T) {
	type callSite struct {
		file string
		line int
	}
	var calls []callSite
	for _, root := range []string{"../..", "../../cmd"} {
		root := filepath.Clean(root)
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				base := d.Name()
				// Hidden dirs cover .git/.codrax/.claude (worktree copies of
				// the repo would double-count every call site).
				if strings.HasPrefix(base, ".") && path != root {
					return filepath.SkipDir
				}
				if base == "eval" || base == "docs" || base == "examples" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for i, line := range strings.Split(string(body), "\n") {
				if strings.Contains(line, ".ResetInvestigationComplete()") {
					calls = append(calls, callSite{file: path, line: i + 1})
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		break // "../.." already covers cmd; the loop shape documents intent
	}
	if len(calls) != 1 || !strings.HasSuffix(calls[0].file, "internal/orchestrator/orchestrator.go") {
		t.Fatalf("ResetInvestigationComplete must keep exactly ONE production call site (the latch-guarded throat in orchestrator.go), got %v", calls)
	}
	src, err := os.ReadFile("orchestrator.go")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(src), "\n")
	guarded := false
	for i, line := range lines {
		if !strings.Contains(line, ".ResetInvestigationComplete()") {
			continue
		}
		for back := i - 1; back >= 0 && back >= i-3; back-- {
			if strings.Contains(lines[back], "if pendingCompletionReset") {
				guarded = true
			}
		}
	}
	if !guarded {
		t.Fatalf("the ResetInvestigationComplete throat must stay guarded by the pendingCompletionReset latch")
	}
}

func sitesToHuman(sites []int) []int {
	out := make([]int, len(sites))
	for i, s := range sites {
		out[i] = s + 1
	}
	return out
}
