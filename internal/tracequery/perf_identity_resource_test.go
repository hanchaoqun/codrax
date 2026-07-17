package tracequery

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"runtime"
	"runtime/debug"
	"sort"
	"sync"
	"testing"
)

func TestPerfGenerationEventCandidateTIDsAllocFree(t *testing.T) {
	candidates := map[int]bool{7: true, 8: true, 9: true, 10: true}
	ev := Event{
		Type:     EventSchedSwitch,
		PID:      7,
		PrevPID:  8,
		NextPID:  9,
		WakeePID: 10,
	}
	var sink perfGenerationCandidateTIDSet
	if got := testing.AllocsPerRun(1_000, func() {
		sink = perfGenerationEventCandidateTIDs(ev, candidates)
	}); got != 0 {
		t.Fatalf("candidate-role intersection allocated %.2f object(s)/call, want zero", got)
	}
	if sink.count != 3 {
		t.Fatalf("sched_switch candidate-role projection drifted: %+v", sink)
	}
}

func TestPerfIdentityPrebuildCancellationIsAtomicAndRetryable(t *testing.T) {
	const sampleCount = 16_384
	idx := &Index{Events: perfIdentityResourceDistinctSamples(sampleCount)}

	// The first Err consultation passes and the first cancellable build-loop
	// checkpoint fires. A failed private build must publish neither a partial
	// pointer nor a consumed sync.Once.
	err := prebuildPerfIdentityLedger(newRunCancelAfterN(1), idx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled prebuild error=%v, want context.Canceled", err)
	}
	if idx.perfIdentity != nil {
		t.Fatalf("canceled prebuild published partial ledger: %+v", idx.perfIdentity)
	}

	if err := prebuildPerfIdentityLedger(context.Background(), idx); err != nil {
		t.Fatalf("healthy retry after cancellation failed: %v", err)
	}
	ledger := ensurePerfIdentityLedger(idx)
	if ledger != idx.perfIdentity || len(ledger.records) != sampleCount || len(ledger.bindings) != sampleCount {
		t.Fatalf("healthy retry did not publish one complete ledger: ptr_same=%t records=%d bindings=%d want=%d",
			ledger == idx.perfIdentity, len(ledger.records), len(ledger.bindings), sampleCount)
	}
	for _, ordinal := range []int{0, sampleCount / 2, sampleCount - 1} {
		_, identity, ok := ledger.identityForEventOrdinalBorrowed(ordinal)
		if !ok || identity.TID != ordinal+1 || identity.Generation != 1 {
			t.Fatalf("retry ledger incomplete at ordinal=%d: ok=%t identity=%+v", ordinal, ok, identity)
		}
	}
}

func TestPerfIdentityCaveatCollectorIsBoundedAndDeterministic(t *testing.T) {
	const distinct = 1_000
	ascending := make([]string, distinct)
	for i := range ascending {
		ascending[i] = fmt.Sprintf("perf_test_caveat=%04d", i)
	}

	build := func(order []int) (*perfIdentityCaveatBuilder, []string) {
		builder := &perfIdentityCaveatBuilder{}
		for _, i := range order {
			builder.add(ascending[i])
			builder.add(ascending[i]) // duplicates must consume no retained budget
		}
		return builder, builder.finish()
	}
	reverse := make([]int, distinct)
	permuted := make([]int, distinct)
	for i := 0; i < distinct; i++ {
		reverse[i] = distinct - 1 - i
		// 37 is coprime with 1000, so this visits every item exactly once.
		permuted[i] = i * 37 % distinct
	}

	leftBuilder, left := build(reverse)
	_, right := build(permuted)
	if !reflect.DeepEqual(left, right) {
		t.Fatalf("caveat projection depends on insertion order:\nreverse=%v\npermuted=%v", left, right)
	}
	if len(leftBuilder.items) != perfIdentityCaveatCap-1 || len(leftBuilder.seen) != perfIdentityCaveatCap-1 {
		t.Fatalf("caveat collector retained an unbounded set: items=%d seen=%d cap=%d",
			len(leftBuilder.items), len(leftBuilder.seen), perfIdentityCaveatCap)
	}
	if len(left) != perfIdentityCaveatCap || left[len(left)-1] != "perf_thread_identity_caveats_compacted=true; omitted_at_least=1" {
		t.Fatalf("compacted caveat disclosure drifted: %v", left)
	}
	for i := 0; i < perfIdentityCaveatCap-1; i++ {
		if left[i] != ascending[i] {
			t.Fatalf("bounded projection is not the deterministic lexical prefix at %d: got=%q want=%q", i, left[i], ascending[i])
		}
	}
}

