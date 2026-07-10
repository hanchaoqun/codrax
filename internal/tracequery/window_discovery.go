package tracequery

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math"
	"sort"
	"strings"
)

// WindowDiscoveryStrategy is the closed deterministic discovery registry.
// Strategies project their private observations into the same typed
// DiscoveredWindow contract; downstream collectors never parse prose or an
// arbitrary tracequery Result field.
type WindowDiscoveryStrategy string

const (
	WindowDiscoveryPairingIntegrity WindowDiscoveryStrategy = "pairing_integrity"
	WindowDiscoveryTraceMarkCarry   WindowDiscoveryStrategy = "trace_mark_carry"

	DefaultWindowDiscoveryMaxWindows       = 3
	HardWindowDiscoveryMaxWindows          = 8
	DefaultWindowDiscoveryMaxWindowMs      = 50.0
	HardPairingDiscoveryMaxWindowMs        = 50.0
	DefaultWindowDiscoveryPaddingMs        = 5.0
	DefaultWindowDiscoveryEndpointLimit    = 2_000_000
	HardWindowDiscoveryEndpointLimit       = 10_000_000
	DefaultWindowDiscoveryActiveLaneLimit  = 65_536
	HardWindowDiscoveryActiveLaneLimit     = 262_144
	DefaultWindowDiscoveryCohortEventLimit = 2_048
	HardWindowDiscoveryCohortEventLimit    = 16_384

	windowDiscoveryCandidatePoolLimit   = 128
	windowDiscoveryCandidateReportLimit = 16
)

type WindowDiscoveryFamily string

const (
	WindowDiscoveryFamilyBlock      WindowDiscoveryFamily = "block"
	WindowDiscoveryFamilyStorage    WindowDiscoveryFamily = "storage"
	WindowDiscoveryFamilyTraceSync  WindowDiscoveryFamily = "trace_sync"
	WindowDiscoveryFamilyTraceAsync WindowDiscoveryFamily = "trace_async"
	WindowDiscoveryFamilyTraceTrack WindowDiscoveryFamily = "trace_track"
)

func WindowDiscoveryStrategyNames() []string {
	return []string{string(WindowDiscoveryPairingIntegrity), string(WindowDiscoveryTraceMarkCarry)}
}

func WindowDiscoveryFamilyNames(strategy WindowDiscoveryStrategy) []string {
	switch strategy {
	case WindowDiscoveryPairingIntegrity:
		return []string{string(WindowDiscoveryFamilyBlock), string(WindowDiscoveryFamilyStorage)}
	case WindowDiscoveryTraceMarkCarry:
		return []string{string(WindowDiscoveryFamilyTraceSync), string(WindowDiscoveryFamilyTraceAsync), string(WindowDiscoveryFamilyTraceTrack)}
	default:
		return nil
	}
}

// WindowDiscoveryPairingStatus is the closed endpoint-state verdict carried
// by both candidate diagnostics and selected windows. Only PairingCompleteExact
// may produce a DiscoveredWindow; every other value is a typed fail-closed
// explanation and never becomes a collection scope.
type WindowDiscoveryPairingStatus string

const (
	WindowDiscoveryPairingCompleteExact      WindowDiscoveryPairingStatus = "complete_exact"
	WindowDiscoveryPairingAmbiguousDuplicate WindowDiscoveryPairingStatus = "ambiguous_duplicate"
	WindowDiscoveryPairingIncompleteOpen     WindowDiscoveryPairingStatus = "incomplete_open"
	WindowDiscoveryPairingOrphanEnd          WindowDiscoveryPairingStatus = "orphan_end"
	WindowDiscoveryPairingLifecycleCut       WindowDiscoveryPairingStatus = "lifecycle_cut"
	WindowDiscoveryPairingMalformedEndpoint  WindowDiscoveryPairingStatus = "malformed_endpoint"
	WindowDiscoveryPairingTimestampRollback  WindowDiscoveryPairingStatus = "timestamp_rollback"
	WindowDiscoveryPairingSourceUnresolved   WindowDiscoveryPairingStatus = "source_unresolved"
	WindowDiscoveryPairingBudgetExceeded     WindowDiscoveryPairingStatus = "budget_exceeded"
)

// WindowDiscoveryCarryClass is the exact relationship between a complete
// endpoint pair and the operator's parent scope. It is descriptive collection
// provenance only; it never asserts causality or rank.
type WindowDiscoveryCarryClass string

const (
	WindowDiscoveryCarryIn      WindowDiscoveryCarryClass = "carry_in"
	WindowDiscoveryCarryOut     WindowDiscoveryCarryClass = "carry_out"
	WindowDiscoveryCarryThrough WindowDiscoveryCarryClass = "carry_through"
	WindowDiscoveryInsidePair   WindowDiscoveryCarryClass = "inside_pair"
)

// WindowDiscoveryEndpointProvenance preserves one exact physical marker
// endpoint. Generation is lifecycle-derived; SourcePath+Line is the physical
// identity authority, while emitter/payload identities remain separate.
type WindowDiscoveryEndpointProvenance struct {
	Action     string  `json:"action"`
	SourcePath string  `json:"source_path"`
	Line       int     `json:"line"`
	Ts         float64 `json:"ts"`
	EmitterPID int     `json:"emitter_pid"`
	PayloadPID int     `json:"payload_pid,omitempty"`
	Generation int     `json:"generation"`
	Name       string  `json:"name,omitempty"`
	Track      string  `json:"track,omitempty"`
	Cookie     string  `json:"cookie,omitempty"`
	RawEvent   string  `json:"raw_event,omitempty"`
}

