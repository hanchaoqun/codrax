package tracequery

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracebundle"
)

func TestPerfIdentityLedgerRenameIsOneNumericCohort(t *testing.T) {
	idx := &Index{Events: []Event{
		perfIdentityTestSample(1, 1.0, 7, 70, "alpha"),
		perfIdentityTestSample(2, 2.0, 7, 70, "beta"),
	}}
	ledger := ensurePerfIdentityLedger(idx)
	key0, identity0, ok0 := ledger.identityForEventOrdinal(0)
	key1, identity1, ok1 := ledger.identityForEventOrdinal(1)
	if !ok0 || !ok1 || key0 != key1 {
		t.Fatalf("comm rename split numeric identity: ok=(%t,%t) keys=(%+v,%+v)", ok0, ok1, key0, key1)
	}
	want := PerfThreadIdentity{
		TID: 7, TGID: 70, Generation: 1, DisplayComm: "beta",
		CommAliases: []string{"alpha", "beta"}, CommAliasCount: 2,
	}
	if !reflect.DeepEqual(identity0, want) || !reflect.DeepEqual(identity1, want) {
		t.Fatalf("rename display/aliases drifted: first=%+v second=%+v want=%+v", identity0, identity1, want)
	}
	// Returned identities are copies: a renderer cannot mutate the memoized
	// ledger or change a later consumer's display aliases.
	identity0.CommAliases[0] = "mutated"
	_, again, ok := ledger.identityForEventOrdinal(0)
	if !ok || !reflect.DeepEqual(again.CommAliases, want.CommAliases) {
		t.Fatalf("identity alias projection was mutable: %+v", again)
	}
}

func TestPerfIdentityLedgerSameCommDifferentTIDNeverMerges(t *testing.T) {
	idx := &Index{Events: []Event{
		perfIdentityTestSample(1, 1.0, 7, 70, "worker"),
		perfIdentityTestSample(2, 2.0, 8, 70, "worker"),
	}}
	ledger := ensurePerfIdentityLedger(idx)
	key0, identity0, ok0 := ledger.identityForEventOrdinal(0)
	key1, identity1, ok1 := ledger.identityForEventOrdinal(1)
	if !ok0 || !ok1 || key0 == key1 || identity0.TID != 7 || identity1.TID != 8 {
		t.Fatalf("same display comm merged distinct tids: first=(%+v,%+v,%t) second=(%+v,%+v,%t)", key0, identity0, ok0, key1, identity1, ok1)
	}
}

func TestPerfIdentityLedgerExactLifecycleSignalsAdvanceGeneration(t *testing.T) {
	t.Run("sched_wakeup_new", func(t *testing.T) {
		idx := &Index{Events: []Event{
			perfIdentityTestSample(1, 1.0, 42, 4, "old"),
			{Line: 2, Ts: 2.0, Type: EventSchedWakeup, Name: "sched_wakeup_new", PID: 9, WakeePID: 42, WakeeComm: "new"},
			perfIdentityTestSample(3, 3.0, 42, 4, "new"),
		}}
		assertPerfIdentityTestGenerations(t, idx, 0, 2)
	})
	t.Run("dead_then_reappears", func(t *testing.T) {
		idx := &Index{Events: []Event{
			perfIdentityTestSample(1, 1.0, 52, 5, "old"),
			{Line: 2, Ts: 2.0, Type: EventSchedSwitch, PrevPID: 52, PrevComm: "old", PrevState: "X", NextPID: 0, NextComm: "idle"},
			perfIdentityTestSample(3, 3.0, 52, 5, "new"),
		}}
		assertPerfIdentityTestGenerations(t, idx, 0, 2)
	})
}

func TestPerfIdentityLedgerTGIDConflictRevokesOnlyCohort(t *testing.T) {
	idx := &Index{Events: []Event{
		perfIdentityTestSample(1, 1.0, 7, 70, "worker"),
		perfIdentityTestSample(2, 2.0, 7, 71, "worker"),
		perfIdentityTestSample(3, 3.0, 8, 80, "healthy"),
	}}
	ledger := ensurePerfIdentityLedger(idx)
	if _, _, ok := ledger.identityForEventOrdinal(0); ok {
		t.Fatal("first conflicting TGID sample retained a thread identity")
	}
	if _, _, ok := ledger.identityForEventOrdinal(1); ok {
		t.Fatal("second conflicting TGID sample retained a thread identity")
	}
	if _, identity, ok := ledger.identityForEventOrdinal(2); !ok || identity.TID != 8 || identity.TGID != 80 {
		t.Fatalf("TGID conflict poisoned healthy sibling: ok=%t identity=%+v", ok, identity)
	}
	if got := strings.Join(ledger.caveats(), "\n"); !strings.Contains(got, "perf_thread_tgid_conflict=true") {
		t.Fatalf("TGID withdrawal lacked typed caveat: %q", got)
	}
}

