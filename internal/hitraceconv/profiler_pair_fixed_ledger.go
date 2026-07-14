package hitraceconv

import "math"

// profilerPairFixedCounts is the closed publication account shared by a
// family and each of its exact endpoint slots. All capture-wide totals use
// checked native ints; uint32 is reserved for one exact lane's bounded state.
type profilerPairFixedCounts struct {
	staged             int
	structured         int
	withheld           int
	structuredWithheld int
}

func (counts profilerPairFixedCounts) valid() bool {
	return counts.staged >= 0 && counts.structured >= 0 && counts.withheld >= 0 &&
		counts.structuredWithheld >= 0 && counts.structured <= counts.staged &&
		counts.withheld <= counts.staged && counts.structuredWithheld <= counts.structured &&
		counts.structuredWithheld <= counts.withheld &&
		counts.structured-counts.structuredWithheld <= counts.staged-counts.withheld
}

func (counts profilerPairFixedCounts) stage(structured, withheld bool) (profilerPairFixedCounts, bool) {
	if !counts.valid() {
		return profilerPairFixedCounts{}, false
	}
	next := counts
	if !checkedProfilerPairFixedAddTo(&next.staged, 1) {
		return profilerPairFixedCounts{}, false
	}
	if structured && !checkedProfilerPairFixedAddTo(&next.structured, 1) {
		return profilerPairFixedCounts{}, false
	}
	if withheld && !checkedProfilerPairFixedAddTo(&next.withheld, 1) {
		return profilerPairFixedCounts{}, false
	}
	if structured && withheld && !checkedProfilerPairFixedAddTo(&next.structuredWithheld, 1) {
		return profilerPairFixedCounts{}, false
	}
	if !next.valid() {
		return profilerPairFixedCounts{}, false
	}
	return next, true
}

func checkedProfilerPairFixedAddTo(dst *int, delta int) bool {
	if dst == nil || delta < 0 || *dst < 0 || *dst > math.MaxInt-delta {
		return false
	}
	*dst += delta
	return true
}

type profilerPairFixedFamilyLedger struct {
	profilerPairFixedCounts
	poisoned bool
	opaque   bool
}

// profilerPairFixedLedger is the capture-wide, allocation-free accounting
// authority. Endpoint slot zero is deliberately unused; every other array
// element is tied to the single closed endpoint descriptor roster.
type profilerPairFixedLedger struct {
	families  [pairRenderKindCount]profilerPairFixedFamilyLedger
	endpoints [profilerPairEndpointSlotCount]profilerPairFixedCounts
}

// profilerPairFixedLedgerPlan retains only one affected family and endpoint.
// Whole-family poison is represented as a closed operation bit rather than a
// second 656-byte ledger snapshot. Planning performs every checked addition;
// apply is consequently an infallible bounded assignment at the commit tail.
type profilerPairFixedLedgerPlan struct {
	kind           pairRenderKind
	endpoint       profilerPairEndpointSlot
	nextFamily     profilerPairFixedFamilyLedger
	nextEndpoint   profilerPairFixedCounts
	poisonFamily   bool
	updateEndpoint bool
}

func (plan profilerPairFixedLedgerPlan) apply(ledger *profilerPairFixedLedger) {
	if plan.poisonFamily {
		poisonProfilerPairFixedFamily(ledger, plan.kind)
	}
	ledger.families[plan.kind] = plan.nextFamily
	if plan.updateEndpoint {
		ledger.endpoints[plan.endpoint] = plan.nextEndpoint
	}
}

// profilerPairFixedLanePoisonPlan commits the ledger and exact-lane poison bit
// together. The current lane state is the idempotency token: a repeated plan
// over an already-poisoned state adds no counters a second time.
type profilerPairFixedLanePoisonPlan struct {
	kind       pairRenderKind
	nextFamily profilerPairFixedFamilyLedger
	nextLane   profilerPairLaneState
}

func (plan profilerPairFixedLanePoisonPlan) apply(
	ledger *profilerPairFixedLedger,
	lane *profilerPairLaneState,
) {
	// apply must remain the immediate no-fail tail of planPoisonLane over this
	// exact ledger/lane prestate. The checked plan proved every addition below;
	// no caller may mutate either authority between plan and apply.
	if !lane.poisoned && plan.nextLane.poisoned {
		first, count, _ := profilerPairFamilyEndpointRange(plan.kind)
		for ordinal := uint8(0); ordinal < count; ordinal++ {
			laneCounts := lane.endpointCounts[ordinal]
			slot := first + profilerPairEndpointSlot(ordinal)
			ledger.endpoints[slot].withheld += int(laneCounts.rows)
			ledger.endpoints[slot].structuredWithheld += int(laneCounts.structuredRows)
		}
	}
	ledger.families[plan.kind] = plan.nextFamily
	*lane = plan.nextLane
}