func TestPerfIdentityEnsureConcurrentSingleCompletePublication(t *testing.T) {
	const (
		goroutines  = 64
		sampleCount = 2_048
	)
	idx := &Index{Events: perfIdentityResourceDistinctSamples(sampleCount)}
	start := make(chan struct{})
	results := make(chan *perfIdentityLedger, goroutines)
	var ready sync.WaitGroup
	var done sync.WaitGroup
	ready.Add(goroutines)
	done.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer done.Done()
			ready.Done()
			<-start
			results <- ensurePerfIdentityLedger(idx)
		}()
	}
	ready.Wait()
	close(start)
	done.Wait()
	close(results)

	var published *perfIdentityLedger
	for ledger := range results {
		if ledger == nil || len(ledger.records) != sampleCount || len(ledger.bindings) != sampleCount {
			t.Fatalf("concurrent ensure observed incomplete ledger: %+v", ledger)
		}
		if published == nil {
			published = ledger
		} else if ledger != published {
			t.Fatalf("concurrent ensure published multiple ledger pointers: first=%p got=%p", published, ledger)
		}
	}
	if published == nil || published != idx.perfIdentity {
		t.Fatalf("published pointer was not installed on Index: result=%p index=%p", published, idx.perfIdentity)
	}
}

func TestPerfIdentityHighCardinalityResourceBudgets(t *testing.T) {
	const (
		sampleCount = 32_000
		// Measured at 926 B/row on the 32k fixture. 1536 leaves ample
		// allocator/runtime headroom while mechanically rejecting the prior
		// ~1.52 KiB/row representation and any return to per-cohort heap
		// objects or duplicate dense ordinal sidecars.
		maxTotalAllocPerRow = uint64(1_536)
	)
	idx := &Index{Events: perfIdentityResourceDistinctSamples(sampleCount)}
	previousGCPercent := debug.SetGCPercent(-1)
	defer debug.SetGCPercent(previousGCPercent)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	if err := prebuildPerfIdentityLedger(context.Background(), idx); err != nil {
		t.Fatalf("high-cardinality prebuild failed: %v", err)
	}
	runtime.ReadMemStats(&after)

	ledger := idx.perfIdentity
	if ledger == nil || len(ledger.records) != sampleCount || len(ledger.bindings) != sampleCount {
		t.Fatalf("high-cardinality ledger incomplete: ledger_nil=%t records=%d bindings=%d want=%d",
			ledger == nil, perfIdentityResourceLenRecords(ledger), perfIdentityResourceLenBindings(ledger), sampleCount)
	}
	totalAlloc := after.TotalAlloc - before.TotalAlloc
	perRow := totalAlloc / sampleCount
	t.Logf("PERF-NORMALIZATION-IDENTITY-a2 resource evidence: samples=%d total_alloc=%d bytes total_alloc_per_sample=%d bytes retained=%d bytes retained_per_sample=%.2f bytes",
		sampleCount, totalAlloc, perRow, ledger.retainedBytes(), float64(ledger.retainedBytes())/sampleCount)
	if perRow > maxTotalAllocPerRow {
		t.Fatalf("ledger build allocation=%d bytes/sample exceeds conservative %d-byte budget", perRow, maxTotalAllocPerRow)
	}
	if retained := ledger.retainedBytes(); retained > int64(sampleCount)*perfIdentityLedgerReservedBytesPerSample {
		t.Fatalf("retained ledger=%d bytes exceeds prepaid cache reserve=%d bytes (%d/sample)",
			retained, int64(sampleCount)*perfIdentityLedgerReservedBytesPerSample, perfIdentityLedgerReservedBytesPerSample)
	}
}

