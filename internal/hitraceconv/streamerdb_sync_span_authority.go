package hitraceconv

import (
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"sort"
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
	traceDBSyncSpanProducerSourceRawMarker
)

type traceDBSyncSpanStableKind uint8

const (
	traceDBSyncSpanStableUnknown traceDBSyncSpanStableKind = iota
	traceDBSyncSpanStableRegistrationITID
	traceDBSyncSpanStableCallstackRowID
	traceDBSyncSpanStableSyscallRowID
	traceDBSyncSpanStableAppStartupRowID
	traceDBSyncSpanStableStaticInitializeRowID
	traceDBSyncSpanStableSourceRawOrdinal
)

type traceDBSyncSpanNameProvenance uint8

const (
	traceDBSyncSpanNameUnknown traceDBSyncSpanNameProvenance = iota
	traceDBSyncSpanNameRegistration
	traceDBSyncSpanNameCallstack
	traceDBSyncSpanNameSyscallNumber
	traceDBSyncSpanNameAppStartupDictionary
	traceDBSyncSpanNameStaticObject
	traceDBSyncSpanNameSourceRawMarker
)

type traceDBSyncSpanCPUProvenance uint8

const (
	traceDBSyncSpanCPUUnknown traceDBSyncSpanCPUProvenance = iota
	traceDBSyncSpanCPURegistrationMetadata
	traceDBSyncSpanCPUCallstackTypedRunning
	traceDBSyncSpanCPUSyscallTypedRunning
	traceDBSyncSpanCPULegacyUnverified
	traceDBSyncSpanCPUCallstackUnavailable
	traceDBSyncSpanCPUSourceRawPage
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

// traceDBSyncSpanViewerDisposition is decided only for a span which survived
// all shared authority suppression/supersession checks. It is an accounting
// result, never an alternate admission path.
type traceDBSyncSpanViewerDisposition uint8

const (
	traceDBSyncSpanViewerDispositionUnknown traceDBSyncSpanViewerDisposition = iota
	traceDBSyncSpanViewerDispositionStandard
	traceDBSyncSpanViewerDispositionCPUUnknownStart
	traceDBSyncSpanViewerDispositionCPUUnknownEnd
	traceDBSyncSpanViewerDispositionCPUSourceTainted
	traceDBSyncSpanViewerDispositionCPULifecycleRejected
	traceDBSyncSpanViewerDispositionCPUAliasAmbiguous
	traceDBSyncSpanViewerDispositionNameUnrepresentable
	traceDBSyncSpanViewerDispositionCount
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
	StartFlags         int64
	EndFlags           int64
	StartPreemptCount  int64
	EndPreemptCount    int64
	StartMarkerBody    string
	EndMarkerBody      string
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

type traceDBSyncSpanFenceKind uint8

const (
	traceDBSyncSpanFenceUnknown traceDBSyncSpanFenceKind = iota
	traceDBSyncSpanFenceInterval
	traceDBSyncSpanFenceSuffix
)

// traceDBSyncSpanLaneFence localizes rejected producer evidence on one
// physical B/E lane. Interval fences suppress only overlapping candidates;
// suffix fences are reserved for evidence whose start is known but whose end
// is not. A rejection without even a trusted timestamp must use lane poison.
type traceDBSyncSpanLaneFence struct {
	Producer           traceDBSyncSpanProducer
	HeaderTID          int64
	CanonicalITID      int64
	CanonicalITIDKnown bool
	Start              int64
	End                int64
	Kind               traceDBSyncSpanFenceKind
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
	SubmittedSpans                     int
	EmittedEndpoints                   int
	StandardViewerSpans                int
	StandardPipeSpans                  int
	ViewerDispositions                 [traceDBSyncSpanViewerDispositionCount]int
	ViewerDispositionWitnesses         [traceDBSyncSpanViewerDispositionCount][traceDBSyncSpanViewerDispositionWitnessCap]traceDBSyncSpanViewerDispositionWitness
	ViewerDispositionWitnessCounts     [traceDBSyncSpanViewerDispositionCount]int
	SuppressedSpans                    int
	SupersededSpans                    int
	SupersededCPUUnavailableSpans      int
	SupersededNameUnrepresentableSpans int
	PoisonDeclarations                 int
	FenceDeclarations                  int
	FenceSuppressedSpans               int
	GlobalPoisonDeclarations           int
}

type traceDBSyncSpanReport struct {
	ByProducer             map[traceDBSyncSpanProducer]traceDBSyncSpanProducerStats
	FenceWitnesses         [traceDBSyncSpanProducerSourceRawMarker + 1][]traceDBSyncSpanLaneFence
	SubmittedSpans         int
	EmittedEndpoints       int
	SuppressedSpans        int
	PoisonedLanes          int
	LocalizedFenceLanes    int
	CrossingLanes          int
	IdenticalLanes         int
	IdentityLanes          int
	DepthLanes             int
	DuplicateLanes         int
	BudgetFailClosedReason string
	SourceFailClosedReason string
	GlobalPoisoned         bool
}

type traceDBSyncSpanViewerDispositionWitness struct {
	StableKind         traceDBSyncSpanStableKind
	StableID           int64
	HeaderTID          int64
	HeaderTGID         int64
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
	StartCPUProvenance traceDBSyncSpanCPUProvenance
	EndCPUProvenance   traceDBSyncSpanCPUProvenance
	Name               string
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
	artifactSource                 string
	state                          traceDBSyncSpanAuthorityState
	stage                          *traceDBSyncSpanStage
	submitted                      [traceDBSyncSpanProducerSourceRawMarker + 1]int
	poisoned                       [traceDBSyncSpanProducerSourceRawMarker + 1]int
	fenced                         [traceDBSyncSpanProducerSourceRawMarker + 1]int
	fenceWitnesses                 [traceDBSyncSpanProducerSourceRawMarker + 1][]traceDBSyncSpanLaneFence
	standardPipeSpans              [traceDBSyncSpanProducerSourceRawMarker + 1]int
	standardViewerSpans            [traceDBSyncSpanProducerSourceRawMarker + 1]int
	viewerDispositions             [traceDBSyncSpanProducerSourceRawMarker + 1][traceDBSyncSpanViewerDispositionCount]int
	viewerDispositionWitnesses     [traceDBSyncSpanProducerSourceRawMarker + 1][traceDBSyncSpanViewerDispositionCount][traceDBSyncSpanViewerDispositionWitnessCap]traceDBSyncSpanViewerDispositionWitness
	viewerDispositionWitnessCounts [traceDBSyncSpanProducerSourceRawMarker + 1][traceDBSyncSpanViewerDispositionCount]int
	superseded                     [traceDBSyncSpanProducerSourceRawMarker + 1]int
	supersededCPUUnavailable       int
	supersededNameUnrepresentable  int
	globalPoisoned                 [traceDBSyncSpanProducerSourceRawMarker + 1]bool
	submittedTotal                 int
	poisonedTotal                  int
	fencedTotal                    int
	globalPoisonedTotal            int
	nullDurationHints              [traceDBCallstackNullDurationHintCap]traceDBCallstackNullDurationHint
	nullDurationHintCount          int
	nullDurationHintTotal          int
	nullDurationHintsComplete      bool
}

const traceDBSyncSpanFenceWitnessCap = 8
const traceDBCallstackNullDurationHintCap = 4096
const traceDBSyncSpanViewerDispositionWitnessCap = 4

// traceDBSyncSpanSemanticKey is an exact logical span identity used only to
// recognize a source-raw marker pair already represented by an earlier DB
// candidate. CPU and display task are intentionally excluded: they are
// placement/display evidence, not marker-stack identity.
type traceDBSyncSpanSemanticKey struct {
	HeaderTID     int64
	HeaderTGID    int64
	MarkerPID     int64
	CanonicalITID int64
	OwnerIPID     int64
	Start         int64
	End           int64
	Name          string
}

type traceDBSyncSpanIntervalCollisionCensus struct {
	Total                   int64
	CallstackCPUKnown       int64
	CallstackCPUUnavailable int64
}

type traceDBSyncSpanCandidateCollisionCensus struct {
	IntervalTotal                int64
	SemanticTotal                int64
	LocallyAdmittedSemanticTotal int64
	LocallyAdmittedInterval      traceDBSyncSpanIntervalCollisionCensus
}

// traceDBCallstackNullDurationHint is diagnostic-only evidence for one
// timestamp-known callstack row whose SQLite duration was NULL. It deliberately
// has no End: only a separately decoded exact raw B/E pair may provide that
// value in a later repair batch.
type traceDBCallstackNullDurationHint struct {
	RowID         int64
	HeaderTID     int64
	HeaderTGID    int64
	MarkerPID     int64
	CanonicalITID int64
	OwnerIPID     int64
	Start         int64
	Name          string
}

func (census traceDBSyncSpanIntervalCollisionCensus) Other() int64 {
	return census.Total - census.CallstackCPUKnown - census.CallstackCPUUnavailable
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
		artifactSource:            filepath.Clean(abs),
		stage:                     stage,
		nullDurationHintsComplete: true,
	}, nil
}

func (authority *traceDBSyncSpanAuthority) recordNullDurationHint(
	hint traceDBCallstackNullDurationHint,
) bool {
	if authority == nil || authority.state != traceDBSyncSpanAuthorityOpen ||
		authority.artifactSource == "" {
		return false
	}
	if hint.RowID <= 0 || hint.HeaderTID <= 0 || hint.HeaderTGID <= 0 ||
		hint.MarkerPID <= 0 || hint.MarkerPID > math.MaxInt32 ||
		hint.CanonicalITID <= 0 || hint.OwnerIPID <= 0 || hint.Start < 0 ||
		!traceDBCallstackSpanName(hint.Name) {
		authority.nullDurationHintsComplete = false
		return false
	}
	if authority.nullDurationHintTotal == math.MaxInt {
		authority.nullDurationHintsComplete = false
		return false
	}
	authority.nullDurationHintTotal++
	if authority.nullDurationHintCount >= traceDBCallstackNullDurationHintCap {
		authority.nullDurationHintsComplete = false
		return false
	}
	authority.nullDurationHints[authority.nullDurationHintCount] = hint
	authority.nullDurationHintCount++
	return true
}

func (authority *traceDBSyncSpanAuthority) nullDurationHintSnapshot() (
	[]traceDBCallstackNullDurationHint, int, bool, error,
) {
	if authority == nil || authority.state != traceDBSyncSpanAuthorityOpen ||
		authority.artifactSource == "" {
		return nil, 0, false,
			&traceDBOutputInvariantError{Reason: "sync_span_authority_not_open"}
	}
	out := append([]traceDBCallstackNullDurationHint(nil),
		authority.nullDurationHints[:authority.nullDurationHintCount]...)
	return out, authority.nullDurationHintTotal,
		authority.nullDurationHintsComplete &&
			authority.nullDurationHintTotal == authority.nullDurationHintCount, nil
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

func traceDBSyncSpanCandidateSemanticKey(candidate traceDBSyncSpanCandidate) (traceDBSyncSpanSemanticKey, bool) {
	if candidate.HeaderTID <= 0 || candidate.HeaderTGID <= 0 ||
		!candidate.CanonicalITIDKnown || candidate.CanonicalITID <= 0 ||
		!candidate.OwnerIPIDKnown || candidate.OwnerIPID <= 0 ||
		candidate.Start < 0 || candidate.End <= candidate.Start ||
		!traceDBCallstackSpanName(candidate.Name) {
		return traceDBSyncSpanSemanticKey{}, false
	}
	markerPID := traceDBSyncSpanMarkerPID(candidate)
	if markerPID <= 0 || markerPID > math.MaxInt32 {
		return traceDBSyncSpanSemanticKey{}, false
	}
	return traceDBSyncSpanSemanticKey{
		HeaderTID: candidate.HeaderTID, HeaderTGID: candidate.HeaderTGID,
		MarkerPID: markerPID, CanonicalITID: candidate.CanonicalITID,
		OwnerIPID: candidate.OwnerIPID, Start: candidate.Start, End: candidate.End,
		Name: candidate.Name,
	}, true
}

func (authority *traceDBSyncSpanAuthority) hasSemanticCandidate(
	ctx context.Context,
	candidate traceDBSyncSpanCandidate,
) (bool, bool, error) {
	if authority == nil || authority.state != traceDBSyncSpanAuthorityOpen ||
		authority.stage == nil {
		return false, false, &traceDBOutputInvariantError{Reason: "sync_span_authority_not_open"}
	}
	return authority.stage.hasSemanticCandidate(ctx, candidate)
}

func (authority *traceDBSyncSpanAuthority) hasIntervalIdentityCandidate(
	ctx context.Context,
	candidate traceDBSyncSpanCandidate,
) (bool, bool, error) {
	if authority == nil || authority.state != traceDBSyncSpanAuthorityOpen ||
		authority.stage == nil {
		return false, false, &traceDBOutputInvariantError{Reason: "sync_span_authority_not_open"}
	}
	return authority.stage.hasIntervalIdentityCandidate(ctx, candidate)
}

func (authority *traceDBSyncSpanAuthority) hasLocallyAdmittedSemanticCandidate(
	ctx context.Context,
	candidate traceDBSyncSpanCandidate,
) (bool, bool, error) {
	if authority == nil || authority.state != traceDBSyncSpanAuthorityOpen ||
		authority.stage == nil {
		return false, false, &traceDBOutputInvariantError{Reason: "sync_span_authority_not_open"}
	}
	return authority.stage.hasLocallyAdmittedSemanticCandidate(ctx, candidate)
}

func (authority *traceDBSyncSpanAuthority) hasLocallyAdmittedIntervalIdentityCandidate(
	ctx context.Context,
	candidate traceDBSyncSpanCandidate,
) (bool, bool, error) {
	if authority == nil || authority.state != traceDBSyncSpanAuthorityOpen ||
		authority.stage == nil {
		return false, false, &traceDBOutputInvariantError{Reason: "sync_span_authority_not_open"}
	}
	return authority.stage.hasLocallyAdmittedIntervalIdentityCandidate(ctx, candidate)
}

func (authority *traceDBSyncSpanAuthority) censusLocallyAdmittedIntervalIdentityCandidates(
	ctx context.Context,
	candidate traceDBSyncSpanCandidate,
) (traceDBSyncSpanIntervalCollisionCensus, bool, error) {
	if authority == nil || authority.state != traceDBSyncSpanAuthorityOpen ||
		authority.stage == nil {
		return traceDBSyncSpanIntervalCollisionCensus{}, false,
			&traceDBOutputInvariantError{Reason: "sync_span_authority_not_open"}
	}
	return authority.stage.censusLocallyAdmittedIntervalIdentityCandidates(ctx, candidate)
}

func (authority *traceDBSyncSpanAuthority) censusCandidateCollisions(
	ctx context.Context,
	candidate traceDBSyncSpanCandidate,
) (traceDBSyncSpanCandidateCollisionCensus, bool, error) {
	if authority == nil || authority.state != traceDBSyncSpanAuthorityOpen ||
		authority.stage == nil {
		return traceDBSyncSpanCandidateCollisionCensus{}, false,
			&traceDBOutputInvariantError{Reason: "sync_span_authority_not_open"}
	}
	return authority.stage.censusCandidateCollisions(ctx, candidate)
}

func (authority *traceDBSyncSpanAuthority) supersedeUniqueLocallyAdmittedCPUUnavailableCallstackCandidate(
	ctx context.Context,
	candidate traceDBSyncSpanCandidate,
) (bool, bool, error) {
	if authority == nil || authority.state != traceDBSyncSpanAuthorityOpen ||
		authority.stage == nil {
		return false, false,
			&traceDBOutputInvariantError{Reason: "sync_span_authority_not_open"}
	}
	if authority.superseded[traceDBSyncSpanProducerCallstack] == math.MaxInt {
		authority.state = traceDBSyncSpanAuthorityFailed
		return false, false,
			&traceDBOutputInvariantError{Reason: "sync_span_authority_supersede_count_overflow"}
	}
	replaced, complete, err :=
		authority.stage.supersedeUniqueLocallyAdmittedCPUUnavailableCallstackCandidate(
			ctx, candidate)
	if err != nil || !complete || !replaced {
		return replaced, complete, err
	}
	authority.superseded[traceDBSyncSpanProducerCallstack]++
	authority.supersededCPUUnavailable++
	return true, true, nil
}

func (authority *traceDBSyncSpanAuthority) supersedeUniqueLocallyAdmittedNameUnrepresentableCallstackCandidate(
	ctx context.Context,
	candidate traceDBSyncSpanCandidate,
) (bool, bool, error) {
	if authority == nil || authority.state != traceDBSyncSpanAuthorityOpen ||
		authority.stage == nil {
		return false, false,
			&traceDBOutputInvariantError{Reason: "sync_span_authority_not_open"}
	}
	if authority.superseded[traceDBSyncSpanProducerCallstack] == math.MaxInt ||
		authority.supersededNameUnrepresentable == math.MaxInt {
		authority.state = traceDBSyncSpanAuthorityFailed
		return false, false,
			&traceDBOutputInvariantError{Reason: "sync_span_authority_supersede_count_overflow"}
	}
	replaced, complete, err :=
		authority.stage.supersedeUniqueLocallyAdmittedNameUnrepresentableCallstackCandidate(
			ctx, candidate)
	if err != nil || !complete || !replaced {
		return replaced, complete, err
	}
	authority.superseded[traceDBSyncSpanProducerCallstack]++
	authority.supersededNameUnrepresentable++
	return true, true, nil
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

func (authority *traceDBSyncSpanAuthority) fenceExactLane(ctx context.Context, fence traceDBSyncSpanLaneFence) error {
	if authority == nil || authority.state != traceDBSyncSpanAuthorityOpen || authority.artifactSource == "" {
		return &traceDBOutputInvariantError{Reason: "sync_span_authority_not_open"}
	}
	if err := validateTraceDBSyncSpanLaneFence(fence); err != nil {
		return err
	}
	if authority.fenced[fence.Producer] == math.MaxInt || authority.fencedTotal == math.MaxInt {
		authority.state = traceDBSyncSpanAuthorityFailed
		return &traceDBOutputInvariantError{Reason: "sync_span_authority_fence_count_overflow"}
	}
	authority.fenced[fence.Producer]++
	authority.fencedTotal++
	if len(authority.fenceWitnesses[fence.Producer]) < traceDBSyncSpanFenceWitnessCap {
		authority.fenceWitnesses[fence.Producer] = append(
			authority.fenceWitnesses[fence.Producer], fence)
	}
	if err := authority.stage.addFence(ctx, fence); err != nil {
		if _, ok := traceDBSyncSpanPureBudgetReason(err); ok {
			return nil
		}
		authority.state = traceDBSyncSpanAuthorityFailed
		return err
	}
	return nil
}

func validateTraceDBSyncSpanLaneFence(fence traceDBSyncSpanLaneFence) error {
	closedReason := fence.Producer == traceDBSyncSpanProducerCallstack &&
		fence.Reason == traceDBSyncSpanLanePoisonRejectedCallstackCandidate
	validGeometry := fence.Kind == traceDBSyncSpanFenceInterval &&
		fence.Start >= 0 && fence.End > fence.Start ||
		fence.Kind == traceDBSyncSpanFenceSuffix &&
			fence.Start >= 0 && fence.End == 0
	if !closedReason || !validGeometry ||
		fence.HeaderTID <= 0 || fence.HeaderTID > math.MaxInt32 ||
		!fence.CanonicalITIDKnown || fence.CanonicalITID <= 0 || fence.CanonicalITID > maxTraceDBInternalID {
		return &traceDBOutputInvariantError{Reason: "invalid_sync_span_exact_lane_fence"}
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
	if candidate.Producer <= traceDBSyncSpanProducerUnknown || candidate.Producer > traceDBSyncSpanProducerSourceRawMarker {
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
	rawEnvelope := candidate.Producer == traceDBSyncSpanProducerSourceRawMarker
	if rawEnvelope {
		if candidate.StartFlags < 0 || candidate.StartFlags > math.MaxUint8 ||
			candidate.EndFlags < 0 || candidate.EndFlags > math.MaxUint8 ||
			candidate.StartPreemptCount < 0 || candidate.StartPreemptCount > math.MaxUint8 ||
			candidate.EndPreemptCount < 0 || candidate.EndPreemptCount > math.MaxUint8 {
			return &traceDBOutputInvariantError{Reason: "invalid_sync_span_raw_envelope"}
		}
		begin := tracequery.DecodeTraceMarkEndpointPayload(candidate.StartMarkerBody)
		end := tracequery.DecodeTraceMarkEndpointPayload(candidate.EndMarkerBody)
		markerPID := traceDBSyncSpanMarkerPID(candidate)
		if !traceDBSinglePhysicalLine(candidate.StartMarkerBody, false) ||
			!traceDBSinglePhysicalLine(candidate.EndMarkerBody, false) ||
			!begin.Admitted || begin.Action != "B" ||
			int64(begin.SpanPID) != markerPID ||
			begin.Name != candidate.Name ||
			!end.Admitted || end.Action != "E" ||
			int64(end.SpanPID) != markerPID {
			return &traceDBOutputInvariantError{Reason: "invalid_sync_span_raw_marker_body"}
		}
	} else if candidate.StartFlags != 0 || candidate.EndFlags != 0 ||
		candidate.StartPreemptCount != 0 || candidate.EndPreemptCount != 0 ||
		candidate.StartMarkerBody != "" || candidate.EndMarkerBody != "" {
		return &traceDBOutputInvariantError{Reason: "unproven_sync_span_raw_envelope"}
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
		if (candidate.Producer != traceDBSyncSpanProducerCallstack &&
			candidate.Producer != traceDBSyncSpanProducerSourceRawMarker) ||
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
	exactSourceName := (candidate.Producer == traceDBSyncSpanProducerCallstack &&
		candidate.NameProvenance == traceDBSyncSpanNameCallstack ||
		candidate.Producer == traceDBSyncSpanProducerSourceRawMarker &&
			candidate.NameProvenance == traceDBSyncSpanNameSourceRawMarker) &&
		traceDBCallstackSpanName(candidate.Name)
	if !traceDBCallstackMarkerToken(candidate.Name) && !exactSourceName {
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
	if strings.ContainsRune(candidate.Name, '|') && !traceDBStandardSyncPipeCandidate(candidate) {
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
	case traceDBSyncSpanProducerSourceRawMarker:
		return candidate.StableKind == traceDBSyncSpanStableSourceRawOrdinal &&
			candidate.StableID > 0 && candidate.OwnerIPIDKnown &&
			!candidate.DepthKnown &&
			candidate.NameProvenance == traceDBSyncSpanNameSourceRawMarker &&
			candidate.StartCPUProvenance == traceDBSyncSpanCPUSourceRawPage &&
			candidate.EndCPUProvenance == traceDBSyncSpanCPUSourceRawPage &&
			candidate.CPUPlacement == traceDBSyncSpanCPUPlacementKnown &&
			candidate.CanonicalITIDKnown && candidate.CanonicalITID > 0
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
			"rejection_fence": "producer-scoped exact intervals suppress only overlapping candidates; timestamp-only evidence suppresses that producer from the timestamp forward; only time-unlocalizable evidence poisons a full physical lane",
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
	coverage.Found = report.SubmittedSpans > 0 || authority.poisonedTotal > 0 ||
		authority.fencedTotal > 0 || authority.globalPoisonedTotal > 0
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
	for producer := traceDBSyncSpanProducerRegistration; producer <= traceDBSyncSpanProducerSourceRawMarker; producer++ {
		stats, exists := report.ByProducer[producer]
		if !exists {
			continue
		}
		stats.EmittedEndpoints = emittedByProducer[producer] * 2
		stats.StandardViewerSpans = authority.standardViewerSpans[producer]
		stats.StandardPipeSpans = authority.standardPipeSpans[producer]
		stats.ViewerDispositions = authority.viewerDispositions[producer]
		stats.ViewerDispositionWitnesses =
			authority.viewerDispositionWitnesses[producer]
		stats.ViewerDispositionWitnessCounts =
			authority.viewerDispositionWitnessCounts[producer]
		report.ByProducer[producer] = stats
		report.EmittedEndpoints += stats.EmittedEndpoints
	}
	if report.EmittedEndpoints != cleanSpans*2 {
		return report, coverage, &traceDBOutputInvariantError{Reason: "sync_span_stage_endpoint_count_mismatch"}
	}
	coverage.RowsEmitted = report.EmittedEndpoints
	standardPipeSpans := 0
	standardViewerSpans := 0
	for _, count := range authority.standardPipeSpans {
		standardPipeSpans += count
	}
	for _, count := range authority.standardViewerSpans {
		standardViewerSpans += count
	}
	traceDBAddCoverageMetric(&coverage, "official_viewer_standard_sync_spans_emitted",
		int64(standardViewerSpans))
	traceDBAddCoverageMetric(&coverage, "standard_sync_pipe_spans_emitted", int64(standardPipeSpans))
	coverage.Skipped = traceDBSyncSpanReportSummary(report)
	authority.state = traceDBSyncSpanAuthorityFinalized
	return report, coverage, nil
}

func (authority *traceDBSyncSpanAuthority) baseReport() traceDBSyncSpanReport {
	report := traceDBSyncSpanReport{ByProducer: map[traceDBSyncSpanProducer]traceDBSyncSpanProducerStats{}}
	for producer := traceDBSyncSpanProducerRegistration; producer <= traceDBSyncSpanProducerSourceRawMarker; producer++ {
		submitted := authority.submitted[producer]
		superseded := authority.superseded[producer]
		poisoned := authority.poisoned[producer]
		fenced := authority.fenced[producer]
		globalPoisoned := authority.globalPoisoned[producer]
		if submitted == 0 && poisoned == 0 && fenced == 0 && !globalPoisoned {
			continue
		}
		globalDeclarations := 0
		if globalPoisoned {
			globalDeclarations = 1
		}
		report.ByProducer[producer] = traceDBSyncSpanProducerStats{
			SubmittedSpans: submitted, SuppressedSpans: superseded,
			SupersededSpans: superseded, PoisonDeclarations: poisoned,
			FenceDeclarations:        fenced,
			GlobalPoisonDeclarations: globalDeclarations,
		}
		if producer == traceDBSyncSpanProducerCallstack {
			stats := report.ByProducer[producer]
			stats.SupersededCPUUnavailableSpans =
				authority.supersededCPUUnavailable
			stats.SupersededNameUnrepresentableSpans =
				authority.supersededNameUnrepresentable
			report.ByProducer[producer] = stats
		}
		report.FenceWitnesses[producer] = append(
			report.FenceWitnesses[producer][:0], authority.fenceWitnesses[producer]...)
		report.SuppressedSpans += superseded
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
		stats.FenceSuppressedSpans = 0
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
	report.LocalizedFenceLanes = 0
	report.CrossingLanes = 0
	report.IdenticalLanes = 0
	report.IdentityLanes = 0
	report.DepthLanes = 0
	report.DuplicateLanes = 0
	for producer, stats := range report.ByProducer {
		stats.EmittedEndpoints = 0
		stats.SuppressedSpans = stats.SubmittedSpans
		stats.FenceSuppressedSpans = 0
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
	fences, err := authority.stage.fenceIterator(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { err = errors.Join(err, fences.close()) }()

	nextCandidate, candidateOK, err := candidates.next(ctx)
	if err != nil {
		return 0, err
	}
	nextForced, forcedOK, err := forced.next(ctx)
	if err != nil {
		return 0, err
	}
	nextFence, fenceOK, err := fences.next(ctx)
	if err != nil {
		return 0, err
	}
	var previousCandidate traceDBSyncSpanStagedCandidate
	havePreviousCandidate := false
	var previousFence traceDBSyncSpanStagedFence
	havePreviousFence := false
	previousForcedTID := int64(-1)
	auditor := traceDBSyncSpanLaneAuditor{stage: authority.stage}
	for candidateOK || forcedOK || fenceOK {
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
		if fenceOK && nextFence.Fence.HeaderTID < tid {
			tid = nextFence.Fence.HeaderTID
		}
		if tid == math.MaxInt64 {
			return 0, &traceDBOutputInvariantError{Reason: "sync_span_stage_lane_merge_stalled"}
		}
		var laneCounts [traceDBSyncSpanProducerSourceRawMarker + 1]int
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
		laneFences := traceDBSyncSpanLaneFenceSet{stage: authority.stage}
		for fenceOK && nextFence.Fence.HeaderTID == tid {
			if havePreviousFence {
				if traceDBSyncSpanFenceLess(nextFence.Fence, previousFence.Fence) ||
					(!traceDBSyncSpanFenceLess(previousFence.Fence, nextFence.Fence) &&
						nextFence.Ordinal <= previousFence.Ordinal) {
					return 0, &traceDBOutputInvariantError{Reason: "sync_span_stage_fence_iterator_order"}
				}
			}
			previousFence = nextFence
			havePreviousFence = true
			if err := laneFences.add(nextFence.Fence); err != nil {
				return 0, err
			}
			nextFence, fenceOK, err = fences.next(ctx)
			if err != nil {
				return 0, err
			}
		}
		fenceDepth, fenceBytes := laneFences.activeDepthAndBytes()
		auditor.stack.baseDepth = fenceDepth
		auditor.stack.baseBytes = fenceBytes
		auditor.reset()
		if fenceDepth > 0 {
			report.LocalizedFenceLanes++
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
			} else if laneFences.affects(nextCandidate.Candidate) {
				stats := report.ByProducer[producer]
				stats.SuppressedSpans++
				stats.FenceSuppressedSpans++
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
		for producer := traceDBSyncSpanProducerRegistration; producer <= traceDBSyncSpanProducerSourceRawMarker; producer++ {
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

type traceDBSyncSpanFenceWindowInterval struct {
	Start int64
	End   int64
}

type traceDBSyncSpanProducerFenceWindow struct {
	Intervals   []traceDBSyncSpanFenceWindowInterval
	Suffix      int64
	SuffixKnown bool
}

type traceDBSyncSpanLaneFenceSet struct {
	stage   *traceDBSyncSpanStage
	windows [traceDBSyncSpanProducerSourceRawMarker + 1]traceDBSyncSpanProducerFenceWindow
	count   int
}

func (set *traceDBSyncSpanLaneFenceSet) add(fence traceDBSyncSpanLaneFence) error {
	if set == nil || set.stage == nil {
		return &traceDBOutputInvariantError{Reason: "missing_sync_span_lane_fence_set"}
	}
	if err := validateTraceDBSyncSpanLaneFence(fence); err != nil {
		return err
	}
	window := &set.windows[fence.Producer]
	switch fence.Kind {
	case traceDBSyncSpanFenceSuffix:
		if !window.SuffixKnown || fence.Start < window.Suffix {
			window.Suffix = fence.Start
			window.SuffixKnown = true
			trimmed := window.Intervals[:0]
			for _, interval := range window.Intervals {
				if interval.Start >= window.Suffix {
					continue
				}
				if interval.End > window.Suffix {
					interval.End = window.Suffix
				}
				if interval.End > interval.Start {
					trimmed = append(trimmed, interval)
				}
			}
			window.Intervals = trimmed
		}
	case traceDBSyncSpanFenceInterval:
		interval := traceDBSyncSpanFenceWindowInterval{Start: fence.Start, End: fence.End}
		if window.SuffixKnown {
			if interval.Start >= window.Suffix {
				return nil
			}
			if interval.End > window.Suffix {
				interval.End = window.Suffix
			}
		}
		if len(window.Intervals) > 0 {
			last := &window.Intervals[len(window.Intervals)-1]
			if interval.Start < last.Start {
				return &traceDBOutputInvariantError{Reason: "sync_span_stage_fence_iterator_order"}
			}
			if interval.Start <= last.End {
				if interval.End > last.End {
					last.End = interval.End
				}
				return nil
			}
		}
		if err := set.noteActiveCount(set.count + 1); err != nil {
			return err
		}
		window.Intervals = append(window.Intervals, interval)
	default:
		return &traceDBOutputInvariantError{Reason: "invalid_sync_span_lane_fence_kind"}
	}
	set.count = 0
	for producer := traceDBSyncSpanProducerRegistration; producer <= traceDBSyncSpanProducerSourceRawMarker; producer++ {
		set.count += len(set.windows[producer].Intervals)
		if set.windows[producer].SuffixKnown {
			set.count++
		}
	}
	return set.noteActiveCount(set.count)
}

func (set *traceDBSyncSpanLaneFenceSet) noteActiveCount(count int) error {
	const fenceActiveBytes int64 = 64
	if count < 0 || count > set.stage.options.MaxActiveDepth ||
		int64(count) > math.MaxInt64/fenceActiveBytes {
		return set.stage.failBudget(traceDBSyncSpanStageBudgetActiveDepthCap)
	}
	return set.stage.noteActive(count, int64(count)*fenceActiveBytes)
}

func (set *traceDBSyncSpanLaneFenceSet) affects(candidate traceDBSyncSpanCandidate) bool {
	if set == nil || candidate.Producer <= traceDBSyncSpanProducerUnknown ||
		candidate.Producer > traceDBSyncSpanProducerSourceRawMarker {
		return false
	}
	window := set.windows[candidate.Producer]
	if window.SuffixKnown {
		if candidate.Start == candidate.End {
			if candidate.Start >= window.Suffix {
				return true
			}
		} else if candidate.End > window.Suffix {
			return true
		}
	}
	index := sort.Search(len(window.Intervals), func(index int) bool {
		return window.Intervals[index].End > candidate.Start
	})
	if index >= len(window.Intervals) {
		return false
	}
	interval := window.Intervals[index]
	if candidate.Start == candidate.End {
		return interval.Start <= candidate.Start && candidate.Start < interval.End
	}
	return interval.Start < candidate.End
}

func (set *traceDBSyncSpanLaneFenceSet) activeDepthAndBytes() (int, int64) {
	if set == nil {
		return 0, 0
	}
	const fenceActiveBytes int64 = 64
	return set.count, int64(set.count) * fenceActiveBytes
}

const traceDBSyncSpanActiveCandidateBytes int64 = 256

type traceDBSyncSpanBoundedCandidateStack struct {
	stage        *traceDBSyncSpanStage
	frames       []traceDBSyncSpanCandidate
	payloadBytes int64
	baseDepth    int
	baseBytes    int64
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
	frameDepth := len(stack.frames) + 1
	if stack.baseDepth > stack.stage.options.MaxActiveDepth-frameDepth {
		return stack.stage.failBudget(traceDBSyncSpanStageBudgetActiveDepthCap)
	}
	depth := stack.baseDepth + frameDepth
	payloadDelta := int64(len(candidate.Task)) + int64(len(candidate.Name))
	if stack.payloadBytes > math.MaxInt64-payloadDelta {
		return stack.stage.failBudget(traceDBSyncSpanStageBudgetActiveByteCap)
	}
	nextPayloadBytes := stack.payloadBytes + payloadDelta
	if frameDepth > cap(stack.frames) {
		// Capacity applies only to candidate frames; depth includes the
		// already-resident merged fence windows.
		frameCapacity := frameDepth
		oldCapacity := cap(stack.frames)
		newCapacity := oldCapacity * 2
		if newCapacity < 8 {
			newCapacity = 8
		}
		if newCapacity < frameCapacity {
			newCapacity = frameCapacity
		}
		maxFrameDepth := stack.stage.options.MaxActiveDepth - stack.baseDepth
		if newCapacity > maxFrameDepth {
			newCapacity = maxFrameDepth
		}
		if stack.baseBytes > stack.stage.options.MaxActiveBytes {
			return stack.stage.failBudget(traceDBSyncSpanStageBudgetActiveByteCap)
		}
		maxByBytes64 := (stack.stage.options.MaxActiveBytes - stack.baseBytes) / traceDBSyncSpanActiveCandidateBytes
		if maxByBytes64 > math.MaxInt {
			maxByBytes64 = math.MaxInt
		}
		maxByBytes := int(maxByBytes64)
		if newCapacity > maxByBytes {
			newCapacity = maxByBytes
		}
		if newCapacity < frameCapacity {
			activeBytes, ok := stack.checkedActiveBytes(frameCapacity, nextPayloadBytes)
			if !ok {
				return stack.stage.failBudget(traceDBSyncSpanStageBudgetActiveByteCap)
			}
			return stack.stage.noteActive(depth, activeBytes)
		}
		transientBytes, ok := stack.checkedActiveBytes(oldCapacity+newCapacity, nextPayloadBytes)
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
		activeBytes, ok := stack.checkedActiveBytes(cap(stack.frames), nextPayloadBytes)
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

func (stack *traceDBSyncSpanBoundedCandidateStack) checkedActiveBytes(capacity int, payload int64) (int64, bool) {
	activeBytes, ok := traceDBSyncSpanCheckedActiveBytes(capacity, payload)
	if !ok || stack.baseBytes > math.MaxInt64-activeBytes {
		return 0, false
	}
	return stack.baseBytes + activeBytes, true
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
) (emitted [traceDBSyncSpanProducerSourceRawMarker + 1]int, err error) {
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
	fences, err := authority.stage.fenceIterator(ctx)
	if err != nil {
		return emitted, err
	}
	defer func() { err = errors.Join(err, fences.close()) }()
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
	nextFence, fenceOK, err := fences.next(ctx)
	if err != nil {
		return emitted, err
	}
	var previousFence traceDBSyncSpanStagedFence
	havePreviousFence := false
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
		for fenceOK && nextFence.Fence.HeaderTID < tid {
			if havePreviousFence {
				if traceDBSyncSpanFenceLess(nextFence.Fence, previousFence.Fence) ||
					(!traceDBSyncSpanFenceLess(previousFence.Fence, nextFence.Fence) &&
						nextFence.Ordinal <= previousFence.Ordinal) {
					return emitted, &traceDBOutputInvariantError{Reason: "sync_span_stage_fence_iterator_order"}
				}
			}
			previousFence = nextFence
			havePreviousFence = true
			nextFence, fenceOK, err = fences.next(ctx)
			if err != nil {
				return emitted, err
			}
		}
		laneFences := traceDBSyncSpanLaneFenceSet{stage: authority.stage}
		for fenceOK && nextFence.Fence.HeaderTID == tid {
			if havePreviousFence {
				if traceDBSyncSpanFenceLess(nextFence.Fence, previousFence.Fence) ||
					(!traceDBSyncSpanFenceLess(previousFence.Fence, nextFence.Fence) &&
						nextFence.Ordinal <= previousFence.Ordinal) {
					return emitted, &traceDBOutputInvariantError{Reason: "sync_span_stage_fence_iterator_order"}
				}
			}
			previousFence = nextFence
			havePreviousFence = true
			if err := laneFences.add(nextFence.Fence); err != nil {
				return emitted, err
			}
			nextFence, fenceOK, err = fences.next(ctx)
			if err != nil {
				return emitted, err
			}
		}
		stack.reset()
		stack.baseDepth, stack.baseBytes = laneFences.activeDepthAndBytes()
		for candidateOK && nextCandidate.Candidate.HeaderTID == tid {
			candidate := nextCandidate.Candidate
			if !bad && !traceDBSyncSpanProducerPoisoned(producerPoisonMask, candidate.Producer) &&
				!laneFences.affects(candidate) {
				viewerDisposition := traceDBSyncSpanViewerDispositionForCandidate(candidate)
				if viewerDisposition <= traceDBSyncSpanViewerDispositionUnknown ||
					viewerDisposition >= traceDBSyncSpanViewerDispositionCount {
					return emitted, &traceDBOutputInvariantError{
						Reason: "unknown_sync_span_viewer_disposition",
					}
				}
				authority.viewerDispositions[candidate.Producer][viewerDisposition]++
				if viewerDisposition != traceDBSyncSpanViewerDispositionStandard {
					authority.recordViewerDispositionWitness(
						candidate, viewerDisposition)
				}
				if viewerDisposition == traceDBSyncSpanViewerDispositionStandard {
					authority.standardViewerSpans[candidate.Producer]++
				}
				if traceDBStandardSyncPipeCandidate(candidate) {
					authority.standardPipeSpans[candidate.Producer]++
				}
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
	for fenceOK {
		if havePreviousFence {
			if traceDBSyncSpanFenceLess(nextFence.Fence, previousFence.Fence) ||
				(!traceDBSyncSpanFenceLess(previousFence.Fence, nextFence.Fence) &&
					nextFence.Ordinal <= previousFence.Ordinal) {
				return emitted, &traceDBOutputInvariantError{Reason: "sync_span_stage_fence_iterator_order"}
			}
		}
		previousFence = nextFence
		havePreviousFence = true
		nextFence, fenceOK, err = fences.next(ctx)
		if err != nil {
			return emitted, err
		}
	}
	return emitted, nil
}

func traceDBPublishSyncSpanEndpoint(sink *traceDBRowSink, candidate traceDBSyncSpanCandidate, begin bool) error {
	ts, cpu := candidate.End, candidate.EndCPU
	flags, preemptCount := candidate.EndFlags, candidate.EndPreemptCount
	markerPID := traceDBSyncSpanMarkerPID(candidate)
	body := fmt.Sprintf("tracing_mark_write: E|%d|", markerPID)
	action, name := "E", ""
	if begin {
		ts, cpu = candidate.Start, candidate.StartCPU
		flags, preemptCount = candidate.StartFlags, candidate.StartPreemptCount
		body = fmt.Sprintf("tracing_mark_write: B|%d|%s", markerPID, candidate.Name)
		action, name = "B", candidate.Name
	}
	if candidate.Producer == traceDBSyncSpanProducerSourceRawMarker {
		if begin {
			body = "tracing_mark_write: " + candidate.StartMarkerBody
		} else {
			body = "tracing_mark_write: " + candidate.EndMarkerBody
		}
	}
	if candidate.CPUPlacement != traceDBSyncSpanCPUPlacementKnown {
		row, err := prepareTraceDBCPUUnavailableTraceMarkRow(ts, sink.stats.RowsAccepted, candidate.Task,
			candidate.HeaderTID, candidate.HeaderTGID, markerPID, action, name, "", candidate.CPUPlacement)
		if err != nil {
			return err
		}
		return sink.add(row)
	}
	if candidate.Producer == traceDBSyncSpanProducerSourceRawMarker {
		row, err := prepareTraceDBRenderedRowEnvelope(
			ts, sink.stats.RowsAccepted, candidate.Task,
			candidate.HeaderTID, candidate.HeaderTGID, cpu,
			flags, preemptCount, false, body)
		if err != nil {
			return err
		}
		return sink.add(row)
	}
	if strings.ContainsRune(candidate.Name, '|') && !traceDBStandardSyncPipeCandidate(candidate) {
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

// traceDBStandardSyncPipeCandidate is the lossless compatibility subset for
// generic systrace consumers. The standard B payload must round-trip through
// the same Harmony trace-mark grammar without treating a trailing name
// component as metadata. Standard timestamps preserve nanoseconds when
// required. CPU-unavailable and async rows never reach this synchronous
// candidate authority.
func traceDBStandardSyncPipeCandidate(candidate traceDBSyncSpanCandidate) bool {
	if candidate.CPUPlacement != traceDBSyncSpanCPUPlacementKnown ||
		!strings.ContainsRune(candidate.Name, '|') ||
		candidate.Start < 0 || candidate.End < 0 {
		return false
	}
	markerPID := traceDBSyncSpanMarkerPID(candidate)
	if markerPID <= 0 || markerPID > math.MaxInt32 {
		return false
	}
	return tracequery.StandardSyncTraceMarkNameRepresentable(int(markerPID), candidate.Name)
}

func traceDBSyncSpanStandardViewerCandidate(candidate traceDBSyncSpanCandidate) bool {
	return traceDBSyncSpanViewerDispositionForCandidate(candidate) ==
		traceDBSyncSpanViewerDispositionStandard
}

func traceDBSyncSpanViewerDispositionForCandidate(
	candidate traceDBSyncSpanCandidate,
) traceDBSyncSpanViewerDisposition {
	switch candidate.CPUPlacement {
	case traceDBSyncSpanCPUPlacementUnknownStart:
		return traceDBSyncSpanViewerDispositionCPUUnknownStart
	case traceDBSyncSpanCPUPlacementUnknownEnd:
		return traceDBSyncSpanViewerDispositionCPUUnknownEnd
	case traceDBSyncSpanCPUPlacementSourceTainted:
		return traceDBSyncSpanViewerDispositionCPUSourceTainted
	case traceDBSyncSpanCPUPlacementLifecycleRejected:
		return traceDBSyncSpanViewerDispositionCPULifecycleRejected
	case traceDBSyncSpanCPUPlacementAliasAmbiguous:
		return traceDBSyncSpanViewerDispositionCPUAliasAmbiguous
	case traceDBSyncSpanCPUPlacementKnown:
		if candidate.Producer == traceDBSyncSpanProducerSourceRawMarker ||
			!strings.ContainsRune(candidate.Name, '|') ||
			traceDBStandardSyncPipeCandidate(candidate) {
			return traceDBSyncSpanViewerDispositionStandard
		}
		return traceDBSyncSpanViewerDispositionNameUnrepresentable
	default:
		return traceDBSyncSpanViewerDispositionUnknown
	}
}

func traceDBSyncSpanViewerDispositionMetric(
	disposition traceDBSyncSpanViewerDisposition,
) string {
	switch disposition {
	case traceDBSyncSpanViewerDispositionCPUUnknownStart:
		return "official_viewer_typed_only_sync_spans_cpu_unknown_start"
	case traceDBSyncSpanViewerDispositionCPUUnknownEnd:
		return "official_viewer_typed_only_sync_spans_cpu_unknown_end"
	case traceDBSyncSpanViewerDispositionCPUSourceTainted:
		return "official_viewer_typed_only_sync_spans_cpu_source_tainted"
	case traceDBSyncSpanViewerDispositionCPULifecycleRejected:
		return "official_viewer_typed_only_sync_spans_cpu_lifecycle_rejected"
	case traceDBSyncSpanViewerDispositionCPUAliasAmbiguous:
		return "official_viewer_typed_only_sync_spans_cpu_alias_ambiguous"
	case traceDBSyncSpanViewerDispositionNameUnrepresentable:
		return "official_viewer_typed_only_sync_spans_name_unrepresentable"
	default:
		return ""
	}
}

func traceDBSyncSpanViewerDispositionSuffix(
	disposition traceDBSyncSpanViewerDisposition,
) string {
	switch disposition {
	case traceDBSyncSpanViewerDispositionCPUUnknownStart:
		return "cpu_unknown_start"
	case traceDBSyncSpanViewerDispositionCPUUnknownEnd:
		return "cpu_unknown_end"
	case traceDBSyncSpanViewerDispositionCPUSourceTainted:
		return "cpu_source_tainted"
	case traceDBSyncSpanViewerDispositionCPULifecycleRejected:
		return "cpu_lifecycle_rejected"
	case traceDBSyncSpanViewerDispositionCPUAliasAmbiguous:
		return "cpu_alias_ambiguous"
	case traceDBSyncSpanViewerDispositionNameUnrepresentable:
		return "name_unrepresentable"
	default:
		return ""
	}
}

func (authority *traceDBSyncSpanAuthority) recordViewerDispositionWitness(
	candidate traceDBSyncSpanCandidate,
	disposition traceDBSyncSpanViewerDisposition,
) {
	if authority == nil ||
		candidate.Producer < traceDBSyncSpanProducerRegistration ||
		candidate.Producer > traceDBSyncSpanProducerSourceRawMarker ||
		disposition <= traceDBSyncSpanViewerDispositionStandard ||
		disposition >= traceDBSyncSpanViewerDispositionCount {
		return
	}
	count := authority.viewerDispositionWitnessCounts[candidate.Producer][disposition]
	if count >= traceDBSyncSpanViewerDispositionWitnessCap {
		return
	}
	authority.viewerDispositionWitnesses[candidate.Producer][disposition][count] =
		traceDBSyncSpanViewerDispositionWitness{
			StableKind: candidate.StableKind, StableID: candidate.StableID,
			HeaderTID: candidate.HeaderTID, HeaderTGID: candidate.HeaderTGID,
			MarkerPID:          traceDBSyncSpanMarkerPID(candidate),
			MarkerPIDKnown:     candidate.MarkerPIDKnown,
			CanonicalITID:      candidate.CanonicalITID,
			CanonicalITIDKnown: candidate.CanonicalITIDKnown,
			OwnerIPID:          candidate.OwnerIPID, OwnerIPIDKnown: candidate.OwnerIPIDKnown,
			Start: candidate.Start, End: candidate.End,
			StartCPU: candidate.StartCPU, EndCPU: candidate.EndCPU,
			StartCPUProvenance: candidate.StartCPUProvenance,
			EndCPUProvenance:   candidate.EndCPUProvenance,
			Name:               traceDBRawMarkerNameWitness(candidate.Name),
		}
	authority.viewerDispositionWitnessCounts[candidate.Producer][disposition]++
}

func traceDBSyncSpanStableKindLabel(kind traceDBSyncSpanStableKind) string {
	switch kind {
	case traceDBSyncSpanStableRegistrationITID:
		return "registration_itid"
	case traceDBSyncSpanStableCallstackRowID:
		return "callstack_row_id"
	case traceDBSyncSpanStableSyscallRowID:
		return "syscall_row_id"
	case traceDBSyncSpanStableAppStartupRowID:
		return "app_startup_row_id"
	case traceDBSyncSpanStableStaticInitializeRowID:
		return "static_initialize_row_id"
	case traceDBSyncSpanStableSourceRawOrdinal:
		return "source_raw_ordinal"
	default:
		return "unknown"
	}
}

func traceDBSyncSpanCPUProvenanceLabel(
	provenance traceDBSyncSpanCPUProvenance,
) string {
	switch provenance {
	case traceDBSyncSpanCPURegistrationMetadata:
		return "registration_metadata"
	case traceDBSyncSpanCPUCallstackTypedRunning:
		return "callstack_typed_running"
	case traceDBSyncSpanCPUSyscallTypedRunning:
		return "syscall_typed_running"
	case traceDBSyncSpanCPULegacyUnverified:
		return "legacy_unverified"
	case traceDBSyncSpanCPUCallstackUnavailable:
		return "callstack_unavailable"
	case traceDBSyncSpanCPUSourceRawPage:
		return "source_raw_page"
	default:
		return "unknown"
	}
}

func traceDBSyncSpanViewerDispositionWitnessString(
	disposition traceDBSyncSpanViewerDisposition,
	witness traceDBSyncSpanViewerDispositionWitness,
) string {
	return fmt.Sprintf(
		"reason=%s/stable_kind=%s/stable_id=%d/tid=%d/tgid=%d/marker_pid=%d/marker_pid_known=%t/itid=%d/itid_known=%t/ipid=%d/ipid_known=%t/start_ns=%d/end_ns=%d/start_cpu=%d/end_cpu=%d/start_cpu_source=%s/end_cpu_source=%s/name=%s",
		traceDBSyncSpanViewerDispositionSuffix(disposition),
		traceDBSyncSpanStableKindLabel(witness.StableKind), witness.StableID,
		witness.HeaderTID, witness.HeaderTGID, witness.MarkerPID,
		witness.MarkerPIDKnown, witness.CanonicalITID,
		witness.CanonicalITIDKnown, witness.OwnerIPID,
		witness.OwnerIPIDKnown, witness.Start, witness.End,
		witness.StartCPU, witness.EndCPU,
		traceDBSyncSpanCPUProvenanceLabel(witness.StartCPUProvenance),
		traceDBSyncSpanCPUProvenanceLabel(witness.EndCPUProvenance),
		witness.Name)
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
	supersededCPUUnavailable := 0
	supersededNameUnrepresentable := 0
	for _, stats := range report.ByProducer {
		supersededCPUUnavailable += stats.SupersededCPUUnavailableSpans
		supersededNameUnrepresentable +=
			stats.SupersededNameUnrepresentableSpans
	}
	if supersededCPUUnavailable > 0 {
		counts["superseded_by_raw_cpu_spans"] = supersededCPUUnavailable
	}
	if supersededNameUnrepresentable > 0 {
		counts["superseded_by_raw_name_spans"] =
			supersededNameUnrepresentable
	}
	if report.PoisonedLanes > 0 {
		counts["poisoned_lanes"] = report.PoisonedLanes
	}
	if report.LocalizedFenceLanes > 0 {
		counts["localized_fence_lanes"] = report.LocalizedFenceLanes
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
		if stats.SubmittedSpans == 0 && stats.PoisonDeclarations == 0 &&
			stats.FenceDeclarations == 0 && stats.GlobalPoisonDeclarations == 0 {
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
		if stats.SupersededSpans !=
			stats.SupersededCPUUnavailableSpans+
				stats.SupersededNameUnrepresentableSpans {
			return &traceDBOutputInvariantError{
				Reason: "sync_span_supersede_reason_census_mismatch",
			}
		}
		item := &items[match]
		item.RowsEmitted += stats.EmittedEndpoints
		traceDBAddCoverageMetric(item, "sync_spans_submitted", int64(stats.SubmittedSpans))
		traceDBAddCoverageMetric(item, "sync_spans_suppressed", int64(stats.SuppressedSpans))
		traceDBAddCoverageMetric(item, "sync_spans_superseded_by_raw_cpu",
			int64(stats.SupersededCPUUnavailableSpans))
		traceDBAddCoverageMetric(item,
			"sync_spans_superseded_by_raw_name_unrepresentable",
			int64(stats.SupersededNameUnrepresentableSpans))
		traceDBAddCoverageMetric(item, "sync_spans_suppressed_by_local_fence", int64(stats.FenceSuppressedSpans))
		traceDBAddCoverageMetric(item, "sync_endpoints_emitted", int64(stats.EmittedEndpoints))
		traceDBAddCoverageMetric(item, "official_viewer_standard_sync_spans_emitted",
			int64(stats.StandardViewerSpans))
		traceDBAddCoverageMetric(item, "standard_sync_pipe_spans_emitted", int64(stats.StandardPipeSpans))
		typedOnlyReasonTotal := 0
		for disposition := traceDBSyncSpanViewerDispositionCPUUnknownStart; disposition < traceDBSyncSpanViewerDispositionCount; disposition++ {
			count := stats.ViewerDispositions[disposition]
			typedOnlyReasonTotal += count
			traceDBAddCoverageMetric(item,
				traceDBSyncSpanViewerDispositionMetric(disposition), int64(count))
			wantWitnesses := count
			if wantWitnesses > traceDBSyncSpanViewerDispositionWitnessCap {
				wantWitnesses = traceDBSyncSpanViewerDispositionWitnessCap
			}
			if stats.ViewerDispositionWitnessCounts[disposition] != wantWitnesses {
				return &traceDBOutputInvariantError{
					Reason: "sync_span_viewer_disposition_witness_census_mismatch",
				}
			}
		}
		if stats.StandardViewerSpans+typedOnlyReasonTotal != stats.EmittedEndpoints/2 {
			return &traceDBOutputInvariantError{
				Reason: "sync_span_viewer_disposition_census_mismatch",
			}
		}
		if item.FieldSources == nil {
			item.FieldSources = map[string]string{}
		}
		item.FieldSources["wire_laminar"] = "single typed sync-span authority over output artifact source + physical header TID; finalized after all governed producers"
		item.FieldSources["viewer_visibility"] = "final emitted-span census after shared suppression and supersession; typed-only reasons are closed publication dispositions and never CPU/name admission authority"
		if item.Metadata == nil {
			item.Metadata = map[string]string{}
		}
		item.Metadata["official_viewer_typed_only_sync_reason_census"] = "complete"
		for disposition := traceDBSyncSpanViewerDispositionCPUUnknownStart; disposition < traceDBSyncSpanViewerDispositionCount; disposition++ {
			total := stats.ViewerDispositions[disposition]
			retained := stats.ViewerDispositionWitnessCounts[disposition]
			if total == 0 {
				continue
			}
			suffix := traceDBSyncSpanViewerDispositionSuffix(disposition)
			if suffix == "" {
				return &traceDBOutputInvariantError{
					Reason: "unknown_sync_span_viewer_disposition_witness",
				}
			}
			key := "official_viewer_typed_only_sync_witnesses_" + suffix
			formatted := make([]string, 0, retained)
			for index := 0; index < retained; index++ {
				formatted = append(formatted,
					traceDBSyncSpanViewerDispositionWitnessString(
						disposition,
						stats.ViewerDispositionWitnesses[disposition][index]))
			}
			item.Metadata[key] = strings.Join(formatted, ";")
			traceDBAddCoverageMetric(item, key+"_emitted", int64(retained))
			if omitted := total - retained; omitted > 0 {
				traceDBAddCoverageMetric(item, key+"_omitted", int64(omitted))
			}
			item.FieldSources[key] =
				"bounded exact final emitted typed-only sync-span dispositions after all shared suppression and supersession; diagnostic identity/interval/CPU provenance only, never alternate publication authority"
		}
		if stats.SuppressedSpans > 0 {
			traceDBAppendCoverageSkipped(item, fmt.Sprintf(
				"sync_span_authority: suppressed_spans=%d suppressed_endpoints=%d",
				stats.SuppressedSpans, stats.SuppressedSpans*2))
		}
		if stats.PoisonDeclarations > 0 {
			traceDBAppendCoverageSkipped(item, fmt.Sprintf(
				"sync_span_authority: exact_lane_poison_declarations=%d", stats.PoisonDeclarations))
		}
		if stats.FenceDeclarations > 0 {
			traceDBAppendCoverageSkipped(item, fmt.Sprintf(
				"sync_span_authority: localized_fence_declarations=%d suppressed_spans=%d",
				stats.FenceDeclarations, stats.FenceSuppressedSpans))
			witnesses := report.FenceWitnesses[producer]
			if len(witnesses) > 0 {
				if item.Metadata == nil {
					item.Metadata = map[string]string{}
				}
				formatted := make([]string, 0, len(witnesses))
				for _, witness := range witnesses {
					kind := "suffix"
					if witness.Kind == traceDBSyncSpanFenceInterval {
						kind = "interval"
					}
					formatted = append(formatted, fmt.Sprintf(
						"tid=%d/itid=%d/kind=%s/start_ns=%d/end_ns=%d/reason=rejected_callstack_candidate",
						witness.HeaderTID, witness.CanonicalITID, kind,
						witness.Start, witness.End))
				}
				item.Metadata["localized_fence_witnesses"] = strings.Join(formatted, ",")
				traceDBAddCoverageMetric(item, "localized_fence_witnesses_emitted",
					int64(len(witnesses)))
				if omitted := stats.FenceDeclarations - len(witnesses); omitted > 0 {
					traceDBAddCoverageMetric(item, "localized_fence_witnesses_omitted",
						int64(omitted))
				}
				item.FieldSources["localized_fence_witnesses"] =
					"bounded exact rejected callstack fence declarations only; diagnostic geometry, never alternate span/CPU/identity authority"
			}
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
	case traceDBSyncSpanProducerSourceRawMarker:
		return "source_rawtrace_marker_sync", "__raw_marker_sync__", true
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
