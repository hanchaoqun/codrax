package agent

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// HMC377 E1E6-R2: the analysed thread's own semantic family whose value is
// folded from more members than its roster lists (Yuanbao bindApplication:
// class verification, 109 members, roster of 32, no per-member lines) must not
// lose its member clue silently in the finalizer handoff.
func TestHMC377HandoffBoundedRosterKeepsMembersAndSaysWhatIsOmitted(t *testing.T) {
	var roster []string
	for i := 0; i < 32; i++ {
		roster = append(roster, fmt.Sprintf("VerifyClass com.example.C%d %.3fms", i, 3.0-float64(i)*0.05))
	}
	node := types.TraceCausalProjectionNode{FamilyMemberCount: 109, FamilyMemberRoster: roster}
	var b strings.Builder
	traceDecisionWriteSemanticMembers(&b, node, 8)
	got := b.String()
	if !strings.Contains(got, "member_1 span_with_duration=`VerifyClass com.example.C0 3.000ms`") ||
		!strings.Contains(got, "member_8 span_with_duration=") || strings.Contains(got, "member_9 ") {
		t.Fatalf("the largest listed members must be handed over (bounded by the limit):\n%s", got)
	}
	if !strings.Contains(got, "members_omitted=101; members_listed_largest_first=8") {
		t.Fatalf("the omission must be stated as a typed count:\n%s", got)
	}
	// A complete roster keeps the per-member line form.
	full := types.TraceCausalProjectionNode{FamilyMemberCount: 2, FamilyMemberRoster: []string{"VerifyClass A 3.023ms", "VerifyClass B 1.664ms"},
		FamilyMemberLineRanges: [][2]int{{86771, 87344}, {62163, 62628}}, FamilyMemberWallMS: []float64{3.023, 1.664}}
	b.Reset()
	traceDecisionWriteSemanticMembers(&b, full, 8)
	if !strings.Contains(b.String(), "member_1 span=`VerifyClass A 3.023ms`; duration=3.023ms; lines=86771..87344") {
		t.Fatalf("complete roster form changed:\n%s", b.String())
	}
}