func TestPerfBoundedTopKMatchesFullReferenceAcrossInsertionOrders(t *testing.T) {
	const (
		cardinality = 64
		limit       = 8
	)
	rng := rand.New(rand.NewSource(0xC0D2A))
	for run := 0; run < 24; run++ {
		order := rng.Perm(cardinality)
		total := int64(cardinality)
		hotspots := make(map[string]*perfHotspotAcc, cardinality)
		threads := make(map[perfThreadKey]*perfThreadAcc, cardinality)
		var identities perfThreadIdentitySet
		for _, value := range order {
			hotspotKey := fmt.Sprintf("callchain-%03d", value)
			identity := PerfThreadIdentity{TID: value + 1, TGID: 7, Generation: 1, DisplayComm: fmt.Sprintf("worker-%03d", value), CommAliases: []string{fmt.Sprintf("alias-%03d", value)}}
			threadKey := perfThreadKey{scope: fmt.Sprintf("scope-%03d", value), TID: identity.TID, Generation: 1}
			var threadSet perfThreadIdentitySet
			threadSet.add(threadKey, identity)
			var cpuSet perfCPUSet
			cpuSet.add(value % 4)
			hotspots[hotspotKey] = &perfHotspotAcc{
				// Every visible label and rank metric ties. The private
				// aggregation key must make the result deterministic.
				item:      PerfHotspot{Symbol: "shared-visible-label", Callchain: hotspotKey, Example: hotspotKey, Period: 10, SampleCount: 1},
				threadSet: threadSet,
				cpuSet:    cpuSet,
				total:     &total,
			}

			// All public identity fields tie; the private capture scope is the
			// final deterministic authority for the thread Top-K.
			tiedIdentity := PerfThreadIdentity{TID: 1, TGID: 1, Generation: 1, DisplayComm: "same"}
			tiedKey := perfThreadKey{scope: fmt.Sprintf("scope-%03d", value), TID: 1, Generation: 1}
			threads[tiedKey] = &perfThreadAcc{
				item:     PerfThreadSummary{Period: 10, SampleCount: 1, Example: tiedKey.scope},
				identity: tiedIdentity,
				cpuSet:   cpuSet,
				total:    &total,
			}
			identities.add(threadKey, identity)
		}

		fullHotspots := make([]perfHotspotCandidate, 0, len(hotspots))
		for key, acc := range hotspots {
			fullHotspots = append(fullHotspots, perfHotspotCandidate{key: key, acc: acc})
		}
		sort.Slice(fullHotspots, func(i, j int) bool { return perfHotspotCandidateLess(fullHotspots[i], fullHotspots[j]) })
		wantHotspots := make([]PerfHotspot, 0, limit)
		for _, candidate := range fullHotspots[:limit] {
			wantHotspots = append(wantHotspots, materializePerfHotspot(candidate.acc))
		}
		if got := sortedPerfHotspots(hotspots, limit); !reflect.DeepEqual(got, wantHotspots) {
			t.Fatalf("run %d bounded hotspot Top-K diverged from full sort:\ngot=%+v\nwant=%+v", run, got, wantHotspots)
		}

		fullThreads := make([]perfThreadCandidate, 0, len(threads))
		for key, acc := range threads {
			fullThreads = append(fullThreads, perfThreadCandidate{key: key, acc: acc})
		}
		sort.Slice(fullThreads, func(i, j int) bool { return perfThreadCandidateLess(fullThreads[i], fullThreads[j]) })
		wantThreadExamples := make([]string, 0, limit)
		for _, candidate := range fullThreads[:limit] {
			wantThreadExamples = append(wantThreadExamples, candidate.acc.item.Example)
		}
		gotThreads := sortedPerfThreads(threads, limit)
		gotThreadExamples := make([]string, 0, len(gotThreads))
		for _, thread := range gotThreads {
			gotThreadExamples = append(gotThreadExamples, thread.Example)
		}
		if !reflect.DeepEqual(gotThreadExamples, wantThreadExamples) {
			t.Fatalf("run %d bounded thread Top-K diverged from full sort: got=%v want=%v", run, gotThreadExamples, wantThreadExamples)
		}

		fullIdentities := make([]perfThreadIdentityCandidate, 0, identities.count())
		for key, identity := range identities.promoted {
			fullIdentities = append(fullIdentities, perfThreadIdentityCandidate{key: key, identity: identity})
		}
		sort.Slice(fullIdentities, func(i, j int) bool { return perfThreadIdentityCandidateLess(fullIdentities[i], fullIdentities[j]) })
		gotIdentities := sortedPerfThreadIdentities(&identities)
		for i := range gotIdentities {
			if gotIdentities[i].TID != fullIdentities[i].identity.TID || gotIdentities[i].Generation != fullIdentities[i].identity.Generation {
				t.Fatalf("run %d bounded identity roster diverged at %d: got=%+v want=%+v", run, i, gotIdentities[i], fullIdentities[i].identity)
			}
		}
	}
}