func (ledger *profilerPairFixedLedger) pristine() bool {
	return ledger != nil && *ledger == (profilerPairFixedLedger{})
}

// valid is O(1): both loops range over compile-time closed enum arrays. Besides
// local subset invariants, it proves that each profiler family's account is
// exactly the sum of its endpoint accounts.
func (ledger *profilerPairFixedLedger) valid() bool {
	if ledger == nil {
		return false
	}
	if ledger.families[pairRenderUnknown] != (profilerPairFixedFamilyLedger{}) ||
		ledger.endpoints[profilerPairEndpointNone] != (profilerPairFixedCounts{}) {
		return false
	}
	for kind := pairRenderKind(1); kind < pairRenderKindCount; kind++ {
		if !ledger.validateFamilyEndpoints(kind) {
			return false
		}
	}
	return true
}

func (ledger *profilerPairFixedLedger) familyLocallyValid(kind pairRenderKind) bool {
	if ledger == nil || kind == pairRenderUnknown || !profilerPairKindValid(kind) {
		return false
	}
	return profilerPairFixedFamilyLocallyValid(ledger.families[kind])
}

func profilerPairFixedFamilyLocallyValid(family profilerPairFixedFamilyLedger) bool {
	return family.profilerPairFixedCounts.valid() &&
		(!family.poisoned || family.withheld == family.staged &&
			family.structuredWithheld == family.structured) &&
		(!family.opaque || family.staged == 0 || family.poisoned)
}

// validateFamilyEndpoints is the non-hot family parity check. It visits at
// most the six slots in one closed family and never scans the global roster.
func (ledger *profilerPairFixedLedger) validateFamilyEndpoints(kind pairRenderKind) bool {
	if !ledger.familyLocallyValid(kind) {
		return false
	}
	if !profilerPairBudgetKind(kind) {
		return true
	}
	first, count, ok := profilerPairFamilyEndpointRange(kind)
	if !ok || count == 0 || count > profilerPairFamilyEndpointCapacity {
		return false
	}
	var sum profilerPairFixedCounts
	for offset := uint8(0); offset < count; offset++ {
		counts := ledger.endpoints[first+profilerPairEndpointSlot(offset)]
		if !counts.valid() ||
			ledger.families[kind].poisoned &&
				(counts.withheld != counts.staged || counts.structuredWithheld != counts.structured) ||
			!addProfilerPairFixedCounts(&sum, counts) {
			return false
		}
	}
	return sum == ledger.families[kind].profilerPairFixedCounts
}

func (ledger *profilerPairFixedLedger) affectedEndpointValid(
	kind pairRenderKind,
	endpoint profilerPairEndpointSlot,
) bool {
	if !ledger.familyLocallyValid(kind) {
		return false
	}
	if !profilerPairBudgetKind(kind) {
		return endpoint == profilerPairEndpointNone
	}
	first, count, ok := profilerPairFamilyEndpointRange(kind)
	if !ok || endpoint < first || uint8(endpoint-first) >= count {
		return false
	}
	counts := ledger.endpoints[endpoint]
	return profilerPairFixedEndpointWithinFamily(ledger.families[kind], counts)
}

func profilerPairFixedEndpointWithinFamily(
	family profilerPairFixedFamilyLedger,
	counts profilerPairFixedCounts,
) bool {
	return profilerPairFixedFamilyLocallyValid(family) && counts.valid() &&
		counts.staged <= family.staged && counts.structured <= family.structured &&
		counts.withheld <= family.withheld && counts.structuredWithheld <= family.structuredWithheld &&
		(!family.poisoned || counts.withheld == counts.staged &&
			counts.structuredWithheld == counts.structured)
}

func addProfilerPairFixedCounts(dst *profilerPairFixedCounts, delta profilerPairFixedCounts) bool {
	if dst == nil || !dst.valid() || !delta.valid() {
		return false
	}
	next := *dst
	if !checkedProfilerPairFixedAddTo(&next.staged, delta.staged) ||
		!checkedProfilerPairFixedAddTo(&next.structured, delta.structured) ||
		!checkedProfilerPairFixedAddTo(&next.withheld, delta.withheld) ||
		!checkedProfilerPairFixedAddTo(&next.structuredWithheld, delta.structuredWithheld) ||
		!next.valid() {
		return false
	}
	*dst = next
	return true
}

