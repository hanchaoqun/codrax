package hitraceconv

import (
	"math"
	"reflect"
	"testing"
)

func TestProfilerPairFixedLedgerLaneAndFamilyPoisonTransitions(t *testing.T) {
	ledger := profilerPairFixedLedger{}
	if !ledger.pristine() || !ledger.valid() {
		t.Fatal("zero fixed ledger is not pristine and valid")
	}
	lane := profilerPairLaneState{}

	stage := func(endpoint profilerPairEndpointSlot, structured bool, lanePoisoned bool) {
		t.Helper()
		nextLane, laneOK := lane.stageEndpointRows(pairRenderF2FS, endpoint, 1, boolUint32(structured))
		plan, planOK := ledger.planStageRow(pairRenderF2FS, endpoint, structured, lanePoisoned)
		if !laneOK || !planOK {
			t.Fatalf("stage endpoint=%d structured=%t poisoned=%t failed: lane=%t ledger=%t",
				endpoint, structured, lanePoisoned, laneOK, planOK)
		}
		plan.apply(&ledger)
		lane = nextLane
	}

	stage(profilerPairEndpointF2FSWriteBegin, true, false)
	stage(profilerPairEndpointF2FSWriteEnd, false, false)
	assertProfilerPairFixedFamily(t, ledger, pairRenderF2FS, profilerPairFixedFamilyLedger{
		profilerPairFixedCounts: profilerPairFixedCounts{staged: 2, structured: 1},
	})

	poisonPlan, ok := ledger.planPoisonLane(pairRenderF2FS, lane)
	if !ok {
		t.Fatal("first exact-lane poison plan failed")
	}
	poisonPlan.apply(&ledger, &lane)
	if !lane.poisoned {
		t.Fatal("lane poison plan did not commit the idempotency bit")
	}
	assertProfilerPairFixedFamily(t, ledger, pairRenderF2FS, profilerPairFixedFamilyLedger{
		profilerPairFixedCounts: profilerPairFixedCounts{
			staged: 2, structured: 1, withheld: 2, structuredWithheld: 1,
		},
	})
	assertProfilerPairFixedEndpoint(t, ledger, profilerPairEndpointF2FSWriteBegin,
		profilerPairFixedCounts{staged: 1, structured: 1, withheld: 1, structuredWithheld: 1})
	assertProfilerPairFixedEndpoint(t, ledger, profilerPairEndpointF2FSWriteEnd,
		profilerPairFixedCounts{staged: 1, withheld: 1})

	beforeRepeatLedger, beforeRepeatLane := ledger, lane
	repeat, ok := ledger.planPoisonLane(pairRenderF2FS, lane)
	if !ok {
		t.Fatal("repeated lane poison did not produce an idempotent plan")
	}
	repeat.apply(&ledger, &lane)
	if ledger != beforeRepeatLedger || lane != beforeRepeatLane {
		t.Fatalf("repeated lane poison changed state: ledger=%+v lane=%+v", ledger, lane)
	}

	// Rows arriving after an exact-lane poison remain represented in the lane
	// endpoint account and are withheld at stage time; no second poison fold is
	// needed.
	stage(profilerPairEndpointF2FSWriteBegin, true, lane.poisoned)
	assertProfilerPairFixedFamily(t, ledger, pairRenderF2FS, profilerPairFixedFamilyLedger{
		profilerPairFixedCounts: profilerPairFixedCounts{
			staged: 3, structured: 2, withheld: 3, structuredWithheld: 2,
		},
	})

	// A clean sibling lane may still publish until the family itself closes.
	plan, ok := ledger.planStageRow(pairRenderF2FS, profilerPairEndpointF2FSWriteEnd, false, false)
	if !ok {
		t.Fatal("clean sibling stage failed")
	}
	plan.apply(&ledger)
	assertProfilerPairFixedFamily(t, ledger, pairRenderF2FS, profilerPairFixedFamilyLedger{
		profilerPairFixedCounts: profilerPairFixedCounts{
			staged: 4, structured: 2, withheld: 3, structuredWithheld: 2,
		},
	})

	plan, ok = ledger.planPoisonFamily(pairRenderF2FS)
	if !ok {
		t.Fatal("whole-family poison plan failed")
	}
	plan.apply(&ledger)
	assertProfilerPairFixedFamily(t, ledger, pairRenderF2FS, profilerPairFixedFamilyLedger{
		profilerPairFixedCounts: profilerPairFixedCounts{
			staged: 4, structured: 2, withheld: 4, structuredWithheld: 2,
		},
		poisoned: true,
	})
	assertProfilerPairFixedEndpoint(t, ledger, profilerPairEndpointF2FSWriteEnd,
		profilerPairFixedCounts{staged: 2, withheld: 2})

	// Family poison never discards endpoint totals. Every later row increments
	// both staged and withheld even when the caller has no lane state to retain.
	plan, ok = ledger.planStageRow(pairRenderF2FS, profilerPairEndpointF2FSWriteEnd, false, false)
	if !ok {
		t.Fatal("post-family-poison row stage failed")
	}
	plan.apply(&ledger)
	assertProfilerPairFixedFamily(t, ledger, pairRenderF2FS, profilerPairFixedFamilyLedger{
		profilerPairFixedCounts: profilerPairFixedCounts{
			staged: 5, structured: 2, withheld: 5, structuredWithheld: 2,
		},
		poisoned: true,
	})
	assertProfilerPairFixedEndpoint(t, ledger, profilerPairEndpointF2FSWriteEnd,
		profilerPairFixedCounts{staged: 3, withheld: 3})

	beforeRepeatLedger = ledger
	plan, ok = ledger.planPoisonFamily(pairRenderF2FS)
	if !ok {
		t.Fatal("repeated family poison plan failed")
	}
	plan.apply(&ledger)
	if ledger != beforeRepeatLedger {
		t.Fatal("repeated family poison was not idempotent")
	}
}

