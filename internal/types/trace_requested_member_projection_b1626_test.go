package types

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func b1626ProjectionProfile() *RuntimeArtifactScopeProfile {
	a, b, c, d := 10.0, 10.01, 10.02, 10.05
	return &RuntimeArtifactScopeProfile{RequestedScope: RuntimeArtifactScopeExplicitWindow, TimeWindows: []RuntimeArtifactTimeWindow{
		{TimeStart: &a, TimeEnd: &b, SourceQuote: "first requested window"},
		{TimeStart: &c, TimeEnd: &d, SourceQuote: "second requested window"},
	}}
}

func b1626ProjectionRows(id string, start, end, total float64, causal bool) []ObservationRecord {
	window := fmt.Sprintf("selected_window=%.6f..%.6f", start, end)
	rows := []ObservationRecord{requestedWindowAuthorityRecord(id+"#state", "target_window_states", "ui-100", "state_partition", fmt.Sprint(total), window,
		"running=0.000", "runnable=0.000", "sleep="+fmt.Sprint(total), "total="+fmt.Sprint(total))}
	if causal {
		rows = append(rows,
			requestedWindowAuthorityRecord(id+"#path", "wakeup_chain", "ui-100", "worker-200 -> ui-100", "", "branch=1", window),
			requestedWindowAuthorityRecord(id+"#rank", "root_cause_primary", "worker-200", "runnable", "2", "rank=1", "tier=primary", "chain_relevance=on_chain", "effective_impact_ms=2", window))
	}
	for i := range rows {
		rows[i].SourceRef = ObservationSourceRef{Kind: ObservationSourceRuntimeArtifact, Path: "/capture/example.trace", PayloadRef: "/results/" + id + ".json", QueryScopeID: id,
			QueryWindowKnown: true, QueryWindowStartTs: start, QueryWindowEndTs: end, QueryTargetPID: 100, QueryTargetScope: "thread", QueryLineRangeKnown: true}
	}
	return rows
}

func TestB1626RequestedMemberProjectionAndFiniteAccounts(t *testing.T) {
	for _, causalB := range []bool{false, true} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("causalB=%t/reverse=%t", causalB, reverse), func(t *testing.T) {
				a, b := b1626ProjectionRows("a", 10, 10.01, 10, true), b1626ProjectionRows("b", 10.02, 10.05, 30, causalB)
				if reverse {
					a, b = b, a
				}
				ledger := ObservationLedger{Records: append(a, b...), RuntimeArtifactScopeProfile: b1626ProjectionProfile(), AnchorUserEntities: []AnchorUserEntity{{Value: "100", TypedLane: true}}}
				before, _ := json.Marshal(ledger)
				set := CompileTraceCausalProjectionSet(ledger)
				active := 0
				for _, p := range set.Projections {
					if p.Active() {
						active++
					}
				}
				wantActive := 1
				if causalB {
					wantActive = 2
				}
				if active != wantActive {
					t.Errorf("independent causal member projections=%d want %d: %+v", active, wantActive, set)
				}
				authorities := BuildTraceTargetStateScopeAuthoritiesFromLedger(ledger)
				if len(authorities) != 2 {
					t.Fatalf("A projection must not swallow B finite account: %+v", authorities)
				}
				for i, want := range []float64{10, 30} {
					if authorities[i].TotalMS != want || authorities[i].WindowScope.Role != TraceQueryWindowScopeRequestedPrincipal {
						t.Errorf("member %d account/ruler lost: %+v", i, authorities[i])
					}
				}
				after, _ := json.Marshal(ledger)
				if !reflect.DeepEqual(before, after) {
					t.Fatal("projection mutated source/profile")
				}
			})
		}
	}
}