func (ledger *profilerPairFixedLedger) family(
	kind pairRenderKind,
) (profilerPairFixedFamilyLedger, bool) {
	if !ledger.validateFamilyEndpoints(kind) {
		return profilerPairFixedFamilyLedger{}, false
	}
	return ledger.families[kind], true
}

func (ledger *profilerPairFixedLedger) endpoint(
	slot profilerPairEndpointSlot,
) (profilerPairFixedCounts, bool) {
	if slot == profilerPairEndpointNone || slot >= profilerPairEndpointSlotCount {
		return profilerPairFixedCounts{}, false
	}
	descriptor, ok := slot.descriptor()
	if !ok || !ledger.validateFamilyEndpoints(descriptor.kind) {
		return profilerPairFixedCounts{}, false
	}
	return ledger.endpoints[slot], true
}

// planStageRow accounts one already-admitted pair row. lanePoisoned is the
// caller's typed exact-lane verdict. Whole-family poison and opacity dominate
// it: every later row remains staged but is immediately withheld.
func (ledger *profilerPairFixedLedger) planStageRow(
	kind pairRenderKind,
	endpoint profilerPairEndpointSlot,
	structured bool,
	lanePoisoned bool,
) (profilerPairFixedLedgerPlan, bool) {
	if kind == pairRenderUnknown || !profilerPairKindValid(kind) ||
		!ledger.affectedEndpointValid(kind, endpoint) {
		return profilerPairFixedLedgerPlan{}, false
	}
	if !profilerPairBudgetKind(kind) &&
		(endpoint != profilerPairEndpointNone || structured || lanePoisoned) {
		return profilerPairFixedLedgerPlan{}, false
	}

	nextFamily := ledger.families[kind]
	nextEndpoint := profilerPairFixedCounts{}
	updateEndpoint := profilerPairBudgetKind(kind)
	if updateEndpoint {
		nextEndpoint = ledger.endpoints[endpoint]
	}
	poisonFamily := nextFamily.opaque && !nextFamily.poisoned
	if poisonFamily {
		if !ledger.validateFamilyEndpoints(kind) {
			return profilerPairFixedLedgerPlan{}, false
		}
		nextFamily.poisoned = true
		nextFamily.withheld = nextFamily.staged
		nextFamily.structuredWithheld = nextFamily.structured
		if updateEndpoint {
			nextEndpoint.withheld = nextEndpoint.staged
			nextEndpoint.structuredWithheld = nextEndpoint.structured
		}
	}
	withheld := nextFamily.poisoned || lanePoisoned
	familyCounts, ok := nextFamily.profilerPairFixedCounts.stage(structured, withheld)
	if !ok {
		return profilerPairFixedLedgerPlan{}, false
	}
	nextFamily.profilerPairFixedCounts = familyCounts
	if updateEndpoint {
		endpointCounts, endpointOK := nextEndpoint.stage(structured, withheld)
		if !endpointOK {
			return profilerPairFixedLedgerPlan{}, false
		}
		nextEndpoint = endpointCounts
	}
	if !profilerPairFixedFamilyLocallyValid(nextFamily) ||
		updateEndpoint && !profilerPairFixedEndpointWithinFamily(nextFamily, nextEndpoint) {
		return profilerPairFixedLedgerPlan{}, false
	}
	return profilerPairFixedLedgerPlan{
		kind: kind, endpoint: endpoint, nextFamily: nextFamily, nextEndpoint: nextEndpoint,
		poisonFamily: poisonFamily, updateEndpoint: updateEndpoint,
	}, true
}

// planPoisonFamily converts every row already staged in the family and its
// endpoint accounts to withheld without discarding endpoint totals. Repeating
// the transition is an exact no-op plan.
func (ledger *profilerPairFixedLedger) planPoisonFamily(
	kind pairRenderKind,
) (profilerPairFixedLedgerPlan, bool) {
	if kind == pairRenderUnknown || !profilerPairKindValid(kind) ||
		!ledger.validateFamilyEndpoints(kind) {
		return profilerPairFixedLedgerPlan{}, false
	}
	nextFamily := ledger.families[kind]
	nextFamily.poisoned = true
	nextFamily.withheld = nextFamily.staged
	nextFamily.structuredWithheld = nextFamily.structured
	return profilerPairFixedLedgerPlan{
		kind: kind, nextFamily: nextFamily, poisonFamily: true,
	}, true
}