type WindowDiscoveryRequest struct {
	Strategy WindowDiscoveryStrategy
	Families []WindowDiscoveryFamily

	TimeStart    float64
	TimeEnd      float64
	TimeStartSet bool
	TimeEndSet   bool
	LineStart    int
	LineEnd      int

	MaxWindows       int
	MaxWindowMs      float64
	PaddingMs        float64
	EndpointLimit    int
	ActiveLaneLimit  int
	CohortEventLimit int
}

type WindowDiscoveryFamilyStats struct {
	Family                   WindowDiscoveryFamily `json:"family"`
	EndpointCount            int                   `json:"endpoint_count"`
	StartCount               int                   `json:"start_count"`
	DoneCount                int                   `json:"done_count"`
	ScopedEndpointCount      int                   `json:"scoped_endpoint_count"`
	ScopedStartCount         int                   `json:"scoped_start_count"`
	ScopedDoneCount          int                   `json:"scoped_done_count"`
	InvalidIdentityCount     int                   `json:"invalid_identity_count,omitempty"`
	UnpairedDoneCount        int                   `json:"unpaired_done_count,omitempty"`
	CompletedPairCount       int                   `json:"completed_pair_count,omitempty"`
	ClosedAmbiguousCount     int                   `json:"closed_ambiguous_count,omitempty"`
	OpenAmbiguousCount       int                   `json:"open_ambiguous_count,omitempty"`
	OpenSingleCount          int                   `json:"open_single_count,omitempty"`
	LifecycleResetLaneCount  int                   `json:"lifecycle_reset_lane_count,omitempty"`
	TimestampRollbackCount   int                   `json:"timestamp_rollback_count,omitempty"`
	CohortEventOverflowCount int                   `json:"cohort_event_overflow_count,omitempty"`
}

type WindowDiscoveryCandidate struct {
	Rank                    int                                `json:"rank"`
	Family                  WindowDiscoveryFamily              `json:"family"`
	Kind                    string                             `json:"kind"`
	Identity                string                             `json:"identity"`
	IdentityFingerprint     string                             `json:"identity_fingerprint"`
	FirstLine               int                                `json:"first_line"`
	LastLine                int                                `json:"last_line"`
	CoreStartTs             float64                            `json:"core_start_ts"`
	CoreEndTs               float64                            `json:"core_end_ts"`
	EndpointCount           int                                `json:"endpoint_count"`
	StartCount              int                                `json:"start_count"`
	DoneCount               int                                `json:"done_count"`
	MaxDepth                int                                `json:"max_depth"`
	Closed                  bool                               `json:"closed"`
	CollectionComplete      bool                               `json:"collection_complete"`
	FitsSingleWindow        bool                               `json:"fits_single_window"`
	RequiredWindowCount     int                                `json:"required_window_count,omitempty"`
	Selected                bool                               `json:"selected"`
	SelectionReason         string                             `json:"selection_reason,omitempty"`
	CollectionBlockedReason string                             `json:"collection_blocked_reason,omitempty"`
	PairingStatus           WindowDiscoveryPairingStatus       `json:"pairing_status,omitempty"`
	CarryClass              WindowDiscoveryCarryClass          `json:"carry_class,omitempty"`
	SemanticClass           string                             `json:"semantic_class,omitempty"`
	StartEndpoint           *WindowDiscoveryEndpointProvenance `json:"start_endpoint,omitempty"`
	EndEndpoint             *WindowDiscoveryEndpointProvenance `json:"end_endpoint,omitempty"`

	events  []Event
	windows []DiscoveredWindow
}

// DiscoveredWindow is the sole typed hand-off from a discovery strategy to a
// collector.  Core* covers the endpoint cluster; Start/End includes bounded
// context padding and is always <= the strategy's configured hard width.
type DiscoveredWindow struct {
	Ordinal             int                                `json:"ordinal"`
	CandidateRank       int                                `json:"candidate_rank"`
	CandidateWindow     int                                `json:"candidate_window"`
	Family              WindowDiscoveryFamily              `json:"family"`
	Kind                string                             `json:"kind"`
	StartTs             float64                            `json:"start_ts"`
	EndTs               float64                            `json:"end_ts"`
	CoreStartTs         float64                            `json:"core_start_ts"`
	CoreEndTs           float64                            `json:"core_end_ts"`
	CoreLineStart       int                                `json:"core_line_start"`
	CoreLineEnd         int                                `json:"core_line_end"`
	WindowOrigin        string                             `json:"window_origin"`
	RankBasis           string                             `json:"rank_basis"`
	IdentityFingerprint string                             `json:"identity_fingerprint"`
	PairingStatus       WindowDiscoveryPairingStatus       `json:"pairing_status,omitempty"`
	CarryClass          WindowDiscoveryCarryClass          `json:"carry_class,omitempty"`
	SemanticClass       string                             `json:"semantic_class,omitempty"`
	StartEndpoint       *WindowDiscoveryEndpointProvenance `json:"start_endpoint,omitempty"`
	EndEndpoint         *WindowDiscoveryEndpointProvenance `json:"end_endpoint,omitempty"`
}

