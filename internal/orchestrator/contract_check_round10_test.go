package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// countNonPlumbingViolations mirrors the round-10 trigger threshold
// counting in runAnswerReviewerOnSuccess. Extracted for testability;
// the production loop inlines the same logic.
func countNonPlumbingViolations(viols []types.Violation) int {
	n := 0
	for _, v := range viols {
		if isPlumbingFailureViolation(v.Kind) {
			continue
		}
		n++
	}
	return n
}

// TestCountNonPlumbingViolations_ExcludesAuthorityOverreach: round-10
// fix — closureViolationCount must NOT include ViolAuthorityOverreach.
// Pre-fix a Run with 2 plumbing-only violations crossed the
// softViolationLearnFloor (2) and triggered a wasted reviewer dispatch.
func TestCountNonPlumbingViolations_ExcludesAuthorityOverreach(t *testing.T) {
	cases := []struct {
		name string
		in   []types.Violation
		want int
	}{
		{
			name: "all-plumbing-zero-count",
			in: []types.Violation{
				{Kind: types.ViolAuthorityOverreach},
				{Kind: types.ViolAuthorityOverreach},
			},
			want: 0,
		},
		{
			name: "mixed-counts-only-content",
			in: []types.Violation{
				{Kind: types.ViolFamilyMismatch},
				{Kind: types.ViolAuthorityOverreach},
				{Kind: types.ViolMustInclude},
			},
			want: 2,
		},
		{
			name: "all-content",
			in: []types.Violation{
				{Kind: types.ViolFamilyMismatch},
				{Kind: types.ViolCitation},
			},
			want: 2,
		},
		{
			name: "empty",
			in:   nil,
			want: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := countNonPlumbingViolations(tc.in)
			if got != tc.want {
				t.Errorf("got %d; want %d", got, tc.want)
			}
		})
	}
}

// TestCountNonPlumbingViolations_TriggerThresholdSemantic: a Run
// with all-plumbing violations must NOT cross the
// softViolationLearnFloor (2). Plumbing-only Runs should not
// dispatch the answer reviewer.
func TestCountNonPlumbingViolations_TriggerThresholdSemantic(t *testing.T) {
	const softViolationLearnFloor = 2
	allPlumbing := []types.Violation{
		{Kind: types.ViolAuthorityOverreach},
		{Kind: types.ViolAuthorityOverreach},
		{Kind: types.ViolAuthorityOverreach},
	}
	if got := countNonPlumbingViolations(allPlumbing); got >= softViolationLearnFloor {
		t.Errorf("plumbing-only count = %d; should be below floor %d to skip dispatch",
			got, softViolationLearnFloor)
	}

	// Conversely: 2 content violations DO cross the floor.
	contentOnly := []types.Violation{
		{Kind: types.ViolFamilyMismatch},
		{Kind: types.ViolMustInclude},
	}
	if got := countNonPlumbingViolations(contentOnly); got < softViolationLearnFloor {
		t.Errorf("content-only count = %d; should reach floor %d", got, softViolationLearnFloor)
	}
}
