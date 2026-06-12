package types

import "reflect"

// ShallowClone returns a field-level shallow clone of ctx for handing
// to a concurrent worker (parallel explore dispatch) or a re-stamped
// sibling view. Every EXPORTED field is copied by straight assignment
// — identical aliasing semantics to a `cp := *ctx` struct copy (slice
// headers, maps, pointers and interface handles still alias the same
// backing data). The unexported answer-surface cache and its
// sync.Mutex are deliberately NOT copied: the clone starts with a
// fresh unlocked mutex and an empty cache.
//
// Why a constructor instead of `cp := *ctx` (go vet copylocks; same
// class as repomap's cloneGraphForRanking):
//
//  1. A value copy duplicates the mutex, so the copy's mutex no
//     longer provides mutual exclusion against the source for any
//     cache state the two might share.
//  2. The copied cache would be keyed (answerSurfaceCacheKey) to the
//     SOURCE's MutableState revision; parallel explore workers swap
//     in a forked MutableState immediately after copying, so an
//     inherited cache hit could serve a projection compiled against
//     the parent's Mutable. An empty cache recomputes deterministically
//     against the worker's own fork on first use.
//  3. The struct copy reads the cache fields without holding the
//     lock — benign only while the source cache is provably
//     quiescent, a scheduling invariant a constructor doesn't rely on.
//
// The cached projection objects themselves are clone-on-read /
// clone-on-store (see answer_plan_cache.go), so dropping them here
// loses no shared state — only a recomputable memo.
func (ctx *BusContext) ShallowClone() *BusContext {
	if ctx == nil {
		return nil
	}
	out := &BusContext{}
	shallowCopyExportedFields(reflect.ValueOf(out).Elem(), reflect.ValueOf(ctx).Elem())
	return out
}

// ShallowClone mirrors BusContext.ShallowClone for AgentContext,
// which carries the same unexported per-dispatch answer-surface
// cache + sync.Mutex. Use instead of `cp := *ctx` when re-stamping
// routing fields (AgentName / Stage) on a shared fixture.
func (ctx *AgentContext) ShallowClone() *AgentContext {
	if ctx == nil {
		return nil
	}
	out := &AgentContext{}
	shallowCopyExportedFields(reflect.ValueOf(out).Elem(), reflect.ValueOf(ctx).Elem())
	return out
}

// shallowCopyExportedFields assigns every exported field of src to
// dst (both must be addressable struct values of the same type).
// Unexported fields cannot be set via reflection and are skipped —
// which is exactly the contract: new exported fields propagate
// automatically with zero drift risk, while the unexported cache
// block stays at its zero value.
func shallowCopyExportedFields(dst, src reflect.Value) {
	for i := 0; i < src.NumField(); i++ {
		f := dst.Field(i)
		if !f.CanSet() {
			continue
		}
		f.Set(src.Field(i))
	}
}