func TestPerfBoundedTopKHighCardinalityAllocationAndRetention(t *testing.T) {
	const (
		cardinality       = 32_000
		limit             = 8
		maxProjectionHeap = uint64(4 << 20)
	)
	total := int64(cardinality)
	hotspots := make(map[string]*perfHotspotAcc, cardinality)
	threads := make(map[perfThreadKey]*perfThreadAcc, cardinality)
	var identities perfThreadIdentitySet
	for value := 0; value < cardinality; value++ {
		label := fmt.Sprintf("%05d", value)
		identity := PerfThreadIdentity{TID: value + 1, TGID: 7, Generation: 1, DisplayComm: "worker-" + label, CommAliases: []string{"alias-" + label}}
		key := perfThreadKey{scope: "resource", TID: identity.TID, Generation: 1}
		var threadSet perfThreadIdentitySet
		threadSet.add(key, identity)
		var cpuSet perfCPUSet
		cpuSet.add(value % 8)
		hotspots[label] = &perfHotspotAcc{
			item:      PerfHotspot{Symbol: "symbol-" + label, Callchain: "callchain-" + label, Period: 1, SampleCount: 1},
			threadSet: threadSet,
			cpuSet:    cpuSet,
			total:     &total,
		}
		threads[key] = &perfThreadAcc{
			item:     PerfThreadSummary{Period: 1, SampleCount: 1, Example: label},
			identity: identity,
			cpuSet:   cpuSet,
			total:    &total,
		}
		identities.add(key, identity)
	}

	// Warm compiler/runtime one-time paths before measuring the projection.
	_ = sortedPerfHotspots(hotspots, limit)
	_ = sortedPerfThreads(threads, limit)
	_ = sortedPerfThreadIdentities(&identities)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	hotspotTop := sortedPerfHotspots(hotspots, limit)
	threadTop := sortedPerfThreads(threads, limit)
	identityTop := sortedPerfThreadIdentities(&identities)
	runtime.GC()
	runtime.ReadMemStats(&after)

	totalAlloc := after.TotalAlloc - before.TotalAlloc
	retained := uint64(0)
	if after.HeapAlloc > before.HeapAlloc {
		retained = after.HeapAlloc - before.HeapAlloc
	}
	t.Logf("bounded perf projection resource evidence: cardinality=%d total_alloc=%d retained=%d hotspot_len/cap=%d/%d thread_len/cap=%d/%d identity_len/cap=%d/%d",
		cardinality, totalAlloc, retained, len(hotspotTop), cap(hotspotTop), len(threadTop), cap(threadTop), len(identityTop), cap(identityTop))
	if totalAlloc > maxProjectionHeap {
		t.Fatalf("bounded Top-K allocated %d bytes, want <= %d; publication cap must also bound projection allocation", totalAlloc, maxProjectionHeap)
	}
	if retained > maxProjectionHeap {
		t.Fatalf("bounded Top-K retained %d bytes, want <= %d; truncated full-cardinality backing arrays remain reachable", retained, maxProjectionHeap)
	}
	if len(hotspotTop) != limit || cap(hotspotTop) != limit || len(threadTop) != limit || cap(threadTop) != limit || len(identityTop) != limit || cap(identityTop) != limit {
		t.Fatalf("bounded Top-K result capacities drifted: hotspot=%d/%d thread=%d/%d identity=%d/%d", len(hotspotTop), cap(hotspotTop), len(threadTop), cap(threadTop), len(identityTop), cap(identityTop))
	}
	if hotspotTop[0].Symbol != "symbol-00000" || hotspotTop[0].Callchain != "callchain-00000" || threadTop[0].Identity == nil || threadTop[0].Identity.TID != 1 || identityTop[0].TID != 1 {
		t.Fatalf("bounded projection changed deterministic winners: hotspot=%+v thread=%+v identity=%+v", hotspotTop[0], threadTop[0], identityTop[0])
	}
	// Inputs must stay live across the second GC so retained measures only the
	// outputs, rather than accidentally collecting the high-cardinality source.
	runtime.KeepAlive(hotspots)
	runtime.KeepAlive(threads)
	runtime.KeepAlive(identities)
	runtime.KeepAlive(hotspotTop)
	runtime.KeepAlive(threadTop)
	runtime.KeepAlive(identityTop)
}

