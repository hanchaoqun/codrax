package orchestrator

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Bind render-time ownership to the exact final shipped bytes, including only
// the known system caveat replay lane and final outer-whitespace trim. A stale/rejected draft or raw fallback
// cannot borrow an accepted document just because one remains in Mutable.
func (o *Orchestrator) finalAnswerRenderedSurfaces(answer string) *types.AnswerRenderedSurfaces {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return nil
	}
	s := o.busCtx.Mutable.AnswerRenderedSurfaces()
	if s == nil {
		return nil
	}
	rendered := o.replayRegisteredAnswerCaveats(s.Answer)
	// appendRuntimeDispatchAdvisoriesToAnswer trims even when it has no
	// advisories. Accept only that exact existing display transform, not a
	// substring/fuzzy match or a later document re-render. Untracked additions
	// and any change within the answer remain unavailable for scoped audits.
	if rendered != answer && strings.TrimSpace(rendered) != answer {
		return nil
	}
	s.Answer = answer
	return s
}
