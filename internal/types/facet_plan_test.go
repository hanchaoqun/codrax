package types

import (
	"testing"
)

// ── ResolveQuestionFamily ──────────────────────────────────────────

func TestResolveQuestionFamily_RootCauseTraceWithLog(t *testing.T) {
	rm := RequestModel{
		Intent:    IntentRootCause,
		LogTriage: &LogBundle{},
	}
	if got := ResolveQuestionFamily(rm); got != QFRootCauseTrace {
		t.Errorf("got %q, want QFRootCauseTrace", got)
	}
}

func TestResolveQuestionFamily_RootCauseTraceWithPerf(t *testing.T) {
	rm := RequestModel{
		Intent:    IntentTrace,
		PerfTrace: &PerfBundle{},
	}
	if got := ResolveQuestionFamily(rm); got != QFRootCauseTrace {
		t.Errorf("got %q, want QFRootCauseTrace", got)
	}
}

func TestResolveQuestionFamily_TraceWithoutArtifactGoesToCallChain(t *testing.T) {
	// Trace intent but NO log/perf attached → call chain, not
	// root cause. Distinguishes "trace this panic" from "how does X
	// reach Y".
	rm := RequestModel{Intent: IntentTrace}
	if got := ResolveQuestionFamily(rm); got != QFCallChain {
		t.Errorf("got %q, want QFCallChain", got)
	}
}

func TestResolveQuestionFamily_ConfigPrecedence(t *testing.T) {
	cases := []struct {
		name string
		rm   RequestModel
	}{
		{"intent config_query", RequestModel{Intent: IntentConfigQuery}},
		{"scenario config_trace", RequestModel{Intent: IntentExplain, Scenario: ScenarioConfigTrace}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ResolveQuestionFamily(c.rm); got != QFConfigPrecedence {
				t.Errorf("got %q, want QFConfigPrecedence", got)
			}
		})
	}
}

func TestResolveQuestionFamily_RoleLookup(t *testing.T) {
	cases := []AnswerSubjectKind{
		SubjectFunctionName, SubjectHandlerRoute,
		SubjectConfigKey, SubjectStructField, SubjectInterface,
	}
	for _, kind := range cases {
		t.Run(string(kind), func(t *testing.T) {
			rm := RequestModel{
				Intent:        IntentExplain,
				AnswerSubject: AnswerSubject{Kind: kind, Confidence: 0.8},
			}
			if got := ResolveQuestionFamily(rm); got != QFRoleLookup {
				t.Errorf("got %q, want QFRoleLookup", got)
			}
		})
	}
}

func TestResolveQuestionFamily_ObligationOverridesRoleLookup(t *testing.T) {
	// AnswerSubject says FunctionName (which would go RoleLookup)
	// BUT EnumerationBoundary is set — obligation wins.
	rm := RequestModel{
		Intent:              IntentExplain,
		AnswerSubject:       AnswerSubject{Kind: SubjectFunctionName, Confidence: 0.8},
		EnumerationBoundary: &RequestedEnumerationBoundary{DeclaredCount: 5, SourceQuote: "5 X"},
	}
	if got := ResolveQuestionFamily(rm); got != QFEnumeration {
		t.Errorf("obligation must override role-lookup; got %q want QFEnumeration", got)
	}
}

// TestResolveQuestionFamily_BucketsRouteToComparison: buckets >= 2
// route to QFComparison (R4.4) — pre-R4.4 fell to QFEnumeration
// via the obligation rule + family_underrepresented telemetry.
func TestResolveQuestionFamily_BucketsRouteToComparison(t *testing.T) {
	rm := RequestModel{
		Intent: IntentExplain,
		Buckets: []QuestionBucket{
			{Label: "A", Index: 1},
			{Label: "B", Index: 2},
		},
	}
	if got := ResolveQuestionFamily(rm); got != QFComparison {
		t.Errorf("got %q, want QFComparison", got)
	}
}

func TestResolveQuestionFamily_EnumerationFromCompleteness(t *testing.T) {
	rm := RequestModel{
		Intent: IntentExplain,
		CompletenessObligation: &CompletenessObligation{
			Required: true, SourceQuote: "all",
		},
	}
	if got := ResolveQuestionFamily(rm); got != QFEnumeration {
		t.Errorf("got %q, want QFEnumeration", got)
	}
}