func TestB1626RequestedMemberScopeDoesNotElectEnvelopeOrDuplicates(t *testing.T) {
	p := b1626ProjectionProfile()
	for _, window := range [][2]float64{{10, 10.01}, {10.02, 10.05}} {
		if scope := ResolveTraceQueryWindowScope(p, window[0], window[1]); scope.Role != TraceQueryWindowScopeRequestedPrincipal {
			t.Errorf("exact member lost: %+v", scope)
		}
	}
	if scope := ResolveTraceQueryWindowScope(p, 10, 10.05); scope.Role != TraceQueryWindowScopeSupportingExploration {
		t.Errorf("envelope elected: %+v", scope)
	}
	p.TimeWindows = append(p.TimeWindows, p.TimeWindows[0])
	if scope := ResolveTraceQueryWindowScope(p, 10, 10.01); scope.Role != TraceQueryWindowScopeSupportingExploration {
		t.Errorf("duplicate member elected: %+v", scope)
	}
}

func TestB1626RequestedMemberSourcesAndFiltersStayIndependent(t *testing.T) {
	for _, tc := range []struct {
		name      string
		change    func(*ObservationSourceRef)
		principal bool
	}{
		{"exact", func(*ObservationSourceRef) {}, true},
		{"line_filter", func(r *ObservationSourceRef) { r.QueryLineStart = 20; r.QueryLineEnd = 90 }, false},
		{"line_unknown", func(r *ObservationSourceRef) { r.QueryLineRangeKnown = false }, false},
		{"wrong_target", func(r *ObservationSourceRef) { r.QueryTargetPID = 999 }, false},
		{"process_scope", func(r *ObservationSourceRef) { r.QueryTargetScope = "process" }, false},
		{"missing_result", func(r *ObservationSourceRef) { r.PayloadRef = "" }, false},
		{"missing_query", func(r *ObservationSourceRef) { r.QueryScopeID = "" }, false},
		{"unknown_window", func(r *ObservationSourceRef) { r.QueryWindowKnown = true; r.QueryWindowEndTs = 0 }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows := b1626ProjectionRows("second", 10, 10.01, 9, false)
			for i := range rows {
				tc.change(&rows[i].SourceRef)
			}
			// Same record ID is not a receipt; a distinct result/filter must
			// retain its original amount rather than borrow the first account.
			rows[0].ID = "first#state"
			ledger := ObservationLedger{Records: append(b1626ProjectionRows("first", 10, 10.01, 10, true), rows...), RuntimeArtifactScopeProfile: b1626ProjectionProfile(), AnchorUserEntities: []AnchorUserEntity{{Value: "100", TypedLane: true}}}
			before, _ := json.Marshal(ledger)
			accounts := BuildTraceTargetStateScopeAuthoritiesFromLedger(ledger)
			if len(accounts) != 2 || accounts[0].TotalMS != 10 || accounts[1].TotalMS != 9 {
				t.Fatalf("independent source accounts lost: %+v", accounts)
			}
			if got := accounts[1].WindowScope.Role == TraceQueryWindowScopeRequestedPrincipal; got != tc.principal {
				t.Errorf("source qualification=%t want %t: %+v", got, tc.principal, accounts[1])
			}
			after, _ := json.Marshal(ledger)
			if string(before) != string(after) {
				t.Fatal("source mutation")
			}
		})
	}
}

