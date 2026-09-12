package types

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"
)

func TestB1626ScopeJSONAndLedgerRetainMembersWithoutEnvelope(t *testing.T) {
	const raw = `{"requested_scope":"explicit_time_window","source_quote":"0..0.003 and 1..1.030","time_windows":[{"time_start":0,"time_end":0.003,"source_quote":"0..0.003"},{"time_start":1,"time_end":1.030,"source_quote":"1..1.030"}]}`
	var p RuntimeArtifactScopeProfile
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatal(err)
	}
	ledger := CompileObservationLedger(ObservationLedgerInput{RequestModel: &RequestModel{RuntimeArtifactScopeProfile: &p}})
	for _, candidate := range []*RuntimeArtifactScopeProfile{&p, ledger.RuntimeArtifactScopeProfile} {
		encoded, err := json.Marshal(candidate)
		if err != nil {
			t.Fatal(err)
		}
		var got, want map[string]any
		if err := json.Unmarshal(encoded, &got); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(raw), &want); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("public scope/ledger JSON lost requested members: %s", encoded)
		}
		if !candidate.Active() {
			t.Error("valid multi-member scope is not active")
		}
		if _, _, ok := candidate.ExplicitTimeWindow(); ok {
			t.Error("collection must not masquerade as one window")
		}
	}
}

func b1626Window(start, end float64, quote string) RuntimeArtifactTimeWindow {
	return RuntimeArtifactTimeWindow{TimeStart: &start, TimeEnd: &end, SourceQuote: quote}
}

func TestB1626MembersKeepOrderAmbiguityAndIndividualContainment(t *testing.T) {
	p := &RuntimeArtifactScopeProfile{RequestedScope: RuntimeArtifactScopeExplicitWindow, TimeWindows: []RuntimeArtifactTimeWindow{
		b1626Window(4, 5, "later"), b1626Window(0, 1, "zero"), b1626Window(0.25, 0.75, "nested"),
	}}
	if !p.Active() || !p.HasExplicitTimeWindows() {
		t.Fatal("member quotes suffice without a fabricated top-level quote")
	}
	for _, tc := range []struct {
		start, end       float64
		index            int
		match, contained bool
	}{
		{4, 5, 0, true, true}, {0, 1, 1, true, true}, {0.25, 0.75, 2, true, true},
		{0.1, 0.9, -1, false, true}, {0, 5, -1, false, false}, {0.5, 4.5, -1, false, false},
		{2, 3, -1, false, false}, {0, 0, -1, false, false},
	} {
		index, ok := p.MatchExplicitTimeWindow(tc.start, tc.end)
		if index != tc.index || ok != tc.match || p.ContainsExplicitTimeWindow(tc.start, tc.end) != tc.contained {
			t.Errorf("member lookup %.6f..%.6f = (%d,%t) contains=%t", tc.start, tc.end, index, ok, p.ContainsExplicitTimeWindow(tc.start, tc.end))
		}
	}
	for _, shift := range []float64{0, TraceCausalProjectionPrincipalValueWindowToleranceS / 2} {
		duplicate := CloneRuntimeArtifactScopeProfile(p)
		duplicate.TimeWindows = append(duplicate.TimeWindows, b1626Window(4+shift, 5+shift, "independent repeated request"))
		if index, ok := duplicate.MatchExplicitTimeWindow(4, 5); ok || index != -1 {
			t.Fatalf("duplicate/precision overlap selected first member: %d %t", index, ok)
		}
		if len(duplicate.ExplicitTimeWindows()) != 4 || !duplicate.HasExplicitTimeWindows() {
			t.Fatal("ambiguous membership silently dropped or invalidated original members")
		}
	}
	got := p.ExplicitTimeWindows()
	*got[0].TimeStart = 99
	got[1].SourceQuote = "changed"
	if *p.TimeWindows[0].TimeStart != 4 || p.TimeWindows[1].SourceQuote != "zero" {
		t.Fatal("member accessor aliases request storage")
	}
}

func TestB1626WholeGroupValidityAndLegacySingleWindow(t *testing.T) {
	base := RuntimeArtifactScopeProfile{RequestedScope: RuntimeArtifactScopeExplicitWindow, TimeWindows: []RuntimeArtifactTimeWindow{b1626Window(0, 1, "first"), b1626Window(3, 4, "second")}}
	for _, invalid := range []RuntimeArtifactTimeWindow{
		{}, b1626Window(-1, 2, "negative"), b1626Window(2, 2, "empty"), b1626Window(3, 2, "reversed"),
		b1626Window(0, math.Inf(1), "infinite"), b1626Window(math.NaN(), 2, "nan"), b1626Window(0, 1, ""),
	} {
		p := CloneRuntimeArtifactScopeProfile(&base)
		p.TimeWindows[1] = invalid
		if p.Active() || p.HasExplicitTimeWindows() || len(p.ExplicitTimeWindows()) != 0 {
			t.Fatal("invalid later member was discarded to authorize a valid prefix")
		}
	}
	one := b1626Window(0, 1, "single")
	legacy := &RuntimeArtifactScopeProfile{RequestedScope: RuntimeArtifactScopeExplicitWindow, TimeStart: one.TimeStart, TimeEnd: one.TimeEnd, SourceQuote: one.SourceQuote}
	list := &RuntimeArtifactScopeProfile{RequestedScope: RuntimeArtifactScopeExplicitWindow, TimeWindows: []RuntimeArtifactTimeWindow{one}}
	for _, p := range []*RuntimeArtifactScopeProfile{legacy, list} {
		if start, end, ok := p.ExplicitTimeWindow(); !ok || start != 0 || end != 1 {
			t.Fatal("old and new true-single forms differ")
		}
	}
	mixed := CloneRuntimeArtifactScopeProfile(legacy)
	mixed.TimeWindows = list.TimeWindows
	if mixed.HasExplicitTimeWindows() {
		t.Fatal("ambiguous scalar/list pair got authority")
	}
	mixed.TimeWindows = []RuntimeArtifactTimeWindow{}
	if mixed.HasExplicitTimeWindows() {
		t.Fatal("empty list fell back to scalar")
	}
	if (*RuntimeArtifactScopeProfile)(nil).HasExplicitTimeWindows() {
		t.Fatal("nil gained authority")
	}
}