func TestResolveQuestionFamily_Architecture(t *testing.T) {
	rm := RequestModel{
		Intent:   IntentExplain,
		Scenario: ScenarioArchitectureExplain,
	}
	if got := ResolveQuestionFamily(rm); got != QFArchitecture {
		t.Errorf("got %q, want QFArchitecture", got)
	}
}

func TestResolveQuestionFamily_GenericFallthrough(t *testing.T) {
	// No intent / no scenario / no subject / no obligation → generic.
	rm := RequestModel{}
	if got := ResolveQuestionFamily(rm); got != QFGeneric {
		t.Errorf("got %q, want QFGeneric", got)
	}
}

// ── CompileFacetCoverage ───────────────────────────────────────────

func TestCompileFacetCoverage_HardDegradesToSoftWhenNoCandidate(t *testing.T) {
	// QFRootCauseTrace template has FacetObservedArtifactFact as
	// HARD with AcceptableForms=[ClaimExternalObservation]. Provide
	// non-empty surface evidence whose ClaimForm is NOT
	// ClaimExternalObservation → no candidate matches → HARD must
	// degrade to SOFT.
	//
	// R3.1 (2026-05-04): empty surface is now short-circuited as
	// "inconclusive" (no evidence collected yet ≠ uncoverable), so
	// the downgrade test must pass NON-empty surface that fails
	// the AcceptableForms match — the real "evidence collected
	// but doesn't fit" case the rule targets.
	rm := RequestModel{
		Intent:    IntentRootCause,
		LogTriage: &LogBundle{},
	}
	// AnchorDefinition → ClaimDefinitionFact, NOT acceptable for
	// FacetObservedArtifactFact (which requires
	// ClaimExternalObservation).
	surface := []EvidenceItem{
		{ID: "e1", Source: "x.go", AnchorKind: AnchorDefinition},
	}
	plan := CompileFacetCoverage(rm, surface)
	if plan == nil {
		t.Fatal("plan must be non-nil for QFRootCauseTrace")
	}
	if plan.Family != QFRootCauseTrace {
		t.Errorf("family = %q, want QFRootCauseTrace", plan.Family)
	}
	for _, req := range plan.Required {
		if req.Kind == FacetObservedArtifactFact && req.Required == FacetHardRequired {
			t.Errorf("FacetObservedArtifactFact must degrade to SOFT when no candidate; got HARD")
		}
	}
}

// TestCompileFacetCoverage_EmptySurfaceInconclusiveNoSoftening
// pins R3.1 (post_shape_residual_audit.md, 2026-05-04): when
// CompileFacetCoverage runs at analyzer-time (before any emit_evidence
// call) surface is empty and we MUST NOT silently downgrade
// HARD facets — the lack of evidence is "not yet collected", not
// "facet uncoverable". Telemetry MUST also stay silent.
func TestCompileFacetCoverage_EmptySurfaceInconclusiveNoSoftening(t *testing.T) {
	rm := RequestModel{
		Intent:    IntentRootCause,
		LogTriage: &LogBundle{},
	}
	sink := &fakeRichnessSink{}
	// nil surface — analyzer-time / pre-evidence call shape.
	plan := CompileFacetCoverage(rm, nil, sink)
	if plan == nil {
		t.Fatal("plan must be non-nil")
	}
	hardKept := false
	for _, req := range plan.Required {
		if req.Kind == FacetObservedArtifactFact && req.Required == FacetHardRequired {
			hardKept = true
		}
	}
	if !hardKept {
		t.Errorf("HARD facet must remain HARD on empty surface (R3.1 inconclusive); got softened")
	}
	for _, sig := range sink.signals {
		if sig.Kind == "facet_softened" {
			t.Errorf("R3.1 must skip facet_softened telemetry on empty surface; got %+v", sig)
		}
	}
}

