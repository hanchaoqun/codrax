package tool

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool/ground"
	"github.com/hanchaoqun/codrax/internal/types"
)

// normalizeArgumentFlowEvidenceReceiver preserves the source-owned receiving
// expression that grounding already proved. Accepting a short receiver name
// must not create a second identity for the same invocation and exact argument.
// This is not a suffix-based merger: the existing parser must locate exactly
// one invocation and one complete argument on the final, already-read line.
// No source lookup, grounding promotion, model predicate or argument rewrite is
// permitted here. Call after coordinate recovery and before identity bindings.
func normalizeArgumentFlowEvidenceReceiver(it *types.EvidenceItem, gc *ground.Context) bool {
	if it == nil || gc == nil || !it.IsCitable() || it.GroundingStatus != types.GroundingGrounded ||
		it.Scope != types.ScopeLine || it.AnchorKind != types.AnchorArgument {
		return false
	}
	argument, receiver, ok := ground.DetectArgumentFlowAtLine(gc, it.Source, it.LineStart, it.Subject, it.Object)
	if !ok || argument != strings.TrimSpace(it.Subject) || receiver == "" || receiver == it.Object {
		return false
	}
	original := it.Object
	it.Object = receiver
	appendGroundingNoteOnce(it, fmt.Sprintf("argument receiver normalized from %q to the uniquely proved source expression %q", original, receiver))
	return true
}
