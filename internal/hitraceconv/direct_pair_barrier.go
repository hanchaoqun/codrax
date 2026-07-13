package hitraceconv

import (
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	directPairBarrierMaxObservations int64 = 4_000_000
	directPairBarrierMaxLaneKeys     int64 = 1_000_000
)

type directPairBarrierRow struct {
	kind pairRenderKind
	lane string
}

type directPairProofDomain struct {
	maxObservations int64
	maxLaneKeys     int64
	observations    int64
	laneKeys        int64
	failureReason   string
}

type directPairLaneClock struct {
	seq  int
	tsNS uint64
}

type directPairBarrierReport struct {
	WithheldRows          int
	PoisonedLanes         int
	PoisonedFamilies      int
	BlockObserved         int64
	BlockRowsStaged       int
	BlockRowsWithheld     int
	LegacyBudgetReason    string
	BlockBudgetReason     string
	SharedAuthorityReason string
}

// directPairCaptureBarrier sees the complete direct RMQ endpoint set before a
// single pair-critical row becomes public. It is intentionally narrower than
// the SQL stage: direct RMQ has no canonical ITID/generation authority, so this
// barrier closes only physical bad-row holes and leaves TID reuse to
// tracequery's lifecycle/incarnation authority.
type directPairCaptureBarrier struct {
	source           string
	maxRows          int64
	legacyProof      directPairProofDomain
	blockProof       directPairProofDomain
	blockObserved    int64
	poisonedKinds    map[pairRenderKind]bool
	poisonedLanes    map[pairRenderKind]map[string]bool
	seenLanes        map[pairRenderKind]map[string]struct{}
	blockLaneClocks  map[string]directPairLaneClock
	rows             map[int]directPairBarrierRow
	stagedRows       map[pairRenderKind]int
	withheldRows     map[pairRenderKind]int
	poisonedRows     int
	poisonedFormats  int
	authorityFailure string
}

func newDirectPairCaptureBarrier(sourcePath string) (*directPairCaptureBarrier, error) {
	if strings.TrimSpace(sourcePath) == "" {
		return nil, &traceDBOutputInvariantError{Reason: "invalid_direct_pair_source_namespace"}
	}
	abs, err := filepath.Abs(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("resolve direct pair source namespace: %w", err)
	}
	abs = filepath.Clean(abs)
	if physical, resolveErr := filepath.EvalSymlinks(abs); resolveErr == nil {
		abs = filepath.Clean(physical)
	}
	if abs == "" || abs == "." {
		return nil, &traceDBOutputInvariantError{Reason: "invalid_direct_pair_source_namespace"}
	}
	return &directPairCaptureBarrier{
		source:  abs,
		maxRows: directPairBarrierMaxObservations,
		legacyProof: directPairProofDomain{
			maxObservations: directPairBarrierMaxObservations, maxLaneKeys: directPairBarrierMaxLaneKeys,
		},
		blockProof: directPairProofDomain{
			maxObservations: directPairBarrierMaxObservations, maxLaneKeys: directPairBarrierMaxLaneKeys,
		},
		poisonedKinds: map[pairRenderKind]bool{}, poisonedLanes: map[pairRenderKind]map[string]bool{},
		seenLanes: map[pairRenderKind]map[string]struct{}{}, blockLaneClocks: map[string]directPairLaneClock{},
		rows: map[int]directPairBarrierRow{}, stagedRows: map[pairRenderKind]int{}, withheldRows: map[pairRenderKind]int{},
	}, nil
}

