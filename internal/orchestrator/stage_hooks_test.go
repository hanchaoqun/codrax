package orchestrator

import (
	"strings"
	"testing"
)

// TestSoftenSuspectListForPreservation_TypedFieldDrivesRewrite pins
// the keyword-gate audit HIGH-2 (2026-05-17) red-line fix: the rewrite
// from "suspect list — the regression is in the edits to these files"
// to "review for compatibility; the critic above identified what to
// preserve" fires ONLY when the typed
// ReflectorOutput.PreservationClauses slice is non-empty, never on
// LLM-prose substring match against the Observation. The two-branch
// table proves the gate is a precise typed signal, not a noisy
// keyword grep.
func TestSoftenSuspectListForPreservation_TypedFieldDrivesRewrite(t *testing.T) {
	const suspectFraming = "Files modified by the previous plan (suspect list — the regression is in the edits to these files):"
	const softenedFraming = "Files modified by the previous plan (review for compatibility; the critic above identified what to preserve):"

	cases := []struct {
		name        string
		heuristic   string
		hasClause   bool
		wantContain string
		wantAbsent  string
	}{
		{
			name:        "no preservation clause → heuristic unchanged (suspect framing kept)",
			heuristic:   "previous failure summary.\n\n" + suspectFraming + "\n- foo.go\n- bar.go\n",
			hasClause:   false,
			wantContain: suspectFraming,
			wantAbsent:  softenedFraming,
		},
		{
			name:        "preservation clause present → suspect framing softened",
			heuristic:   "previous failure summary.\n\n" + suspectFraming + "\n- foo.go\n- bar.go\n",
			hasClause:   true,
			wantContain: softenedFraming,
			wantAbsent:  suspectFraming,
		},
		{
			// Red-line guard: even when the critique prose itself
			// contains the literal substring "Preserve:" (e.g.
			// quoted from a code example, or in a critique that
			// argues against preservation), the gate MUST NOT fire
			// unless the typed slice is set. The pre-audit prose
			// grep would have fired here; the typed gate does not.
			name:        "prose substring 'Preserve:' alone never fires the gate (typed-field authoritative)",
			heuristic:   "Reviewer observation: do not Preserve: anything.\n\n" + suspectFraming + "\n- foo.go\n",
			hasClause:   false,
			wantContain: suspectFraming,
			wantAbsent:  softenedFraming,
		},
		{
			// Symmetric guard: when the critique is in a non-English
			// language (Chinese "保留"), the pre-audit prose grep
			// would have missed it. The typed gate fires correctly
			// because the LLM populated the typed slice regardless
			// of the prose language.
			name:        "non-English critique with typed clause → still fires (language-agnostic)",
			heuristic:   "Reviewer observation: 保留新增的 helper 函数.\n\n" + suspectFraming + "\n- foo.go\n",
			hasClause:   true,
			wantContain: softenedFraming,
			wantAbsent:  suspectFraming,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := softenSuspectListForPreservation(tc.heuristic, tc.hasClause)
			if !strings.Contains(got, tc.wantContain) {
				t.Errorf("output missing want-contain %q\nfull output:\n%s", tc.wantContain, got)
			}
			if strings.Contains(got, tc.wantAbsent) {
				t.Errorf("output unexpectedly contains want-absent %q\nfull output:\n%s", tc.wantAbsent, got)
			}
		})
	}
}

// TestSoftenSuspectListForPreservation_NoSuspectMarkerIsNoop pins the
// edge case where the heuristic carries no suspect-list framing at
// all (e.g. the previous plan had no TargetPaths). The helper must
// be a no-op rather than corrupt the heuristic text.
func TestSoftenSuspectListForPreservation_NoSuspectMarkerIsNoop(t *testing.T) {
	const heuristic = "previous failure summary only — no suspect-list section.\n"
	if got := softenSuspectListForPreservation(heuristic, true); got != heuristic {
		t.Errorf("heuristic without suspect-list framing must round-trip unchanged; got %q", got)
	}
	if got := softenSuspectListForPreservation(heuristic, false); got != heuristic {
		t.Errorf("heuristic without suspect-list framing must round-trip unchanged (no-clause path); got %q", got)
	}
}