func TestPerfIdentityLedgerSourceOnlyAndZeroStayAnonymous(t *testing.T) {
	unknown, unverified := false, true
	idx := &Index{Events: []Event{
		{
			Line: 1, Ts: 1, Type: EventPerfSample,
			PerfFields: &PerfFields{Source: "trace_streamer_db", Resolution: perfSourceOnlyResolution,
				SourceTID: 7, SourcePID: 70, SourceComm: "audit-only", ThreadIdentityKnown: &unknown, LifecycleUnverified: &unverified},
		},
		{Line: 2, Ts: 2, Type: EventPerfSample, PerfFields: &PerfFields{Source: "simpleperf_report_sample", TID: 0, PID: 0, Comm: "idle"}},
		perfIdentityTestSample(3, 3, 8, 80, "healthy"),
	}}
	ledger := ensurePerfIdentityLedger(idx)
	for _, ordinal := range []int{0, 1} {
		if _, _, ok := ledger.identityForEventOrdinal(ordinal); ok {
			t.Fatalf("anonymous sample ordinal %d gained a thread identity", ordinal)
		}
	}
	if _, identity, ok := ledger.identityForEventOrdinal(2); !ok || identity.TID != 8 {
		t.Fatalf("anonymous inventory poisoned healthy sample: ok=%t identity=%+v", ok, identity)
	}
}

func TestPerfIdentityLedgerUnboundMultiSourceFailsOnlySharedNumericTID(t *testing.T) {
	sources := []TraceArtifactSource{
		{SourcePath: "/capture/a.systrace", CausalCompatible: true, VirtualLineBase: 0, LocalLineCount: 10},
		{SourcePath: "/capture/b.perftrace", CausalCompatible: true, VirtualLineBase: 100, LocalLineCount: 10},
	}
	idx := &Index{TraceArtifacts: sources, Events: []Event{
		perfIdentityTestSample(1, 1, 7, 70, "a"),
		perfIdentityTestSample(101, 1, 7, 70, "b"),
		perfIdentityTestSample(102, 2, 8, 80, "healthy"),
	}}
	ledger := ensurePerfIdentityLedger(idx)
	for _, ordinal := range []int{0, 1} {
		if _, _, ok := ledger.identityForEventOrdinal(ordinal); ok {
			t.Fatalf("unbound cross-source tid retained identity at ordinal %d", ordinal)
		}
	}
	if _, identity, ok := ledger.identityForEventOrdinal(2); !ok || identity.TID != 8 {
		t.Fatalf("unbound source conflict poisoned source-local healthy tid: ok=%t identity=%+v", ok, identity)
	}
	if got := strings.Join(ledger.caveats(), "\n"); !strings.Contains(got, "perf_thread_cross_source_unbound=true") {
		t.Fatalf("unbound withdrawal lacked caveat: %q", got)
	}

	for i := range sources {
		sources[i].BundleSchema = tracebundle.SchemaV2
		sources[i].CaptureID = "capture-verified"
	}
	bound := &Index{TraceArtifacts: sources, Events: append([]Event(nil), idx.Events...)}
	bound.Events[1].Ts = 1.1
	boundLedger := ensurePerfIdentityLedger(bound)
	key0, _, ok0 := boundLedger.identityForEventOrdinal(0)
	key1, _, ok1 := boundLedger.identityForEventOrdinal(1)
	if !ok0 || !ok1 || key0 != key1 {
		t.Fatalf("verified V2 capture did not share numeric cohort: first=(%+v,%t) second=(%+v,%t) caveats=%v", key0, ok0, key1, ok1, boundLedger.caveats())
	}
}

