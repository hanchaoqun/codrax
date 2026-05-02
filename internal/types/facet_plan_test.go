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

func TestResolveQuestionFamily_EnumerationFromBuckets(t *testing.T) {
	rm := RequestModel{
		Intent: IntentExplain,
		Buckets: []QuestionBucket{
			{Label: "A", Index: 1},
			{Label: "B", Index: 2},
		},
	}
	if got := ResolveQuestionFamily(rm); got != QFEnumeration {
		t.Errorf("got %q, want QFEnumeration", got)
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
	// no log evidence → no candidate → HARD must degrade to SOFT.
	rm := RequestModel{
		Intent:    IntentRootCause,
		LogTriage: &LogBundle{},
	}
	plan := CompileFacetCoverage(rm, nil)
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
