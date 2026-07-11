package hitraceconv

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"time"
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
	traceDBSyncSpanCPULegacyUnverified
)

type traceDBSyncSpanDepthProvenance uint8

const (
	traceDBSyncSpanDepthUnknown traceDBSyncSpanDepthProvenance = iota
	traceDBSyncSpanDepthCallstack
)

type traceDBSyncSpanCandidate struct {
	Producer           traceDBSyncSpanProducer
	StableKind         traceDBSyncSpanStableKind
	StableID           int64
	HeaderTID          int64
	HeaderTGID         int64
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
)

type traceDBSyncSpanLanePoison struct {
	Producer           traceDBSyncSpanProducer
	HeaderTID          int64
	CanonicalITID      int64
	CanonicalITIDKnown bool
	Reason             traceDBSyncSpanLanePoisonReason
}

type traceDBSyncSpanProducerStats struct {
	SubmittedSpans     int
	EmittedEndpoints   int
	SuppressedSpans    int
	PoisonDeclarations int
}

type traceDBSyncSpanReport struct {
	ByProducer       map[traceDBSyncSpanProducer]traceDBSyncSpanProducerStats
	SubmittedSpans   int
	EmittedEndpoints int
	SuppressedSpans  int
	PoisonedLanes    int
	CrossingLanes    int
	IdenticalLanes   int
	IdentityLanes    int
	DepthLanes       int
	DuplicateLanes   int
}

type traceDBSyncSpanAuthorityState uint8

const (
	traceDBSyncSpanAuthorityOpen traceDBSyncSpanAuthorityState = iota
	traceDBSyncSpanAuthorityFinalizing
	traceDBSyncSpanAuthorityFinalized
	traceDBSyncSpanAuthorityFailed
)

// traceDBSyncSpanAuthority is the sole authority for synthetic B/E rows made
// from one Trace Streamer SQLite artifact. It intentionally stages in memory;
// bounded external staging remains the separate B1-c batch.
type traceDBSyncSpanAuthority struct {
	artifactSource string
	state          traceDBSyncSpanAuthorityState
	candidates     []traceDBSyncSpanCandidate
	poisons        []traceDBSyncSpanLanePoison
	identities     map[traceDBSyncSpanIdentity]traceDBSyncSpanLane
	duplicateLanes map[traceDBSyncSpanLane]bool
}

func newTraceDBSyncSpanAuthority(outputArtifact string) (*traceDBSyncSpanAuthority, error) {
	trimmed := strings.TrimSpace(outputArtifact)
	if trimmed == "" {
		return nil, &traceDBOutputInvariantError{Reason: "missing_sync_span_artifact_source"}
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return nil, fmt.Errorf("resolve sync span artifact source: %w", err)
	}
	return &traceDBSyncSpanAuthority{
		artifactSource: filepath.Clean(abs),
		identities:     map[traceDBSyncSpanIdentity]traceDBSyncSpanLane{},
		duplicateLanes: map[traceDBSyncSpanLane]bool{},
	}, nil
}

func (authority *traceDBSyncSpanAuthority) submit(candidate traceDBSyncSpanCandidate) error {
	if authority == nil || authority.state != traceDBSyncSpanAuthorityOpen || authority.artifactSource == "" {
		return &traceDBOutputInvariantError{Reason: "sync_span_authority_not_open"}
	}
	if err := validateTraceDBSyncSpanCandidate(candidate); err != nil {
		return err
	}
	identity := traceDBSyncSpanIdentity{
		Producer: candidate.Producer, StableKind: candidate.StableKind, StableID: candidate.StableID,
	}
	lane := traceDBSyncSpanLane{ArtifactSource: authority.artifactSource, HeaderTID: candidate.HeaderTID}
	if previousLane, exists := authority.identities[identity]; exists {
		authority.duplicateLanes[previousLane] = true
		authority.duplicateLanes[lane] = true
	} else {
		authority.identities[identity] = lane
	}
	authority.candidates = append(authority.candidates, candidate)
	return nil
}

