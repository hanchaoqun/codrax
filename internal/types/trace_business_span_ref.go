package types

import (
	"crypto/rand"
	"encoding/hex"
	"math"
	"strings"
)

// TraceBusinessSpanRefRunLimit bounds all issued navigation receipts in one
// epoch, including sibling forks and receipts never published. Capacity is
// reserved before exposure and never reclaimed, so merges need not evict an
// already published token. Per-result publication uses TraceBusinessSpanFactLimit.
const TraceBusinessSpanRefRunLimit = 1024

// TraceBusinessSpanCandidate is supplied only by the native complete sync B/E
// producer. The thread owner and full instance window travel as one value;
// observation subjects, notes and serialized tool output cannot create it.
type TraceBusinessSpanCandidate struct {
	Path      string
	TID       int
	Thread    string
	Name      string
	Kind      string
	StartLine int
	EndLine   int
	StartTs   float64
	EndTs     float64
}

// TraceBusinessSpanRef is an opaque run-local navigation receipt, not target,
// closure or root-cause authority. Its original physical source read is never
// replaced by a later receipt for the same path.
type TraceBusinessSpanRef struct {
	token  string
	data   TraceBusinessSpanCandidate
	source TraceQuerySourceReadRef
}

func (r TraceBusinessSpanRef) Token() string                    { return r.token }
func (r TraceBusinessSpanRef) Data() TraceBusinessSpanCandidate { return r.data }
func (r TraceBusinessSpanRef) SourceRead() TraceQuerySourceReadRef {
	return r.source
}

// TraceBusinessSpanQuerySourceMatches checks only physical-query provenance;
// it neither publishes a receipt nor requires a nonempty observation result.
// Call before StampTraceQuerySourceRead clears an unpublishable candidate.
// The consumer must also check TraceBusinessSpanRefCurrent before and after
// its query. A multi-source promotion has no single-physical qualification.
func TraceBusinessSpanQuerySourceMatches(ref TraceBusinessSpanRef, before TraceQuerySourceReadRef, result ToolResult) bool {
	if ref.source.generation == nil || before.generation != ref.source.generation ||
		before.path != ref.source.path || !ref.source.identity.SameVersion(before.identity) {
		return false
	}
	path, identity, ok := traceSourcePhysicalPath(result.TraceQuerySourceRead.physicalSource)
	return ok && path == ref.source.path && ref.source.identity.SameVersion(identity)
}

func (generation *traceSourceReadGeneration) reserveTraceBusinessSpanRef() bool {
	for {
		issued := generation.businessSpanIssued.Load()
		if issued >= TraceBusinessSpanRefRunLimit {
			return false
		}
		if generation.businessSpanIssued.CompareAndSwap(issued, issued+1) {
			return true
		}
	}
}

func validTraceBusinessSpanCandidate(candidate TraceBusinessSpanCandidate, source TraceQuerySourceReadRef) (TraceBusinessSpanCandidate, bool) {
	if candidate.Kind != "sync" || candidate.TID <= 0 || candidate.TID > RuntimeTargetMaxPID ||
		strings.TrimSpace(candidate.Thread) == "" || strings.TrimSpace(candidate.Name) == "" ||
		candidate.StartLine <= 0 || candidate.EndLine <= candidate.StartLine ||
		math.IsNaN(candidate.StartTs) || math.IsNaN(candidate.EndTs) ||
		math.IsInf(candidate.StartTs, 0) || math.IsInf(candidate.EndTs, 0) ||
		candidate.StartTs < 0 || candidate.EndTs <= candidate.StartTs {
		return TraceBusinessSpanCandidate{}, false
	}
	path, identity, ok := traceSourcePhysicalPath(candidate.Path)
	if !ok || source.generation == nil || path != source.path || !source.identity.SameVersion(identity) {
		return TraceBusinessSpanCandidate{}, false
	}
	candidate.Path = path
	return candidate, true
}