func TestB1626ScopeCollectionDoesNotSilentlyTruncateMembers(t *testing.T) {
	p := &RuntimeArtifactScopeProfile{RequestedScope: RuntimeArtifactScopeExplicitWindow}
	for i := 0; i < 40; i++ {
		start := float64(i * 2)
		p.TimeWindows = append(p.TimeWindows, b1626Window(start, start+1, "current-request member"))
	}
	got := p.ExplicitTimeWindows()
	if len(got) != 40 {
		t.Fatal("scope authority silently inherited a display cap")
	}
	if index, ok := p.MatchExplicitTimeWindow(78, 79); !ok || index != 39 {
		t.Fatal("last member lost request identity")
	}
	ledger := CompileObservationLedger(ObservationLedgerInput{RequestModel: &RequestModel{RuntimeArtifactScopeProfile: p}})
	if !reflect.DeepEqual(ledger.RuntimeArtifactScopeProfile, p) {
		t.Fatal("ledger truncated/reordered full member authority")
	}
}

func TestB1626ScopeCopiesCloseMutableForkAndLedger(t *testing.T) {
	p := &RuntimeArtifactScopeProfile{RequestedScope: RuntimeArtifactScopeExplicitWindow, TimeWindows: []RuntimeArtifactTimeWindow{b1626Window(0, 1, "first"), b1626Window(3, 4, "second")}}
	rm := RequestModel{RuntimeArtifactScopeProfile: p}
	mu := NewMutableState("request")
	mu.SetRequestModel(rm)
	*p.TimeWindows[0].TimeStart = 100
	if *mu.RequestModel().RuntimeArtifactScopeProfile.TimeWindows[0].TimeStart != 0 {
		t.Fatal("stored request aliases producer endpoints")
	}
	snapshot := mu.RequestModel()
	*snapshot.RuntimeArtifactScopeProfile.TimeWindows[1].TimeEnd = 100
	if *mu.RequestModel().RuntimeArtifactScopeProfile.TimeWindows[1].TimeEnd != 4 {
		t.Fatal("getter aliases stored member endpoints")
	}
	fork := mu.ForkForExploreDispatch()
	*fork.requestModel.RuntimeArtifactScopeProfile.TimeWindows[0].TimeEnd = 100
	if *mu.RequestModel().RuntimeArtifactScopeProfile.TimeWindows[0].TimeEnd != 1 {
		t.Fatal("explore fork aliases parent request")
	}
	stored := mu.RequestModel()
	ledger := CompileObservationLedger(ObservationLedgerInput{RequestModel: stored})
	*ledger.RuntimeArtifactScopeProfile.TimeWindows[0].TimeEnd = 99
	if *stored.RuntimeArtifactScopeProfile.TimeWindows[0].TimeEnd != 1 {
		t.Fatal("ledger aliases request members")
	}
	legacy := &RuntimeArtifactScopeProfile{RequestedScope: RuntimeArtifactScopeExplicitWindow, SourceQuote: "one", TimeStart: b1626Window(0, 1, "one").TimeStart, TimeEnd: b1626Window(0, 1, "one").TimeEnd}
	copied := CloneRuntimeArtifactScopeProfile(legacy)
	*copied.TimeStart = 9
	if *legacy.TimeStart != 0 || copied.TimeWindows != nil {
		t.Fatal("legacy pointer/nil shape copy changed")
	}
}

func TestB1626RuntimeShapeKeepsBoundedFactsAndExplicitCausalGuardSeparate(t *testing.T) {
	p := &RuntimeArtifactScopeProfile{RequestedScope: RuntimeArtifactScopeExplicitWindow, TimeWindows: []RuntimeArtifactTimeWindow{b1626Window(0, 1, "one"), b1626Window(3, 4, "two")}}
	rm := RequestModel{RuntimeArtifactScopeProfile: p, Intent: IntentExplain}
	if decided, allowed := RuntimeTraceReportShapeAuthority(&rm); !decided || !allowed {
		t.Fatal("explicit members lost old explicit-window report authority")
	}
	rm.RuntimeQuestionProfile = &RuntimeQuestionProfile{Scope: RuntimeQuestionScopeBoundedFactSet, FactFamilies: []RuntimeQuestionFactFamily{RuntimeQuestionFactTargetSchedulerState}}
	if decided, allowed := RuntimeTraceReportShapeAuthority(&rm); !decided || allowed {
		t.Fatal("multiple windows forced a causal report for bounded facts")
	}
	rm.LogTriage = &LogBundle{OperationalSemantics: []LogOperationalSemantic{{TransitionAuthority: LogOperationalTransitionEventLocalOnly}, {TransitionAuthority: LogOperationalTransitionEventLocalOnly}}}
	if rootCauseCrossEventTransitionUnproven(&AnalysisIR{RequestModel: rm}) {
		t.Fatal("typed explicit members lost the old root-cause guard exemption")
	}
	rm.RuntimeArtifactScopeProfile = nil
	if !rootCauseCrossEventTransitionUnproven(&AnalysisIR{RequestModel: rm}) {
		t.Fatal("unscoped event-local relation was accidentally upgraded")
	}
}