func (barrier *directPairCaptureBarrier) observe(audit directPairLineAudit) {
	if barrier == nil || !audit.Governed {
		return
	}
	if audit.Kind == pairRenderBlock && !barrier.countBlockObservation() {
		return
	}
	if !barrier.observeProofDomain(audit.Kind) {
		return
	}
	if barrier.poisonedKinds[audit.Kind] {
		return
	}
	if audit.Kind == pairRenderMMC {
		lane, laneKnown := directMMCLaneKey(audit, barrier.source)
		if laneKnown && !barrier.admitLane(audit.Kind, lane) {
			return
		}
		if !audit.EndpointAdmitted {
			if laneKnown {
				// Header TID is already a deterministic endpoint identity in the
				// downstream coarse key. It is the finest source-pinned locality
				// available without promoting unproven MRQ/tag request tokens.
				barrier.poisonLane(audit.Kind, lane)
			} else {
				barrier.poisonedKinds[pairRenderMMC] = true
			}
		}
		return
	}
	lane, laneKnown := pairingEndpointLaneKey(audit.Verdict, barrier.source)
	if laneKnown {
		if !barrier.admitLane(audit.Kind, lane) {
			return
		}
	}
	if audit.EndpointAdmitted {
		return
	}
	if audit.HeaderOwnerKnown && audit.Verdict.EmitterKnown && audit.Verdict.KeyKnown && laneKnown {
		barrier.poisonLane(audit.Kind, lane)
		return
	}
	barrier.poisonedKinds[audit.Kind] = true
}

func (barrier *directPairCaptureBarrier) countBlockObservation() bool {
	if barrier == nil {
		return false
	}
	if barrier.blockObserved == math.MaxInt64 {
		barrier.failAuthority("block_observation_counter_overflow")
		return false
	}
	barrier.blockObserved++
	return true
}

// addPublishedRow is retained for focused barrier fixtures whose synthetic
// sequence is also their monotonic clock. Production must call the timestamped
// choke point below with the physical RMQ timestamp.
func (barrier *directPairCaptureBarrier) addPublishedRow(seq int, audit directPairLineAudit) {
	barrier.addPublishedRowAt(seq, uint64(seq), audit)
}

func (barrier *directPairCaptureBarrier) addPublishedRowAt(seq int, tsNS uint64, audit directPairLineAudit) {
	if barrier == nil || !audit.Governed || !audit.EndpointAdmitted || seq < 0 {
		return
	}
	barrier.stagedRows[audit.Kind]++
	if barrier.poisonedKinds[audit.Kind] {
		// Freeze will suppress this whole family; retaining per-row mappings
		// would spend proof budget without adding any locality guarantee.
		return
	}
	lane, ok := pairingEndpointLaneKey(audit.Verdict, barrier.source)
	if audit.Kind == pairRenderMMC {
		lane, ok = directMMCLaneKey(audit, barrier.source)
	}
	if !ok {
		barrier.failAuthority("published_row_lane_missing")
		return
	}
	if barrier.authorityFailure != "" {
		return
	}
	if int64(len(barrier.rows)) >= barrier.maxRows {
		barrier.failAuthority("shared_row_capacity")
		return
	}
	if _, exists := barrier.rows[seq]; exists {
		barrier.failAuthority("duplicate_published_seq")
		return
	}
	barrier.rows[seq] = directPairBarrierRow{kind: audit.Kind, lane: lane}
	if audit.Kind == pairRenderBlock {
		if previous, found := barrier.blockLaneClocks[lane]; found {
			if seq <= previous.seq {
				barrier.failAuthority("block_physical_sequence_regression")
				return
			}
			if tsNS < previous.tsNS {
				barrier.poisonLane(pairRenderBlock, lane)
			}
		}
		barrier.blockLaneClocks[lane] = directPairLaneClock{seq: seq, tsNS: tsNS}
	}
}

func (barrier *directPairCaptureBarrier) observeProofDomain(kind pairRenderKind) bool {
	if barrier == nil || barrier.authorityFailure != "" {
		return false
	}
	domain := &barrier.legacyProof
	if kind == pairRenderBlock {
		domain = &barrier.blockProof
	}
	if domain.failureReason != "" {
		return false
	}
	domain.observations++
	if domain.observations > domain.maxObservations {
		barrier.failProofDomain(kind, "observation_cap")
		return false
	}
	return true
}

func (barrier *directPairCaptureBarrier) poisonLane(kind pairRenderKind, lane string) {
	if barrier == nil || kind == pairRenderUnknown || lane == "" || barrier.poisonedKinds[kind] {
		return
	}
	if barrier.poisonedLanes[kind] == nil {
		barrier.poisonedLanes[kind] = map[string]bool{}
	}
	barrier.poisonedLanes[kind][lane] = true
}

