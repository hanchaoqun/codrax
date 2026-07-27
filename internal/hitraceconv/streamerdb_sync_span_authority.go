package hitraceconv

import (
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// traceDBSyncSpanProducer is a closed set of Trace Streamer DB exporters that
// share the physical synchronous trace-marker stack. S/F, I and C producers
// deliberately do not enter this authority.
type traceDBSyncSpanProducer uint8

const (
	traceDBSyncSpanProducerUnknown traceDBSyncSpanProducer = iota
	traceDBSyncSpanProducerRegistration
	traceDBSyncSpanProducerCallstack
	traceDBSyncSpanProducerSyscall
	traceDBSyncSpanProducerAppStartup
	traceDBSyncSpanProducerStaticInitialize
)

type traceDBSyncSpanStableKind uint8

const (
	traceDBSyncSpanStableUnknown traceDBSyncSpanStableKind = iota
	traceDBSyncSpanStableRegistrationITID
	traceDBSyncSpanStableCallstackRowID
	traceDBSyncSpanStableSyscallRowID
	traceDBSyncSpanStableAppStartupRowID
	traceDBSyncSpanStableStaticInitializeRowID
)

type traceDBSyncSpanNameProvenance uint8

const (
	traceDBSyncSpanNameUnknown traceDBSyncSpanNameProvenance = iota
	traceDBSyncSpanNameRegistration
	traceDBSyncSpanNameCallstack
	traceDBSyncSpanNameSyscallNumber
	traceDBSyncSpanNameAppStartupDictionary
	traceDBSyncSpanNameStaticObject
)

type traceDBSyncSpanCPUProvenance uint8

const (
	traceDBSyncSpanCPUUnknown traceDBSyncSpanCPUProvenance = iota
	traceDBSyncSpanCPURegistrationMetadata
	traceDBSyncSpanCPUCallstackTypedRunning
	traceDBSyncSpanCPUSyscallTypedRunning
	traceDBSyncSpanCPULegacyUnverified
	traceDBSyncSpanCPUCallstackUnavailable
)

// traceDBSyncSpanCPUPlacement is deliberately separate from CPU provenance.
// A callstack row may prove the span identity and interval while scheduler
// evidence cannot place it on a physical CPU. Such a row remains publishable
// through the typed comment lane and must never be rendered as CPU 0.
type traceDBSyncSpanCPUPlacement uint8

const (
	traceDBSyncSpanCPUPlacementKnown traceDBSyncSpanCPUPlacement = iota
	traceDBSyncSpanCPUPlacementUnknownStart
	traceDBSyncSpanCPUPlacementUnknownEnd
	traceDBSyncSpanCPUPlacementSourceTainted
	traceDBSyncSpanCPUPlacementLifecycleRejected
	traceDBSyncSpanCPUPlacementAliasAmbiguous
)

type traceDBSyncSpanDepthProvenance uint8

const (
	traceDBSyncSpanDepthUnknown traceDBSyncSpanDepthProvenance = iota
	traceDBSyncSpanDepthCallstack
)

type traceDBSyncSpanCandidate struct {
	Producer   traceDBSyncSpanProducer
	StableKind traceDBSyncSpanStableKind
	StableID   int64
	HeaderTID  int64
	HeaderTGID int64
	// MarkerPID is the PID encoded inside tracing_mark_write. When absent it
	// is exactly HeaderTGID. A known differing value preserves namespace PID
	// syntax without falsifying the host ftrace envelope.
	MarkerPID          int64
	MarkerPIDKnown     bool
	CanonicalITID      int64
	CanonicalITIDKnown bool
	OwnerIPID          int64
	OwnerIPIDKnown     bool
	Start              int64
	End                int64
	StartCPU           int64
	EndCPU             int64
	CPUPlacement       traceDBSyncSpanCPUPlacement
	StartCPUProvenance traceDBSyncSpanCPUProvenance
	EndCPUProvenance   traceDBSyncSpanCPUProvenance
	Task               string
	Name               string
	NameProvenance     traceDBSyncSpanNameProvenance
	Depth              int64
	DepthKnown         bool
	DepthProvenance    traceDBSyncSpanDepthProvenance
}

type traceDBSyncSpanLane struct {
	ArtifactSource string
	HeaderTID      int64
}

type traceDBSyncSpanIdentity struct {
	Producer   traceDBSyncSpanProducer
	StableKind traceDBSyncSpanStableKind
	StableID   int64
}

type traceDBSyncSpanLanePoisonReason uint8

const (
	traceDBSyncSpanLanePoisonUnknown traceDBSyncSpanLanePoisonReason = iota
	traceDBSyncSpanLanePoisonRejectedCallstackCandidate
	traceDBSyncSpanLanePoisonRejectedSyscallCandidate
)

type traceDBSyncSpanLanePoison struct {
	Producer           traceDBSyncSpanProducer
	HeaderTID          int64
	CanonicalITID      int64
	CanonicalITIDKnown bool
	Reason             traceDBSyncSpanLanePoisonReason
}

type traceDBSyncSpanGlobalPoisonReason uint8

const (
	traceDBSyncSpanGlobalPoisonUnknown traceDBSyncSpanGlobalPoisonReason = iota
	traceDBSyncSpanGlobalPoisonUnlocalizableSyscallCandidate
)

type traceDBSyncSpanGlobalPoison struct {
	Producer traceDBSyncSpanProducer
	Reason   traceDBSyncSpanGlobalPoisonReason
}

type traceDBSyncSpanProducerStats struct {
	SubmittedSpans           int
	EmittedEndpoints         int
	SuppressedSpans          int
	PoisonDeclarations       int
	GlobalPoisonDeclarations int
}

type traceDBSyncSpanReport struct {
	ByProducer             map[traceDBSyncSpanProducer]traceDBSyncSpanProducerStats
	SubmittedSpans         int
	EmittedEndpoints       int
	SuppressedSpans        int
	PoisonedLanes          int
	CrossingLanes          int
	IdenticalLanes         int
	IdentityLanes          int
	DepthLanes             int
	DuplicateLanes         int
	BudgetFailClosedReason string
	SourceFailClosedReason string
	GlobalPoisoned         bool
}

type traceDBSyncSpanAuthorityState uint8

const (
	traceDBSyncSpanAuthorityOpen traceDBSyncSpanAuthorityState = iota
	traceDBSyncSpanAuthorityFinalizing
	traceDBSyncSpanAuthorityFinalized
	traceDBSyncSpanAuthorityFailed
)

// traceDBSyncSpanAuthority is the sole authority for synthetic B/E rows made
// from one Trace Streamer SQLite artifact. Candidate storage and duplicate /
// poison arbitration are delegated to one bounded typed stage.
type traceDBSyncSpanAuthority struct {
	artifactSource      string
	state               traceDBSyncSpanAuthorityState
	stage               *traceDBSyncSpanStage
	submitted           [traceDBSyncSpanProducerStaticInitialize + 1]int
	poisoned            [traceDBSyncSpanProducerStaticInitialize + 1]int
	globalPoisoned      [traceDBSyncSpanProducerStaticInitialize + 1]bool
	submittedTotal      int
	poisonedTotal       int
	globalPoisonedTotal int
}

func newTraceDBSyncSpanAuthority(ctx context.Context, outputArtifact string) (*traceDBSyncSpanAuthority, error) {
	return newTraceDBSyncSpanAuthorityWithOptions(ctx, outputArtifact, traceDBSyncSpanStageOptions{})
}

func newTraceDBSyncSpanAuthorityWithOptions(ctx context.Context, outputArtifact string, options traceDBSyncSpanStageOptions) (*traceDBSyncSpanAuthority, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	trimmed := strings.TrimSpace(outputArtifact)
	if trimmed == "" {
		return nil, &traceDBOutputInvariantError{Reason: "missing_sync_span_artifact_source"}
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return nil, fmt.Errorf("resolve sync span artifact source: %w", err)
	}
	stage, err := newTraceDBSyncSpanStage(ctx, options)
	if err != nil {
		return nil, err
	}
	return &traceDBSyncSpanAuthority{
		artifactSource: filepath.Clean(abs),
		stage:          stage,
	}, nil
}

func (authority *traceDBSyncSpanAuthority) submit(ctx context.Context, candidate traceDBSyncSpanCandidate) error {
	if authority == nil || authority.state != traceDBSyncSpanAuthorityOpen || authority.artifactSource == "" {
		return &traceDBOutputInvariantError{Reason: "sync_span_authority_not_open"}
	}
	if err := validateTraceDBSyncSpanCandidate(candidate); err != nil {
		return err
	}
	if authority.submitted[candidate.Producer] == math.MaxInt || authority.submittedTotal == math.MaxInt {
		authority.state = traceDBSyncSpanAuthorityFailed
		return &traceDBOutputInvariantError{Reason: "sync_span_authority_submission_count_overflow"}
	}
	authority.submitted[candidate.Producer]++
	authority.submittedTotal++
	if err := authority.stage.addCandidate(ctx, candidate); err != nil {
		if _, ok := traceDBSyncSpanPureBudgetReason(err); ok {
			return nil
		}
		authority.state = traceDBSyncSpanAuthorityFailed
		return err
	}
	return nil
}

func (authority *traceDBSyncSpanAuthority) poisonExactLane(ctx context.Context, poison traceDBSyncSpanLanePoison) error {
	if authority == nil || authority.state != traceDBSyncSpanAuthorityOpen || authority.artifactSource == "" {
		return &traceDBOutputInvariantError{Reason: "sync_span_authority_not_open"}
	}
	if err := validateTraceDBSyncSpanLanePoison(poison); err != nil {
		return err
	}
	if authority.poisoned[poison.Producer] == math.MaxInt || authority.poisonedTotal == math.MaxInt {
		authority.state = traceDBSyncSpanAuthorityFailed
		return &traceDBOutputInvariantError{Reason: "sync_span_authority_poison_count_overflow"}
	}
	authority.poisoned[poison.Producer]++
	authority.poisonedTotal++
	if err := authority.stage.addPoison(ctx, poison); err != nil {
		if _, ok := traceDBSyncSpanPureBudgetReason(err); ok {
			return nil
		}
		authority.state = traceDBSyncSpanAuthorityFailed
		return err
	}
	return nil
}

func validateTraceDBSyncSpanLanePoison(poison traceDBSyncSpanLanePoison) error {
	closedReason := poison.Producer == traceDBSyncSpanProducerCallstack &&
		poison.Reason == traceDBSyncSpanLanePoisonRejectedCallstackCandidate ||
		poison.Producer == traceDBSyncSpanProducerSyscall &&
			poison.Reason == traceDBSyncSpanLanePoisonRejectedSyscallCandidate
	if !closedReason || poison.HeaderTID <= 0 || poison.HeaderTID > math.MaxInt32 ||
		!poison.CanonicalITIDKnown || poison.CanonicalITID <= 0 || poison.CanonicalITID > maxTraceDBInternalID {
		return &traceDBOutputInvariantError{Reason: "invalid_sync_span_exact_lane_poison"}
	}
	return nil
}

// poisonGlobally records a source-admission failure whose physical B/E lane
// cannot be proven. It is deliberately constant-state and idempotent per
// producer: an untrusted table cannot grow an in-memory diagnostic ledger.
// Finalize remains the sole publication point and suppresses every governed
// B/E candidate while leaving S/F/I/C producers outside this authority alive.
func (authority *traceDBSyncSpanAuthority) poisonGlobally(ctx context.Context, poison traceDBSyncSpanGlobalPoison) error {
	if authority == nil || authority.state != traceDBSyncSpanAuthorityOpen || authority.artifactSource == "" {
		return &traceDBOutputInvariantError{Reason: "sync_span_authority_not_open"}
	}
	if ctx == nil {
		return &traceDBOutputInvariantError{Reason: "missing_sync_span_global_poison_context"}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateTraceDBSyncSpanGlobalPoison(poison); err != nil {
		return err
	}
	if !authority.globalPoisoned[poison.Producer] {
		authority.globalPoisoned[poison.Producer] = true
		authority.globalPoisonedTotal++
	}
	return nil
}

func validateTraceDBSyncSpanGlobalPoison(poison traceDBSyncSpanGlobalPoison) error {
	if poison.Producer != traceDBSyncSpanProducerSyscall ||
		poison.Reason != traceDBSyncSpanGlobalPoisonUnlocalizableSyscallCandidate {
		return &traceDBOutputInvariantError{Reason: "invalid_sync_span_global_poison"}
	}
	return nil
}

func (authority *traceDBSyncSpanAuthority) cleanup() error {
	if authority == nil || authority.stage == nil {
		return nil
	}
	return authority.stage.cleanup()
}

func traceDBSyncSpanPureBudgetReason(err error) (string, bool) {
	reason, ok := traceDBSyncSpanStageBudgetReason(err)
	if !ok || !traceDBSyncSpanErrorTreeOnlyBudget(err) {
		return "", false
	}
	return reason, true
}

func traceDBSyncSpanErrorTreeOnlyBudget(err error) bool {
	if err == nil {
		return true
	}
	var budget *traceDBSyncSpanStageBudgetError
	if errors.As(err, &budget) {
		if joined, ok := err.(interface{ Unwrap() []error }); ok {
			children := joined.Unwrap()
			if len(children) == 0 {
				return false
			}
			for _, child := range children {
				if !traceDBSyncSpanErrorTreeOnlyBudget(child) {
					return false
				}
			}
			return true
		}
		if _, direct := err.(*traceDBSyncSpanStageBudgetError); direct {
			return true
		}
		if wrapped, ok := err.(interface{ Unwrap() error }); ok {
			return traceDBSyncSpanErrorTreeOnlyBudget(wrapped.Unwrap())
		}
	}
	return false
}

func validateTraceDBSyncSpanCandidate(candidate traceDBSyncSpanCandidate) error {
	if candidate.Producer <= traceDBSyncSpanProducerUnknown || candidate.Producer > traceDBSyncSpanProducerStaticInitialize {
		return &traceDBOutputInvariantError{Reason: "invalid_sync_span_candidate"}
	}
	if candidate.Start < 0 {
		return &traceDBOutputInvariantError{Reason: "invalid_timestamp"}
	}
	if candidate.End < candidate.Start {
		return &traceDBOutputInvariantError{Reason: "invalid_interval"}
	}
	if candidate.CPUPlacement > traceDBSyncSpanCPUPlacementAliasAmbiguous {
		return &traceDBOutputInvariantError{Reason: "invalid_sync_span_cpu_placement"}
	}
	if candidate.CPUPlacement == traceDBSyncSpanCPUPlacementKnown &&
		(!validTraceDBCPUIndex(candidate.StartCPU) || !validTraceDBCPUIndex(candidate.EndCPU)) {
		return &traceDBOutputInvariantError{Reason: "invalid_cpu"}
	}
	if candidate.CPUPlacement != traceDBSyncSpanCPUPlacementKnown &&
		(candidate.Producer != traceDBSyncSpanProducerCallstack ||
			candidate.StartCPU != 0 || candidate.EndCPU != 0 ||
			candidate.StartCPUProvenance != traceDBSyncSpanCPUCallstackUnavailable ||
			candidate.EndCPUProvenance != traceDBSyncSpanCPUCallstackUnavailable) {
		return &traceDBOutputInvariantError{Reason: "invalid_sync_span_unavailable_cpu_placement"}
	}
	if candidate.HeaderTID < 0 || candidate.HeaderTID > math.MaxInt32 {
		return &traceDBOutputInvariantError{Reason: "invalid_tid"}
	}
	if candidate.HeaderTGID < 0 || candidate.HeaderTGID > math.MaxInt32 {
		return &traceDBOutputInvariantError{Reason: "invalid_tgid"}
	}
	if (candidate.HeaderTID == 0) != (candidate.HeaderTGID == 0) {
		return &traceDBOutputInvariantError{Reason: "incomplete_header_identity"}
	}
	if candidate.MarkerPIDKnown {
		if candidate.Producer != traceDBSyncSpanProducerCallstack ||
			candidate.MarkerPID <= 0 || candidate.MarkerPID > math.MaxInt32 {
			return &traceDBOutputInvariantError{Reason: "invalid_marker_pid"}
		}
	} else if candidate.MarkerPID != 0 {
		return &traceDBOutputInvariantError{Reason: "unproven_marker_pid"}
	}
	if candidate.HeaderTID == 0 && (candidate.Producer != traceDBSyncSpanProducerRegistration ||
		!candidate.CanonicalITIDKnown || candidate.CanonicalITID != 0 ||
		candidate.StableKind != traceDBSyncSpanStableRegistrationITID || candidate.StableID != 0) {
		return &traceDBOutputInvariantError{Reason: "unproven_sync_span_idle_subject"}
	}
	if !traceDBSinglePhysicalLine(candidate.Task, true) {
		return &traceDBOutputInvariantError{Reason: "invalid_task"}
	}
	exactCallstackName := candidate.Producer == traceDBSyncSpanProducerCallstack &&
		candidate.NameProvenance == traceDBSyncSpanNameCallstack &&
		traceDBCallstackSpanName(candidate.Name)
	if !traceDBCallstackMarkerToken(candidate.Name) && !exactCallstackName {
		return &traceDBOutputInvariantError{Reason: "invalid_span_name"}
	}
	if candidate.CanonicalITIDKnown && (candidate.CanonicalITID < 0 || candidate.CanonicalITID > maxTraceDBInternalID) {
		return &traceDBOutputInvariantError{Reason: "invalid_sync_span_canonical_itid"}
	}
	if !candidate.CanonicalITIDKnown && candidate.CanonicalITID != 0 {
		return &traceDBOutputInvariantError{Reason: "unproven_sync_span_canonical_itid"}
	}
	if candidate.OwnerIPIDKnown && (candidate.OwnerIPID < 0 || candidate.OwnerIPID > maxTraceDBInternalID) {
		return &traceDBOutputInvariantError{Reason: "invalid_sync_span_owner_ipid"}
	}
	if !candidate.OwnerIPIDKnown && candidate.OwnerIPID != 0 {
		return &traceDBOutputInvariantError{Reason: "unproven_sync_span_owner_ipid"}
	}
	if candidate.DepthKnown && (candidate.Depth < 0 || candidate.Depth > math.MaxInt32 ||
		candidate.DepthProvenance != traceDBSyncSpanDepthCallstack) {
		return &traceDBOutputInvariantError{Reason: "invalid_sync_span_depth"}
	}
	if !candidate.DepthKnown && (candidate.Depth != 0 || candidate.DepthProvenance != traceDBSyncSpanDepthUnknown) {
		return &traceDBOutputInvariantError{Reason: "unproven_sync_span_depth"}
	}
	if !traceDBSyncSpanCandidateProvenanceMatches(candidate) {
		return &traceDBOutputInvariantError{Reason: "sync_span_candidate_provenance_mismatch"}
	}
	// Validate both physical envelopes now, but do not publish either endpoint.
	markerPID := traceDBSyncSpanMarkerPID(candidate)
	if candidate.CPUPlacement != traceDBSyncSpanCPUPlacementKnown {
		if _, err := prepareTraceDBCPUUnavailableTraceMarkRow(candidate.Start, 0, candidate.Task,
			candidate.HeaderTID, candidate.HeaderTGID, markerPID, "B", candidate.Name, "",
			candidate.CPUPlacement); err != nil {
			return err
		}
		if _, err := prepareTraceDBCPUUnavailableTraceMarkRow(candidate.End, 1, candidate.Task,
			candidate.HeaderTID, candidate.HeaderTGID, markerPID, "E", "", "",
			candidate.CPUPlacement); err != nil {
			return err
		}
		return nil
	}
	if strings.ContainsRune(candidate.Name, '|') {
		if _, err := prepareTraceDBExactTraceMarkRow(candidate.Start, 0, candidate.Task, candidate.HeaderTID,
			candidate.HeaderTGID, candidate.StartCPU, markerPID, "B", candidate.Name, ""); err != nil {
			return err
		}
		if _, err := prepareTraceDBExactTraceMarkRow(candidate.End, 1, candidate.Task, candidate.HeaderTID,
			candidate.HeaderTGID, candidate.EndCPU, markerPID, "E", "", ""); err != nil {
			return err
		}
		return nil
	}
	if _, err := prepareTraceDBRenderedRow(candidate.Start, 0, candidate.Task, candidate.HeaderTID,
		candidate.HeaderTGID, candidate.StartCPU,
		fmt.Sprintf("tracing_mark_write: B|%d|%s", markerPID, candidate.Name)); err != nil {
		return err
	}
	if _, err := prepareTraceDBRenderedRow(candidate.End, 1, candidate.Task, candidate.HeaderTID,
		candidate.HeaderTGID, candidate.EndCPU,
		fmt.Sprintf("tracing_mark_write: E|%d|", markerPID)); err != nil {
		return err
	}
	return nil
}

func traceDBSyncSpanCandidateProvenanceMatches(candidate traceDBSyncSpanCandidate) bool {
	switch candidate.Producer {
	case traceDBSyncSpanProducerRegistration:
		return candidate.StableKind == traceDBSyncSpanStableRegistrationITID &&
			candidate.OwnerIPIDKnown &&
			!candidate.DepthKnown &&
			candidate.NameProvenance == traceDBSyncSpanNameRegistration &&
			candidate.StartCPUProvenance == traceDBSyncSpanCPURegistrationMetadata &&
			candidate.EndCPUProvenance == traceDBSyncSpanCPURegistrationMetadata &&
			candidate.CanonicalITIDKnown && candidate.CanonicalITID == candidate.StableID
	case traceDBSyncSpanProducerCallstack:
		return candidate.StableKind == traceDBSyncSpanStableCallstackRowID && candidate.StableID > 0 &&
			candidate.OwnerIPIDKnown &&
			candidate.NameProvenance == traceDBSyncSpanNameCallstack &&
			(candidate.CPUPlacement == traceDBSyncSpanCPUPlacementKnown &&
				candidate.StartCPUProvenance == traceDBSyncSpanCPUCallstackTypedRunning &&
				candidate.EndCPUProvenance == traceDBSyncSpanCPUCallstackTypedRunning ||
				candidate.CPUPlacement != traceDBSyncSpanCPUPlacementKnown &&
					candidate.StartCPUProvenance == traceDBSyncSpanCPUCallstackUnavailable &&
					candidate.EndCPUProvenance == traceDBSyncSpanCPUCallstackUnavailable) &&
			candidate.CanonicalITIDKnown && candidate.CanonicalITID > 0
	case traceDBSyncSpanProducerSyscall:
		return candidate.StableKind == traceDBSyncSpanStableSyscallRowID &&
			candidate.OwnerIPIDKnown &&
			!candidate.DepthKnown &&
			candidate.NameProvenance == traceDBSyncSpanNameSyscallNumber &&
			candidate.StartCPUProvenance == traceDBSyncSpanCPUSyscallTypedRunning &&
			candidate.EndCPUProvenance == traceDBSyncSpanCPUSyscallTypedRunning &&
			candidate.CanonicalITIDKnown && candidate.CanonicalITID > 0
	case traceDBSyncSpanProducerAppStartup:
		return candidate.StableKind == traceDBSyncSpanStableAppStartupRowID &&
			candidate.OwnerIPIDKnown &&
			!candidate.DepthKnown &&
			candidate.NameProvenance == traceDBSyncSpanNameAppStartupDictionary &&
			candidate.StartCPUProvenance == traceDBSyncSpanCPULegacyUnverified &&
			candidate.EndCPUProvenance == traceDBSyncSpanCPULegacyUnverified &&
			!candidate.CanonicalITIDKnown
	case traceDBSyncSpanProducerStaticInitialize:
		return candidate.StableKind == traceDBSyncSpanStableStaticInitializeRowID &&
			candidate.OwnerIPIDKnown &&
			!candidate.DepthKnown &&
			candidate.NameProvenance == traceDBSyncSpanNameStaticObject &&
			candidate.StartCPUProvenance == traceDBSyncSpanCPULegacyUnverified &&
			candidate.EndCPUProvenance == traceDBSyncSpanCPULegacyUnverified &&
			!candidate.CanonicalITIDKnown
	default:
		return false
	}
}

type traceDBSyncSpanLaneAuditReason uint8

const (
	traceDBSyncSpanLaneClean traceDBSyncSpanLaneAuditReason = iota
	traceDBSyncSpanLaneDeclaredPoison
	traceDBSyncSpanLaneCrossing
	traceDBSyncSpanLaneIdenticalUnproven
	traceDBSyncSpanLaneIdentityConflict
	traceDBSyncSpanLaneDepthConflict
	traceDBSyncSpanLaneDuplicateStableIdentity
)

func (authority *traceDBSyncSpanAuthority) finalize(ctx context.Context, sink *traceDBRowSink) (
	report traceDBSyncSpanReport, coverage TraceDBCoverage, err error,
) {
	started := time.Now()
	coverage = TraceDBCoverage{
		Family: "integrity", Table: "sync_span_authority", Role: "query_ready_export",
		FieldSources: map[string]string{
			"lane_identity":   "exact output artifact source + physical row-header TID; payload TGID, canonical ITID, producer and name never split the B/E stack",
			"candidate_order": "interval geometry, closed producer/stable kind, canonical/owner identity, exact depth tuple, then stable row identity; name never orders",
			"publication":     "bounded pass 1 freezes and audits every governed lane plus a checksummed bad-lane journal; pass 2 alone may publish clean synthetic B/E endpoints",
			"buffering": fmt.Sprintf(
				"hybrid candidate-byte-bounded memory to private indexed SQLite stage with record/temp/active/audit caps; final generic row sorter bounded at %d retained bytes/%d rows, %d input runs/%d run FDs, %d active/%d live temp bytes",
				defaultTraceDBRowBufferBytes, defaultTraceDBRowSinkThreshold,
				defaultTraceDBRowMergeFanIn, defaultTraceDBRowMergeFanIn+1,
				defaultTraceDBActiveTempBytes, defaultTraceDBLiveTempBytes),
		},
	}
	defer func() {
		stats := traceDBSyncSpanStageStats{}
		if authority != nil && authority.stage != nil {
			stats = authority.stage.snapshotStats()
			coverage.PeakBuffered = stats.PeakResidentCandidates
			coverage.SpillChunks = stats.ExternalArtifacts
			coverage.TempBytes = stats.PeakTempBytes
			coverage.FieldSources["stage_backend"] = fmt.Sprintf(
				"%s; peak_candidate_bytes=%d peak_active_depth=%d peak_active_bytes=%d audit_comparisons=%d indexed_lane_plan=%t",
				stats.Backend, stats.PeakResidentBytes, stats.PeakActiveDepth, stats.PeakActiveBytes,
				stats.AuditComparisons, stats.LanePlanVerified)
		}
		if cleanupErr := authority.cleanup(); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("cleanup sync span stage: %w", cleanupErr))
		}
		traceDBSetCoverageElapsed(&coverage, started)
		if err != nil {
			if authority != nil {
				authority.state = traceDBSyncSpanAuthorityFailed
			}
			coverage.Error = err.Error()
		}
	}()
	if authority == nil || authority.state != traceDBSyncSpanAuthorityOpen || authority.artifactSource == "" || authority.stage == nil || sink == nil || ctx == nil {
		return report, coverage, &traceDBOutputInvariantError{Reason: "sync_span_authority_finalize_state"}
	}
	authority.state = traceDBSyncSpanAuthorityFinalizing
	report = authority.baseReport()
	coverage.Found = report.SubmittedSpans > 0 || authority.poisonedTotal > 0 || authority.globalPoisonedTotal > 0
	if report.SubmittedSpans > math.MaxInt/2 {
		return report, coverage, &traceDBOutputInvariantError{Reason: "sync_span_authority_coverage_count_overflow"}
	}
	coverage.RowsRead = report.SubmittedSpans * 2
	if report.GlobalPoisoned {
		authority.applySourceFailClosed(&report, "unlocalizable_syscall_candidate")
		if reason := authority.stage.budget(); reason != "" {
			report.BudgetFailClosedReason = reason
		}
		coverage.Skipped = traceDBSyncSpanReportSummary(report)
		authority.state = traceDBSyncSpanAuthorityFinalized
		return report, coverage, nil
	}
	if reason := authority.stage.budget(); reason != "" {
		authority.applyBudgetFailClosed(&report, reason)
		coverage.Skipped = traceDBSyncSpanReportSummary(report)
		authority.state = traceDBSyncSpanAuthorityFinalized
		return report, coverage, nil
	}
	if sealErr := authority.stage.seal(ctx); sealErr != nil {
		if reason, ok := traceDBSyncSpanPureBudgetReason(sealErr); ok {
			authority.applyBudgetFailClosed(&report, reason)
			coverage.Skipped = traceDBSyncSpanReportSummary(report)
			authority.state = traceDBSyncSpanAuthorityFinalized
			return report, coverage, nil
		}
		return report, coverage, sealErr
	}
	journal, journalErr := authority.stage.newBadLaneJournal()
	if journalErr != nil {
		return report, coverage, journalErr
	}
	defer func() {
		if abortErr := journal.abort(); abortErr != nil {
			err = errors.Join(err, abortErr)
		}
	}()
	cleanSpans, auditErr := authority.auditFrozenLanes(ctx, &report, journal)
	if auditErr != nil {
		if reason, ok := traceDBSyncSpanPureBudgetReason(auditErr); ok {
			authority.applyBudgetFailClosed(&report, reason)
			coverage.Skipped = traceDBSyncSpanReportSummary(report)
			authority.state = traceDBSyncSpanAuthorityFinalized
			return report, coverage, nil
		}
		return report, coverage, auditErr
	}
	if journalErr := journal.seal(ctx); journalErr != nil {
		if reason, ok := traceDBSyncSpanPureBudgetReason(journalErr); ok {
			authority.applyBudgetFailClosed(&report, reason)
			coverage.Skipped = traceDBSyncSpanReportSummary(report)
			authority.state = traceDBSyncSpanAuthorityFinalized
			return report, coverage, nil
		}
		return report, coverage, journalErr
	}
	if sink.stats.RowsAccepted < 0 || cleanSpans > (math.MaxInt-sink.stats.RowsAccepted)/2 {
		sequenceErr := authority.stage.failBudget(traceDBSyncSpanStageBudgetSequenceCap)
		reason, _ := traceDBSyncSpanPureBudgetReason(sequenceErr)
		authority.applyBudgetFailClosed(&report, reason)
		coverage.Skipped = traceDBSyncSpanReportSummary(report)
		authority.state = traceDBSyncSpanAuthorityFinalized
		return report, coverage, nil
	}
	emittedByProducer, publishErr := authority.publishFrozenCleanLanes(ctx, sink, journal)
	if publishErr != nil {
		return report, coverage, publishErr
	}
	for producer := traceDBSyncSpanProducerRegistration; producer <= traceDBSyncSpanProducerStaticInitialize; producer++ {
		stats, exists := report.ByProducer[producer]
		if !exists {
			continue
		}
		stats.EmittedEndpoints = emittedByProducer[producer] * 2
		report.ByProducer[producer] = stats
		report.EmittedEndpoints += stats.EmittedEndpoints
	}
	if report.EmittedEndpoints != cleanSpans*2 {
		return report, coverage, &traceDBOutputInvariantError{Reason: "sync_span_stage_endpoint_count_mismatch"}
	}
	coverage.RowsEmitted = report.EmittedEndpoints
	coverage.Skipped = traceDBSyncSpanReportSummary(report)
	authority.state = traceDBSyncSpanAuthorityFinalized
	return report, coverage, nil
}