func TestPerfIdentityLedgerAliasProjectionIsBoundedSortedAndCounted(t *testing.T) {
	events := make([]Event, 0, perfIdentityAliasProjectionCap+5)
	for i := 0; i < cap(events); i++ {
		events = append(events, perfIdentityTestSample(i+1, float64(i+1), 7, 70, "alias-"+perfIdentityTwoDigits(i)))
	}
	ledger := ensurePerfIdentityLedger(&Index{Events: events})
	_, identity, ok := ledger.identityForEventOrdinal(len(events) - 1)
	if !ok {
		t.Fatalf("alias-heavy cohort lost identity: caveats=%v", ledger.caveats())
	}
	if identity.CommAliasCount != len(events) || len(identity.CommAliases) != perfIdentityAliasProjectionCap {
		t.Fatalf("alias cap/count mismatch: %+v", identity)
	}
	if !sort.StringsAreSorted(identity.CommAliases) {
		t.Fatalf("alias projection is not deterministic/sorted: %v", identity.CommAliases)
	}
	wantDisplay := "alias-" + perfIdentityTwoDigits(len(events)-1)
	if identity.DisplayComm != wantDisplay || !perfIdentityStringSliceContains(identity.CommAliases, wantDisplay) {
		t.Fatalf("latest display alias was not retained: %+v want=%q", identity, wantDisplay)
	}
}

func TestPerfIdentityLedgerPrivateAliasesDriveSelectorsBeyondPublicProjection(t *testing.T) {
	events := []Event{perfIdentityTestSample(1, 1, 7, 70, "zz-old-name")}
	for i := 0; i < perfIdentityAliasProjectionCap+3; i++ {
		events = append(events, perfIdentityTestSample(i+2, float64(i+2), 7, 70, "aa-name-"+perfIdentityTwoDigits(i)))
	}
	ledger := ensurePerfIdentityLedger(&Index{Events: events})
	key, identity, ok := ledger.identityForEventOrdinal(0)
	if !ok {
		t.Fatalf("alias-heavy cohort lost identity: caveats=%v", ledger.caveats())
	}
	if perfIdentityStringSliceContains(identity.CommAliases, "zz-old-name") {
		t.Fatalf("fixture failed to place old alias beyond public projection: %+v", identity)
	}
	if !ledger.matchesComm(key, "ZZ-OLD-NAME") {
		t.Fatal("case-insensitive exact matcher lost an alias beyond the public projection")
	}
	if !ledger.matchesThreadSelector(key, parseThreadSelector("old-name")) {
		t.Fatal("thread-selector semantics lost an alias beyond the public projection")
	}
	if !ledger.matchesThreadSelector(key, parseThreadSelector("zz-old-name-7")) {
		t.Fatal("PID+name selector did not match the same typed cohort")
	}
	if ledger.matchesThreadSelector(key, parseThreadSelector("zz-old-name-8")) {
		t.Fatal("PID+name selector crossed into a different numeric TID")
	}
	aliases := ledger.aliasesForKey(key)
	aliases[0] = "mutated"
	if again := ledger.aliasesForKey(key); len(again) == 0 || again[0] == "mutated" {
		t.Fatalf("private full aliases were mutable: %v", again)
	}
}

func TestPerfIdentityLedgerFullAliasOverflowRetainsNumericIdentityAndWithholdsNameSelector(t *testing.T) {
	events := make([]Event, 0, perfIdentityFullAliasCap+2)
	for i := 0; i < cap(events); i++ {
		events = append(events, perfIdentityTestSample(i+1, float64(i+1), 7, 70, "alias-overflow-"+perfIdentityThreeDigits(i)))
	}
	ledger := ensurePerfIdentityLedger(&Index{Events: events})
	key, identity, ok := ledger.identityForEventOrdinal(0)
	if !ok || identity.TID != 7 || !identity.CommAliasesTruncated || identity.CommAliasCount != 0 || identity.CommAliasCountAtLeast != perfIdentityFullAliasCap+1 {
		t.Fatalf("alias overflow lost honest numeric identity/lower bound: ok=%t identity=%+v", ok, identity)
	}
	if !ledger.matchesThreadSelector(key, parseThreadSelector("mismatched-display-7")) {
		t.Fatal("precise TID selector was vetoed by display-name drift")
	}
	if got := ledger.selectorVerdictForEventOrdinal(0, parseThreadSelector("alias-overflow-000")); got != perfIdentitySelectorWithheld {
		t.Fatalf("name-only selector should be withheld after alias cap, got %v", got)
	}
	if got := strings.Join(ledger.caveats(), "\n"); !strings.Contains(got, "perf_thread_alias_authority_capped=true") {
		t.Fatalf("full-alias overflow lacked typed caveat: %q", got)
	}
}

