package agent

// P0-E 复核收尾④ pins (2026-07-09): the 系统补充 bucket's in-bucket order for
// per-branch wakeup_chain path records — the ELECTED branch's record heads the
// bucket (typed WakeupPathBranch match), siblings follow in BRANCH-ordinal
// ascending order (engine segment order), never the lexical key order that
// sorted #10 before #2. Under the >40-row quota window the bucket floor keeps
// head rows, so the record the whole tree is rooted on can no longer fall
// into the omitted region.
//
// MUTATION self-check: dropping the Head/Branch comparator arms (reverting to
// the bare Key order) reds TestSupplementElectedBranchPathHeadsItsBucketP0E
// (#10 sorts before #2 again and the elected #2 loses its head seat).

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func branchP0ESupplementPathRecord(branch int, path string) types.ObservationRecord {
	return types.ObservationRecord{
		ID:              "trace_query:t#wakeup_chain:path:" + itoaAgentP0E(branch),
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            types.AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: types.ClaimGroundingHard,
		SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "seven.systrace"},
		ClaimKey:        "wakeup_chain:path#" + itoaAgentP0E(branch),
		Subject:         "target-9",
		Predicate:       "wakeup_chain",
		Object:          path,
		RichNotes: []string{
			"path=" + path,
			"target=target-9",
			"branch=" + itoaAgentP0E(branch),
			"branches=4",
		},
		Confidence: 0.82,
	}
}

func itoaAgentP0E(n int) string {
	digits := ""
	if n == 0 {
		return "0"
	}
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}

func TestSupplementElectedBranchPathHeadsItsBucketP0E(t *testing.T) {
	// Publication order 2,10,1,3: with no user entities the election takes
	// candidates[0] → branch 2 is the ELECTED branch (WakeupPathBranch=2).
	observations := []types.ObservationRecord{
		branchP0ESupplementPathRecord(2, "waker-two-2 -> target-9"),
		branchP0ESupplementPathRecord(10, "waker-ten-10 -> target-9"),
		branchP0ESupplementPathRecord(1, "waker-one-1 -> target-9"),
		branchP0ESupplementPathRecord(3, "waker-three-3 -> target-9"),
	}
	final := cmpbSupplementParse(t, observations)
	idx := func(sub string) int {
		i := strings.Index(final, sub)
		if i < 0 {
			t.Fatalf("supplement missing %q:\n%s", sub, final)
		}
		return i
	}
	elected := idx("waker-two-2 -> target-9")
	one := idx("waker-one-1 -> target-9")
	three := idx("waker-three-3 -> target-9")
	ten := idx("waker-ten-10 -> target-9")
	// Elected branch heads the bucket even though #1 sorts before #2 both
	// lexically and numerically.
	if !(elected < one) {
		t.Fatalf("elected branch #2 must head its bucket (before #1):\n%s", final)
	}
	// Siblings order by BRANCH ordinal: 1 < 3 < 10 — the lexical key order
	// (#10 < #3) must not survive.
	if !(one < three && three < ten) {
		t.Fatalf("sibling branch records must order by branch ordinal (1,3,10):\n%s", final)
	}
}