func TestCompileFacetCoverage_HardKeptWhenCandidateExists(t *testing.T) {
	// Provide a LogFrame-origin EvidenceItem so
	// FacetObservedArtifactFact has a SourceCandidate.
	rm := RequestModel{
		Intent:    IntentRootCause,
		LogTriage: &LogBundle{},
	}
	surface := []EvidenceItem{
		{ID: "ev-log-1", Origin: ClaimOriginLog, Source: "panic.log"},
	}
	plan := CompileFacetCoverage(rm, surface)
	if plan == nil {
		t.Fatal("plan must be non-nil")
	}
	foundHard := false
	for _, req := range plan.Required {
		if req.Kind == FacetObservedArtifactFact &&
			req.Required == FacetHardRequired &&
			contains(req.SourceCandidate, "ev-log-1") {
			foundHard = true
		}
	}
	if !foundHard {
		t.Errorf("FacetObservedArtifactFact must keep HARD with bound candidate; got %+v", plan.Required)
	}
}

func TestCompileFacetCoverage_BucketLabelInjectedFromQuestionStructure(t *testing.T) {
	rm := RequestModel{
		Intent: IntentExplain,
		Buckets: []QuestionBucket{
			{Label: "A", Index: 1}, {Label: "B", Index: 2},
		},
	}
	plan := CompileFacetCoverage(rm, nil)
	if plan == nil {
		t.Fatal("plan must be non-nil")
	}
	// Bucket label is HARD-required regardless of candidate presence
	// (bucket labels are user-named partitions, not evidence-bound)
	// — but the Phase-1 fallback rule degrades to SOFT when no
	// candidate exists. We check that the requirement APPEARS
	// somewhere (HARD or SOFT) so finalizer prompt can surface it.
	found := false
	for _, req := range append(plan.Required, plan.Optional...) {
		if req.Kind == FacetBucketLabel {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("FacetBucketLabel must appear when Buckets >= 2; got %+v", plan)
	}
}

func TestCompileFacetCoverage_EnumerationItemFromCount(t *testing.T) {
	rm := RequestModel{
		Intent:              IntentExplain,
		EnumerationBoundary: &RequestedEnumerationBoundary{DeclaredCount: 7, SourceQuote: "7 X"},
	}
	plan := CompileFacetCoverage(rm, nil)
	if plan == nil {
		t.Fatal("plan must be non-nil")
	}
	found := false
	for _, req := range append(plan.Required, plan.Optional...) {
		if req.Kind == FacetEnumerationItem {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("FacetEnumerationItem must appear when EnumerationBoundary set; got %+v", plan)
	}
}

func TestCompileFacetCoverage_GenericMinimal(t *testing.T) {
	plan := CompileFacetCoverage(RequestModel{}, nil)
	if plan == nil {
		t.Fatal("plan must be non-nil for QFGeneric")
	}
	if plan.Family != QFGeneric {
		t.Errorf("family = %q, want QFGeneric", plan.Family)
	}
}

func TestCompileFacetCoverage_NilOnNoTemplate(t *testing.T) {
	// Sanity: family resolver must not return an unmapped family
	// that yields nil template — guards against future
	// QuestionFamily additions without template definitions.
	for _, f := range []QuestionFamily{
		QFRootCauseTrace, QFConfigPrecedence, QFRoleLookup,
		QFCallChain, QFEnumeration, QFArchitecture, QFGeneric,
	} {
		t.Run(string(f), func(t *testing.T) {
			rm := familyToRequestModel(f)
			plan := CompileFacetCoverage(rm, nil)
			if plan == nil {
				t.Errorf("family %q yields nil plan — template missing", f)
			}
		})
	}
}

// ── claimFormMatches helper ────────────────────────────────────────

func TestClaimFormMatches_EmptyAllowedAcceptsNonUnknown(t *testing.T) {
	if !claimFormMatches(nil, ClaimDefinitionFact) {
		t.Error("empty allowed must accept any non-unknown")
	}
	if claimFormMatches(nil, ClaimUnknown) {
		t.Error("empty allowed must reject unknown")
	}
}

func TestClaimFormMatches_WhitelistFiltering(t *testing.T) {
	allowed := []ClaimForm{ClaimDefinitionFact, ClaimCallEdge}
	if !claimFormMatches(allowed, ClaimDefinitionFact) {
		t.Error("whitelist must accept listed form")
	}
	if !claimFormMatches(allowed, ClaimCallEdge) {
		t.Error("whitelist must accept listed form")
	}
	if claimFormMatches(allowed, ClaimGuardCondition) {
		t.Error("whitelist must reject non-listed form")
	}
}

// ── helpers ────────────────────────────────────────────────────────

func contains(s []string, target string) bool {
	for _, v := range s {
		if v == target {
			return true
		}
	}
	return false
}

// ── B5-F2 / B5-F3 richness telemetry tests ─────────────────────────

// fakeRichnessSink captures every signal that flows through the
// CompileFacetCoverage / ResolveQuestionFamily channel for a single
// test case. Mirrors MutableState.AppendRichnessTelemetry's surface
// (the production sink) so the production deduper does not interfere
// with assertions.
type fakeRichnessSink struct {
	signals []RichnessTelemetrySignal
}

func (f *fakeRichnessSink) AppendRichnessTelemetry(sig RichnessTelemetrySignal) {
	f.signals = append(f.signals, sig)
}

func TestCompileFacetCoverage_FacetSofteningTelemetryFires(t *testing.T) {
	// QFRootCauseTrace + non-empty surface evidence that DOESN'T
	// match the FacetObservedArtifactFact AcceptableForms (which
	// requires ClaimExternalObservation only). With a sink wired,
	// we expect at least one "facet_softened" signal.
	//
	// R3.1 (2026-05-04): emptySurface=nil now short-circuits the
	// downgrade path (treat as inconclusive — analyzer-time call
	// before any emit_evidence). The test must pass NON-empty
	// surface that fails to satisfy the facet AcceptableForms,
	// which is the genuine "evidence collected but doesn't match"
	// case the softening rule was designed for.
	rm := RequestModel{
		Intent:    IntentRootCause,
		LogTriage: &LogBundle{},
	}
	// AnchorDefinition → ClaimDefinitionFact, NOT in
	// FacetObservedArtifactFact AcceptableForms (which is
	// ClaimExternalObservation only).
	surface := []EvidenceItem{
		{ID: "e1", Source: "x.go", AnchorKind: AnchorDefinition},
	}
	sink := &fakeRichnessSink{}
	plan := CompileFacetCoverage(rm, surface, sink)
	if plan == nil {
		t.Fatal("plan must be non-nil")
	}
	gotSoftened := false
	for _, sig := range sink.signals {
		if sig.Kind == "facet_softened" {
			gotSoftened = true
			if sig.Family != string(QFRootCauseTrace) {
				t.Errorf("family on signal = %q, want QFRootCauseTrace", sig.Family)
			}
			if sig.FacetKind == "" {
				t.Errorf("facet_kind must be populated; got empty signal %+v", sig)
			}
			if sig.Reason == "" {
				t.Errorf("reason must be populated; got empty signal %+v", sig)
			}
		}
	}
	if !gotSoftened {
		t.Errorf("no facet_softened signal recorded; got %+v", sink.signals)
	}
}

func TestCompileFacetCoverage_NoSofteningWhenAllCovered(t *testing.T) {
	// QFRoleLookup + a candidate that satisfies the facet → no
	// HARD→SOFT degradation, no signal.
	rm := RequestModel{
		Intent: IntentExplain,
		AnswerSubject: AnswerSubject{
			Kind: SubjectFunctionName, Confidence: 0.8,
		},
	}
	surface := []EvidenceItem{
		{ID: "e1", Source: "x.go", AnchorKind: AnchorDefinition, Summary: "definition"},
	}
	sink := &fakeRichnessSink{}
	_ = CompileFacetCoverage(rm, surface, sink)
	for _, sig := range sink.signals {
		if sig.Kind == "facet_softened" {
			t.Errorf("unexpected softening signal on satisfied requirement: %+v", sig)
		}
	}
}

// TestResolveQuestionFamily_BucketedRoutesToComparison pins the
// R4.4 QFComparison routing: any RequestModel with Buckets >= 2
// routes to QFComparison regardless of Intent (comparison takes
// priority over enumeration / call-chain / generic so the user's
// mental partition survives end-to-end).
func TestResolveQuestionFamily_BucketedRoutesToComparison(t *testing.T) {
	cases := []struct {
		name   string
		intent Intent
	}{
		{"with_enumerate_intent", IntentEnumerate},
		{"with_explain_intent", IntentExplain},
		{"with_trace_intent", IntentTrace},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rm := RequestModel{
				Intent: tc.intent,
				Buckets: []QuestionBucket{
					{Label: "read mode", Index: 1},
					{Label: "write mode", Index: 2},
				},
			}
			got := ResolveQuestionFamily(rm)
			if got != QFComparison {
				t.Errorf("intent=%s: family = %q, want QFComparison", tc.intent, got)
			}
		})
	}
}

