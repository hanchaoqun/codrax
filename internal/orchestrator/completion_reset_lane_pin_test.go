package orchestrator

// completion_reset_lane_pin_test.go — PIN-1 B3 (§29.65 回归口, 2026-07-13):
// structural enum pin for the §29.60/.1 completion-gate Reset narrowing.
//
// The §29.60.2 as-built keeps accepted completion TERMINAL except for exactly
// FOUR typed lanes that may clear it on the next scheduler iteration
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
//	4. a window-scoped completion while required multi-topic evidence nodes
//	   remain (the current node closed; sibling topic lanes did not).
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
	"fmt"
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
	"windowScoped",       // required multi-topic sibling evidence remains
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
		// §40.55 V7-5: the 4th lane (4c7a0d0a3) must stay named on the
		// declaration alongside the original three.
		"window-scoped completion",
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
	// backtrack site + one multi-topic window-scope site. A new set site is a NEW completion-reset lane: extend
	// completionResetLaneMarkers, the declaration comment and this count in
	// the same commit, with a §29.60 lane justification (typed carrier,
	// fatal-class argument) — never silently.
	if len(sites) != 5 {
		t.Fatalf("pendingCompletionReset set-site count drifted: want the 5 adjudicated lane sites, got %d at lines %v",
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
	if shape, detail := classifyResetThroat(strings.Split(string(src), "\n")); shape != resetThroatOK {
		t.Fatalf("orchestrator.go: %s", resetThroatFailureMessage(shape, detail))
	}
}

// resetThroatShape is the closed set of outcomes classifyResetThroat can
// report about the single ResetInvestigationComplete() consume throat. Every
// reader (the pin above and the self-red table below) classifies through
// resetThroatFailureMessage, which is total over this set.
type resetThroatShape int

const (
	resetThroatOK             resetThroatShape = iota
	resetThroatNoCall                          // no ResetInvestigationComplete() line in the source
	resetThroatUnguarded                       // the call is not inside the `if pendingCompletionReset` guard
	resetThroatNoComment                       // nothing but code sits directly above the guard line
	resetThroatCommentOffLane                  // a comment group sits on the guard but never says §29.60 … terminal
)

// resetThroatLaneSubstrings are the two exact substrings one line of the
// throat's comment group must carry (precise signal, not a count).
var resetThroatLaneSubstrings = [2]string{"§29.60", "terminal"}

// classifyResetThroat locates the `.ResetInvestigationComplete()` call, the
// `if pendingCompletionReset` guard it sits under (the call must be the
// guard's first statement: guard line == call line − 1), and the contiguous
// `//` comment group whose last line is directly above the guard — the
// group's extent is structural (it ends at the first non-comment line), so
// growing the comment never trips the pin while a blank or code line between
// the group and the guard, or a comment placed earlier in the loop, does.
//
// §40.55 V7-5 history: 4c7a0d0a3 deleted the throat's §29.60 "terminal"
// comment to stay under the LOC ratchet and the previous pin stayed green
// (ratchet paid with comments). The first replacement pin used a fixed
// 6-line window sized exactly to the restored 5-line comment, so growing
// the comment by one line tripped it with a "lost its comment" message
// (§40.55 合流复核, G6-ratchet #1) — hence the structural bounds here.
//
// detail is the human-readable location for the failure message.
func classifyResetThroat(lines []string) (resetThroatShape, string) {
	call := -1
	for i, line := range lines {
		if strings.Contains(line, ".ResetInvestigationComplete()") {
			call = i
			break
		}
	}
	if call < 0 {
		return resetThroatNoCall, "no .ResetInvestigationComplete() call"
	}
	guard := call - 1
	if guard < 0 || !strings.Contains(lines[guard], "if pendingCompletionReset") {
		return resetThroatUnguarded, fmt.Sprintf("call at line %d", call+1)
	}
	group := guard - 1
	for group >= 0 && strings.HasPrefix(strings.TrimSpace(lines[group]), "//") {
		group--
	}
	first, last := group+1, guard-1
	if first > last {
		return resetThroatNoComment, fmt.Sprintf("guard at line %d", guard+1)
	}
	for i := first; i <= last; i++ {
		text := strings.TrimSpace(lines[i])
		if strings.Contains(text, resetThroatLaneSubstrings[0]) && strings.Contains(text, resetThroatLaneSubstrings[1]) {
			return resetThroatOK, ""
		}
	}
	return resetThroatCommentOffLane, fmt.Sprintf("comment group at lines %d-%d", first+1, last+1)
}

// resetThroatFailureMessage renders every non-OK shape; an unrecognized
// shape is a bug in the classifier and fails loud (§40.50) instead of being
// read as a pass.
func resetThroatFailureMessage(shape resetThroatShape, detail string) string {
	const notCompliance = "restoring the LOC ratchet by compressing or deleting this comment is not compliance — extract a concern file instead"
	switch shape {
	case resetThroatOK:
		return ""
	case resetThroatNoCall:
		return "ResetInvestigationComplete must keep its single latch-guarded throat (" + detail + ")"
	case resetThroatUnguarded:
		return "the ResetInvestigationComplete throat must be the first statement under the `if pendingCompletionReset` guard (" + detail + ")"
	case resetThroatNoComment:
		return "the ResetInvestigationComplete throat has no comment group on it — the §29.60 lane comment must sit directly above the guard, contiguous with it (" + detail + "); " + notCompliance
	case resetThroatCommentOffLane:
		return fmt.Sprintf("the comment group on the ResetInvestigationComplete throat does not name the lane — one of its lines must contain both %q and %q (%s); %s", resetThroatLaneSubstrings[0], resetThroatLaneSubstrings[1], detail, notCompliance)
	}
	panic(fmt.Sprintf("classifyResetThroat produced an unrecognized shape %d — extend resetThroatFailureMessage", shape))
}

// TestClassifyResetThroatShapes is the pin's self-red: one synthetic source
// per shape the classifier can report, so the pin is known to bite on each
// and, just as important, known NOT to bite when the comment grows.
func TestClassifyResetThroatShapes(t *testing.T) {
	t.Parallel()
	const call = "\t\t\to.busCtx.Mutable.ResetInvestigationComplete()"
	const guard = "\t\tif pendingCompletionReset {"
	const lane = "\t\t// §29.60: accepted completion is terminal — clear it only when a"
	const laneRest = "\t\t// typed lane latched a reset."
	const unrelated = "\t\t// Phase 2 cancel checkpoint at the top of every iteration."
	code := "\t\tif cerr := o.checkCanceled(); cerr != nil {\n\t\t\treturn\n\t\t}"
	cases := []struct {
		name  string
		lines []string
		want  resetThroatShape
	}{
		{"green_current_shape", []string{code, lane, laneRest, guard, call, "\t\t}"}, resetThroatOK},
		// The anti-compression direction must stay green: extra lines
		// appended below the lane line, or prepended above it, are still
		// one contiguous group on the guard.
		{"green_comment_grown_below", []string{code, lane, laneRest, "\t\t// (a fifth lane would be listed on the declaration)", guard, call}, resetThroatOK},
		{"green_comment_grown_above", []string{code, "\t\t// Consumed before buildEnv.", lane, laneRest, guard, call}, resetThroatOK},
		{"green_lane_line_only", []string{lane, guard, call}, resetThroatOK},
		{"red_no_call", []string{code, lane, guard, "\t\t\tpendingCompletionReset = false"}, resetThroatNoCall},
		{"red_call_not_first_statement", []string{lane, guard, "\t\t\tlogging.Info(\"reset\")", call}, resetThroatUnguarded},
		{"red_unguarded", []string{lane, call}, resetThroatUnguarded},
		{"red_comment_deleted", []string{code, guard, call}, resetThroatNoComment},
		{"red_blank_line_between_comment_and_guard", []string{lane, laneRest, "", guard, call}, resetThroatNoComment},
		{"red_comment_earlier_in_loop", []string{lane, laneRest, code, guard, call}, resetThroatNoComment},
		{"red_only_unrelated_comment", []string{unrelated, guard, call}, resetThroatCommentOffLane},
		{"red_lane_words_split_across_lines", []string{"\t\t// §29.60 lane", "\t\t// accepted completion is terminal", guard, call}, resetThroatCommentOffLane},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, detail := classifyResetThroat(tc.lines)
			if got != tc.want {
				t.Fatalf("shape = %d (%s), want %d", got, detail, tc.want)
			}
			msg := resetThroatFailureMessage(got, detail)
			if (msg == "") != (got == resetThroatOK) {
				t.Fatalf("message/shape disagree: shape %d, message %q", got, msg)
			}
		})
	}
	// The failure renderer is total over the closed set; an out-of-set
	// value must not be read as silence.
	defer func() {
		if recover() == nil {
			t.Fatalf("resetThroatFailureMessage accepted an unrecognized shape silently")
		}
	}()
	_ = resetThroatFailureMessage(resetThroatShape(99), "")
}

func sitesToHuman(sites []int) []int {
	out := make([]int, len(sites))
	for i, s := range sites {
		out[i] = s + 1
	}
	return out
}