func TestProfilerPairFixedLedgerOpacityAndNonProfilerFamilies(t *testing.T) {
	ledger := profilerPairFixedLedger{}
	plan, ok := ledger.planMarkOpaque(pairRenderMMC)
	if !ok {
		t.Fatal("empty family opacity plan failed")
	}
	plan.apply(&ledger)
	assertProfilerPairFixedFamily(t, ledger, pairRenderMMC, profilerPairFixedFamilyLedger{opaque: true})

	// The first row after an empty opaque observation atomically closes the
	// family, while retaining its exact endpoint account.
	plan, ok = ledger.planStageRow(pairRenderMMC, profilerPairEndpointMMCRequestStart, true, false)
	if !ok {
		t.Fatal("stage after opacity failed")
	}
	plan.apply(&ledger)
	assertProfilerPairFixedFamily(t, ledger, pairRenderMMC, profilerPairFixedFamilyLedger{
		profilerPairFixedCounts: profilerPairFixedCounts{
			staged: 1, structured: 1, withheld: 1, structuredWithheld: 1,
		},
		poisoned: true, opaque: true,
	})

	plan, ok = ledger.planStageRow(pairRenderWorkqueue, profilerPairEndpointNone, false, false)
	if !ok {
		t.Fatal("non-profiler pair family stage failed")
	}
	plan.apply(&ledger)
	plan, ok = ledger.planMarkOpaque(pairRenderWorkqueue)
	if !ok {
		t.Fatal("populated non-profiler opacity plan failed")
	}
	plan.apply(&ledger)
	assertProfilerPairFixedFamily(t, ledger, pairRenderWorkqueue, profilerPairFixedFamilyLedger{
		profilerPairFixedCounts: profilerPairFixedCounts{staged: 1, withheld: 1},
		poisoned:                true, opaque: true,
	})

	for _, invalid := range []struct {
		name         string
		kind         pairRenderKind
		endpoint     profilerPairEndpointSlot
		structured   bool
		lanePoisoned bool
	}{
		{name: "unknown family", kind: pairRenderUnknown},
		{name: "sentinel family", kind: pairRenderKindCount},
		{name: "profiler endpoint missing", kind: pairRenderF2FS},
		{name: "foreign endpoint", kind: pairRenderMMC, endpoint: profilerPairEndpointF2FSWriteBegin},
		{name: "nonprofiler endpoint", kind: pairRenderDMAFence, endpoint: profilerPairEndpointBlockRQIssue},
		{name: "nonprofiler structured", kind: pairRenderDMAFence, structured: true},
		{name: "nonprofiler lane poison", kind: pairRenderDMAFence, lanePoisoned: true},
	} {
		t.Run(invalid.name, func(t *testing.T) {
			before := ledger
			if rejected, accepted := ledger.planStageRow(invalid.kind, invalid.endpoint,
				invalid.structured, invalid.lanePoisoned); accepted || rejected != (profilerPairFixedLedgerPlan{}) || ledger != before {
				t.Fatalf("invalid stage admitted or mutated receiver: accepted=%t plan=%+v", accepted, rejected)
			}
		})
	}
	if _, ok := ledger.family(pairRenderUnknown); ok {
		t.Fatal("unknown family exposed by fixed ledger")
	}
	if _, ok := ledger.endpoint(profilerPairEndpointNone); ok {
		t.Fatal("none endpoint exposed by fixed ledger")
	}
	if _, ok := ledger.endpoint(profilerPairEndpointSlotCount); ok {
		t.Fatal("endpoint sentinel exposed by fixed ledger")
	}
}