// TestResolveQuestionFamily_SingleBucketDoesNotRouteToComparison
// confirms only Buckets >= 2 triggers QFComparison;
// single-bucket / no-bucket questions route by Intent as before.
func TestResolveQuestionFamily_SingleBucketDoesNotRouteToComparison(t *testing.T) {
	rm := RequestModel{
		Intent: IntentEnumerate,
		Buckets: []QuestionBucket{
			{Label: "only one", Index: 1},
		},
	}
	got := ResolveQuestionFamily(rm)
	if got == QFComparison {
		t.Errorf("single-bucket must NOT route to QFComparison; got %q", got)
	}
	if got != QFEnumeration {
		t.Errorf("single-bucket enumeration: got %q, want QFEnumeration", got)
	}
}

func TestResolveQuestionFamily_NoSignalWhenSinkNil(t *testing.T) {
	// Calling without a sink must not panic and must not invoke any
	// telemetry side-effect — back-compat for tests / non-orchestrator
	// callers.
	rm := RequestModel{
		Intent: IntentEnumerate,
		Buckets: []QuestionBucket{
			{Label: "read mode", Index: 1},
			{Label: "write mode", Index: 2},
		},
	}
	_ = ResolveQuestionFamily(rm)
	_ = CompileFacetCoverage(rm, nil)
}