func poisonProfilerPairFixedFamily(ledger *profilerPairFixedLedger, kind pairRenderKind) {
	family := &ledger.families[kind]
	family.poisoned = true
	family.withheld = family.staged
	family.structuredWithheld = family.structured
	first, count, ok := profilerPairFamilyEndpointRange(kind)
	if !ok {
		return
	}
	for offset := uint8(0); offset < count; offset++ {
		slot := first + profilerPairEndpointSlot(offset)
		ledger.endpoints[slot].withheld = ledger.endpoints[slot].staged
		ledger.endpoints[slot].structuredWithheld = ledger.endpoints[slot].structured
	}
}

// planMarkOpaque retains opacity as an independent family fact. If rows
// already exist it closes the family immediately; otherwise the first later
// row closes and stages itself atomically in planStageRow.
func (ledger *profilerPairFixedLedger) planMarkOpaque(
	kind pairRenderKind,
) (profilerPairFixedLedgerPlan, bool) {
	if kind == pairRenderUnknown || !profilerPairKindValid(kind) ||
		!ledger.validateFamilyEndpoints(kind) {
		return profilerPairFixedLedgerPlan{}, false
	}
	nextFamily := ledger.families[kind]
	nextFamily.opaque = true
	poisonFamily := nextFamily.staged > 0
	if poisonFamily {
		nextFamily.poisoned = true
		nextFamily.withheld = nextFamily.staged
		nextFamily.structuredWithheld = nextFamily.structured
	}
	if !profilerPairFixedFamilyLocallyValid(nextFamily) {
		return profilerPairFixedLedgerPlan{}, false
	}
	return profilerPairFixedLedgerPlan{
		kind: kind, nextFamily: nextFamily, poisonFamily: poisonFamily,
	}, true
}

// planPoisonLane folds one exact lane's fixed six-slot account into the
// capture-wide withheld totals exactly once. A current poisoned bit yields an
// idempotent no-op plan; the caller need not maintain any second lane set.
func (ledger *profilerPairFixedLedger) planPoisonLane(
	kind pairRenderKind,
	lane profilerPairLaneState,
) (profilerPairFixedLanePoisonPlan, bool) {
	if !profilerPairBudgetKind(kind) || !ledger.validateFamilyEndpoints(kind) ||
		!lane.endpointCountsValid(kind) {
		return profilerPairFixedLanePoisonPlan{}, false
	}
	if ledger.families[kind].poisoned || lane.poisoned {
		return profilerPairFixedLanePoisonPlan{
			kind: kind, nextFamily: ledger.families[kind], nextLane: lane,
		}, true
	}

	nextFamily := ledger.families[kind]
	nextEndpoints := [profilerPairFamilyEndpointCapacity]profilerPairFixedCounts{}
	nextLane := lane
	first, count, rangeOK := profilerPairFamilyEndpointRange(kind)
	if !rangeOK || count == 0 || count > profilerPairFamilyEndpointCapacity {
		return profilerPairFixedLanePoisonPlan{}, false
	}
	for ordinal := uint8(0); ordinal < count; ordinal++ {
		slot := first + profilerPairEndpointSlot(ordinal)
		laneCounts := lane.endpointCounts[ordinal]
		nextEndpoints[ordinal] = ledger.endpoints[slot]
		if !checkedProfilerPairFixedAddTo(&nextFamily.withheld, int(laneCounts.rows)) ||
			!checkedProfilerPairFixedAddTo(&nextFamily.structuredWithheld, int(laneCounts.structuredRows)) ||
			!checkedProfilerPairFixedAddTo(&nextEndpoints[ordinal].withheld, int(laneCounts.rows)) ||
			!checkedProfilerPairFixedAddTo(&nextEndpoints[ordinal].structuredWithheld, int(laneCounts.structuredRows)) {
			return profilerPairFixedLanePoisonPlan{}, false
		}
	}
	nextLane.poisoned = true
	if !profilerPairFixedFamilyLocallyValid(nextFamily) {
		return profilerPairFixedLanePoisonPlan{}, false
	}
	for ordinal := uint8(0); ordinal < count; ordinal++ {
		if !profilerPairFixedEndpointWithinFamily(nextFamily, nextEndpoints[ordinal]) {
			return profilerPairFixedLanePoisonPlan{}, false
		}
	}
	return profilerPairFixedLanePoisonPlan{
		kind: kind, nextFamily: nextFamily, nextLane: nextLane,
	}, true
}