func TestPerfSingletonAccumulatorsHighCardinalityAllocationBudget(t *testing.T) {
	const (
		cardinality          = 32_000
		maxTotalAllocPerItem = uint64(1_536)
	)
	labels := make([]string, cardinality)
	callchains := make([]string, cardinality)
	threadKeys := make([]perfThreadKey, cardinality)
	identities := make([]PerfThreadIdentity, cardinality)
	for value := 0; value < cardinality; value++ {
		labels[value] = fmt.Sprintf("symbol-%05d", value)
		callchains[value] = fmt.Sprintf("callchain-%05d", value)
		identities[value] = PerfThreadIdentity{
			TID: value + 1, TGID: 7, Generation: 1,
			DisplayComm: "worker-" + labels[value], CommAliases: []string{"alias-" + labels[value]},
		}
		threadKeys[value] = perfThreadKey{scope: "resource", TID: value + 1, Generation: 1}
	}

	previousGCPercent := debug.SetGCPercent(-1)
	defer debug.SetGCPercent(previousGCPercent)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	hotspots := make(map[string]*perfHotspotAcc, cardinality)
	threads := make(map[perfThreadKey]*perfThreadAcc, cardinality)
	var total int64
	for value := 0; value < cardinality; value++ {
		addPerfHotspot(hotspots, labels[value], PerfHotspot{Symbol: labels[value], Callchain: callchains[value]}, threadKeys[value], identities[value], true, 0, value+1, 1, labels[value], &total)
		addPerfThread(threads, threadKeys[value], identities[value], 0, value+1, 1, labels[value], &total)
	}
	runtime.ReadMemStats(&after)
	totalAlloc := after.TotalAlloc - before.TotalAlloc
	perItem := totalAlloc / cardinality
	t.Logf("singleton perf accumulator resource evidence: cardinality=%d total_alloc=%d total_alloc_per_item=%d", cardinality, totalAlloc, perItem)
	if len(hotspots) != cardinality || len(threads) != cardinality {
		t.Fatalf("high-cardinality accumulators lost rows: hotspots=%d threads=%d want=%d", len(hotspots), len(threads), cardinality)
	}
	if perItem > maxTotalAllocPerItem {
		t.Fatalf("singleton hotspot/thread accumulation allocated %d bytes/item, want <= %d; first identity/CPU must stay inline until a second distinct value promotes a map", perItem, maxTotalAllocPerItem)
	}
	for _, value := range []int{0, cardinality / 2, cardinality - 1} {
		hotspot := hotspots[labels[value]]
		thread := threads[threadKeys[value]]
		if hotspot == nil || hotspot.threadSet.count() != 1 || hotspot.threadSet.promoted != nil || hotspot.cpuSet.promoted != nil || thread == nil || thread.cpuSet.promoted != nil || thread.identity.TID != value+1 {
			t.Fatalf("singleton accumulator promoted early at %d: hotspot=%+v thread=%+v", value, hotspot, thread)
		}
	}
	runtime.KeepAlive(labels)
	runtime.KeepAlive(callchains)
	runtime.KeepAlive(threadKeys)
	runtime.KeepAlive(identities)
	runtime.KeepAlive(hotspots)
	runtime.KeepAlive(threads)
}