type WindowDiscoveryResult struct {
	Strategy               WindowDiscoveryStrategy      `json:"strategy"`
	SourcePath             string                       `json:"source_path"`
	SourceFingerprint      string                       `json:"source_fingerprint"`
	Complete               bool                         `json:"complete"`
	IdentityComplete       bool                         `json:"identity_complete"`
	ParseComplete          bool                         `json:"parse_complete"`
	ScannedLineCount       int                          `json:"scanned_line_count"`
	ParsedKnown            int                          `json:"parsed_known"`
	UnparsedLineCount      int                          `json:"unparsed_line_count"`
	ParseLinePanics        int                          `json:"parse_line_panics"`
	ClockRegressions       int                          `json:"clock_regressions"`
	EndpointCount          int                          `json:"endpoint_count"`
	ScopedEndpointCount    int                          `json:"scoped_endpoint_count"`
	BudgetStopped          bool                         `json:"budget_stopped"`
	CandidatePoolTruncated bool                         `json:"candidate_pool_truncated"`
	RetainedCandidateCount int                          `json:"retained_candidate_count"`
	SelectionBasis         string                       `json:"selection_basis"`
	Families               []WindowDiscoveryFamilyStats `json:"families"`
	Candidates             []WindowDiscoveryCandidate   `json:"candidates,omitempty"`
	Windows                []DiscoveredWindow           `json:"windows,omitempty"`
	Caveats                []string                     `json:"caveats,omitempty"`
}

type pairingDiscoveryLane struct {
	cohort          pairingCohortState
	family          WindowDiscoveryFamily
	key             string
	identity        string
	events          []Event
	endpointCount   int
	startCount      int
	doneCount       int
	maxDepth        int
	eventsTruncated bool
	timeInvalid     bool
	lastTs          float64
	lastTsSet       bool
	pid             int
}

type pairingWindowDiscovery struct {
	req                WindowDiscoveryRequest
	scope              Query
	source             string
	families           map[WindowDiscoveryFamily]bool
	stats              map[WindowDiscoveryFamily]*WindowDiscoveryFamilyStats
	lanes              map[string]*pairingDiscoveryLane
	ambiguous          []*WindowDiscoveryCandidate
	schema             map[WindowDiscoveryFamily]*WindowDiscoveryCandidate
	endpointCount      int
	budgetStopped      bool
	poolTruncated      bool
	identityIncomplete bool
}

func DiscoverWindows(ctx context.Context, path string, flavorHint TraceFlavor, request WindowDiscoveryRequest) (WindowDiscoveryResult, error) {
	req, err := normalizeWindowDiscoveryRequest(request)
	if err != nil {
		return WindowDiscoveryResult{}, err
	}
	version, err := CaptureTraceSourceVersion(path)
	if err != nil {
		return WindowDiscoveryResult{}, err
	}
	canonicalPath := canonicalTraceIndexPath(path)
	var shell *Index
	switch req.Strategy {
	case WindowDiscoveryPairingIntegrity:
		d := newPairingWindowDiscovery(req, canonicalPath)
		shell, err = StreamScan(ctx, path, flavorHint, func(ev Event) bool { return d.observe(ev) })
		if err == nil {
			if windowDiscoveryAfterStreamScanHook != nil {
				windowDiscoveryAfterStreamScanHook()
			}
			if err = version.Validate(path); err == nil {
				return d.finalize(shell, version), nil
			}
		}
	case WindowDiscoveryTraceMarkCarry:
		d := newTraceMarkCarryDiscovery(req, canonicalPath)
		shell, err = StreamScan(ctx, path, flavorHint, func(ev Event) bool { return d.observe(canonicalPath, ev) })
		if err == nil {
			if windowDiscoveryAfterStreamScanHook != nil {
				windowDiscoveryAfterStreamScanHook()
			}
			if err = version.Validate(path); err == nil {
				return d.finalize(shell, version), nil
			}
		}
	}
	if err != nil {
		return WindowDiscoveryResult{}, err
	}
	return WindowDiscoveryResult{}, fmt.Errorf("window discovery: unsupported strategy %q", req.Strategy)
}

// Test-only seam: production leaves this nil. It proves that the immutable
// source-universe version captured before StreamScan is revalidated before a
// discovery result can be published.
var windowDiscoveryAfterStreamScanHook func()