func TestPerfIdentityLedgerLifecycleAuditCapAndCaveatCopiesFailClosed(t *testing.T) {
	idx := &Index{
		Events:                          []Event{perfIdentityTestSample(1, 1, 7, 70, "worker")},
		threadIncarnationFailuresCapped: true,
	}
	ledger := ensurePerfIdentityLedger(idx)
	if _, _, ok := ledger.identityForEventOrdinal(0); ok {
		t.Fatal("capped lifecycle audit retained numeric thread attribution")
	}
	first := ledger.caveats()
	if len(first) == 0 || !strings.Contains(first[0], "perf_thread_generation_audit_capped=true") {
		t.Fatalf("capped audit lacked caveat: %v", first)
	}
	first[0] = "mutated"
	if again := ledger.caveats(); len(again) == 0 || again[0] == "mutated" {
		t.Fatalf("caveat slice was not copied: %v", again)
	}
}

func TestDerivedWindowRebuildsPerfIdentityOrdinalLedger(t *testing.T) {
	full := &Index{Events: []Event{
		perfIdentityTestSample(1, 1, 7, 70, "old"),
		perfIdentityTestSample(2, 2, 8, 80, "selected"),
	}, TimestampOrder: TraceTimestampOrderMonotonic}
	if _, identity, ok := ensurePerfIdentityLedger(full).identityForEventOrdinal(0); !ok || identity.TID != 7 {
		t.Fatalf("full fixture identity failed: ok=%t identity=%+v", ok, identity)
	}
	derived := deriveWindowedIndex(full, BuildOptions{TimeStart: 1.5, TimeEnd: 2.5, TimeStartSet: true, TimeEndSet: true})
	if derived == nil || len(derived.Events) != 1 {
		t.Fatalf("derived fixture drifted: %+v", derived)
	}
	ledger := ensurePerfIdentityLedger(derived)
	if len(ledger.records) != 1 {
		t.Fatalf("derived index copied stale parent ordinal ledger: records=%d events=%d", len(ledger.records), len(derived.Events))
	}
	if _, identity, ok := ledger.identityForEventOrdinal(0); !ok || identity.TID != 8 {
		t.Fatalf("derived ordinal 0 retained parent identity: ok=%t identity=%+v", ok, identity)
	}
}

func TestDerivedWindowUsesPreservedPrefixBoundaryForCaptureGeneration(t *testing.T) {
	full := &Index{
		Events: []Event{
			perfIdentityTestSample(1, 1.0, 77, 7, "old"),
			{Line: 2, Ts: 2.0, Type: EventSchedWakeup, Name: "sched_wakeup_new", PID: 9, WakeePID: 77, WakeeComm: "new"},
			perfIdentityTestSample(3, 3.0, 77, 8, "new"),
		},
		TimestampOrder: TraceTimestampOrderMonotonic,
		threadIncarnationFailures: []threadIncarnationConflict{{
			PID: 77, PreviousLine: 1, PreviousTs: 1.0, BoundaryLine: 2, BoundaryTs: 2.0, Signal: "sched_wakeup_new",
		}},
	}
	derived := deriveWindowedIndex(full, BuildOptions{LineStart: 3, LineEnd: 3})
	if derived == nil || len(derived.Events) != 1 || derived.Events[0].Line != 3 {
		t.Fatalf("window fixture did not retain only the new-generation sample: %+v", derived)
	}
	_, identity, ok := ensurePerfIdentityLedger(derived).identityForEventOrdinal(0)
	if !ok || identity.TID != 77 || identity.Generation != 2 || identity.DisplayComm != "new" {
		t.Fatalf("preserved prefix boundary was lost at window head: ok=%t identity=%+v caveats=%v", ok, identity, ensurePerfIdentityLedger(derived).caveats())
	}
}

func TestPerfIdentityLedgerSameTimestampUsesPhysicalLineOrder(t *testing.T) {
	idx := &Index{Events: []Event{
		perfIdentityTestSample(10, 5.0, 77, 7, "old"),
		{Line: 11, Ts: 5.0, Type: EventSchedWakeup, Name: "sched_wakeup_new", PID: 9, WakeePID: 77, WakeeComm: "new"},
		perfIdentityTestSample(12, 5.0, 77, 8, "new"),
	}}
	ledger := ensurePerfIdentityLedger(idx)
	_, before, beforeOK := ledger.identityForEventOrdinal(0)
	_, after, afterOK := ledger.identityForEventOrdinal(2)
	if !beforeOK || !afterOK || before.Generation != 1 || after.Generation != 2 {
		t.Fatalf("same-ts physical line order did not govern boundary: before=(%+v,%t) after=(%+v,%t) caveats=%v", before, beforeOK, after, afterOK, ledger.caveats())
	}
}

