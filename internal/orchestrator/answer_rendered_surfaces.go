package orchestrator

import "github.com/hanchaoqun/codrax/internal/types"

// Bind render-time ownership to the exact final shipped bytes, including only
// the known system caveat replay lane. A stale/rejected draft or raw fallback
// cannot borrow an accepted document just because one remains in Mutable.
func (o *Orchestrator) finalAnswerRenderedSurfaces(answer string) *types.AnswerRenderedSurfaces {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return nil
	}
	s := o.busCtx.Mutable.AnswerRenderedSurfaces()
	if s == nil {
		return nil
	}
	if o.replayRegisteredAnswerCaveats(s.Answer) != answer {
		return nil
	}
	s.Answer = answer
	return s
}