func (authority *traceDBSyncSpanAuthority) baseReport() traceDBSyncSpanReport {
	report := traceDBSyncSpanReport{ByProducer: map[traceDBSyncSpanProducer]traceDBSyncSpanProducerStats{}}
	for producer := traceDBSyncSpanProducerRegistration; producer <= traceDBSyncSpanProducerStaticInitialize; producer++ {
		submitted := authority.submitted[producer]
		poisoned := authority.poisoned[producer]
		globalPoisoned := authority.globalPoisoned[producer]
		if submitted == 0 && poisoned == 0 && !globalPoisoned {
			continue
		}
		globalDeclarations := 0
		if globalPoisoned {
			globalDeclarations = 1
		}
		report.ByProducer[producer] = traceDBSyncSpanProducerStats{
			SubmittedSpans: submitted, PoisonDeclarations: poisoned,
			GlobalPoisonDeclarations: globalDeclarations,
		}
	}
	report.SubmittedSpans = authority.submittedTotal
	report.GlobalPoisoned = authority.globalPoisonedTotal > 0
	return report
}

func (authority *traceDBSyncSpanAuthority) applySourceFailClosed(report *traceDBSyncSpanReport, reason string) {
	if report == nil {
		return
	}
	if strings.TrimSpace(reason) == "" {
		reason = "unknown_source"
	}
	report.SourceFailClosedReason = reason
	report.GlobalPoisoned = true
	report.EmittedEndpoints = 0
	report.SuppressedSpans = report.SubmittedSpans
	for producer, stats := range report.ByProducer {
		stats.EmittedEndpoints = 0
		stats.SuppressedSpans = stats.SubmittedSpans
		report.ByProducer[producer] = stats
	}
}