func TestPerfIdentityLedgerV2CrossChildCanonicalOrderAndLocalAmbiguity(t *testing.T) {
	sources := []TraceArtifactSource{
		{SourcePath: "/capture/sched.systrace", BundleSchema: tracebundle.SchemaV2, CaptureID: "capture-v2", CausalCompatible: true, VirtualLineBase: 0, LocalLineCount: 20},
		{SourcePath: "/capture/samples.perftrace", BundleSchema: tracebundle.SchemaV2, CaptureID: "capture-v2", CausalCompatible: true, VirtualLineBase: 100, LocalLineCount: 20},
	}
	t.Run("same timestamp across children is simultaneous and withheld", func(t *testing.T) {
		idx := &Index{TraceArtifacts: sources, Events: []Event{
			perfIdentityTestSample(101, 4.0, 77, 7, "old"),
			// At the shared timestamp the lower canonical virtual line is the
			// lifecycle boundary, so the perf child sample at line 102 is new.
			{Line: 2, Ts: 5.0, Type: EventSchedWakeup, Name: "sched_wakeup_new", PID: 9, WakeePID: 77, WakeeComm: "new"},
			perfIdentityTestSample(102, 5.0, 77, 8, "new"),
		}}
		ledger := ensurePerfIdentityLedger(idx)
		_, oldIdentity, oldOK := ledger.identityForEventOrdinal(0)
		_, newIdentity, newOK := ledger.identityForEventOrdinal(2)
		if oldOK || newOK {
			t.Fatalf("cross-child simultaneous boundary fabricated order: old=(%+v,%t) new=(%+v,%t) caveats=%v", oldIdentity, oldOK, newIdentity, newOK, ledger.caveats())
		}
		if got := strings.Join(ledger.caveats(), "\n"); !strings.Contains(got, "cross_artifact_simultaneous") {
			t.Fatalf("simultaneous withdrawal lacked typed caveat: %q", got)
		}
	})
	t.Run("duplicate canonical coordinate withdraws only involved tid", func(t *testing.T) {
		idx := &Index{TraceArtifacts: sources, Events: []Event{
			perfIdentityTestSample(101, 5.0, 77, 7, "ambiguous-a"),
			perfIdentityTestSample(101, 5.0, 77, 7, "ambiguous-b"),
			perfIdentityTestSample(102, 5.1, 88, 8, "healthy"),
		}}
		ledger := ensurePerfIdentityLedger(idx)
		for _, ordinal := range []int{0, 1} {
			if _, _, ok := ledger.identityForEventOrdinal(ordinal); ok {
				t.Fatalf("ambiguous coordinate retained tid=77 at ordinal %d", ordinal)
			}
		}
		if _, identity, ok := ledger.identityForEventOrdinal(2); !ok || identity.TID != 88 {
			t.Fatalf("coordinate ambiguity poisoned healthy sibling: ok=%t identity=%+v caveats=%v", ok, identity, ledger.caveats())
		}
		if got := strings.Join(ledger.caveats(), "\n"); !strings.Contains(got, "perf_thread_event_order_unproven=true") {
			t.Fatalf("ambiguous coordinate lacked typed caveat: %q", got)
		}
	})
}

func perfIdentityTestSample(line int, ts float64, tid, tgid int, comm string) Event {
	return Event{
		Line: line, Ts: ts, Type: EventPerfSample, PID: tid, TGID: tgid, Comm: comm,
		PerfFields: &PerfFields{TID: tid, PID: tgid, Comm: comm, Period: 1, EventName: "cpu-cycles", Symbol: "hot"},
	}
}

func assertPerfIdentityTestGenerations(t *testing.T, idx *Index, firstOrdinal, secondOrdinal int) {
	t.Helper()
	ledger := ensurePerfIdentityLedger(idx)
	key0, identity0, ok0 := ledger.identityForEventOrdinal(firstOrdinal)
	key1, identity1, ok1 := ledger.identityForEventOrdinal(secondOrdinal)
	if !ok0 || !ok1 || key0 == key1 || identity0.TID != identity1.TID || identity0.Generation != 1 || identity1.Generation != 2 {
		t.Fatalf("exact lifecycle boundary did not split generations: first=(%+v,%+v,%t) second=(%+v,%+v,%t) caveats=%v", key0, identity0, ok0, key1, identity1, ok1, ledger.caveats())
	}
}

func perfIdentityTwoDigits(value int) string {
	return string([]byte{'0' + byte(value/10), '0' + byte(value%10)})
}

func perfIdentityThreeDigits(value int) string {
	return string([]byte{'0' + byte(value/100), '0' + byte(value/10%10), '0' + byte(value%10)})
}