// TestMutableState_AppendRichnessTelemetry_Dedups confirms identical
// signals only land once even when many call sites fire.
func TestMutableState_AppendRichnessTelemetry_Dedups(t *testing.T) {
	mut := &MutableState{}
	sig := RichnessTelemetrySignal{Kind: "facet_softened", FacetKind: "X", Family: "QFGeneric", Reason: "r"}
	mut.AppendRichnessTelemetry(sig)
	mut.AppendRichnessTelemetry(sig)
	mut.AppendRichnessTelemetry(sig)
	got := mut.RichnessTelemetry()
	if len(got) != 1 {
		t.Errorf("expected dedup → len=1; got %d (%+v)", len(got), got)
	}

	// A signal that differs in any field is recorded separately.
	mut.AppendRichnessTelemetry(RichnessTelemetrySignal{
		Kind: "facet_softened", FacetKind: "Y", Family: "QFGeneric", Reason: "r",
	})
	if got := mut.RichnessTelemetry(); len(got) != 2 {
		t.Errorf("expected len=2 after distinct signal; got %d", len(got))
	}
}

// ───────────────────────────────────────────────────────────────────

func familyToRequestModel(f QuestionFamily) RequestModel {
	switch f {
	case QFRootCauseTrace:
		return RequestModel{Intent: IntentRootCause, LogTriage: &LogBundle{}}
	case QFConfigPrecedence:
		return RequestModel{Intent: IntentConfigQuery}
	case QFRoleLookup:
		return RequestModel{
			Intent:        IntentExplain,
			AnswerSubject: AnswerSubject{Kind: SubjectFunctionName, Confidence: 0.8},
		}
	case QFCallChain:
		return RequestModel{Intent: IntentTrace}
	case QFEnumeration:
		return RequestModel{Intent: IntentEnumerate}
	case QFArchitecture:
		return RequestModel{
			Intent: IntentExplain, Scenario: ScenarioArchitectureExplain,
		}
	case QFGeneric:
		return RequestModel{}
	}
	return RequestModel{}
}