func directMMCLaneKey(audit directPairLineAudit, source string) (string, bool) {
	if !audit.HeaderOwnerKnown || audit.HeaderTID < 0 || source == "" {
		return "", false
	}
	return source + "\x00mmc\x00tid=" + strconv.FormatInt(audit.HeaderTID, 10), true
}

func (barrier *directPairCaptureBarrier) poisonFormatFamilies(mask pairCriticalFormatFamilyMask) {
	if barrier == nil || mask == 0 {
		return
	}
	barrier.poisonedFormats++
	if mask&pairCriticalFormatFamilyWorkqueue != 0 {
		barrier.poisonedKinds[pairRenderWorkqueue] = true
	}
	if mask&pairCriticalFormatFamilyDMAFence != 0 {
		barrier.poisonedKinds[pairRenderDMAFence] = true
	}
	if mask&pairCriticalFormatFamilyMMC != 0 {
		barrier.poisonedKinds[pairRenderMMC] = true
	}
	if mask&pairCriticalFormatFamilyF2FS != 0 {
		barrier.poisonedKinds[pairRenderF2FS] = true
	}
	if mask&pairCriticalFormatFamilyBlock != 0 {
		if barrier.countBlockObservation() {
			barrier.observeProofDomain(pairRenderBlock)
		}
		barrier.poisonedKinds[pairRenderBlock] = true
	}
}

func (barrier *directPairCaptureBarrier) filter(rows []renderedRow) []renderedRow {
	if barrier == nil || len(rows) == 0 {
		return rows
	}
	// First prove that every pair-critical publisher row is represented by the
	// frozen barrier. A missing/mismatched mapping is an authority failure: if
	// it were handled row-locally, the remaining endpoints could still pair
	// across the invisible physical row. Close every governed family before emitting
	// any pair row; inventory remains independent.
	for _, row := range rows {
		if row.pairKind == pairRenderUnknown || barrier.poisonedKinds[row.pairKind] {
			continue
		}
		pair, ok := barrier.rows[row.seq]
		if !ok || pair.kind != row.pairKind || pair.lane == "" {
			barrier.failAuthority("published_row_mapping_mismatch")
			break
		}
	}
	out := rows[:0]
	for _, row := range rows {
		if row.pairKind == pairRenderUnknown {
			out = append(out, row)
			continue
		}
		pair, laneKnown := barrier.rows[row.seq]
		if barrier.authorityFailure != "" || barrier.poisonedKinds[row.pairKind] || !laneKnown ||
			pair.kind != row.pairKind || barrier.poisonedLanes[row.pairKind][pair.lane] {
			barrier.poisonedRows++
			barrier.withheldRows[row.pairKind]++
			continue
		}
		out = append(out, row)
	}
	return out
}

func (barrier *directPairCaptureBarrier) admitLane(kind pairRenderKind, lane string) bool {
	if lane == "" || kind == pairRenderUnknown {
		barrier.poisonedKinds[kind] = true
		return false
	}
	items := barrier.seenLanes[kind]
	if items == nil {
		items = map[string]struct{}{}
		barrier.seenLanes[kind] = items
	}
	if _, found := items[lane]; found {
		return true
	}
	domain := &barrier.legacyProof
	if kind == pairRenderBlock {
		domain = &barrier.blockProof
	}
	if domain.failureReason != "" {
		return false
	}
	if domain.laneKeys >= domain.maxLaneKeys {
		barrier.failProofDomain(kind, "lane_key_cap")
		return false
	}
	items[lane] = struct{}{}
	domain.laneKeys++
	return true
}

func (barrier *directPairCaptureBarrier) failProofDomain(kind pairRenderKind, reason string) {
	if barrier == nil || reason == "" {
		return
	}
	if kind == pairRenderBlock {
		if barrier.blockProof.failureReason == "" {
			barrier.blockProof.failureReason = reason
		}
		barrier.poisonedKinds[pairRenderBlock] = true
		delete(barrier.seenLanes, pairRenderBlock)
		delete(barrier.poisonedLanes, pairRenderBlock)
		barrier.blockLaneClocks = nil
		return
	}
	if barrier.legacyProof.failureReason == "" {
		barrier.legacyProof.failureReason = reason
	}
	for _, legacyKind := range []pairRenderKind{pairRenderWorkqueue, pairRenderDMAFence, pairRenderMMC, pairRenderF2FS} {
		barrier.poisonedKinds[legacyKind] = true
		delete(barrier.seenLanes, legacyKind)
		delete(barrier.poisonedLanes, legacyKind)
	}
}