func normalizeWindowDiscoveryRequest(in WindowDiscoveryRequest) (WindowDiscoveryRequest, error) {
	if in.Strategy == "" {
		return in, fmt.Errorf("window discovery: strategy is required; supported: %s", strings.Join(WindowDiscoveryStrategyNames(), ", "))
	}
	if in.Strategy != WindowDiscoveryPairingIntegrity && in.Strategy != WindowDiscoveryTraceMarkCarry {
		return in, fmt.Errorf("window discovery: unknown strategy %q; supported: %s", in.Strategy, strings.Join(WindowDiscoveryStrategyNames(), ", "))
	}
	if in.TimeStartSet != in.TimeEndSet {
		return in, fmt.Errorf("window discovery: time_start/time_end must be set together")
	}
	if in.TimeStartSet {
		if math.IsNaN(in.TimeStart) || math.IsInf(in.TimeStart, 0) || math.IsNaN(in.TimeEnd) || math.IsInf(in.TimeEnd, 0) || in.TimeStart < 0 || in.TimeEnd <= in.TimeStart {
			return in, fmt.Errorf("window discovery: invalid finite time window %.9g..%.9g", in.TimeStart, in.TimeEnd)
		}
	}
	if in.LineStart < 0 || in.LineEnd < 0 || (in.LineStart > 0 && in.LineEnd > 0 && in.LineEnd < in.LineStart) {
		return in, fmt.Errorf("window discovery: invalid line window %d..%d", in.LineStart, in.LineEnd)
	}
	if in.TimeStartSet && (in.LineStart > 0 || in.LineEnd > 0) {
		return in, fmt.Errorf("window discovery: time and line scopes cannot be combined")
	}
	familySet := map[WindowDiscoveryFamily]bool{}
	allowedFamilies := map[WindowDiscoveryFamily]bool{}
	for _, name := range WindowDiscoveryFamilyNames(in.Strategy) {
		allowedFamilies[WindowDiscoveryFamily(name)] = true
	}
	if len(in.Families) == 0 {
		for _, name := range WindowDiscoveryFamilyNames(in.Strategy) {
			in.Families = append(in.Families, WindowDiscoveryFamily(name))
		}
	}
	for _, family := range in.Families {
		if !allowedFamilies[family] {
			return in, fmt.Errorf("window discovery: unknown %s family %q; supported: %s", in.Strategy, family, strings.Join(WindowDiscoveryFamilyNames(in.Strategy), ", "))
		}
		if familySet[family] {
			return in, fmt.Errorf("window discovery: duplicate pairing family %q", family)
		}
		familySet[family] = true
	}
	if in.Strategy == WindowDiscoveryTraceMarkCarry && !in.TimeStartSet && in.LineStart == 0 && in.LineEnd == 0 {
		return in, fmt.Errorf("window discovery: trace_mark_carry requires a bounded parent time or line window")
	}
	if in.Strategy == WindowDiscoveryTraceMarkCarry && !in.TimeStartSet && (in.LineStart == 0 || in.LineEnd == 0) {
		return in, fmt.Errorf("window discovery: trace_mark_carry line parent requires both line_start and line_end")
	}
	if in.MaxWindows == 0 {
		in.MaxWindows = DefaultWindowDiscoveryMaxWindows
	}
	if in.MaxWindows < 1 || in.MaxWindows > HardWindowDiscoveryMaxWindows {
		return in, fmt.Errorf("window discovery: max_windows=%d outside 1..%d", in.MaxWindows, HardWindowDiscoveryMaxWindows)
	}
	if in.MaxWindowMs == 0 {
		in.MaxWindowMs = DefaultWindowDiscoveryMaxWindowMs
	}
	if math.IsNaN(in.MaxWindowMs) || math.IsInf(in.MaxWindowMs, 0) || in.MaxWindowMs <= 0 || in.MaxWindowMs > HardPairingDiscoveryMaxWindowMs {
		return in, fmt.Errorf("window discovery: max_window_ms=%.9g outside (0,%.0f] for %s", in.MaxWindowMs, HardPairingDiscoveryMaxWindowMs, in.Strategy)
	}
	if in.PaddingMs == 0 {
		in.PaddingMs = DefaultWindowDiscoveryPaddingMs
	}
	if math.IsNaN(in.PaddingMs) || math.IsInf(in.PaddingMs, 0) || in.PaddingMs < 0 || in.PaddingMs*2 > in.MaxWindowMs {
		return in, fmt.Errorf("window discovery: padding_ms=%.9g must be finite, >=0, and <= max_window_ms/2", in.PaddingMs)
	}
	if in.EndpointLimit == 0 {
		in.EndpointLimit = DefaultWindowDiscoveryEndpointLimit
	}
	if in.EndpointLimit < 1 || in.EndpointLimit > HardWindowDiscoveryEndpointLimit {
		return in, fmt.Errorf("window discovery: endpoint_limit=%d outside 1..%d", in.EndpointLimit, HardWindowDiscoveryEndpointLimit)
	}
	if in.ActiveLaneLimit == 0 {
		in.ActiveLaneLimit = DefaultWindowDiscoveryActiveLaneLimit
	}
	if in.ActiveLaneLimit < 1 || in.ActiveLaneLimit > HardWindowDiscoveryActiveLaneLimit {
		return in, fmt.Errorf("window discovery: active_lane_limit=%d outside 1..%d", in.ActiveLaneLimit, HardWindowDiscoveryActiveLaneLimit)
	}
	if in.CohortEventLimit == 0 {
		in.CohortEventLimit = DefaultWindowDiscoveryCohortEventLimit
	}
	if in.CohortEventLimit < 2 || in.CohortEventLimit > HardWindowDiscoveryCohortEventLimit {
		return in, fmt.Errorf("window discovery: cohort_event_limit=%d outside 2..%d", in.CohortEventLimit, HardWindowDiscoveryCohortEventLimit)
	}
	return in, nil
}