func TestB1626RequestedMemberRecursiveWindowDoesNotMoveParent(t *testing.T) {
	profile := b1626ProjectionProfile()
	*profile.TimeWindows[0].TimeEnd = .05 + 10
	*profile.TimeWindows[1].TimeEnd = .03 + 10
	a := b1626ProjectionRows("outer", 10, 10.05, 50, true)
	b := b1626ProjectionRows("inner-query", 10.02, 10.03, 10, true)
	child := requestedWindowAuthorityRecord("outer#child", "root_cause_primary", "recursive-300", "runnable", "1", "rank=2", "tier=primary", "chain_relevance=on_chain", "effective_impact_ms=1", "selected_window=10.020000..10.030000")
	child.SourceRef = a[0].SourceRef
	ledger := ObservationLedger{Records: append(append(a, child), b...), RuntimeArtifactScopeProfile: profile, AnchorUserEntities: []AnchorUserEntity{{Value: "100", TypedLane: true}}}
	set := CompileTraceCausalProjectionSet(ledger)
	if len(set.Projections) != 2 {
		t.Fatalf("recursive local window split result: %+v", set)
	}
	for i, p := range set.Projections {
		if p.WindowScope.RequestedWindowOrdinal != i+1 || p.ArtifactPath != "/capture/example.trace" || p.ArtifactLabel != "example.trace" {
			t.Errorf("member/artifact identity drift: %+v", p)
		}
		encoded, _ := json.Marshal(p)
		if i == 0 && (!strings.Contains(string(encoded), "outer#child") || strings.Contains(string(encoded), "inner-query#")) {
			t.Errorf("native recursive record lost or cross-result imported: %s", encoded)
		}
		if i == 1 && strings.Contains(string(encoded), "outer#") {
			t.Errorf("A native fragment became B query source: %s", encoded)
		}
	}
	if set.Projections[0].WindowStartTs != 10 || set.Projections[0].WindowEndTs != 10.05 || set.Projections[0].TargetStateAccount.TotalMS != 50 || set.Projections[1].TargetStateAccount.TotalMS != 10 {
		t.Fatal("parent/child accounts mixed")
	}
	if got := CompileTraceCausalProjection(ledger); got.Active() {
		t.Fatal("singular API must not select a multi-member winner")
	}
}

func TestB1626RequestedMemberLegacyParentConflictAndScopeCopy(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		rows := b1626ProjectionRows("legacy", 10, 10.01, 10, true)
		for i := range rows {
			rows[i].SourceRef.QueryWindowKnown = false
			rows[i].SourceRef.QueryWindowStartTs = 0
			rows[i].SourceRef.QueryWindowEndTs = 0
			rows[i].SourceRef.QueryTargetPID = 0
		}
		if conflict {
			rows[1].RichNotes = []string{"branch=1", "selected_window=10.020000..10.050000"}
		}
		ledger := ObservationLedger{Records: rows, RuntimeArtifactScopeProfile: b1626ProjectionProfile(), AnchorUserEntities: []AnchorUserEntity{{Value: "100", TypedLane: true}}}
		accounts := BuildTraceTargetStateScopeAuthoritiesFromLedger(ledger)
		if len(accounts) != 1 || accounts[0].TotalMS != 10 || (accounts[0].WindowScope.Role == TraceQueryWindowScopeRequestedPrincipal) == conflict {
			t.Errorf("same-result fallback conflict=%t: %+v", conflict, accounts)
		}
	}
	p := b1626ProjectionProfile()
	scope := ResolveTraceQueryWindowScope(p, 10.02, 10.05)
	encoded, _ := json.Marshal(scope)
	var restored TraceQueryWindowScope
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	*p.TimeWindows[1].TimeEnd = 99
	if restored.ForWindow(10.02, 10.05) != scope || scope.RequestedWindowOrdinal != 2 || scope.RequestedWindowCount != 2 {
		t.Fatal("scope identity depends on mutable profile or unpersisted list")
	}
	if restored.ForWindow(10, 10.01).Role == TraceQueryWindowScopeRequestedPrincipal {
		t.Fatal("ForWindow must not jump between independently bound query members")
	}
}