func (authority *traceDBSyncSpanAuthority) poisonExactLane(poison traceDBSyncSpanLanePoison) error {
	if authority == nil || authority.state != traceDBSyncSpanAuthorityOpen || authority.artifactSource == "" {
		return &traceDBOutputInvariantError{Reason: "sync_span_authority_not_open"}
	}
	if poison.Producer != traceDBSyncSpanProducerCallstack ||
		poison.Reason != traceDBSyncSpanLanePoisonRejectedCallstackCandidate ||
		poison.HeaderTID <= 0 || poison.HeaderTID > math.MaxInt32 ||
		!poison.CanonicalITIDKnown || poison.CanonicalITID <= 0 || poison.CanonicalITID > maxTraceDBInternalID {
		return &traceDBOutputInvariantError{Reason: "invalid_sync_span_exact_lane_poison"}
	}
	authority.poisons = append(authority.poisons, poison)
	return nil
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
	if !validTraceDBCPUIndex(candidate.StartCPU) || !validTraceDBCPUIndex(candidate.EndCPU) {
		return &traceDBOutputInvariantError{Reason: "invalid_cpu"}
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
	if candidate.HeaderTID == 0 && (candidate.Producer != traceDBSyncSpanProducerRegistration ||
		!candidate.CanonicalITIDKnown || candidate.CanonicalITID != 0 ||
		candidate.StableKind != traceDBSyncSpanStableRegistrationITID || candidate.StableID != 0) {
		return &traceDBOutputInvariantError{Reason: "unproven_sync_span_idle_subject"}
	}
	if !traceDBSinglePhysicalLine(candidate.Task, true) {
		return &traceDBOutputInvariantError{Reason: "invalid_task"}
	}
	if !traceDBCallstackMarkerToken(candidate.Name) {
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
	if _, err := prepareTraceDBRenderedRow(candidate.Start, 0, candidate.Task, candidate.HeaderTID,
		candidate.HeaderTGID, candidate.StartCPU,
		fmt.Sprintf("tracing_mark_write: B|%d|%s", candidate.HeaderTGID, candidate.Name)); err != nil {
		return err
	}
	if _, err := prepareTraceDBRenderedRow(candidate.End, 1, candidate.Task, candidate.HeaderTID,
		candidate.HeaderTGID, candidate.EndCPU,
		fmt.Sprintf("tracing_mark_write: E|%d|", candidate.HeaderTGID)); err != nil {
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
			candidate.StartCPUProvenance == traceDBSyncSpanCPUCallstackTypedRunning &&
			candidate.EndCPUProvenance == traceDBSyncSpanCPUCallstackTypedRunning &&
			candidate.CanonicalITIDKnown && candidate.CanonicalITID > 0
	case traceDBSyncSpanProducerSyscall:
		return candidate.StableKind == traceDBSyncSpanStableSyscallRowID &&
			candidate.OwnerIPIDKnown &&
			!candidate.DepthKnown &&
			candidate.NameProvenance == traceDBSyncSpanNameSyscallNumber &&
			candidate.StartCPUProvenance == traceDBSyncSpanCPULegacyUnverified &&
			candidate.EndCPUProvenance == traceDBSyncSpanCPULegacyUnverified &&
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
			"publication":     "all governed candidates are frozen and every lane is audited before any synthetic B/E endpoint reaches the row sink",
			"buffering":       "in-memory complete-set staging; bounded external stage/spill remains open as B1-c",
		},
	}
	defer func() {
		traceDBSetCoverageElapsed(&coverage, started)
		if err != nil {
			coverage.Error = err.Error()
		}
	}()
	if authority == nil || authority.state != traceDBSyncSpanAuthorityOpen || authority.artifactSource == "" || sink == nil {
		return report, coverage, &traceDBOutputInvariantError{Reason: "sync_span_authority_finalize_state"}
	}
	authority.state = traceDBSyncSpanAuthorityFinalizing
	report.ByProducer = map[traceDBSyncSpanProducer]traceDBSyncSpanProducerStats{}
	coverage.Found = len(authority.candidates) > 0 || len(authority.poisons) > 0
	coverage.RowsRead = len(authority.candidates) * 2
	coverage.PeakBuffered = len(authority.candidates)
	lanes := map[traceDBSyncSpanLane][]traceDBSyncSpanCandidate{}
	forced := map[traceDBSyncSpanLane]traceDBSyncSpanLaneAuditReason{}
	for _, candidate := range authority.candidates {
		if err := ctx.Err(); err != nil {
			authority.state = traceDBSyncSpanAuthorityFailed
			return report, coverage, err
		}
		lane := traceDBSyncSpanLane{ArtifactSource: authority.artifactSource, HeaderTID: candidate.HeaderTID}
		lanes[lane] = append(lanes[lane], candidate)
		stats := report.ByProducer[candidate.Producer]
		stats.SubmittedSpans++
		report.ByProducer[candidate.Producer] = stats
		report.SubmittedSpans++
	}
	for _, poison := range authority.poisons {
		lane := traceDBSyncSpanLane{ArtifactSource: authority.artifactSource, HeaderTID: poison.HeaderTID}
		forced[lane] = traceDBSyncSpanLaneDeclaredPoison
		stats := report.ByProducer[poison.Producer]
		stats.PoisonDeclarations++
		report.ByProducer[poison.Producer] = stats
	}
	for lane := range authority.duplicateLanes {
		if forced[lane] == traceDBSyncSpanLaneClean {
			forced[lane] = traceDBSyncSpanLaneDuplicateStableIdentity
		}
	}
	laneKeys := make([]traceDBSyncSpanLane, 0, len(lanes)+len(forced))
	seenLane := map[traceDBSyncSpanLane]bool{}
	for lane := range lanes {
		seenLane[lane] = true
		laneKeys = append(laneKeys, lane)
	}
	for lane := range forced {
		if !seenLane[lane] {
			laneKeys = append(laneKeys, lane)
		}
	}
	sort.Slice(laneKeys, func(i, j int) bool {
		if laneKeys[i].ArtifactSource != laneKeys[j].ArtifactSource {
			return laneKeys[i].ArtifactSource < laneKeys[j].ArtifactSource
		}
		return laneKeys[i].HeaderTID < laneKeys[j].HeaderTID
	})
	var clean []traceDBSyncSpanCandidate
	for _, lane := range laneKeys {
		forcedReason := forced[lane]
		auditReason := auditTraceDBSyncSpanLane(lanes[lane])
		if forcedReason == traceDBSyncSpanLaneClean && auditReason == traceDBSyncSpanLaneClean {
			clean = append(clean, lanes[lane]...)
			continue
		}
		report.PoisonedLanes++
		for _, reason := range []traceDBSyncSpanLaneAuditReason{forcedReason, auditReason} {
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
		for _, candidate := range lanes[lane] {
			stats := report.ByProducer[candidate.Producer]
			stats.SuppressedSpans++
			report.ByProducer[candidate.Producer] = stats
			report.SuppressedSpans++
		}
	}
	endpoints := traceDBSyncSpanEndpoints(clean)
	prepared := make([]renderedRow, 0, len(endpoints))
	for i, endpoint := range endpoints {
		if err := ctx.Err(); err != nil {
			authority.state = traceDBSyncSpanAuthorityFailed
			return report, coverage, err
		}
		body := fmt.Sprintf("tracing_mark_write: E|%d|", endpoint.Candidate.HeaderTGID)
		if endpoint.Begin {
			body = fmt.Sprintf("tracing_mark_write: B|%d|%s", endpoint.Candidate.HeaderTGID, endpoint.Candidate.Name)
		}
		cpu := endpoint.Candidate.EndCPU
		if endpoint.Begin {
			cpu = endpoint.Candidate.StartCPU
		}
		row, renderErr := prepareTraceDBRenderedRow(endpoint.TS, sink.stats.RowsAccepted+i,
			endpoint.Candidate.Task, endpoint.Candidate.HeaderTID, endpoint.Candidate.HeaderTGID, cpu, body)
		if renderErr != nil {
			authority.state = traceDBSyncSpanAuthorityFailed
			return report, coverage, renderErr
		}
		prepared = append(prepared, row)
	}
	for _, row := range prepared {
		if err := sink.add(row); err != nil {
			authority.state = traceDBSyncSpanAuthorityFailed
			return report, coverage, err
		}
	}
	for _, candidate := range clean {
		stats := report.ByProducer[candidate.Producer]
		stats.EmittedEndpoints += 2
		report.ByProducer[candidate.Producer] = stats
		report.EmittedEndpoints += 2
	}
	coverage.RowsEmitted = report.EmittedEndpoints
	coverage.Skipped = traceDBSyncSpanReportSummary(report)
	authority.state = traceDBSyncSpanAuthorityFinalized
	return report, coverage, nil
}

func auditTraceDBSyncSpanLane(candidates []traceDBSyncSpanCandidate) traceDBSyncSpanLaneAuditReason {
	positive := make([]traceDBSyncSpanCandidate, 0, len(candidates))
	zero := make([]traceDBSyncSpanCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.End > candidate.Start {
			positive = append(positive, candidate)
		} else {
			zero = append(zero, candidate)
		}
	}
	sort.Slice(positive, func(i, j int) bool { return traceDBSyncSpanCandidateLess(positive[i], positive[j]) })
	stack := make([]traceDBSyncSpanCandidate, 0, len(positive))
	for _, candidate := range positive {
		for len(stack) > 0 && candidate.Start >= stack[len(stack)-1].End {
			stack = stack[:len(stack)-1]
		}
		if len(stack) > 0 && candidate.End > stack[len(stack)-1].End {
			return traceDBSyncSpanLaneCrossing
		}
		for _, open := range stack {
			if traceDBSyncSpanIdentityConflicts(open, candidate) {
				return traceDBSyncSpanLaneIdentityConflict
			}
			identical := open.Start == candidate.Start && open.End == candidate.End
			if !identical && traceDBSyncSpanDepthComparable(open, candidate) && open.Depth >= candidate.Depth {
				return traceDBSyncSpanLaneDepthConflict
			}
		}
		if len(stack) > 0 {
			parent := stack[len(stack)-1]
			identical := parent.Start == candidate.Start && parent.End == candidate.End
			comparableDepth := traceDBSyncSpanDepthComparable(parent, candidate)
			if identical && (!comparableDepth || parent.Depth == candidate.Depth) {
				return traceDBSyncSpanLaneIdenticalUnproven
			}
		}
		stack = append(stack, candidate)
	}
	// Zero-duration pairs are atomic and never conflict with each other. At a
	// positive interval's exact start they publish before B; at its exact end
	// they publish after E. Strictly inside an open interval, however, their
	// B/E pair is physically nested and must retain the same proven identity.
	sort.Slice(zero, func(i, j int) bool { return traceDBSyncSpanCandidateLess(zero[i], zero[j]) })
	stack = stack[:0]
	positiveIndex := 0
	for _, point := range zero {
		for positiveIndex < len(positive) && positive[positiveIndex].Start < point.Start {
			candidate := positive[positiveIndex]
			for len(stack) > 0 && candidate.Start >= stack[len(stack)-1].End {
				stack = stack[:len(stack)-1]
			}
			stack = append(stack, candidate)
			positiveIndex++
		}
		for len(stack) > 0 && point.Start >= stack[len(stack)-1].End {
			stack = stack[:len(stack)-1]
		}
		for _, open := range stack {
			if open.Start < point.Start && point.Start < open.End && traceDBSyncSpanIdentityConflicts(open, point) {
				return traceDBSyncSpanLaneIdentityConflict
			}
		}
	}
	return traceDBSyncSpanLaneClean
}

func traceDBSyncSpanIdentityConflicts(left, right traceDBSyncSpanCandidate) bool {
	return left.HeaderTGID != right.HeaderTGID ||
		(left.CanonicalITIDKnown && right.CanonicalITIDKnown && left.CanonicalITID != right.CanonicalITID) ||
		(left.OwnerIPIDKnown && right.OwnerIPIDKnown && left.OwnerIPID != right.OwnerIPID)
}

func traceDBSyncSpanDepthComparable(left, right traceDBSyncSpanCandidate) bool {
	return left.DepthKnown && right.DepthKnown &&
		left.DepthProvenance == traceDBSyncSpanDepthCallstack && right.DepthProvenance == traceDBSyncSpanDepthCallstack &&
		left.CanonicalITIDKnown && right.CanonicalITIDKnown && left.CanonicalITID == right.CanonicalITID
}

func traceDBSyncSpanCandidateLess(left, right traceDBSyncSpanCandidate) bool {
	if left.Start != right.Start {
		return left.Start < right.Start
	}
	if left.End != right.End {
		return left.End > right.End
	}
	if left.Producer != right.Producer {
		return left.Producer < right.Producer
	}
	if left.StableKind != right.StableKind {
		return left.StableKind < right.StableKind
	}
	if left.CanonicalITIDKnown != right.CanonicalITIDKnown {
		return left.CanonicalITIDKnown
	}
	if left.CanonicalITID != right.CanonicalITID {
		return left.CanonicalITID < right.CanonicalITID
	}
	if left.OwnerIPIDKnown != right.OwnerIPIDKnown {
		return left.OwnerIPIDKnown
	}
	if left.OwnerIPID != right.OwnerIPID {
		return left.OwnerIPID < right.OwnerIPID
	}
	if left.DepthKnown != right.DepthKnown {
		return left.DepthKnown
	}
	if left.DepthProvenance != right.DepthProvenance {
		return left.DepthProvenance < right.DepthProvenance
	}
	if left.Depth != right.Depth {
		return left.Depth < right.Depth
	}
	return traceDBSyncSpanStableLess(left, right)
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

type traceDBSyncSpanEndpoint struct {
	Candidate traceDBSyncSpanCandidate
	TS        int64
	Begin     bool
	Zero      bool
}

func traceDBSyncSpanEndpoints(candidates []traceDBSyncSpanCandidate) []traceDBSyncSpanEndpoint {
	endpoints := make([]traceDBSyncSpanEndpoint, 0, len(candidates)*2)
	for _, candidate := range candidates {
		zero := candidate.Start == candidate.End
		endpoints = append(endpoints,
			traceDBSyncSpanEndpoint{Candidate: candidate, TS: candidate.Start, Begin: true, Zero: zero},
			traceDBSyncSpanEndpoint{Candidate: candidate, TS: candidate.End, Begin: false, Zero: zero},
		)
	}
	sort.SliceStable(endpoints, func(i, j int) bool {
		left, right := endpoints[i], endpoints[j]
		if left.TS != right.TS {
			return left.TS < right.TS
		}
		if left.Candidate.HeaderTID != right.Candidate.HeaderTID {
			return left.Candidate.HeaderTID < right.Candidate.HeaderTID
		}
		leftPhase, rightPhase := traceDBSyncSpanEndpointPhase(left), traceDBSyncSpanEndpointPhase(right)
		if leftPhase != rightPhase {
			return leftPhase < rightPhase
		}
		if left.Zero && right.Zero {
			if traceDBSyncSpanStableLess(left.Candidate, right.Candidate) {
				return true
			}
			if traceDBSyncSpanStableLess(right.Candidate, left.Candidate) {
				return false
			}
			return left.Begin && !right.Begin
		}
		if left.Begin && right.Begin {
			return traceDBSyncSpanCandidateLess(left.Candidate, right.Candidate)
		}
		if !left.Begin && !right.Begin {
			if left.Candidate.Start != right.Candidate.Start {
				return left.Candidate.Start > right.Candidate.Start
			}
			return traceDBSyncSpanCandidateLess(right.Candidate, left.Candidate)
		}
		return left.Begin && !right.Begin
	})
	return endpoints
}

func traceDBSyncSpanEndpointPhase(endpoint traceDBSyncSpanEndpoint) int {
	if endpoint.Zero {
		return 1
	}
	if !endpoint.Begin {
		return 0
	}
	return 2
}

func traceDBSyncSpanReportSummary(report traceDBSyncSpanReport) string {
	counts := map[string]int{}
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
	return traceDBCountSummary(counts)
}

func reconcileTraceDBSyncSpanCoverage(items []TraceDBCoverage, report traceDBSyncSpanReport) error {
	for producer, stats := range report.ByProducer {
		if stats.SubmittedSpans == 0 && stats.PoisonDeclarations == 0 {
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