func newPairingWindowDiscovery(req WindowDiscoveryRequest, source string) *pairingWindowDiscovery {
	d := &pairingWindowDiscovery{
		req:      req,
		scope:    Query{TimeStart: req.TimeStart, TimeEnd: req.TimeEnd, TimeStartSet: req.TimeStartSet, TimeEndSet: req.TimeEndSet, LineStart: req.LineStart, LineEnd: req.LineEnd},
		source:   source,
		families: map[WindowDiscoveryFamily]bool{},
		stats:    map[WindowDiscoveryFamily]*WindowDiscoveryFamilyStats{},
		lanes:    map[string]*pairingDiscoveryLane{},
		schema:   map[WindowDiscoveryFamily]*WindowDiscoveryCandidate{},
	}
	for _, family := range req.Families {
		d.families[family] = true
		d.stats[family] = &WindowDiscoveryFamilyStats{Family: family}
	}
	return d
}

type pairingDiscoveryEndpoint struct {
	family   WindowDiscoveryFamily
	phase    string
	key      string
	identity string
	pid      int
	valid    bool
}

func (d *pairingWindowDiscovery) decode(ev Event) (pairingDiscoveryEndpoint, bool) {
	if d.families[WindowDiscoveryFamilyBlock] {
		family, phase, endpoint := blockLatencyEndpoint(ev)
		if endpoint {
			identity, valid := blockIdentity(ev)
			phaseName := "start"
			if phase == blockEndpointDone {
				phaseName = "done"
			}
			if !valid {
				return pairingDiscoveryEndpoint{family: WindowDiscoveryFamilyBlock, phase: phaseName}, true
			}
			label := fmt.Sprintf("family=%s dev=%s op=%s sector=%d len=%d", family, identity.Dev, identity.Op, identity.Sector, identity.Len)
			return pairingDiscoveryEndpoint{family: WindowDiscoveryFamilyBlock, phase: phaseName, key: d.source + "\x00" + identity.laneKey(), identity: label, valid: true}, true
		}
	}
	if d.families[WindowDiscoveryFamilyStorage] {
		identity, phase, endpoint := genericStorageEndpoint(ev)
		if endpoint {
			return pairingDiscoveryEndpoint{family: WindowDiscoveryFamilyStorage, phase: phase, key: d.source + "\x00" + identity.laneKey(), identity: genericStorageIdentityLabel(identity), pid: identity.PID, valid: true}, true
		}
	}
	return pairingDiscoveryEndpoint{}, false
}

func (d *pairingWindowDiscovery) observe(ev Event) bool {
	if resetPID, reset := schedulerLifecycleResetPID(ev); reset {
		d.resetStoragePID(resetPID, ev)
	}
	endpoint, recognized := d.decode(ev)
	if !recognized {
		return true
	}
	if d.endpointCount >= d.req.EndpointLimit {
		d.budgetStopped = true
		return false
	}
	d.endpointCount++
	stats := d.stats[endpoint.family]
	stats.EndpointCount++
	if endpoint.phase == "start" {
		stats.StartCount++
	} else {
		stats.DoneCount++
	}
	if pairingEventInsideQuery(ev, d.scope) {
		stats.ScopedEndpointCount++
		if endpoint.phase == "start" {
			stats.ScopedStartCount++
		} else {
			stats.ScopedDoneCount++
		}
	}
	if !endpoint.valid {
		if pairingEventInsideQuery(ev, d.scope) {
			stats.InvalidIdentityCount++
			d.identityIncomplete = true
		}
		return true
	}
	lane := d.lanes[endpoint.key]
	if endpoint.phase == "done" && lane == nil {
		if pairingEventInsideQuery(ev, d.scope) {
			stats.UnpairedDoneCount++
		}
		return true
	}
	if lane == nil {
		if len(d.lanes) >= d.req.ActiveLaneLimit {
			d.budgetStopped = true
			return false
		}
		lane = &pairingDiscoveryLane{family: endpoint.family, key: endpoint.key, identity: endpoint.identity, pid: endpoint.pid}
		d.lanes[endpoint.key] = lane
	}
	lane.endpointCount++
	if endpoint.phase == "start" {
		lane.startCount++
	} else {
		lane.doneCount++
	}
	if lane.lastTsSet && ev.Ts < lane.lastTs {
		lane.timeInvalid = true
	}
	lane.lastTs, lane.lastTsSet = ev.Ts, true
	if len(lane.events) < d.req.CohortEventLimit {
		lane.events = append(lane.events, ev)
	} else if !lane.eventsTruncated {
		lane.eventsTruncated = true
	}
	var transition pairingCohortTransition
	if endpoint.phase == "start" {
		transition = lane.cohort.observeStart(ev)
		if lane.cohort.depth > lane.maxDepth {
			lane.maxDepth = lane.cohort.depth
		}
	} else {
		transition = lane.cohort.observeDone(ev)
	}
	if transition.cohortClosed {
		d.finishClosedLane(lane, transition)
		delete(d.lanes, endpoint.key)
	}
	return true
}

