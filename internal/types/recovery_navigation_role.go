package types

import (
	"context"
	"path/filepath"
	"strings"
	"sync"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

// A material purpose is separate from its address. A real line in a recovery
// roster describes history, not the current checkout or a fresh measurement.
// This private registry cannot be restored from ToolResult/JSON role fields.
type recoveryNavigationRoles struct {
	mu      sync.RWMutex
	revoked bool
	files   map[string]filegeneration.Identity
}

// RecoveryNavigationRead is a producer-only pre-read candidate. Preparing it
// grants neither filesystem access nor evidence. Only the successful complete
// snapshot reader/resumer may commit it after validating the snapshot schema.
type RecoveryNavigationRead struct {
	epoch    *recoveryNavigationRoles
	path     string
	readPath string
	identity filegeneration.Identity
}

func (m *MutableState) PrepareRecoveryNavigationRead(path string) RecoveryNavigationRead {
	if m == nil {
		return RecoveryNavigationRead{}
	}
	m.mu.RLock()
	epoch := m.recoveryNavigationRoles
	m.mu.RUnlock()
	physical, identity, ok := traceSourcePhysicalPath(path)
	if !ok || epoch == nil {
		return RecoveryNavigationRead{}
	}
	return RecoveryNavigationRead{epoch: epoch, path: physical, readPath: path, identity: identity}
}

// RecordValidatedRecoveryNavigation records only negative source qualification.
// The caller must have consumed a complete DecodeSnapshot/Load result. Members,
// execution statuses, runtime values and causal relationships gain no authority.
func (m *MutableState) RecordValidatedRecoveryNavigation(ctx context.Context, read RecoveryNavigationRead) bool {
	if m == nil || ctx == nil || ctx.Err() != nil || read.epoch == nil {
		return false
	}
	path, current, ok := traceSourcePhysicalPath(read.readPath)
	if !ok || path != read.path || !read.identity.SameVersion(current) {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.recoveryNavigationRoles != read.epoch || ctx.Err() != nil {
		return false
	}
	read.epoch.mu.Lock()
	defer read.epoch.mu.Unlock()
	if read.epoch.revoked || ctx.Err() != nil {
		return false
	}
	if read.epoch.files == nil {
		read.epoch.files = map[string]filegeneration.Identity{}
	}
	read.epoch.files[path] = current
	return true
}

// IsRecoveryNavigationMaterial is the common qualification for source coverage,
// deterministic enrichment, ledger and direct citations. No CWD, basename or
// extension matching: relative addresses need the consumer's repository root.
// The same physical generation is revalidated each time; replacing the file
// with ordinary source/configuration does not inherit its historical purpose.
func (m *MutableState) IsRecoveryNavigationMaterial(root, path string) bool {
	if m == nil || strings.TrimSpace(path) == "" {
		return false
	}
	// The ordinary source path pays no filesystem cost when no navigation was
	// registered. Read the shared epoch under its lock before resolving aliases.
	m.mu.RLock()
	epoch := m.recoveryNavigationRoles
	m.mu.RUnlock()
	if epoch == nil {
		return false
	}
	epoch.mu.RLock()
	active := !epoch.revoked && len(epoch.files) > 0
	epoch.mu.RUnlock()
	if !active {
		return false
	}
	if !filepath.IsAbs(path) {
		if !filepath.IsAbs(root) {
			return false
		}
		path = filepath.Join(root, filepath.FromSlash(path))
	}
	physical, identity, ok := traceSourcePhysicalPath(path)
	if !ok {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if epoch != m.recoveryNavigationRoles {
		return false
	}
	epoch.mu.RLock()
	defer epoch.mu.RUnlock()
	previous, found := epoch.files[physical]
	return !epoch.revoked && found && previous.SameVersion(identity)
}

func (input ObservationLedgerInput) currentSourceSupportWitnesses() currentSourceSupportWitnessIndex {
	index := compileCurrentSourceSupportWitnessIndex(input.EvidenceItems, input.ToolResults)
	out := currentSourceSupportWitnessIndex{}
	for _, witness := range index.witnesses {
		if !input.recoveryNavigationConsumer.IsRecoveryNavigationMaterial(input.RepoRoot, witness.path) {
			out.witnesses = append(out.witnesses, witness)
		}
	}
	return out
}

func recoveryNavigationConsumerForMode(m *MutableState, mode PipelineMode) *MutableState {
	if mode.IsWrite() {
		return nil
	}
	return m
}

// A previously compiled claim pool may outlive the full read that established
// material purpose. Requalify cached coordinates too, without deleting their
// stored bytes or re-reading content as new evidence.
func (input ObservationLedgerInput) qualifyRecoverySourceClaims(source *aggregateSourceClaimContext) *aggregateSourceClaimContext {
	if source == nil || input.recoveryNavigationConsumer == nil {
		return source
	}
	out := &aggregateSourceClaimContext{}
	for _, witness := range source.coordinates.witnesses {
		if !input.recoveryNavigationConsumer.IsRecoveryNavigationMaterial(input.RepoRoot, witness.path) {
			out.coordinates.witnesses = append(out.coordinates.witnesses, witness)
		}
	}
	for _, item := range source.items {
		if !input.recoveryNavigationConsumer.IsRecoveryNavigationMaterial(input.RepoRoot, item.Source) {
			out.items = append(out.items, item)
		}
	}
	for _, member := range source.inventory {
		if !input.recoveryNavigationConsumer.IsRecoveryNavigationMaterial(input.RepoRoot, member.path) {
			out.inventory = append(out.inventory, member)
		}
	}
	return out
}

func (m *MutableState) resetRecoveryNavigationRolesLocked() {
	if m.recoveryNavigationRoles != nil {
		m.recoveryNavigationRoles.mu.Lock()
		m.recoveryNavigationRoles.revoked = true
		m.recoveryNavigationRoles.mu.Unlock()
	}
	m.recoveryNavigationRoles = &recoveryNavigationRoles{}
}

func (input ObservationLedgerInput) qualifyRecoveryNavigation(record ObservationRecord) ObservationRecord {
	if input.recoveryNavigationConsumer.IsRecoveryNavigationMaterial(input.RepoRoot, record.SourceRef.Path) {
		// Keep the field and exact coordinates, but do not relabel historical
		// data as model inference or as a current runtime observation.
		record.Origin = AnswerEvidenceOriginExternalDocument
		record.SourceRef.Kind = ObservationSourceRecoveryNavigation
		record.GroundingPolicy = ClaimGroundingRepairable
		record.ClaimAuthority = ObservationClaimAuthorityDirectObservation
		record.RichNotes = append(record.RichNotes, "Recovery navigation: historical fields, not current source, current measurements or completion evidence.")
	}
	return record
}
