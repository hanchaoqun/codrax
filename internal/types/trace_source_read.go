package types

import (
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

// TraceQuerySourceReadRef is an opaque, run-local receipt for one physical
// capture actually queried. It is not serializable or reconstructible from a
// path, prose, or historical ToolResult. It reuses the query engine's shared
// O(1) physical file-generation identity, never a second full capture hash.
type TraceQuerySourceReadRef struct {
	path           string
	identity       filegeneration.Identity
	generation     *traceSourceReadGeneration
	physicalSource string
}

type traceSourceReadGeneration struct{ identity byte }

func (r TraceQuerySourceReadRef) Path() string { return r.path }

// TraceQueryPhysicalSourceReadCandidate is a producer-only single-physical-
// capture qualification, not a read permission. Keeping it on the opaque
// ToolResult receipt leaves serializable observation/frequency facts intact.
func TraceQueryPhysicalSourceReadCandidate(path string) TraceQuerySourceReadRef {
	return TraceQuerySourceReadRef{physicalSource: path}
}

func traceSourcePhysicalPath(path string) (string, filegeneration.Identity, bool) {
	path = strings.TrimSpace(path)
	// Never resolve a producer's relative path against the process CWD.
	if !filepath.IsAbs(path) {
		return "", filegeneration.Identity{}, false
	}
	physical, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil {
		return "", filegeneration.Identity{}, false
	}
	identity, err := filegeneration.FromPath(physical)
	if err != nil || !identity.Mode().IsRegular() {
		return "", filegeneration.Identity{}, false
	}
	return filepath.Clean(physical), identity, true
}

// PrepareTraceQuerySourceRead is called only by the native query producer,
// after physical source resolution and before reading. Preparing does not
// grant permission; successful native hard rows must still publish this path.
func (m *MutableState) PrepareTraceQuerySourceRead(actualPath string) TraceQuerySourceReadRef {
	if m == nil {
		return TraceQuerySourceReadRef{}
	}
	m.mu.RLock()
	generation := m.traceSourceReadGeneration
	m.mu.RUnlock()
	path, identity, ok := traceSourcePhysicalPath(actualPath)
	if !ok || generation == nil {
		return TraceQuerySourceReadRef{}
	}
	return TraceQuerySourceReadRef{path: path, identity: identity, generation: generation}
}

func traceQueryPublishesSource(result ToolResult, ref TraceQuerySourceReadRef) bool {
	if !result.Success || CanonicalToolName(result.ToolName) != "trace_query" || ref.generation == nil {
		return false
	}
	physical, identity, ok := traceSourcePhysicalPath(ref.physicalSource)
	if !ok || physical != ref.path || !ref.identity.SameVersion(identity) {
		return false
	}
	for _, obs := range result.Observations {
		if !RuntimeObservationProducerIsDeterministicQuery(obs.Producer) || obs.Origin != AnswerEvidenceOriginRuntimeArtifact ||
			obs.GroundingPolicy != ClaimGroundingHard || obs.SourceRef.Kind != ObservationSourceRuntimeArtifact ||
			obs.SourceRef.ArtifactKind != "trace" {
			continue
		}
		path, identity, ok := traceSourcePhysicalPath(obs.SourceRef.Path)
		if ok && path == ref.path && ref.identity.SameVersion(identity) {
			return true
		}
	}
	return false
}

// StampTraceQuerySourceRead keeps the producer's pre-read identity only when
// its successful typed result publishes that exact original source. Output
// payloads and summaries are not source coordinates. Registration occurs at
// AppendDispatchToolResult, never during this preparation step.
func (m *MutableState) StampTraceQuerySourceRead(ref TraceQuerySourceReadRef, result *ToolResult) {
	if result == nil {
		return
	}
	ref.physicalSource = result.TraceQuerySourceRead.physicalSource
	result.TraceQuerySourceRead = TraceQuerySourceReadRef{}
	// The existing memo key does not pin inode identity. A memo hit may keep
	// already registered permission, but must not grant the current file the
	// identity of bytes queried before a same-size/mtime file replacement.
	if m == nil || result.ReusedFromRunMemo || !traceQueryPublishesSource(*result, ref) {
		return
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if ref.generation == m.traceSourceReadGeneration {
		result.TraceQuerySourceRead = ref
	}
}

func (m *MutableState) registerTraceQuerySourceReadLocked(result ToolResult) {
	ref := result.TraceQuerySourceRead
	if ref.generation == nil || ref.generation != m.traceSourceReadGeneration || !traceQueryPublishesSource(result, ref) {
		return
	}
	if m.traceQuerySourceReads == nil {
		m.traceQuerySourceReads = map[string]TraceQuerySourceReadRef{}
	}
	m.traceQuerySourceReads[ref.path] = ref
}

// ResolveTraceQuerySourceRead accepts the actual absolute input path, with
// canonical/symlink equivalence but no basename or suffix aliases. Readers
// must still validate the receipt against their opened file descriptor.
func (m *MutableState) ResolveTraceQuerySourceRead(actualPath string) (TraceQuerySourceReadRef, bool) {
	if m == nil {
		return TraceQuerySourceReadRef{}, false
	}
	m.mu.RLock()
	empty := len(m.traceQuerySourceReads) == 0
	m.mu.RUnlock()
	if empty {
		return TraceQuerySourceReadRef{}, false
	}
	path, identity, ok := traceSourcePhysicalPath(actualPath)
	if !ok {
		return TraceQuerySourceReadRef{}, false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	ref, exists := m.traceQuerySourceReads[path]
	return ref, exists && ref.generation != nil && ref.generation == m.traceSourceReadGeneration && ref.identity.SameVersion(identity)
}

// TraceQuerySourceReadCurrent rechecks the opened descriptor and run epoch.
// It is used before and after bounded I/O, so rename/symlink retargets cannot
// substitute a different file between the agent gate and the real reader.
func (m *MutableState) TraceQuerySourceReadCurrent(ref TraceQuerySourceReadRef, opened filegeneration.Identity) bool {
	if m == nil || ref.generation == nil || !ref.identity.SameVersion(opened) {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	registered, ok := m.traceQuerySourceReads[ref.path]
	return ok && ref.generation == m.traceSourceReadGeneration && registered.generation == ref.generation &&
		registered.identity.SameVersion(opened)
}

func cloneTraceQuerySourceReads(in map[string]TraceQuerySourceReadRef) map[string]TraceQuerySourceReadRef {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]TraceQuerySourceReadRef, len(in))
	for path, ref := range in {
		out[path] = ref
	}
	return out
}

func (m *MutableState) mergeTraceQuerySourceReadsLocked(generation *traceSourceReadGeneration, refs map[string]TraceQuerySourceReadRef) {
	if generation == nil || generation != m.traceSourceReadGeneration {
		return // a late fork must never reopen a completed turn's permission
	}
	for path, ref := range refs {
		if ref.generation != generation {
			continue
		}
		if m.traceQuerySourceReads == nil {
			m.traceQuerySourceReads = map[string]TraceQuerySourceReadRef{}
		}
		if _, exists := m.traceQuerySourceReads[path]; !exists {
			m.traceQuerySourceReads[path] = ref
		}
	}
}