func TestB1626RequestedMemberReaderSourceAndOwnedCopy(t *testing.T) {
	a, b := b1626ProjectionRows("a", 10, 10.01, 10, true), b1626ProjectionRows("b", 10, 10.01, 10, true)
	offset := 1.0
	for i := range a {
		a[i].SourceRef.ClockOffsetSec = &offset
	}
	for i := range b {
		b[i].SourceRef.QueryLineStart = 20
		b[i].SourceRef.QueryLineEnd = 40
	}
	ledger := ObservationLedger{Records: append(a, b...), RuntimeArtifactScopeProfile: b1626ProjectionProfile(), AnchorUserEntities: []AnchorUserEntity{{Value: "100", TypedLane: true}}}
	set := CompileTraceCausalProjectionSet(ledger)
	if len(set.Projections) != 2 {
		t.Fatalf("want two result sources: %+v", set)
	}
	for i, p := range set.Projections {
		for j, row := range []ObservationRecord{a[2], b[2]} {
			if got := TraceCausalProjectionMatchesRecordSource(p, row); got != (i == j) {
				t.Errorf("projection%d matched source%d=%t", i, j, got)
			}
		}
	}
	child := a[2]
	child.RichNotes = []string{"selected_window=10.020000..10.030000"}
	if !TraceCausalProjectionMatchesRecordSource(set.Projections[0], child) {
		t.Fatal("recursive local window lost parent result")
	}
	foreign := child
	foreign.SourceRef.PayloadRef = "/elsewhere/result.json"
	if TraceCausalProjectionMatchesRecordSource(set.Projections[0], foreign) {
		t.Fatal("same window/target admitted foreign result")
	}
	second := CompileTraceCausalProjectionSet(ledger)
	*set.Projections[0].QuerySourceRef.ClockOffsetSec = 7
	set.Projections[0].QuerySourceRef.QueryScopeID = "changed"
	if offset != 1 || *second.Projections[0].QuerySourceRef.ClockOffsetSec != 1 || second.Projections[0].QuerySourceRef.QueryScopeID != "a" {
		t.Fatal("query source aliases input or another compile")
	}
}

func TestB1626RequestedMemberWaitAndWakeupReaders(t *testing.T) {
	profile := b1626ProjectionProfile()
	rm := &RequestModel{RuntimeArtifactScopeProfile: profile, RuntimeTargets: []RuntimeTarget{{Kind: RuntimeTargetKindThread, PID: 100, Thread: "ui-100", Source: "user_explicit"}}}
	var rows []ObservationRecord
	for i, spec := range []struct {
		id         string
		start, end float64
	}{{"z-first", 10, 10.01}, {"a-second", 10.02, 10.05}, {"filtered", 10, 10.01}} {
		base := b1626ProjectionRows(spec.id, spec.start, spec.end, 1, false)[0]
		if i == 2 {
			base.SourceRef.QueryLineStart = 20
			base.SourceRef.QueryLineEnd = 40
		}
		count := 0
		wait := base
		wait.ID = spec.id + "#target_window_wait_occurrences"
		wait.Predicate = "target_window_wait_occurrences"
		wait.Object = "complete"
		wait.Value = "0"
		wait.ResultCount = &count
		wait.Span = ObservationSpan{StartTs: spec.start, EndTs: spec.end}
		edge := traceWakeupEdgeRoleTestRecord(spec.id, "worker-200", "ui-100", "20/ohos_cfs", "52/ohos_rt")
		edge.ID = spec.id + "#wakeup_chain_edge:1"
		edge.SourceRef = base.SourceRef
		rows = append(rows, wait, edge)
	}
	ledger := ObservationLedger{Records: rows, RuntimeArtifactScopeProfile: profile, AnchorUserEntities: []AnchorUserEntity{{Value: "100", TypedLane: true}}}
	before, _ := json.Marshal(ledger)
	waits := BuildTraceTargetWaitSummaryAuthorities(ledger, rm)
	if len(waits) != 3 {
		t.Fatalf("query-specific complete rosters lost: %+v", waits)
	}
	principal := 0
	for _, wait := range waits {
		if wait.Count != 0 || wait.WallClockMS != 0 {
			t.Fatal("changed native empty roster")
		}
		if wait.IsRequestedScopePrincipal() {
			principal++
			if wait.WindowScope.Role != TraceQueryWindowScopeRequestedPrincipal {
				t.Fatal("scope faces disagree")
			}
		} else if wait.WindowScope.Role != TraceQueryWindowScopeSupportingExploration {
			t.Fatal("filtered roster misclassified")
		}
	}
	if principal != 2 {
		t.Fatalf("principal rosters=%d: %+v", principal, waits)
	}
	edges := BuildTraceWakeupEdgeRoleAuthorities(ledger, rm)
	if len(edges) != 2 {
		t.Fatalf("wakeups need exact own-result query domain: %+v", edges)
	}
	for _, edge := range edges {
		if edge.WakerPriority != "20/ohos_cfs" || edge.WakeePriority != "52/ohos_rt" || edge.WindowScope.RequestedWindowOrdinal == 0 {
			t.Errorf("endpoint values/member scope lost: %+v", edge)
		}
	}
	after, _ := json.Marshal(ledger)
	if string(before) != string(after) {
		t.Fatal("reader mutated observation inputs")
	}
}