func (barrier *directPairCaptureBarrier) failAuthority(reason string) {
	if barrier == nil || reason == "" {
		return
	}
	if barrier.authorityFailure == "" {
		barrier.authorityFailure = reason
	}
	for _, kind := range []pairRenderKind{
		pairRenderWorkqueue, pairRenderDMAFence, pairRenderMMC, pairRenderF2FS, pairRenderBlock,
	} {
		barrier.poisonedKinds[kind] = true
	}
	barrier.seenLanes = nil
	barrier.poisonedLanes = nil
	barrier.blockLaneClocks = nil
}

func (barrier *directPairCaptureBarrier) poisonedFamilyCount() int {
	if barrier == nil {
		return 0
	}
	count := 0
	for _, kind := range []pairRenderKind{pairRenderWorkqueue, pairRenderDMAFence, pairRenderMMC, pairRenderF2FS, pairRenderBlock} {
		if barrier.poisonedKinds[kind] {
			count++
		}
	}
	return count
}

func (barrier *directPairCaptureBarrier) poisonedLaneCount() int {
	if barrier == nil {
		return 0
	}
	total := 0
	for _, lanes := range barrier.poisonedLanes {
		total += len(lanes)
	}
	return total
}

func (barrier *directPairCaptureBarrier) report() directPairBarrierReport {
	if barrier == nil {
		return directPairBarrierReport{}
	}
	return directPairBarrierReport{
		WithheldRows: barrier.poisonedRows, PoisonedLanes: barrier.poisonedLaneCount(),
		PoisonedFamilies: barrier.poisonedFamilyCount(), BlockObserved: barrier.blockObserved,
		BlockRowsStaged: barrier.stagedRows[pairRenderBlock], BlockRowsWithheld: barrier.withheldRows[pairRenderBlock],
		LegacyBudgetReason: barrier.legacyProof.failureReason,
		BlockBudgetReason:  barrier.blockProof.failureReason, SharedAuthorityReason: barrier.authorityFailure,
	}
}

// validateAccounting is the last private-stage invariant before ConvertFile
// can sort rows or create an output artifact. Pair rows are accepted exactly
// once at addPublishedRowAt; freeze may only move them from staged to withheld.
func (barrier *directPairCaptureBarrier) validateAccounting(rows []renderedRow) error {
	if barrier == nil {
		return &traceDBOutputInvariantError{Reason: "direct_pair_barrier_missing"}
	}
	stagedTotal := 0
	withheldTotal := 0
	for _, kind := range []pairRenderKind{
		pairRenderWorkqueue, pairRenderDMAFence, pairRenderMMC, pairRenderF2FS, pairRenderBlock,
	} {
		staged := barrier.stagedRows[kind]
		withheld := barrier.withheldRows[kind]
		if staged < 0 || withheld < 0 || withheld > staged {
			return &traceDBOutputInvariantError{Reason: "direct_pair_barrier_withheld_exceeds_staged"}
		}
		stagedTotal += staged
		withheldTotal += withheld
	}
	if withheldTotal != barrier.poisonedRows {
		return &traceDBOutputInvariantError{Reason: "direct_pair_barrier_withheld_account_mismatch"}
	}
	published := 0
	for _, row := range rows {
		if row.pairKind != pairRenderUnknown {
			published++
		}
	}
	if published != stagedTotal-withheldTotal {
		return &traceDBOutputInvariantError{Reason: "direct_pair_barrier_published_account_mismatch"}
	}
	if barrier.blockObserved < int64(barrier.stagedRows[pairRenderBlock]) {
		return &traceDBOutputInvariantError{Reason: "direct_block_barrier_observation_account_mismatch"}
	}
	return nil
}
