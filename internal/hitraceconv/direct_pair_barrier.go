package hitraceconv

import (
	"fmt"
	"path/filepath"
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

// directPairCaptureBarrier sees the complete direct RMQ endpoint set before a
// single pair-critical row becomes public. It is intentionally narrower than
// the SQL stage: direct RMQ has no canonical ITID/generation authority, so this
// barrier closes only physical bad-row holes and leaves TID reuse to
// tracequery's lifecycle/incarnation authority.
type directPairCaptureBarrier struct {
	source          string
	maxObservations int64
	maxLaneKeys     int64
	maxRows         int64
	observations    int64
	laneKeys        int64
	poisonedKinds   map[pairRenderKind]bool
	poisonedLanes   map[string]bool
	seenLanes       map[pairRenderKind]map[string]struct{}
	rows            map[int]directPairBarrierRow
	poisonedRows    int
	poisonedFormats int
	budgetFailed    bool
}

func newDirectPairCaptureBarrier(outputPath string) (*directPairCaptureBarrier, error) {
	if strings.TrimSpace(outputPath) == "" {
		return nil, &traceDBOutputInvariantError{Reason: "invalid_direct_pair_output_namespace"}
	}
	abs, err := filepath.Abs(outputPath)
	if err != nil {
		return nil, fmt.Errorf("resolve direct pair output namespace: %w", err)
	}
	abs = filepath.Clean(abs)
	if abs == "" || abs == "." {
		return nil, &traceDBOutputInvariantError{Reason: "invalid_direct_pair_output_namespace"}
	}
	return &directPairCaptureBarrier{
		source:          abs,
		maxObservations: directPairBarrierMaxObservations,
		maxLaneKeys:     directPairBarrierMaxLaneKeys,
		maxRows:         directPairBarrierMaxObservations,
		poisonedKinds:   map[pairRenderKind]bool{}, poisonedLanes: map[string]bool{},
		seenLanes: map[pairRenderKind]map[string]struct{}{}, rows: map[int]directPairBarrierRow{},
	}, nil
}

func (barrier *directPairCaptureBarrier) observe(audit directPairLineAudit) {
	if barrier == nil || !audit.Governed {
		return
	}
	barrier.observations++
	if barrier.observations > barrier.maxObservations {
		barrier.failBudget()
		return
	}
	if barrier.poisonedKinds[audit.Kind] {
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
	if audit.HeaderOwnerKnown && audit.Verdict.KeyKnown && laneKnown {
		barrier.poisonedLanes[lane] = true
		return
	}
	barrier.poisonedKinds[audit.Kind] = true
}

func (barrier *directPairCaptureBarrier) addPublishedRow(seq int, audit directPairLineAudit) {
	if barrier == nil || !audit.Governed || !audit.EndpointAdmitted || seq < 0 {
		return
	}
	if barrier.poisonedKinds[audit.Kind] {
		// Freeze will suppress this whole family; retaining per-row mappings
		// would spend proof budget without adding any locality guarantee.
		return
	}
	lane, ok := pairingEndpointLaneKey(audit.Verdict, barrier.source)
	if !ok {
		barrier.failBudget()
		return
	}
	if barrier.budgetFailed {
		return
	}
	if int64(len(barrier.rows)) >= barrier.maxRows {
		barrier.failBudget()
		return
	}
	if _, exists := barrier.rows[seq]; exists {
		barrier.failBudget()
		return
	}
	barrier.rows[seq] = directPairBarrierRow{kind: audit.Kind, lane: lane}
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
}

func (barrier *directPairCaptureBarrier) filter(rows []renderedRow) []renderedRow {
	if barrier == nil || len(rows) == 0 {
		return rows
	}
	// First prove that every pair-critical publisher row is represented by the
	// frozen barrier. A missing/mismatched mapping is an authority failure: if
	// it were handled row-locally, the remaining endpoints could still pair
	// across the invisible physical row. Close both families before emitting
	// any pair row; inventory remains independent.
	for _, row := range rows {
		if row.pairKind == pairRenderUnknown || barrier.poisonedKinds[row.pairKind] {
			continue
		}
		pair, ok := barrier.rows[row.seq]
		if !ok || pair.kind != row.pairKind || pair.lane == "" {
			barrier.failBudget()
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
		if barrier.budgetFailed || barrier.poisonedKinds[row.pairKind] || !laneKnown ||
			pair.kind != row.pairKind || barrier.poisonedLanes[pair.lane] {
			barrier.poisonedRows++
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
	if barrier.laneKeys >= barrier.maxLaneKeys {
		// The full pair-critical set can no longer be represented. This is a
		// global proof-budget failure, not merely a busy family: publishing
		// the other family would turn resource pressure into source-dependent
		// correctness. Close both families and disclose the budget barrier.
		barrier.failBudget()
		return false
	}
	items[lane] = struct{}{}
	barrier.laneKeys++
	return true
}

func (barrier *directPairCaptureBarrier) failBudget() {
	barrier.budgetFailed = true
	barrier.poisonedKinds[pairRenderWorkqueue] = true
	barrier.poisonedKinds[pairRenderDMAFence] = true
	barrier.seenLanes = nil
	barrier.poisonedLanes = nil
}

func (barrier *directPairCaptureBarrier) poisonedFamilyCount() int {
	if barrier == nil {
		return 0
	}
	count := 0
	for _, kind := range []pairRenderKind{pairRenderWorkqueue, pairRenderDMAFence} {
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
	return len(barrier.poisonedLanes)
}