func TestPerfUnknownCoverageWitnessAllocationIsAggregateBounded(t *testing.T) {
	measure := func(unknownSamples int, mixed bool) (float64, int, *bool) {
		var count int
		var witness *bool
		allocs := testing.AllocsPerRun(100, func() {
			count = 0
			witness = perfThreadIdentityCountExactPtr(true)
			if mixed {
				// One typed sample changes neither counter nor witness.
			}
			for i := 0; i < unknownSamples; i++ {
				notePerfThreadIdentityUnknown(&count, &witness)
			}
		})
		return allocs, count, witness
	}
	for _, mixed := range []bool{false, true} {
		one, oneCount, oneWitness := measure(1, mixed)
		many, manyCount, manyWitness := measure(8_192, mixed)
		if oneCount != 1 || manyCount != 8_192 || oneWitness == nil || *oneWitness || manyWitness == nil || *manyWitness {
			t.Fatalf("mixed=%t unknown coverage semantics drifted: one=(%d,%v) many=(%d,%v)", mixed, oneCount, oneWitness, manyCount, manyWitness)
		}
		if many > one+1 {
			t.Fatalf("mixed=%t unknown witness allocated per sample: one=%.2f allocations many=%.2f allocations", mixed, one, many)
		}
		t.Logf("unknown coverage allocation evidence: mixed=%t one=%.2f many8192=%.2f", mixed, one, many)
	}
}

func TestPerfTimelinePreCanceledContextDoesNotBuildSyntheticLedger(t *testing.T) {
	idx := &Index{Events: []Event{perfIdentityTestSample(1, 1, 7, 70, "worker")}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res := BuildPerfTimeline(idx, (Query{View: "perf_timeline"}).WithRunContext(ctx))
	if idx.perfIdentity != nil {
		t.Fatalf("pre-canceled timeline built a synthetic lazy ledger: %+v", idx.perfIdentity)
	}
	if len(res.Buckets) != 0 {
		t.Fatalf("pre-canceled timeline published buckets: %+v", res.Buckets)
	}
}

func TestPerfWindowStatsPreCanceledContextDoesNotBuildSyntheticLedger(t *testing.T) {
	idx := &Index{Events: []Event{perfIdentityTestSample(1, 1, 7, 70, "worker")}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stats := ComputeWindowStats(idx, (Query{View: "window_stats"}).WithRunContext(ctx))
	if idx.perfIdentity != nil {
		t.Fatalf("pre-canceled window_stats built a synthetic lazy ledger: %+v", idx.perfIdentity)
	}
	if stats.PerfSamples != nil {
		t.Fatalf("pre-canceled window_stats published perf samples: %+v", stats.PerfSamples)
	}
}

func perfIdentityResourceDistinctSamples(count int) []Event {
	events := make([]Event, count)
	for i := range events {
		tid := i + 1
		events[i] = perfIdentityTestSample(tid, float64(tid), tid, tid, "worker")
	}
	return events
}

func perfIdentityResourceLenRecords(ledger *perfIdentityLedger) int {
	if ledger == nil {
		return 0
	}
	return len(ledger.records)
}

func perfIdentityResourceLenBindings(ledger *perfIdentityLedger) int {
	if ledger == nil {
		return 0
	}
	return len(ledger.bindings)
}