func TestProfilerPairFixedLedgerCheckedOverflowAndCorruptionFailPlanning(t *testing.T) {
	maxed := profilerPairFixedLedger{}
	maxed.families[pairRenderF2FS].staged = math.MaxInt
	maxed.endpoints[profilerPairEndpointF2FSWriteBegin].staged = math.MaxInt
	if !maxed.valid() {
		t.Fatal("consistent MaxInt ledger fixture is invalid")
	}
	before := maxed
	if plan, ok := maxed.planStageRow(pairRenderF2FS, profilerPairEndpointF2FSWriteBegin, false, false); ok ||
		plan != (profilerPairFixedLedgerPlan{}) || maxed != before {
		t.Fatalf("staged counter overflow was admitted: ok=%t plan=%+v", ok, plan)
	}

	// This state is locally and globally valid but has no withheld capacity for
	// another lane. The checked fold must fail before either receiver changes.
	fullWithheld := maxed
	fullWithheld.families[pairRenderF2FS].withheld = math.MaxInt
	fullWithheld.endpoints[profilerPairEndpointF2FSWriteBegin].withheld = math.MaxInt
	lane := profilerPairLaneState{}
	lane.endpointCounts[0].rows = 1
	if !fullWithheld.valid() || !lane.endpointCountsValid(pairRenderF2FS) {
		t.Fatal("lane poison overflow fixture is invalid")
	}
	beforeLedger, beforeLane := fullWithheld, lane
	if plan, ok := fullWithheld.planPoisonLane(pairRenderF2FS, lane); ok ||
		plan != (profilerPairFixedLanePoisonPlan{}) || fullWithheld != beforeLedger || lane != beforeLane {
		t.Fatalf("lane fold overflow was admitted: ok=%t plan=%+v", ok, plan)
	}

	corruptions := []struct {
		name              string
		localKind         pairRenderKind
		expectLocalReject bool
		mutate            func(*profilerPairFixedLedger)
	}{
		{name: "unknown family residue", mutate: func(value *profilerPairFixedLedger) {
			value.families[pairRenderUnknown].staged = 1
		}},
		{name: "none endpoint residue", mutate: func(value *profilerPairFixedLedger) {
			value.endpoints[profilerPairEndpointNone].staged = 1
		}},
		{name: "negative family", localKind: pairRenderWorkqueue, expectLocalReject: true, mutate: func(value *profilerPairFixedLedger) {
			value.families[pairRenderWorkqueue].staged = -1
		}},
		{name: "structured exceeds staged", localKind: pairRenderWorkqueue, expectLocalReject: true, mutate: func(value *profilerPairFixedLedger) {
			value.families[pairRenderWorkqueue].structured = 1
		}},
		{name: "structured withheld intersection impossible", localKind: pairRenderWorkqueue, expectLocalReject: true, mutate: func(value *profilerPairFixedLedger) {
			value.families[pairRenderWorkqueue].profilerPairFixedCounts = profilerPairFixedCounts{
				staged: 1, structured: 1, withheld: 1,
			}
		}},
		{name: "partial family poison", localKind: pairRenderWorkqueue, expectLocalReject: true, mutate: func(value *profilerPairFixedLedger) {
			value.families[pairRenderWorkqueue] = profilerPairFixedFamilyLedger{
				profilerPairFixedCounts: profilerPairFixedCounts{staged: 1}, poisoned: true,
			}
		}},
		{name: "opaque rows not closed", localKind: pairRenderWorkqueue, expectLocalReject: true, mutate: func(value *profilerPairFixedLedger) {
			value.families[pairRenderWorkqueue] = profilerPairFixedFamilyLedger{
				profilerPairFixedCounts: profilerPairFixedCounts{staged: 1}, opaque: true,
			}
		}},
		{name: "endpoint family mismatch", localKind: pairRenderBlock, expectLocalReject: true, mutate: func(value *profilerPairFixedLedger) {
			value.families[pairRenderBlock].staged = 1
		}},
		{name: "endpoint subset invalid", localKind: pairRenderBlock, expectLocalReject: true, mutate: func(value *profilerPairFixedLedger) {
			value.families[pairRenderBlock] = profilerPairFixedFamilyLedger{
				profilerPairFixedCounts: profilerPairFixedCounts{staged: 1},
			}
			value.endpoints[profilerPairEndpointBlockBIOQueue] = profilerPairFixedCounts{
				staged: 1, withheld: 2,
			}
		}},
	}
	for _, corruption := range corruptions {
		t.Run(corruption.name, func(t *testing.T) {
			value := profilerPairFixedLedger{}
			corruption.mutate(&value)
			if value.valid() {
				t.Fatalf("corrupt ledger remained valid: %+v", value)
			}
			if corruption.expectLocalReject {
				before := value
				if plan, ok := value.planPoisonFamily(corruption.localKind); ok ||
					plan != (profilerPairFixedLedgerPlan{}) || value != before {
					t.Fatalf("locally corrupt ledger produced a plan: ok=%t plan=%+v", ok, plan)
				}
			}
		})
	}
}

