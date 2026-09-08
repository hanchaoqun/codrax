package types

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

// The compiler must keep the elected exploration account, while the shared
// reader formatter distinguishes its ruler from the requested answer ruler.
func TestB1619CompiledExplorationAccountDisclosesRequestedWindow(t *testing.T) {
	ledger := ObservationLedger{
		Records:                     requestedWindowAuthorityFixture(false),
		RuntimeArtifactScopeProfile: requestedWindowAuthorityProfile(),
		AnchorUserEntities:          []AnchorUserEntity{{Value: "100", TypedLane: true}},
	}
	before, err := json.Marshal(ledger.Records)
	if err != nil {
		t.Fatal(err)
	}
	projection := CompileTraceCausalProjection(ledger)
	if projection.WindowStartTs != 10.02 || projection.WindowEndTs != 10.07 ||
		projection.TargetStateAccount == nil || projection.TargetStateAccount.TotalMS != 50 ||
		len(projection.WakeupPath) != 2 || projection.WakeupPath[0] != "worker-sub-200" {
		t.Fatalf("legacy exploration election/account must remain intact: %+v", projection)
	}
	authorities := BuildTraceTargetStateScopeAuthoritiesFromLedger(ledger)
	if len(authorities) != 1 || authorities[0].TotalMS != 50 || authorities[0].RunnableMS != 5 {
		t.Fatalf("original exploration state values must remain: %+v", authorities)
	}
	for _, tc := range []struct {
		lang string
		want []string
	}{
		{"zh", []string{"用户指定范围 10.000000–10.100000", "补充查询范围 10.020000–10.070000", "不能代替指定范围的独立账户"}},
		{"en", []string{"requested window 10.000000–10.100000", "supplementary query window 10.020000–10.070000", "does not substitute for an account of the requested window"}},
	} {
		t.Run(tc.lang, func(t *testing.T) {
			got := FormatTargetStateAccount(authorities[0], tc.lang)
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("shared account formatter lost scope boundary %q: %s", want, got)
				}
			}
		})
	}
	after, err := json.Marshal(ledger.Records)
	if err != nil || !reflect.DeepEqual(after, before) {
		t.Fatal("scope disclosure must not mutate observation records")
	}
}

func b1619WindowProfile(start, end float64) *RuntimeArtifactScopeProfile {
	return &RuntimeArtifactScopeProfile{
		RequestedScope: RuntimeArtifactScopeExplicitWindow,
		TimeStart:      &start, TimeEnd: &end, SourceQuote: "a validated explicit window",
	}
}

