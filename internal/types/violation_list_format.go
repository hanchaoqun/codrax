package types

import (
	"fmt"
	"strings"
)

// MaxListedViolations caps every accumulated reject listing (EMITBURN-1
// §29.173 aggregate_facts, V2-4 §40.51 patch structure and the sibling emit
// validators): a pathological payload lists the first ten violations plus a
// count instead of flooding the retry prompt.
const MaxListedViolations = 10

// NumberedViolationList is the ONE formatter behind every "fix ALL of them in
// this one re-emit" listing: `[1] …; [2] …` capped at MaxListedViolations with
// a trailing `; ... and N more violation(s)`. Hoisted from the
// emit_investigation_complete aggregate-facts renderer (V2-4, §40.51) so the
// patch-structure error, the block normalizers and the aggregate walker never
// diverge in shape.
func NumberedViolationList(violations []string) string {
	shown := violations
	more := 0
	if len(shown) > MaxListedViolations {
		more = len(shown) - MaxListedViolations
		shown = shown[:MaxListedViolations]
	}
	var b strings.Builder
	for i, violation := range shown {
		if i > 0 {
			b.WriteString("; ")
		}
		fmt.Fprintf(&b, "[%d] %s", i+1, violation)
	}
	if more > 0 {
		fmt.Fprintf(&b, "; ... and %d more violation(s)", more)
	}
	return b.String()
}

// ViolationListMessage renders an accumulated validator result as ONE reject
// message: a single violation is returned verbatim (byte-identical to the
// historical serial gate), while two or more become `<count sentence>: [1] …;
// [2] …`. countSentence receives the violation count and must end with the
// separator it wants before the list (":" / " — ").
func ViolationListMessage(violations []string, countSentence func(n int) string) string {
	switch len(violations) {
	case 0:
		return ""
	case 1:
		return violations[0]
	}
	return countSentence(len(violations)) + NumberedViolationList(violations)
}
