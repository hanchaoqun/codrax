package types

import (
	"context"
	"path/filepath"
	"sync"

	"github.com/hanchaoqun/codrax/internal/attachment"
)

// One successful query universe, or permanently ambiguous for this epoch.
// It is independent of bounded handoff/result slices: dropping an older A
// result must never make the later B the only selection. No JSON can mint it.
type tracePreparedQuerySelectionLedger struct {
	mu          sync.Mutex
	first       *traceIntervalNavigation
	original    string
	ambiguous   bool
	archiveSeen bool
}

// TracePreparedSelectionStatus distinguishes missing navigation from a
// selection which must not fall back to another original. It grants no data
// or causal authority and is never part of model-authored JSON.
type TracePreparedSelectionStatus uint8

const (
	TracePreparedSelectionAbsent TracePreparedSelectionStatus = iota
	TracePreparedSelectionUnavailable
	TracePreparedSelectionUnique
)

func (m *MutableState) registerTracePreparedQuerySelection(ctx context.Context, ref *traceIntervalNavigation) {
	if m == nil || ref == nil || ref.material == nil || ref.source.generation == nil ||
		ctx != nil && ctx.Err() != nil || m.SystemTraceSupplementInProgress() {
		return
	}
	original, err := filepath.EvalSymlinks(ref.material.SourcePath())
	if err != nil {
		return
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if ref.source.generation != m.traceSourceReadGeneration {
		return // A late fork cannot reopen the next turn's selection.
	}
	ledger := &ref.source.generation.preparedSelection
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.archiveSeen = ledger.archiveSeen || ref.material.ArchiveMember() != ""
	if ledger.ambiguous {
		return
	}
	if ledger.first == nil {
		ledger.first, ledger.original = ref, original
		return
	}
	first := ledger.first
	if ledger.original != original || first.queryPath != ref.queryPath ||
		first.material.ArchiveMember() != ref.material.ArchiveMember() ||
		!first.source.identity.SameVersion(ref.source.identity) {
		ledger.ambiguous = true
	}
	// Equal coordinates never replace the first receipt: a new wrapper cannot
	// refresh its original/manifest/member generation or its query universe.
}

// SuccessfulPreparedArchiveSelection is navigation for the system's
// existing source resolver, not new read, window, causal or completion rights.
// Full preparation bindings and the current run epoch are rechecked. Failed,
// canceled and memo queries never reach registration in the producer stamp.
func (m *MutableState) SuccessfulPreparedArchiveSelection(ctx context.Context) (*attachment.TraceMaterial, TracePreparedSelectionStatus) {
	if m == nil {
		return nil, TracePreparedSelectionAbsent
	}
	m.mu.RLock()
	generation := m.traceSourceReadGeneration
	m.mu.RUnlock()
	if generation == nil {
		return nil, TracePreparedSelectionAbsent
	}
	ledger := &generation.preparedSelection
	ledger.mu.Lock()
	ref, ambiguous, archiveSeen := ledger.first, ledger.ambiguous, ledger.archiveSeen
	ledger.mu.Unlock()
	if !archiveSeen {
		return nil, TracePreparedSelectionAbsent
	}
	if ref == nil || ambiguous {
		return nil, TracePreparedSelectionUnavailable
	}
	path, _, current := m.resolveTracePreparedQuerySource(ctx, ref)
	if !current || ref.material == nil || path != ref.material.QueryPath() {
		return nil, TracePreparedSelectionUnavailable
	}
	return ref.material, TracePreparedSelectionUnique
}