func TestB1619QueryWindowScopeUsesExactTypedRulers(t *testing.T) {
	for _, tc := range []struct {
		name       string
		profile    *RuntimeArtifactScopeProfile
		start, end float64
		role       TraceQueryWindowScopeRole
		requested  bool
	}{
		{"exact", b1619WindowProfile(2, 2.020), 2, 2.020, TraceQueryWindowScopeRequestedPrincipal, true},
		{"representation_drift", b1619WindowProfile(2, 2.020), 2.000001, 2.020001, TraceQueryWindowScopeRequestedPrincipal, true},
		{"twenty_us_is_not_same", b1619WindowProfile(2, 2.020), 2, 2.020020, TraceQueryWindowScopeSupportingExploration, true},
		{"wider", b1619WindowProfile(2, 2.020), 2, 2.021, TraceQueryWindowScopeSupportingExploration, true},
		{"narrower", b1619WindowProfile(2, 2.020), 2.001, 2.019, TraceQueryWindowScopeSupportingExploration, true},
		{"disjoint", b1619WindowProfile(2, 2.020), 3, 3.020, TraceQueryWindowScopeSupportingExploration, true},
		{"rebased_zero", b1619WindowProfile(0, 0.020), 0, 0.020, TraceQueryWindowScopeRequestedPrincipal, true},
		{"zero_unknown", b1619WindowProfile(0, 0.020), 0, 0, TraceQueryWindowScopeUnknownQueryWindow, true},
		{"no_query_window", b1619WindowProfile(2, 2.020), 0, 0, TraceQueryWindowScopeUnknownQueryWindow, true},
		{"negative_query", b1619WindowProfile(2, 2.020), -1, 0, TraceQueryWindowScopeUnknownQueryWindow, true},
		{"nan_query", b1619WindowProfile(2, 2.020), math.NaN(), 2.020, TraceQueryWindowScopeUnknownQueryWindow, true},
		{"inf_query", b1619WindowProfile(2, 2.020), 2, math.Inf(1), TraceQueryWindowScopeUnknownQueryWindow, true},
		{"legacy", nil, 2, 2.021, TraceQueryWindowScopeElectedQueryWindow, false},
		{"legacy_unknown", nil, 0, 0, TraceQueryWindowScopeUnknownQueryWindow, false},
		{"no_anchored_quote", &RuntimeArtifactScopeProfile{RequestedScope: RuntimeArtifactScopeExplicitWindow, TimeStart: ptrFloatB1619(2), TimeEnd: ptrFloatB1619(2.020)}, 2, 2.021, TraceQueryWindowScopeElectedQueryWindow, false},
		{"full_artifact_not_explicit_time", &RuntimeArtifactScopeProfile{RequestedScope: RuntimeArtifactScopeFullArtifact, SourceQuote: "the whole trace"}, 2, 2.021, TraceQueryWindowScopeElectedQueryWindow, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scope := ResolveTraceQueryWindowScope(tc.profile, tc.start, tc.end)
			if scope.Role != tc.role || scope.RequestedWindowKnown != tc.requested ||
				scope.IsSupportingExploration() != (tc.role == TraceQueryWindowScopeSupportingExploration) {
				t.Fatalf("scope=%+v, want %s requested=%v", scope, tc.role, tc.requested)
			}
			if tc.requested && (scope.RequestedWindowStartTs != *tc.profile.TimeStart || scope.RequestedWindowEndTs != *tc.profile.TimeEnd) {
				t.Fatalf("request ruler changed: %+v", scope)
			}
			if tc.role != TraceQueryWindowScopeUnknownQueryWindow && (scope.QueryWindowStartTs != tc.start || scope.QueryWindowEndTs != tc.end) {
				t.Fatalf("query ruler changed: %+v", scope)
			}
			for _, lang := range []string{"zh-CN", "en"} {
				text := scope.Format(lang)
				if text == "" || strings.ContainsAny(text, "\n\r") || strings.Contains(text, string(tc.role)) {
					t.Fatalf("missing, multiline, or internal-role disclosure: %q", text)
				}
				if strings.Contains(text, "覆盖完整") || strings.Contains(text, "complete coverage") || strings.Contains(text, "尚无独立") {
					t.Fatalf("one query's scope cannot establish global coverage/absence: %q", text)
				}
			}
			if _, err := json.Marshal(scope); err != nil {
				t.Fatalf("unknown query must not introduce non-finite wire values: %v", err)
			}
		})
	}
	if (TraceQueryWindowScope{}).Format("zh") != "" {
		t.Fatal("legacy unstamped carriers must not invent a request scope")
	}
}

func ptrFloatB1619(v float64) *float64 { return &v }

func TestB1619ScopeForWindowKeepsRequestIdentityByValue(t *testing.T) {
	profile := b1619WindowProfile(2, 2.020)
	wide := ResolveTraceQueryWindowScope(profile, 2, 2.021)
	*profile.TimeEnd = 99
	exact := wide.ForWindow(2, 2.020)
	unknown := wide.ForWindow(0, 0)
	if wide.Role != TraceQueryWindowScopeSupportingExploration || wide.RequestedWindowEndTs != 2.020 || wide.QueryWindowEndTs != 2.021 ||
		exact.Role != TraceQueryWindowScopeRequestedPrincipal || exact.RequestedWindowEndTs != 2.020 ||
		unknown.Role != TraceQueryWindowScopeUnknownQueryWindow || !unknown.RequestedWindowKnown {
		t.Fatalf("request identity lost or alias-mutated: wide=%+v exact=%+v unknown=%+v", wide, exact, unknown)
	}
}

func b1619IOWindowRecords(id, path string, exact bool) []ObservationRecord {
	window, total, runnable := "2.000000..2.021000", "20.020", "0.020"
	if exact {
		window, total, runnable = "2.000000..2.020000", "20.000", "0.000"
	}
	rows := []ObservationRecord{
		requestedWindowAuthorityRecord(id+"#path", "wakeup_chain", "app-100", "threadpool-400 -> network-300 -> cookie-200 -> app-100", "", "branch=1", "selected_window="+window),
		requestedWindowAuthorityRecord(id+"#rank", "root_cause_primary", "threadpool-400", "io_wait", "11.000", "rank=1", "tier=primary", "chain_relevance=on_chain", "effective_impact_ms=11.000", "selected_window="+window),
		requestedWindowAuthorityRecord(id+"#state", "target_window_states", "app-100", "state_partition", total, "selected_window="+window, "running=0.000", "runnable="+runnable, "sleep=20.000", "d_state=0.000", "io_wait=0.000", "total="+total),
	}
	for i := range rows {
		rows[i].SourceRef.Path = path
	}
	return rows
}