func (d *pairingWindowDiscovery) resetStoragePID(pid int, boundary Event) {
	if pid <= 0 || !d.families[WindowDiscoveryFamilyStorage] {
		return
	}
	for key, lane := range d.lanes {
		if lane.family != WindowDiscoveryFamilyStorage || lane.pid != pid {
			continue
		}
		transition := lane.cohort.finishEOF()
		relevant := pairingIntervalIntersectsQuery(transition.first, boundary, d.scope)
		if relevant {
			stats := d.stats[lane.family]
			stats.LifecycleResetLaneCount++
			d.identityIncomplete = true
			if !transition.ambiguous {
				d.recordLaneIdentityIssues(lane)
			}
		}
		if transition.ambiguous && relevant {
			candidate := d.candidateFromLane(lane, transition, "ambiguous_lifecycle_cut", false)
			candidate.CollectionBlockedReason = "task_generation_boundary_before_cohort_closed"
			d.retainAmbiguous(candidate)
		}
		delete(d.lanes, key)
	}
}

func (d *pairingWindowDiscovery) finishClosedLane(lane *pairingDiscoveryLane, transition pairingCohortTransition) {
	if !pairingIntervalIntersectsQuery(transition.first, transition.last, d.scope) {
		return
	}
	stats := d.stats[lane.family]
	if transition.ambiguous {
		stats.ClosedAmbiguousCount++
	} else if transition.pairReady {
		stats.CompletedPairCount++
	}
	if transition.ambiguous {
		d.retainAmbiguous(d.candidateFromLane(lane, transition, "ambiguous_closed", true))
		return
	}
	if !transition.pairReady {
		return
	}
	candidate := d.candidateFromLane(lane, transition, "schema_probe", true)
	current := d.schema[lane.family]
	if current == nil || discoveryCandidateLess(candidate, current) {
		d.schema[lane.family] = candidate
	}
}

func (d *pairingWindowDiscovery) candidateFromLane(lane *pairingDiscoveryLane, transition pairingCohortTransition, kind string, closed bool) *WindowDiscoveryCandidate {
	d.recordLaneIdentityIssues(lane)
	first, last := eventEnvelope(lane.events)
	if first.Line == 0 {
		first, last = transition.first, transition.last
	}
	sum := sha256.Sum256([]byte(lane.key))
	candidate := &WindowDiscoveryCandidate{
		Family:              lane.family,
		Kind:                kind,
		Identity:            lane.identity,
		IdentityFingerprint: fmt.Sprintf("sha256:%x", sum[:8]),
		FirstLine:           first.Line,
		LastLine:            last.Line,
		CoreStartTs:         first.Ts,
		CoreEndTs:           last.Ts,
		EndpointCount:       lane.endpointCount,
		StartCount:          lane.startCount,
		DoneCount:           lane.doneCount,
		MaxDepth:            lane.maxDepth,
		Closed:              closed,
		events:              append([]Event(nil), lane.events...),
	}
	if candidate.CoreEndTs < candidate.CoreStartTs {
		candidate.CoreStartTs, candidate.CoreEndTs = candidate.CoreEndTs, candidate.CoreStartTs
	}
	if !closed {
		candidate.CollectionBlockedReason = "cohort_not_closed"
	} else if lane.eventsTruncated {
		candidate.CollectionBlockedReason = "cohort_endpoint_roster_exceeded_budget"
	} else if lane.timeInvalid {
		candidate.CollectionBlockedReason = "same_lane_timestamp_rollback"
	} else {
		candidate.windows, candidate.CollectionComplete, candidate.RequiredWindowCount = buildCandidateWindows(candidate, d.req)
		candidate.FitsSingleWindow = len(candidate.windows) == 1
		if !candidate.CollectionComplete && candidate.CollectionBlockedReason == "" {
			candidate.CollectionBlockedReason = "candidate_requires_more_than_hard_window_budget"
		}
	}
	return candidate
}

func (d *pairingWindowDiscovery) recordLaneIdentityIssues(lane *pairingDiscoveryLane) {
	if lane == nil {
		return
	}
	stats := d.stats[lane.family]
	if lane.timeInvalid {
		stats.TimestampRollbackCount++
		d.identityIncomplete = true
	}
	if lane.eventsTruncated {
		stats.CohortEventOverflowCount++
		d.identityIncomplete = true
	}
}

func eventEnvelope(events []Event) (Event, Event) {
	if len(events) == 0 {
		return Event{}, Event{}
	}
	first, last := events[0], events[0]
	for _, ev := range events[1:] {
		if ev.Line < first.Line {
			first = ev
		}
		if ev.Line > last.Line {
			last = ev
		}
	}
	return first, last
}