// StampTraceBusinessSpanRefs runs after StampTraceQuerySourceRead. It mints
// fresh, non-reconstructible tokens but does not publish them into the state.
// Memo hits never mint or carry references into a new publication.
func (m *MutableState) StampTraceBusinessSpanRefs(result *ToolResult) {
	if result == nil {
		return
	}
	result.TraceBusinessSpanRefs = nil
	if len(result.TraceBusinessSpanCandidates) > TraceBusinessSpanFactLimit {
		result.TraceBusinessSpanCandidates = result.TraceBusinessSpanCandidates[:TraceBusinessSpanFactLimit]
	}
	result.TraceBusinessSpanCandidates = append([]TraceBusinessSpanCandidate(nil), result.TraceBusinessSpanCandidates...)
	if m == nil || result.ReusedFromRunMemo || len(result.TraceBusinessSpanCandidates) == 0 ||
		!traceQueryPublishesSource(*result, result.TraceQuerySourceRead) {
		return
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	source := result.TraceQuerySourceRead
	if source.generation != m.traceSourceReadGeneration {
		return
	}
	for _, candidate := range result.TraceBusinessSpanCandidates {
		data, ok := validTraceBusinessSpanCandidate(candidate, source)
		if !ok {
			continue
		}
		var random [24]byte
		if _, err := rand.Read(random[:]); err != nil {
			// Fail closed: ordinary facts remain available without navigation.
			result.TraceBusinessSpanRefs = nil
			return
		}
		if !source.generation.reserveTraceBusinessSpanRef() {
			return
		}
		result.TraceBusinessSpanRefs = append(result.TraceBusinessSpanRefs, TraceBusinessSpanRef{
			token: "business-span:" + hex.EncodeToString(random[:]), data: data, source: source,
		})
	}
}

func sameTraceBusinessSpanRef(a, b TraceBusinessSpanRef) bool {
	return a.token != "" && a.token == b.token && a.data == b.data &&
		a.source.path == b.source.path && a.source.generation == b.source.generation &&
		a.source.identity.SameVersion(b.source.identity)
}

func (m *MutableState) registerTraceBusinessSpanRefsLocked(result ToolResult) {
	source := result.TraceQuerySourceRead
	if result.ReusedFromRunMemo || source.generation == nil || source.generation != m.traceSourceReadGeneration ||
		len(result.TraceBusinessSpanRefs) == 0 || !traceQueryPublishesSource(result, source) {
		return
	}
	for _, ref := range result.TraceBusinessSpanRefs {
		if ref.token == "" || ref.source.generation != source.generation || ref.source.path != source.path ||
			!ref.source.identity.SameVersion(source.identity) {
			continue
		}
		matched := false
		for _, candidate := range result.TraceBusinessSpanCandidates {
			data, ok := validTraceBusinessSpanCandidate(candidate, source)
			if ok && data == ref.data {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		if m.traceBusinessSpanRefs == nil {
			m.traceBusinessSpanRefs = map[string]TraceBusinessSpanRef{}
		}
		// A token never changes the instance it names, even on duplicate append.
		if _, exists := m.traceBusinessSpanRefs[ref.token]; !exists {
			m.traceBusinessSpanRefs[ref.token] = ref
		}
	}
}

// ResolveTraceBusinessSpanRef accepts only a published token in this run. It
// validates the saved original receipt, not the latest receipt for its path.
func (m *MutableState) ResolveTraceBusinessSpanRef(token string) (TraceBusinessSpanRef, bool) {
	if m == nil || token == "" {
		return TraceBusinessSpanRef{}, false
	}
	m.mu.RLock()
	ref, exists := m.traceBusinessSpanRefs[token]
	m.mu.RUnlock()
	if !exists || !m.TraceBusinessSpanRefCurrent(ref) {
		return TraceBusinessSpanRef{}, false
	}
	return ref, true
}

// TraceBusinessSpanRefCurrent rechecks both publication and the original file
// generation. Query consumers call it before and after consuming the window.
func (m *MutableState) TraceBusinessSpanRefCurrent(ref TraceBusinessSpanRef) bool {
	if m == nil || ref.token == "" || ref.source.generation == nil {
		return false
	}
	path, identity, ok := traceSourcePhysicalPath(ref.source.path)
	if !ok || path != ref.source.path || !ref.source.identity.SameVersion(identity) {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	registered, exists := m.traceBusinessSpanRefs[ref.token]
	return exists && ref.source.generation == m.traceSourceReadGeneration && sameTraceBusinessSpanRef(ref, registered)
}

func cloneTraceBusinessSpanRefs(in map[string]TraceBusinessSpanRef) map[string]TraceBusinessSpanRef {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]TraceBusinessSpanRef, len(in))
	for token, ref := range in {
		out[token] = ref
	}
	return out
}

// Clone only these new private slices, preserving the existing ToolResult
// copy contract for unrelated carriers.
func cloneTraceBusinessSpanToolResult(in ToolResult) ToolResult {
	out := in
	if in.Observations != nil {
		out.Observations = append([]ObservationRecord{}, in.Observations...)
		for i := range out.Observations {
			out.Observations[i].StateAccounting = CloneTraceSchedulerStateAccounts(in.Observations[i].StateAccounting)
		}
	}
	if in.Handoff != nil && in.Handoff.Documentation != nil {
		carrier := *in.Handoff
		carrier.Documentation = cloneToolDocumentation(in.Handoff.Documentation)
		out.Handoff = &carrier
	}
	out.TraceBusinessSpanCandidates = append([]TraceBusinessSpanCandidate(nil), in.TraceBusinessSpanCandidates...)
	out.TraceBusinessSpanRefs = append([]TraceBusinessSpanRef(nil), in.TraceBusinessSpanRefs...)
	return out
}

func cloneTraceBusinessSpanToolResults(in []ToolResult) []ToolResult {
	if in == nil {
		return nil
	}
	out := make([]ToolResult, len(in))
	for i, result := range in {
		out[i] = cloneTraceBusinessSpanToolResult(result)
	}
	return out
}

func traceBusinessSpanToolResultBytes(result ToolResult) int {
	n := 0
	for _, candidate := range result.TraceBusinessSpanCandidates {
		n += len(candidate.Path) + len(candidate.Thread) + len(candidate.Name) + len(candidate.Kind) + 80
	}
	for _, ref := range result.TraceBusinessSpanRefs {
		n += len(ref.token) + len(ref.data.Path) + len(ref.data.Thread) + len(ref.data.Name) + len(ref.data.Kind) + 160
	}
	return n
}

func (m *MutableState) mergeTraceBusinessSpanRefsLocked(generation *traceSourceReadGeneration, refs map[string]TraceBusinessSpanRef) {
	if generation == nil || generation != m.traceSourceReadGeneration {
		return
	}
	for token, ref := range refs {
		if token == "" || token != ref.token || ref.source.generation != generation {
			continue
		}
		if m.traceBusinessSpanRefs == nil {
			m.traceBusinessSpanRefs = map[string]TraceBusinessSpanRef{}
		}
		if _, exists := m.traceBusinessSpanRefs[token]; !exists {
			m.traceBusinessSpanRefs[token] = ref
		}
	}
}