func TestB1619TwentyMSRequestKeepsTwentyOneMSExplorationUntilExactSupplement(t *testing.T) {
	ledger := ObservationLedger{
		Records:                     b1619IOWindowRecords("wide", "/capture/customer.trace", false),
		RuntimeArtifactScopeProfile: b1619WindowProfile(2, 2.020),
		AnchorUserEntities:          []AnchorUserEntity{{Value: "100", TypedLane: true}},
	}
	wide := CompileTraceCausalProjection(ledger)
	legacyLedger := ledger
	legacyLedger.RuntimeArtifactScopeProfile = nil
	legacy := CompileTraceCausalProjection(legacyLedger)
	wideWithoutScope, legacyWithoutScope := wide, legacy
	wideWithoutScope.WindowScope, legacyWithoutScope.WindowScope = TraceQueryWindowScope{}, TraceQueryWindowScope{}
	if !reflect.DeepEqual(wideWithoutScope, legacyWithoutScope) {
		t.Fatal("scope classification changed an existing projection field, node, or value")
	}
	if !wide.WindowScope.IsSupportingExploration() || wide.WindowEndTs != 2.021 || wide.TargetStateAccount == nil ||
		wide.TargetStateAccount.RunnableMS != 0.020 || wide.TargetStateAccount.TotalMS != 20.020 ||
		len(wide.WakeupPath) != 4 || wide.PrimaryRootCause == nil || wide.PrimaryRootCause.ImpactMS != 11 {
		t.Fatalf("actual exploration graph/IO/state must remain unchanged: %+v", wide)
	}
	accounts := BuildTraceTargetStateScopeAuthoritiesFromLedger(ledger)
	if len(accounts) != 1 || !accounts[0].WindowScope.IsSupportingExploration() || accounts[0].RunnableMS != .020 {
		t.Fatalf("state authority must inherit the query ruler without clipping .020: %+v", accounts)
	}
	text := FormatTargetStateAccount(accounts[0], "zh")
	if !strings.Contains(text, "补充查询范围 2.000000–2.021000") || !strings.Contains(text, "本查询范围内：") || !strings.Contains(text, "0.020 毫秒") {
		t.Fatalf("wide state must be precise supplemental data: %s", text)
	}
	ledger.Records = append(ledger.Records, b1619IOWindowRecords("exact", "/capture/customer.trace", true)...)
	exact := CompileTraceCausalProjection(ledger)
	if exact.WindowScope.Role != TraceQueryWindowScopeRequestedPrincipal || exact.WindowEndTs != 2.020 ||
		exact.TargetStateAccount == nil || exact.TargetStateAccount.TotalMS != 20 || exact.TargetStateAccount.RunnableMS != 0 {
		t.Fatalf("existing exact-window supplement election must remain: %+v", exact)
	}
	if len(ledger.Records) != 6 || ledger.Records[2].Value != "20.020" || !exact.WindowScope.ForWindow(2, 2.021).IsSupportingExploration() {
		t.Fatal("exact supplement must not erase the earlier query or relabel its original value")
	}
}

func TestB1619PerCaptureScopeAndFiniteStateFallbackRemainIndependent(t *testing.T) {
	rows := append(b1619IOWindowRecords("wide", "/capture/A/customer.trace", false), b1619IOWindowRecords("exact", "/capture/B/customer.trace", true)...)
	ledger := ObservationLedger{Records: rows, RuntimeArtifactScopeProfile: b1619WindowProfile(2, 2.020), AnchorUserEntities: []AnchorUserEntity{{Value: "100", TypedLane: true}}}
	set := CompileTraceCausalProjectionSet(ledger)
	if len(set.Projections) != 2 {
		t.Fatalf("same-basename captures must remain two projections: %+v", set)
	}
	for _, p := range set.Projections {
		switch p.ArtifactPath {
		case "/capture/A/customer.trace":
			if !p.WindowScope.IsSupportingExploration() || p.TargetStateAccount.RunnableMS != .020 {
				t.Fatalf("wide capture borrowed exact capture scope: %+v", p)
			}
		case "/capture/B/customer.trace":
			if p.WindowScope.Role != TraceQueryWindowScopeRequestedPrincipal || p.TargetStateAccount.RunnableMS != 0 {
				t.Fatalf("exact capture lost its ruler: %+v", p)
			}
		default:
			t.Fatalf("unexpected capture: %+v", p)
		}
	}
	// No causal projection is required for the existing finite state fallback.
	ledger.Records = []ObservationRecord{rows[5]}
	authorities := BuildTraceTargetStateScopeAuthoritiesFromLedger(ledger)
	if len(authorities) != 1 || authorities[0].WindowScope.Role != TraceQueryWindowScopeRequestedPrincipal || authorities[0].TotalMS != 20 {
		t.Fatalf("finite exact state fallback lost scope or original value: %+v", authorities)
	}
}
