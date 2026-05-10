// Package tool — analysis_limits_default_test.go (2026-05-10).
//
// Pin the prescan budget default. Bumped 2 → 3 after 65% of recent
// eval cases hit "Pre-scan budget reached" — see
// docs/design/forced_read_remediation.md §4.1.
package tool

import "testing"

func TestDefaultAnalysisLimits_MaxPrescanRoundsDefault(t *testing.T) {
	got := DefaultAnalysisLimits().MaxPrescanRounds
	if got != 3 {
		t.Errorf("DefaultAnalysisLimits().MaxPrescanRounds = %d, want 3 (post-2026-05-10 budget bump)", got)
	}
}

func TestDefaultAnalysisLimits_OtherFloorsUnchanged(t *testing.T) {
	// Tripwire: if anybody bumps WarnBelowKeywords or other defaults
	// without updating the design doc, this test guards. These are
	// the floors a budget bump shouldn't accidentally touch.
	defaults := DefaultAnalysisLimits()
	cases := []struct {
		name string
		got  int
		want int
	}{
		{"WarnBelowKeywords", defaults.WarnBelowKeywords, 8},
		{"RejectBelowKeywords", defaults.RejectBelowKeywords, 0},
		{"GhostAnchorExpandSearchThreshold", defaults.GhostAnchorExpandSearchThreshold, 3},
		{"Phase1UnreadTopK", defaults.Phase1UnreadTopK, 5},
		{"Phase1UnreadMinUnread", defaults.Phase1UnreadMinUnread, 2},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s = %d, want %d (unrelated to prescan budget bump)", tc.name, tc.got, tc.want)
		}
	}
}