func buildCandidateWindows(candidate *WindowDiscoveryCandidate, req WindowDiscoveryRequest) ([]DiscoveredWindow, bool, int) {
	if candidate == nil || len(candidate.events) == 0 {
		return nil, false, 0
	}
	events := append([]Event(nil), candidate.events...)
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].Ts != events[j].Ts {
			return events[i].Ts < events[j].Ts
		}
		return events[i].Line < events[j].Line
	})
	for _, ev := range events {
		if math.IsNaN(ev.Ts) || math.IsInf(ev.Ts, 0) || ev.Ts < 0 {
			return nil, false, 0
		}
	}
	maxSec := req.MaxWindowMs / 1000
	var windows []DiscoveredWindow
	for start := 0; start < len(events); {
		end := start + 1
		for end < len(events) && events[end].Ts-events[start].Ts <= maxSec+1e-12 {
			end++
		}
		cluster := events[start:end]
		coreStart, coreEnd := cluster[0].Ts, cluster[len(cluster)-1].Ts
		coreLineStart, coreLineEnd := cluster[0].Line, cluster[0].Line
		for _, ev := range cluster[1:] {
			if ev.Line < coreLineStart {
				coreLineStart = ev.Line
			}
			if ev.Line > coreLineEnd {
				coreLineEnd = ev.Line
			}
		}
		availablePad := (maxSec - (coreEnd - coreStart)) / 2
		pad := math.Min(req.PaddingMs/1000, math.Max(0, availablePad))
		windowStart := math.Max(0, coreStart-pad)
		windowEnd := coreEnd + pad
		if windowEnd <= windowStart {
			windowEnd = math.Min(windowStart+maxSec, math.Nextafter(windowStart, math.Inf(1)))
		}
		if windowEnd-windowStart > maxSec+1e-12 {
			return nil, false, len(windows) + 1
		}
		windows = append(windows, DiscoveredWindow{
			CandidateWindow:     len(windows) + 1,
			Family:              candidate.Family,
			Kind:                candidate.Kind,
			StartTs:             windowStart,
			EndTs:               windowEnd,
			CoreStartTs:         coreStart,
			CoreEndTs:           coreEnd,
			CoreLineStart:       coreLineStart,
			CoreLineEnd:         coreLineEnd,
			WindowOrigin:        string(WindowDiscoveryPairingIntegrity),
			RankBasis:           "closed_ambiguous_then_open_ambiguous_then_schema_probe;max_depth;endpoint_count;physical_line",
			IdentityFingerprint: candidate.IdentityFingerprint,
		})
		if len(windows) > HardWindowDiscoveryMaxWindows {
			return nil, false, len(windows)
		}
		start = end
	}
	return windows, len(windows) > 0, len(windows)
}

func (d *pairingWindowDiscovery) retainAmbiguous(candidate *WindowDiscoveryCandidate) {
	if candidate == nil {
		return
	}
	d.ambiguous = append(d.ambiguous, candidate)
	sort.SliceStable(d.ambiguous, func(i, j int) bool { return discoveryCandidateLess(d.ambiguous[i], d.ambiguous[j]) })
	if len(d.ambiguous) > windowDiscoveryCandidatePoolLimit {
		d.ambiguous = d.ambiguous[:windowDiscoveryCandidatePoolLimit]
		d.poolTruncated = true
	}
}

func discoveryCandidateKindPriority(kind string) int {
	switch kind {
	case "ambiguous_closed":
		return 0
	case "ambiguous_eof", "ambiguous_lifecycle_cut":
		return 1
	case "schema_probe":
		return 2
	default:
		return 3
	}
}

func discoveryCandidateLess(a, b *WindowDiscoveryCandidate) bool {
	if a == nil || b == nil {
		return b != nil
	}
	if pa, pb := discoveryCandidateKindPriority(a.Kind), discoveryCandidateKindPriority(b.Kind); pa != pb {
		return pa < pb
	}
	if a.CollectionComplete != b.CollectionComplete {
		return a.CollectionComplete
	}
	if a.FitsSingleWindow != b.FitsSingleWindow {
		return a.FitsSingleWindow
	}
	if a.MaxDepth != b.MaxDepth {
		return a.MaxDepth > b.MaxDepth
	}
	if a.EndpointCount != b.EndpointCount {
		return a.EndpointCount > b.EndpointCount
	}
	if a.FirstLine != b.FirstLine {
		return a.FirstLine < b.FirstLine
	}
	if a.LastLine != b.LastLine {
		return a.LastLine < b.LastLine
	}
	if a.Family != b.Family {
		return a.Family < b.Family
	}
	return a.IdentityFingerprint < b.IdentityFingerprint
}