func TestProfilerPairFixedLedgerStorageIsClosedAndCaptureWideCountersAreInts(t *testing.T) {
	assertClosedFixedLedgerType(t, reflect.TypeOf(profilerPairFixedLedger{}), "ledger", true)
	assertClosedFixedLedgerType(t, reflect.TypeOf(profilerPairFixedLedgerPlan{}), "plan", true)
	if got := reflect.TypeOf(profilerPairFixedLedger{}).Field(0).Type.Len(); got != int(pairRenderKindCount) {
		t.Fatalf("family ledger width=%d want=%d", got, pairRenderKindCount)
	}
	if got := reflect.TypeOf(profilerPairFixedLedger{}).Field(1).Type.Len(); got != int(profilerPairEndpointSlotCount) {
		t.Fatalf("endpoint ledger width=%d want=%d", got, profilerPairEndpointSlotCount)
	}
	if size := reflect.TypeOf(profilerPairFixedLedgerPlan{}).Size(); size > 128 {
		t.Fatalf("row/family mutation plan retained whole-ledger scale: size=%d want<=128", size)
	}
	if size := reflect.TypeOf(profilerPairFixedLanePoisonPlan{}).Size(); size > 128 {
		t.Fatalf("lane poison mutation plan exceeded bounded family-local shape: size=%d want<=128", size)
	}
}

func TestProfilerPairFixedLedgerPlansAllocateNothing(t *testing.T) {
	base := profilerPairFixedLedger{}
	stage, ok := base.planStageRow(
		pairRenderF2FS, profilerPairEndpointF2FSWriteBegin, true, false,
	)
	if !ok {
		t.Fatal("seed fixed-ledger stage plan failed")
	}
	stage.apply(&base)
	lane := profilerPairLaneState{}
	lane, ok = lane.stageEndpointRows(
		pairRenderF2FS, profilerPairEndpointF2FSWriteBegin, 1, 1,
	)
	if !ok {
		t.Fatal("seed fixed-lane stage plan failed")
	}
	if allocs := testing.AllocsPerRun(1_000, func() {
		ledgerCopy := base
		laneCopy := lane
		poison, planned := ledgerCopy.planPoisonLane(pairRenderF2FS, laneCopy)
		if !planned {
			panic("fixed lane poison plan failed")
		}
		poison.apply(&ledgerCopy, &laneCopy)
		row, planned := ledgerCopy.planStageRow(
			pairRenderF2FS, profilerPairEndpointF2FSWriteEnd, false, laneCopy.poisoned,
		)
		if !planned {
			panic("fixed row stage plan failed")
		}
		row.apply(&ledgerCopy)
	}); allocs != 0 {
		t.Fatalf("fixed-ledger plans allocated %.2f objects per run", allocs)
	}
}

func assertClosedFixedLedgerType(t *testing.T, typ reflect.Type, path string, captureWide bool) {
	t.Helper()
	switch typ.Kind() {
	case reflect.Struct:
		for index := 0; index < typ.NumField(); index++ {
			field := typ.Field(index)
			assertClosedFixedLedgerType(t, field.Type, path+"."+field.Name, captureWide)
		}
	case reflect.Array:
		assertClosedFixedLedgerType(t, typ.Elem(), path+"[]", captureWide)
	case reflect.Int, reflect.Bool, reflect.Uint8:
		return
	case reflect.Uint32:
		if captureWide {
			t.Fatalf("capture-wide fixed ledger retained uint32 at %s", path)
		}
	default:
		t.Fatalf("fixed ledger retained dynamic or unsupported kind %s at %s", typ.Kind(), path)
	}
}

func assertProfilerPairFixedFamily(
	t *testing.T,
	ledger profilerPairFixedLedger,
	kind pairRenderKind,
	want profilerPairFixedFamilyLedger,
) {
	t.Helper()
	got, ok := ledger.family(kind)
	if !ok || got != want {
		t.Fatalf("family %d=%+v,%t want=%+v ledger=%+v", kind, got, ok, want, ledger)
	}
}

func assertProfilerPairFixedEndpoint(
	t *testing.T,
	ledger profilerPairFixedLedger,
	endpoint profilerPairEndpointSlot,
	want profilerPairFixedCounts,
) {
	t.Helper()
	got, ok := ledger.endpoint(endpoint)
	if !ok || got != want {
		t.Fatalf("endpoint %d=%+v,%t want=%+v ledger=%+v", endpoint, got, ok, want, ledger)
	}
}

func boolUint32(value bool) uint32 {
	if value {
		return 1
	}
	return 0
}