func TestB1626RequestedMemberParentRulerPrecedesNativeAttachments(t *testing.T) {
	row := b1626ProjectionRows("rank-only", 10, 10.01, 10, true)[2]
	row.RichNotes = []string{"rank=1", "tier=primary", "effective_impact_ms=2", "selected_window=10.020000..10.050000"}
	row.Span = ObservationSpan{StartTs: 10.001, EndTs: 10.003}
	ledger := ObservationLedger{Records: []ObservationRecord{row}, RuntimeArtifactScopeProfile: b1626ProjectionProfile(), AnchorUserEntities: []AnchorUserEntity{{Value: "100", TypedLane: true}}}
	set := CompileTraceCausalProjectionSet(ledger)
	if len(set.Projections) != 1 || !set.Projections[0].Active() {
		t.Fatalf("pure rank result lost: %+v", set)
	}
	p := set.Projections[0]
	if p.WindowStartTs != 10 || p.WindowEndTs != 10.01 || len(p.RankedSeats) != 1 || p.RankedSeats[0].WithinRequestedWindow == nil || !*p.RankedSeats[0].WithinRequestedWindow {
		t.Fatalf("leaf window governed parent attachment: board=%.6f..%.6f ranked seats=%d", p.WindowStartTs, p.WindowEndTs, len(p.RankedSeats))
	}
	if p.RankedSeats[0].QueryWindowStartTs != 10.02 || p.RankedSeats[0].QueryWindowEndTs != 10.05 || p.RankedSeats[0].ImpactMS != 2 {
		t.Fatal("native local window/value was overwritten")
	}
}

func TestB1626RequestedMemberCountIsNotArtifactCap(t *testing.T) {
	p := &RuntimeArtifactScopeProfile{RequestedScope: RuntimeArtifactScopeExplicitWindow}
	var rows []ObservationRecord
	for i := 0; i < 5; i++ {
		start, end := float64(i+1), float64(i+1)+.01
		p.TimeWindows = append(p.TimeWindows, RuntimeArtifactTimeWindow{TimeStart: &start, TimeEnd: &end, SourceQuote: fmt.Sprint("window", i)})
		rows = append(rows, b1626ProjectionRows(fmt.Sprint("q", i), start, end, 10, true)...)
	}
	ledger := ObservationLedger{Records: rows, RuntimeArtifactScopeProfile: p, AnchorUserEntities: []AnchorUserEntity{{Value: "100", TypedLane: true}}}
	set := CompileTraceCausalProjectionSet(ledger)
	if len(set.Projections) != 5 || len(set.OmittedArtifactLabels) != 0 || len(TraceCausalProjectionSetArtifactLabels(set)) != 1 {
		t.Fatalf("member became artifact/cap: %+v", set)
	}
	for i, p := range set.Projections {
		if p.WindowScope.RequestedWindowOrdinal != i+1 || p.WindowScope.RequestedWindowCount != 5 {
			t.Fatalf("request member order lost: %+v", p.WindowScope)
		}
	}
	ledger.RuntimeArtifactScopeProfile = b1619WindowProfile(1, 1.01)
	legacy := CompileTraceCausalProjectionSet(ledger)
	if len(legacy.Projections) != 1 || legacy.Projections[0].QuerySourceRef != nil || legacy.Projections[0].WindowScope.RequestedWindowCount != 0 {
		t.Fatal("legacy single-window shape changed")
	}
}