func (d *pairingWindowDiscovery) finalize(shell *Index, version TraceSourceVersion) WindowDiscoveryResult {
	for _, lane := range d.lanes {
		transition := lane.cohort.finishEOF()
		stats := d.stats[lane.family]
		if transition.ambiguous {
			if pairingOpenCohortIntersectsIndex(transition.first, shell, d.scope) {
				stats.OpenAmbiguousCount++
				candidate := d.candidateFromLane(lane, transition, "ambiguous_eof", false)
				d.retainAmbiguous(candidate)
			}
		} else if transition.cohortStarts > 0 {
			if pairingOpenCohortIntersectsIndex(transition.first, shell, d.scope) {
				stats.OpenSingleCount++
				d.recordLaneIdentityIssues(lane)
			}
		}
	}
	pool := append([]*WindowDiscoveryCandidate(nil), d.ambiguous...)
	for _, family := range []WindowDiscoveryFamily{WindowDiscoveryFamilyBlock, WindowDiscoveryFamilyStorage} {
		if candidate := d.schema[family]; candidate != nil {
			pool = append(pool, candidate)
		}
	}
	sort.SliceStable(pool, func(i, j int) bool { return discoveryCandidateLess(pool[i], pool[j]) })
	for i, candidate := range pool {
		candidate.Rank = i + 1
		for j := range candidate.windows {
			candidate.windows[j].CandidateRank = candidate.Rank
		}
	}
	d.selectWindows(pool)
	reportCandidates := selectedAndTopCandidates(pool, windowDiscoveryCandidateReportLimit)
	result := WindowDiscoveryResult{
		Strategy:               d.req.Strategy,
		SourcePath:             d.source,
		SourceFingerprint:      version.Fingerprint(),
		Complete:               !d.budgetStopped,
		IdentityComplete:       !d.budgetStopped && !d.identityIncomplete,
		ParseComplete:          shell != nil && shell.UnparsedLines == 0 && shell.ParseLinePanics == 0,
		EndpointCount:          d.endpointCount,
		BudgetStopped:          d.budgetStopped,
		CandidatePoolTruncated: d.poolTruncated,
		RetainedCandidateCount: len(pool),
		SelectionBasis:         "closed ambiguous cohorts first; then open ambiguous disclosure; then completed-pair schema probes; stable family seat and physical-line tie-break",
		Candidates:             reportCandidates,
	}
	if shell != nil {
		result.ScannedLineCount = shell.ScannedLineCount
		result.ParsedKnown = shell.ParsedKnown
		result.UnparsedLineCount = shell.UnparsedLines
		result.ParseLinePanics = shell.ParseLinePanics
		result.ClockRegressions = shell.ClockRegressions
	}
	for _, family := range []WindowDiscoveryFamily{WindowDiscoveryFamilyBlock, WindowDiscoveryFamilyStorage} {
		if stats := d.stats[family]; stats != nil {
			result.Families = append(result.Families, *stats)
			result.ScopedEndpointCount += stats.ScopedEndpointCount
		}
	}
	for _, candidate := range pool {
		if !candidate.Selected {
			continue
		}
		for _, window := range candidate.windows {
			window.Ordinal = len(result.Windows) + 1
			result.Windows = append(result.Windows, window)
		}
	}
	if d.budgetStopped {
		result.Caveats = append(result.Caveats, fmt.Sprintf("discovery_complete=false; endpoint_or_active_lane_budget_reached endpoints=%d/%d active_lane_limit=%d; no negative absence claim is allowed", d.endpointCount, d.req.EndpointLimit, d.req.ActiveLaneLimit))
	}
	if !result.ParseComplete {
		result.Caveats = append(result.Caveats, fmt.Sprintf("parse_complete=false; unparsed_lines=%d parse_panics=%d; zero overlap applies only to the parsed endpoint closed set", result.UnparsedLineCount, result.ParseLinePanics))
	}
	if !result.IdentityComplete {
		result.Caveats = append(result.Caveats, "identity_complete=false; malformed identity, lifecycle cut, same-lane timestamp rollback, or cohort roster budget prevented a complete pairing claim")
	}
	if d.poolTruncated {
		result.Caveats = append(result.Caveats, fmt.Sprintf("candidate_pool_truncated=true; retained_top=%d with stable comparator; lower-ranked candidate detail was omitted", windowDiscoveryCandidatePoolLimit))
	}
	if len(result.Windows) == 0 {
		result.Caveats = append(result.Caveats, "generated_windows=0; no closed candidate with a complete endpoint roster fit the configured bounded fan-out; dependent collection must fail explicit instead of falling back to the parent window")
	}
	return result
}

func (d *pairingWindowDiscovery) selectWindows(pool []*WindowDiscoveryCandidate) {
	remaining := d.req.MaxWindows
	selectedFamily := map[WindowDiscoveryFamily]bool{}
	selectCandidate := func(candidate *WindowDiscoveryCandidate, reason string) bool {
		if candidate == nil || candidate.Selected || !candidate.CollectionComplete || len(candidate.windows) == 0 || len(candidate.windows) > remaining {
			return false
		}
		candidate.Selected = true
		candidate.SelectionReason = reason
		remaining -= len(candidate.windows)
		selectedFamily[candidate.Family] = true
		return true
	}
	for _, candidate := range pool {
		if selectCandidate(candidate, "highest_ranked_collectible_candidate") {
			break
		}
	}
	if remaining > 0 && len(d.families) > 1 {
		for _, candidate := range pool {
			if selectedFamily[candidate.Family] {
				continue
			}
			if selectCandidate(candidate, "cross_family_witness_seat") {
				break
			}
		}
	}
	for _, candidate := range pool {
		if remaining == 0 {
			break
		}
		selectCandidate(candidate, "rank_order_budget_fill")
	}
	for _, candidate := range pool {
		if candidate.Selected || candidate.SelectionReason != "" {
			continue
		}
		switch {
		case !candidate.CollectionComplete:
			candidate.SelectionReason = "not_collectible:" + candidate.CollectionBlockedReason
		case len(candidate.windows) > remaining:
			candidate.SelectionReason = "generated_window_budget_exhausted"
		default:
			candidate.SelectionReason = "lower_rank_than_selected_budget"
		}
	}
}

func selectedAndTopCandidates(pool []*WindowDiscoveryCandidate, limit int) []WindowDiscoveryCandidate {
	include := map[int]bool{}
	for i := 0; i < len(pool) && i < limit; i++ {
		include[i] = true
	}
	for i, candidate := range pool {
		if candidate.Selected {
			include[i] = true
		}
	}
	indexes := make([]int, 0, len(include))
	for i := range include {
		indexes = append(indexes, i)
	}
	sort.Ints(indexes)
	out := make([]WindowDiscoveryCandidate, 0, len(indexes))
	for _, i := range indexes {
		copy := *pool[i]
		copy.events = nil
		copy.windows = nil
		out = append(out, copy)
	}
	return out
}