func (authority *traceDBSyncSpanAuthority) applyBudgetFailClosed(report *traceDBSyncSpanReport, reason string) {
	if report == nil {
		return
	}
	if strings.TrimSpace(reason) == "" {
		reason = "unknown_budget"
	}
	report.BudgetFailClosedReason = reason
	report.EmittedEndpoints = 0
	report.SuppressedSpans = report.SubmittedSpans
	report.PoisonedLanes = 0
	report.CrossingLanes = 0
	report.IdenticalLanes = 0
	report.IdentityLanes = 0
	report.DepthLanes = 0
	report.DuplicateLanes = 0
	for producer, stats := range report.ByProducer {
		stats.EmittedEndpoints = 0
		stats.SuppressedSpans = stats.SubmittedSpans
		report.ByProducer[producer] = stats
	}
}

func (authority *traceDBSyncSpanAuthority) auditFrozenLanes(
	ctx context.Context, report *traceDBSyncSpanReport, journal *traceDBSyncSpanBadLaneJournal,
) (cleanSpans int, err error) {
	candidates, err := authority.stage.candidateIterator(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { err = errors.Join(err, candidates.close()) }()
	forced, err := authority.stage.forcedIterator(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { err = errors.Join(err, forced.close()) }()

	nextCandidate, candidateOK, err := candidates.next(ctx)
	if err != nil {
		return 0, err
	}
	nextForced, forcedOK, err := forced.next(ctx)
	if err != nil {
		return 0, err
	}
	var previousCandidate traceDBSyncSpanStagedCandidate
	havePreviousCandidate := false
	previousForcedTID := int64(-1)
	auditor := traceDBSyncSpanLaneAuditor{stage: authority.stage}
	for candidateOK || forcedOK {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		tid := int64(math.MaxInt64)
		if candidateOK {
			tid = nextCandidate.Candidate.HeaderTID
		}
		if forcedOK && nextForced.HeaderTID < tid {
			tid = nextForced.HeaderTID
		}
		if tid == math.MaxInt64 {
			return 0, &traceDBOutputInvariantError{Reason: "sync_span_stage_lane_merge_stalled"}
		}
		auditor.reset()
		var laneCounts [traceDBSyncSpanProducerStaticInitialize + 1]int
		laneSpans := 0
		forcedMask := traceDBSyncSpanForcedNone
		if forcedOK && nextForced.HeaderTID == tid {
			if nextForced.HeaderTID <= previousForcedTID {
				return 0, &traceDBOutputInvariantError{Reason: "sync_span_stage_forced_iterator_order"}
			}
			previousForcedTID = nextForced.HeaderTID
			forcedMask = nextForced.Reason
			nextForced, forcedOK, err = forced.next(ctx)
			if err != nil {
				return 0, err
			}
		}
		producerPoisonMask := forcedMask & traceDBSyncSpanForcedCallstackPoison
		laneHasProducerPoison := producerPoisonMask != traceDBSyncSpanForcedNone
		if laneHasProducerPoison {
			report.PoisonedLanes++
		}
		for candidateOK && nextCandidate.Candidate.HeaderTID == tid {
			if havePreviousCandidate {
				if traceDBSyncSpanLaneCandidateLess(nextCandidate.Candidate, previousCandidate.Candidate) ||
					(!traceDBSyncSpanLaneCandidateLess(previousCandidate.Candidate, nextCandidate.Candidate) &&
						nextCandidate.Ordinal <= previousCandidate.Ordinal) {
					return 0, &traceDBOutputInvariantError{Reason: "sync_span_stage_candidate_iterator_order"}
				}
			}
			previousCandidate = nextCandidate
			havePreviousCandidate = true
			producer := nextCandidate.Candidate.Producer
			if traceDBSyncSpanProducerPoisoned(producerPoisonMask, producer) {
				stats := report.ByProducer[producer]
				stats.SuppressedSpans++
				report.ByProducer[producer] = stats
				report.SuppressedSpans++
			} else {
				laneCounts[producer]++
				laneSpans++
				if err := auditor.consume(nextCandidate.Candidate); err != nil {
					return 0, err
				}
			}
			nextCandidate, candidateOK, err = candidates.next(ctx)
			if err != nil {
				return 0, err
			}
		}
		forcedReason := traceDBSyncSpanForcedAuditReason(forcedMask)
		auditReason := auditor.finalReason()
		if forcedReason == traceDBSyncSpanLaneClean && auditReason == traceDBSyncSpanLaneClean {
			cleanSpans += laneSpans
			continue
		}
		if err := journal.add(ctx, tid); err != nil {
			return 0, err
		}
		if !laneHasProducerPoison {
			report.PoisonedLanes++
		}
		traceDBCountSyncSpanLaneReason(report, forcedReason)
		traceDBCountSyncSpanLaneReason(report, auditReason)
		for producer := traceDBSyncSpanProducerRegistration; producer <= traceDBSyncSpanProducerStaticInitialize; producer++ {
			count := laneCounts[producer]
			if count == 0 {
				continue
			}
			stats := report.ByProducer[producer]
			stats.SuppressedSpans += count
			report.ByProducer[producer] = stats
			report.SuppressedSpans += count
		}
	}
	return cleanSpans, nil
}

func traceDBSyncSpanForcedAuditReason(mask traceDBSyncSpanForcedReason) traceDBSyncSpanLaneAuditReason {
	if mask&traceDBSyncSpanForcedSyscallPoison != 0 {
		return traceDBSyncSpanLaneDeclaredPoison
	}
	if mask&traceDBSyncSpanForcedDuplicate != 0 {
		return traceDBSyncSpanLaneDuplicateStableIdentity
	}
	return traceDBSyncSpanLaneClean
}

func traceDBCountSyncSpanLaneReason(report *traceDBSyncSpanReport, reason traceDBSyncSpanLaneAuditReason) {
	switch reason {
	case traceDBSyncSpanLaneCrossing:
		report.CrossingLanes++
	case traceDBSyncSpanLaneIdenticalUnproven:
		report.IdenticalLanes++
	case traceDBSyncSpanLaneIdentityConflict:
		report.IdentityLanes++
	case traceDBSyncSpanLaneDepthConflict:
		report.DepthLanes++
	case traceDBSyncSpanLaneDuplicateStableIdentity:
		report.DuplicateLanes++
	}
}

const traceDBSyncSpanActiveCandidateBytes int64 = 256

type traceDBSyncSpanBoundedCandidateStack struct {
	stage        *traceDBSyncSpanStage
	frames       []traceDBSyncSpanCandidate
	payloadBytes int64
}

func (stack *traceDBSyncSpanBoundedCandidateStack) reset() {
	for index := range stack.frames {
		stack.frames[index] = traceDBSyncSpanCandidate{}
	}
	stack.frames = stack.frames[:0]
	stack.payloadBytes = 0
}

func (stack *traceDBSyncSpanBoundedCandidateStack) pop() traceDBSyncSpanCandidate {
	last := len(stack.frames) - 1
	candidate := stack.frames[last]
	stack.payloadBytes -= int64(len(candidate.Task)) + int64(len(candidate.Name))
	stack.frames[last] = traceDBSyncSpanCandidate{}
	stack.frames = stack.frames[:last]
	return candidate
}

func (stack *traceDBSyncSpanBoundedCandidateStack) push(candidate traceDBSyncSpanCandidate) error {
	depth := len(stack.frames) + 1
	payloadDelta := int64(len(candidate.Task)) + int64(len(candidate.Name))
	if stack.payloadBytes > math.MaxInt64-payloadDelta {
		return stack.stage.failBudget(traceDBSyncSpanStageBudgetActiveByteCap)
	}
	nextPayloadBytes := stack.payloadBytes + payloadDelta
	if depth > cap(stack.frames) {
		oldCapacity := cap(stack.frames)
		newCapacity := oldCapacity * 2
		if newCapacity < 8 {
			newCapacity = 8
		}
		if newCapacity < depth {
			newCapacity = depth
		}
		if newCapacity > stack.stage.options.MaxActiveDepth {
			newCapacity = stack.stage.options.MaxActiveDepth
		}
		maxByBytes64 := stack.stage.options.MaxActiveBytes / traceDBSyncSpanActiveCandidateBytes
		if maxByBytes64 > math.MaxInt {
			maxByBytes64 = math.MaxInt
		}
		maxByBytes := int(maxByBytes64)
		if newCapacity > maxByBytes {
			newCapacity = maxByBytes
		}
		if newCapacity < depth {
			activeBytes, ok := traceDBSyncSpanCheckedActiveBytes(depth, nextPayloadBytes)
			if !ok {
				return stack.stage.failBudget(traceDBSyncSpanStageBudgetActiveByteCap)
			}
			return stack.stage.noteActive(depth, activeBytes)
		}
		transientBytes, ok := traceDBSyncSpanCheckedActiveBytes(oldCapacity+newCapacity, nextPayloadBytes)
		if !ok {
			return stack.stage.failBudget(traceDBSyncSpanStageBudgetActiveByteCap)
		}
		if err := stack.stage.noteActive(depth, transientBytes); err != nil {
			return err
		}
		grown := make([]traceDBSyncSpanCandidate, len(stack.frames), newCapacity)
		copy(grown, stack.frames)
		stack.frames = grown
	} else {
		activeBytes, ok := traceDBSyncSpanCheckedActiveBytes(cap(stack.frames), nextPayloadBytes)
		if !ok {
			return stack.stage.failBudget(traceDBSyncSpanStageBudgetActiveByteCap)
		}
		if err := stack.stage.noteActive(depth, activeBytes); err != nil {
			return err
		}
	}
	stack.frames = append(stack.frames, candidate)
	stack.payloadBytes = nextPayloadBytes
	return nil
}

func traceDBSyncSpanCheckedActiveBytes(capacity int, payload int64) (int64, bool) {
	if capacity < 0 || payload < 0 || int64(capacity) > (math.MaxInt64-payload)/traceDBSyncSpanActiveCandidateBytes {
		return 0, false
	}
	return int64(capacity)*traceDBSyncSpanActiveCandidateBytes + payload, true
}

type traceDBSyncSpanLaneAuditor struct {
	stage      *traceDBSyncSpanStage
	stack      traceDBSyncSpanBoundedCandidateStack
	reason     traceDBSyncSpanLaneAuditReason
	zeroReason traceDBSyncSpanLaneAuditReason
}

func (auditor *traceDBSyncSpanLaneAuditor) reset() {
	auditor.stack.stage = auditor.stage
	auditor.stack.reset()
	auditor.reason = traceDBSyncSpanLaneClean
	auditor.zeroReason = traceDBSyncSpanLaneClean
}

func (auditor *traceDBSyncSpanLaneAuditor) consume(candidate traceDBSyncSpanCandidate) error {
	if auditor.reason != traceDBSyncSpanLaneClean {
		return nil
	}
	for len(auditor.stack.frames) > 0 && candidate.Start >= auditor.stack.frames[len(auditor.stack.frames)-1].End {
		auditor.stack.pop()
	}
	if candidate.Start == candidate.End {
		if auditor.zeroReason != traceDBSyncSpanLaneClean {
			return nil
		}
		for _, open := range auditor.stack.frames {
			if open.Start >= candidate.Start || candidate.Start >= open.End {
				continue
			}
			if err := auditor.stage.noteAuditComparison(); err != nil {
				return err
			}
			if traceDBSyncSpanIdentityConflicts(open, candidate) {
				auditor.zeroReason = traceDBSyncSpanLaneIdentityConflict
				return nil
			}
		}
		return nil
	}
	if len(auditor.stack.frames) > 0 && candidate.End > auditor.stack.frames[len(auditor.stack.frames)-1].End {
		auditor.reason = traceDBSyncSpanLaneCrossing
		return nil
	}
	for _, open := range auditor.stack.frames {
		if err := auditor.stage.noteAuditComparison(); err != nil {
			return err
		}
		if traceDBSyncSpanIdentityConflicts(open, candidate) {
			auditor.reason = traceDBSyncSpanLaneIdentityConflict
			return nil
		}
		identical := open.Start == candidate.Start && open.End == candidate.End
		if !identical && traceDBSyncSpanDepthComparable(open, candidate) && open.Depth >= candidate.Depth {
			auditor.reason = traceDBSyncSpanLaneDepthConflict
			return nil
		}
	}
	if len(auditor.stack.frames) > 0 {
		parent := auditor.stack.frames[len(auditor.stack.frames)-1]
		identical := parent.Start == candidate.Start && parent.End == candidate.End
		comparableDepth := traceDBSyncSpanDepthComparable(parent, candidate)
		if identical && (!comparableDepth || parent.Depth == candidate.Depth) {
			auditor.reason = traceDBSyncSpanLaneIdenticalUnproven
			return nil
		}
	}
	return auditor.stack.push(candidate)
}

func (auditor *traceDBSyncSpanLaneAuditor) finalReason() traceDBSyncSpanLaneAuditReason {
	if auditor.reason != traceDBSyncSpanLaneClean {
		return auditor.reason
	}
	return auditor.zeroReason
}

func (authority *traceDBSyncSpanAuthority) publishFrozenCleanLanes(
	ctx context.Context, sink *traceDBRowSink, journal *traceDBSyncSpanBadLaneJournal,
) (emitted [traceDBSyncSpanProducerStaticInitialize + 1]int, err error) {
	candidates, err := authority.stage.candidateIterator(ctx)
	if err != nil {
		return emitted, err
	}
	defer func() { err = errors.Join(err, candidates.close()) }()
	forced, err := authority.stage.forcedIterator(ctx)
	if err != nil {
		return emitted, err
	}
	defer func() { err = errors.Join(err, forced.close()) }()
	badLanes, err := journal.reader(ctx)
	if err != nil {
		return emitted, err
	}
	defer func() { err = errors.Join(err, badLanes.close()) }()
	nextBad, badOK, err := badLanes.next(ctx)
	if err != nil {
		return emitted, err
	}
	nextCandidate, candidateOK, err := candidates.next(ctx)
	if err != nil {
		return emitted, err
	}
	nextForced, forcedOK, err := forced.next(ctx)
	if err != nil {
		return emitted, err
	}
	stack := traceDBSyncSpanBoundedCandidateStack{stage: authority.stage}
	for candidateOK {
		tid := nextCandidate.Candidate.HeaderTID
		for badOK && nextBad < tid {
			nextBad, badOK, err = badLanes.next(ctx)
			if err != nil {
				return emitted, err
			}
		}
		bad := badOK && nextBad == tid
		for forcedOK && nextForced.HeaderTID < tid {
			nextForced, forcedOK, err = forced.next(ctx)
			if err != nil {
				return emitted, err
			}
		}
		producerPoisonMask := traceDBSyncSpanForcedNone
		if forcedOK && nextForced.HeaderTID == tid {
			producerPoisonMask = nextForced.Reason & traceDBSyncSpanForcedCallstackPoison
			nextForced, forcedOK, err = forced.next(ctx)
			if err != nil {
				return emitted, err
			}
		}
		stack.reset()
		for candidateOK && nextCandidate.Candidate.HeaderTID == tid {
			candidate := nextCandidate.Candidate
			if !bad && !traceDBSyncSpanProducerPoisoned(producerPoisonMask, candidate.Producer) {
				for len(stack.frames) > 0 && stack.frames[len(stack.frames)-1].End <= candidate.Start {
					if err := traceDBPublishSyncSpanEndpoint(sink, stack.pop(), false); err != nil {
						return emitted, err
					}
				}
				if candidate.Start == candidate.End {
					if err := traceDBPublishSyncSpanEndpoint(sink, candidate, true); err != nil {
						return emitted, err
					}
					if err := traceDBPublishSyncSpanEndpoint(sink, candidate, false); err != nil {
						return emitted, err
					}
				} else {
					if err := traceDBPublishSyncSpanEndpoint(sink, candidate, true); err != nil {
						return emitted, err
					}
					if err := stack.push(candidate); err != nil {
						return emitted, err
					}
				}
				emitted[candidate.Producer]++
			}
			nextCandidate, candidateOK, err = candidates.next(ctx)
			if err != nil {
				return emitted, err
			}
		}
		if !bad {
			for len(stack.frames) > 0 {
				if err := traceDBPublishSyncSpanEndpoint(sink, stack.pop(), false); err != nil {
					return emitted, err
				}
			}
		} else {
			nextBad, badOK, err = badLanes.next(ctx)
			if err != nil {
				return emitted, err
			}
		}
	}
	for badOK {
		nextBad, badOK, err = badLanes.next(ctx)
		if err != nil {
			return emitted, err
		}
	}
	return emitted, nil
}

func traceDBPublishSyncSpanEndpoint(sink *traceDBRowSink, candidate traceDBSyncSpanCandidate, begin bool) error {
	ts, cpu := candidate.End, candidate.EndCPU
	markerPID := traceDBSyncSpanMarkerPID(candidate)
	body := fmt.Sprintf("tracing_mark_write: E|%d|", markerPID)
	action, name := "E", ""
	if begin {
		ts, cpu = candidate.Start, candidate.StartCPU
		body = fmt.Sprintf("tracing_mark_write: B|%d|%s", markerPID, candidate.Name)
		action, name = "B", candidate.Name
	}
	if candidate.CPUPlacement != traceDBSyncSpanCPUPlacementKnown {
		row, err := prepareTraceDBCPUUnavailableTraceMarkRow(ts, sink.stats.RowsAccepted, candidate.Task,
			candidate.HeaderTID, candidate.HeaderTGID, markerPID, action, name, "", candidate.CPUPlacement)
		if err != nil {
			return err
		}
		return sink.add(row)
	}
	if strings.ContainsRune(candidate.Name, '|') {
		row, err := prepareTraceDBExactTraceMarkRow(ts, sink.stats.RowsAccepted, candidate.Task,
			candidate.HeaderTID, candidate.HeaderTGID, cpu, markerPID, action, name, "")
		if err != nil {
			return err
		}
		return sink.add(row)
	}
	row, err := prepareTraceDBRenderedRow(ts, sink.stats.RowsAccepted, candidate.Task,
		candidate.HeaderTID, candidate.HeaderTGID, cpu, body)
	if err != nil {
		return err
	}
	return sink.add(row)
}

func traceDBSyncSpanIdentityConflicts(left, right traceDBSyncSpanCandidate) bool {
	return left.HeaderTGID != right.HeaderTGID ||
		traceDBSyncSpanMarkerPID(left) != traceDBSyncSpanMarkerPID(right) ||
		(left.CanonicalITIDKnown && right.CanonicalITIDKnown && left.CanonicalITID != right.CanonicalITID) ||
		(left.OwnerIPIDKnown && right.OwnerIPIDKnown && left.OwnerIPID != right.OwnerIPID)
}

func traceDBSyncSpanMarkerPID(candidate traceDBSyncSpanCandidate) int64 {
	if candidate.MarkerPIDKnown {
		return candidate.MarkerPID
	}
	return candidate.HeaderTGID
}

func traceDBSyncSpanCPUUnavailableReason(placement traceDBSyncSpanCPUPlacement) string {
	switch placement {
	case traceDBSyncSpanCPUPlacementUnknownStart:
		return tracequery.TraceMarkCPUReasonUnknownStart
	case traceDBSyncSpanCPUPlacementUnknownEnd:
		return tracequery.TraceMarkCPUReasonUnknownEnd
	case traceDBSyncSpanCPUPlacementSourceTainted:
		return tracequery.TraceMarkCPUReasonSourceTainted
	case traceDBSyncSpanCPUPlacementLifecycleRejected:
		return tracequery.TraceMarkCPUReasonLifecycleRejected
	case traceDBSyncSpanCPUPlacementAliasAmbiguous:
		return tracequery.TraceMarkCPUReasonAliasAmbiguous
	default:
		return ""
	}
}

func traceDBSyncSpanDepthComparable(left, right traceDBSyncSpanCandidate) bool {
	return left.DepthKnown && right.DepthKnown &&
		left.DepthProvenance == traceDBSyncSpanDepthCallstack && right.DepthProvenance == traceDBSyncSpanDepthCallstack &&
		left.CanonicalITIDKnown && right.CanonicalITIDKnown && left.CanonicalITID == right.CanonicalITID
}

func traceDBSyncSpanStableLess(left, right traceDBSyncSpanCandidate) bool {
	if left.Producer != right.Producer {
		return left.Producer < right.Producer
	}
	if left.StableKind != right.StableKind {
		return left.StableKind < right.StableKind
	}
	return left.StableID < right.StableID
}

func traceDBSyncSpanReportSummary(report traceDBSyncSpanReport) string {
	counts := map[string]int{}
	parts := make([]string, 0, 2)
	if report.BudgetFailClosedReason != "" {
		parts = append(parts, "sync_family_budget_fail_closed="+report.BudgetFailClosedReason)
	}
	if report.SourceFailClosedReason != "" {
		parts = append(parts, "sync_family_source_fail_closed="+report.SourceFailClosedReason)
	}
	if report.SuppressedSpans > 0 {
		counts["suppressed_spans"] = report.SuppressedSpans
		counts["suppressed_endpoints"] = report.SuppressedSpans * 2
	}
	if report.PoisonedLanes > 0 {
		counts["poisoned_lanes"] = report.PoisonedLanes
	}
	if report.CrossingLanes > 0 {
		counts["crossing_lanes"] = report.CrossingLanes
	}
	if report.IdenticalLanes > 0 {
		counts["unproven_identical_lanes"] = report.IdenticalLanes
	}
	if report.IdentityLanes > 0 {
		counts["identity_conflict_lanes"] = report.IdentityLanes
	}
	if report.DepthLanes > 0 {
		counts["depth_conflict_lanes"] = report.DepthLanes
	}
	if report.DuplicateLanes > 0 {
		counts["duplicate_stable_identity_lanes"] = report.DuplicateLanes
	}
	if summary := traceDBCountSummary(counts); summary != "" {
		parts = append(parts, summary)
	}
	return strings.Join(parts, "; ")
}

func reconcileTraceDBSyncSpanCoverage(items []TraceDBCoverage, report traceDBSyncSpanReport) error {
	for producer, stats := range report.ByProducer {
		if stats.SubmittedSpans == 0 && stats.PoisonDeclarations == 0 && stats.GlobalPoisonDeclarations == 0 {
			continue
		}
		family, table, ok := traceDBSyncSpanProducerCoverageKey(producer)
		if !ok {
			return &traceDBOutputInvariantError{Reason: "unknown_sync_span_coverage_producer"}
		}
		match := -1
		for i := range items {
			if items[i].Family == family && items[i].Table == table {
				if match >= 0 {
					return &traceDBOutputInvariantError{Reason: "duplicate_sync_span_producer_coverage"}
				}
				match = i
			}
		}
		if match < 0 {
			return &traceDBOutputInvariantError{Reason: "missing_sync_span_producer_coverage"}
		}
		item := &items[match]
		item.RowsEmitted += stats.EmittedEndpoints
		traceDBAddCoverageMetric(item, "sync_spans_submitted", int64(stats.SubmittedSpans))
		traceDBAddCoverageMetric(item, "sync_spans_suppressed", int64(stats.SuppressedSpans))
		traceDBAddCoverageMetric(item, "sync_endpoints_emitted", int64(stats.EmittedEndpoints))
		if item.FieldSources == nil {
			item.FieldSources = map[string]string{}
		}
		item.FieldSources["wire_laminar"] = "single typed sync-span authority over output artifact source + physical header TID; finalized after all governed producers"
		if stats.SuppressedSpans > 0 {
			traceDBAppendCoverageSkipped(item, fmt.Sprintf(
				"sync_span_authority: suppressed_spans=%d suppressed_endpoints=%d",
				stats.SuppressedSpans, stats.SuppressedSpans*2))
		}
		if stats.PoisonDeclarations > 0 {
			traceDBAppendCoverageSkipped(item, fmt.Sprintf(
				"sync_span_authority: exact_lane_poison_declarations=%d", stats.PoisonDeclarations))
		}
		if stats.GlobalPoisonDeclarations > 0 {
			traceDBAppendCoverageSkipped(item, fmt.Sprintf(
				"sync_span_authority: global_poison_declarations=%d", stats.GlobalPoisonDeclarations))
		}
		if report.SourceFailClosedReason != "" {
			traceDBAppendCoverageSkipped(item,
				"sync_span_authority: sync_family_source_fail_closed="+report.SourceFailClosedReason)
		}
		if report.BudgetFailClosedReason != "" {
			traceDBAppendCoverageSkipped(item,
				"sync_span_authority: sync_family_budget_fail_closed="+report.BudgetFailClosedReason)
		}
	}
	return nil
}

func traceDBSyncSpanProducerCoverageKey(producer traceDBSyncSpanProducer) (string, string, bool) {
	switch producer {
	case traceDBSyncSpanProducerRegistration:
		return "metadata", "thread", true
	case traceDBSyncSpanProducerCallstack:
		return "slice", "callstack", true
	case traceDBSyncSpanProducerSyscall:
		return "slice", "syscall", true
	case traceDBSyncSpanProducerAppStartup:
		return "slice", "app_startup", true
	case traceDBSyncSpanProducerStaticInitialize:
		return "slice", "static_initalize", true
	default:
		return "", "", false
	}
}

func traceDBSyncSpanHiddenRowID(ctx context.Context, tdb *traceDB, coverage *TraceDBCoverage) (string, bool, error) {
	if coverage == nil || tdb == nil {
		return "", false, &traceDBOutputInvariantError{Reason: "missing_sync_span_stable_identity_context"}
	}
	if coverage.RowsRead == 0 {
		return "", false, nil
	}
	expr, source, err := traceDBHiddenRowIDExpr(ctx, tdb.db, coverage.Table)
	if err != nil {
		if coverage.FieldSources == nil {
			coverage.FieldSources = map[string]string{}
		}
		coverage.FieldSources["stable_identity"] = "unavailable: no provable SQLite hidden rowid; scan ordinals are forbidden"
		traceDBAppendCoverageSkipped(coverage,
			fmt.Sprintf("stable_row_identity_unavailable=%d", coverage.RowsRead))
		return "", false, nil
	}
	if coverage.FieldSources == nil {
		coverage.FieldSources = map[string]string{}
	}
	coverage.FieldSources["stable_identity"] = source + "; signed hidden rowid is used only for deterministic typed candidate identity/order"
	return expr, true, nil
}
